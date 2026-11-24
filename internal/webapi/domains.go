package webapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AloofLibra/zator/internal/domains"
	"github.com/AloofLibra/zator/internal/googlevideo"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/paths"
)

const (
	listKindDomain    = "domain"
	listKindSubstring = "substring"

	autoHostlistMsg = "Автосбор списков включён: домены RKN zapret2 определяет автоматически. " +
		"Выключите автосбор в настройках, чтобы пополнять список вручную."
)

// listDef описывает один из четырёх управляемых списков.
type listDef struct {
	name, title, description, kind string
	file                           func(s *Server) string
	custom                         bool
}

var domainLists = map[string]listDef{
	"netrogat": {
		name: "netrogat", title: "Исключения (netrogat.txt)", kind: listKindDomain,
		description: "Домены, исключаемые из обработки zapret2 (--hostlist-exclude).",
		file:        func(s *Server) string { return filepath.Join(s.paths.ListsDir, "netrogat.txt") },
	},
	"custom_rkn": {
		name: "custom_rkn", title: "TCP_Custom (RKN-домены)", kind: listKindDomain, custom: true,
		description: "Свои домены RKN с персональной стратегией (TCP_Custom.txt).",
		file:        func(s *Server) string { return filepath.Join(s.paths.ExtraStratsDir, "TCP_Custom.txt") },
	},
	"substring": {
		name: "substring", title: "Подстроки RKN (TCP_RKN_domains_by_substring.txt)", kind: listKindSubstring,
		description: "Домены RKN, выбираемые по подстроке имени.",
		file: func(s *Server) string {
			return filepath.Join(s.paths.ExtraStratsDir, "TCP_RKN_domains_by_substring.txt")
		},
	},
	"netrogat_substring": {
		name: "netrogat_substring", title: "Исключения-подстроки (netrogat_substrings.txt)", kind: listKindSubstring,
		description: "Подстроки имени, исключаемые из обработки (Lua exclude_substrings).",
		file:        func(s *Server) string { return filepath.Join(s.paths.ListsDir, "netrogat_substrings.txt") },
	},
	"googlevideo": {
		name: "googlevideo", title: "автосбор Googlevideo", kind: listKindDomain,
		description: "Media CDN YouTube: ^rr\\d+---sn-[a-z0-9-]+\\.googlevideo\\.com$ (SNI + DNS). Файл lists/googlevideo-hosts.txt.",
		file:        func(s *Server) string { return s.paths.GooglevideoHostsFile() },
	},
}

func googlevideoNormalize(v string) (string, bool) {
	return googlevideo.ParseHostInput(v)
}

// listNormalize нормализует значение через domains.Manager (кроме googlevideo).
func (d listDef) normalize(v string) (string, error) {
	v = strings.TrimSpace(v)
	if d.name == "googlevideo" {
		if v == "" {
			return "", errf(http.StatusBadRequest, "Не указан домен")
		}
		n, ok := googlevideoNormalize(v)
		if !ok {
			return "", errf(http.StatusBadRequest, "Нужен media CDN host rrN---sn-….googlevideo.com: "+v)
		}
		return n, nil
	}
	kind, ok := domains.KindFromAPIList(d.name)
	if !ok {
		return "", errf(http.StatusBadRequest, "Неизвестный список")
	}
	if v == "" {
		if d.kind == listKindSubstring {
			return "", errf(http.StatusBadRequest, "Пустая подстрока")
		}
		return "", errf(http.StatusBadRequest, "Не указан домен")
	}
	n, err := domains.Normalize(kind, v)
	if err != nil {
		if d.kind == listKindSubstring {
			return "", errf(http.StatusBadRequest, "Пустая подстрока")
		}
		return "", errf(http.StatusBadRequest, "Некорректный домен: "+v)
	}
	return n, nil
}

func (d listDef) domainKind() (domains.Kind, bool) {
	return domains.KindFromAPIList(d.name)
}

func (s *Server) customMax() int {
	cfg, _ := s.readConfig()
	if n := cfg.ProfileMaxStrategyOrDefault(3); n > 0 {
		return n
	}
	return 19
}

func mapDomainsErr(err error, domainMsg string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domains.ErrInvalid):
		return errf(http.StatusBadRequest, domainMsg)
	case errors.Is(err, domains.ErrNotFound):
		return errf(http.StatusBadRequest, "Записи нет в списке")
	case errors.Is(err, domains.ErrExists):
		return errf(http.StatusConflict, err.Error())
	default:
		return errf(http.StatusInternalServerError, err.Error())
	}
}

// readListLines / listEntries — вспомогательные для settings (googlevideo-ipv4.txt).
func readListLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func listEntries(lines []string) []string {
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// --- GET ---

func (s *Server) domainsList(d listDef, scope string) (map[string]any, error) {
	if d.name == "googlevideo" {
		return s.googlevideoList(d)
	}
	kind, ok := d.domainKind()
	if !ok {
		return nil, errf(http.StatusBadRequest, "Неизвестный список")
	}
	if d.custom {
		_, _ = s.domains.RestoreFromLocks()
	}
	entries, err := s.domains.ListWithStrategiesInScope(kind, scope)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		item := map[string]any{"value": e.Value}
		if d.custom && !e.Strategy.IsAuto() {
			if n := int(e.Strategy); n >= 1 {
				item["strategy"] = n
			}
		}
		items = append(items, item)
	}
	max := 0
	if d.custom {
		max = s.customMax()
	}
	return map[string]any{
		"list": d.name, "title": d.title, "description": d.description, "kind": d.kind,
		"is_custom_rkn": d.custom, "max_strategy": max, "items": items,
	}, nil
}

func (s *Server) googlevideoList(d listDef) (map[string]any, error) {
	_, _ = s.app.SyncGooglevideoHosts()
	hosts, err := s.app.GooglevideoHosts().List()
	if err != nil {
		return nil, err
	}
	byHost := map[string]googlevideo.LocalRow{}
	for _, r := range s.app.GooglevideoCollector().Local() {
		prev, ok := byHost[r.Host]
		if !ok || r.Observed >= prev.Observed {
			byHost[r.Host] = r
		}
	}
	items := make([]map[string]any, 0, len(hosts))
	for _, h := range hosts {
		item := map[string]any{"value": h, "source": "file"}
		if r, ok := byHost[h]; ok {
			item["ip"] = r.IP
			if r.Observed > 0 {
				item["observed_at"] = r.Observed
			}
			item["source"] = "dns"
		}
		items = append(items, item)
	}
	autoLocks := s.googlevideoAutoLocks()
	seed := s.app.GooglevideoSeedStatus()
	cfg, _ := s.readConfig()
	wssizeOn := false
	ipv4Count := 0
	ipv4Fresh := false
	if cfg != nil {
		wssizeOn = cfg.GetDefault("Z2R_WSSIZE_RAMP_ENABLE", "1") == "1"
	}
	if cl := s.app.GooglevideoCentralList(); cl != nil {
		ipv4Count = cl.Count()
		ipv4Fresh = cl.Fresh()
	}
	return map[string]any{
		"list": d.name, "title": d.title, "description": d.description, "kind": d.kind,
		"is_custom_rkn": false, "max_strategy": 0, "items": items,
		"collection": s.app.GooglevideoCollectionStatus(),
		"auto_locks": autoLocks,
		"seed": map[string]any{
			"state": seed.State, "host_key": seed.HostKey, "strategy": seed.Strategy,
			"last_host": seed.LastHost, "last_strategy": seed.LastStrat,
			"last_result": seed.LastResult, "last_error": seed.LastError,
			"updated_at": seed.UpdatedAt, "pending": seed.Pending,
		},
		// BP4: единый блок качества YouTube media CDN.
		"quality": map[string]any{
			"seed_state":       seed.State,
			"auto_locks_count": len(autoLocks),
			"ipv4_count":       ipv4Count,
			"ipv4_fresh":       ipv4Fresh,
			"wssize":           wssizeOn,
			"hosts_count":      len(items),
		},
	}, nil
}

// googlevideoAutoLocks читает выученные sn→strat для профиля 2 (live + state backup).
func (s *Server) googlevideoAutoLocks() []map[string]any {
	live, _ := googlevideo.ParseAutoLockedTSV(paths.LiveAutoLockedTSV())
	backup, _ := googlevideo.ParseAutoLockedTSV(s.paths.AutoLockedTSV)
	merged := googlevideo.FilterAskey(googlevideo.MergeAutoLocked(live, backup), "2")
	out := make([]map[string]any, 0, len(merged))
	for _, l := range merged {
		src := l.Source
		if src == "" {
			src = googlevideo.SourceLearned
		}
		out = append(out, map[string]any{
			"host": l.Host, "strategy": l.Strategy, "scope": l.Scope, "askey": l.Askey,
			"source": src,
		})
	}
	return out
}

// GET|POST /cgi-bin/domains.cgi
func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request, p params) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	// BP7: explain не требует list= (domains?action=explain&domain=).
	if r.Method == http.MethodGet && strings.EqualFold(p.Get("action"), "explain") {
		out, err := s.explainDomain(p.Get("domain"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	d, ok := domainLists[p.Get("list")]
	if !ok {
		writeError(w, http.StatusBadRequest, "Неизвестный список")
		return
	}
	scope, err := scopeParam(p)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.listMu.Lock()
	held := true
	unlock := func() {
		if held {
			held = false
			s.listMu.Unlock()
		}
	}
	defer unlock()

	if r.Method == http.MethodGet {
		v, err := s.domainsList(d, scope)
		unlock()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось прочитать список: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, v)
		return
	}
	out, err := s.domainsAction(r, d, p, scope)
	var pendingCheck string
	if out != nil {
		pendingCheck, _ = out["_pending_check"].(string)
		delete(out, "_pending_check")
	}
	unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	// Сетевая проверка вне listMu — иначе GET списков блокируется на минуты.
	if pendingCheck != "" && out != nil {
		out["check"] = s.checkDomain(r, pendingCheck)
	}
	writeJSON(w, http.StatusOK, out)
}

// autoGuard запрещает ручное пополнение RKN-списков при hostlist=авто.
func (s *Server) autoGuard(d listDef) error {
	if d.name != "custom_rkn" && d.name != "substring" {
		return nil
	}
	cfg, _ := s.readConfig()
	if cfg.HostlistAuto() {
		return errf(http.StatusConflict, autoHostlistMsg)
	}
	return nil
}

func (s *Server) checkDomain(r *http.Request, domain string) netcheck.CheckResponse {
	ctx, cancel := s.checkCtx(r)
	defer cancel()
	res := s.checker.CheckTarget(ctx, domain, "https://"+domain+"/")
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{res}}
}

func (s *Server) domainsAction(r *http.Request, d listDef, p params, scope string) (map[string]any, error) {
	if d.name == "googlevideo" {
		return s.googlevideoAction(p)
	}
	kind, ok := d.domainKind()
	if !ok {
		return nil, errf(http.StatusBadRequest, "Неизвестный список")
	}
	action := p.Get("action")
	if d.custom {
		_, _ = s.domains.RestoreFromLocks()
	}

	switch action {
	case "add":
		if err := s.autoGuard(d); err != nil {
			return nil, err
		}
		value, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		norm, added, aerr := s.domains.Add(kind, value)
		if aerr != nil {
			msg := "Некорректный домен: " + p.Get("domain")
			if d.kind == listKindSubstring {
				msg = "Пустая подстрока"
			}
			return nil, mapDomainsErr(aerr, msg)
		}
		out := map[string]any{"ok": true, "duplicate": !added}
		if d.custom {
			out["_pending_check"] = norm
		}
		return out, nil

	case "remove":
		if err := s.autoGuard(d); err != nil {
			return nil, err
		}
		value, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		removed, rerr := s.domains.Remove(kind, value)
		if rerr != nil {
			if removed && d.custom {
				return map[string]any{
					"ok": true, "warning": "Домен удалён из списка, но лок стратегии не снят: " + rerr.Error(),
				}, nil
			}
			return nil, errf(http.StatusInternalServerError, "Не удалось записать список: "+rerr.Error())
		}
		return map[string]any{"ok": true}, nil

	case "rename":
		if err := s.autoGuard(d); err != nil {
			return nil, err
		}
		old, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		next, err := d.normalize(p.Get("new_domain"))
		if err != nil {
			return nil, err
		}
		if old == next {
			return map[string]any{"ok": true, "domain": old}, nil
		}
		if err := s.domains.Rename(kind, old, next); err != nil {
			if errors.Is(err, domains.ErrNotFound) {
				return nil, errf(http.StatusBadRequest, "Записи нет в списке")
			}
			if errors.Is(err, domains.ErrExists) {
				return nil, errf(http.StatusConflict, "Запись "+next+" уже есть в списке")
			}
			return nil, errf(http.StatusInternalServerError, "Не удалось перенести стратегию домена: "+err.Error())
		}
		return map[string]any{"ok": true, "domain": next}, nil

	case "import":
		if err := s.autoGuard(d); err != nil {
			return nil, err
		}
		raw := p.Raw("domain")
		if u := strings.TrimSpace(p.Get("url")); u != "" {
			body, ferr := s.fetchDomainListURL(r.Context(), u)
			if ferr != nil {
				return nil, errf(http.StatusBadGateway, "Не удалось скачать список: "+ferr.Error())
			}
			raw = body
		}
		added, dups, skipped, ierr := s.domains.Import(kind, raw)
		if ierr != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось записать список: "+ierr.Error())
		}
		return map[string]any{"ok": true, "added": added, "duplicates": dups, "skipped": skipped}, nil

	case "clear":
		if err := s.autoGuard(d); err != nil {
			return nil, err
		}
		cleared, failed, cerr := s.domains.Clear(kind)
		if cerr != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось записать список: "+cerr.Error())
		}
		out := map[string]any{"ok": true, "cleared": cleared}
		if failed > 0 {
			out["warning"] = fmt.Sprintf("Список очищен, но не снято локов: %d", failed)
		}
		return out, nil

	case "set_strategy":
		if !d.custom {
			return nil, errf(http.StatusBadRequest, "Стратегия применяется только к TCP_Custom")
		}
		domain, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		raw := p.Get("strategy")
		if !strategyRe.MatchString(raw) {
			return nil, errf(http.StatusBadRequest, "Некорректная стратегия")
		}
		n, _ := strconv.Atoi(raw)
		max := s.customMax()
		if n < 1 || n > max {
			return nil, errf(http.StatusBadRequest, fmt.Sprintf("Стратегия вне диапазона (1..%d)", max))
		}
		if err := s.domains.SetStrategyInScope(scope, domain, orchestra.Strategy(n)); err != nil {
			if errors.Is(err, domains.ErrNotFound) {
				return nil, errf(http.StatusBadRequest, "Домена нет в списке")
			}
			return nil, errf(http.StatusInternalServerError, "Не удалось сохранить стратегию: "+err.Error())
		}
		return map[string]any{"ok": true, "strategy": n, "_pending_check": domain}, nil

	case "clear_strategy":
		if !d.custom {
			return nil, errf(http.StatusBadRequest, "Стратегия применяется только к TCP_Custom")
		}
		domain, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		if err := s.domains.ClearStrategy(scope, domain); err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось сбросить стратегию: "+err.Error())
		}
		return map[string]any{"ok": true}, nil

	case "check":
		if !d.custom {
			return nil, errf(http.StatusBadRequest, "Проверка применяется только к TCP_Custom")
		}
		domain, err := d.normalize(p.Get("domain"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "_pending_check": domain}, nil
	}
	return nil, errf(http.StatusBadRequest, "Неизвестное действие: "+action)
}

func (s *Server) googlevideoAction(p params) (map[string]any, error) {
	h := s.app.GooglevideoHosts()
	switch p.Get("action") {
	case "refresh":
		n, err := s.app.SyncGooglevideoHosts()
		if err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось обновить список: "+err.Error())
		}
		return map[string]any{"ok": true, "added": n}, nil
	case "refresh_ips":
		force := p.Get("force") == "1" || strings.EqualFold(p.Get("force"), "true")
		res, err := s.app.RefreshGooglevideoIPv4(context.Background(), force)
		if err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось обновить googlevideo-ipv4: "+err.Error())
		}
		return map[string]any{
			"ok": true, "status": res.Status, "count": res.Count, "generated": res.Generated,
		}, nil
	case "refresh_check":
		// BP4: hosts sync + optional ipv4 + короткий check профиля 2.
		n, err := s.app.SyncGooglevideoHosts()
		if err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось обновить список: "+err.Error())
		}
		ips, _ := s.app.RefreshGooglevideoIPv4(context.Background(), false)
		cctx, cancel := context.WithTimeout(context.Background(), relockCheckTimeout)
		check := s.checker.CheckProfile(cctx, 2)
		cancel()
		return map[string]any{
			"ok": true, "hosts_added": n,
			"ipv4_status": ips.Status, "ipv4_count": ips.Count,
			"check": check, "check_ok": profileCheckOK(check),
		}, nil
	case "add":
		host, ok := googlevideoNormalize(p.Get("domain"))
		if !ok {
			return nil, errf(http.StatusBadRequest, "Нужен media CDN host rrN---sn-….googlevideo.com: "+p.Get("domain"))
		}
		added, aerr := h.Add(host)
		if aerr != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось записать: "+aerr.Error())
		}
		return map[string]any{"ok": true, "duplicate": !added}, nil
	case "remove":
		host, ok := googlevideoNormalize(p.Get("domain"))
		if !ok {
			return nil, errf(http.StatusBadRequest, "Нужен media CDN host rrN---sn-….googlevideo.com: "+p.Get("domain"))
		}
		if _, err := h.Remove(host); err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось удалить: "+err.Error())
		}
		return map[string]any{"ok": true}, nil
	case "clear":
		if err := h.Clear(); err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось очистить: "+err.Error())
		}
		return map[string]any{"ok": true}, nil
	case "import":
		raw := p.Get("domain")
		var lines []string
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		}) {
			lines = append(lines, strings.TrimSpace(part))
		}
		n, err := h.MergeFrom(lines)
		if err != nil {
			return nil, errf(http.StatusInternalServerError, "Не удалось импортировать: "+err.Error())
		}
		return map[string]any{"ok": true, "added": n}, nil
	}
	return nil, errf(http.StatusBadRequest, "Неизвестное действие: "+p.Get("action"))
}

