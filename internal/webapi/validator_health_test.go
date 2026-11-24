package webapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seedValidatorScript(t *testing.T, e *env) {
	t.Helper()
	path := filepath.Join(e.srv.paths.LuaDir, "strategy-validator.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestValidatorHealthOffWithoutUnit(t *testing.T) {
	e := newEnv(t, envOpts{})
	if got := e.srv.validatorHealth(); got != validatorHealthOff {
		t.Fatalf("got %q want off (no script)", got)
	}
	seedValidatorScript(t, e)
	if got := e.srv.validatorHealth(); got != validatorHealthOff {
		t.Fatalf("got %q want off (unit down)", got)
	}
}

func TestValidatorHealthOKWhenRunningClean(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedValidatorScript(t, e)
	e.svc.running = true
	if got := e.srv.validatorHealth(); got != validatorHealthOK {
		t.Fatalf("got %q want ok", got)
	}
	h := e.srv.healthPayload(true)
	if h["validator"] != validatorHealthOK {
		t.Fatalf("health.validator=%v", h["validator"])
	}
}

func TestValidatorHealthDegradedFromRelock(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedValidatorScript(t, e)
	e.svc.running = true
	e.srv.relock.mu.Lock()
	e.srv.relock.degraded[1] = true
	e.srv.relock.mu.Unlock()

	if got := e.srv.validatorHealth(); got != validatorHealthDegraded {
		t.Fatalf("got %q want degraded", got)
	}
	h := e.srv.healthPayload(true)
	if h["validator"] != validatorHealthDegraded {
		t.Fatalf("health.validator=%v", h["validator"])
	}
	if h["overall"] != "degraded" {
		t.Fatalf("overall=%v want degraded", h["overall"])
	}
}

func TestValidatorHealthDegradedFromQueueFail(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedValidatorScript(t, e)
	e.svc.running = true

	dir := t.TempDir()
	t.Setenv("Z2R_VALIDATION_QUEUE", dir)
	fresh := filepath.Join(dir, "result.42")
	if err := os.WriteFile(fresh, []byte("42\tFAIL\ta\tb\t3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := e.srv.validatorHealth(); got != validatorHealthDegraded {
		t.Fatalf("got %q want degraded", got)
	}

	if err := os.Remove(fresh); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "result.7")
	if err := os.WriteFile(old, []byte("7\tFAIL\ta\tb\t1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * validatorFailFresh)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if got := e.srv.validatorHealth(); got != validatorHealthOK {
		t.Fatalf("stale fail: got %q want ok", got)
	}
}
