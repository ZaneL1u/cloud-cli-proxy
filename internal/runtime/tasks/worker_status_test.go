package tasks

import (
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

func TestActionToHostStatusPrepareHostDoesNotChangeStatus(t *testing.T) {
	tests := []struct {
		name   string
		action agentapi.HostAction
		want   string
	}{
		{name: "create", action: agentapi.ActionCreateHost, want: "running"},
		{name: "start", action: agentapi.ActionStartHost, want: "running"},
		{name: "rebuild", action: agentapi.ActionRebuildHost, want: "running"},
		{name: "stop", action: agentapi.ActionStopHost, want: "stopped"},
		{name: "prepare host", action: agentapi.ActionPrepareHost, want: ""},
		{name: "reload bypass", action: agentapi.ActionReloadHostBypass, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actionToHostStatus(tt.action); got != tt.want {
				t.Fatalf("actionToHostStatus(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}
