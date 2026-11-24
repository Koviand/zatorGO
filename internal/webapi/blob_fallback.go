package webapi

import (
	"context"
	"strconv"

	"github.com/AloofLibra/zator/internal/orchestra"
)

// tryBlobFallback — BP9: при fail после смены strategy попробовать 1–2 blob/mode.
// Возвращает true, только если override реально применён и check стал ok.
// При неуспехе откатывает предыдущие overrides.
func (s *Server) tryBlobFallback(ctx context.Context, profile int) bool {
	if !sFlags(s).BlobFallback || s.store == nil {
		return false
	}
	key := strconv.Itoa(profile)
	prevBlob, _, _ := s.store.GetOverride(orchestra.OverrideBlob, key)
	prevMode, _ := s.store.GetFakeMode(key)
	prevClone, _, _ := s.store.GetOverride(orchestra.OverrideCloneSize, key)

	candidates := []struct {
		blob, mode, clone string
	}{
		{blob: "fake_default_tls", mode: orchestra.ModeClassic, clone: ""},
		{blob: "fake_default_tls", mode: orchestra.ModeClone, clone: "6"},
	}

	for _, c := range candidates {
		if err := s.applyBlobCandidate(key, c.blob, c.mode, c.clone); err != nil {
			continue
		}
		_ = sleepBrief(ctx, relockSettleDefault)
		if profileCheckOK(s.checker.CheckProfile(ctx, profile)) {
			return true
		}
	}

	_ = restoreOverride(s.store, orchestra.OverrideBlob, key, prevBlob)
	_ = restoreOverride(s.store, orchestra.OverrideMode, key, prevMode)
	_ = restoreOverride(s.store, orchestra.OverrideCloneSize, key, prevClone)
	return false
}

// applyBlobCandidate пишет overrides; classic → Clear mode (как UI), не «classic» в TSV.
func (s *Server) applyBlobCandidate(key, blob, mode, clone string) error {
	if err := s.store.SetOverride(orchestra.OverrideBlob, key, blob); err != nil {
		return err
	}
	if mode == "" || mode == orchestra.ModeClassic {
		if err := s.store.ClearOverride(orchestra.OverrideMode, key); err != nil {
			return err
		}
	} else if err := s.store.SetOverride(orchestra.OverrideMode, key, mode); err != nil {
		return err
	}
	if clone != "" {
		return s.store.SetOverride(orchestra.OverrideCloneSize, key, clone)
	}
	return s.store.ClearOverride(orchestra.OverrideCloneSize, key)
}

func restoreOverride(store *orchestra.Store, kind orchestra.OverrideKind, key, prev string) error {
	if store == nil {
		return nil
	}
	if prev == "" || (kind == orchestra.OverrideMode && prev == orchestra.ModeClassic) {
		return store.ClearOverride(kind, key)
	}
	return store.SetOverride(kind, key, prev)
}
