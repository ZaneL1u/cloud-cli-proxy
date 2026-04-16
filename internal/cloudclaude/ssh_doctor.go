package cloudclaude

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
)

// SSHDoctorOptions 控制 ssh doctor 行为。
type SSHDoctorOptions struct {
	Fix bool
}

// FileReport 描述 /workspace/.ssh 下单个文件的健康状况。
type FileReport struct {
	Path       string
	Kind       string // "private" | "public" | "authorized_keys" | "known_hosts" | "config" | "other"
	Owner      string // 形如 "workspace:workspace" 或 "root:root"
	Mode       string // 形如 "0600"
	FirstLine  string // 仅保留，便于报告调试
	OwnerOK    bool
	ModeOK     bool
	PEMEndsNL  *bool // 仅 Kind=="private" 时有意义
	FixApplied []string
	FixFailed  []string
}

// SSHDoctorResult 是一次体检的结构化结果。
type SSHDoctorResult struct {
	User    string
	SSHDir  string
	Files   []FileReport
	Missing bool // 目标目录不存在
	FixMode bool
	SudoOK  bool
}

const defaultSSHDir = "/workspace/.ssh"

// scanScript 是一次性远端扫描脚本。输出格式：
//
//	USER=<name>
//	SUDO_OK=y|n
//	SSHDIR_MISSING=<path>    (仅当目录不存在时出现，后续无 FILE 行)
//	FILE|<relpath>|<owner>|<mode>|<firstline>|<lastbyte_hex>
//
// firstline 已 tr 掉 '|' 与 '\r'，防止破坏列分隔。
const scanScript = `set -u
D=/workspace/.ssh
echo "USER=$(id -un)"
echo "SUDO_OK=$(sudo -n true 2>/dev/null && echo y || echo n)"
if [ ! -d "$D" ]; then
  echo "SSHDIR_MISSING=$D"
  exit 0
fi
cd "$D" || exit 0
find . -maxdepth 1 -type f -print0 2>/dev/null | sort -z | while IFS= read -r -d '' f; do
  rel="${f#./}"
  path="$D/$rel"
  owner=$(stat -c '%U:%G' "$path" 2>/dev/null || echo "?:?")
  mode=$(stat -c '%a' "$path" 2>/dev/null || echo "?")
  first=$(head -n 1 "$path" 2>/dev/null | tr -d '\r|')
  last=$(tail -c 1 "$path" 2>/dev/null | od -An -tx1 | tr -d ' \n')
  printf 'FILE|%s|%s|%s|%s|%s\n' "$rel" "$owner" "$mode" "$first" "$last"
done
`

// RunSSHDoctor 通过 SSH 连接远端容器，扫描 /workspace/.ssh 下所有文件的健康状况；
// 若 opts.Fix 为 true，则尝试逐项修复（chown / chmod / 追加 PEM 末尾换行）。
func RunSSHDoctor(cfg SSHConfig, opts SSHDoctorOptions) (*SSHDoctorResult, error) {
	conn, err := sshConnect(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	raw, err := runSSHSession(conn, scanScript)
	if err != nil {
		return nil, fmt.Errorf("远端扫描失败: %w", err)
	}

	result := parseScanOutput(raw)
	result.FixMode = opts.Fix

	if opts.Fix {
		applyFixes(conn, result)
	}

	return result, nil
}

// runSSHSession 在一个新 session 里跑一条脚本，返回 stdout。
// 出错时把 stderr / stdout 一并带到 error 里便于定位。
func runSSHSession(conn *ssh.Client, script string) (string, error) {
	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(script); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("%w (%s)", err, msg)
	}
	return stdout.String(), nil
}

// parseScanOutput 解析远端扫描脚本的 stdout，生成 SSHDoctorResult。
// 拆成独立纯函数以便单测不依赖真实 SSH。
func parseScanOutput(raw string) *SSHDoctorResult {
	r := &SSHDoctorResult{
		SSHDir: defaultSSHDir,
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "USER="):
			r.User = strings.TrimPrefix(line, "USER=")
		case strings.HasPrefix(line, "SUDO_OK="):
			r.SudoOK = strings.TrimPrefix(line, "SUDO_OK=") == "y"
		case strings.HasPrefix(line, "SSHDIR_MISSING="):
			r.Missing = true
			r.SSHDir = strings.TrimPrefix(line, "SSHDIR_MISSING=")
		case strings.HasPrefix(line, "FILE|"):
			if fr, ok := parseFileRow(line); ok {
				fr.Path = r.SSHDir + "/" + fr.Path
				fillFileExpectations(&fr, r.User)
				r.Files = append(r.Files, fr)
			}
		}
	}
	sort.Slice(r.Files, func(i, j int) bool {
		return r.Files[i].Path < r.Files[j].Path
	})
	return r
}

// parseFileRow 解析一行 FILE|rel|owner|mode|firstline|lastbytehex。
func parseFileRow(line string) (FileReport, bool) {
	parts := strings.Split(line, "|")
	if len(parts) < 6 || parts[0] != "FILE" {
		return FileReport{}, false
	}
	fr := FileReport{
		Path:      parts[1],
		Owner:     parts[2],
		Mode:      normalizeMode(parts[3]),
		FirstLine: parts[4],
	}
	fr.Kind = detectKind(parts[1], parts[4])
	if fr.Kind == "private" {
		ok := strings.ToLower(strings.TrimSpace(parts[5])) == "0a"
		fr.PEMEndsNL = &ok
	}
	return fr, true
}

// normalizeMode 把 "600" / "0600" / "644" 统一成四位。
func normalizeMode(m string) string {
	m = strings.TrimSpace(m)
	if m == "" || m == "?" {
		return m
	}
	if len(m) == 3 {
		return "0" + m
	}
	return m
}

// detectKind 基于文件名 + 首行判定文件类型。
func detectKind(name, firstLine string) string {
	switch name {
	case "authorized_keys":
		return "authorized_keys"
	case "known_hosts", "known_hosts.old":
		return "known_hosts"
	case "config":
		return "config"
	}
	if strings.HasSuffix(name, ".pub") {
		return "public"
	}
	if detectPrivateKey(firstLine) {
		return "private"
	}
	return "other"
}

// detectPrivateKey 判定首行是否 PEM 私钥头。
func detectPrivateKey(firstLine string) bool {
	trimmed := strings.TrimSpace(firstLine)
	if !strings.HasPrefix(trimmed, "-----BEGIN ") {
		return false
	}
	return strings.Contains(trimmed, "PRIVATE KEY-----")
}

// expectedMode 返回某类文件期望的 mode 字符串。返回 "" 表示不对该类文件做 mode 判定。
func expectedMode(kind string) string {
	switch kind {
	case "private", "authorized_keys", "known_hosts", "config":
		return "0600"
	case "public":
		return "0644"
	default:
		return ""
	}
}

// pemEndsWithNewline 判定 PEM 文件的最后一字节是否 '\n'。
func pemEndsWithNewline(lastByte byte) bool {
	return lastByte == '\n'
}

// fillFileExpectations 基于 Kind、当前用户与 stat 结果，填写 OwnerOK / ModeOK。
func fillFileExpectations(fr *FileReport, currentUser string) {
	// Owner：比较 owner 字段的用户部分（':' 前半段）。
	ownerUser := fr.Owner
	if idx := strings.IndexByte(fr.Owner, ':'); idx >= 0 {
		ownerUser = fr.Owner[:idx]
	}
	fr.OwnerOK = currentUser != "" && ownerUser == currentUser

	// Mode：有期望值才判定；否则视为 OK。
	want := expectedMode(fr.Kind)
	if want == "" {
		fr.ModeOK = true
	} else {
		fr.ModeOK = fr.Mode == want
	}
}

// applyFixes 对每个问题项尝试修复，更新 FileReport.FixApplied / FixFailed，
// 并在成功后重新拉取 stat / lastbyte 刷新状态。
func applyFixes(conn *ssh.Client, r *SSHDoctorResult) {
	if r.User == "" {
		return
	}
	for i := range r.Files {
		fr := &r.Files[i]

		if !fr.OwnerOK {
			fixOwner(conn, r, fr)
		}
		if !fr.ModeOK {
			fixMode(conn, r, fr)
		}
		if fr.Kind == "private" && fr.PEMEndsNL != nil && !*fr.PEMEndsNL {
			fixPEMNewline(conn, r, fr)
		}

		// 若有任何修复动作，刷新一次状态。
		if len(fr.FixApplied) > 0 {
			if nr := restatFile(conn, fr.Path, fr.Kind); nr != nil {
				fr.Owner = nr.Owner
				fr.Mode = nr.Mode
				fr.PEMEndsNL = nr.PEMEndsNL
				fillFileExpectations(fr, r.User)
			}
		}
	}
}

func fixOwner(conn *ssh.Client, r *SSHDoctorResult, fr *FileReport) {
	if !r.SudoOK {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("chown 需要 sudo 免密，但远端当前用户不具备；请手工: sudo chown %s:%s %s", r.User, r.User, fr.Path))
		return
	}
	cmd := fmt.Sprintf("sudo -n chown %s:%s %s", shellescape.Quote(r.User), shellescape.Quote(r.User), shellescape.Quote(fr.Path))
	if _, err := runSSHSession(conn, cmd); err != nil {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("chown 失败: %v", err))
		return
	}
	fr.FixApplied = append(fr.FixApplied, fmt.Sprintf("chown %s:%s", r.User, r.User))
}

func fixMode(conn *ssh.Client, r *SSHDoctorResult, fr *FileReport) {
	want := expectedMode(fr.Kind)
	if want == "" {
		return
	}
	// 先普通 chmod；失败再 sudo 降级。
	cmd := fmt.Sprintf("chmod %s %s", want, shellescape.Quote(fr.Path))
	if _, err := runSSHSession(conn, cmd); err == nil {
		fr.FixApplied = append(fr.FixApplied, fmt.Sprintf("chmod %s", want))
		return
	}
	if !r.SudoOK {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("chmod 失败且无 sudo 免密；请手工: chmod %s %s", want, fr.Path))
		return
	}
	cmd = fmt.Sprintf("sudo -n chmod %s %s", want, shellescape.Quote(fr.Path))
	if _, err := runSSHSession(conn, cmd); err != nil {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("sudo chmod 仍失败: %v", err))
		return
	}
	fr.FixApplied = append(fr.FixApplied, fmt.Sprintf("sudo chmod %s", want))
}

func fixPEMNewline(conn *ssh.Client, r *SSHDoctorResult, fr *FileReport) {
	// 自属主场景：直接 printf >> 即可；非自属主：sudo tee -a。
	quoted := shellescape.Quote(fr.Path)
	cmd := fmt.Sprintf("printf '\\n' >> %s", quoted)
	if _, err := runSSHSession(conn, cmd); err == nil {
		fr.FixApplied = append(fr.FixApplied, "追加 PEM 末尾换行")
		return
	}
	if !r.SudoOK {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("追加换行失败且无 sudo 免密；请手工: printf '\\n' >> %s", fr.Path))
		return
	}
	cmd = fmt.Sprintf("printf '\\n' | sudo -n tee -a %s > /dev/null", quoted)
	if _, err := runSSHSession(conn, cmd); err != nil {
		fr.FixFailed = append(fr.FixFailed, fmt.Sprintf("sudo tee 追加换行仍失败: %v", err))
		return
	}
	fr.FixApplied = append(fr.FixApplied, "追加 PEM 末尾换行（sudo）")
}

// restatFile 重新拉取单个文件的 owner/mode/lastbyte，用于修复后刷新。
func restatFile(conn *ssh.Client, path, kind string) *FileReport {
	script := fmt.Sprintf(`set -u
P=%s
owner=$(stat -c '%%U:%%G' "$P" 2>/dev/null || echo "?:?")
mode=$(stat -c '%%a' "$P" 2>/dev/null || echo "?")
last=$(tail -c 1 "$P" 2>/dev/null | od -An -tx1 | tr -d ' \n')
printf 'STAT|%%s|%%s|%%s\n' "$owner" "$mode" "$last"
`, shellescape.Quote(path))
	out, err := runSSHSession(conn, script)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "STAT|") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		fr := &FileReport{
			Owner: parts[1],
			Mode:  normalizeMode(parts[2]),
		}
		if kind == "private" {
			ok := strings.ToLower(strings.TrimSpace(parts[3])) == "0a"
			fr.PEMEndsNL = &ok
		}
		return fr
	}
	return nil
}

// Print 打印 SSH 密钥体检报告，风格对齐 EnvCheckResult.Print。
func (r *SSHDoctorResult) Print() {
	fmt.Println("╭─────────────────────────────────────────╮")
	fmt.Println("│    Cloud Claude SSH 密钥体检报告        │")
	fmt.Println("╰─────────────────────────────────────────╯")
	fmt.Println()

	section("目标")
	row("远端用户", r.User)
	row("SSH 目录", r.SSHDir)
	if r.FixMode {
		row("运行模式", "--fix（尝试自动修复）")
	} else {
		row("运行模式", "只读扫描")
	}
	if r.SudoOK {
		row("sudo 免密", "✓ 可用")
	} else {
		row("sudo 免密", "✗ 不可用（chown 类修复将降级为提示）")
	}
	fmt.Println()

	if r.Missing {
		fmt.Println("  ⚠ 远端 SSH 目录不存在，容器可能尚未初始化。")
		return
	}
	if len(r.Files) == 0 {
		fmt.Println("  ⚠ SSH 目录为空，未发现任何文件。")
		return
	}

	printGroup("私钥", r.Files, "private")
	printGroup("公钥", r.Files, "public")
	printGroup("authorized_keys", r.Files, "authorized_keys")
	printGroup("known_hosts", r.Files, "known_hosts")
	printGroup("config", r.Files, "config")
	printGroup("其他", r.Files, "other")

	fmt.Println()
	summarize(r)
}

// printGroup 打印某一 Kind 下的所有文件。
func printGroup(title string, files []FileReport, kind string) {
	var subset []FileReport
	for _, f := range files {
		if f.Kind == kind {
			subset = append(subset, f)
		}
	}
	if len(subset) == 0 {
		return
	}
	section(title)
	for _, f := range subset {
		icon := "✓"
		if !fileOverallOK(f) {
			icon = "✗"
		}
		parts := []string{
			fmt.Sprintf("owner=%s", f.Owner),
			fmt.Sprintf("mode=%s", safeVal(f.Mode)),
		}
		if !f.OwnerOK {
			parts[0] += " (期望 " + ownerExpectLabel(f) + ")"
		}
		if !f.ModeOK && expectedMode(f.Kind) != "" {
			parts[1] += " (期望 " + expectedMode(f.Kind) + ")"
		}
		if f.PEMEndsNL != nil {
			if *f.PEMEndsNL {
				parts = append(parts, "PEM=EOL ✓")
			} else {
				parts = append(parts, "PEM=!EOL（libcrypto 会拒绝）")
			}
		}
		fmt.Printf("  %s  %s  %s\n", icon, f.Path, strings.Join(parts, "  "))
		for _, fx := range f.FixApplied {
			fmt.Printf("       ✓ 已修复: %s\n", fx)
		}
		for _, fx := range f.FixFailed {
			fmt.Printf("       ✗ 修复失败: %s\n", fx)
		}
	}
	fmt.Println()
}

func fileOverallOK(f FileReport) bool {
	if !f.OwnerOK || !f.ModeOK {
		return false
	}
	if f.PEMEndsNL != nil && !*f.PEMEndsNL {
		return false
	}
	return true
}

func ownerExpectLabel(f FileReport) string {
	// 从上下文推断不出 User，这里用通用占位 —— 实际修复时会带上具体值。
	return "<当前用户>"
}

func safeVal(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func summarize(r *SSHDoctorResult) {
	problems := 0
	fixable := 0
	for _, f := range r.Files {
		if !fileOverallOK(f) {
			problems++
			if len(f.FixApplied) == 0 && len(f.FixFailed) == 0 {
				fixable++
			}
		}
	}
	switch {
	case problems == 0:
		fmt.Println("  ✓ 全部检查通过。")
	case r.FixMode && fixable == 0:
		fmt.Println("  ⚠ 仍有问题项，请查看上方每个文件的 \"修复失败\" 条目。")
	default:
		fmt.Println("  ⚠ 发现问题项。建议: cloud-claude ssh doctor --fix")
	}
}
