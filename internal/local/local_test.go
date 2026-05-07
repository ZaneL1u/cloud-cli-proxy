package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestGeneratePassword(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"8 chars", 8},
		{"16 chars", 16},
		{"32 chars", 32},
		{"1 char", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pw, err := GeneratePassword(tt.length)
			if err != nil {
				t.Fatalf("GeneratePassword(%d) error: %v", tt.length, err)
			}
			if len(pw) != tt.length {
				t.Errorf("GeneratePassword(%d) length = %d, want %d", tt.length, len(pw), tt.length)
			}
			for _, c := range pw {
				if !unicode.Is(unicode.Hex_Digit, c) {
					t.Errorf("GeneratePassword(%d) contains non-hex char: %c", tt.length, c)
				}
			}
		})
	}

	t.Run("zero length", func(t *testing.T) {
		_, err := GeneratePassword(0)
		if err == nil {
			t.Error("GeneratePassword(0) should return error")
		}
	})

	t.Run("negative length", func(t *testing.T) {
		_, err := GeneratePassword(-1)
		if err == nil {
			t.Error("GeneratePassword(-1) should return error")
		}
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			pw, err := GeneratePassword(16)
			if err != nil {
				t.Fatalf("iteration %d: %v", i, err)
			}
			if seen[pw] {
				t.Fatalf("duplicate password on iteration %d", i)
			}
			seen[pw] = true
		}
	})
}

func TestLocalContainerName(t *testing.T) {
	t.Run("consistent", func(t *testing.T) {
		name1 := localContainerName("/Users/test/project")
		name2 := localContainerName("/Users/test/project")
		if name1 != name2 {
			t.Errorf("same path should produce same name: %s != %s", name1, name2)
		}
	})

	t.Run("different paths", func(t *testing.T) {
		name1 := localContainerName("/Users/test/project-a")
		name2 := localContainerName("/Users/test/project-b")
		if name1 == name2 {
			t.Errorf("different paths should produce different names: %s == %s", name1, name2)
		}
	})

	t.Run("prefix", func(t *testing.T) {
		name := localContainerName("/some/path")
		if !strings.HasPrefix(name, containerPrefix) {
			t.Errorf("name %q should have prefix %q", name, containerPrefix)
		}
	})
}

func TestBuildCreateArgs(t *testing.T) {
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("mock"), nil
	}
	m := NewLocalManagerWithRunner(LocalOptions{
		ProjectDir:    "/test/project",
		Port:          0,
		MemoryLimitMB: 2048,
		CPULimit:      1.5,
	}, runner)

	args := m.buildCreateArgs("test-container", "/test/project", "secret123")
	argStr := strings.Join(args, " ")

	checks := []struct {
		name string
		want string
	}{
		{"name", "--name test-container"},
		{"hostname", "--hostname test-container"},
		{"mode", "MODE=local"},
		{"user", "CONTAINER_USER=workspace"},
		{"password", "CONTAINER_SSH_PASSWORD=secret123"},
		{"volume", "/test/project:/workspace"},
		{"memory", "--memory 2048m"},
		{"cpu", "--cpus 1.5"},
		{"shm", "--shm-size 1g"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(argStr, c.want) {
				t.Errorf("args should contain %q, got: %s", c.want, argStr)
			}
		})
	}
}

func TestBuildCreateArgsWithPort(t *testing.T) {
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("mock"), nil
	}
	m := NewLocalManagerWithRunner(LocalOptions{
		ProjectDir: "/test/project",
		Port:       2222,
	}, runner)

	args := m.buildCreateArgs("test-container", "/test/project", "pw")
	argStr := strings.Join(args, " ")

	if !strings.Contains(argStr, "-p 2222:22") {
		t.Errorf("args should contain '-p 2222:22', got: %s", argStr)
	}
}

func TestInspectSSHPort(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "inspect" {
				return []byte("49153\n"), nil
			}
			return nil, fmt.Errorf("unexpected command")
		}
		port, err := inspectSSHPort(context.Background(), runner, "test-container")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if port != "49153" {
			t.Errorf("port = %q, want %q", port, "49153")
		}
	})

	t.Run("no such container", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("Error: No such container: test")
		}
		_, err := inspectSSHPort(context.Background(), runner, "test")
		if err == nil {
			t.Error("expected error for non-existent container")
		}
	})
}

func TestContainerExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			return []byte("abc123"), nil
		}
		exists, err := containerExists(context.Background(), runner, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Error("expected container to exist")
		}
	})

	t.Run("not exists", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("Error: No such container: test")
		}
		exists, err := containerExists(context.Background(), runner, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Error("expected container to not exist")
		}
	})
}

func TestInspectContainerStatus(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			return []byte("running\n"), nil
		}
		status, err := inspectContainerStatus(context.Background(), runner, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "running" {
			t.Errorf("status = %q, want %q", status, "running")
		}
	})

	t.Run("not found", func(t *testing.T) {
		runner := func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("Error: No such container: test")
		}
		status, err := inspectContainerStatus(context.Background(), runner, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "not_found" {
			t.Errorf("status = %q, want %q", status, "not_found")
		}
	})
}

func TestDetectEgressMode(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected EgressMode
		wantErr  bool
	}{
		{"socks", `{"type":"socks","server":"1.2.3.4","server_port":1080}`, EgressModeProxy, false},
		{"http", `{"type":"http","server":"1.2.3.4","server_port":8080}`, EgressModeProxy, false},
		{"vmess", `{"type":"vmess","server":"1.2.3.4","server_port":443}`, EgressModeTun, false},
		{"vless", `{"type":"vless","server":"1.2.3.4","server_port":443}`, EgressModeTun, false},
		{"shadowsocks", `{"type":"shadowsocks","server":"1.2.3.4","server_port":8388}`, EgressModeTun, false},
		{"trojan", `{"type":"trojan","server":"1.2.3.4","server_port":443}`, EgressModeTun, false},
		{"no type", `{"server":"1.2.3.4"}`, "", true},
		{"invalid json", `not json`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := DetectEgressMode([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.expected {
				t.Errorf("mode = %q, want %q", mode, tt.expected)
			}
		})
	}
}

func TestValidateEgressConfig(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "outbound.json")
		os.WriteFile(file, []byte(`{"type":"socks","server":"1.2.3.4","server_port":1080}`), 0644)

		data, mode, err := ValidateEgressConfig(file)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) == 0 {
			t.Error("expected non-empty data")
		}
		if mode != EgressModeProxy {
			t.Errorf("mode = %q, want %q", mode, EgressModeProxy)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, _, err := ValidateEgressConfig("/nonexistent/path/outbound.json")
		if err == nil {
			t.Error("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "不存在") {
			t.Errorf("error should mention '不存在', got: %v", err)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "empty.json")
		os.WriteFile(file, []byte(""), 0644)

		_, _, err := ValidateEgressConfig(file)
		if err == nil {
			t.Error("expected error for empty file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "bad.json")
		os.WriteFile(file, []byte("not json"), 0644)

		_, _, err := ValidateEgressConfig(file)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "JSON") {
			t.Errorf("error should mention JSON, got: %v", err)
		}
	})
}

func TestEgressMountArg(t *testing.T) {
	arg := egressMountArg("/path/to/outbound.json")
	if !strings.Contains(arg, ":/etc/cloud-claude/sing-box-outbound.json:ro") {
		t.Errorf("mount arg should contain target path, got: %s", arg)
	}
}

func TestBuildCreateArgsWithEgress(t *testing.T) {
	// Test tun mode (vmess)
	dir := t.TempDir()
	file := filepath.Join(dir, "outbound.json")
	os.WriteFile(file, []byte(`{"type":"vmess","server":"1.2.3.4","server_port":443}`), 0644)

	_, mode, _ := ValidateEgressConfig(file)
	if mode != EgressModeTun {
		t.Errorf("expected tun mode, got %q", mode)
	}
}

func TestBuildCreateArgsWithProxyEgress(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "outbound.json")
	os.WriteFile(file, []byte(`{"type":"socks","server":"1.2.3.4","server_port":1080}`), 0644)

	_, mode, err := ValidateEgressConfig(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != EgressModeProxy {
		t.Errorf("expected proxy mode, got %q", mode)
	}
}
