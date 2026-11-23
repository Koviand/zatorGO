package webapi

import (
	"net/http"

	"github.com/AloofLibra/zator/internal/config"
)

// postFn — обработчик POST settings.cgi; возвращает тело успешного ответа.
type postFn func(s *Server, r *http.Request, p params) (map[string]any, error)

var postSettingsTable map[string]postFn

func init() {
	postSettingsTable = map[string]postFn{
		"tls_blob":         (*Server).postTLSBlob,
		"tls_blob_profile": (*Server).postTLSBlobProfile,
		"fake_mode":        (*Server).postFakeMode,
		"clone_size":       (*Server).postCloneSize,
		"wg_blob":          (*Server).postWGBlob,
		"wg_repeats":       (*Server).postWGRepeats,
		"ports_add":        (*Server).postPortsAdd,
		"ports_remove":     (*Server).postPortsRemove,
		"provider_set":      (*Server).postProviderSet,
		"provider_redetect": (*Server).postProviderRedetect,
		"client_scope": (*Server).postClientScope,
		"wssize_ramp":  (*Server).postWssizeRamp,
		"supersweep":    (*Server).postSupersweep,
		"fwtype":        varSetting("FWTYPE", "Некорректное значение: ", "iptables", "nftables"),
		"flowoffload":   varSetting("FLOWOFFLOAD", "Некорректное значение: ", "donttouch", "none", "software", "hardware"),
		"ifaces":        (*Server).postIfaces,
		"iface":         (*Server).postIfaces,
		"interfaces":    (*Server).postIfaces,
		"config_raw":         (*Server).postConfigRaw,
		"config":             (*Server).postConfigRaw,
		"update_apply":       (*Server).postUpdateApply,
		"strategy_register":  (*Server).postStrategyRegister,
	}
	for _, t := range toggles {
		t := t
		fn := func(s *Server, r *http.Request, p params) (map[string]any, error) {
			return s.postToggle(r, p, t)
		}
		for _, n := range t.names {
			postSettingsTable[n] = fn
		}
	}
}

// settingsPost разбирает setting и вызывает обработчик.
func (s *Server) settingsPost(r *http.Request, p params) (map[string]any, error) {
	name := normSetting(p.Get("setting"))
	fn, ok := postSettingsTable[name]
	if !ok {
		return nil, errf(http.StatusBadRequest, "Неизвестная настройка")
	}
	return fn(s, r, p)
}

// varSetting создаёт обработчик простой переменной конфига с белым списком значений.
func varSetting(name, errPrefix string, allowed ...string) postFn {
	return func(s *Server, r *http.Request, p params) (map[string]any, error) {
		v := p.Get("value")
		okVal := false
		for _, a := range allowed {
			okVal = okVal || a == v
		}
		if !okVal {
			return nil, errf(http.StatusBadRequest, errPrefix+v)
		}
		if err := s.editConfigs(func(c *config.Config) error { return c.Set(name, v) }); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}, nil
	}
}
