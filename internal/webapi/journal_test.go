package webapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendJournalRespectsOptIn(t *testing.T) {
	e := newEnv(t, envOpts{})
	path := filepath.Join(e.srv.paths.StateDir, journalFile)

	e.srv.appendJournal(journalEntry{
		Time: time.Now().UTC(), Profile: 1, Strategy: 3, Result: "ok", Source: OriginManual,
	})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("journal must not exist when disabled, err=%v", err)
	}

	if err := e.srv.flags.set(BypassFlags{JournalEnabled: true, AutoRelockFails: 3}); err != nil {
		t.Fatal(err)
	}
	e.srv.appendJournal(journalEntry{
		Time: time.Now().UTC(), Profile: 2, Strategy: 5, Result: "ok", Source: OriginRecommendations,
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"profile":2`) || !strings.Contains(string(data), OriginRecommendations) {
		t.Fatalf("journal content: %s", data)
	}

	// выкл снова — новых строк нет
	if err := e.srv.flags.set(BypassFlags{JournalEnabled: false, AutoRelockFails: 3}); err != nil {
		t.Fatal(err)
	}
	before := len(data)
	e.srv.appendJournal(journalEntry{
		Time: time.Now().UTC(), Profile: 9, Strategy: 1, Result: "fail", Source: OriginAutoRelock,
	})
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != before {
		t.Fatalf("grew while disabled: %d → %d", before, len(after))
	}
}

func TestJournalEnabledSettingAndGet(t *testing.T) {
	e := newEnv(t, envOpts{})
	body := url.Values{"setting": {"journal_enabled"}, "value": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if !e.srv.flags.get().JournalEnabled {
		t.Fatal("flag not set")
	}

	e.srv.appendJournal(journalEntry{
		Time: time.Now().UTC(), Profile: 1, Strategy: 1, Result: "ok", Source: "test",
	})
	_, out := e.get("/api/settings?setting=strategy_journal")
	if out["enabled"] != true {
		t.Fatalf("%v", out)
	}
	entries, _ := out["entries"].([]any)
	if len(entries) < 1 {
		t.Fatalf("entries=%v", out["entries"])
	}

	body = url.Values{"setting": {"journal_enabled"}, "value": {"0"}}
	req = httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	_, out = e.get("/api/settings?setting=strategy_journal")
	if out["enabled"] != false {
		t.Fatalf("%v", out)
	}
	entries, _ = out["entries"].([]any)
	if len(entries) != 0 {
		t.Fatalf("GET must hide entries when off: %v", entries)
	}
}
