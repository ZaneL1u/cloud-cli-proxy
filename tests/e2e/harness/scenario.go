//go:build e2e

// Package harness 中的 scenario.go 提供 Scenario builder API。
//
// v4.0 (Phase 55) 单容器化重构：
//   - 删除 gatewaySpec / GatewayHandle / WithSingBoxGateway / SingBoxGateway
//   - 合并到 userSpec / User，outbound 通过 WithOutboundConfig 设置
//   - WithHost 不再要求先声明 gateway
//   - 4 个访问器改为 3 个（ControlPlane/Host/User）
//
// 设计契约（不可在后续阶段破坏）：
//   - builder 链每个方法都返回 *Scenario，支持继续链式
//   - 重复声明同名 host/user 立即 t.Fatalf
//   - Start 任一步失败 → 跑 cleanups LIFO → 返回 fmt.Errorf 包装错
//   - Stop 幂等 + best-effort，多次调用不 panic、不报错
//   - 3 个访问器（ControlPlane/Host/User）在 Start 之前调用
//     立即 t.Fatal("scenario not started")
package harness

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// ErrScenarioStepNotImplemented 是 Plan 02 当前阶段 Step 2..7 的 sentinel error。
var ErrScenarioStepNotImplemented = errors.New("scenario start: step not yet implemented in plan 02 (TODO Step 2..7)")

// ─── 声明阶段数据结构 ────────────────────────────────────────────────────

// controlPlaneSpec 描述用户对 control-plane 的声明。
type controlPlaneSpec struct {
	ExtraEnv map[string]string
}

// hostSpec 描述用户对一个 host 的声明。
// v4.0 (Phase 55): GatewayName 字段已删除，单容器架构下 host 直接绑定 outbound。
type hostSpec struct {
	Name string
}

// userSpec 描述用户对一个 user 的声明。默认绑定到 *最近一次声明* 的 Host。
// v4.0 (Phase 55): 新增 OutboundConfig 字段，原 gatewaySpec 合并到此处。
type userSpec struct {
	Name           string
	HostName       string
	OutboundConfig json.RawMessage
}

// ─── 运行时句柄（Start 后填充） ────────────────────────────────────────

// ControlPlaneHandle 由访问器返回。
type ControlPlaneHandle struct {
	Addr       string // http://127.0.0.1:<port>
	AdminToken string
	DBURL      string // SQLite file path（Step 1 后填充）
}

// HostHandle 由访问器返回。
type HostHandle struct {
	ID            string // DB row id（Step 3 后填充）
	Name          string // logical name（builder 阶段填充）
	ContainerName string // cloudproxy-<host_id>（Step 7 后填充）
}

// UserHandle 由访问器返回。
type UserHandle struct {
	ID            string
	Username      string
	EntryPassword string // 仅 e2e 用，明文
}

// ─── Scenario 主结构 ───────────────────────────────────────────────────

// Scenario 是 e2e 拓扑的 builder + 状态机。
//
// v4.0 单容器用法：
//
//	sc := harness.New(t).
//	    WithControlPlane().
//	    WithOutboundConfig(outboundJSON).
//	    WithHost("alpha").
//	    WithUser("alice")
//	if err := sc.Start(ctx); err != nil { t.Fatal(err) }
//	defer sc.Stop(ctx)
//
//	cp := sc.ControlPlane()
//	host := sc.Host("alpha")
//	user := sc.User("alice")
type Scenario struct {
	mu          sync.Mutex
	t           *testing.T
	logger      *slog.Logger
	projectRoot string
	scenarioID  string // 8 位随机 hex，避免并发 e2e 资源命名冲突

	// 声明阶段累积的拓扑
	controlPlane  *controlPlaneSpec
	outboundConfig json.RawMessage // v4.0: 全局 outbound（替代 v3.6 gatewaySpec.OutboundConfig）
	hosts         map[string]*hostSpec
	hostDeclOrder []string
	users         map[string]*userSpec

	// Start 后填充的运行时句柄
	cpHandle    *ControlPlaneHandle
	hostHandles map[string]*HostHandle
	userHandles map[string]*UserHandle

	// LIFO 清理列表，Start 内每完成一步就 append 一个回滚 func
	cleanups []func(context.Context) error

	started bool
	stopped bool
}

// New 返回一个未启动的 Scenario。
func New(t *testing.T) *Scenario {
	t.Helper()
	return &Scenario{
		t:           t,
		logger:      newScenarioLogger(),
		projectRoot: projectRootFromCaller(),
		scenarioID:  mustRandomHex(4),
		hosts:       map[string]*hostSpec{},
		users:       map[string]*userSpec{},
		hostHandles: map[string]*HostHandle{},
		userHandles: map[string]*UserHandle{},
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

// WithControlPlaneEnv 注入额外的环境变量给控制面子进程。
func (s *Scenario) WithControlPlaneEnv(envs map[string]string) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controlPlane == nil {
		s.t.Fatalf("scenario: WithControlPlaneEnv called before WithControlPlane")
	}
	if s.controlPlane.ExtraEnv == nil {
		s.controlPlane.ExtraEnv = map[string]string{}
	}
	for k, v := range envs {
		s.controlPlane.ExtraEnv[k] = v
	}
	return s
}

// WithOutboundConfig 设置全局 outbound config（sing-box proxy outbound JSON）。
// v4.0 (Phase 55): 替代 v3.6 WithSingBoxGateway，outbound 直接与 scenario 关联，
// 由 PrepareHost 写入容器内 sing-box config。
func (s *Scenario) WithOutboundConfig(outboundConfig json.RawMessage) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outboundConfig = outboundConfig
	return s
}

// WithHost 声明一个 host。v4.0 单容器架构下不再要求先声明 gateway。
func (s *Scenario) WithHost(name string) *Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.hosts[name]; exists {
		s.t.Fatalf("scenario: duplicate Host name %q", name)
	}
	s.hosts[name] = &hostSpec{Name: name}
	s.hostDeclOrder = append(s.hostDeclOrder, name)
	return s
}

// WithUser 声明一个 user，默认绑定到最近一次 WithHost 的 host。
// 如果已通过 WithOutboundConfig 设置全局 outbound，自动关联到 user。
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
		Name:           name,
		HostName:       s.hostDeclOrder[len(s.hostDeclOrder)-1],
		OutboundConfig: s.outboundConfig,
	}
	return s
}

// ─── Start / Stop ──────────────────────────────────────────────────────

// Start 按 Step 1..7 顺序执行真实启动序列。任一步失败 → 跑 cleanups LIFO → 返回错。
func (s *Scenario) Start(ctx context.Context) (retErr error) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scenario already started")
	}
	s.mu.Unlock()

	defer func() {
		if retErr != nil {
			s.logger.Warn("scenario start failed, running cleanups", "err", retErr)
			s.runCleanups(context.Background())
		}
	}()

	// ─── Step 1: SQLite ───────────────────────────────────────────────
	if err := s.startSQLite(ctx); err != nil {
		return fmt.Errorf("scenario start step1 (sqlite): %w", err)
	}

	// ─── Step 2: control-plane 子进程 ─────────────────────────────────
	if s.controlPlane != nil {
		return ErrScenarioStepNotImplemented
	}

	// ─── Step 3: admin login + fixture ────────────────────────────────

	// ─── Step 4: PrepareHost（v4.0 单容器, 替代 v3.6 PrepareGateway） ──
	// v4.0: host-agent PrepareHost 写入 sing-box config 到容器内，
	// 使用 s.outboundConfig 作为 outbound。

	// ─── Step 5: ready ───────────────────────────────────────────────

	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	return nil
}

// startSQLite 是 Start 的 Step 1：创建临时 SQLite 数据库并通过 DATABASE_URL 传给子进程。
func (s *Scenario) startSQLite(ctx context.Context) error {
	dbPath := filepath.Join(os.TempDir(), "cloud-cli-proxy-e2e-"+s.scenarioID+".db")

	// 创建 SQLite 数据库文件并验证连通性。
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_texttotime=true")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("ping sqlite: %w", err)
	}
	db.Close()

	s.mu.Lock()
	s.cpHandle = &ControlPlaneHandle{
		DBURL: "file:" + dbPath + "?_texttotime=true",
	}
	s.cleanups = append(s.cleanups, func(_ context.Context) error {
		return os.Remove(dbPath)
	})
	s.mu.Unlock()

	s.logger.Info("scenario step1 done",
		"step", "sqlite",
		"dbPath", dbPath,
	)
	return nil
}

// Stop 幂等 best-effort 跑所有 cleanups（LIFO）。多次调用安全。
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

func newScenarioLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func mustRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("scenario: read random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}
