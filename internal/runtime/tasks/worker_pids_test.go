package tasks

import (
	"strings"
	"testing"
)

func TestBuildCreateArgs_PidsLimit(t *testing.T) {
	w := &Worker{}

	tests := []struct {
		name string
		set  func(*int) *int
		want string
	}{
		{name: "default", set: func(_ *int) *int { return nil }, want: "1024"},
		{name: "zero_uses_default", set: func(v *int) *int { *v = 0; return v }, want: "1024"},
		{name: "custom", set: func(v *int) *int { *v = 2048; return v }, want: "2048"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := minimalCreateHostRequest("h-" + tt.name)
			var value int
			req.PidsLimit = tt.set(&value)

			args, err := w.buildCreateArgs(req, "c1", "c1", nil)
			if err != nil {
				t.Fatalf("buildCreateArgs: %v", err)
			}

			joined := strings.Join(args, " ")
			want := "--pids-limit " + tt.want
			if !strings.Contains(joined, want) {
				t.Fatalf("missing %q in args: %v", want, args)
			}
		})
	}
}
