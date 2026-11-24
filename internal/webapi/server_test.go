package webapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestStaticAndNotFound(t *testing.T) {
	e := newEnv(t, envOpts{})
	for _, name := range []string{"/", "/index.html", "/app.js", "/styles.css", "/favicon.png", "/icon.png"} {
		resp, body := e.do(http.MethodGet, name, nil, nil)
		if resp.StatusCode != http.StatusOK || len(body) == 0 {
			t.Errorf("%s: status=%d len=%d", name, resp.StatusCode, len(body))
		}
	}
	// SPA fallback для маршрутов без расширения.
	resp, body := e.do(http.MethodGet, "/settings/tls-blob", nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<div id=\"app\">") {
		t.Errorf("SPA fallback: status=%d", resp.StatusCode)
	}
	// Несуществующий файл с расширением — 404.
	if resp, _ := e.do(http.MethodGet, "/missing.js", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing.js: %d", resp.StatusCode)
	}
	// Неизвестный CGI — JSON 404.
	resp, out := e.get("/cgi-bin/nope.cgi")
	wantError(t, resp, out, http.StatusNotFound, "Не найдено")
}

func TestJSONContentTypeAndAlias(t *testing.T) {
	e := newEnv(t, envOpts{})
	for _, path := range []string{"/cgi-bin/status.cgi", "/api/status"} {
		resp, out := e.get(path)
		wantStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", path, ct)
		}
		if _, ok := out["profiles"]; !ok {
			t.Errorf("%s: нет profiles", path)
		}
	}
	// Ошибка тоже JSON.
	resp, _ := e.get("/api/set-lock")
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("405: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t, envOpts{})
	if err := e.app.SetPassword("s3cret12"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Save(); err != nil {
		t.Fatal(err)
	}
	session := e.app.Config.SessionToken

	resp, out := e.get("/api/status")
	wantError(t, resp, out, http.StatusUnauthorized, "Требуется авторизация")
	if resp.Header.Get("WWW-Authenticate") != "" {
		t.Errorf("/api/* не должен слать WWW-Authenticate: %q", resp.Header.Get("WWW-Authenticate"))
	}
	// Статика SPA открыта без cookie.
	if resp, body := e.do(http.MethodGet, "/", nil, nil); resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Errorf("static without session: %d", resp.StatusCode)
	}

	// Bearer session.
	resp, _ = e.do(http.MethodGet, "/api/status", nil, map[string]string{"Authorization": "Bearer " + session})
	wantStatus(t, resp, http.StatusOK)
	resp, _ = e.do(http.MethodGet, "/api/status", nil, map[string]string{"Authorization": "Bearer wrong"})
	wantStatus(t, resp, http.StatusUnauthorized)

	// Basic: только пароль.
	req, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/api/status", nil)
	req.SetBasicAuth("admin", "s3cret12")
	r, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	wantStatus(t, r, http.StatusOK)
	req.SetBasicAuth("admin", "nope")
	r, _ = e.ts.Client().Do(req)
	r.Body.Close()
	wantStatus(t, r, http.StatusUnauthorized)

	resp, _ = e.do(http.MethodGet, "/api/status", nil, map[string]string{"Cookie": CookieName + "=" + session})
	wantStatus(t, resp, http.StatusOK)
}

func TestNoAuthAllowsAll(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, _ := e.get("/api/status")
	wantStatus(t, resp, http.StatusOK)
}

func TestParamsGetAndPost(t *testing.T) {
	e := newEnv(t, envOpts{})
	// scope из строки запроса GET (percent-decoding, '+' -> пробел).
	resp, out := e.get("/cgi-bin/status.cgi?scope=mark%3A7")
	wantStatus(t, resp, http.StatusOK)
	if _, ok := out["profiles"]; !ok {
		t.Fatal("нет profiles")
	}
	resp, out = e.get("/cgi-bin/status.cgi?scope=bogus")
	wantStatus(t, resp, http.StatusBadRequest)
	if !strings.Contains(out["error"].(string), "scope") {
		t.Errorf("error = %v", out["error"])
	}
	// POST: параметры из тела.
	resp, out = e.post("/cgi-bin/set-lock.cgi", "profile", "2", "strategy", "3")
	wantOK(t, resp, out)
	// POST: query + тело (тело приоритетнее).
	resp, out = e.post("/cgi-bin/set-lock.cgi?profile=1&strategy=1", "profile", "2", "strategy", "4")
	wantOK(t, resp, out)
	if st, _ := e.srv.Store().GetLock("default", "2", "tls"); int(st) != 4 {
		t.Errorf("profile 2 lock = %v, want 4", st)
	}
}

func TestStateAggregate(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/cgi-bin/state.cgi")
	wantStatus(t, resp, http.StatusOK)
	for _, k := range []string{"status", "version", "scopes", "tls_blob", "wg_blob", "wg_state", "fallback",
		"udp_games", "wssize_ramp", "auto_mode", "hostlist", "rst_guard", "reasm", "quic443", "dns_desync",
		"ports", "provider", "backups"} {
		v, ok := out[k]
		if !ok {
			t.Errorf("нет секции %s", k)
			continue
		}
		if m, ok := v.(map[string]any); ok {
			if _, bad := m["_error"]; bad {
				t.Errorf("секция %s: %v", k, m["_error"])
			}
		}
	}
	ver := out["version"].(map[string]any)
	if ver["zator_version"] == "" {
		t.Error("пустая zator_version")
	}
	st := out["status"].(map[string]any)
	if len(st["profiles"].([]any)) != 10 {
		t.Errorf("profiles = %d", len(st["profiles"].([]any)))
	}
}

func TestNotInstalledStatusAndErrors(t *testing.T) {
	e := newEnv(t, envOpts{noZapret: true})
	// status работает по config.default / встроенному шаблону.
	resp, out := e.get("/cgi-bin/status.cgi")
	wantStatus(t, resp, http.StatusOK)
	if out["zapret2_running"] != false {
		t.Errorf("zapret2_running = %v", out["zapret2_running"])
	}
	// Запись настроек без config zapret2 — 503 в форме {"error":...}.
	resp, out = e.post("/cgi-bin/settings.cgi", "setting", "hostlist_state", "value", "1")
	wantStatus(t, resp, http.StatusServiceUnavailable)
	if !strings.Contains(out["error"].(string), "zapret2 не установлен") {
		t.Errorf("error = %v", out["error"])
	}
	// Локи пишутся и без zapret2.
	resp, out = e.post("/cgi-bin/set-lock.cgi", "profile", "1", "strategy", "2")
	wantOK(t, resp, out)
}

func TestCheck(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/cgi-bin/check.cgi")
	wantStatus(t, resp, http.StatusOK)
	if len(out["results"].([]any)) != 1 {
		t.Errorf("results = %v", out["results"])
	}
	resp, out = e.post("/cgi-bin/check.cgi", "profile", "5")
	wantStatus(t, resp, http.StatusOK)
	if out["message"] == nil || len(out["results"].([]any)) != 0 {
		t.Errorf("UDP profile: %v", out)
	}
	resp, out = e.post("/cgi-bin/check.cgi", "profile", "abc")
	wantError(t, resp, out, http.StatusBadRequest, "Некорректный профиль")
}

func TestScopes(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/cgi-bin/scopes.cgi")
	wantStatus(t, resp, http.StatusOK)
	if out["enabled"] != false || out["warning"] != "" {
		t.Errorf("scopes = %v", out)
	}
	if sc := out["scopes"].([]any); len(sc) != 1 || sc[0] != "default" {
		t.Errorf("scopes list = %v", sc)
	}
	resp, o := e.post("/cgi-bin/set-lock.cgi", "profile", "1", "strategy", "5", "scope", "mark:3")
	wantOK(t, resp, o)
	_, out = e.get("/cgi-bin/scopes.cgi")
	if sc := out["scopes"].([]any); len(sc) != 2 || sc[1] != "mark:3" {
		t.Errorf("scopes list = %v", sc)
	}
	// status для scope: слой scoped и номер стратегии.
	_, st := e.get("/cgi-bin/status.cgi?scope=mark:3")
	p1 := st["profiles"].([]any)[0].(map[string]any)
	if p1["current_lock"] != "5" || p1["lock_source"] != "scoped" {
		t.Errorf("profile 1 in scope = %v", p1)
	}
	// default остаётся auto.
	_, st = e.get("/cgi-bin/status.cgi")
	p1 = st["profiles"].([]any)[0].(map[string]any)
	if p1["current_lock"] != "auto" || p1["lock_source"] != "auto" {
		t.Errorf("profile 1 default = %v", p1)
	}
}
