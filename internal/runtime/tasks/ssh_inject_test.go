package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// fakeContainer 模拟容器内的文件系统，供 execInContainer 的 fake 实现使用。
// 只识别本实现实际用到的几种脚本形态，不做通用 bash 解释。
type fakeContainer struct {
	files map[string]string
	log   []string
}

var (
	reCheckNonEmpty = regexp.MustCompile(`\[ -s "\$P" \]`)
	reReadFile      = regexp.MustCompile(`\[ -f "\$P" \] && cat "\$P"`)
	reWriteCat      = regexp.MustCompile(`cat > (\S+)`)
)

func newFakeContainer() *fakeContainer {
	return &fakeContainer{files: map[string]string{}}
}

func (fc *fakeContainer) runner(_ context.Context, _ string, script, stdin string) ([]byte, error) {
	fc.log = append(fc.log, script)

	switch {
	case reCheckNonEmpty.MatchString(script):
		path := strings.TrimSpace(stdin)
		if content, ok := fc.files[path]; ok && content != "" {
			return []byte("y\n"), nil
		}
		return []byte("n\n"), nil

	case reReadFile.MatchString(script):
		path := strings.TrimSpace(stdin)
		content, ok := fc.files[path]
		if !ok {
			return nil, errors.New("exit 42")
		}
		return []byte(content), nil

	case strings.Contains(script, "cat >"):
		m := reWriteCat.FindStringSubmatch(script)
		if len(m) < 2 {
			return nil, errors.New("fake: cannot parse cat > target")
		}
		fc.files[m[1]] = stdin
		return nil, nil

	default:
		// chown/chmod 以及其它脚本视为 no-op 成功。
		return nil, nil
	}
}

// fakeWorkerRepo 最小实现 WorkerRepo 接口，只记录 RecordEvent 参数。
type fakeWorkerRepo struct {
	events []repository.RecordEventParams
}

func (r *fakeWorkerRepo) UpdateTaskStatus(_ context.Context, _, _, _, _, _ string) (repository.Task, error) {
	return repository.Task{}, nil
}

func (r *fakeWorkerRepo) UpdateHostStatus(_ context.Context, _ string, _ string) error {
	return nil
}

func (r *fakeWorkerRepo) GetEgressIPByHost(_ context.Context, _ string) (repository.EgressIP, error) {
	return repository.EgressIP{}, nil
}

func (r *fakeWorkerRepo) RecordEvent(_ context.Context, params repository.RecordEventParams) (repository.Event, error) {
	r.events = append(r.events, params)
	return repository.Event{}, nil
}

func (r *fakeWorkerRepo) UpsertClaudeAccountPersistentVolumeName(_ context.Context, _, _ string) error {
	return nil
}

func (r *fakeWorkerRepo) ReportTaskProgress(_ context.Context, _ string, _ int, _ string) error {
	return nil
}

// Phase 47 Plan 01：扩展 WorkerRepo 后必须实现 Bypass 三件套；ssh_inject_test 系列
// 走的是 SSH 注入路径，不会触发这三个方法 —— 给最小 no-op 返回即可。
func (r *fakeWorkerRepo) GetBypassSnapshotByID(_ context.Context, _ string) (repository.BypassSnapshot, error) {
	return repository.BypassSnapshot{}, nil
}

func (r *fakeWorkerRepo) UpdateBypassSnapshotStatus(_ context.Context, _ string, _ string) (repository.BypassSnapshot, error) {
	return repository.BypassSnapshot{}, nil
}

func (r *fakeWorkerRepo) GetLatestAppliedBypassSnapshot(_ context.Context, _ string) (repository.BypassSnapshot, error) {
	return repository.BypassSnapshot{}, nil
}

// setupInjectTest 装配 fake 容器、fake repo、代理公钥与 execInContainer 注入点，
// 返回 worker + 容器 + repo，供测试断言。
func setupInjectTest(t *testing.T, proxyPub string) (*Worker, *fakeContainer, *fakeWorkerRepo) {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)
	if proxyPub != "" {
		if err := os.WriteFile(filepath.Join(tmp, "ssh_host_ed25519_key.pub"), []byte(proxyPub+"\n"), 0o600); err != nil {
			t.Fatalf("write proxy pub: %v", err)
		}
	}

	fc := newFakeContainer()
	prev := execInContainer
	execInContainer = fc.runner
	t.Cleanup(func() { execInContainer = prev })

	repo := &fakeWorkerRepo{}
	w := NewWorker(repo, nil)
	return w, fc, repo
}

func hasEventWithFile(events []repository.RecordEventParams, eventType, file string) bool {
	for _, ev := range events {
		if ev.Type != eventType {
			continue
		}
		if got, ok := ev.Metadata["file"].(string); ok && got == file {
			return true
		}
	}
	return false
}

func hasEventType(events []repository.RecordEventParams, eventType string) bool {
	for _, ev := range events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func TestInjectSSHKeys(t *testing.T) {
	t.Run("empty_container_writes_outbound", func(t *testing.T) {
		w, fc, repo := setupInjectTest(t, "")

		req := agentapi.HostActionRequest{
			HostID: "h1",
			SSHKeys: []agentapi.SSHKeyEntry{
				{
					Purpose:    "outbound",
					PublicKey:  "ssh-ed25519 OUTBOUND_PUB",
					PrivateKey: "OUTBOUND_PRIV",
				},
			},
		}

		w.injectSSHKeys(context.Background(), req, "c1")

		if got := fc.files["/workspace/.ssh/id_ed25519"]; got != "OUTBOUND_PRIV" {
			t.Fatalf("id_ed25519 = %q, want %q", got, "OUTBOUND_PRIV")
		}
		if got := fc.files["/workspace/.ssh/id_ed25519.pub"]; got != "ssh-ed25519 OUTBOUND_PUB" {
			t.Fatalf("id_ed25519.pub = %q, want %q", got, "ssh-ed25519 OUTBOUND_PUB")
		}
		if hasEventType(repo.events, "runtime.ssh_key_skipped_existing") {
			t.Errorf("did not expect ssh_key_skipped_existing event when container was empty")
		}
	})

	t.Run("existing_outbound_is_preserved", func(t *testing.T) {
		w, fc, repo := setupInjectTest(t, "")
		fc.files["/workspace/.ssh/id_ed25519"] = "USER-GENERATED-PRIV"

		req := agentapi.HostActionRequest{
			HostID: "h1",
			SSHKeys: []agentapi.SSHKeyEntry{
				{
					Purpose:    "outbound",
					PublicKey:  "ssh-ed25519 CONTROLLER_PUB",
					PrivateKey: "CONTROLLER_PRIV",
				},
			},
		}

		w.injectSSHKeys(context.Background(), req, "c1")

		if got := fc.files["/workspace/.ssh/id_ed25519"]; got != "USER-GENERATED-PRIV" {
			t.Fatalf("id_ed25519 overwritten: %q, want %q", got, "USER-GENERATED-PRIV")
		}
		if !hasEventWithFile(repo.events, "runtime.ssh_key_skipped_existing", "/workspace/.ssh/id_ed25519") {
			t.Errorf("expected ssh_key_skipped_existing event for /workspace/.ssh/id_ed25519, events=%+v", repo.events)
		}
	})

	t.Run("authorized_keys_fresh_write", func(t *testing.T) {
		proxyPub := "ssh-ed25519 PROXY_PUB"
		w, fc, _ := setupInjectTest(t, proxyPub)

		inboundPub := "ssh-ed25519 INBOUND_PUB"
		req := agentapi.HostActionRequest{
			HostID: "h1",
			SSHKeys: []agentapi.SSHKeyEntry{
				{Purpose: "inbound", PublicKey: inboundPub},
			},
		}

		w.injectSSHKeys(context.Background(), req, "c1")

		content, ok := fc.files["/workspace/.ssh/authorized_keys"]
		if !ok {
			t.Fatalf("authorized_keys not written, files=%v", fc.files)
		}

		lines := strings.Split(content, "\n")
		beginIdx, endIdx := -1, -1
		for i, l := range lines {
			if l == sshManagedBeginMarker && beginIdx == -1 {
				beginIdx = i
			}
			if l == sshManagedEndMarker && beginIdx != -1 {
				endIdx = i
				break
			}
		}
		if beginIdx == -1 || endIdx == -1 || endIdx <= beginIdx {
			t.Fatalf("markers not found or out of order, content=\n%s", content)
		}

		block := lines[beginIdx+1 : endIdx]
		if !containsLine(block, proxyPub) {
			t.Errorf("marker block missing proxy pub, block=%v", block)
		}
		if !containsLine(block, inboundPub) {
			t.Errorf("marker block missing inbound pub, block=%v", block)
		}
		// proxy pub 按 Task 2 逻辑先入列，排在 inbound 之前。
		if indexOfLine(block, proxyPub) > indexOfLine(block, inboundPub) {
			t.Errorf("proxy pub should precede inbound pub, block=%v", block)
		}
	})

	t.Run("authorized_keys_preserves_user_lines", func(t *testing.T) {
		proxyPub := "ssh-ed25519 PROXY_PUB"
		w, fc, _ := setupInjectTest(t, proxyPub)

		fc.files["/workspace/.ssh/authorized_keys"] = strings.Join([]string{
			"ssh-ed25519 USERLINE1",
			sshManagedBeginMarker,
			"ssh-ed25519 OLDMANAGED",
			sshManagedEndMarker,
			"ssh-ed25519 USERLINE2",
			"",
		}, "\n")

		newManaged := "ssh-ed25519 NEWMANAGED"
		req := agentapi.HostActionRequest{
			HostID: "h1",
			SSHKeys: []agentapi.SSHKeyEntry{
				{Purpose: "inbound", PublicKey: newManaged},
			},
		}

		w.injectSSHKeys(context.Background(), req, "c1")

		content := fc.files["/workspace/.ssh/authorized_keys"]
		lines := strings.Split(content, "\n")

		if !containsLine(lines, "ssh-ed25519 USERLINE1") {
			t.Errorf("USERLINE1 lost, content=\n%s", content)
		}
		if !containsLine(lines, "ssh-ed25519 USERLINE2") {
			t.Errorf("USERLINE2 lost, content=\n%s", content)
		}

		beginIdx := indexOfLine(lines, sshManagedBeginMarker)
		endIdx := indexOfLineFrom(lines, sshManagedEndMarker, beginIdx+1)
		if beginIdx < 0 || endIdx < 0 {
			t.Fatalf("markers not found, content=\n%s", content)
		}
		block := lines[beginIdx+1 : endIdx]

		if containsLine(block, "ssh-ed25519 OLDMANAGED") {
			t.Errorf("OLDMANAGED should be removed, block=%v", block)
		}
		if !containsLine(block, proxyPub) {
			t.Errorf("marker block missing proxy pub, block=%v", block)
		}
		if !containsLine(block, newManaged) {
			t.Errorf("marker block missing NEWMANAGED, block=%v", block)
		}
	})

	t.Run("stable_on_second_call", func(t *testing.T) {
		proxyPub := "ssh-ed25519 PROXY_PUB"
		w, fc, repo := setupInjectTest(t, proxyPub)

		fc.files["/workspace/.ssh/authorized_keys"] = strings.Join([]string{
			"ssh-ed25519 USERLINE1",
			sshManagedBeginMarker,
			"ssh-ed25519 OLDMANAGED",
			sshManagedEndMarker,
			"ssh-ed25519 USERLINE2",
			"",
		}, "\n")

		req := agentapi.HostActionRequest{
			HostID: "h1",
			SSHKeys: []agentapi.SSHKeyEntry{
				{Purpose: "inbound", PublicKey: "ssh-ed25519 NEWMANAGED"},
			},
		}

		w.injectSSHKeys(context.Background(), req, "c1")
		first := fc.files["/workspace/.ssh/authorized_keys"]

		// 第二次调用同样的 request，期望字节级不变
		repo.events = nil
		w.injectSSHKeys(context.Background(), req, "c1")
		second := fc.files["/workspace/.ssh/authorized_keys"]

		if first != second {
			t.Fatalf("authorized_keys drifted on second call:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
		if hasEventType(repo.events, "runtime.ssh_authorized_keys_failed") {
			t.Errorf("unexpected authorized_keys_failed event on second call: %+v", repo.events)
		}
	})
}

func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if l == target {
			return true
		}
	}
	return false
}

func indexOfLine(lines []string, target string) int {
	for i, l := range lines {
		if l == target {
			return i
		}
	}
	return -1
}

func indexOfLineFrom(lines []string, target string, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i < len(lines); i++ {
		if lines[i] == target {
			return i
		}
	}
	return -1
}
