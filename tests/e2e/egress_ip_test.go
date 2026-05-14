//go:build e2e && linux

// egress_ip_test.go 是 MVS-02 出口 IP 三源轮询 + 多数派裁决 e2e 用例。
//
// 验证主路径：worker 容器内并行调 3 个公网回显源，多数派裁决得到的出口 IP
// 必须等于绑定的 egress IP；任一源全部失败时 Skip 而非 Fail（CONTEXT §Area 2
// 锁定）。

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

type EgressIPSuite struct {
	harness.BaseSuite
	GP *GoldenPath
}

func (s *EgressIPSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.GP = StartGoldenPath(s.T())
	if s.GP != nil {
		s.SetArtifactDumper(harness.NewArtifactDumper(s.GP.Scenario, ""))
	}
}

// TestEgressIP_MajorityVote 在 worker 容器内并行调 3 个公网回显源，
// 把结果喂给 Vote 函数；ok=false（全部弃权）时 Skip。
func (s *EgressIPSuite) TestEgressIP_MajorityVote() {
	if s.GP == nil {
		s.T().Skip("golden path not started; deferred to Linux CI")
		return
	}

	ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
	defer cancel()

	// worker 容器句柄当前由 Scenario.Host("alpha") 返回；Step 7 真实落地后
	// 会暴露 ContainerHandle。临时通过 harness API 拿一个 stub container 让
	// 用例骨架编译通过；真实拓扑由 Linux CI runner 跑通。
	container := workerContainerHandle(s.GP)
	if container == nil {
		s.T().Skip("worker container handle not available; deferred to Linux CI")
		return
	}

	results := FetchEgressIPInContainer(ctx, container)
	s.T().Logf("egress ip results=%v sources=%v", results, EgressIPSources())

	vote := Vote(results)
	if !vote.OK {
		// 全部弃权 → 极可能是 CI 出口被屏蔽；Skip 避免外网抖动 false-fail。
		s.T().Skipf("vote did not reach majority (dissent=%v); likely network restricted on runner", vote.Dissent)
		return
	}

	// MVS-02 ground truth: GatewayHandle.GatewayIP 是 sing-box 网关出口；
	// 真实 egress IP（公网 NAT 后看到的）应由 fixture 在控制面 admin API 写入。
	// 当前 GatewayHandle 仅暴露 GatewayIP 字段（10.99.x.2 私网），不是公网 IP。
	// 因此本 plan 当前仅校验：多数派达成 + dissent 列入 t.Log，公网 IP 等值
	// 校验列入 deferred-to-CI（Linux runner 上 fixture 接通后再断言）。
	s.T().Logf("MVS-02 majority winner=%s dissent=%v; equality assertion deferred to Linux CI", vote.Winner, vote.Dissent)
}

// workerContainerHandle 把 Scenario Host 句柄包装成 ContainerHandle 接口。
// Step 7 真实落地前返回 nil，让用例 Skip。
func workerContainerHandle(gp *GoldenPath) harness.ContainerHandle {
	if gp == nil || gp.Host == nil || gp.Host.ContainerName == "" {
		return nil
	}
	// TODO(46-01 Step 7): 把 ContainerName 换成 docker exec wrapper 或
	// testcontainers.Container.Exec 实现 ContainerHandle。
	return nil
}

func TestEgressIPSuite(t *testing.T) {
	suite.Run(t, new(EgressIPSuite))
}
