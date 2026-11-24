package webapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/sweep"
)

// POST /api/repair — узкий ремонт профиля (RM0c).
// mode=auto|relock|recommendations|sweep (default auto).
func (s *Server) handleRepair(w http.ResponseWriter, r *http.Request, p params) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	id, err := strconv.Atoi(p.Get("profile"))
	if err != nil || id < 1 || id > 10 {
		writeErrorCode(w, http.StatusBadRequest, CodeValidation, "Некорректный профиль", "")
		return
	}
	scope, err := scopeParam(p)
	if err != nil {
		writeErr(w, err)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(p.Get("mode")))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "relock", "recommendations", "sweep":
	default:
		writeErrorCode(w, http.StatusBadRequest, CodeValidation,
			"mode должен быть auto|relock|recommendations|sweep", "")
		return
	}

	cfg, installed := s.readConfig()
	if !installed {
		writeErr(w, s.notInstalledErr())
		return
	}
	if err := autoConflict(cfg, id); err != nil {
		writeErr(w, err)
		return
	}
	if err := profileModeGuard(cfg, id); err != nil {
		writeErr(w, err)
		return
	}

	prevLock := s.profileLockLabel(scope, id)
	chosen := mode
	if chosen == "auto" {
		chosen = s.pickRepairMode(id)
	}

	s.publishEvent(Event{
		Type:    "repair",
		State:   "starting",
		Profile: id,
		Message: "repair mode=" + chosen,
	})

	switch chosen {
	case "recommendations":
		out, err := s.postRecommendationsApply(r, paramsWith(p, map[string]string{
			"profile": strconv.Itoa(id),
			"scope":   scope,
			"top_k":   firstNonEmpty(p.Get("top_k"), "1"),
		}))
		if err != nil {
			s.publishEvent(Event{Type: "repair", State: "fail", Profile: id, Message: err.Error()})
			writeErr(w, err)
			return
		}
		out["mode"] = chosen
		out["previous_lock"] = prevLock
		out["started"] = true
		s.publishEvent(Event{Type: "repair", State: "done", Profile: id, Message: "recommendations applied"})
		writeJSON(w, http.StatusOK, out)
		return

	case "sweep":
		sw, err := s.sweeperOrDefault()
		if err != nil {
			writeErrorCode(w, http.StatusServiceUnavailable, CodeUnavailable,
				"Суперавтопрогон недоступен", err.Error())
			return
		}
		if sw.Running() || s.runtimeBusy() {
			writeErrorCode(w, http.StatusConflict, CodeBusy,
				"Служба занята суперавтопрогоном или auto-relock", "")
			return
		}
		worker, ok := sweep.ProfileWorker(id)
		if !ok {
			worker = sweep.WorkerRKN
		}
		err = sw.Start(r.Context(), sweep.Params{
			Scope:   scope,
			Workers: []string{worker},
		})
		if err != nil {
			s.publishEvent(Event{Type: "repair", State: "fail", Profile: id, Message: err.Error()})
			if ae := mapSweepStartErr(err); ae != nil {
				writeErr(w, ae)
				return
			}
			writeErrorCode(w, http.StatusConflict, CodeConflict, err.Error(), "")
			return
		}
		s.publishEvent(Event{Type: "repair", State: "running", Profile: id, Message: "sweep started"})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "started": true, "mode": chosen,
			"previous_lock": prevLock, "worker": worker, "scope": scope,
		})
		return

	default: // relock
		if s.relock == nil {
			s.relock = newRelockMonitor(s)
		}
		if s.runtimeBusy() {
			writeErrorCode(w, http.StatusConflict, CodeBusy,
				"Служба занята суперавтопрогоном или auto-relock", "")
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), relockProbeTimeout+2*time.Minute)
			defer cancel()
			s.relock.tryRelock(ctx, id)
			s.publishEvent(Event{Type: "repair", State: "done", Profile: id, Message: "relock finished"})
		}()
		s.publishEvent(Event{Type: "repair", State: "running", Profile: id, Message: "relock started"})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "started": true, "mode": "relock",
			"previous_lock": prevLock, "scope": scope, "profile": id,
			"async": true,
		})
	}
}

func (s *Server) pickRepairMode(profile int) string {
	if s.lockOrigin != nil {
		if o := s.lockOrigin.Get(profile); o == OriginRecommendations {
			return "recommendations"
		}
	}
	return "relock"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// paramsWith копирует params и перезаписывает ключи.
func paramsWith(p params, extra map[string]string) params {
	out := make(map[string][]string)
	for k, v := range p {
		out[k] = append([]string(nil), v...)
	}
	for k, v := range extra {
		out[k] = []string{v}
	}
	return params(out)
}

func mapSweepStartErr(err error) *apiError {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "уже") || strings.Contains(strings.ToLower(msg), "running") {
		return errCode(http.StatusConflict, CodeBusy, msg)
	}
	return errf(http.StatusBadRequest, msg)
}
