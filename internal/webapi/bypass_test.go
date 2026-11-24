package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/domains"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
)

func TestBypassFlagsDefaultOff(t *testing.T) {
	e := newEnv(t, envOpts{})
	f := e.srv.flags.get()
	if f.AutoRelock || f.SweepOnProviderChange || f.BlobFallback {
		t.Fatalf("defaults must be off: %+v", f)
	}
	if f.AutoRelockFails != 3 {
		t.Fatalf("fails default = %d", f.AutoRelockFails)
	}
}

func TestAutoRelockSettingRoundTrip(t *testing.T) {
	e := newEnv(t, envOpts{})
	body := url.Values{"setting": {"auto_relock"}, "value": {"1"}, "fails": {"2"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	f := e.srv.flags.get()
	if !f.AutoRelock || f.AutoRelockFails != 2 {
		t.Fatalf("flags = %+v", f)
	}
}

func TestLockOriginSidecar(t *testing.T) {
	e := newEnv(t, envOpts{})
	if err := e.srv.lockOrigin.Set(1, OriginRecommendations); err != nil {
		t.Fatal(err)
	}
	if got := e.srv.lockOrigin.Get(1); got != OriginRecommendations {
		t.Fatalf("got %q", got)
	}
}

func TestStatusIncludesLockOrigin(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(5))
	_ = e.srv.lockOrigin.Set(1, OriginRecommendations)

	_, st := e.get("/cgi-bin/status.cgi")
	p1 := profileOf(t, st, 1)
	if p1["current_lock"] != "5" {
		t.Fatalf("lock=%v", p1["current_lock"])
	}
	if p1["lock_origin"] != OriginRecommendations {
		t.Fatalf("lock_origin=%v want %s", p1["lock_origin"], OriginRecommendations)
	}
	if p1["lock_source"] == nil || p1["lock_source"] == "" {
		t.Fatalf("lock_source missing: %v", p1)
	}

	// auto — origin не отдаём (даже если sidecar ещё не очищен)
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.StrategyAuto)
	_, st = e.get("/cgi-bin/status.cgi")
	p1 = profileOf(t, st, 1)
	if _, ok := p1["lock_origin"]; ok {
		t.Fatalf("lock_origin must be omitted for auto: %v", p1["lock_origin"])
	}
}

func TestSetLockWritesManualOrigin(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/cgi-bin/set-lock.cgi", "profile", "1", "strategy", "4")
	wantOK(t, resp, out)
	if out["origin"] != OriginManual {
		t.Fatalf("origin=%v", out["origin"])
	}
	if e.srv.lockOrigin.Get(1) != OriginManual {
		t.Fatalf("sidecar=%q", e.srv.lockOrigin.Get(1))
	}
	resp, out = e.post("/cgi-bin/clear-lock.cgi", "profile", "1")
	wantOK(t, resp, out)
	if e.srv.lockOrigin.Get(1) != "" {
		t.Fatalf("origin not cleared: %q", e.srv.lockOrigin.Get(1))
	}
}

func TestRelockTriggersAfterFails(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.flags.set(BypassFlags{AutoRelock: true, AutoRelockFails: 2})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	e.srv.prober = &stubStrategyProber{Result: 0} // probe fail — live lock не трогаем
	e.srv.checker = &failingChecker{failProfile: 1}
	ctx := context.Background()
	e.srv.relock.tick(ctx)
	e.srv.relock.tick(ctx)
	_, deg, _, _ := e.srv.relock.snapshot()
	if !deg[1] && e.srv.relock.fails[1] < 2 {
		t.Fatalf("expected fails>=2 or degraded, fails=%v deg=%v", e.srv.relock.fails, deg)
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "3" {
		t.Fatalf("live lock must stay 3 on probe fail, got %q", got)
	}
}

func TestHealthIncludesAutoRelock(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.flags.set(BypassFlags{AutoRelock: true, AutoRelockFails: 3})
	h := e.srv.healthPayload(true)
	if h["auto_relock"] != true {
		t.Fatalf("health = %+v", h)
	}
	if _, ok := h["validator"]; !ok {
		t.Fatal("missing validator")
	}
}

func TestExplainDomainNone(t *testing.T) {
	e := newEnv(t, envOpts{})
	out, err := e.srv.explainDomain("example.invalid.test")
	if err != nil {
		t.Fatal(err)
	}
	if out["matched"] != false {
		t.Fatalf("%+v", out)
	}
	if out["hostlist_mode"] == nil || out["hostlist_mode"] == "" {
		t.Fatalf("hostlist_mode missing: %+v", out)
	}
}

func TestExplainDomainDiscordAndExclude(t *testing.T) {
	e := newEnv(t, envOpts{})
	extra := e.srv.paths.ExtraStratsDir
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "TCP_Discord.txt"), []byte("discord.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "TCP_RKN_list.txt"), []byte("blocked.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(e.srv.paths.ZapretRoot, "ipset"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.srv.paths.ZapretRoot, "ipset", "zapret-hosts-auto.txt"), []byte("auto.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := e.srv.domains.Add(domains.KindNetrogat, "skip.example")
	if err != nil {
		t.Fatal(err)
	}

	out, err := e.srv.explainDomain("discord.com")
	if err != nil {
		t.Fatal(err)
	}
	if out["matched"] != true {
		t.Fatalf("%+v", out)
	}
	if !explainHas(out, "discord") {
		t.Fatalf("no discord match: %+v", out)
	}

	out, err = e.srv.explainDomain("blocked.example")
	if err != nil || !explainHas(out, "rkn_list") {
		t.Fatalf("rkn: err=%v out=%+v", err, out)
	}
	out, err = e.srv.explainDomain("auto.example")
	if err != nil || !explainHas(out, "autohostlist") {
		t.Fatalf("auto: err=%v out=%+v", err, out)
	}
	out, err = e.srv.explainDomain("skip.example")
	if err != nil || !explainHas(out, "netrogat") {
		t.Fatalf("exclude: err=%v out=%+v", err, out)
	}
}

func explainHas(out map[string]any, list string) bool {
	switch ms := out["matches"].(type) {
	case []map[string]any:
		for _, m := range ms {
			if m["list"] == list {
				return true
			}
		}
	case []any:
		for _, item := range ms {
			m, _ := item.(map[string]any)
			if m != nil && m["list"] == list {
				return true
			}
		}
	}
	return false
}

// failingChecker — CheckProfile fail для указанного профиля.
type failingChecker struct {
	failProfile int
	fakeChecker
}

func (c *failingChecker) CheckProfile(ctx context.Context, id int) netcheck.CheckResponse {
	return c.CheckProfileOpts(ctx, id, netcheck.ProfileCheckOptions{})
}

func (c *failingChecker) CheckProfileOpts(_ context.Context, id int, _ netcheck.ProfileCheckOptions) netcheck.CheckResponse {
	if id == c.failProfile {
		return netcheck.CheckResponse{Results: []netcheck.TargetResult{{
			Label: "x", Target: "https://x/", Verdict: netcheck.VerdictFail, Text: "fail",
		}}}
	}
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{okResult("profile", "https://x/")}}
}
