package cloudclaude

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	configDirName  = ".cloud-claude"
	configFileName = "config.yaml"
	dirPerm        = 0700
	filePerm       = 0600
)

var DefaultProxyCommands = []string{"git"}

type Config struct {
	Gateway          string   `yaml:"gateway"`
	Username         string   `yaml:"username"`
	Password         string   `yaml:"password"`
	ProxyCommands    []string `yaml:"proxy_commands,omitempty"`
	HotSyncMaxFileMB int      `yaml:"hot_sync_max_file_mb,omitempty"`
}

// EffectiveProxyCommands 返回生效的代理命令列表。
func (c *Config) EffectiveProxyCommands() []string {
	if len(c.ProxyCommands) > 0 {
		return c.ProxyCommands
	}
	return DefaultProxyCommands
}

const defaultHotSyncMaxFileMB = 50

// EffectiveHotSyncMaxFileMB 返回有效的单文件热同步大小上限（MB）。
// 零值或负值时返回默认值 50MB（D-04）。
func (c *Config) EffectiveHotSyncMaxFileMB() int {
	if c.HotSyncMaxFileMB <= 0 {
		return defaultHotSyncMaxFileMB
	}
	return c.HotSyncMaxFileMB
}

func (c *Config) Validate() error {
	if c.Gateway == "" {
		return fmt.Errorf("gateway 不能为空")
	}
	if c.Username == "" {
		return fmt.Errorf("username 不能为空")
	}
	if c.Password == "" {
		return fmt.Errorf("password 不能为空")
	}
	return nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户主目录: %w", err)
	}
	return filepath.Join(home, configDirName), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("配置文件不存在，请先运行 cloud-claude init")
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置无效: %w", err)
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	path := filepath.Join(dir, configFileName)
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
