//go:build e2e

package harness

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnvArtifactBaseDir 环境变量可覆盖默认 artifact 根目录。CI workflow 通过
// `env: CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR` 设置即可（Plan 05 e2e.yml 使用）。
const EnvArtifactBaseDir = "CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR"

// DefaultArtifactBaseDir 项目根相对路径，禁绝对路径（CONVENTIONS.md §Privacy）。
const DefaultArtifactBaseDir = "./out/e2e-artifacts"

// ArtifactSubdirs 与 REQUIREMENTS.md §OBS-02 的五子目录契约保持一致。
// Phase 52 OBS-01..03 接入后会在每个子目录写真实收集结果（容器日志 / nft
// ruleset / docker inspect / pg_dump / 系统状态）；Phase 45 Plan 04 仅
// 创建空目录 + 一份中文 README 占位。
var ArtifactSubdirs = []string{"logs", "network", "docker", "postgres", "system"}

// ArtifactDumper 把失败用例的排障证据归档到 baseDir/<sanitizedName>/<timestamp>/。
//
// 设计契约：
//   - 幂等：同一 name + 同一 timestamp 多次调用，已存在目录与文件不报错
//   - 不污染：unit test 用 t.TempDir() 作为 baseDir，由 testing 框架自动清理
//   - 不真实执行：本 plan 不调 docker / nft / pg_dump，那是 Phase 52 的范围
//
// scenario 字段当前未被使用，留作 Phase 52 OBS-01..03 接入时的扩展挂点
// （从中读 GatewayHandle.ContainerID / HostHandle.ContainerName 决定收集哪些
// 容器）。可为 nil（unit test 时）。
type ArtifactDumper struct {
	scenario *Scenario
	baseDir  string
	logger   *slog.Logger
}

// NewArtifactDumper 构造 dumper。
//
// baseDir 为空时按以下优先级解析：
//  1. 环境变量 EnvArtifactBaseDir（CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR）
//  2. 编译默认 DefaultArtifactBaseDir（"./out/e2e-artifacts"）
//
// scenario 可为 nil；当前阶段未被使用，仅保留指针供 Phase 52 扩展。
func NewArtifactDumper(scenario *Scenario, baseDir string) *ArtifactDumper {
	if baseDir == "" {
		baseDir = defaultBaseDir()
	}
	return &ArtifactDumper{
		scenario: scenario,
		baseDir:  baseDir,
		logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

// Collect 在 baseDir/<sanitizedName>/<timestamp>/ 下建 5 个子目录与 README，
// 返回该目录的绝对路径。
//
// timestamp 用 RFC3339-like 格式（20060102T150405Z），文件系统安全 + 字典序
// 等价时间序，便于 ls 排序。
func (d *ArtifactDumper) Collect(_ context.Context, name string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(d.baseDir, sanitizeName(name), timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("artifact: mkdir root: %w", err)
	}
	for _, sub := range ArtifactSubdirs {
		subDir := filepath.Join(dir, sub)
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			return dir, fmt.Errorf("artifact: mkdir %s: %w", sub, err)
		}
		readmePath := filepath.Join(subDir, "README.md")
		if _, err := os.Stat(readmePath); os.IsNotExist(err) {
			if writeErr := os.WriteFile(readmePath, []byte(readmeContentFor(sub)), 0o644); writeErr != nil {
				return dir, fmt.Errorf("artifact: write README %s: %w", sub, writeErr)
			}
		}
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return dir, fmt.Errorf("artifact: abs path: %w", err)
	}
	d.logger.Info("artifact collected", "name", name, "dir", absDir)
	return absDir, nil
}

// OnWaitForTimeout 实现 DumpHook（Plan 03 接口）：
//  1. 调 Collect 建好 5 子目录占位
//  2. 在 <dir>/system/wait-timeout.txt 追加一行
//     `<RFC3339 timestamp> name=<name> last_err=<lastErr.Error()>`
//  3. 返回任一步的 error（best-effort，hook 自身不 panic）
func (d *ArtifactDumper) OnWaitForTimeout(ctx context.Context, name string, lastErr error) error {
	dir, err := d.Collect(ctx, name)
	if err != nil {
		return fmt.Errorf("waitfor dump: %w", err)
	}
	note := fmt.Sprintf("%s name=%s last_err=%v\n",
		time.Now().UTC().Format(time.RFC3339), name, lastErr)
	notePath := filepath.Join(dir, "system", "wait-timeout.txt")
	f, err := os.OpenFile(notePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("waitfor dump: open note: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(note); err != nil {
		return fmt.Errorf("waitfor dump: append note: %w", err)
	}
	return nil
}

// sanitizeName 把 t.Name()（含 "/" 等）转成文件系统安全的目录名。
// 空字符串 → "_unnamed"；分隔符与冒号统一替换为下划线。
func sanitizeName(s string) string {
	if s == "" {
		return "_unnamed"
	}
	r := strings.NewReplacer("/", "_", " ", "_", "\\", "_", ":", "_")
	return r.Replace(s)
}

// defaultBaseDir 解析环境变量 > 编译默认。
func defaultBaseDir() string {
	if v := os.Getenv(EnvArtifactBaseDir); v != "" {
		return v
	}
	return DefaultArtifactBaseDir
}

// readmeContentFor 返回 5 个子目录各自的 README.md 占位文本（中文，
// 解释 Phase 52 OBS-01..03 接入后会写什么）。
func readmeContentFor(sub string) string {
	switch sub {
	case "logs":
		return "# 容器日志（Phase 52 OBS-01 接入后真实收集）\n\nPhase 52 OBS-01 落地后，本目录会自动收集：\n- control-plane.log（控制面进程 stderr）\n- host-agent.log（host-agent stderr）\n- gateway-<host_id>.log（sing-box 容器）\n- worker-<host_id>.log（worker 容器内 entrypoint 输出）\n\n当前 Phase 45 Plan 04 仅创建本目录与 README 占位。\n"
	case "network":
		return "# 网络状态（Phase 52 OBS-02 接入后真实收集）\n\nPhase 52 OBS-02 落地后，本目录会包含：\n- nft-list-ruleset.txt（host root namespace 与每个 worker netns 的 nft list ruleset）\n- ip-netns.txt（ip netns list）\n- ip-route-host.txt（host root namespace ip route）\n- ip-route-<host_id>.txt（每个 worker netns）\n"
	case "docker":
		return "# Docker 元信息（Phase 52 OBS-02 接入后真实收集）\n\nPhase 52 OBS-02 落地后，本目录会包含：\n- docker-ps.txt（docker ps -a 含全状态）\n- docker-inspect-<name>.json（每个相关容器的 inspect 输出）\n"
	case "postgres":
		return "# Postgres dump（Phase 52 OBS-03 接入后真实收集）\n\nPhase 52 OBS-03 落地后，本目录会包含：\n- pg-dump-hosts.csv 等关键表的 CSV 导出\n- schema-version.txt（migrations 版本）\n"
	case "system":
		return "# 宿主机系统状态（Phase 52 OBS-02 接入后真实收集）\n\nPhase 52 OBS-02 落地后，本目录会包含：\n- dmesg-tail.txt / proc-meminfo.txt / kernel-version.txt 等\n- wait-timeout.txt（waitFor 超时即时备忘录，Phase 45 Plan 04 已开始在写）\n"
	default:
		return "# Phase 45 Plan 04 占位\n"
	}
}
