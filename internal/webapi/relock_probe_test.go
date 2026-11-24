package webapi

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
)

func TestTryRelockUsesProbeThenAppliesLive(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))

	stub := &stubStrategyProber{Result: 7}
	calls := &atomic.Int32{}
	e.srv.prober = &countingProber{inner: stub, n: calls}
	// Мониторинг fail → probe → live verify ok.
	e.srv.checker = &seqChecker{
		failN:       0, // CheckProfile после apply — ok
		failProfile: 1,
	}

	e.srv.relock.tryRelock(context.Background(), 1)

	if calls.Load() != 1 || stub.Calls != 1 {
		t.Fatalf("probe calls=%d stub=%d", calls.Load(), stub.Calls)
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "7" {
		t.Fatalf("live lock = %q, want 7", got)
	}
	if e.srv.lockOrigin.Get(1) != OriginAutoRelock {
		t.Fatalf("origin = %q", e.srv.lockOrigin.Get(1))
	}
}

func TestTryRelockKeepsLiveLockOnProbeFail(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	e.srv.prober = &stubStrategyProber{Result: 0}
	e.srv.relock.tryRelock(context.Background(), 1)
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "3" {
		t.Fatalf("lock = %q, want 3 preserved", got)
	}
}

func TestTryRelockRollsBackOnLiveVerifyFail(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	e.srv.prober = &stubStrategyProber{Result: 7}
	e.srv.checker = &failingChecker{failProfile: 1} // live verify всегда fail
	e.srv.relock.tryRelock(context.Background(), 1)
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "3" {
		t.Fatalf("lock = %q after rollback, want 3", got)
	}
}

func TestTryRelockBlockedByAutoMode(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	if err := e.srv.editConfigs(func(c *config.Config) error {
		return c.SetAutoMode(true)
	}); err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	e.srv.prober = &countingProber{inner: &stubStrategyProber{Result: 9}, n: calls}
	e.srv.relock.tryRelock(context.Background(), 1)
	if calls.Load() != 0 {
		t.Fatal("probe must not run under auto_mode for profile 1–4")
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "3" {
		t.Fatalf("lock = %q", got)
	}
	_, _, warn, _ := e.srv.relock.snapshot()
	if warn == "" {
		t.Fatal("expected auto-conflict warning")
	}
}

func TestBeginRelockBlocksSoftSweep(t *testing.T) {
	e := newEnv(t, envOpts{})
	if !e.srv.beginRelock() {
		t.Fatal("beginRelock")
	}
	defer e.srv.endRelock()
	if err := e.srv.queueSoftSweep(context.Background(), []string{"yt"}, "test"); err == nil {
		t.Fatal("expected busy while relock active")
	}
}

func TestTryRelockSkipsWhenSweepBusy(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(2))
	e.sweeper.started = true
	e.srv.sweeper = e.sweeper
	calls := &atomic.Int32{}
	e.srv.prober = &countingProber{inner: &stubStrategyProber{Result: 5}, n: calls}
	e.srv.relock.tryRelock(context.Background(), 1)
	if calls.Load() != 0 {
		t.Fatal("probe must not run while supersweep running")
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "2" {
		t.Fatalf("lock = %q", got)
	}
}

func TestTickTriggersProbeRelock(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.flags.set(BypassFlags{AutoRelock: true, AutoRelockFails: 2})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	stub := &stubStrategyProber{Result: 8}
	e.srv.prober = stub
	e.srv.checker = &failingThenOK{failLeft: 2, failProfile: 1}

	ctx := context.Background()
	e.srv.relock.tick(ctx) // fail 1
	e.srv.relock.tick(ctx) // fail 2 → tryRelock
	if stub.Calls != 1 {
		t.Fatalf("probe calls = %d, want 1", stub.Calls)
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "8" {
		t.Fatalf("lock = %q, want 8", got)
	}
}

type countingProber struct {
	inner StrategyProber
	n     *atomic.Int32
}

func (c *countingProber) Probe(ctx context.Context, profile int, candidates []int) (int, bool, error) {
	c.n.Add(1)
	return c.inner.Probe(ctx, profile, candidates)
}

// seqChecker — всегда ok (для live verify после apply).
type seqChecker struct {
	failN       int
	failProfile int
	fakeChecker
}

func (c *seqChecker) CheckProfile(ctx context.Context, id int) netcheck.CheckResponse {
	return c.CheckProfileOpts(ctx, id, netcheck.ProfileCheckOptions{})
}

func (c *seqChecker) CheckProfileOpts(_ context.Context, id int, _ netcheck.ProfileCheckOptions) netcheck.CheckResponse {
	if id == c.failProfile && c.failN > 0 {
		c.failN--
		return netcheck.CheckResponse{Results: []netcheck.TargetResult{{
			Label: "x", Target: "https://x/", Verdict: netcheck.VerdictFail, Text: "fail",
		}}}
	}
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{okResult("profile", "https://x/")}}
}

// failingThenOK — первые failLeft CheckProfile(failProfile) fail, затем ok.
type failingThenOK struct {
	failLeft    int
	failProfile int
	fakeChecker
}

func (c *failingThenOK) CheckProfile(ctx context.Context, id int) netcheck.CheckResponse {
	return c.CheckProfileOpts(ctx, id, netcheck.ProfileCheckOptions{})
}

func (c *failingThenOK) CheckProfileOpts(_ context.Context, id int, _ netcheck.ProfileCheckOptions) netcheck.CheckResponse {
	if id == c.failProfile && c.failLeft > 0 {
		c.failLeft--
		return netcheck.CheckResponse{Results: []netcheck.TargetResult{{
			Label: "x", Target: "https://x/", Verdict: netcheck.VerdictFail, Text: "fail",
		}}}
	}
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{okResult("profile", "https://x/")}}
}
