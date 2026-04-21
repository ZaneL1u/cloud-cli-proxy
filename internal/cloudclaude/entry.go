package cloudclaude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultPollInterval = 3 * time.Second
	DefaultPollTimeout  = 120 * time.Second
)

type AuthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`

	SSHUser string `json:"ssh_user,omitempty"`
	SSHPass string `json:"ssh_pass,omitempty"`
	SSHHost string `json:"ssh_host,omitempty"`
	SSHPort int    `json:"ssh_port,omitempty"`

	// Phase 30 D-03/D-06/D-08：v3 gateway 在 ready 响应追加的增量能力字段。
	// omitempty 是契约：旧 gateway 缺省这些字段时不会变成「明确的 false/空串」，
	// 客户端 SSH 四元组校验依旧以 SSHHost/SSHPort/SSHUser 为唯一真值。
	ImageVersion     string `json:"image_version,omitempty"`
	SupportsMergerfs bool   `json:"supports_mergerfs,omitempty"`
	// ClaudeAccountID 由控制面按 D-05 的两阶段规则解析；未命中则省略字段。
	ClaudeAccountID string `json:"claude_account_id,omitempty"`
}

type EntryClient struct {
	httpClient   *http.Client
	gateway      string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

func NewEntryClient(gateway string) *EntryClient {
	return &EntryClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		gateway:      gateway,
		pollInterval: DefaultPollInterval,
		pollTimeout:  DefaultPollTimeout,
	}
}

func (c *EntryClient) CheckGateway(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.gateway+"/", nil)
	if err != nil {
		return fmt.Errorf("网关地址无效: %w", err)
	}
	req.Header.Set("User-Agent", "cloud-claude/1.0")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网关不可达: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (c *EntryClient) Authenticate(ctx context.Context, shortID, password string) (*AuthResponse, error) {
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/entry/%s/auth", c.gateway, shortID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造认证请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cloud-claude/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("认证请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取认证响应失败: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusBadRequest:
		return nil, fmt.Errorf("认证失败：请求参数无效")
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("认证失败：用户名或密码错误")
	case http.StatusForbidden:
		return nil, fmt.Errorf("账号未激活，请联系管理员")
	case http.StatusNotFound:
		return nil, fmt.Errorf("未找到对应主机，请联系管理员")
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("服务器内部错误，请稍后重试")
	default:
		return nil, fmt.Errorf("认证请求异常（HTTP %d）", resp.StatusCode)
	}

	var authResp AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("解析认证响应失败: %w", err)
	}

	if authResp.Status == "ready" {
		if authResp.SSHHost == "" || authResp.SSHPort == 0 || authResp.SSHUser == "" {
			return nil, fmt.Errorf("服务器返回的 SSH 连接参数不完整")
		}
	}

	return &authResp, nil
}

func (c *EntryClient) AuthenticateAndWait(ctx context.Context, shortID, password string, onWait func(msg string)) (*AuthResponse, error) {
	if err := c.CheckGateway(ctx); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(c.pollTimeout)

	for {
		resp, err := c.Authenticate(ctx, shortID, password)
		if err != nil {
			return nil, err
		}

		if resp.Status == "ready" {
			return resp, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("等待主机就绪超时（已等待 %v）", c.pollTimeout)
		}

		msg := "主机正在启动中，请稍候..."
		if resp.Message != "" {
			msg = resp.Message
		}
		if onWait != nil {
			onWait(msg)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}
