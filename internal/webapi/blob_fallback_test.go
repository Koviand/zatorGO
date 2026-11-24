package webapi

import (
	"context"
	"testing"

	"github.com/AloofLibra/zator/internal/orchestra"
)

func TestTryBlobFallbackClassicClearsMode(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.flags.set(BypassFlags{BlobFallback: true, AutoRelockFails: 3})
	_ = e.srv.store.SetOverride(orchestra.OverrideMode, "1", orchestra.ModeClone)
	_ = e.srv.store.SetOverride(orchestra.OverrideBlob, "1", "old_blob")

	// checker always ok → первый candidate (classic) «успешен»
	ok := e.srv.tryBlobFallback(context.Background(), 1)
	if !ok {
		t.Fatal("expected success")
	}
	mode, err := e.srv.store.GetFakeMode("1")
	if err != nil || mode != orchestra.ModeClassic {
		t.Fatalf("mode=%q err=%v want classic (cleared override)", mode, err)
	}
	// classic не должен лежать как строка в mode_override
	if v, present, _ := e.srv.store.GetOverride(orchestra.OverrideMode, "1"); present {
		t.Fatalf("classic must ClearOverride, got present value %q", v)
	}
	blob, present, _ := e.srv.store.GetOverride(orchestra.OverrideBlob, "1")
	if !present || blob != "fake_default_tls" {
		t.Fatalf("blob=%q present=%v", blob, present)
	}
}

func TestTryBlobFallbackOff(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.store.SetOverride(orchestra.OverrideMode, "1", orchestra.ModeClone)
	if e.srv.tryBlobFallback(context.Background(), 1) {
		t.Fatal("disabled flag must no-op")
	}
	mode, _ := e.srv.store.GetFakeMode("1")
	if mode != orchestra.ModeClone {
		t.Fatalf("mode changed while disabled: %s", mode)
	}
}

func TestTryBlobFallbackRestoresOnFail(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.flags.set(BypassFlags{BlobFallback: true, AutoRelockFails: 3})
	_ = e.srv.store.SetOverride(orchestra.OverrideMode, "1", orchestra.ModeClone)
	_ = e.srv.store.SetOverride(orchestra.OverrideBlob, "1", "kept_blob")
	e.srv.checker = &failingChecker{failProfile: 1}

	if e.srv.tryBlobFallback(context.Background(), 1) {
		t.Fatal("expected fail")
	}
	mode, _ := e.srv.store.GetFakeMode("1")
	if mode != orchestra.ModeClone {
		t.Fatalf("mode not restored: %s", mode)
	}
	blob, _, _ := e.srv.store.GetOverride(orchestra.OverrideBlob, "1")
	if blob != "kept_blob" {
		t.Fatalf("blob not restored: %s", blob)
	}
}
