//go:build e2e && linux

// helpers_linux.go 收纳 Phase 46 中依赖 docker / linux netns / testcontainers
// 的 e2e 入口与容器侧 helper。darwin 上不参与编译（保护本地 `go build ./...`
// 与 `go test ./tests/e2e/ -run Helpers` 的清洁度）。
//
// 关键约定：
//   - 任一前置缺失（无 docker / Scenario.Start 仍是 Step 2..7 sentinel）→ t.Skip。
//   - 这里只放「需要 GoldenPath 句柄 / Container Exec / 控制面 admin API」的
//     函数；其它纯函数（Vote / Classify / Summarize）放 helpers.go 共享。

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

// GoldenPath 封装 Phase 46 MVS 所需的完整 e2e 拓扑：
// 控制面 + host-agent + Postgres + sing-box gateway + 1 user + 1 host。
//
// 字段在 StartGoldenPath 成功返回后才填充；用例代码不应自行构造 GoldenPath。
type GoldenPath struct {
	Scenario        *harness.Scenario
	Gateway         *harness.GatewayHandle
	Host            *harness.HostHandle
	User            *harness.UserHandle
	ControlPlaneURL string

	// BootstrapScript 指向 deploy/bootstrap/cloud-bootstrap.sh 的项目相对路径，
	// e2e 用例通过 exec.CommandContext("bash", g.BootstrapScript) 起子进程。
	BootstrapScript string
}

// StartGoldenPath 启动并返回 GoldenPath 句柄。
//
// 行为约定：
//   - 任一前置缺失（无 docker daemon / Scenario.Start 命中 Phase 45 Plan 02
//     Step 2..7 sentinel 错误）→ t.Skip 并返回 nil。
//   - 用例代码必须先判 `if g == nil { return }` 再访问 GoldenPath 字段，
//     避免对 nil 解引用。
//   - Cleanup 通过 t.Cleanup(func(){ scenario.Stop }) 注册，调用者无需手动 Stop。
//
// 失败 fast path：除了 Skip 之外的硬错（控制面真的启动不起来、PrepareGateway
// 真的报错）通过 t.Fatalf 上抛，让 CI 上的失败立刻冒泡。
func StartGoldenPath(t *testing.T) *GoldenPath {
	t.Helper()

	if _, err := testcontainers.NewDockerProvider(); err != nil {
		t.Skipf("docker provider unavailable, skipping golden path: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)

	outbound := json.RawMessage(`{"type":"direct","tag":"proxy-out"}`)
	sc := harness.New(t).
		WithControlPlane().
		WithSingBoxGateway("primary", outbound).
		WithHost("alpha").
		WithUser("alice")

	if err := sc.Start(ctx); err != nil {
		// Phase 45 Plan 02 当前 Step 2..7 仍是 sentinel。
		// 把它转 Skip，让 Phase 46 用例骨架先合入；真实拓扑由 CI runner 在
		// Scenario.Start Step 2..7 实现完成后自然解锁。
		if errors.Is(err, harness.ErrScenarioStepNotImplemented) {
			t.Skipf("scenario step 2..7 not yet implemented (Phase 45 follow-up); deferred to Linux CI: %v", err)
			return nil
		}
		t.Fatalf("StartGoldenPath: scenario start: %v", err)
		return nil
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer stopCancel()
		_ = sc.Stop(stopCtx)
	})

	cp := sc.ControlPlane()
	gw := sc.SingBoxGateway("primary")
	host := sc.Host("alpha")
	user := sc.User("alice")

	return &GoldenPath{
		Scenario:        sc,
		Gateway:         gw,
		Host:            host,
		User:            user,
		ControlPlaneURL: cp.Addr,
		BootstrapScript: projectRelativePath("deploy/bootstrap/cloud-bootstrap.sh"),
	}
}

// projectRelativePath 返回项目根 + 相对路径；禁绝对路径硬编码。
func projectRelativePath(rel string) string {
	_, file, _, _ := runtime.Caller(0) // tests/e2e/helpers_linux.go
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, rel)
}

// ipv4Re 抽出回显文本中的第一个 IPv4 字面量；空字符串表示未抓到。
var ipv4Re = regexp.MustCompile(`\b(\d{1,3}\.){3}\d{1,3}\b`)

// FetchEgressIPInContainer 并行调容器内的 curl 拉 EgressIPSources() 的 3 源，
// 返回结果切片（顺序对齐 EgressIPSources()）。某源 timeout / 非 200 → 对应
// 位置空字符串。
//
// 单源 5s 超时；总 ctx 由调用方决定（推荐 15s）。
func FetchEgressIPInContainer(ctx context.Context, c harness.ContainerHandle) []string {
	sources := EgressIPSources()
	results := make([]string, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			cmd := []string{"curl", "-fsS", "--max-time", "5", url}
			code, reader, err := c.Exec(ctx, cmd)
			if err != nil || code != 0 || reader == nil {
				return
			}
			body, err := io.ReadAll(io.LimitReader(reader, 1024))
			if err != nil || len(body) == 0 {
				return
			}
			results[idx] = ipv4Re.FindString(string(body))
		}(i, src)
	}
	wg.Wait()
	return results
}

// RunBootstrapScript 起子进程跑 deploy/bootstrap/cloud-bootstrap.sh，喂 stdin，
// 返回 exitCode + stdout + stderr。MVS-05 用例与 MVS-01 用例共用。
//
// 行为约定：
//   - 通过 *exec.ExitError 解包 exit code；进程正常退出 → exit 0。
//   - exec.CommandContext 启动失败（如脚本不存在）→ exitCode=-1, err 非 nil。
//   - 调用方应通过 context 控制总超时；本函数自身不设硬超时。
func RunBootstrapScript(
	ctx context.Context,
	scriptPath string,
	env []string,
	stdin string,
) (exitCode int, stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr == nil {
		return 0, stdout, stderr, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), stdout, stderr, nil
	}
	return -1, stdout, stderr, fmt.Errorf("run bootstrap script: %w", runErr)
}

// ControlPlaneHealthURL 拼接 GoldenPath.ControlPlaneURL 的 /healthz 入口。
// 用例可直接拿来喂 harness.WaitForHTTP。
func (g *GoldenPath) ControlPlaneHealthURL() string {
	if g == nil {
		return ""
	}
	base := strings.TrimRight(g.ControlPlaneURL, "/")
	return base + "/healthz"
}

// SeedBootstrapErrorFixtures 把 tests/e2e/fixtures/error-codes.sql 灌进控制面
// Postgres，让 MVS-05 用例的 disabled / expired / no-host 用户预先存在。
//
// 实现策略：通过控制面 admin API 创建 user（避免直接连 Postgres，保持 e2e 走
// 真实生产路径）。如果 admin API 不支持 disabled/expired 状态字段，则 fallback
// 到直接 SQL 注入；fallback 实现由 Phase 46 Plan 05 落地时补全。
//
// 当前阶段：返回 nil 占位（实际灌种放在 cli_error_codes_test.go 内联，配合
// admin API 真实路径）。
func SeedBootstrapErrorFixtures(_ context.Context, _ *GoldenPath) error {
	// TODO(46-05): 通过 admin API + 直接 SQL 双路径灌种 disabled/expired/no-host 用户。
	// 当前阶段不阻塞 build，CI runner 接通 Step 2..7 后再补全。
	return nil
}

// ─── Phase 47 Plan 01 / MVS-06：到期治理 ────────────────────────────

// SimulateExpiry 把 user 的 expires_at 拉到过去 1 秒，等价于该 user 立刻到期。
//
// 行为：
//   - 直接连 Scenario.ControlPlane().DBURL，UPDATE users.expires_at = NOW() - 1s。
//   - 不调 ExpiryScanner.Scan()；通过生产路径上的 EXPIRY_SCAN_INTERVAL=1s 让真实
//     scanner 在下一 tick 触发，避免绕过 scheduler 包裹层。
//   - waitForTick=true：调 harness.WaitFor 30s 内轮询 events 表中是否出现
//     type='user.expired' AND user_id=$1，等到出现为止。
//   - waitForTick=false：UPDATE 返回即返回，由调用方自己等。
//
// 注意：本函数依赖 DBURL 字段；GoldenPath / scenario.startPostgres 必须先完成
// Step 1。Step 2..7 仍 sentinel 的当下，本函数仅供 Linux runner 在 Step 完整
// 落地后使用；darwin 上整个测试经 t.Skip 跳过。
func (p *GoldenPath) SimulateExpiry(ctx context.Context, userID string, waitForTick bool) error {
	if p == nil || p.Scenario == nil {
		return errors.New("simulate expiry: golden path not initialized")
	}
	cp := p.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		return errors.New("simulate expiry: control plane DBURL empty")
	}

	conn, err := pgx.Connect(ctx, cp.DBURL)
	if err != nil {
		return fmt.Errorf("simulate expiry: connect db: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	tag, err := conn.Exec(ctx,
		`UPDATE users SET expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("simulate expiry: update users: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("simulate expiry: user %q not found", userID)
	}

	if !waitForTick {
		return nil
	}

	return harness.WaitFor(ctx, fmt.Sprintf("user.expired:%s", userID),
		func(ctx context.Context) error {
			var hits int
			row := conn.QueryRow(ctx,
				`SELECT COUNT(*) FROM events WHERE type = $1 AND user_id = $2`,
				UserExpiredEventType, userID,
			)
			if err := row.Scan(&hits); err != nil {
				return fmt.Errorf("scan events count: %w", err)
			}
			if hits == 0 {
				return fmt.Errorf("user.expired event not yet recorded for %s", userID)
			}
			return nil
		},
		harness.WithTimeout(30*time.Second),
		harness.WithPollInterval(500*time.Millisecond),
	)
}

// ─── Phase 47 Plan 02 / MVS-07：出口 IP 双绑互斥 ─────────────────────

// adminLoginRequest / adminLoginResponse 对应 POST /v1/auth/login 当前 schema。
type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminLoginResponse struct {
	Token string `json:"token"`
}

// AdminLogin 通过 POST /v1/auth/login 拿一个 admin JWT。
//
// 复用 Phase 46 admin fixture 入路；用户名 / 密码当前从环境变量
// E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD 取（默认 admin / admin-pw，与
// Phase 46 Plan 01 §Step 2 scenario.go TODO 注释中描述的一致）。
//
// 缺 token 字段 / 非 200 → 返回错误。
func (p *GoldenPath) AdminLogin(ctx context.Context) (string, error) {
	if p == nil || p.ControlPlaneURL == "" {
		return "", errors.New("admin login: control plane URL empty")
	}
	username := strings.TrimSpace(getEnvOrDefault("E2E_ADMIN_USERNAME", "admin"))
	password := getEnvOrDefault("E2E_ADMIN_PASSWORD", "admin-pw")

	payload, _ := json.Marshal(adminLoginRequest{Username: username, Password: password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.ControlPlaneURL, "/")+"/v1/auth/login",
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", fmt.Errorf("admin login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := disableKeepAliveClient(5 * time.Second).Do(req)
	if err != nil {
		return "", fmt.Errorf("admin login: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("admin login: status %d body=%s", resp.StatusCode, string(body))
	}
	var parsed adminLoginResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("admin login: decode body: %w", err)
	}
	if parsed.Token == "" {
		return "", errors.New("admin login: empty token in response")
	}
	return parsed.Token, nil
}

// bindEgressIPRequest 对应 POST /v1/admin/bindings 的请求 schema（admin_bindings.go::Bind）。
type bindEgressIPRequest struct {
	HostID     string `json:"host_id"`
	EgressIPID string `json:"egress_ip_id"`
}

// PostBindEgressIP 调 POST /v1/admin/bindings 绑一个 egress IP 到一个 host。
//
// 返回 BindEgressIPResponse，包含 status code、error message（若有）、raw body。
// 401 / 403 等鉴权错由调用方自行判断；本函数不区分。
func (p *GoldenPath) PostBindEgressIP(ctx context.Context, hostID, egressIPID string) (BindEgressIPResponse, error) {
	if p == nil || p.ControlPlaneURL == "" {
		return BindEgressIPResponse{}, errors.New("bind egress: control plane URL empty")
	}
	token, err := p.AdminLogin(ctx)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: admin login: %w", err)
	}
	payload, _ := json.Marshal(bindEgressIPRequest{HostID: hostID, EgressIPID: egressIPID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.ControlPlaneURL, "/")+"/v1/admin/bindings",
		bytes.NewReader(payload),
	)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := disableKeepAliveClient(10 * time.Second).Do(req)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
	out, err := ParseBindEgressIPResponse(resp.StatusCode, body)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: parse: %w", err)
	}
	return out, nil
}

// QueryBindingExists 直接连 DB 查 (host_id, egress_ip_id) 绑定行是否存在。
//
// 用例在「断言 A 原绑定不被破坏」时使用。通过 admin API GET /v1/admin/hosts/{X}
// 也能查，但 schema 经多次演进；直查 host_egress_bindings 表更稳。
func (p *GoldenPath) QueryBindingExists(ctx context.Context, hostID, egressIPID string) (bool, error) {
	if p == nil || p.Scenario == nil {
		return false, errors.New("query binding: golden path not initialized")
	}
	cp := p.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		return false, errors.New("query binding: control plane DBURL empty")
	}
	conn, err := pgx.Connect(ctx, cp.DBURL)
	if err != nil {
		return false, fmt.Errorf("query binding: connect db: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var hits int
	row := conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM host_egress_bindings WHERE host_id = $1 AND egress_ip_id = $2`,
		hostID, egressIPID,
	)
	if err := row.Scan(&hits); err != nil {
		return false, fmt.Errorf("query binding: scan: %w", err)
	}
	return hits > 0, nil
}

// ─── Phase 47 Plan 03 / MVS-08：host-agent 心跳与恢复 ────────────────

// KillHostAgent 在 host-agent 所在容器内执行 `pkill -9 -f host-agent`，
// 杀进程但不杀容器，让容器内 supervisor（dumb-init/systemd/supervisord）拉起。
//
// 行为约定：
//   - GoldenPath 当前没有 host-agent 容器句柄字段；本函数通过 host-agent 容器名
//     约定（沿用 v1 单宿主机 deploy 风格的 `host-agent` 容器）调 docker exec。
//   - 容器名通过 E2E_HOST_AGENT_CONTAINER 环境变量覆盖；默认 `host-agent`。
//   - embedded 模式下没有独立 host-agent 容器；调用方应先用
//     IsEmbeddedHostAgent() 判断，embedded 则 t.Skip 本用例。
//
// 不用 docker kill 整容器：CONTEXT §Area 3 决策——契约是「进程级恢复」，杀容器
// 会绕过被测路径。
func (p *GoldenPath) KillHostAgent(ctx context.Context) error {
	if p == nil {
		return errors.New("kill host-agent: golden path not initialized")
	}
	containerName := getEnvOrDefault("E2E_HOST_AGENT_CONTAINER", "host-agent")
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"sh", "-c", "pkill -9 -f host-agent")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kill host-agent: docker exec %s: %w (stderr=%s)",
			containerName, err, stderr.String())
	}
	return nil
}

// IsEmbeddedHostAgent 返回当前控制面是否以 embedded 模式运行 host-agent。
//
// 通过环境变量 HOST_AGENT_MODE 推断（与 cmd/control-plane/main.go ENV 流相同）。
// embedded 模式下杀 host-agent = 杀控制面，MVS-08 用例应 t.Skip。
func IsEmbeddedHostAgent() bool {
	return getEnvOrDefault("HOST_AGENT_MODE", "") == "embedded"
}

// WaitHostHealthStatus 反复轮询 /healthz，直到 agent 字段等于期望状态或 timeout。
//
// expected 通常是 HostHealthHealthy / HostHealthUnhealthy。
// 单次请求 2s 超时，DisableKeepAlives=true（避免连接复用造成的假阳）。
func (p *GoldenPath) WaitHostHealthStatus(ctx context.Context, expected HostHealthStatus, timeout time.Duration) error {
	if p == nil || p.ControlPlaneURL == "" {
		return errors.New("wait health: control plane URL empty")
	}
	healthURL := strings.TrimRight(p.ControlPlaneURL, "/") + "/healthz"
	client := disableKeepAliveClient(2 * time.Second)

	name := fmt.Sprintf("agent_status=%s", expected)
	return harness.WaitFor(ctx, name, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build req: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("do: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		_, agent, perr := ParseControlPlaneHealth(body)
		if perr != nil {
			return fmt.Errorf("parse: %w (body=%s)", perr, string(body))
		}
		if agent != expected {
			return fmt.Errorf("agent=%s want=%s (body=%s)", agent, expected, string(body))
		}
		return nil
	},
		harness.WithTimeout(timeout),
		harness.WithPollInterval(500*time.Millisecond),
	)
}

// ─── Phase 48 Plan 01 / MVS-09：sing-box 崩溃断网 ─────────────────────

// errWorkerContainerHandleUnavailable 是 ProbeOutboundFromUser / ProbeDNSFromUser
// 在 worker 容器名尚未填充（Scenario Step 7 sentinel 期间）时返回的错误，
// 调用方应据此 t.Skip 而不是 t.Fatalf。
var errWorkerContainerHandleUnavailable = errors.New("worker container handle unavailable (scenario step 7 未实现)")

// gatewayDockerName 优先返回 GatewayHandle.ContainerID（Phase 45 Step 4..6
// 真实填充），否则回退到约定命名 `cloudproxy-gw-<HostID>`。
//
// 在 Step 4..6 sentinel 期间，ContainerID 与 HostID 都可能为空；调用方需要
// 据此判 t.Skip。
func (p *GoldenPath) gatewayDockerName() (string, error) {
	if p == nil || p.Gateway == nil {
		return "", errors.New("gateway handle nil")
	}
	if id := strings.TrimSpace(p.Gateway.ContainerID); id != "" {
		return id, nil
	}
	if p.Gateway.HostID != "" {
		return "cloudproxy-gw-" + p.Gateway.HostID, nil
	}
	if p.Host != nil && p.Host.ID != "" {
		return "cloudproxy-gw-" + p.Host.ID, nil
	}
	return "", errors.New("gateway container id/name unavailable (scenario step 4..6 未实现)")
}

// workerDockerName 类似 gatewayDockerName，但走 worker 容器命名约定。
func (p *GoldenPath) workerDockerName() (string, error) {
	if p == nil || p.Host == nil {
		return "", errors.New("host handle nil")
	}
	if name := strings.TrimSpace(p.Host.ContainerName); name != "" {
		return name, nil
	}
	if p.Host.ID != "" {
		return "cloudproxy-" + p.Host.ID, nil
	}
	return "", errWorkerContainerHandleUnavailable
}

// KillGateway 通过 `docker kill --signal=KILL <gateway>` 强杀 sing-box 网关容器
// （MVS-09）。
//
// 行为约定：
//   - 优先用 GatewayHandle.ContainerID；回退到 `cloudproxy-gw-<HostID>` 命名。
//   - 固定 SIGKILL，不发 SIGTERM（CONTEXT §Area 1 决策，避免触发 sing-box
//     graceful shutdown / cleanup 路径）。
//   - 调完后通过 `docker inspect -f '{{.State.Running}}'` 二次确认；非 false
//     即视为 kill 未生效（与 docker exit code 0 互不替代）。
//   - 句柄未填充（Step 4..6 sentinel） → 返回错，调用方 t.Skip。
//
// 不用 docker stop / docker rm：契约是「sing-box 崩溃」而不是「优雅退出」。
func (p *GoldenPath) KillGateway(ctx context.Context) error {
	name, err := p.gatewayDockerName()
	if err != nil {
		return fmt.Errorf("kill gateway: %w", err)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "kill", "--signal=KILL", name)
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("kill gateway: docker kill %s: %w (stderr=%s)",
			name, runErr, stderr.String())
	}

	var inspectOut bytes.Buffer
	insp := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	insp.Stdout = &inspectOut
	if inspErr := insp.Run(); inspErr != nil {
		// inspect 失败本身不致命：可能容器已被 docker 自动清理。容器不在 = 已死。
		return nil
	}
	if strings.TrimSpace(inspectOut.String()) != "false" {
		return fmt.Errorf("kill gateway: container %s still running after docker kill (state=%q)",
			name, strings.TrimSpace(inspectOut.String()))
	}
	return nil
}

// ProbeOutboundFromUser 在 worker 容器内跑 `curl -sS --max-time <N> <url>`，
// 返回 exit code 与 err（MVS-09 主断言：kill gateway 后 curl 必须非 0 退出）。
//
// 行为：
//   - 通过 `docker exec` 走 worker 容器（句柄未就绪 → errWorkerContainerHandleUnavailable）。
//   - timeout 转换为整数秒；< 1s 一律按 1s 处理（curl --max-time 不支持小数）。
//   - exec.ExitError 解包出退出码；其它错（docker daemon 不通）返回 err 非 nil
//     + exitCode=-1。
//
// 不暴露 stdout（kill-switch 验证只看 exitCode）。
func (p *GoldenPath) ProbeOutboundFromUser(ctx context.Context, url string, timeout time.Duration) (int, error) {
	name, err := p.workerDockerName()
	if err != nil {
		return -1, err
	}
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", name,
		"curl", "-sS", "--max-time", strconv.Itoa(secs), url)
	if runErr := cmd.Run(); runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("probe outbound: docker exec %s: %w", name, runErr)
	}
	return 0, nil
}

// TcpdumpOnHostEth0 在宿主机 eth0 上以「host network + cap-add NET_RAW/NET_ADMIN」
// sidecar 起一次 tcpdump 子进程，返回 BPF 命中的包数（MVS-09 / MVS-10 独立 oracle）。
//
// 实现路径：
//   - 默认走 `docker run --rm --network host --cap-add NET_RAW --cap-add NET_ADMIN
//     nicolaka/netshoot:v0.13 tcpdump -nn -i eth0 -c <count> <bpfFilter>`。
//     使用 host network 让 sidecar 看到真实宿主机 NIC；count 命中或 timeout 到即退出。
//   - 路径 B（`E2E_ALLOW_HOST_TCPDUMP=1` + uid==0）：直接 `tcpdump -i eth0 ...`，
//     不起 sidecar；仅在自管 runner 上启用。
//
// 解析 stderr 中 `N packets captured`（通过 ParseTcpdumpCountOutput）得到包数。
// 解析失败 → 返回 0 + 包装后的 ErrTcpdumpCountNotFound（调用方应据此把
// tcpdump 整段 stderr 打 t.Logf 排障）。
//
// timeout 控制 tcpdump 自身硬退出（通过 ctx 派生 + `-G`/`-W` 不用，sidecar 走
// ctx.Done 即可）。
func (p *GoldenPath) TcpdumpOnHostEth0(ctx context.Context, bpfFilter string, count int, timeout time.Duration) (int, error) {
	if count <= 0 {
		count = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	tcpdumpArgs := []string{
		"-nn", "-i", "eth0", "-c", strconv.Itoa(count), bpfFilter,
	}

	var stderr bytes.Buffer
	subCtx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()

	useHostNative := getEnvOrDefault("E2E_ALLOW_HOST_TCPDUMP", "") == "1" && os.Geteuid() == 0
	var cmd *exec.Cmd
	if useHostNative {
		args := append([]string{}, tcpdumpArgs...)
		cmd = exec.CommandContext(subCtx, "tcpdump", args...)
	} else {
		dockerArgs := []string{
			"run", "--rm",
			"--network", "host",
			"--cap-add", "NET_RAW", "--cap-add", "NET_ADMIN",
			getEnvOrDefault("E2E_TCPDUMP_IMAGE", "nicolaka/netshoot:v0.13"),
			"tcpdump",
		}
		dockerArgs = append(dockerArgs, tcpdumpArgs...)
		cmd = exec.CommandContext(subCtx, "docker", dockerArgs...)
	}
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	// tcpdump 在 `-c N` 命中后退出码 0；超时被 ctx 杀掉时 exit code 非 0。
	// 即便子进程被杀，统计行通常已经写到 stderr（tcpdump 在 SIGTERM 时也会
	// 输出 captured 计数）。
	packets, parseErr := ParseTcpdumpCountOutput(stderr.String())
	if parseErr != nil {
		if runErr != nil {
			return 0, fmt.Errorf("tcpdump host eth0: %w; run err: %v; stderr=%s",
				parseErr, runErr, stderr.String())
		}
		return 0, fmt.Errorf("tcpdump host eth0: %w; stderr=%s",
			parseErr, stderr.String())
	}
	return packets, nil
}

// InspectContainerIPv4 通过 `docker inspect -f '{{...}}'` 拿容器在 *指定 docker
// network* 内的 IPv4 地址。用例需要 worker 容器 IP 来拼 host eth0 的 BPF filter。
//
// 行为：
//   - networkName 为空 → 取 NetworkSettings.IPAddress（旧默认 bridge 网络字段）。
//   - networkName 非空 → 取 NetworkSettings.Networks[<name>].IPAddress。
//   - 出错 / 空字符串 → 返回 err 非 nil，调用方 t.Skip。
func (p *GoldenPath) InspectContainerIPv4(ctx context.Context, containerName, networkName string) (string, error) {
	if containerName == "" {
		return "", errors.New("inspect container ipv4: container name empty")
	}
	var format string
	if networkName == "" {
		format = "{{.NetworkSettings.IPAddress}}"
	} else {
		format = fmt.Sprintf(`{{(index .NetworkSettings.Networks "%s").IPAddress}}`, networkName)
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", format, containerName)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("inspect %s: %w", containerName, err)
	}
	ip := strings.TrimSpace(out.String())
	if ip == "" {
		return "", fmt.Errorf("inspect %s: ipv4 empty (network=%s)", containerName, networkName)
	}
	return ip, nil
}

// ─── Phase 48 Plan 02 / MVS-10：resolv.conf 篡改免疫 ──────────────────

// TamperResolvConf 在 worker 容器内尝试以用户态手法改写 `/etc/resolv.conf`，
// 模拟用户绕过 ro bind mount 的尝试（MVS-10）。
//
// 实现路径：
//   - `cp /etc/resolv.conf /tmp/r.bak 2>/dev/null; echo 'nameserver X' > /etc/resolv.conf
//     && grep -q 'X' /etc/resolv.conf`
//   - exit 0 → TamperApplied（绕过成功，文件已被覆盖）
//   - exit != 0 → TamperRejected（系统侧抗住了，e.g. ro mount / EROFS / EBUSY）
//   - docker exec 本身报错 → TamperUnknown + err 非 nil（用例 t.Fatalf）
//
// CONTEXT §Area 2：Applied 与 Rejected 都是合法分支，由 ClassifyResolvConfDNSOutcome
// 据 DNS 结果与抓包合成最终裁决。
func (p *GoldenPath) TamperResolvConf(ctx context.Context, nameserver string) (ResolvConfTamperResult, error) {
	name, err := p.workerDockerName()
	if err != nil {
		return TamperUnknown, err
	}
	script := fmt.Sprintf(
		"cp /etc/resolv.conf /tmp/r.bak 2>/dev/null; "+
			"echo 'nameserver %s' > /etc/resolv.conf && grep -q '%s' /etc/resolv.conf",
		nameserver, nameserver,
	)
	cmd := exec.CommandContext(ctx, "docker", "exec", name, "bash", "-c", script)
	runErr := cmd.Run()
	if runErr == nil {
		return TamperApplied, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return TamperRejected, nil
	}
	return TamperUnknown, fmt.Errorf("tamper resolv.conf: docker exec %s: %w", name, runErr)
}

// ProbeDNSFromUser 在 worker 容器内跑 `dig +short +time=<sec> +tries=1 <domain>`，
// 把 exit code + stderr 喂给 ClassifyDNSResult 得到 DNS 通路分类（MVS-10）。
//
// 返回值：
//   - 标准 DNSProbeResult 枚举（Tunneled / Denied / Leaked / Unknown）。
//   - err：仅在 docker exec 本身报错（容器不在 / docker 不通）时非 nil。
//
// 单源不做 vote；本 plan 关心通路本身，不关心回显 IP。
func (p *GoldenPath) ProbeDNSFromUser(ctx context.Context, domain string, timeout time.Duration) (DNSProbeResult, error) {
	name, err := p.workerDockerName()
	if err != nil {
		return DNSResultUnknown, err
	}
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "exec", name,
		"dig", "+short", fmt.Sprintf("+time=%d", secs), "+tries=1", domain)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	runErr := cmd.Run()
	if runErr == nil {
		if strings.TrimSpace(stdout.String()) == "" {
			// exit 0 但 stdout 空：dig 在某些 timeout 场景下也走这条路；归 Denied。
			return DNSResultDenied, nil
		}
		return DNSResultTunneled, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return ClassifyDNSResult(exitErr.ExitCode(), stderr.String()), nil
	}
	return DNSResultUnknown, fmt.Errorf("probe dns: docker exec %s: %w", name, runErr)
}

// ─── Phase 49 / LEAK-01..08 容器内探测 helpers ─────────────────────────
//
// 通用约定：
//   - 所有 LEAK 探测方法返回 *LeakProbeResult + error；err 仅在容器句柄缺失
//     或 docker exec 本身报错（容器不在 / docker daemon 不通）时非 nil。
//   - 子进程退出码非 0 不视为本函数错误；通过 LeakProbeResult.ExitCode +
//     Reason 表达 Blocked / 未阻断分支。
//   - 调用方拿 *LeakProbeResult 后再调 ClassifyLeakProbe 与「期望」合成 verdict。

// execWorkerCapture 是 LEAK-* 探测方法的统一入口：
// `docker exec <worker> <argv...>`，捕获 stdout/stderr/exit code/duration，
// 包装成 LeakProbeResult 雏形。Blocked / Reason 由调用方根据自己的语义填。
func (p *GoldenPath) execWorkerCapture(ctx context.Context, argv []string) (*LeakProbeResult, error) {
	name, err := p.workerDockerName()
	if err != nil {
		return nil, err
	}
	full := append([]string{"exec", name}, argv...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	res := &LeakProbeResult{
		RawStdout: stdout.String(),
		RawStderr: stderr.String(),
		Duration:  duration,
	}
	if runErr == nil {
		res.ExitCode = 0
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	res.ExitCode = -1
	return res, fmt.Errorf("docker exec %s: %w", name, runErr)
}

// digIPv4Re 抓 dig +short 输出中的 IPv4 字面量；命中即 dig 拿到了 A 记录。
var digIPv4Re = regexp.MustCompile(`(?m)^\s*(\d{1,3}\.){3}\d{1,3}\s*$`)

// DigPlainDNS 在 worker 容器内跑 `dig +short +time=3 +tries=1 @<server> <name>`，
// 解析为 LeakProbeResult（LEAK-01）。
//
// 语义：
//   - exit 0 + stdout 含 IPv4 → Blocked=false（dig 拿到了 A 记录，疑似泄漏）。
//   - exit 0 + stdout 空 / 仅 ; 注释 → Blocked=true, Reason="dig_empty"。
//   - exit 9 (SERVFAIL) / exit 10 (timeout) / stderr 含 timeout 关键字 → Blocked=true。
//   - 其它 → Blocked=true, Reason="dig_other_error"（保守归 Blocked，避免误判 Fail）。
func (p *GoldenPath) DigPlainDNS(ctx context.Context, server, name string) (*LeakProbeResult, error) {
	argv := []string{"dig", "+short", "+time=3", "+tries=1", "@" + server, name}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	if res.ExitCode == 0 && digIPv4Re.MatchString(res.RawStdout) {
		res.Blocked = false
		res.Reason = "dig_resolved"
		return res, nil
	}
	lower := strings.ToLower(res.RawStderr + res.RawStdout)
	switch {
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		res.Blocked = true
		res.Reason = "dig_timeout"
	case strings.Contains(lower, "servfail"):
		res.Blocked = true
		res.Reason = "dig_servfail"
	case strings.Contains(lower, "connection refused"):
		res.Blocked = true
		res.Reason = "dig_refused"
	case strings.Contains(lower, "no servers could be reached"):
		res.Blocked = true
		res.Reason = "dig_no_servers"
	default:
		res.Blocked = true
		res.Reason = "dig_other_error"
	}
	return res, nil
}

// DigDoT 在 worker 容器内尝试 DoT (TCP/853) 直连（LEAK-02）。
//
// 实现：
//
//	bash -c 'command -v kdig >/dev/null && kdig +tls +time=3 @<server> <name>
//	         || (timeout 5 openssl s_client -connect <server>:853 -brief </dev/null)'
//
// 解析：
//   - exit 0 + stdout 含 `Verify return code: 0` 或 IPv4 字面量 → Blocked=false。
//   - 其它 → Blocked=true，Reason 按 stderr 关键字分类。
func (p *GoldenPath) DigDoT(ctx context.Context, server, name string) (*LeakProbeResult, error) {
	script := fmt.Sprintf(
		"command -v kdig >/dev/null && kdig +tls +time=3 @%s %s "+
			"|| (timeout 5 openssl s_client -connect %s:853 -brief </dev/null 2>&1)",
		server, name, server,
	)
	argv := []string{"bash", "-c", script}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	combined := res.RawStdout + res.RawStderr
	lower := strings.ToLower(combined)
	if res.ExitCode == 0 && (strings.Contains(combined, "Verify return code: 0") ||
		digIPv4Re.MatchString(res.RawStdout)) {
		res.Blocked = false
		res.Reason = "dot_handshake_ok"
		return res, nil
	}
	switch {
	case strings.Contains(lower, "connection refused"):
		res.Blocked = true
		res.Reason = "dot_refused"
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		res.Blocked = true
		res.Reason = "dot_timeout"
	case strings.Contains(lower, "handshake failed"), strings.Contains(lower, "verify error"):
		res.Blocked = true
		res.Reason = "dot_tls_failed"
	default:
		res.Blocked = true
		res.Reason = "dot_other_error"
	}
	return res, nil
}

// PingICMP 在 worker 容器内跑 `ping -c 1 -W 3 <target>`（LEAK-03）。
//
// 解析：
//   - exit 0 + stdout 含 `1 received` → Blocked=false（ICMP 通了）。
//   - exit 2 + stderr 含 `Operation not permitted` / `Permission denied`
//     → Blocked=true, Reason="raw_socket_denied"（同时映射 LEAK-06 行为）。
//   - exit 1 + stdout 含 `0 received` / stderr 含 `Network is unreachable`
//     / `Destination Host Unreachable` → Blocked=true, Reason="ping_no_reply"
//     或 "route_unreachable"。
func (p *GoldenPath) PingICMP(ctx context.Context, target string) (*LeakProbeResult, error) {
	argv := []string{"ping", "-c", "1", "-W", "3", target}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	combined := res.RawStdout + res.RawStderr
	lower := strings.ToLower(combined)
	if res.ExitCode == 0 && strings.Contains(combined, "1 received") {
		res.Blocked = false
		res.Reason = "ping_replied"
		return res, nil
	}
	switch {
	case strings.Contains(lower, "operation not permitted"),
		strings.Contains(lower, "permission denied"):
		res.Blocked = true
		res.Reason = "raw_socket_denied"
	case strings.Contains(lower, "network is unreachable"),
		strings.Contains(lower, "destination host unreachable"),
		strings.Contains(lower, "no route to host"):
		res.Blocked = true
		res.Reason = "route_unreachable"
	case strings.Contains(lower, "0 received"):
		res.Blocked = true
		res.Reason = "ping_no_reply"
	default:
		res.Blocked = true
		res.Reason = "ping_other_error"
	}
	return res, nil
}

// CurlIPv6 在 worker 容器内跑 `curl -6 -sS --max-time 3 <url>`（LEAK-04）。
//
// 解析：
//   - exit 0 + stdout 非空 → Blocked=false（IPv6 出网成功）。
//   - exit 6 / 7 / 28 / stderr 含 unreachable / disabled → Blocked=true。
func (p *GoldenPath) CurlIPv6(ctx context.Context, url string) (*LeakProbeResult, error) {
	argv := []string{"curl", "-6", "-sS", "--max-time", "3", url}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	if res.ExitCode == 0 && strings.TrimSpace(res.RawStdout) != "" {
		res.Blocked = false
		res.Reason = "ipv6_curl_ok"
		return res, nil
	}
	lower := strings.ToLower(res.RawStderr)
	switch {
	case strings.Contains(lower, "could not resolve host"):
		res.Blocked = true
		res.Reason = "ipv6_dns_failed"
	case strings.Contains(lower, "couldn't connect"),
		strings.Contains(lower, "network unreachable"),
		strings.Contains(lower, "network is unreachable"):
		res.Blocked = true
		res.Reason = "ipv6_unreachable"
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		res.Blocked = true
		res.Reason = "ipv6_timeout"
	default:
		res.Blocked = true
		res.Reason = "ipv6_other_error"
	}
	return res, nil
}

// ReadProcFile 在 worker 容器内 `cat <path>` 读 /proc 文件（LEAK-04 双保险用）。
func (p *GoldenPath) ReadProcFile(ctx context.Context, path string) (string, error) {
	argv := []string{"cat", path}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return strings.TrimSpace(res.RawStdout),
			fmt.Errorf("read proc file %s: exit=%d stderr=%s", path, res.ExitCode, res.RawStderr)
	}
	return strings.TrimSpace(res.RawStdout), nil
}

// CurlIMDS 在 worker 容器内打 IMDS 端点（LEAK-05）。
//
// 命令：`curl -sS -o /dev/null -w '%{http_code}' --max-time 3 <url>`。
//
// 解析：
//   - exit 0 + stdout == "200" → Blocked=false（IMDS 响应成功，疑似真泄漏）。
//   - exit 0 + http 4xx/5xx → Blocked=true, Reason="imds_http_error"。
//   - exit 7 / 28 / stderr 含 refused / timeout → Blocked=true。
func (p *GoldenPath) CurlIMDS(ctx context.Context, url string) (*LeakProbeResult, error) {
	argv := []string{"curl", "-sS", "-o", "/dev/null", "-w", "%{http_code}",
		"--max-time", "3", url}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	httpCode := strings.TrimSpace(res.RawStdout)
	lower := strings.ToLower(res.RawStderr)
	switch {
	case res.ExitCode == 0 && httpCode == "200":
		res.Blocked = false
		res.Reason = "imds_responded_200"
	case res.ExitCode == 0 && len(httpCode) == 3:
		res.Blocked = true
		res.Reason = "imds_http_" + httpCode
	case strings.Contains(lower, "connection refused"):
		res.Blocked = true
		res.Reason = "imds_refused"
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		res.Blocked = true
		res.Reason = "imds_timeout"
	case strings.Contains(lower, "couldn't connect"),
		strings.Contains(lower, "no route to host"),
		strings.Contains(lower, "network is unreachable"):
		res.Blocked = true
		res.Reason = "imds_unreachable"
	default:
		res.Blocked = true
		res.Reason = "imds_other_error"
	}
	return res, nil
}

// TryRawSocket 在 worker 容器内尝试创建 SOCK_RAW socket（LEAK-06）。
//
// 优先 python3 路径；缺失时回退 bash exec /dev/raw/icmp（旧风格行为相同）。
//
// 解析：
//   - stdout 含 `RAW_OK` → Blocked=false（capability 允许 raw socket，实锤 leak）。
//   - stderr 含 `PermissionError` / `Operation not permitted` → Blocked=true。
func (p *GoldenPath) TryRawSocket(ctx context.Context) (*LeakProbeResult, error) {
	script := `if command -v python3 >/dev/null; then
  python3 -c 'import socket; s=socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_ICMP); print("RAW_OK")' 2>&1
elif command -v python >/dev/null; then
  python -c 'import socket; s=socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_ICMP); print("RAW_OK")' 2>&1
else
  bash -c 'exec 3<>/dev/raw/icmp 2>&1' && echo "RAW_OK"
fi`
	argv := []string{"bash", "-c", script}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil || res == nil {
		return res, err
	}
	combined := res.RawStdout + res.RawStderr
	lower := strings.ToLower(combined)
	switch {
	case strings.Contains(combined, "RAW_OK"):
		res.Blocked = false
		res.Reason = "raw_socket_allowed"
	case strings.Contains(lower, "permissionerror"),
		strings.Contains(lower, "operation not permitted"),
		strings.Contains(lower, "permission denied"):
		res.Blocked = true
		res.Reason = "raw_socket_denied"
	default:
		res.Blocked = true
		res.Reason = "raw_socket_other_error"
	}
	return res, nil
}

// ListNftRulesOnHost 在宿主机执行 `nft list ruleset`（LEAK-07）。
//
// 实现路径：
//   - 默认走 `docker run --rm --network host --cap-add NET_ADMIN --cap-add SYS_ADMIN
//     <netshoot> nft list ruleset`。
//   - 路径 B（`E2E_ALLOW_HOST_TCPDUMP=1` + uid==0）：直接 `nft list ruleset`。
//
// 失败 → 返 err，调用方 t.Skip。
func (p *GoldenPath) ListNftRulesOnHost(ctx context.Context) (string, error) {
	subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	useHostNative := getEnvOrDefault("E2E_ALLOW_HOST_TCPDUMP", "") == "1" && os.Geteuid() == 0
	var cmd *exec.Cmd
	if useHostNative {
		cmd = exec.CommandContext(subCtx, "nft", "list", "ruleset")
	} else {
		image := getEnvOrDefault("E2E_TCPDUMP_IMAGE", "nicolaka/netshoot:v0.13")
		cmd = exec.CommandContext(subCtx, "docker", "run", "--rm",
			"--network", "host",
			"--cap-add", "NET_ADMIN", "--cap-add", "SYS_ADMIN",
			image, "nft", "list", "ruleset")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("nft list ruleset: %w (stderr=%s)", err, stderr.String())
	}
	return stdout.String(), nil
}

// GetProcCapabilities 在 worker 容器内 `cat /proc/<pid>/status`（LEAK-08）。
//
// 返回完整 stdout 文本；调用方传给 ParseProcCapabilities 解析。
func (p *GoldenPath) GetProcCapabilities(ctx context.Context, pid int) (string, error) {
	argv := []string{"cat", fmt.Sprintf("/proc/%d/status", pid)}
	res, err := p.execWorkerCapture(ctx, argv)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return res.RawStdout,
			fmt.Errorf("cat /proc/%d/status: exit=%d stderr=%s", pid, res.ExitCode, res.RawStderr)
	}
	return res.RawStdout, nil
}

// EnsureWorkerLeakTools 在 LeakSuite SetupSuite 中调用，最多 best-effort
// 安装 LEAK-* 用例需要的工具：dig (dnsutils) / kdig (knot-dnsutils) /
// openssl / iputils-ping / python3-minimal。
//
// 行为：
//   - 已存在 → 跳过；安装失败 → 仅 logger.Warn，让具体用例自行决定 Skip 或 Fail。
//   - 一次性 apt-get update + install；不重复刷新 cache。
//   - worker 容器句柄未填充 → 直接返 nil（用例侧会 Skip）。
//
// 镜像内已装 curl / cat / bash / grep / awk，不再处理。
func (p *GoldenPath) EnsureWorkerLeakTools(ctx context.Context) error {
	name, err := p.workerDockerName()
	if err != nil {
		return nil
	}
	script := `set -e
need=()
for pkg in dig kdig openssl ping python3; do
  command -v "$pkg" >/dev/null 2>&1 || need+=("$pkg")
done
if [ "${#need[@]}" -eq 0 ]; then
  echo "all-present"
  exit 0
fi
apt-get update -qq >/dev/null 2>&1 || true
apt-get install -y --no-install-recommends \
  dnsutils knot-dnsutils openssl iputils-ping python3-minimal >/dev/null 2>&1 || true
echo "install-attempted"`
	cmd := exec.CommandContext(ctx, "docker", "exec", name, "bash", "-c", script)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("ensure leak tools: %w (out=%s)", runErr, string(out))
	}
	return nil
}

// ─── Phase 50 / KILL-01..04 压力测试 helpers ────────────────────────────

// SetTunDevDown 在 gateway 容器内执行 `ip link set tun0 down`，模拟
// sing-box tun 设备被关掉的软故障（KILL-02）。
//
// 行为：
//   - 通过 gatewayDockerName() 拿容器名；句柄缺失 → 返回 err，调用方 t.Skip。
//   - `docker exec <gw> ip link set tun0 down`；exit 非 0 → wrap 后返回，
//     用例 t.Fatalf。
//   - 设备名锁定为 `tun0`（grep `internal/network/singbox_provider_linux.go`：
//     sing-box auto_route 在 gateway 容器内创建 tun0 设备）。
//   - 与 worker netns 内 nft `oifname "sb-tun0"` 中的接口名不同：sb-tun0 是
//     worker 侧防火墙规则用的标识，KILL-02 关的是 gateway 容器内的设备。
func (p *GoldenPath) SetTunDevDown(ctx context.Context) error {
	name, err := p.gatewayDockerName()
	if err != nil {
		return fmt.Errorf("set tun0 down: %w", err)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "exec", name, "ip", "link", "set", "tun0", "down")
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("set tun0 down: docker exec %s: %w (stderr=%s)",
			name, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// SetTunDevUp 在 gateway 容器内执行 `ip link set tun0 up`，是 KILL-02 cleanup
// 的对称操作。
//
// 与 SetTunDevDown 不同：本函数失败应视为 best-effort（用例侧仅 t.Logf），
// 因为该用例的契约结果在 t.Cleanup 触发前已经定盘，恢复失败只影响
// 该 Scenario 实例后续可用性（每用例独立 GoldenPath 隔离）。
func (p *GoldenPath) SetTunDevUp(ctx context.Context) error {
	name, err := p.gatewayDockerName()
	if err != nil {
		return fmt.Errorf("set tun0 up: %w", err)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "exec", name, "ip", "link", "set", "tun0", "up")
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("set tun0 up: docker exec %s: %w (stderr=%s)",
			name, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DisconnectGatewayFromBridge 把 gateway 从其专属自定义 bridge
// (`cloudproxy-net-<HostID>`) 摘走（KILL-04）。
//
// 行为：
//  1. `docker inspect` 拿当前 gateway 接入的所有网络名 + IP。
//  2. 用 PickGatewayBridgeNetwork 纯函数挑出 `cloudproxy-net-*` 前缀的网络
//     （兜底首个非 default `bridge` 的网络）。
//  3. `docker network disconnect <netName> <gateway>`。
//
// 返回 (savedNet, savedIP, nil) 供 ReconnectGatewayToBridge 在 cleanup 时使用。
// savedNet == "" → gateway 未接 cloudproxy-net 类网络，调用方 t.Skipf 并把
// backend gap 流转 Phase 51（同 Phase 49 LEAK-06/07/08 流程）。
func (p *GoldenPath) DisconnectGatewayFromBridge(ctx context.Context) (string, string, error) {
	name, err := p.gatewayDockerName()
	if err != nil {
		return "", "", fmt.Errorf("disconnect gateway: %w", err)
	}
	var inspectOut bytes.Buffer
	insp := exec.CommandContext(ctx, "docker", "inspect", "-f",
		`{{range $k, $v := .NetworkSettings.Networks}}{{$k}}={{$v.IPAddress}};{{end}}`,
		name)
	insp.Stdout = &inspectOut
	if inspErr := insp.Run(); inspErr != nil {
		return "", "", fmt.Errorf("inspect gateway %s: %w", name, inspErr)
	}
	savedNet, savedIP := PickGatewayBridgeNetwork(inspectOut.String())
	if savedNet == "" {
		return "", "", fmt.Errorf("disconnect gateway: %s has no cloudproxy-net / custom bridge network", name)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "network", "disconnect", savedNet, name)
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return savedNet, savedIP, fmt.Errorf("disconnect %s from %s: %w (stderr=%s)",
			name, savedNet, runErr, strings.TrimSpace(stderr.String()))
	}
	return savedNet, savedIP, nil
}

// ReconnectGatewayToBridge 把 gateway 接回 KILL-04 disconnect 前的 docker 网络。
//
// 静态 IP 为空 → 走 docker 自动分配（subnet 范围内不一定能拿回原 IP）。
// 失败仅 wrap err；调用方在 t.Cleanup 内 best-effort（t.Logf）。
func (p *GoldenPath) ReconnectGatewayToBridge(ctx context.Context, netName, staticIP string) error {
	if strings.TrimSpace(netName) == "" {
		return errors.New("reconnect gateway: net name empty")
	}
	name, err := p.gatewayDockerName()
	if err != nil {
		return fmt.Errorf("reconnect gateway: %w", err)
	}
	args := []string{"network", "connect"}
	if strings.TrimSpace(staticIP) != "" {
		args = append(args, "--ip", staticIP)
	}
	args = append(args, netName, name)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("reconnect %s to %s: %w (stderr=%s)",
			name, netName, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// InjectPumbaNetem 起一个 Pumba sidecar 进程，对 target 容器注入 netem 故障
// （KILL-03）。
//
// 行为：
//   - 调 BuildPumbaNetemArgs 拼 argv；空 → 返回 err。
//   - 起 `docker run --rm --name <sidecar> -v /var/run/docker.sock:/var/run/docker.sock <image> netem ...`，
//     用 `cmd.Start()` detached；返回 cleanup 函数：cleanup 通过
//     `docker kill <sidecar>` + cmd.Wait 收尾。
//   - 启动失败（exec.LookPath docker 缺、image pull 错）→ 返回 (nil, err)，
//     调用方 t.Skipf 并把 ImageMissing / DaemonDown 流转 VERIFICATION。
//
// 注意：Pumba `--duration` 自然结束时 cmd 自动退出；用例侧 cleanup 是兜底，
// 避免 t.Fatalf 之后 sidecar 还在跑。
func (p *GoldenPath) InjectPumbaNetem(ctx context.Context, target string, params PumbaNetemParams) (func(), error) {
	if _, lookErr := exec.LookPath("docker"); lookErr != nil {
		return nil, fmt.Errorf("inject pumba: docker not in PATH: %w", lookErr)
	}
	argv := BuildPumbaNetemArgs(target, params)
	if len(argv) == 0 {
		return nil, fmt.Errorf("inject pumba: empty argv for target=%q params=%+v", target, params)
	}

	sidecarName := fmt.Sprintf("pumba-%s-%d", sanitizeContainerName(target), time.Now().UnixNano())
	full := append([]string{
		"run", "--rm", "--name", sidecarName,
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
	}, argv[1:]...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("inject pumba: start: %w (stderr=%s)",
			startErr, strings.TrimSpace(stderr.String()))
	}

	cleanup := func() {
		killCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(killCtx, "docker", "kill", sidecarName).Run()
		_ = cmd.Wait()
	}
	return cleanup, nil
}

// sanitizeContainerName 把 Pumba sidecar 名字中的非法字符替换为 `-`，避免
// `docker run --name` 拒收（target 可能含点号 / 大写字母）。
func sanitizeContainerName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "target"
	}
	return out
}

// ProbeSSHBanner 在 worker 容器内通过 `bash -c 'exec 3<>/dev/tcp/localhost/22
// && head -c 6 <&3 | grep -q "^SSH-"'` 探测 SSH 22 端口是否仍能拿到 banner
// （KILL-03 SSH 存活契约）。
//
// 行为：
//   - timeout 通过 ctx 控制；外层用例传 10s 即可。
//   - exit 0 → nil（banner 拿到「SSH-」前缀）。
//   - exit 非 0 / docker exec 错 → wrap err 返回。
//   - 不暴露 stdout（banner 内容用例不消费）。
//
// 选 `/dev/tcp` 而非 `nc -z`：避免依赖 worker 镜像内有无 netcat；bash 内建
// `/dev/tcp` 在常见镜像（debian-slim / ubuntu）都可用。
func (p *GoldenPath) ProbeSSHBanner(ctx context.Context, timeout time.Duration) error {
	name, err := p.workerDockerName()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	subCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	script := `exec 3<>/dev/tcp/localhost/22 && head -c 6 <&3 | grep -q '^SSH-'`
	cmd := exec.CommandContext(subCtx, "docker", "exec", name, "bash", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("probe ssh banner: docker exec %s: %w (stderr=%s)",
			name, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ─── 内部 helpers ──────────────────────────────────────────────────────

// disableKeepAliveClient 返回一个禁 keep-alive 的 http.Client，避免长连接造成
// /healthz 等高频轮询时的连接复用假阳。
func disableKeepAliveClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

// getEnvOrDefault 与 cmd/control-plane/main.go::envOrDefault 同语义；
// 本包内独立实现，避免反向 import cmd 包。
func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// 防御性强引用，避免 goimports 把这些 import 删掉。
var _ = http.MethodGet
