package sshproxy

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// channelOpenDirectMsg is the SSH wire format for direct-tcpip channel open.
// Field order must match the SSH RFC 4253 encoding.
type channelOpenDirectMsg struct {
	Raddr string `sshtype:"string"`
	Rport uint32 `sshtype:"uint32"`
	Laddr string `sshtype:"string"`
	Lport uint32 `sshtype:"uint32"`
}

var forbiddenCIDRs = []string{"10.99.0.0/16"}

var forbiddenHosts = []string{
	"metadata.google.internal",
	"169.254.169.254",
}

var forbiddenPorts = map[int]bool{2375: true, 2376: true}

// isForbiddenTarget checks whether a forwarding target should be blocked.
// Blocked targets include management subnets, cloud metadata endpoints, and
// Docker socket ports.
func isForbiddenTarget(host string, port int) bool {
	if forbiddenPorts[port] {
		return true
	}
	for _, h := range forbiddenHosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	ip := net.ParseIP(host)
	if ip != nil {
		for _, cidr := range forbiddenCIDRs {
			_, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if ipNet.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// dialContainer establishes an SSH connection to the target container.
// Authentication uses private key if available, falling back to password.
func (s *Server) dialContainer(targetAddr, targetUser, targetPassword, targetPrivateKey string) (*ssh.Client, error) {
	user := targetUser
	if user == "" {
		user = s.containerUser
	}
	pass := targetPassword
	if pass == "" {
		pass = s.containerPassword
	}

	var authMethods []ssh.AuthMethod
	if targetPrivateKey != "" && (strings.Contains(targetPrivateKey, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN RSA PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN EC PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN DSA PRIVATE KEY")) {
		signer, err := ssh.ParsePrivateKey([]byte(targetPrivateKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			s.logger.Debug("SSH proxy using private key auth for container", "addr", targetAddr, "user", user)
		} else {
			s.logger.Warn("SSH proxy failed to parse target private key, falling back to password", "error", err)
		}
	}
	authMethods = append(authMethods, ssh.Password(pass), passwordKeyboardInteractive(pass), ssh.PublicKeys(s.hostKey))

	targetConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	return ssh.Dial("tcp", targetAddr, targetConfig)
}

// handleDirectTCPIP proxies a direct-tcpip channel request from the client
// to the target container. It validates the target against the security
// blocklist before forwarding.
func (s *Server) handleDirectTCPIP(newChan ssh.NewChannel, targetAddr, targetUser, targetPassword, targetPrivateKey string) {
	var msg channelOpenDirectMsg
	if err := ssh.Unmarshal(newChan.ExtraData(), &msg); err != nil {
		s.logger.Warn("direct-tcpip payload unmarshal failed", "error", err)
		newChan.Reject(ssh.ConnectionFailed, "invalid direct-tcpip payload")
		return
	}

	// SSH-04: security validation — reject forwarding to forbidden targets.
	if isForbiddenTarget(msg.Raddr, int(msg.Rport)) {
		s.logger.Warn("forwarding to forbidden target rejected", "raddr", msg.Raddr, "rport", msg.Rport)
		newChan.Reject(ssh.Prohibited, "forwarding to this target is not allowed")
		return
	}

	clientChan, clientReqs, err := newChan.Accept()
	if err != nil {
		s.logger.Error("accept direct-tcpip channel failed", "error", err)
		return
	}
	defer clientChan.Close()
	go ssh.DiscardRequests(clientReqs)

	targetClient, err := s.dialContainer(targetAddr, targetUser, targetPassword, targetPrivateKey)
	if err != nil {
		s.logger.Error("dial container for direct-tcpip failed", "target", targetAddr, "error", err)
		fmt.Fprintf(clientChan.Stderr(), "forwarding failed: %v\r\n", err)
		return
	}
	defer targetClient.Close()

	targetChan, targetReqs, err := targetClient.OpenChannel("direct-tcpip", ssh.Marshal(&msg))
	if err != nil {
		s.logger.Error("open target direct-tcpip failed", "target", targetAddr, "error", err)
		fmt.Fprintf(clientChan.Stderr(), "forwarding failed: %v\r\n", err)
		return
	}
	defer targetChan.Close()
	go ssh.DiscardRequests(targetReqs)

	// Bidirectional concurrent copy with CloseWrite signaling.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(targetChan, clientChan)
		targetChan.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientChan, targetChan)
		clientChan.CloseWrite()
	}()
	wg.Wait()

	s.logger.Debug("direct-tcpip channel closed", "target", targetAddr, "raddr", msg.Raddr, "rport", msg.Rport)
}
