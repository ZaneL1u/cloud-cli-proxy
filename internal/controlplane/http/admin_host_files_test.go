package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"dir1", "dir2", "subdir"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	// create a file — should be filtered out
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	f.Close()
	// create a symlink — should be filtered out
	_ = os.Symlink(filepath.Join(dir, "dir1"), filepath.Join(dir, "link"))
	return dir
}

func TestAdminHostFiles_NormalRequest(t *testing.T) {
	dir := setupTestDir(t)
	h := NewAdminHostFilesHandler(slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path="+dir, nil)
	rr := httptest.NewRecorder()
	h.List().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"dir1", "dir2", "subdir"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q, got %s", want, body)
		}
	}
	// file and symlink must NOT appear
	for _, avoid := range []string{"file.txt", "link"} {
		if strings.Contains(body, avoid) {
			t.Errorf("expected body NOT to contain %q, got %s", avoid, body)
		}
	}
}

func TestAdminHostFiles_PathTraversal(t *testing.T) {
	h := NewAdminHostFilesHandler(slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path=../etc/passwd", nil)
	rr := httptest.NewRecorder()
	h.List().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "error") {
		t.Errorf("expected error in body, got %s", rr.Body.String())
	}
}

func TestAdminHostFiles_SensitiveDir(t *testing.T) {
	h := NewAdminHostFilesHandler(slog.Default())
	for _, path := range []string{"/proc", "/sys", "/dev", "/boot", "/etc/ssh", "/root/.ssh"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path="+path, nil)
		rr := httptest.NewRecorder()
		h.List().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("path %q: expected 403, got %d: %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestAdminHostFiles_NotFound(t *testing.T) {
	h := NewAdminHostFilesHandler(slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path=/nonexistent/path/12345", nil)
	rr := httptest.NewRecorder()
	h.List().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminHostFiles_NotADirectory(t *testing.T) {
	dir := setupTestDir(t)
	h := NewAdminHostFilesHandler(slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path="+filepath.Join(dir, "file.txt"), nil)
	rr := httptest.NewRecorder()
	h.List().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminHostFiles_EmptyPath(t *testing.T) {
	h := NewAdminHostFilesHandler(slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path=", nil)
	rr := httptest.NewRecorder()
	h.List().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminHostFiles_RelativePath(t *testing.T) {
	h := NewAdminHostFilesHandler(slog.Default())
	for _, path := range []string{"foo/bar", "tmp", "./etc"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/host-files?path="+path, nil)
		rr := httptest.NewRecorder()
		h.List().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("path %q: expected 400, got %d: %s", path, rr.Code, rr.Body.String())
		}
	}
}
