package webapi

import (
	"net/http"
	"runtime"

	"github.com/AloofLibra/zator"
)

// GET /api/capabilities — флаги фич и версия для UI/скриптов (RM0a).
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request, _ params) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	writeJSON(w, http.StatusOK, s.capabilitiesPayload())
}

func (s *Server) capabilitiesPayload() map[string]any {
	flags := sFlags(s)
	authReq := false
	if s.app != nil {
		authReq = s.app.AuthRequiredLocked()
	}
	return map[string]any{
		"ok":            true,
		"version":       zator.Version(),
		"goos":          runtime.GOOS,
		"goarch":        runtime.GOARCH,
		"auth_required": authReq,
		"features": map[string]any{
			"auto_relock":              true,
			"auto_relock_enabled":      flags.AutoRelock,
			"auto_relock_fails":        flags.AutoRelockFails,
			"recommendations_apply":    true,
			"strategies_feed":          true,
			"journal":                  true,
			"journal_enabled":          flags.JournalEnabled,
			"blob_fallback":            true,
			"blob_fallback_enabled":    flags.BlobFallback,
			"sweep_on_provider_change": flags.SweepOnProviderChange,
			"sweep_schedule_hours":     flags.SweepScheduleHours,
			"media_check":              true,
			"repair":                   true,
			"events":                   true,
			"events_mode":              "long-poll",
			"batch_check":              true,
			"capabilities":             true,
		},
		"endpoints": []string{
			"status", "state", "capabilities", "set-lock", "clear-lock",
			"service", "check", "repair", "events", "domains", "backups",
			"scopes", "settings", "blob-upload", "login", "logout",
		},
	}
}
