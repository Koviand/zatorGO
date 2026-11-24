package webapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/deploy"
)

// postStrategiesFeed — BP12: скачать подписанный каталог и зарегистрировать с dedupe.
// Единый путь с CLI (`zator update -strategies`): verify → state → EnsureProfileStrategy → restart.
func (s *Server) postStrategiesFeed(r *http.Request, p params) (map[string]any, error) {
	action := strings.ToLower(strings.TrimSpace(p.Get("value")))
	if action == "" {
		action = "update"
	}
	if action != "update" && action != "apply" {
		return nil, errf(http.StatusBadRequest, "Некорректное действие: "+action)
	}
	u := deploy.New(s.paths)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, err := u.ApplyStrategiesFeed(ctx)
	if err != nil {
		msg := "Каталог стратегий: " + err.Error()
		switch {
		case errors.Is(err, deploy.ErrBadSignature):
			return nil, errCode(http.StatusForbidden, CodeForbidden, msg)
		case errors.Is(err, deploy.ErrOffline):
			return nil, errCode(http.StatusServiceUnavailable, CodeUnavailable, msg)
		case strings.Contains(err.Error(), "нет "+deploy.StrategiesFeedName) ||
			strings.Contains(err.Error(), "нет "+deploy.StrategiesFeedSigName) ||
			strings.Contains(err.Error(), "нет strategies-extra"):
			return nil, errCode(http.StatusNotFound, CodeNotFound, msg)
		default:
			return nil, errCode(http.StatusBadGateway, CodeUnavailable, msg)
		}
	}

	var applied config.StrategiesFeedApply
	if err := s.editConfigs(func(c *config.Config) error {
		applied = c.ApplyStrategiesFeedLines(res.Strategies)
		return nil
	}); err != nil {
		return nil, errf(http.StatusInternalServerError, "Регистрация каталога: "+err.Error())
	}

	out := map[string]any{
		"ok":         true,
		"applied":    res.Applied,
		"registered": applied.Registered,
		"skipped":    applied.Skipped,
		"tag":        res.Tag,
		"path":       res.Path,
		"strategies": res.Strategies,
	}
	if applied.Registered > 0 {
		out["restarted"] = s.maybeRestart(r.Context(), p)
		out["restart_required"] = true
	} else {
		out["restarted"] = false
	}
	if len(applied.Warnings) > 0 {
		out["warning"] = strings.Join(applied.Warnings, "; ")
	}
	return out, nil
}
