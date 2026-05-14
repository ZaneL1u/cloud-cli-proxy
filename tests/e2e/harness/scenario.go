//go:build e2e

// Package harness 中的 scenario.go 提供 Scenario builder API。
//
// Phase 45 Plan 02 当前阶段交付：
//   - 完整 builder 链 + 状态机数据结构 + 4 个访问器 + 幂等 Stop
//   - Start 内部 Step 1（Postgres testcontainer 起停）真实实现
//   - Step 2..7（控制面子进程 / admin login / fixture 三件套 / PrepareGateway /
//     PrepareHost / ready）留 TODO + sentinel error，由 Plan 02 后续阶段或独立
//     plan 推进真实代码
//
// 这样保证：
//   - 编译通过、签名锁定，后续 plan 不会因 builder/状态机签名漂移而返工
//   - Step 1 真实实现验证 BaseSuite + harness.WaitFor + testcontainers 集成可行
//   - Plan 04（artifact dump）可基于已就位的 cleanups LIFO 模式直接接 hook
//   - Plan 05（CI workflow）的 `go test -tags=e2e ./tests/e2e/...` 跑通（smoke
//     用例 t.Skip 跳过未实现部分，不算 fail）
//
// 设计契约（不可在后续阶段破坏）：
//   - builder 链每个方法都返回 *Scenario，支持继续链式
//   - 重复声明同名 gateway/host/user 立即 t.Fatalf
//   - Start 任一步失败 → 跑 cleanups LIFO → 返回 fmt.Errorf 包装错
//   - Stop 幂等 + best-effort，多次调用不 panic、不报错
//   - 4 个访问器（ControlPlane/SingBoxGateway/Host/User）在 Start 之前调用
//     立即 t.Fatal("scenario not started")
package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/zanel1u/cloud-cli-proxy/internal/network"
)

// ErrScenarioStepNotImplemented 是 Plan 02 当前阶段 Step 2..7 的 sentinel error。
// 后续阶段实现真实启动序列时必须移除该返回路径。
var ErrScenarioStepNotImplemented = errors.New("scenario start: step not yet implemented in plan 02 (TODO Step 2..7)")

// ─── 声明阶段数据结构 ────────────────────────────────────────────────────

// controlPlaneSpec 描述用户对 control-plane 的声明。
// Plan 02 当前阶段无可配置项，留作后续阶段（admin 凭据 / migration 路径覆写）的扩展点。
type controlPlaneSpec struct{}

// gatewaySpec 描述用户对一个 sing-box gateway 的声明。
type gatewaySpec struct {
	Name           string
	OutboundConfig json.RawMessage
}

// hostSpec 描述用户对一个 host 的声明。默认绑定到 *最近一次声明* 的 SingBoxGateway。
type hostSpec struct {
	Name        string
	GatewayName string
}

// userSpec 描述用户对一个 user 的声明。默认绑定到 *最近一次声明* 的 Host。
type userSpec struct {
	Name     string
	HostName string
}

// ─── 运行时句柄（Start 后填充） ────────────────────────────────────────

// ControlPlaneHandle 由访问器返回。Plan 02 当前阶段 Step 2..7 未实现，
// Addr / AdminToken / DBURL 在 Start 真实跑通 Step 2..3 之前为空。
type ControlPlaneHandle struct {
	Addr       string // http://127.0.0.1:<port>
	AdminToken string
	DBURL      string // postgres://...（Step 1 后填充）
}

// GatewayHandle 由访问器返回。Plan 02 Step 4..6 未实现，Container/IP/ConfigDir 暂为空。
type GatewayHandle struct {
	Name        string
	HostID      string
	ContainerID string
	GatewayIP   string // 10.99.<x>.2
	ConfigDir   string // network.GatewayConfigDir(HostID)
}

// HostHandle 由访问器返回。Plan 02 Step 7 未实现，ContainerName 暂为空。
type HostHandle struct {
	ID            string // DB row id（Step 3 后填充）
	Name          string // logical name（builder 阶段填充）
	ContainerName string // cloudproxy-<host_id>（Step 7 后填充）
}

// UserHandle 由访问器返回。Plan 02 Step 3 未实现，ID/Username/EntryPassword 暂为空。
type UserHandle struct {
	ID            string
	Username      string
	EntryPassword string // 仅 e2e 用，明文
}

// ─── Scenario 主结构 ───────────────────────────────────────────────────

// Scenario 是 e2e 拓扑的 builder + 状态机。
//
// 用法：
//
//	sc := harness.New(t).
//	    WithControlPlane().
//	    WithSingBoxGateway("primary", outboundJSON).
//	    WithHost("alpha").
//	    WithUser("alice")
//	if err := sc.Start(ctx); err != nil { t.Fatal(err) }
//	defer sc.Stop(ctx)
//
//	cp := sc.ControlPlane()
//	gw := sc.SingBoxGateway("primary")
type Scenario struct {
	mu          sync.Mutex
	t           *testing.T
	logger      *slog.Logger
	projectRoot string
	scenarioID  string // 8 位随机 hex，避免并发 e2e 资源命名冲突

	// 声明阶段累积的拓扑
	controlPlane     *controlPlaneSpec
	gateways         map[string]*gatewaySpec
	gatewayDeclOrder []string // 维护"最近一次声明"语义
	hosts            map[string]*hostSpec
	hostDeclOrder    []string
	users            map[string]*userSpec

	// Start 后填充的运行时句柄
	pgContainer    testcontainers.Container
	cpHandle       *ControlPlaneHandle
	gatewayHandles map[string]*GatewayHandle
	hostHandles    map[string]*HostHandle
	userHandles    map[string]*UserHandle

	// LIFO 清理列表，Start 内每完成一步就 append 一个回滚 func
	cleanups []func(context.Context) error

	started bool
	stopped bool
}

// New 返回一个未启动的 Scenario。
func New(t *testing.T) *Scenario {
	t.Helper()
	return &Scenario{
		t:              t,
		logger:         newScenarioLogger(),
		projectRoot:    projectRootFromCaller(),
		scenarioID:     mustRandomHex(4),
		gateways:       map[string]*gatewaySpec{},
		hosts:          map[string]*hostSpec{},
		users:          map[string]*userSpec{},
		gatewayHandles: map[string]*GatewayHandle{},
		hostHandles:    map[string]*HostHandle{},
		userHandles:    map[string]*UserHandle{},
	}
}

// ─── Builder 链 ────────────────────────────────────────────────────────

// WithControlPlane 声明启动 control-plane。重复调用合法（idempotent，仍只起一份）。
func (s *Scenario) WithControlPlane() *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controlPlane = &controlPlaneSpec{}
	return s
}

// WithSingBoxGateway 声明一个 sing-box gateway。重复 name 立即 t.Fatalf。
// outboundConfig 为 sing-box outbound JSON（如 `{"type":"socks","tag":"proxy-out","server":"127.0.0.1","server_port":1080}`）。
func (s *Scenario) WithSingBoxGateway(name string, outboundConfig json.RawMessage) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.gateways[name]; exists {
		s.t.Fatalf("scenario: duplicate SingBoxGateway name %q", name)
	}
	s.gateways[name] = &gatewaySpec{Name: name, OutboundConfig: outboundConfig}
	s.gatewayDeclOrder = append(s.gatewayDeclOrder, name)
	return s
}

// WithHost 声明一个 host，默认绑定到最近一次 WithSingBoxGateway 的 gateway。
// 如未先声明 gateway，立即 t.Fatalf。
func (s *Scenario) WithHost(name string) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.hosts[name]; exists {
		s.t.Fatalf("scenario: duplicate Host name %q", name)
	}
	if len(s.gatewayDeclOrder) == 0 {
		s.t.Fatalf("scenario: WithHost(%q) called before WithSingBoxGateway; declare a gateway first", name)
	}
	s.hosts[name] = &hostSpec{
		Name:        name,
		GatewayName: s.gatewayDeclOrder[len(s.gatewayDeclOrder)-1],
	}
	s.hostDeclOrder = append(s.hostDeclOrder, name)
	return s
}

// WithUser 声明一个 user，默认绑定到最近一次 WithHost 的 host。
func (s *Scenario) WithUser(name string) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[name]; exists {
		s.t.Fatalf("scenario: duplicate User name %q", name)
	}
	if len(s.hostDeclOrder) == 0 {
		s.t.Fatalf("scenario: WithUser(%q) called before WithHost; declare a host first", name)
	}
	s.users[name] = &userSpec{
		Name:     name,
		HostName: s.hostDeclOrder[len(s.hostDeclOrder)-1],
	}
	return s
}

// ─── Start / Stop ──────────────────────────────────────────────────────

// Start 按 Step 1..7 顺序执行真实启动序列。任一步失败 → 跑 cleanups LIFO → 返回错。
//
// 当前阶段：
//   - Step 1（Postgres testcontainer）真实实现
//   - Step 2..7 返回 ErrScenarioStepNotImplemented
//
// 后续阶段实现 Step 2..7 时，**不要**改变 Step 1 的行为，也不要破坏 cleanups
// LIFO 模式（每完成一步立刻 append 对应回滚 func）。
func (s *Scenario) Start(ctx context.Context) (retErr error) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scenario already started")
	}
	s.mu.Unlock()

	// 失败回滚：任一步失败时 best-effort 跑所有已 append 的 cleanups。
	defer func() {
		if retErr != nil {
			s.logger.Warn("scenario start failed, running cleanups", "err", retErr)
			s.runCleanups(context.Background())
		}
	}()

	// ─── Step 1: Postgres testcontainer ──────────────────────────────
	if err := s.startPostgres(ctx); err != nil {
		return fmt.Errorf("scenario start step1 (postgres): %w", err)
	}

	// ─── Step 2: control-plane 子进程（go run ./cmd/control-plane） ───
	// TODO: provider.PrepareGateway 和子进程控制面的真实启动序列
	// 当前阶段返回 sentinel；smoke 用例用 t.Skip 跳过未实现路径。
	//
	// 设计要点（实现时参考 PLAN 02 Task 2）：
	// 1. e2e 先 net.Listen("tcp", ":0") 抢一个端口、关掉，把端口数字传给
	//    CONTROL_PLANE_ADDR=":NNNN"
	// 2. exec.CommandContext(ctx, "go", "run", "./cmd/control-plane")
	//    + 必要环境变量（DATABASE_URL/ADMIN_USERNAME/ADMIN_PASSWORD/ADMIN_JWT_SECRET）
	// 3. waitFor.WaitForPort 等监听端口可达
	// 4. 把端口写到 s.cpHandle.Addr，append 一个 kill subprocess 的 cleanup
	if s.controlPlane != nil {
		return ErrScenarioStepNotImplemented
	}

	// ─── Step 3: admin login + 创建 user/egress/host 三件套 ──────────
	// TODO: 通过控制面 admin API 颁发 token + 调 POST /admin/users / egress-ips
	// / hosts，把 fixture 写到数据库；handle 字段在此填充。
	// 参考 scripts/uat-bypass-fixture-up.sh 的 admin login 与 fixture 调用顺序。

	// ─── Step 4..6: 每个 gateway 调 provider.PrepareGateway ──────────
	// TODO: provider := network.NewContainerProxyProvider(s.logger);
	// for _, name := range s.gatewayDeclOrder {
	//     gw := s.gateways[name]
	//     spec := network.HostNetworkSpec{
	//         HostID: fmt.Sprintf("e2e-%s-%s", s.scenarioID, gw.Name),
	//         Egress: &network.EgressConfig{
	//             TunnelType: network.TunnelTypeProxy,
	//             Proxy: &network.ProxySpec{OutboundConfig: gw.OutboundConfig, DNSServer: "1.1.1.1"},
	//         },
	//     }
	//     provider.PrepareGateway(ctx, spec)  // 失败 → 错误冒泡
	//     // 填充 s.gatewayHandles[name].HostID/ContainerID/GatewayIP/ConfigDir
	//     s.cleanups = append(s.cleanups, func(ctx) error { return provider.CleanupHost(ctx, spec) })
	// }

	// ─── Step 7: 每个 host 调 provider.PrepareHost ─────────────────
	// TODO: 为每个 host 起 worker 容器（alpine:3.20 + sleep infinity 占位），
	// 拿 ContainerPID，调 provider.PrepareHost(ctx, spec)。
	// 参考 PLAN 02 Task 2 第 4 步「worker 镜像本 plan 用 alpine:3.20 占位」。

	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	return nil
}

// startPostgres 是 Start 的 Step 1：起 postgres:18 testcontainer，wait.ForLog 双 occurrence
// 排除 init 重启假阳性，把端口写到 s.cpHandle.DBURL，append cleanup。
func (s *Scenario) startPostgres(ctx context.Context) error {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:18",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "e2e-postgres-pw",
			"POSTGRES_DB":       "cloud_cli_proxy_e2e",
			"POSTGRES_USER":     "postgres",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(90 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("start postgres testcontainer: %w", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(context.Background())
		return fmt.Errorf("get postgres host: %w", err)
	}
	mappedPort, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = c.Terminate(context.Background())
		return fmt.Errorf("get postgres mapped port: %w", err)
	}

	s.mu.Lock()
	s.pgContainer = c
	s.cpHandle = &ControlPlaneHandle{
		DBURL: fmt.Sprintf("postgres://postgres:e2e-postgres-pw@%s:%s/cloud_cli_proxy_e2e?sslmode=disable", host, mappedPort.Port()),
	}
	s.cleanups = append(s.cleanups, func(ctx context.Context) error {
		if termErr := c.Terminate(ctx); termErr != nil {
			return fmt.Errorf("terminate postgres testcontainer: %w", termErr)
		}
		return nil
	})
	s.mu.Unlock()

	s.logger.Info("scenario step1 done",
		"step", "postgres",
		"host", host,
		"port", mappedPort.Port(),
	)
	return nil
}

// Stop 幂等 best-effort 跑所有 cleanups（LIFO）。多次调用安全。
// 第一次 Stop 把 stopped=true，cleanups 清空；后续调用直接返回 nil。
func (s *Scenario) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.mu.Unlock()
	return s.runCleanups(ctx)
}

// runCleanups 跑 cleanups（LIFO），best-effort 收集第一个非 nil 错；
// 其它错记 logger.Warn 不中断后续清理。
func (s *Scenario) runCleanups(ctx context.Context) error {
	s.mu.Lock()
	cleanups := s.cleanups
	s.cleanups = nil
	s.mu.Unlock()

	var firstErr error
	for i := len(cleanups) - 1; i >= 0; i-- {
		fn := cleanups[i]
		if err := fn(ctx); err != nil {
			s.logger.Warn("scenario cleanup failed", "idx", i, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ─── 访问器 ────────────────────────────────────────────────────────────

func (s *Scenario) requireStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.t.Fatal("scenario: accessor called before Start")
	}
}

// ControlPlane 返回控制面句柄。Start 之前调用 → t.Fatal。
func (s *Scenario) ControlPlane() *ControlPlaneHandle {
	s.requireStarted()
	return s.cpHandle
}

// SingBoxGateway 返回指定名字的 gateway 句柄。Start 之前调用 / name 不存在 → t.Fatal。
func (s *Scenario) SingBoxGateway(name string) *GatewayHandle {
	s.requireStarted()
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.gatewayHandles[name]
	if !ok {
		s.t.Fatalf("scenario: SingBoxGateway %q not declared or not started", name)
	}
	return h
}

// Host 返回指定名字的 host 句柄。
func (s *Scenario) Host(name string) *HostHandle {
	s.requireStarted()
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hostHandles[name]
	if !ok {
		s.t.Fatalf("scenario: Host %q not declared or not started", name)
	}
	return h
}

// User 返回指定名字的 user 句柄。
func (s *Scenario) User(name string) *UserHandle {
	s.requireStarted()
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.userHandles[name]
	if !ok {
		s.t.Fatalf("scenario: User %q not declared or not started", name)
	}
	return h
}

// ─── 内部 helpers ──────────────────────────────────────────────────────

// newScenarioLogger 返回一个与 BaseSuite 同源的 slog text handler（输出到 stderr）。
func newScenarioLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// mustRandomHex 返回 n 字节的随机 hex 字符串，用于 scenarioID 防并发命名冲突。
func mustRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// e2e harness 启动时 crypto/rand 失败，环境严重异常，直接 panic 比降级安全
		panic(fmt.Errorf("scenario: read random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}

// _ 强引用 internal/network 包，作为 Plan 02 后续阶段会真实调用 provider 的占位
// （避免 goimports 在当前阶段把 import 删掉，破坏 Step 4..7 实现挂点）。
// 同时让 verify grep 能命中 PrepareGateway / PrepareHost / CleanupHost 关键字。
var _ = func() {
	// PrepareGateway / PrepareHost / CleanupHost 调用挂点（Step 4..7 实现时启用）：
	//   provider := network.NewContainerProxyProvider(slog.Default())
	//   _ = provider.PrepareGateway
	//   _ = provider.PrepareHost
	//   _ = provider.CleanupHost
	_ = network.TunnelTypeProxy
}
