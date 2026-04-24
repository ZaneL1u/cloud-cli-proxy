package cloudclaude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LastSessionSnapshot 是 ~/.cloud-claude/last-session.json 的 schema_version=1 结构。
//
// 由 mount_strategy.go 在每次 mount 路径完成（成功或失败均写）时落盘。
// Phase 34 doctor 第一屏读取展示降级历史与 conflict 计数。
type LastSessionSnapshot struct {
	SchemaVersion       int             `json:"schema_version"`
	Timestamp           time.Time       `json:"timestamp"`
	IntendedMode        string          `json:"intended_mode"`
	ActualMode          string          `json:"actual_mode"`
	DowngradeChain      []DowngradeStep `json:"downgrade_chain"`
	ConflictCount       int             `json:"conflict_count"`
	ClaudeAccountID     string          `json:"claude_account_id,omitempty"`
	ImageVersion        string          `json:"image_version,omitempty"`
	APFSCaseInsensitive bool            `json:"apfs_case_insensitive"`

	// [Phase 32 D-27 新增] 全部 omitempty + schema_version 保持 1（向后兼容）
	TmuxSession    string `json:"tmux_session,omitempty"`    // 实际 attach 的 tmux session 名（Plan 02 写）
	ClientRole     string `json:"client_role,omitempty"`     // "primary" | "secondary"（Plan 03 写）
	ReconnectCount int    `json:"reconnect_count,omitempty"` // Reconnector.ReconnectCount() 累计值

	// [Phase 36 D-09] omitempty + schema_version=1 不变（向后兼容）
	OversizedFiles []OversizedFile `json:"oversized_files,omitempty"`

	// [Phase 37 D-12] promotion 统计（omitempty + schema_version=1 不变）
	PromotionCount       int   `json:"promotion_count,omitempty"`
	PromotionBytes       int64 `json:"promotion_bytes,omitempty"`
	PromotionFailedCount int   `json:"promotion_failed_count,omitempty"`
}

// DowngradeStep 描述单次降级动作。
type DowngradeStep struct {
	From          string `json:"from"`
	To            string `json:"to"`
	ReasonCode    string `json:"reason_code"`
	ReasonMessage string `json:"reason_message"`
}

// OversizedFile 描述热同步阶段因超过大小阈值而被跳过的单个文件。
// 对应 REQUIREMENTS.md REQ-MOUNT-V31-03 / Phase 36 D-09。
type OversizedFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

// WriteLastSession 把 snap 序列化为 JSON 后原子写入 path。
//
// 行为：
//  1. 父目录不存在则 0700 创建（与 ConfigDir 一致）
//  2. SchemaVersion 字段为 0 时强制设置为 1（防御调用方遗漏）
//  3. 写到 path+".tmp" 后 os.Rename → 原子替换
//  4. 失败仅返回 error，不 panic（调用方在 stderr warning 但不阻断 mount 路径）
func WriteLastSession(path string, snap LastSessionSnapshot) error {
	if snap.SchemaVersion == 0 {
		snap.SchemaVersion = 1
	}
	if snap.Timestamp.IsZero() {
		snap.Timestamp = time.Now().UTC()
	}
	if snap.DowngradeChain == nil {
		snap.DowngradeChain = []DowngradeStep{}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建 last-session 目录失败: %w", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 last-session 失败: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写 last-session 临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename last-session 失败: %w", err)
	}
	return nil
}

// DefaultLastSessionPath 返回默认 last-session.json 路径（~/.cloud-claude/last-session.json）。
// home 目录无法定位时返回空串 + error；调用方应当 best-effort 处理（doctor 第一屏 nil banner 兜底）。
func DefaultLastSessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位 home 目录: %w", err)
	}
	return filepath.Join(home, ".cloud-claude", "last-session.json"), nil
}

// LoadLastSession 从默认路径读取 last-session.json，反序列化为 LastSessionSnapshot。
//
// 行为：
//  1. 文件不存在 → (nil, error)；调用方按 nil 兜底（doctor 第一屏走 STATE_LAST_SESSION_MISSING）
//  2. JSON 解析失败 → (nil, error)
//  3. 成功 → (*snap, nil)
//
// Phase 34 Plan 02 Task 2.10：doctor RunDoctor 复用，读后传给 convertSnapshotToBanner。
func LoadLastSession() (*LastSessionSnapshot, error) {
	path, err := DefaultLastSessionPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap LastSessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("解析 last-session 失败: %w", err)
	}
	return &snap, nil
}
