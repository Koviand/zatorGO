package webapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AloofLibra/zator/internal/app"
)

func TestLoginLogoutAndMiddleware(t *testing.T) {
	resetLoginAttempts()
	e := newEnv(t, envOpts{})
	e.app.Config.AuthEnabled = false
	resp, out := e.get("/api/status")
	wantStatus(t, resp, http.StatusOK)
	if out["profiles"] == nil {
		t.Fatalf("open status: %v", out)
	}

	if err := e.app.SetPassword("s3cret12"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Save(); err != nil {
		t.Fatal(err)
	}
	beforeLogout := e.app.Config.SessionToken

	resp, out = e.get("/api/status")
	wantError(t, resp, out, http.StatusUnauthorized, "Требуется авторизация")

	resp, body := e.do(http.MethodGet, "/", nil, map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("статика без auth: %d %s", resp.StatusCode, body[:min(80, len(body))])
	}

	resp, out = e.post("/api/login", "password", "wrong")
	wantError(t, resp, out, http.StatusUnauthorized, "Неверный пароль")

	form := url.Values{"password": {"s3cret12"}}
	resp, raw := e.do(http.MethodPost, "/api/login", form, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d %s", resp.StatusCode, raw)
	}
	var cookie string
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			cookie = c.Value
		}
	}
	if cookie == "" || cookie != e.app.Config.SessionToken {
		t.Fatalf("cookie=%q session=%q", cookie, e.app.Config.SessionToken)
	}

	resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Cookie": CookieName + "=" + cookie,
	})
	wantStatus(t, resp, http.StatusOK)
	if out["profiles"] == nil {
		t.Fatalf("authed status: %v", out)
	}

	resp, _ = e.do(http.MethodPost, "/api/logout", url.Values{}, map[string]string{
		"Cookie": CookieName + "=" + cookie,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Cookie": CookieName + "=" + cookie,
	})
	wantError(t, resp, out, http.StatusUnauthorized, "Требуется авторизация")

	// SPA deep-link без cookie → HTML (Vue login), не JSON 401.
	resp, body = e.do(http.MethodGet, "/settings/tls", nil, map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("SPA deep-link без auth: %d %s", resp.StatusCode, body[:min(80, len(body))])
	}

	// Logout без cookie: чистит Set-Cookie, но НЕ ротирует session_token (anti-CSRF).
	sessionBefore := e.app.Config.SessionToken
	if sessionBefore == "" || sessionBefore == beforeLogout {
		// после logout с cookie токен уже сменился — это ок
	}
	resp, raw = e.do(http.MethodPost, "/api/logout", url.Values{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout без cookie: %d %s", resp.StatusCode, raw)
	}
	if e.app.Config.SessionToken != sessionBefore {
		t.Fatalf("logout без auth не должен RotateSession: %q → %q", sessionBefore, e.app.Config.SessionToken)
	}
	cleared := false
	for _, c := range resp.Cookies() {
		if c.Name == CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		for _, h := range resp.Header.Values("Set-Cookie") {
			if strings.Contains(h, CookieName+"=") && (strings.Contains(h, "Max-Age=0") || strings.Contains(strings.ToLower(h), "max-age=-1")) {
				cleared = true
			}
		}
	}
	if !cleared {
		t.Fatalf("logout без cookie не очистил Set-Cookie: %v", resp.Header.Values("Set-Cookie"))
	}
}

func TestCSRFOriginRejected(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, raw := e.do(http.MethodPost, "/api/login", url.Values{"password": {"x"}}, map[string]string{
		"Origin": "https://evil.example",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("csrf: %d %s", resp.StatusCode, raw)
	}
	resp, raw = e.do(http.MethodPost, "/api/login", url.Values{"password": {"x"}}, map[string]string{
		"Origin":         "https://evil.example",
		"X-Zator-Client": "test",
	})
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("header should bypass origin check: %s", raw)
	}
}

func TestClientIPIgnoresXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.RemoteAddr = "203.0.113.10:54321"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := clientIP(r); got != "203.0.113.10" {
		t.Fatalf("clientIP=%q want RemoteAddr host", got)
	}
}

func TestLoginRateLimitAndBasic(t *testing.T) {
	resetLoginAttempts()
	e := newEnv(t, envOpts{})
	if err := e.app.SetPassword("s3cret12"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Save(); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < loginMaxFails; i++ {
		resp, out := e.post("/api/login", "password", "wrong")
		wantError(t, resp, out, http.StatusUnauthorized, "Неверный пароль")
	}
	const lockedMsg = "Слишком много попыток входа. Подождите несколько минут."
	resp, out := e.post("/api/login", "password", "wrong")
	wantError(t, resp, out, http.StatusTooManyRequests, lockedMsg)

	// Подмена XFF не должна открыть новый лимит.
	resp, raw := e.do(http.MethodPost, "/api/login", url.Values{"password": {"wrong"}}, map[string]string{
		"X-Forwarded-For": "198.51.100.99",
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("xff bypass: %d %s", resp.StatusCode, raw)
	}

	resetLoginAttempts()
	badBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("x:wrong-pass"))
	for i := 0; i < loginMaxFails; i++ {
		resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
			"Authorization": badBasic,
		})
		wantError(t, resp, out, http.StatusUnauthorized, "Требуется авторизация")
		if resp.Header.Get("WWW-Authenticate") != "" {
			t.Fatalf("api 401 не должен слать WWW-Authenticate: %q", resp.Header.Get("WWW-Authenticate"))
		}
	}
	resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Authorization": badBasic,
	})
	wantError(t, resp, out, http.StatusTooManyRequests, lockedMsg)

	// Валидная сессия проходит даже при blocked IP.
	if err := e.app.RotateSession(); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Save(); err != nil {
		t.Fatal(err)
	}
	tok := e.app.Config.SessionToken
	resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Cookie":        CookieName + "=" + tok,
		"Authorization": badBasic,
	})
	wantStatus(t, resp, http.StatusOK)
	if out["profiles"] == nil {
		t.Fatalf("session during lock: %v", out)
	}
}

func TestIsStaticAssetSPAPaths(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/index.html", true},
		{"/app.js", true},
		{"/styles.css", true},
		{"/settings/tls", true},
		{"/strategies", true},
		{"/api/status", false},
		{"/cgi-bin/status.cgi", false},
		{"/api/login", false}, // public, но не static
	}
	for _, tc := range cases {
		if got := isStaticAsset(tc.path); got != tc.want {
			t.Errorf("isStaticAsset(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func doJSONAuth(e *env, method, path string, form url.Values, hdr map[string]string) (*http.Response, map[string]any) {
	e.t.Helper()
	resp, body := e.do(method, path, form, hdr)
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		e.t.Fatalf("%s %s: не JSON: %v\n%s", method, path, err, body)
	}
	return resp, out
}

// RM12: CLI set-password ротирует session_token; старый cookie отвергается после mtime reload.
func TestSessionRejectedAfterCLISetPassword(t *testing.T) {
	resetLoginAttempts()
	e := newEnv(t, envOpts{})
	if err := e.app.SetPassword("s3cret12"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Save(); err != nil {
		t.Fatal(err)
	}
	oldTok := e.app.Config.SessionToken
	if oldTok == "" {
		t.Fatal("empty session")
	}
	resp, out := doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Cookie": CookieName + "=" + oldTok,
	})
	wantStatus(t, resp, http.StatusOK)
	if out["profiles"] == nil {
		t.Fatalf("%v", out)
	}

	cli := app.NewWith(e.app.Paths, e.app.Platform)
	cli.ConfigPath = e.app.ConfigPath
	if err := cli.Load(); err != nil {
		t.Fatal(err)
	}
	if err := cli.SetPassword("newpass99"); err != nil {
		t.Fatal(err)
	}
	if err := cli.Save(); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(e.app.ConfigPath, newer, newer); err != nil {
		t.Fatal(err)
	}
	// ReloadAuthIfStale троттлит проверку mtime (~1s).
	time.Sleep(1100 * time.Millisecond)

	resp, out = doJSONAuth(e, http.MethodGet, "/api/status", nil, map[string]string{
		"Cookie": CookieName + "=" + oldTok,
	})
	wantError(t, resp, out, http.StatusUnauthorized, "Требуется авторизация")
}

func TestSessionCookieSecureOnHTTPSHints(t *testing.T) {
	e := newEnv(t, envOpts{})
	if err := e.app.SetPassword("s3cret12"); err != nil {
		t.Fatal(err)
	}
	_ = e.app.Save()
	form := url.Values{"password": {"s3cret12"}}
	resp, _ := e.do(http.MethodPost, "/api/login", form, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login http: %d", resp.StatusCode)
	}
	found := false
	for _, c := range resp.Cookies() {
		if c.Name != CookieName {
			continue
		}
		found = true
		if c.Secure {
			t.Fatal("HTTP login не должен ставить Secure")
		}
		if c.MaxAge != sessionCookieMaxAge {
			t.Fatalf("MaxAge=%d want %d", c.MaxAge, sessionCookieMaxAge)
		}
	}
	if !found {
		t.Fatalf("нет session cookie: %v", resp.Header.Values("Set-Cookie"))
	}
	if got := sessionCookieMaxAge; got != 7*24*3600 {
		t.Fatalf("MaxAge const=%d", got)
	}

	resp, _ = e.do(http.MethodPost, "/api/login", form, map[string]string{
		"X-Forwarded-Proto": "https",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login xfp: %d", resp.StatusCode)
	}
	secure := false
	for _, h := range resp.Header.Values("Set-Cookie") {
		if strings.Contains(h, CookieName+"=") && strings.Contains(strings.ToLower(h), "secure") {
			secure = true
		}
	}
	if !secure {
		t.Fatalf("X-Forwarded-Proto=https должен дать Secure: %v", resp.Header.Values("Set-Cookie"))
	}
}

func TestRequestLooksHTTPS(t *testing.T) {
	if requestLooksHTTPS(nil) {
		t.Fatal("nil")
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if requestLooksHTTPS(r) {
		t.Fatal("plain http")
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	if !requestLooksHTTPS(r) {
		t.Fatal("xfp")
	}
}
