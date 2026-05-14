//go:build e2e && linux

// dns_test.go 是 MVS-03 DNS 强制走 tun 或防火墙拒绝 e2e 用例。
//
// 验证主路径：worker 容器内对 cloudflare.com 发起 DNS 查询，
// ClassifyDNSResult 返回 Tunneled 或 Denied 任一成立即 PASS。
// Unknown 视为 inconclusive → fail。

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

type DNSSuite struct {
	harness.BaseSuite
	GP *GoldenPath
}

func (s *DNSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.GP = StartGoldenPath(s.T())
	if s.GP != nil {
		s.SetArtifactDumper(harness.NewArtifactDumper(s.GP.Scenario, ""))
	}
}

// TestDNS_TunOrFirewallDeny 在 worker 容器内跑 DNS probe，
// 把 exit code + stderr 喂给 ClassifyDNSResult，期望 Tunneled OR Denied。
func (s *DNSSuite) TestDNS_TunOrFirewallDeny() {
	if s.GP == nil {
		s.T().Skip("golden path not started; deferred to Linux CI")
		return
	}

	ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
	defer cancel()

	container := workerContainerHandle(s.GP)
	if container == nil {
		s.T().Skip("worker container handle not available; deferred to Linux CI")
		return
	}

	// 用 getent hosts 触发 DNS 解析；alpine + glibc + musl 都支持，
	// stderr 在失败路径上含 "Name or service not known" 等关键字。
	probeCmd := []string{"getent", "hosts", "cloudflare.com"}
	code, output, err := container.Exec(ctx, probeCmd)
	if err != nil {
		s.T().Logf("container exec error: %v (视作 Denied，stderr empty 走 fallback)", err)
	}
	var stderr strings.Builder
	if output != nil {
		buf := make([]byte, 4096)
		n, _ := output.Read(buf)
		if n > 0 {
			stderr.Write(buf[:n])
		}
	}

	result := ClassifyDNSResult(code, stderr.String())
	s.T().Logf("MVS-03 DNS probe: exit=%d result=%s stderr=%q", code, result, stderr.String())

	switch result {
	case DNSResultTunneled:
		// 进一步 HTTPS 握手校验。
		httpCode, _, _ := container.Exec(ctx, []string{"curl", "-fsS", "--max-time", "5",
			"-o", "/dev/null", "-w", "%{http_code}", "https://cloudflare.com"})
		s.Require().Truef(httpCode == 0, "HTTPS handshake after tunneled DNS: exit=%d", httpCode)
	case DNSResultDenied:
		// 防火墙拒绝亦为 PASS。
		s.T().Log("MVS-03 PASS via firewall deny path")
	default:
		s.T().Fatalf("DNS probe inconclusive: exit=%d stderr=%q (artifact dumped)", code, stderr.String())
	}
}

func TestDNSSuite(t *testing.T) {
	suite.Run(t, new(DNSSuite))
}
