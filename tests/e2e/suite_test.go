//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

// SmokeSuite 是 Phase 45 Plan 01 的最小烟雾套件。
//
// 目的：证明 testcontainers-go + testify/suite + e2e build tag 三件套
// 在 docker 可用的机器上能正常起容器并跑断言。
//
// 它故意不依赖 Scenario builder（Plan 02）/ waitFor helper（Plan 03）/
// artifact dump（Plan 04），保持成为后续 plan 的反向独立基线 —— 即使后续
// helper 全坏了，本套件仍能用作"e2e 链路自检"的最小烟雾。
//
// 嵌入策略：嵌入 *值类型* harness.BaseSuite（不是 *harness.BaseSuite 指针）。
// 原因：testify suite.Run 在 new(SmokeSuite) 时通过反射调嵌入链上的
// (*Suite).SetT(t)，若嵌入指针类型则 BaseSuite 字段是 nil → SetT panic。
// 嵌入值类型时 BaseSuite.Suite 自动初始化为零值，方法 promote 安全。
type SmokeSuite struct {
	harness.BaseSuite
}

// TestPostgresReady 起一个 postgres:18 容器，等待其就绪日志（出现 2 次以
// 排除中间的 init 重启假阳性），然后在容器内执行 pg_isready，断言退出码 0。
//
// docker daemon 不可用时（如本机没启动 OrbStack）自动 t.Skip，避免在开发机
// 上跑 `go test -tags=e2e` 因环境缺失而 FAIL；CI 在 hosted ubuntu-24.04
// 上 docker 永远可用，会真实跑通。
func (s *SmokeSuite) TestPostgresReady() {
	if _, err := testcontainers.NewDockerProvider(); err != nil {
		s.T().Skipf("docker provider unavailable, skipping: %v", err)
	}

	ctx, cancel := context.WithTimeout(s.Ctx, 120*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:18",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "e2e-postgres-pw",
			"POSTGRES_DB":       "e2e",
			"POSTGRES_USER":     "postgres",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(90 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	s.Require().NoError(err, "start postgres testcontainer")
	defer func() {
		if termErr := container.Terminate(s.Ctx); termErr != nil {
			s.Logger.Warn("terminate postgres container", "err", termErr)
		}
	}()

	code, _, err := container.Exec(ctx, []string{"pg_isready", "-U", "postgres", "-d", "e2e"})
	s.Require().NoError(err, "exec pg_isready in container")
	s.Require().Equal(0, code, "pg_isready exit code")
}

// TestE2ESmokeSuite 是 go test 的入口；suite.Run 由 testify 提供。
func TestE2ESmokeSuite(t *testing.T) {
	suite.Run(t, new(SmokeSuite))
}
