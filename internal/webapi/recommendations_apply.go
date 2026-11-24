package webapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/telemetry"
)

// postRecommendationsApply — BP2: применить top-K рекомендаций с TLS verify + rollback.
func (s *Server) postRecommendationsApply(r *http.Request, p params) (map[string]any, error) {
	id, err := strconv.Atoi(p.Get("profile"))
	if err != nil || id < 1 || id > 10 {
		return nil, errf(http.StatusBadRequest, "Некорректный профиль")
	}
	topK := 1
	if raw := p.Get("top_k"); raw != "" {
		n, cerr := strconv.Atoi(raw)
		if cerr != nil || n < 1 || n > 5 {
			return nil, errf(http.StatusBadRequest, "Некорректный top_k")
		}
		topK = n
	}
	scope, err := scopeParam(p)
	if err != nil {
		return nil, err
	}
	cfg, installed := s.readConfig()
	if !installed {
		return nil, s.notInstalledErr()
	}
	if err := autoConflict(cfg, id); err != nil {
		return nil, err
	}
	if err := profileModeGuard(cfg, id); err != nil {
		return nil, err
	}

	c := telemetry.New(s.paths)
	ctx, cancel := context.WithTimeout(r.Context(), telemetry.RecsTimeout)
	defer cancel()
	rec, ferr := c.FetchRecommendations(ctx)
	if ferr != nil || rec == nil || rec.Status == telemetry.StatusUnavailable {
		msg := "Рекомендации недоступны"
		if ferr != nil {
			msg = msg + ": " + ferr.Error()
		} else if rec != nil && rec.Status != "" {
			msg = msg + " (status=" + rec.Status + ")"
		}
		return nil, errf(http.StatusServiceUnavailable, msg)
	}
	if rec.Status != telemetry.StatusReady && rec.Status != telemetry.StatusStale {
		return nil, errf(http.StatusServiceUnavailable, "Рекомендации недоступны (status="+rec.Status+")")
	}
	tops := rec.Top(id)
	if len(tops) == 0 {
		return nil, errf(http.StatusNotFound, "Нет рекомендаций для профиля "+strconv.Itoa(id))
	}
	if topK > len(tops) {
		topK = len(tops)
	}

	prevLabel := s.profileLockLabel(scope, id)
	prevSt := parseStrategyLabel(prevLabel)

	var lastErr string
	for i := 0; i < topK; i++ {
		st := tops[i].Strategy
		if err := s.applyProfileLock(scope, id, orchestra.Strategy(st)); err != nil {
			lastErr = err.Error()
			continue
		}
		_ = sleepBrief(r.Context(), relockSettleDefault)
		cctx, ccancel := context.WithTimeout(r.Context(), relockCheckTimeout)
		ok := profileCheckOK(s.checker.CheckProfile(cctx, id))
		ccancel()
		if ok {
			_ = s.lockOrigin.Set(id, OriginRecommendations)
			s.appendJournal(journalEntry{
				Time: s.now().UTC(), Profile: id, Strategy: st,
				Result: "ok", Source: OriginRecommendations,
			})
			return map[string]any{
				"ok": true, "profile": id, "strategy": st, "previous": prevLabel,
				"lock": strconv.Itoa(st), "origin": OriginRecommendations,
				"tried": i + 1,
			}, nil
		}
		lastErr = "TLS check fail для стратегии " + strconv.Itoa(st)
	}

	_ = s.applyProfileLock(scope, id, prevSt)
	s.appendJournal(journalEntry{
		Time: s.now().UTC(), Profile: id, Strategy: int(prevSt),
		Result: "fail", Source: OriginRecommendations,
	})
	detail := lastErr
	if detail == "" {
		detail = "ни одна стратегия не прошла проверку"
	}
	return map[string]any{
		"ok": false, "rolled_back": true, "previous": prevLabel,
		"profile": id, "error": detail,
	}, nil
}
