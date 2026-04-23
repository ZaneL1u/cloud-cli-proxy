package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// 包级 var mock 注入点。
var (
	readOSRelease        = func() ([]byte, error) { return os.ReadFile("/etc/os-release") }
	readAppArmorOverride = func() ([]byte, error) { return os.ReadFile("/etc/apparmor.d/local/fusermount3") }
	execLookPath         = exec.LookPath
	execMountList        = func() (string, error) {
		out, err := exec.Command("mount").CombinedOutput()
		return string(out), err
	}
)

// checkMergerfsBranches 远端 getfattr + mount 参数 6 字面量断言（C2 / RESEARCH §8.1）。
func checkMergerfsBranches(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "mergerfs_branches", "未能连接远端容器，跳过")
	}
	xattr, _, _ := runner.RunScript("mergerfs_xattr",
		"getfattr --only-values -n user.mergerfs.branches /workspace/.mergerfs 2>/dev/null")
	mountOut, _, _ := runner.RunScript("mergerfs_mount", "mount | grep mergerfs | head -1")

	xattrOK := strings.Contains(xattr, "RW") && strings.Contains(xattr, "NC,RO")
	want := []string{
		"func.readdir=cor:4", "cache.attr=30", "cache.entry=30",
		"cache.readdir=true", "cache.files=off", "category.create=ff",
	}
	var missing []string
	for _, w := range want {
		if !strings.Contains(mountOut, w) {
			missing = append(missing, w)
		}
	}
	if !xattrOK {
		return newFail("mount", "mergerfs_branches", errcodes.MOUNT_MERGERFS_FAILED,
			"branches xattr 缺 RW 或 NC,RO")
	}
	if len(missing) > 0 {
		return newFail("mount", "mergerfs_branches", errcodes.MOUNT_MERGERFS_FAILED,
			"mount 参数缺少 "+strings.Join(missing, ","))
	}
	return Check{
		Domain: "mount", Name: "mergerfs_branches", Status: StatusPass,
		Message: "mergerfs 参数与 branches 均符合 Phase 29 基线",
		Details: map[string]any{"branches_xattr": strings.TrimSpace(xattr), "mount": strings.TrimSpace(mountOut)},
	}
}

// checkSSHFSMountpoint 远端 mountpoint -q /workspace-cold（RESEARCH §3.4）。
func checkSSHFSMountpoint(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "sshfs_mountpoint", "未能连接远端容器，跳过")
	}
	_, _, err := runner.RunScript("sshfs_mp", "mountpoint -q /workspace-cold")
	if err != nil {
		return newWarn("mount", "sshfs_mountpoint", errcodes.MOUNT_SSHFS_DISCONNECTED)
	}
	return newPass("mount", "sshfs_mountpoint", "/workspace-cold 已挂载")
}

// checkFUSEResidual 本地扫 mount 输出（RESEARCH §3.4 + §4.2）。
func checkFUSEResidual(ctx context.Context) Check {
	out, err := execMountList()
	if err != nil {
		return newSkip("mount", "fuse_residual", "mount 命令失败，跳过: "+err.Error())
	}
	var re *regexp.Regexp
	switch runtime.GOOS {
	case "darwin":
		re = regexp.MustCompile(`(?m)^.*?\s+on\s+(\S+)\s+\(.*?(macfuse|osxfuse)`)
	case "linux":
		re = regexp.MustCompile(`(?m)^\S+\s+on\s+(\S+)\s+type\s+fuse\.(sshfs|mergerfs)\b`)
	default:
		return newSkip("mount", "fuse_residual", "非 Linux/macOS，跳过")
	}
	matches := re.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return newPass("mount", "fuse_residual", "未发现残留 FUSE 挂载")
	}
	var points []string
	for _, m := range matches {
		points = append(points, m[1])
	}
	// Plan 03 Task 3.3：fix.go 依赖 Details["mountpoints"] 列表做批量 fusermount -u
	entry, _ := errcodes.Lookup(errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT)
	return Check{
		Domain: "mount", Name: "fuse_residual",
		Status:     StatusWarn,
		Code:       errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT,
		Message:    fmt.Sprintf(entry.Message, len(points), strings.Join(points, ",")),
		NextAction: entry.NextAction,
		Details:    map[string]any{"mountpoints": points},
	}
}

// checkAppArmorFusermount3 本地 5-Gate 检测（RESEARCH §8.3）。
// Go 改写 deploy/scripts/host-preflight.sh:check_apparmor_fusermount3。
func checkAppArmorFusermount3(ctx context.Context) Check {
	if runtime.GOOS != "linux" {
		return newSkip("mount", "apparmor_fusermount3", "非 Linux，跳过")
	}
	osRel, err := readOSRelease()
	if err != nil {
		return newSkip("mount", "apparmor_fusermount3", "无 /etc/os-release，跳过")
	}
	if !regexp.MustCompile(`(?m)^ID=ubuntu\b`).Match(osRel) {
		return newSkip("mount", "apparmor_fusermount3", "非 Ubuntu，跳过")
	}
	// Gate 3: Ubuntu >= 25.04
	vre := regexp.MustCompile(`(?m)^VERSION_ID="?(\d+)\.(\d+)"?`)
	m := vre.FindSubmatch(osRel)
	if len(m) >= 3 {
		major := string(m[1])
		minor := string(m[2])
		if major < "25" || (major == "25" && minor < "04") {
			return newSkip("mount", "apparmor_fusermount3",
				fmt.Sprintf("Ubuntu %s.%s < 25.04，跳过", major, minor))
		}
	}
	// Gate 4: aa-status
	if _, err := execLookPath("aa-status"); err != nil {
		return newSkip("mount", "apparmor_fusermount3", "apparmor-utils 未安装，跳过")
	}
	// Gate 5: override 文件
	content, err := readAppArmorOverride()
	if err != nil {
		return newFail("mount", "apparmor_fusermount3", errcodes.SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
			"/etc/apparmor.d/local/fusermount3 不存在")
	}
	if !regexp.MustCompile(`(?m)^\s*capability\s+dac_override\b`).Match(content) {
		return newFail("mount", "apparmor_fusermount3", errcodes.SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
			"override 文件缺 `capability dac_override` 行")
	}
	return newPass("mount", "apparmor_fusermount3", "AppArmor fusermount3 override 就位")
}

// ── Phase 36 D-13 新增 5 项 mount check ─────────────────────────────────────

// gitRevParseTopLevel 在 cwd 执行 git rev-parse --show-toplevel；
// doctor 包不能 import cmd/cloud-claude（main package），故此处复制 4 行 exec 实现。
func gitRevParseTopLevel(cwd string) error {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	return cmd.Run()
}

// checkRequireGitRepo 检查 cwd 是否在 git 仓库内（REQ-MOUNT-V31-01 / D-13 / L2）。
// doctor 命令约定在工程目录下运行，os.Getwd() 语义与 main.go 保持一致。
func checkRequireGitRepo(ctx context.Context) Check {
	cwd, err := os.Getwd()
	if err != nil {
		return newSkip("mount", "require_git_repo", "无法获取 cwd: "+err.Error())
	}
	if err := gitRevParseTopLevel(cwd); err != nil {
		return newFail("mount", "require_git_repo", errcodes.MOUNT_REQUIRE_GIT_REPO, cwd)
	}
	return newPass("mount", "require_git_repo", "当前目录位于 git 仓库内: "+cwd)
}

// checkOversizedFilesCount 读取 last-session.json::oversized_files，
// 报告上次会话因超大文件被跳过的数量（D-13 / L8）。
// 注意：MOUNT_OVERSIZED_FILE_SKIPPED Message 自带 %s/%dMB/%d 占位符，
// 不能直接用 newWarn；参照 checkFUSEResidual 模式直接构造 Check{}。
func checkOversizedFilesCount(ctx context.Context) Check {
	snap, err := cloudclaude.LoadLastSession()
	if err != nil || snap == nil {
		return newSkip("mount", "oversized_files_count",
			"last-session.json 不存在，跳过（STATE_LAST_SESSION_MISSING）")
	}
	n := len(snap.OversizedFiles)
	if n == 0 {
		return newPass("mount", "oversized_files_count", "上次会话无超大文件跳过记录")
	}
	top5 := make([]string, 0, 5)
	for i, f := range snap.OversizedFiles {
		if i >= 5 {
			break
		}
		top5 = append(top5, fmt.Sprintf("%s (%dMB)", f.Path, f.SizeBytes/1024/1024))
	}
	entry, _ := errcodes.Lookup(errcodes.MOUNT_OVERSIZED_FILE_SKIPPED)
	return Check{
		Domain:     "mount",
		Name:       "oversized_files_count",
		Status:     StatusWarn,
		Code:       errcodes.MOUNT_OVERSIZED_FILE_SKIPPED,
		Message:    fmt.Sprintf("上次会话跳过了 %d 个超大文件，由 cold sshfs 兜底", n),
		NextAction: entry.NextAction,
		Details:    map[string]any{"oversized_count": n, "top5_files": top5},
	}
}

// checkSSHFSCacheArgs 通过远端 mount 输出验证 sshfs 命令是否包含全部 4 个 cache 参数（D-13）。
// 与 checkMergerfsBranches 精确镜像：remote runner + want 列表 + missing join。
// Plan 36-05 在 mount_sshfs.go::sshfsCmd 字面量末尾追加的 4 个参数顺序与 want 列表一一对应。
func checkSSHFSCacheArgs(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "sshfs_cache_args", "未能连接远端容器，跳过")
	}
	mountOut, _, _ := runner.RunScript("sshfs_mount", "mount | grep sshfs | head -1")
	want := []string{"cache=yes", "kernel_cache", "auto_cache", "cache_timeout=300"}
	var missing []string
	for _, w := range want {
		if !strings.Contains(mountOut, w) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		return newFail("mount", "sshfs_cache_args", errcodes.MOUNT_SSHFS_FAILED,
			"sshfs cache 参数缺少: "+strings.Join(missing, ", "))
	}
	return Check{
		Domain:  "mount",
		Name:    "sshfs_cache_args",
		Status:  StatusPass,
		Message: "sshfs cache 参数完整（cache=yes,kernel_cache,auto_cache,cache_timeout=300）",
		Details: map[string]any{"mount": strings.TrimSpace(mountOut)},
	}
}

// checkGitProxyEnabled 检查 proxy_commands 是否包含 git（D-13 / L6）。
// config 未 init（LoadConfig 失败）时走 Skip，不影响其他 check。
func checkGitProxyEnabled(ctx context.Context) Check {
	cfg, err := cloudclaude.LoadConfig()
	if err != nil {
		return newSkip("mount", "git_proxy_enabled", "配置未 init，跳过: "+err.Error())
	}
	for _, p := range cfg.EffectiveProxyCommands() {
		if p == "git" {
			return newPass("mount", "git_proxy_enabled", "proxy_commands 包含 git")
		}
	}
	return newWarn("mount", "git_proxy_enabled", errcodes.AUTH_CONFIG_MISSING,
		"proxy_commands 未包含 git")
}

// checkDefaultIgnoreLoaded 检查默认二进制黑名单是否被 CLOUD_CLAUDE_NO_DEFAULT_IGNORE 禁用（D-13）。
func checkDefaultIgnoreLoaded(ctx context.Context) Check {
	if os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE") == "1" {
		return newWarn("mount", "default_ignore_loaded", errcodes.AUTH_CONFIG_MISSING,
			"CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，已禁用默认二进制黑名单")
	}
	return newPass("mount", "default_ignore_loaded", "默认二进制黑名单已加载")
}
