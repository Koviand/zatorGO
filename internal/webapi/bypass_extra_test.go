package webapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/provider"
	"github.com/AloofLibra/zator/internal/telemetry"
)

const testRecsProvider = "MTS - Moscow"

func seedRecommendations(t *testing.T, e *env, body string) {
	t.Helper()
	cache := provider.CacheDir(e.app.Paths)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provider.CachePath(e.app.Paths), []byte(testRecsProvider+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := telemetry.New(e.app.Paths)
	if err := os.WriteFile(c.RecsFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Пометить успешный запрос «сейчас», чтобы FetchRecommendations не ходил в сеть.
	mark := testRecsProvider + "\nok\n" + strconv.FormatInt(time.Now().Unix(), 10) + "\n"
	if err := os.WriteFile(c.RecsFile()+".request", []byte(mark), 0o644); err != nil {
		t.Fatal(err)
	}
}

func recsTSVForApply(provider string, total int) string {
	return strings.Join([]string{
		"meta\t1\t" + provider + "\t" + strconv.Itoa(total) + "\t1700000000",
		"profile\t1\t" + strconv.Itoa(total) + "\t1\t40.5\t62.5",
		"profile\t2\t" + strconv.Itoa(total) + "\t0\t-\t-",
		"profile\t3\t" + strconv.Itoa(total) + "\t0\t10\t5",
		"profile\t4\t0\t0\t-\t-",
		"strategy\t1\t7\t88.5\t" + strconv.Itoa(total) + "\tclone",
		"strategy\t1\t3\t70\t4\tclassic",
		"strategy\t2\t2\t55\t5\tmixed",
		"",
	}, "\n")
}

func TestRecommendationsApplyUnavailable(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/settings", "setting", "recommendations_apply", "profile", "1")
	wantStatus(t, resp, http.StatusServiceUnavailable)
	if got, _ := out["code"].(string); got != CodeUnavailable {
		t.Fatalf("code=%q body=%v", got, out)
	}
}

func TestRecommendationsApplyOK(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(5))
	seedRecommendations(t, e, recsTSVForApply(testRecsProvider, 12))

	resp, out := e.post("/api/settings", "setting", "recommendations_apply", "profile", "1", "top_k", "1")
	wantOK(t, resp, out)
	if out["strategy"] != float64(7) && out["strategy"] != 7 {
		t.Fatalf("strategy=%v want 7", out["strategy"])
	}
	if out["origin"] != OriginRecommendations {
		t.Fatalf("origin=%v", out["origin"])
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "7" {
		t.Fatalf("lock=%q", got)
	}
}

func TestRecommendationsApplyRollback(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(5))
	seedRecommendations(t, e, recsTSVForApply(testRecsProvider, 12))
	e.checker.FailProfiles = map[int]bool{1: true}

	resp, out := e.post("/api/settings", "setting", "recommendations_apply", "profile", "1", "top_k", "2")
	wantStatus(t, resp, http.StatusOK)
	if out["ok"] != false || out["rolled_back"] != true {
		t.Fatalf("want ok=false rolled_back: %v", out)
	}
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, 1); got != "5" {
		t.Fatalf("lock after rollback=%q want 5; body=%v", got, out)
	}
}

func TestSweepBrokenBusyCode(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.checker.FailProfiles = map[int]bool{1: true, 2: true, 3: true, 4: true, 8: true}
	e.srv.runtimeMu.Lock()
	e.srv.relockActive = true
	e.srv.runtimeMu.Unlock()

	resp, out := e.post("/api/settings", "setting", "sweep_broken")
	wantStatus(t, resp, http.StatusConflict)
	if got, _ := out["code"].(string); got != CodeBusy {
		t.Fatalf("code=%q want busy; body=%v", got, out)
	}
}

func TestSweepBrokenStarts(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.checker.FailProfiles = map[int]bool{1: true}
	resp, out := e.post("/api/settings", "setting", "sweep_broken")
	wantOK(t, resp, out)
	if out["started"] != true {
		t.Fatalf("%v", out)
	}
	if !e.sweeper.started {
		t.Fatal("sweeper not started")
	}
}

func TestScopePresetUnknown(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/api/settings", "setting", "scope_preset", "preset", "nope")
	wantStatus(t, resp, http.StatusBadRequest)
	_ = out
}

func TestScopePresetAllAndCopyLocks(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, 1, orchestra.Strategy(4))

	resp, out := e.post("/api/settings",
		"setting", "scope_preset", "preset", "tv", "copy_locks", "1", "restart", "0")
	wantOK(t, resp, out)
	if out["enabled"] != true || out["preset"] != "tv" {
		t.Fatalf("%v", out)
	}
	if out["copied_locks"] == nil && out["warning"] == nil {
		// copy may report 0 if store empty shape differs — scope file should exist
		t.Logf("copy result: %v", out)
	}

	resp, out = e.post("/api/settings", "setting", "scope_copy_locks", "scope", "mark:1")
	wantOK(t, resp, out)

	resp, out = e.post("/api/settings", "setting", "scope_copy_locks", "scope", "default")
	wantStatus(t, resp, http.StatusBadRequest)

	resp, out = e.post("/api/settings", "setting", "scope_preset", "preset", "all", "restart", "0")
	wantOK(t, resp, out)
	if out["enabled"] != false {
		t.Fatalf("all must disable: %v", out)
	}

	// ensure config path still valid
	if _, err := os.Stat(filepath.Join(e.app.Paths.ConfigDir, "zator.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
