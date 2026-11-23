package webapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AloofLibra/zator/internal/config"
)

// --- переключатели 0|1 ---


type toggle struct {
	names    []string
	apply    func(c *config.Config, on bool) error
	state    func(c *config.Config) string // nil — поле state не возвращается
	guard    func(s *Server, c *config.Config, on bool) error
	notFound string
}

func parseOnOff(p params) (bool, error) {
	switch v := p.Get("value"); v {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, errf(http.StatusBadRequest, "Некорректное значение: "+v)
	}
}

var toggles = []toggle{
	{
		names:    []string{"wg_state", "wg"},
		apply:    (*config.Config).SetWireGuard,
		notFound: "Стратегия WireGuard не найдена в конфиге (нет блока #Z2R_WG_*)",
	},
	{
		names: []string{"fallback_state", "fallback"},
		apply: (*config.Config).SetFallback,
		guard: func(_ *Server, c *config.Config, on bool) error {
			if on && c.AutoModeText() == "включен" {
				return errf(http.StatusConflict, "Безразборный режим недоступен при включённой авторотации TCP/HTTP. Сначала выключите авторотацию.")
			}
			return nil
		},
		notFound: "Блоки безразборного режима не найдены в конфиге." + cliHint,
	},
	{
		names:    []string{"udp_games_state", "udp_games"},
		apply:    (*config.Config).SetUDPGames,
		notFound: "Блок игрового UDP не найден в конфиге." + cliHint,
	},
	{
		names: []string{"auto_mode_state", "auto_mode"},
		apply: (*config.Config).SetAutoMode,
		state: (*config.Config).AutoModeText,
		guard: func(_ *Server, c *config.Config, on bool) error {
			if on && c.FallbackText() == "включен" {
				return errf(http.StatusConflict, "Авторотация недоступна при включённом безразборном режиме. Сначала выключите безразборный режим.")
			}
			return nil
		},
		notFound: "В конфиге не найдены маркеры авторежима." + cliHint,
	},
	{
		names: []string{"hostlist_state", "hostlist"},
		apply: func(c *config.Config, on bool) error { return c.SetHostlistAuto(on) },
		state: (*config.Config).HostlistText,
	},
	{
		names: []string{"rst_guard_state", "rst_guard"},
		apply: (*config.Config).SetRSTGuard,
		state: (*config.Config).RSTGuardText,
		guard: func(s *Server, _ *config.Config, on bool) error {
			if !on {
				return nil
			}
			if _, err := os.Stat(filepath.Join(s.paths.LuaDir, "rst-guard.lua")); err != nil {
				return errf(http.StatusInternalServerError, "Файл rst-guard.lua отсутствует на устройстве. Включите защиту один раз через CLI (пункт 18).")
			}
			return nil
		},
		notFound: "Строки rst_guard не найдены в конфиге." + cliHint,
	},
	{
		names:    []string{"reasm_state", "reasm"},
		apply:    (*config.Config).SetReasm,
		state:    (*config.Config).ReasmText,
		notFound: "Не найден блок NFQWS2_OPT в конфиге." + cliHint,
	},
	{
		names:    []string{"quic443_state", "quic443"},
		apply:    (*config.Config).SetQUIC443,
		state:    (*config.Config).QUIC443Text,
		notFound: "Блок QUIC (UDP443) не найден в конфиге." + cliHint,
	},
	{
		names:    []string{"dns_desync_state", "dns_desync"},
		apply:    (*config.Config).SetDNSDesync,
		state:    (*config.Config).DNSDesyncText,
		notFound: "Блок DNS (UDP:53) не найден в конфиге." + cliHint,
	},
}

func (s *Server) postToggle(r *http.Request, p params, t toggle) (map[string]any, error) {
	on, err := parseOnOff(p)
	if err != nil {
		return nil, err
	}
	cur, installed := s.readConfig()
	if !installed {
		return nil, s.notInstalledErr()
	}
	if t.guard != nil {
		if err := t.guard(s, cur, on); err != nil {
			return nil, err
		}
	}
	if err := s.editConfigs(func(c *config.Config) error { return t.apply(c, on) }); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, t.notFound)
		}
		return nil, err
	}
	out := map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}
	if t.state != nil {
		c, _ := s.readConfig()
		out["state"] = t.state(c)
	}
	return out, nil
}
