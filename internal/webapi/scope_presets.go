package webapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/orchestra"
)

// Пресеты client-scope (BP8).
var scopePresets = map[string]struct {
	Label string
	Mark  int
	Hint  string
}{
	"all":    {Label: "Все устройства", Mark: 0, Hint: "Выключить client scope — единые локи"},
	"tv":     {Label: "Отдельно TV", Mark: 1, Hint: "mark:1 — приставка / Smart TV"},
	"guest":  {Label: "Гостевая сеть", Mark: 2, Hint: "mark:2 — гостевой Wi‑Fi / VLAN"},
}

// postScopePreset — включить пресет без ручного hex (если платформа позволяет).
func (s *Server) postScopePreset(r *http.Request, p params) (map[string]any, error) {
	name := strings.ToLower(strings.TrimSpace(p.Get("preset")))
	pr, ok := scopePresets[name]
	if !ok {
		return nil, errf(http.StatusBadRequest, "Неизвестный пресет: "+p.Get("preset"))
	}
	cfg, installed := s.readConfig()
	if !installed {
		return nil, s.notInstalledErr()
	}
	_ = cfg

	if name == "all" {
		err := s.editConfigs(func(c *config.Config) error {
			return c.Set("CLIENT_SCOPE_ENABLE", "0")
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"ok": true, "preset": name, "enabled": false,
			"hint": pr.Hint, "restarted": s.maybeRestart(r.Context(), p),
		}, nil
	}

	mask := p.Get("client_scope_mark_mask")
	if mask == "" {
		mask = "0xff00"
	}
	shift := p.Get("client_scope_mark_shift")
	if shift == "" {
		shift = "8"
	}
	max := p.Get("client_scope_mark_max")
	if max == "" {
		max = "7"
	}

	err := s.editConfigs(func(c *config.Config) error {
		if e := c.Set("CLIENT_SCOPE_ENABLE", "1"); e != nil {
			return e
		}
		if e := c.Set("CLIENT_SCOPE_MARK_MASK", mask); e != nil {
			return e
		}
		if e := c.Set("CLIENT_SCOPE_MARK_SHIFT", shift); e != nil {
			return e
		}
		return c.Set("CLIENT_SCOPE_MARK_MAX", max)
	})
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"ok": true, "preset": name, "enabled": true,
		"scope": "mark:" + strconv.Itoa(pr.Mark),
		"mark":  pr.Mark, "hint": pr.Hint,
		"mark_mask": mask, "mark_shift": shift, "mark_max": max,
		"restarted": s.maybeRestart(r.Context(), p),
	}

	if p.Get("copy_locks") == "1" {
		copied, cerr := s.copyLocksDefaultToScope("mark:" + strconv.Itoa(pr.Mark))
		if cerr != nil {
			out["warning"] = "Пресет включён, копирование локов: " + cerr.Error()
		} else {
			out["copied_locks"] = copied
		}
	}
	return out, nil
}

// postScopeCopyLocks — копирование локов default → scope (обратимо через clear).
func (s *Server) postScopeCopyLocks(_ *http.Request, p params) (map[string]any, error) {
	scope, err := scopeParam(p)
	if err != nil {
		return nil, err
	}
	if scope == orchestra.ScopeDefault {
		return nil, errf(http.StatusBadRequest, "Укажите целевой scope (mark:N)")
	}
	if p.Get("undo") == "1" {
		n, err := s.clearScopeLocks(scope)
		if err != nil {
			return nil, errf(http.StatusInternalServerError, err.Error())
		}
		return map[string]any{"ok": true, "undone": true, "cleared": n, "scope": scope}, nil
	}
	n, err := s.copyLocksDefaultToScope(scope)
	if err != nil {
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	return map[string]any{"ok": true, "copied": n, "scope": scope}, nil
}

func (s *Server) copyLocksDefaultToScope(scope string) (int, error) {
	locks, err := s.store.ListLocks(orchestra.ScopeDefault)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, l := range locks {
		if err := s.store.SetLock(scope, l.Profile, string(l.Proto), l.Strategy); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Server) clearScopeLocks(scope string) (int, error) {
	locks, err := s.store.ListLocks(scope)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, l := range locks {
		if err := s.store.ClearLock(scope, l.Profile, string(l.Proto)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
