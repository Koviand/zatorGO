package webapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) postAutoRelock(_ *http.Request, p params) (map[string]any, error) {
	on, err := parseOnOff(p)
	if err != nil {
		return nil, err
	}
	failsN := 0
	if fails := p.Get("fails"); fails != "" {
		n, cerr := strconv.Atoi(fails)
		if cerr != nil || n < 1 || n > 10 {
			return nil, errf(http.StatusBadRequest, "Некорректное число fails: "+fails)
		}
		failsN = n
	}
	if err := s.flags.patch(func(f *BypassFlags) {
		f.AutoRelock = on
		if failsN > 0 {
			f.AutoRelockFails = failsN
		}
	}); err != nil {
		return nil, errf(http.StatusInternalServerError, "Не удалось сохранить auto_relock: "+err.Error())
	}
	cur := s.flags.get()
	return map[string]any{
		"ok": true, "auto_relock": on, "enabled": on,
		"auto_relock_fails": cur.AutoRelockFails,
	}, nil
}

func (s *Server) postSweepOnProviderChange(_ *http.Request, p params) (map[string]any, error) {
	on, err := parseOnOff(p)
	if err != nil {
		return nil, err
	}
	if err := s.flags.patch(func(f *BypassFlags) { f.SweepOnProviderChange = on }); err != nil {
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	return map[string]any{"ok": true, "sweep_on_provider_change": on, "enabled": on}, nil
}

func (s *Server) postSweepSchedule(_ *http.Request, p params) (map[string]any, error) {
	raw := strings.TrimSpace(p.Get("value"))
	if raw == "" {
		return nil, errf(http.StatusBadRequest, "Укажите value=часов (0=выкл)")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 168 {
		return nil, errf(http.StatusBadRequest, "Некорректный интервал: "+raw)
	}
	if err := s.flags.patch(func(f *BypassFlags) { f.SweepScheduleHours = n }); err != nil {
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	out := map[string]any{"ok": true, "sweep_schedule_hours": n}
	if n > 0 {
		out["warning"] = "Редкий schedule нагружает роутер; по умолчанию выключен"
	}
	return out, nil
}

func (s *Server) postBlobFallback(_ *http.Request, p params) (map[string]any, error) {
	on, err := parseOnOff(p)
	if err != nil {
		return nil, err
	}
	if err := s.flags.patch(func(f *BypassFlags) { f.BlobFallback = on }); err != nil {
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	return map[string]any{"ok": true, "blob_fallback": on, "enabled": on}, nil
}

func (s *Server) postJournalEnabled(_ *http.Request, p params) (map[string]any, error) {
	on, err := parseOnOff(p)
	if err != nil {
		return nil, err
	}
	if err := s.flags.patch(func(f *BypassFlags) { f.JournalEnabled = on }); err != nil {
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	return map[string]any{"ok": true, "journal_enabled": on, "enabled": on}, nil
}

func (s *Server) bypassFlagsGet() map[string]any {
	f := sFlags(s)
	return map[string]any{
		"auto_relock":              f.AutoRelock,
		"auto_relock_fails":        f.AutoRelockFails,
		"sweep_on_provider_change": f.SweepOnProviderChange,
		"sweep_schedule_hours":     f.SweepScheduleHours,
		"blob_fallback":            f.BlobFallback,
		"journal_enabled":          f.JournalEnabled,
	}
}
