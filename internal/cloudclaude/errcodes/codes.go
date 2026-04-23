// Package errcodes 是 Phase 31 引入的统一错误码注册表雏形。
// Phase 34 doctor / explain 直接复用本包的 Registry / Lookup / Format。
// 命名规范：^[A-Z]+_[A-Z]+_[A-Z0-9]+$（DOMAIN_KIND_NAME，全大写下划线）。
//
//nolint:revive,stylecheck // 常量名为错误码字面量，与 doctor --explain 输入字符串保持一致，禁止 camelCase
package errcodes

import (
	"fmt"
	"regexp"
	"sync"
)

// Code 是错误码的字面值，例如 "MOUNT_MUTAGEN_VERSION_SKEW"。
type Code string

// Severity 表示错误码的严重程度，用于日志着色与 doctor 排序。
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
	SeverityFatal
)

// String 返回大写枚举名（INFO / WARN / ERROR / FATAL），仅用于日志输出。
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	case SeverityFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Entry 是注册表中的一条错误码定义。
//   - Message 可以包含 fmt 占位符（%s/%d），由 Format(args...) 渲染。
//   - NextAction 必须是中文，长度 ≤ 80 runes。
type Entry struct {
	Code       Code
	Severity   Severity
	Message    string
	NextAction string
}

var (
	registryMu sync.RWMutex
	registry   = map[Code]Entry{}
	codeRe     = regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`)
)

// MustRegister 注册一条错误码。重复 code、命名不合法、Message/NextAction 为空时直接 panic。
// 由各域文件的 init() 调用，问题在进程启动时即暴露。
func MustRegister(e Entry) {
	if !codeRe.MatchString(string(e.Code)) {
		panic(fmt.Sprintf("errcodes: 非法 code 命名 %q（必须匹配 ^[A-Z]+_[A-Z]+_[A-Z0-9]+$）", e.Code))
	}
	if e.Message == "" {
		panic(fmt.Sprintf("errcodes: code %q Message 不能为空", e.Code))
	}
	if e.NextAction == "" {
		panic(fmt.Sprintf("errcodes: code %q NextAction 不能为空", e.Code))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[e.Code]; exists {
		panic(fmt.Sprintf("errcodes: 重复注册 code %q", e.Code))
	}
	registry[e.Code] = e
}

// Lookup 根据 Code 取出 Entry；未注册返回 (zero, false)。
func Lookup(c Code) (Entry, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	e, ok := registry[c]
	return e, ok
}

// Registry 返回注册表的浅拷贝，避免外部直接修改内部 map。
// Phase 34 doctor / explain 子命令复用此函数遍历全部错误码。
func Registry() map[Code]Entry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make(map[Code]Entry, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// Format 渲染统一两段输出：
//
//	[<CODE>] <Message>
//	  建议: <NextAction>
//
// args 用于填充 Message 中的 %s/%d 占位。code 未注册时返回带 "(unknown code)" 的占位字符串，不 panic。
func Format(c Code, args ...any) string {
	registryMu.RLock()
	e, ok := registry[c]
	registryMu.RUnlock()
	if !ok {
		return fmt.Sprintf("[%s] (unknown code)\n  建议: 联系维护者", c)
	}
	msg := e.Message
	if len(args) > 0 {
		msg = fmt.Sprintf(e.Message, args...)
	}
	return fmt.Sprintf("[%s] %s\n  建议: %s", c, msg, e.NextAction)
}

// Code 常量：变量名与 Code 字面值完全一致，方便 grep / Phase 34 doctor --explain。
const (
	MOUNT_MUTAGEN_VERSION_SKEW       Code = "MOUNT_MUTAGEN_VERSION_SKEW"
	MOUNT_MUTAGEN_WHITELIST_REJECT   Code = "MOUNT_MUTAGEN_WHITELIST_REJECT"
	MOUNT_MUTAGEN_SAFETY_GUARD       Code = "MOUNT_MUTAGEN_SAFETY_GUARD"
	MOUNT_MUTAGEN_DAEMON_UNAVAILABLE Code = "MOUNT_MUTAGEN_DAEMON_UNAVAILABLE"
	MOUNT_MUTAGEN_SYNC_FAILED        Code = "MOUNT_MUTAGEN_SYNC_FAILED"
	MOUNT_MUTAGEN_TRANSPORT_FAILED   Code = "MOUNT_MUTAGEN_TRANSPORT_FAILED"
	MOUNT_HOT_SYNC_FAILED            Code = "MOUNT_HOT_SYNC_FAILED"
	MOUNT_SSHFS_FAILED               Code = "MOUNT_SSHFS_FAILED"
	MOUNT_SSHFS_DISCONNECTED         Code = "MOUNT_SSHFS_DISCONNECTED"
	MOUNT_MERGERFS_FAILED            Code = "MOUNT_MERGERFS_FAILED"
	MOUNT_AUTO_DOWNGRADED            Code = "MOUNT_AUTO_DOWNGRADED"
	MOUNT_FORCE_MODE_FAILED          Code = "MOUNT_FORCE_MODE_FAILED"
	MOUNT_APFS_CASE_INSENSITIVE      Code = "MOUNT_APFS_CASE_INSENSITIVE"
	MOUNT_OVERSIZED_FILE_SKIPPED     Code = "MOUNT_OVERSIZED_FILE_SKIPPED"
	MOUNT_REQUIRE_GIT_REPO           Code = "MOUNT_REQUIRE_GIT_REPO"
	NET_OAUTH_EXPIRED                Code = "NET_OAUTH_EXPIRED"
	NET_OAUTH_EXPIRING_SOON          Code = "NET_OAUTH_EXPIRING_SOON"
	NET_OAUTH_NOT_FOUND              Code = "NET_OAUTH_NOT_FOUND"
	SESSION_KEEPALIVE_TOO_AGGRESSIVE Code = "SESSION_KEEPALIVE_TOO_AGGRESSIVE"
	SESSION_TMUX_UNAVAILABLE         Code = "SESSION_TMUX_UNAVAILABLE"
	SESSION_NOT_FOUND                Code = "SESSION_NOT_FOUND"
	SESSION_TAKEOVER_NOTIFIED        Code = "SESSION_TAKEOVER_NOTIFIED"
	SESSION_TAKEOVER_FAILED          Code = "SESSION_TAKEOVER_FAILED"
	SESSION_SYNC_LOCKED              Code = "SESSION_SYNC_LOCKED"
	SESSION_BUFFER_OVERFLOW          Code = "SESSION_BUFFER_OVERFLOW"
	NET_RECONNECT_BACKOFF            Code = "NET_RECONNECT_BACKOFF"
	NET_RECONNECT_GAVE_UP            Code = "NET_RECONNECT_GAVE_UP"
	NET_TCP_KEEPALIVE_UNSUPPORTED    Code = "NET_TCP_KEEPALIVE_UNSUPPORTED"

	// Phase 34 D-21 新增：8 域闭合第 4-8 域（STATE / SYSTEM / SSH / AUTH / DISK）

	// STATE_* （持久化 / volume / 容器状态）
	STATE_LAST_SESSION_MISSING  Code = "STATE_LAST_SESSION_MISSING"
	STATE_VOLUME_IN_USE_001     Code = "STATE_VOLUME_IN_USE_001"
	STATE_CONTAINER_NOT_RUNNING Code = "STATE_CONTAINER_NOT_RUNNING"

	// SYSTEM_* （OS / kernel / FUSE / DNS / timeout）
	SYSTEM_APPARMOR_FUSERMOUNT3_MISSING Code = "SYSTEM_APPARMOR_FUSERMOUNT3_MISSING"
	SYSTEM_FUSE_RESIDUAL_MOUNT          Code = "SYSTEM_FUSE_RESIDUAL_MOUNT"
	SYSTEM_DNS_RESOLVE_FAILED           Code = "SYSTEM_DNS_RESOLVE_FAILED"
	SYSTEM_CHECK_TIMEOUT                Code = "SYSTEM_CHECK_TIMEOUT"

	// SSH_* （known_hosts / sshd 基线漂移）
	SSH_KNOWN_HOSTS_CONFLICT Code = "SSH_KNOWN_HOSTS_CONFLICT"
	SSH_SSHD_KEEPALIVE_DRIFT Code = "SSH_SSHD_KEEPALIVE_DRIFT"

	// AUTH_* （CLI 配置 / Entry token / OAuth refresh）
	AUTH_CONFIG_MISSING       Code = "AUTH_CONFIG_MISSING"
	AUTH_GATEWAY_UNREACHABLE  Code = "AUTH_GATEWAY_UNREACHABLE"
	AUTH_TOKEN_EXPIRED        Code = "AUTH_TOKEN_EXPIRED"
	AUTH_OAUTH_REFRESH_FAILED Code = "AUTH_OAUTH_REFRESH_FAILED"

	// NET_* 扩展（doctor network.egress_ip 检查）
	NET_EGRESS_IP_DRIFT Code = "NET_EGRESS_IP_DRIFT"

	// DISK_* （本地 / 容器 disk usage）
	DISK_LOCAL_LOW          Code = "DISK_LOCAL_LOW"
	DISK_CONTAINER_LOW      Code = "DISK_CONTAINER_LOW"
	DISK_MUTAGEN_DATA_BLOAT Code = "DISK_MUTAGEN_DATA_BLOAT"
)
