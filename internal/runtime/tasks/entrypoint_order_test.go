package tasks

import (
	"os"
	"strings"
	"testing"
)

func TestManagedUserEntrypointAppliesNftBeforeSingBoxInterfaceDetection(t *testing.T) {
	src, err := os.ReadFile("../../../deploy/docker/managed-user/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	content := string(src)

	sequence := []struct {
		name   string
		needle string
	}{
		{name: "prepare bypass rule sets", needle: "\nprepare_bypass_rule_sets\n"},
		{name: "apply nft default-deny", needle: "\napply_nft_or_die\n"},
		{name: "prepare sing-box runtime config", needle: "\nprepare_singbox_runtime_config_or_die\n"},
		{name: "start sing-box", needle: "\nstart_singbox_or_die\n"},
		{name: "fix sing-box routing", needle: "\nfix_singbox_routing\n"},
	}

	positions := make([]int, len(sequence))
	for i, step := range sequence {
		pos := strings.Index(content, step.needle)
		if pos < 0 {
			t.Fatalf("missing entrypoint call %q (%q)", step.name, step.needle)
		}
		positions[i] = pos
	}

	for i := 1; i < len(positions); i++ {
		if positions[i-1] >= positions[i] {
			t.Fatalf("%s must run before %s: positions=%v", sequence[i-1].name, sequence[i].name, positions)
		}
	}
}
