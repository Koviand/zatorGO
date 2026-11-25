package webapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AloofLibra/zator/internal/orchestra"
)

func TestErrorEnvelopeHasCode(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/api/no-such-endpoint")
	wantStatus(t, resp, http.StatusNotFound)
	if out["ok"] != false {
		t.Fatalf("ok=%v want false", out["ok"])
	}
	if out["error"] == nil || out["error"] == "" {
		t.Fatalf("missing error: %v", out)
	}
	if got, _ := out["code"].(string); got != CodeNotFound {
		t.Fatalf("code=%q want %s", got, CodeNotFound)
	}
}

func TestCapabilities(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/api/capabilities")
	wantStatus(t, resp, http.StatusOK)
	if out["ok"] != true {
		t.Fatalf("%v", out)
	}
	if out["version"] == nil || out["version"] == "" {
		t.Fatalf("version missing: %v", out)
	}
	feat, _ := out["features"].(map[string]any)
	if feat == nil || feat["repair"] != true || feat["events"] != true {
		t.Fatalf("features: %v", feat)
	}
	eps, _ := out["endpoints"].([]any)
	if len(eps) < 10 {
		t.Fatalf("endpoints short: %v", eps)
	}
}

func TestStateIncludeCapabilities(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/api/state?include=capabilities,journal")
	wantStatus(t, resp, http.StatusOK)
	if out["capabilities"] == nil {
		t.Fatalf("capabilities missing: %v", out)
	}
	if out["journal"] == nil {
		t.Fatalf("journal missing: %v", out)
	}
}

func TestBatchCheck(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/api/check?profiles=1,2")
	wantStatus(t, resp, http.StatusOK)
	if out["ok"] != true {
		t.Fatalf("%v", out)
	}
	checks, _ := out["checks"].([]any)
	if len(checks) != 2 {
		t.Fatalf("checks=%v", checks)
	}
}

func TestRepairValidation(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/repair", "profile", "99")
	wantError(t, resp, out, http.StatusBadRequest, "Некорректный профиль")
	if got, _ := out["code"].(string); got != CodeValidation {
		t.Fatalf("code=%q", got)
	}
	resp, out = e.post("/api/repair", "profile", "1", "mode", "nope")
	wantStatus(t, resp, http.StatusBadRequest)
	if got, _ := out["code"].(string); got != CodeValidation {
		t.Fatalf("code=%q body=%v", got, out)
	}
}

func TestRepairRelockStarts(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/repair", "profile", "1", "mode", "relock")
	wantStatus(t, resp, http.StatusOK)
	if out["started"] != true || out["mode"] != "relock" {
		t.Fatalf("%v", out)
	}
	if out["async"] != true {
		t.Fatalf("expected async relock: %v", out)
	}
}

func TestRepairRecommendationsFallsBackToRelock(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.svc.running = true
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(3))
	resp, out := e.post("/api/repair", "profile", "1", "mode", "recommendations")
	wantStatus(t, resp, http.StatusOK)
	if out["mode"] != "relock" || out["started"] != true {
		t.Fatalf("want relock fallback, got %v", out)
	}
	if out["recommendations_fallback"] == nil || out["recommendations_fallback"] == "" {
		t.Fatalf("expected recommendations_fallback: %v", out)
	}
}

func TestRepairSweepStartsWithDefaultDomains(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/repair", "profile", "1", "mode", "sweep")
	wantStatus(t, resp, http.StatusOK)
	if out["started"] != true || out["mode"] != "sweep" {
		t.Fatalf("%v", out)
	}
	if !e.sweeper.started {
		t.Fatal("sweeper not started")
	}
	if len(e.sweeper.params.Domains) == 0 {
		t.Fatal("ожидались домены из DefaultParams (иначе ErrNoDomains)")
	}
	if len(e.sweeper.params.Workers) != 1 || e.sweeper.params.Workers[0] == "" {
		t.Fatalf("workers=%v", e.sweeper.params.Workers)
	}
}

func TestRepairSweepRejectsUDPProfile(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/repair", "profile", "5", "mode", "sweep")
	wantStatus(t, resp, http.StatusBadRequest)
	if got, _ := out["code"].(string); got != CodeValidation {
		t.Fatalf("code=%q body=%v", got, out)
	}
	if e.sweeper.started {
		t.Fatal("sweep must not start for profile 5")
	}
}

func TestEventHubRemovesWaiterOnTimeout(t *testing.T) {
	h := newEventHub()
	done := make(chan struct{})
	go func() {
		_, _ = h.wait(0, 30*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("wait hung")
	}
	h.mu.Lock()
	n := len(h.waiters)
	h.mu.Unlock()
	if n != 0 {
		t.Fatalf("waiters left after timeout: %d", n)
	}
}

func TestEventsLongPoll(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.srv.publishEvent(Event{Type: "repair", State: "starting", Profile: 1, Message: "test"})
	resp, out := e.get("/api/events?timeout=0")
	wantStatus(t, resp, http.StatusOK)
	if out["ok"] != true || out["mode"] != "long-poll" {
		t.Fatalf("%v", out)
	}
	events, _ := out["events"].([]any)
	if len(events) < 1 {
		t.Fatalf("events empty: %v", out)
	}
	seq, _ := out["seq"].(float64)
	if seq < 1 {
		t.Fatalf("seq=%v", seq)
	}

	resp, out = e.get("/api/events?timeout=0&after=" + strconv.FormatUint(uint64(seq), 10))
	wantStatus(t, resp, http.StatusOK)
	events, _ = out["events"].([]any)
	if len(events) != 0 {
		t.Fatalf("expected empty after catch-up: %v", out)
	}

	done := make(chan map[string]any, 1)
	go func() {
		_, body := e.get("/api/events?timeout=2&after=" + strconv.FormatUint(uint64(seq), 10))
		done <- body
	}()
	time.Sleep(50 * time.Millisecond)
	e.srv.publishEvent(Event{Type: "supersweep", State: "starting", Message: "wake"})
	select {
	case body := <-done:
		raw, _ := json.Marshal(body["events"])
		if !strings.Contains(string(raw), "supersweep") {
			t.Fatalf("expected supersweep event: %s", raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long-poll did not wake")
	}
}

func TestUnknownSettingCode(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/settings", "setting", "definitely_not_a_setting", "value", "1")
	wantError(t, resp, out, http.StatusBadRequest, "Неизвестная настройка")
	if got, _ := out["code"].(string); got != CodeValidation {
		t.Fatalf("code=%q want validation", got)
	}
}

func TestCSRFCode(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, raw := e.do(http.MethodPost, "/api/login", url.Values{"password": {"x"}}, map[string]string{
		"Origin": "https://evil.example",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if got, _ := out["code"].(string); got != CodeCSRF {
		t.Fatalf("code=%q want csrf body=%v", got, out)
	}
}
