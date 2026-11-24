package webapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/orchestra"
)

func TestFixDiscordTCPOkVoiceConfigured(t *testing.T) {
	e := newEnv(t, envOpts{})
	// hostlist на месте
	if err := os.MkdirAll(e.srv.paths.ExtraStratsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.srv.paths.ExtraStratsDir, "TCP_Discord.txt"), []byte("discord.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.srv.prober = &stubStrategyProber{Result: 5}
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, discordVoiceProfile, orchestra.Strategy(2))

	body := url.Values{"setting": {"fix_discord"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	got := rr.Body.String()
	if !strings.Contains(got, `"tcp":"ok"`) {
		t.Fatalf("tcp not ok: %s", got)
	}
	if !strings.Contains(got, `"udp":"configured"`) {
		t.Fatalf("udp must be configured (not fake ok): %s", got)
	}
	if !strings.Contains(got, `"udp_verified":false`) {
		t.Fatalf("missing udp_verified=false: %s", got)
	}
	if e.srv.profileLockLabel(orchestra.ScopeDefault, discordTCPProfile) != "5" {
		t.Fatalf("tcp lock = %s", e.srv.profileLockLabel(orchestra.ScopeDefault, discordTCPProfile))
	}
}

func TestFixDiscordSkipsVoiceWhenTCPFailAndNoVoiceLock(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.srv.prober = &stubStrategyProber{Result: 0}
	e.srv.checker = &failingChecker{failProfile: discordTCPProfile}
	// voice auto — не должно включаться насильно
	body := url.Values{"setting": {"fix_discord"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	got := rr.Body.String()
	if !strings.Contains(got, `"tcp":"fail"`) {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, `"udp":"skipped"`) {
		t.Fatalf("udp should be skipped: %s", got)
	}
	if !strings.Contains(got, `"result":"fail"`) {
		t.Fatalf("%s", got)
	}
}

func TestFixDiscordPartialTCPOkVoiceSkippedNoStrat(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.srv.prober = &stubStrategyProber{Result: 4}
	// voice auto, нет recs — порты включатся, но udp=skipped без стратегии
	body := url.Values{"setting": {"fix_discord"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	got := rr.Body.String()
	if !strings.Contains(got, `"tcp":"ok"`) {
		t.Fatalf("%s", got)
	}
	// без voice lock и без recs → skipped (или configured если бы была стратегия)
	if !strings.Contains(got, `"udp":"skipped"`) && !strings.Contains(got, `"result":"partial"`) {
		t.Fatalf("%s", got)
	}
}

func TestFixDiscordPreservesTCPOnProbeFail(t *testing.T) {
	e := newEnv(t, envOpts{})
	_ = e.srv.applyProfileLock(orchestra.ScopeDefault, discordTCPProfile, orchestra.Strategy(9))
	e.srv.prober = &stubStrategyProber{Result: 0}
	e.srv.checker = &failingChecker{failProfile: discordTCPProfile}
	body := url.Values{"setting": {"fix_discord"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, req)
	if got := e.srv.profileLockLabel(orchestra.ScopeDefault, discordTCPProfile); got != "9" {
		t.Fatalf("tcp lock changed to %q", got)
	}
}
