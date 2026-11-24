package webapi

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/telemetry"
)

const (
	discordTCPProfile   = 4
	discordVoiceProfile = 6
)

// Статусы сторон Discord repair (BP5).
// tcp: ok|fail — TLS-проверка.
// udp: configured|fail|skipped — UDP/voice нельзя TLS-проверить (netcheck пуст для 5/6);
//
//	configured = стратегия зафиксирована + порты включены (datapath готов, не «голос работает»);
//	skipped = голос не трогали (нет стратегии / TCP fail без прежнего voice-lock).
const (
	discordUDPConfigured = "configured"
	discordUDPFail       = "fail"
	discordUDPSkipped    = "skipped"
)

// postFixDiscord — BP5: согласованный ремонт Discord TCP (4) + voice (6).
func (s *Server) postFixDiscord(r *http.Request, p params) (map[string]any, error) {
	scope, err := scopeParam(p)
	if err != nil {
		return nil, err
	}
	hostlist := filepath.Join(s.paths.ExtraStratsDir, "TCP_Discord.txt")
	hostOK := pathExists(hostlist)

	cfg, installed := s.readConfig()
	if !installed {
		return nil, s.notInstalledErr()
	}
	if err := autoConflict(cfg, discordTCPProfile); err != nil {
		return nil, err
	}
	if err := profileModeGuard(cfg, discordTCPProfile); err != nil {
		return nil, err
	}

	c := telemetry.New(s.paths)
	rec, _ := c.LoadRecommendations()

	tcpStatus, tcpStrategy, tcpWarn := s.repairDiscordTCP(r.Context(), scope, rec)
	udpStatus, voiceStrategy, voiceWarn := s.repairDiscordVoice(r.Context(), scope, rec, tcpStatus == "ok")

	result := "fail"
	switch {
	case tcpStatus == "ok" && udpStatus == discordUDPConfigured:
		result = "ok"
	case tcpStatus == "ok" || udpStatus == discordUDPConfigured:
		result = "partial"
	}
	if !hostOK && result == "ok" {
		result = "partial"
	}

	out := map[string]any{
		"ok":             result == "ok",
		"result":         result,
		"tcp":            tcpStatus,
		"udp":            udpStatus,
		"udp_verified":   false, // честно: нет UDP/voice probe в netcheck
		"tcp_profile":    discordTCPProfile,
		"voice_profile":  discordVoiceProfile,
		"hostlist_ok":    hostOK,
		"hostlist":       hostlist,
	}
	if tcpStrategy > 0 {
		out["tcp_strategy"] = tcpStrategy
	}
	if voiceStrategy > 0 {
		out["voice_strategy"] = voiceStrategy
	}
	var warns []string
	if tcpWarn != "" {
		warns = append(warns, tcpWarn)
	}
	if voiceWarn != "" {
		warns = append(warns, voiceWarn)
	}
	if !hostOK {
		warns = append(warns, "hostlist TCP_Discord.txt не найден")
	}
	if len(warns) > 0 {
		out["warning"] = joinWarnings(warns)
	}

	s.appendJournal(journalEntry{
		Time: s.now().UTC(), Profile: discordTCPProfile, Strategy: tcpStrategy,
		Result: result, Source: OriginFixDiscord,
	})
	if voiceStrategy > 0 {
		s.appendJournal(journalEntry{
			Time: s.now().UTC(), Profile: discordVoiceProfile, Strategy: voiceStrategy,
			Result: udpStatus, Source: OriginFixDiscord,
		})
	}
	_ = p
	return out, nil
}

// repairDiscordTCP — изолированный probe при наличии prober, иначе recs+TLS check+rollback.
func (s *Server) repairDiscordTCP(ctx context.Context, scope string, rec *telemetry.Recommendations) (status string, strategy int, warning string) {
	prev := parseStrategyLabel(s.profileLockLabel(scope, discordTCPProfile))
	candidates := discordCandidates(rec, discordTCPProfile, prev)

	// Предпочтительно изолированный probe (не портит live во время перебора).
	if len(candidates) > 0 {
		best, ok, err := s.strategyProberOrDefault().Probe(ctx, discordTCPProfile, candidates)
		if err != nil {
			warning = "TCP probe: " + err.Error()
		} else if ok && best > 0 {
			if err := s.applyProfileLock(scope, discordTCPProfile, orchestra.Strategy(best)); err != nil {
				return "fail", 0, "TCP SetLock: " + err.Error()
			}
			_ = sleepBrief(ctx, relockSettleDefault)
			cctx, cancel := context.WithTimeout(ctx, relockCheckTimeout)
			liveOK := profileCheckOK(s.checker.CheckProfile(cctx, discordTCPProfile))
			cancel()
			if !liveOK {
				_ = s.applyProfileLock(scope, discordTCPProfile, prev)
				return "fail", 0, "TCP live verify fail, откат"
			}
			_ = s.lockOrigin.Set(discordTCPProfile, OriginFixDiscord)
			return "ok", best, warning
		}
	}

	// Уже рабочий текущий лок.
	cctx, cancel := context.WithTimeout(ctx, relockCheckTimeout)
	ok := profileCheckOK(s.checker.CheckProfile(cctx, discordTCPProfile))
	cancel()
	if ok {
		st := 0
		if !prev.IsAuto() {
			st = int(prev)
		}
		return "ok", st, warning
	}
	return "fail", 0, warning
}

// repairDiscordVoice — фиксирует стратегию профиля 6 и включает порты при необходимости.
// UDP TLS-check отсутствует: статус configured ≠ «голос работает».
func (s *Server) repairDiscordVoice(ctx context.Context, scope string, rec *telemetry.Recommendations, tcpOK bool) (status string, strategy int, warning string) {
	prev := parseStrategyLabel(s.profileLockLabel(scope, discordVoiceProfile))
	hadLock := !prev.IsAuto()

	// Без TCP-успеха и без прежнего voice-lock — не включаем порты насильно.
	if !tcpOK && !hadLock {
		return discordUDPSkipped, 0, "voice пропущен: TCP fail и нет прежней фиксации профиля 6"
	}

	chosen := 0
	if rec != nil && rec.Status == telemetry.StatusReady {
		if tops := rec.Top(discordVoiceProfile); len(tops) > 0 {
			chosen = tops[0].Strategy
		}
	}
	if chosen < 1 && hadLock {
		chosen = int(prev)
	}
	if chosen < 1 {
		// Есть TCP ok, но нечего фиксировать — порты всё же включим под auto? Нет: без стратегии voice бесполезен.
		if tcpOK {
			if w := s.voicePorts(ctx, true); w != "" {
				return discordUDPFail, 0, w
			}
			return discordUDPSkipped, 0, "нет рекомендации/лока для voice (профиль 6); порты включены, стратегия auto"
		}
		return discordUDPSkipped, 0, "нет стратегии для voice"
	}

	if err := s.applyProfileLock(scope, discordVoiceProfile, orchestra.Strategy(chosen)); err != nil {
		return discordUDPFail, 0, "voice SetLock: " + err.Error()
	}
	if w := s.voicePorts(ctx, true); w != "" {
		_ = s.applyProfileLock(scope, discordVoiceProfile, prev)
		return discordUDPFail, 0, w
	}
	// Origin только после успешного apply+ports (не при skipped/fail).
	_ = s.lockOrigin.Set(discordVoiceProfile, OriginFixDiscord)
	return discordUDPConfigured, chosen, ""
}

func discordCandidates(rec *telemetry.Recommendations, profile int, prev orchestra.Strategy) []int {
	var out []int
	seen := map[int]bool{}
	add := func(n int) {
		if n < 1 || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	if rec != nil && rec.Status == telemetry.StatusReady {
		for _, r := range rec.Top(profile) {
			add(r.Strategy)
		}
	}
	if !prev.IsAuto() {
		p := int(prev)
		for _, d := range []int{0, 1, -1, 2, -2} {
			add(p + d)
		}
	}
	for i := 1; i <= 12 && len(out) < 10; i++ {
		add(i)
	}
	return out
}

func joinWarnings(parts []string) string {
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "; "
		}
		out += p
	}
	return out
}
