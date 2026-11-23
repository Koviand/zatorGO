package webapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/firewall"
	"github.com/AloofLibra/zator/internal/provider"
)

// --- порты ---


var portTokenRe = regexp.MustCompile(`^([0-9]{1,5})(-([0-9]{1,5}))?$`)

func validPortToken(t string) bool {
	m := portTokenRe.FindStringSubmatch(t)
	if m == nil {
		return false
	}
	lo, _ := strconv.Atoi(m[1])
	if lo < 1 || lo > 65535 {
		return false
	}
	if m[3] != "" {
		hi, _ := strconv.Atoi(m[3])
		return hi >= lo && hi <= 65535
	}
	return true
}

func portProto(p params) (string, error) {
	proto := strings.ToLower(p.Get("proto"))
	if proto != "tcp" && proto != "udp" {
		return "", errf(http.StatusBadRequest, "Некорректный протокол: "+p.Get("proto"))
	}
	return proto, nil
}

func (s *Server) postPortsAdd(r *http.Request, p params) (map[string]any, error) {
	proto, err := portProto(p)
	if err != nil {
		return nil, err
	}
	raw := p.Get("value")
	var valid, invalid []string
	for _, t := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t' }) {
		if validPortToken(t) {
			valid = append(valid, t)
		} else {
			invalid = append(invalid, t)
		}
	}
	var added, skipped []string
	err = s.editConfigs(func(c *config.Config) error {
		var e error
		added, skipped, e = c.AddUserPorts(proto, valid)
		return e
	})
	if err != nil {
		return nil, err
	}
	skipped = append(skipped, invalid...)
	if len(added) == 0 {
		return nil, errf(http.StatusBadRequest, "Ничего не добавлено (некорректные значения или дубликаты): "+strings.Join(skipped, ","))
	}
	return map[string]any{
		"ok": true, "added": strings.Join(added, ","), "skipped": strings.Join(skipped, ","),
		"restarted": s.maybeRestart(r.Context(), p),
	}, nil
}

func (s *Server) postPortsRemove(r *http.Request, p params) (map[string]any, error) {
	proto, err := portProto(p)
	if err != nil {
		return nil, err
	}
	v := p.Get("value")
	if proto == "udp" && v == config.UDPGamesPorts {
		return nil, errf(http.StatusBadRequest, "Диапазон 1026-65531 управляется переключателем игрового UDP на вкладке Настройки")
	}
	found := false
	err = s.editConfigs(func(c *config.Config) error {
		f, e := c.RemoveUserPort(proto, v)
		if e == nil && f {
			found = true
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errf(http.StatusBadRequest, "Порт не найден среди добавленных: "+v)
	}
	return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}, nil
}

// --- провайдер ---

func (s *Server) postProviderSet(_ *http.Request, p params) (map[string]any, error) {
	name := p.Get("name")
	city := p.Get("city")
	full, err := s.provider.Set(name, city)
	if err != nil {
		if errors.Is(err, provider.ErrEmpty) {
			return nil, errf(http.StatusBadRequest, "Укажите название провайдера")
		}
		return nil, err
	}
	return map[string]any{"ok": true, "provider": full}, nil
}

func (s *Server) postProviderRedetect(r *http.Request, _ params) (map[string]any, error) {
	res, err := s.provider.Redetect(r.Context())
	if err != nil {
		if errors.Is(err, provider.ErrNotDetected) {
			return map[string]any{
				"ok":       true,
				"provider": provider.Unknown,
				"warning":  "Не удалось определить провайдера",
			}, nil
		}
		return nil, errf(http.StatusBadGateway, "Не удалось определить провайдера: "+err.Error())
	}
	return map[string]any{
		"ok":       true,
		"provider": res.Display,
		"asn":      res.ASN,
		"city":     res.City,
		"brand":    res.Brand,
		"source":   res.Source,
	}, nil
}

// --- client scope ---

var markMaskRe = regexp.MustCompile(`^0[xX][0-9a-fA-F]{1,8}$`)

func (s *Server) postClientScope(r *http.Request, p params) (map[string]any, error) {
	enable := p.Get("client_scope_enable")
	if enable != "0" && enable != "1" {
		return nil, errf(http.StatusBadRequest, "Некорректное значение: "+enable)
	}
	mask := p.Get("client_scope_mark_mask")
	if mask != "" && !markMaskRe.MatchString(mask) {
		return nil, errf(http.StatusBadRequest, "Некорректная маска: "+mask)
	}
	shift, max := p.Get("client_scope_mark_shift"), p.Get("client_scope_mark_max")
	if shift == "" {
		shift = "8"
	}
	if max == "" {
		max = "255"
	}
	if n, err := strconv.Atoi(shift); err != nil || n < 0 || n > 31 {
		return nil, errf(http.StatusBadRequest, "Некорректный сдвиг: "+shift)
	}
	if n, err := strconv.Atoi(max); err != nil || n < 0 || n > 255 {
		return nil, errf(http.StatusBadRequest, "Некорректный максимум: "+max)
	}
	warning := ""
	enabled := enable == "1"
	err := s.editConfigs(func(c *config.Config) error {
		for k, v := range map[string]string{
			"CLIENT_SCOPE_MARK_MASK": mask, "CLIENT_SCOPE_MARK_SHIFT": shift, "CLIENT_SCOPE_MARK_MAX": max,
		} {
			if err := c.Set(k, v); err != nil {
				return err
			}
		}
		on := enabled
		if on {
			if mask == "" {
				on, warning = false, "Маска метки пуста: client scope безопасно выключен"
			} else if err := s.validScope(c); err != nil {
				on, warning = false, err.Error()
			}
		}
		if on {
			return c.Set("CLIENT_SCOPE_ENABLE", "1")
		}
		return c.Set("CLIENT_SCOPE_ENABLE", "0")
	})
	if err != nil {
		return nil, err
	}
	c, _ := s.readConfig()
	_ = s.app.SyncCustomDHooks(c)
	return map[string]any{
		"ok": true, "restarted": s.maybeRestart(r.Context(), p),
		"enabled": c.ClientScopeText() == "включен", "warning": warning,
	}, nil
}

func (s *Server) validScope(c *config.Config) error {
	cc, err := firewall.ClientScopeConfigFromGetter(c.Get, s.paths.ZatorRoot)
	if err != nil {
		return err
	}
	return cc.Validate()
}

// --- wssize_ramp ---

func (s *Server) postWssizeRamp(r *http.Request, p params) (map[string]any, error) {
	vars := map[string]string{}
	if v := p.Get("value"); v != "" {
		if v != "0" && v != "1" {
			return nil, errf(http.StatusBadRequest, "Некорректное значение: "+v)
		}
		vars["Z2R_WSSIZE_RAMP_ENABLE"] = v
	}
	if m := strings.ToLower(p.Get("mode")); m != "" {
		if m != "full" && m != "lite" {
			return nil, errf(http.StatusBadRequest, "Некорректный режим: "+m)
		}
		vars["Z2R_WSSIZE_RAMP_MODE"] = m
	}
	if sc := p.Get("scale"); sc != "" {
		n, err := strconv.Atoi(sc)
		if err != nil || n < 0 || n > 14 {
			return nil, errf(http.StatusBadRequest, "Некорректный scale: "+sc)
		}
		vars["Z2R_WSSIZE_RAMP_SCALE"] = strconv.Itoa(n)
	}
	if len(vars) == 0 {
		return nil, errf(http.StatusBadRequest, "Некорректное значение: ")
	}
	if vars["Z2R_WSSIZE_RAMP_ENABLE"] == "1" {
		if _, err := os.Stat(filepath.Join(s.paths.LuaDir, "wssize-ramp.lua")); err != nil {
			return nil, errf(http.StatusInternalServerError, "Файл wssize-ramp.lua отсутствует на устройстве.")
		}
	}
	err := s.editConfigs(func(c *config.Config) error {
		for k, v := range vars {
			if err := c.Set(k, v); err != nil {
				return err
			}
		}
		if _, err := c.SyncWssizeRampVars(); err != nil {
			return err
		}
		c.StripWssizeShellIf()
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}
	// При включении WSSIZE подтянуть ipv4 (force при явном refresh_ips=1).
	if vars["Z2R_WSSIZE_RAMP_ENABLE"] == "1" || p.Get("refresh_ips") == "1" {
		force := p.Get("refresh_ips") == "1"
		if res, e := s.app.RefreshGooglevideoIPv4(r.Context(), force); e == nil {
			out["ipv4_status"] = res.Status
			out["ipv4_count"] = res.Count
		} else {
			out["ipv4_error"] = e.Error()
		}
	}
	return out, nil
}
