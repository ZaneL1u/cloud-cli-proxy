package cloudclaude

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForMount(t *testing.T) {
	t.Run("succeeds when check passes immediately", func(t *testing.T) {
		check := func() error { return nil }

		err := waitForMount(check, 10*time.Millisecond, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		var attempts atomic.Int32
		check := func() error {
			n := attempts.Add(1)
			if n < 3 {
				return errors.New("not mounted")
			}
			return nil
		}

		err := waitForMount(check, 10*time.Millisecond, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got := attempts.Load(); got < 3 {
			t.Errorf("expected at least 3 attempts, got %d", got)
		}
	})

	t.Run("returns MountNotReadyError on timeout", func(t *testing.T) {
		check := func() error { return errors.New("not mounted") }

		err := waitForMount(check, 10*time.Millisecond, 50*time.Millisecond)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var mountErr *MountNotReadyError
		if !errors.As(err, &mountErr) {
			t.Fatalf("expected MountNotReadyError, got %T: %v", err, err)
		}
		if mountErr.MountPath != "/workspace" {
			t.Errorf("MountPath = %q, want %q", mountErr.MountPath, "/workspace")
		}
		if mountErr.Timeout != 50*time.Millisecond {
			t.Errorf("Timeout = %v, want %v", mountErr.Timeout, 50*time.Millisecond)
		}
		if mountErr.LastErr == nil {
			t.Error("expected non-nil LastErr")
		}
	})
}
