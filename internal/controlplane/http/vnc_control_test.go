package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

func TestInspectContainerVNCParsesStatusJSON(t *testing.T) {
	orig := runContainerVNCCommand
	runContainerVNCCommand = func(_ context.Context, containerName string, args ...string) ([]byte, error) {
		if containerName != "cloudproxy-h-1" {
			t.Fatalf("containerName=%q, want cloudproxy-h-1", containerName)
		}
		if strings.Join(args, " ") != "vnc-status --json" {
			t.Fatalf("args=%q, want vnc-status --json", strings.Join(args, " "))
		}
		return []byte(`{"status":"running","running":true,"can_start":false,"can_restart":true,"auto_restart_limited":false,"display":":99","websocket_port":6080}`), nil
	}
	t.Cleanup(func() { runContainerVNCCommand = orig })

	status, err := inspectContainerVNC(context.Background(), "cloudproxy-h-1", true)
	if err != nil {
		t.Fatalf("inspectContainerVNC returned error: %v", err)
	}
	if status.Status != "running" || !status.Running {
		t.Fatalf("status=%+v, want running", status)
	}
	if status.CanStart {
		t.Fatalf("running VNC must not expose can_start: %+v", status)
	}
	if !status.CanRestart {
		t.Fatalf("running VNC must expose can_restart: %+v", status)
	}
	if status.WebsocketPort != 6080 || status.Display != ":99" {
		t.Fatalf("endpoint fields=%+v, want :99/6080", status)
	}
}

func TestInspectContainerVNCForStoppedHostDoesNotExecDocker(t *testing.T) {
	orig := runContainerVNCCommand
	called := false
	runContainerVNCCommand = func(context.Context, string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { runContainerVNCCommand = orig })

	status, err := inspectContainerVNC(context.Background(), "cloudproxy-h-1", false)
	if err != nil {
		t.Fatalf("inspectContainerVNC returned error: %v", err)
	}
	if called {
		t.Fatal("stopped host must not docker exec for VNC status")
	}
	if status.Status != "host_stopped" || status.CanStart || status.CanRestart {
		t.Fatalf("status=%+v, want host_stopped with no actions", status)
	}
}

func TestControlContainerVNCUsesStartAction(t *testing.T) {
	orig := runContainerVNCCommand
	var gotArgs []string
	runContainerVNCCommand = func(_ context.Context, containerName string, args ...string) ([]byte, error) {
		if containerName != "cloudproxy-h-1" {
			t.Fatalf("containerName=%q, want cloudproxy-h-1", containerName)
		}
		gotArgs = append([]string(nil), args...)
		return []byte("VNC started"), nil
	}
	t.Cleanup(func() { runContainerVNCCommand = orig })

	if err := controlContainerVNC(context.Background(), "cloudproxy-h-1", "start"); err != nil {
		t.Fatalf("controlContainerVNC returned error: %v", err)
	}
	if strings.Join(gotArgs, " ") != "restart-vnc start" {
		t.Fatalf("args=%q, want restart-vnc start", strings.Join(gotArgs, " "))
	}
}

func TestAdminVNCStatusAndStartEndpoints(t *testing.T) {
	orig := runContainerVNCCommand
	runContainerVNCCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "vnc-status --json":
			return []byte(`{"status":"stopped","running":false,"can_start":true,"can_restart":false,"auto_restart_limited":false,"display":":99","websocket_port":6080}`), nil
		case "restart-vnc start":
			return []byte("VNC started"), nil
		default:
			t.Fatalf("unexpected vnc command args: %q", strings.Join(args, " "))
			return nil, nil
		}
	}
	t.Cleanup(func() { runContainerVNCCommand = orig })

	router := adminTestRouter(t, Dependencies{
		Logger: slog.Default(),
		AdminHosts: &stubHostStore{
			host: repository.Host{ID: "h-1", Status: "running"},
		},
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/hosts/h-1/vnc/status", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("status code=%d, want 200", resp.StatusCode)
	}
	var body VNCServiceStatus
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Status != "stopped" || !body.CanStart || body.CanRestart {
		t.Fatalf("status body=%+v, want stopped can_start only", body)
	}

	req, _ = nethttp.NewRequest("POST", srv.URL+"/v1/admin/hosts/h-1/vnc/start", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err = nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("start status code=%d, want 200", resp.StatusCode)
	}
	var startBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		t.Fatalf("decode start body: %v", err)
	}
	if startBody["status"] != "started" {
		t.Fatalf("start response=%v, want status=started", startBody)
	}
}

func TestAdminVNCStatusHostNotFound(t *testing.T) {
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminHosts:    &stubHostStore{hostErr: sql.ErrNoRows},
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/hosts/missing/vnc/status", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

func TestUserVNCStatusRespectsOwnership(t *testing.T) {
	router := adminTestRouter(t, Dependencies{
		Logger: slog.Default(),
		UserHosts: &stubHostStore{
			host: repository.Host{ID: "h-1", UserID: "owner", Status: "running"},
		},
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/user/hosts/h-1/vnc/status", nil)
	req.Header.Set("Authorization", "Bearer "+validUserToken(t, "other"))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("status=%d, want 403", resp.StatusCode)
	}
}

func TestVNCScriptsDeclareWatcherLimitAndSafeChromiumWindow(t *testing.T) {
	entrypoint := readRepoFile(t, "deploy/docker/managed-user/entrypoint.sh")
	if !strings.Contains(entrypoint, "VNC_AUTOSTART_WINDOW_SECONDS=30") {
		t.Fatal("entrypoint must define a 30s VNC auto-start window")
	}
	if !strings.Contains(entrypoint, "VNC_AUTOSTART_MAX_ATTEMPTS=3") {
		t.Fatal("entrypoint must cap VNC auto-start attempts at 3 per window")
	}
	if !strings.Contains(entrypoint, "start_vnc_watchdog_async") {
		t.Fatal("entrypoint must start a VNC watchdog")
	}
	if !strings.Contains(entrypoint, "VNC_AUTO_START=1 /usr/local/bin/restart-vnc start") {
		t.Fatal("watchdog auto-start must be marked so it does not reset the retry budget")
	}

	statusScript := readRepoFile(t, "deploy/docker/managed-user/vnc-status.sh")
	if !strings.Contains(statusScript, "auto_restart_limited") {
		t.Fatal("vnc-status must expose auto_restart_limited")
	}

	restartScript := readRepoFile(t, "deploy/docker/managed-user/restart-vnc.sh")
	if !strings.Contains(restartScript, `if [ "${VNC_AUTO_START:-0}" != "1" ]`) {
		t.Fatal("restart-vnc must preserve the retry budget for watchdog auto-starts")
	}

	chromium := readRepoFile(t, "deploy/docker/managed-user/launch-chromium.sh")
	if strings.Contains(chromium, "--window-size=1920,1080") {
		t.Fatal("Chromium default window must not equal full desktop size")
	}
	if !strings.Contains(chromium, `CHROMIUM_WINDOW_SIZE="${CHROMIUM_WINDOW_SIZE:-1880,1000}"`) {
		t.Fatal("Chromium window size must default to 1880x1000 and remain env-overridable")
	}
	if !strings.Contains(chromium, `--window-size="${CHROMIUM_WINDOW_SIZE}"`) {
		t.Fatal("Chromium launch must use CHROMIUM_WINDOW_SIZE")
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	parts := append([]string{"..", "..", ".."}, strings.Split(rel, "/")...)
	body, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func validUserToken(t *testing.T, userID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "cloud-cli-proxy",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: userID,
		Role:   "user",
	})
	s, err := token.SignedString(testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *stubHostStore) ListHostsWithEgressByUserID(context.Context, string) ([]repository.UserHostSummary, error) {
	return nil, nil
}
