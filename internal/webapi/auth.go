package webapi

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// ClientHeader — заголовок SPA/клиента; наличие снимает зависимость от Origin
// для мутаций (и блокирует «простые» cross-site form POST без preflight).
const ClientHeader = "X-Zator-Client"

// csrfOK защищает мутирующие запросы от cross-site form POST (особенно при
// auth_enabled=false). GET/HEAD всегда ок. Разрешено, если:
//   - есть X-Zator-Client / X-Requested-With (не «простой» CORS-запрос), или
//   - Origin отсутствует (curl/same-origin без Origin), или
//   - Origin совпадает с Host запроса.
func csrfOK(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "":
		return true
	}
	if r.Header.Get(ClientHeader) != "" || r.Header.Get("X-Requested-With") != "" {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	return originMatchesHost(origin, r.Host)
}

func originMatchesHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func isPublicAuthPath(p string) bool {
	switch p {
	case "/api/login", "/cgi-bin/login.cgi":
		// logout намеренно НЕ public: без сессии только чистим cookie, без RotateSession.
		return true
	default:
		return false
	}
}

// hasAssetExt — известные статические расширения SPA.
func hasAssetExt(base string) bool {
	switch {
	case strings.HasSuffix(base, ".js"),
		strings.HasSuffix(base, ".css"),
		strings.HasSuffix(base, ".map"),
		strings.HasSuffix(base, ".svg"),
		strings.HasSuffix(base, ".png"),
		strings.HasSuffix(base, ".ico"),
		strings.HasSuffix(base, ".woff"),
		strings.HasSuffix(base, ".woff2"),
		strings.HasSuffix(base, ".html"):
		return true
	default:
		return false
	}
}

// isAPIPath — защищённые JSON-эндпоинты (не SPA shell).
func isAPIPath(p string) bool {
	return strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/cgi-bin/")
}

// isStaticAsset — index/js/css/картинки и SPA deep-link без auth (логин только во Vue).
func isStaticAsset(p string) bool {
	if p == "/" || p == "/index.html" {
		return true
	}
	if isAPIPath(p) {
		return false
	}
	base := path.Base(p)
	if hasAssetExt(base) {
		return true
	}
	// Путь без «файлового» расширения → SPA fallback на index.html.
	if !strings.Contains(base, ".") {
		return true
	}
	return false
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Подтянуть auth с диска при смене mtime (CLI `zator auth …`), не на каждый запрос.
		if s.app != nil {
			_ = s.app.ReloadAuthIfStale()
		}
		// Logout: всегда пропускаем до handler — там сами решим rotate vs clear-cookie-only.
		if r.URL.Path == "/api/logout" || r.URL.Path == "/cgi-bin/logout.cgi" {
			if !csrfOK(r) {
				writeError(w, http.StatusForbidden, "Запрос отклонён (CSRF)")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if isPublicAuthPath(r.URL.Path) || isStaticAsset(r.URL.Path) {
			if isPublicAuthPath(r.URL.Path) && !csrfOK(r) {
				writeError(w, http.StatusForbidden, "Запрос отклонён (CSRF)")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !csrfOK(r) {
			writeError(w, http.StatusForbidden, "Запрос отклонён (CSRF)")
			return
		}
		if !s.app.AuthRequiredLocked() {
			next.ServeHTTP(w, r)
			return
		}
		ok, setCookie, limited := s.authorized(r)
		if limited {
			writeError(w, http.StatusTooManyRequests, "Слишком много попыток входа. Подождите несколько минут.")
			return
		}
		if !ok {
			// Не ставим WWW-Authenticate на /api/* — иначе браузер показывает
			// нативный Basic-диалог поверх SPA и кэширует креды без rate-limit UI.
			if !isAPIPath(r.URL.Path) {
				w.Header().Set("WWW-Authenticate", `Basic realm="ZatorGO"`)
			}
			writeError(w, http.StatusUnauthorized, "Требуется авторизация")
			return
		}
		if setCookie {
			s.setSessionCookie(w)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter) {
	tok := s.app.Config.SessionToken
	if tok == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// authorized: session cookie / Bearer session / Basic с паролем.
// limited=true — парольный вход с этого IP временно закрыт и сессии нет.
// Валидные cookie/Bearer проходят даже во время блокировки IP.
func (s *Server) authorized(r *http.Request) (ok, setCookie, limited bool) {
	basicLimited := false
	if h := r.Header.Get("Authorization"); h != "" {
		if scheme, rest, found := strings.Cut(h, " "); found {
			switch strings.ToLower(scheme) {
			case "bearer":
				if s.app.SessionEqual(strings.TrimSpace(rest)) {
					return true, false, false
				}
			case "basic":
				_, pass, hasBasic := r.BasicAuth()
				if !hasBasic {
					break
				}
				ip := clientIP(r)
				if !loginBegin(ip) {
					basicLimited = true
					break
				}
				if s.app.VerifyPassword(pass) {
					loginOK(ip)
					return true, true, false
				}
				loginEndFail(ip)
			}
		}
	}
	if c, err := r.Cookie(CookieName); err == nil && s.app.SessionEqual(c.Value) {
		return true, false, false
	}
	if basicLimited {
		return false, false, true
	}
	return false, false, false
}

// --- login rate limit (in-memory, per-IP; только RemoteAddr, без XFF) ---

const (
	loginMaxFails   = 8
	loginLockWindow = 5 * time.Minute
	loginMaxEntries = 1024
)

type loginAttempt struct {
	fails       int
	pending     int
	lockedUntil time.Time
	lastSeen    time.Time
}

var (
	loginMu       sync.Mutex
	loginAttempts = map[string]*loginAttempt{}
)

// clientIP берёт только RemoteAddr. X-Forwarded-For намеренно игнорируется:
// без доверенного прокси клиент подменяет заголовок и обходит лимит.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func resetLoginAttempts() {
	loginMu.Lock()
	defer loginMu.Unlock()
	loginAttempts = map[string]*loginAttempt{}
}

func pruneLoginAttemptsLocked(now time.Time) {
	for ip, a := range loginAttempts {
		if a.pending > 0 {
			continue
		}
		if !a.lockedUntil.IsZero() {
			if now.Before(a.lockedUntil) {
				continue
			}
			delete(loginAttempts, ip)
			continue
		}
		if now.Sub(a.lastSeen) >= loginLockWindow {
			delete(loginAttempts, ip)
		}
	}
	for len(loginAttempts) > loginMaxEntries {
		var drop string
		var oldest time.Time
		first := true
		for ip, a := range loginAttempts {
			if a.pending > 0 {
				continue
			}
			if first || a.lastSeen.Before(oldest) {
				drop, oldest, first = ip, a.lastSeen, false
			}
		}
		if drop == "" {
			break
		}
		delete(loginAttempts, drop)
	}
}

// loginBegin резервирует слот попытки до bcrypt. false — IP уже заблокирован
// или исчерпал лимит с учётом параллельных запросов.
func loginBegin(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	pruneLoginAttemptsLocked(now)
	a := loginAttempts[ip]
	if a != nil && !a.lockedUntil.IsZero() && now.Before(a.lockedUntil) {
		return false
	}
	if a != nil && !a.lockedUntil.IsZero() && !now.Before(a.lockedUntil) {
		delete(loginAttempts, ip)
		a = nil
	}
	if a == nil {
		a = &loginAttempt{}
		loginAttempts[ip] = a
	}
	if a.fails+a.pending >= loginMaxFails {
		a.lockedUntil = now.Add(loginLockWindow)
		a.fails = 0
		a.pending = 0
		a.lastSeen = now
		return false
	}
	a.pending++
	a.lastSeen = now
	return true
}

func loginEndFail(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	a := loginAttempts[ip]
	if a == nil {
		a = &loginAttempt{}
		loginAttempts[ip] = a
	}
	if a.pending > 0 {
		a.pending--
	}
	a.fails++
	a.lastSeen = now
	if a.fails >= loginMaxFails {
		a.lockedUntil = now.Add(loginLockWindow)
		a.fails = 0
		a.pending = 0
	}
}

func loginOK(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, ip)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, p params) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Метод не поддерживается")
		return
	}
	if !s.app.AuthRequiredLocked() {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auth_required": false})
		return
	}
	ip := clientIP(r)
	if !loginBegin(ip) {
		writeError(w, http.StatusTooManyRequests, "Слишком много попыток входа. Подождите несколько минут.")
		return
	}
	pass := p.Get("password")
	if !s.app.VerifyPassword(pass) {
		loginEndFail(ip)
		writeError(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}
	loginOK(ip)
	// Всегда ротируем session при успешном входе (украденный токен перестаёт работать).
	if err := s.app.RotateSession(); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Save(); err != nil {
		writeErr(w, err)
		return
	}
	s.setSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auth_required": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, _ params) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Метод не поддерживается")
		return
	}
	// Без auth — только сброс cookie в браузере.
	if !s.app.AuthRequiredLocked() {
		s.clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auth_required": false})
		return
	}
	ok, _, _ := s.authorized(r)
	if ok {
		_ = s.app.RotateSession()
		_ = s.app.Save()
	}
	// Без валидной сессии: только clear cookie, session_token НЕ трогаем (anti-CSRF logout).
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
