//go:build linux

package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// GetContainerNetNS retrieves the network namespace handle and init PID
// for a running Docker container identified by name.
func GetContainerNetNS(containerName string) (netns.NsHandle, uint32, error) {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", containerName).Output()
	if err != nil {
		return 0, 0, &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("inspect container %s pid: %v", containerName, err),
		}
	}

	pid, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32)
	if err != nil {
		return 0, 0, &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("parse container pid: %v", err),
		}
	}
	if pid == 0 {
		return 0, 0, &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("container %s not running (pid=0)", containerName),
		}
	}

	const maxRetry = 5
	var lastErr error
	for attempt := 1; attempt <= maxRetry; attempt++ {
		// 每次重试前验证容器是否仍在运行
		runningOut, runErr := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName).Output()
		if runErr != nil || strings.TrimSpace(string(runningOut)) != "true" {
			if attempt < maxRetry {
				time.Sleep(300 * time.Millisecond)
				continue
			}
		}

		ns, nsErr := netns.GetFromPid(int(pid))
		if nsErr == nil {
			return ns, uint32(pid), nil
		}
		lastErr = nsErr
		if attempt < maxRetry {
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 最后一次失败时，获取容器状态信息嵌入错误
	statusOut, _ := exec.Command("docker", "inspect", "-f", "{{.State.Status}}|{{.State.ExitCode}}", containerName).Output()
	statusInfo := strings.TrimSpace(string(statusOut))
	if statusInfo == "" {
		statusInfo = "unknown"
	}

	return 0, 0, &NetworkError{
		Type:    ErrTunnelSetupFailed,
		Message: fmt.Sprintf("get netns from pid %d after %d attempts (container status=%s): %v", pid, maxRetry, statusInfo, lastErr),
	}
}

// InjectManagementVeth creates a veth pair between the host and container
// network namespaces for SSH management access. The container side intentionally
// has no default gateway to prevent it from becoming an egress bypass path.
func InjectManagementVeth(hostNS, containerNS netns.NsHandle, hostID string) (hostIP, containerIP string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	idx := mgmtSubnetIndex(hostID)
	block := idx / 128
	offset := (idx % 128) * 2

	hostAddr := fmt.Sprintf("10.99.%d.%d/30", block+1, offset+1)
	containerAddr := fmt.Sprintf("10.99.%d.%d/30", block+1, offset+2)

	hostVethName := fmt.Sprintf("mgmt-%s", truncateID(hostID, 8))
	containerVethName := "mgmt0"

	if err := netns.Set(hostNS); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("set host netns: %v", err),
		}
	}

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostVethName},
		PeerName:  containerVethName,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("create veth pair: %v", err),
		}
	}

	peer, err := netlink.LinkByName(containerVethName)
	if err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("find peer veth %s: %v", containerVethName, err),
		}
	}
	if err := netlink.LinkSetNsFd(peer, int(containerNS)); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("move peer to container netns: %v", err),
		}
	}

	hostLink, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("find host veth %s: %v", hostVethName, err),
		}
	}

	hostIPNet, err := netlink.ParseAddr(hostAddr)
	if err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("parse host addr %s: %v", hostAddr, err),
		}
	}
	if err := netlink.AddrAdd(hostLink, hostIPNet); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("add addr to host veth: %v", err),
		}
	}
	if err := netlink.LinkSetUp(hostLink); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("bring up host veth: %v", err),
		}
	}

	if err := netns.Set(containerNS); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("set container netns: %v", err),
		}
	}
	defer netns.Set(hostNS) //nolint:errcheck

	containerLink, err := netlink.LinkByName(containerVethName)
	if err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("find container veth %s: %v", containerVethName, err),
		}
	}

	containerIPNet, err := netlink.ParseAddr(containerAddr)
	if err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("parse container addr %s: %v", containerAddr, err),
		}
	}
	if err := netlink.AddrAdd(containerLink, containerIPNet); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("add addr to container veth: %v", err),
		}
	}
	if err := netlink.LinkSetUp(containerLink); err != nil {
		return "", "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("bring up container veth: %v", err),
		}
	}

	hostIPOnly, _, _ := net.ParseCIDR(hostAddr)
	containerIPOnly, _, _ := net.ParseCIDR(containerAddr)

	return hostIPOnly.String(), containerIPOnly.String(), nil
}

// mgmtSubnetIndex derives a unique /30 subnet index from hostID
// to avoid address collisions across concurrent containers.
func mgmtSubnetIndex(hostID string) uint16 {
	b := []byte(hostID)
	if len(b) < 4 {
		padded := make([]byte, 4)
		copy(padded, b)
		b = padded
	}
	return binary.BigEndian.Uint16(b[:2]) ^ binary.BigEndian.Uint16(b[2:4])%16382
}
