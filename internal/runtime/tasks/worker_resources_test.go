package tasks

import (
	"strings"
	"testing"
)

func TestBuildCreateArgs_ZeroResourceLimitsUseSafeDefaults(t *testing.T) {
	w := &Worker{}
	req := minimalCreateHostRequest("h-safe-defaults")
	req.MemoryLimitMB = 0
	req.CPULimit = 0
	zeroPids := 0
	req.PidsLimit = &zeroPids

	args, err := w.buildCreateArgs(req, "c-safe", "c-safe", nil)
	if err != nil {
		t.Fatalf("buildCreateArgs: %v", err)
	}
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--memory 4096m",
		"--memory-swap 4096m",
		"--cpus 2.0",
		"--pids-limit 1024",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in args: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--cpus 0") ||
		strings.Contains(joined, "--memory 0") ||
		strings.Contains(joined, "--memory-swap 0") ||
		strings.Contains(joined, "--pids-limit -1") {
		t.Fatalf("resource args must not request unlimited limits: %s", joined)
	}
}
