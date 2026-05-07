package local

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EgressMode represents how sing-box should operate.
type EgressMode string

const (
	EgressModeTun   EgressMode = "tun"
	EgressModeProxy EgressMode = "proxy"
)

// DetectEgressMode determines the sing-box mode from outbound JSON type.
// socks/http protocols use proxy mode (macOS compatible).
// Other protocols (vmess, vless, shadowsocks, trojan) use tun mode.
func DetectEgressMode(outboundJSON []byte) (EgressMode, error) {
	var config struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(outboundJSON, &config); err != nil {
		return "", fmt.Errorf("parse outbound config: %w", err)
	}
	if config.Type == "" {
		return "", fmt.Errorf("outbound config missing 'type' field")
	}
	switch config.Type {
	case "socks", "http":
		return EgressModeProxy, nil
	default:
		return EgressModeTun, nil
	}
}

// ValidateEgressConfig reads and validates an egress config file.
// Returns the raw JSON bytes and the detected mode.
func ValidateEgressConfig(filePath string) ([]byte, EgressMode, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("egress config 文件不存在: %s", absPath)
		}
		return nil, "", fmt.Errorf("读取 egress config: %w", err)
	}

	if len(data) == 0 {
		return nil, "", fmt.Errorf("egress config 文件为空: %s", absPath)
	}

	// Validate JSON
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", fmt.Errorf("egress config 不是合法 JSON: %w", err)
	}

	mode, err := DetectEgressMode(data)
	if err != nil {
		return nil, "", fmt.Errorf("检测 egress 模式: %w", err)
	}

	return data, mode, nil
}

// egressMountArg returns the docker create bind mount argument for the egress config file.
func egressMountArg(filePath string) string {
	absPath, _ := filepath.Abs(filePath)
	return absPath + ":/etc/cloud-claude/sing-box-outbound.json:ro"
}
