package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubBindingStore struct {
	host      repository.Host
	hostErr   error
	binding   repository.HostBinding
	bindErr   error
	unbindErr error
	hostID    string
	hostIDErr error
	// Phase 51 Plan 09：双绑 pre-check 字段。
	// existingEgressHostID = 当前 egress_ip_id 已绑定到的 host_id；
	// existingEgressErr = lookup error；默认 sql.ErrNoRows 表示「未绑定」。
	existingEgressHostID string
	existingEgressErr    error
}

func (s *stubBindingStore) GetHost(_ context.Context, _ string) (repository.Host, error) {
	return s.host, s.hostErr
}

func (s *stubBindingStore) BindEgressIPToHost(_ context.Context, _, _ string) (repository.HostBinding, error) {
	return s.binding, s.bindErr
}

func (s *stubBindingStore) UnbindEgressIPFromHost(_ context.Context, _ string) error {
	return s.unbindErr
}

func (s *stubBindingStore) GetBindingHostID(_ context.Context, _ string) (string, error) {
	return s.hostID, s.hostIDErr
}

func (s *stubBindingStore) GetBindingHostIDByEgressIP(_ context.Context, _ string) (string, error) {
	if s.existingEgressErr != nil {
		return "", s.existingEgressErr
	}
	if s.existingEgressHostID == "" {
		// 默认行为：未配置 stub 字段 → 视为未绑定（与 Phase 47 之前测试预期一致）
		return "", sql.ErrNoRows
	}
	return s.existingEgressHostID, nil
}

func TestAdminBindingsHandler(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stoppedHost := repository.Host{ID: "h1", Status: "stopped", CreatedAt: now, UpdatedAt: now}
	runningHost := repository.Host{ID: "h2", Status: "running", CreatedAt: now, UpdatedAt: now}
	sampleBinding := repository.HostBinding{
		BindingID: "b1", HostID: "h1", EgressIPID: "ip1", CreatedAt: now,
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		store      *stubBindingStore
		wantStatus int
		wantField  string
	}{
		{
			name:   "Bind 201 success",
			method: "POST",
			path:   "/v1/admin/bindings",
			body:   map[string]string{"host_id": "h1", "egress_ip_id": "ip1"},
			store: &stubBindingStore{
				host: stoppedHost, binding: sampleBinding,
			},
			wantStatus: 201,
			wantField:  "binding",
		},
		{
			name:       "Bind missing host_id 400",
			method:     "POST",
			path:       "/v1/admin/bindings",
			body:       map[string]string{"egress_ip_id": "ip1"},
			store:      &stubBindingStore{},
			wantStatus: 400,
		},
		{
			name:       "Bind missing egress_ip_id 400",
			method:     "POST",
			path:       "/v1/admin/bindings",
			body:       map[string]string{"host_id": "h1"},
			store:      &stubBindingStore{},
			wantStatus: 400,
		},
		{
			name:   "Bind host not found 404",
			method: "POST",
			path:   "/v1/admin/bindings",
			body:   map[string]string{"host_id": "missing", "egress_ip_id": "ip1"},
			store: &stubBindingStore{
				hostErr: sql.ErrNoRows,
			},
			wantStatus: 404,
		},
		{
			name:   "Bind running host 409",
			method: "POST",
			path:   "/v1/admin/bindings",
			body:   map[string]string{"host_id": "h2", "egress_ip_id": "ip1"},
			store: &stubBindingStore{
				host: runningHost,
			},
			wantStatus: 409,
		},
		{
			name:   "Unbind 204 success",
			method: "DELETE",
			path:   "/v1/admin/bindings/b1",
			store: &stubBindingStore{
				hostID: "h1", host: stoppedHost,
			},
			wantStatus: 204,
		},
		{
			name:   "Unbind binding not found 404",
			method: "DELETE",
			path:   "/v1/admin/bindings/missing",
			store: &stubBindingStore{
				hostIDErr: sql.ErrNoRows,
			},
			wantStatus: 404,
		},
		{
			name:   "Unbind running host 409",
			method: "DELETE",
			path:   "/v1/admin/bindings/b2",
			store: &stubBindingStore{
				hostID: "h2", host: runningHost,
			},
			wantStatus: 409,
		},
		// Phase 51 Plan 09 / 闭 Phase 47 D-47-3：双绑互斥 pre-check
		{
			name:   "Bind double-bind to another host 409 with error_code",
			method: "POST",
			path:   "/v1/admin/bindings",
			body:   map[string]string{"host_id": "h1", "egress_ip_id": "ip1"},
			store: &stubBindingStore{
				host:                 stoppedHost,
				existingEgressHostID: "h2", // ip1 已绑定到 h2
			},
			wantStatus: 409,
		},
		{
			name:   "Bind same host re-bind 201 idempotent",
			method: "POST",
			path:   "/v1/admin/bindings",
			body:   map[string]string{"host_id": "h1", "egress_ip_id": "ip1"},
			store: &stubBindingStore{
				host:                 stoppedHost,
				binding:              sampleBinding,
				existingEgressHostID: "h1", // 同 host，pre-check 跳过
			},
			wantStatus: 201,
			wantField:  "binding",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			events := &stubEventRecorder{}
			router := adminTestRouter(t, Dependencies{
				Logger:        slog.Default(),
				AdminBindings: tt.store,
				EventRecorder: events,
			})
			srv := httptest.NewServer(router)
			defer srv.Close()

			var body []byte
			if tt.body != nil {
				body, _ = json.Marshal(tt.body)
			}

			req, _ := nethttp.NewRequest(tt.method, srv.URL+tt.path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
			req.Header.Set("Content-Type", "application/json")

			resp, err := nethttp.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				var respBody map[string]any
				json.NewDecoder(resp.Body).Decode(&respBody)
				t.Errorf("status = %d, want %d; body = %v", resp.StatusCode, tt.wantStatus, respBody)
				return
			}

			if tt.wantField != "" && resp.StatusCode != 204 {
				var respBody map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if _, ok := respBody[tt.wantField]; !ok {
					t.Errorf("response missing field %q: %v", tt.wantField, respBody)
				}
			}
		})
	}
}

// TestAdminBindings_DoubleBind_ErrorCode Phase 51 Plan 09 / 闭 Phase 47 D-47-3：
// 显式断言双绑互斥 409 响应同时携带：
//   - error_code = "egress_ip_already_bound"（稳定机器可读常量；锁
//     ErrCodeEgressIPAlreadyBound）
//   - error message 含中文「已绑定」+ 英文子串「already bound」（与 Phase 47
//     helpers ParseBindEgressIPResponse 锁定的英文断言兼容）
//   - host_id = 实际占用的 host
//   - egress_ip_id 回显请求体
func TestAdminBindings_DoubleBind_ErrorCode(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stoppedHost := repository.Host{ID: "h1", Status: "stopped", CreatedAt: now, UpdatedAt: now}

	store := &stubBindingStore{
		host:                 stoppedHost,
		existingEgressHostID: "h2",
	}
	events := &stubEventRecorder{}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminBindings: store,
		EventRecorder: events,
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"host_id": "h1", "egress_ip_id": "ip1"})
	req, _ := nethttp.NewRequest("POST", srv.URL+"/v1/admin/bindings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	req.Header.Set("Content-Type", "application/json")

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}

	var respBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if code, _ := respBody["error_code"].(string); code != ErrCodeEgressIPAlreadyBound {
		t.Errorf("error_code = %q, want %q (body=%v)", code, ErrCodeEgressIPAlreadyBound, respBody)
	}

	msg, _ := respBody["error"].(string)
	if msg == "" {
		t.Errorf("error message empty (body=%v)", respBody)
	}
	for _, sub := range []string{"已绑定", "already bound"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error message %q missing substring %q", msg, sub)
		}
	}

	if hid, _ := respBody["host_id"].(string); hid != "h2" {
		t.Errorf("host_id in response = %q, want %q", hid, "h2")
	}
	if eid, _ := respBody["egress_ip_id"].(string); eid != "ip1" {
		t.Errorf("egress_ip_id in response = %q, want %q", eid, "ip1")
	}
}

// Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor
// Phase 54 / CTRL-04：双绑互斥契约保护测试（单容器化重构的不变式锁）。
//
// 单容器化重构（Plan 54-01 + 54-02）改动了 internal/network/container_proxy_provider.go
// 大幅删行 ≥ 300 行（删 gw 容器创建 / bridge 创建 / teardownGateway / host 端 nft 注入）；
// 本测试作为不变式锁，确认 admin_bindings.go 的 Bind() pre-check 链路：
//
//  1. HTTP status = 409
//  2. JSON body error_code = "egress_ip_already_bound"（机器可读稳定常量）
//  3. JSON body error 含中文「已绑定」+ 英文「already bound」（v3.6 51-09 双语契约）
//  4. JSON body host_id = 既有绑定的 host_id（不是请求 host_id）
//  5. JSON body egress_ip_id = 请求 egress_ip_id 回显
//
// 与 v3.6 TestAdminBindings_DoubleBind_ErrorCode 用例并存（不替代）：
//   - v3.6 用例：Phase 51 落地时的契约验证（功能新增）
//   - 本用例：Phase 54 重构后的契约保护（不变式锁）
//
// 若本测试在 Phase 54 / Phase 55 / 后续任何 milestone 失败，说明双绑契约
// 被无意打破，必须立即恢复。两份用例并存是设计意图（D-54-11）：合并需走
// Phase 56 / v4.1 显式 audit。
func Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stoppedHost := repository.Host{ID: "host-requesting", Status: "stopped", CreatedAt: now, UpdatedAt: now}
	occupierHostID := "host-already-occupying"
	requestEgressIPID := "eip-contested"

	store := &stubBindingStore{
		host:                 stoppedHost,
		existingEgressHostID: occupierHostID,
	}
	events := &stubEventRecorder{}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminBindings: store,
		EventRecorder: events,
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{
		"host_id":      stoppedHost.ID,
		"egress_ip_id": requestEgressIPID,
	})
	req, _ := nethttp.NewRequest("POST", srv.URL+"/v1/admin/bindings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	req.Header.Set("Content-Type", "application/json")

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// 契约 1: HTTP 409
	if resp.StatusCode != nethttp.StatusConflict {
		t.Fatalf("[contract-1 status] = %d, want 409 (Phase 51-09 双绑互斥)", resp.StatusCode)
	}

	var respBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// 契约 2: error_code 稳定字面量
	if code, _ := respBody["error_code"].(string); code != ErrCodeEgressIPAlreadyBound {
		t.Errorf("[contract-2 error_code] = %q, want %q (body=%v)",
			code, ErrCodeEgressIPAlreadyBound, respBody)
	}
	if ErrCodeEgressIPAlreadyBound != "egress_ip_already_bound" {
		t.Errorf("[contract-2 constant-value] ErrCodeEgressIPAlreadyBound = %q, want %q",
			ErrCodeEgressIPAlreadyBound, "egress_ip_already_bound")
	}

	// 契约 3: 中英双语 message
	msg, _ := respBody["error"].(string)
	if msg == "" {
		t.Errorf("[contract-3 message-nonempty] error message empty (body=%v)", respBody)
	}
	for _, sub := range []string{"已绑定", "already bound"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("[contract-3 bilingual] message %q missing %q", msg, sub)
		}
	}

	// 契约 4: host_id = 既有占用者，不是请求者
	if hid, _ := respBody["host_id"].(string); hid != occupierHostID {
		t.Errorf("[contract-4 host_id] = %q, want %q (must be the occupier, not requester)",
			hid, occupierHostID)
	}

	// 契约 5: egress_ip_id 回显
	if eid, _ := respBody["egress_ip_id"].(string); eid != requestEgressIPID {
		t.Errorf("[contract-5 egress_ip_id] = %q, want %q", eid, requestEgressIPID)
	}
}
