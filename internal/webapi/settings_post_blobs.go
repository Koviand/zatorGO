package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/orchestra"
)

// --- блобы ---


func (s *Server) blobExists(name string) bool {
	st, err := os.Stat(filepath.Join(s.paths.FakeDir, name))
	return err == nil && !st.IsDir()
}

func regexpMust(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

var (
	maxruDeclRe = regexpMust(`(?m)^(\s*--blob=maxru:@\S*/)[^/\s]+`)
	wgDeclRe    = regexpMust(`(--blob=fakewgblob:@\S*/)[^/\s]+`)
	wgRepeatsRe = regexpMust(`blob=fakewgblob:repeats=[0-9]+`)
)

// editOpt применяет fn к тексту NFQWS2_OPT.
func editOpt(c *config.Config, fn func(opt string) (string, error)) error {
	opt, ok := c.NFQWS2Opt()
	if !ok {
		return fmt.Errorf("%w: %s", config.ErrNotFound, config.OptVar)
	}
	next, err := fn(opt)
	if err != nil {
		return err
	}
	return c.SetNFQWS2Opt(next)
}

// switchTLSBlob переключает ссылки стратегий maxru <-> fake_default_tls и путь блоба.
func switchTLSBlob(c *config.Config, value string) error {
	return editOpt(c, func(opt string) (string, error) {
		lines := strings.Split(opt, "\n")
		for i, ln := range lines {
			if !strings.Contains(ln, "--lua-desync=") || strings.Contains(ln, "strategy=26") {
				continue
			}
			if value == blobDefault {
				lines[i] = strings.ReplaceAll(ln, "blob=maxru", "blob="+blobDefault)
			} else {
				lines[i] = strings.ReplaceAll(ln, "blob="+blobDefault, "blob=maxru")
			}
		}
		out := strings.Join(lines, "\n")
		if value != blobDefault {
			if !maxruDeclRe.MatchString(out) {
				return "", fmt.Errorf("%w: --blob=maxru", config.ErrNotFound)
			}
			out = maxruDeclRe.ReplaceAllString(out, "${1}"+value)
		}
		return out, nil
	})
}

func (s *Server) postTLSBlob(r *http.Request, p params) (map[string]any, error) {
	v := p.Get("value")
	if v != blobDefault && !tlsBlobFileRe.MatchString(v) {
		return nil, errf(http.StatusBadRequest, "Некорректное значение блоба: "+v)
	}
	if v != blobDefault && !s.blobExists(v) {
		return nil, errf(http.StatusBadRequest, "Файл блоба не существует: "+v)
	}
	if err := s.editConfigs(func(c *config.Config) error { return switchTLSBlob(c, v) }); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, "TLS-блоб не найден в конфиге (нет --blob=maxru:@...)")
		}
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}, nil
}

// overrideProfile проверяет profile из набора профилей с переопределениями.
func overrideProfile(p params) (string, error) {
	pr := p.Get("profile")
	for _, id := range orchestra.OverrideProfiles {
		if strconv.Itoa(id) == pr {
			return pr, nil
		}
	}
	return "", errf(http.StatusBadRequest, "Некорректный профиль")
}

func (s *Server) postTLSBlobProfile(r *http.Request, p params) (map[string]any, error) {
	profile, err := overrideProfile(p)
	if err != nil {
		return nil, err
	}
	v := p.Get("value")
	switch {
	case v == "":
		if err := s.store.ClearOverride(orchestra.OverrideBlob, profile); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "restarted": false, "restart_required": false}, nil
	case v == blobDefault:
		if err := s.store.SetOverride(orchestra.OverrideBlob, profile, blobDefault); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "restarted": false, "restart_required": false}, nil
	case !tlsBlobFileRe.MatchString(v):
		return nil, errf(http.StatusBadRequest, "Некорректное значение блоба: "+v)
	case !s.blobExists(v):
		return nil, errf(http.StatusBadRequest, "Файл блоба не существует: "+v)
	}
	slot := regexpMust(`(?m)^(\s*--blob=z2r_prof_` + profile + `:@\S*/)[^/\s]+`)
	err = s.editConfigs(func(c *config.Config) error {
		return editOpt(c, func(opt string) (string, error) {
			if !slot.MatchString(opt) {
				return "", fmt.Errorf("%w: слот z2r_prof_%s", config.ErrNotFound, profile)
			}
			return slot.ReplaceAllString(opt, "${1}"+v), nil
		})
	})
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, "Слот блоба z2r_prof_"+profile+" не найден в конфиге")
		}
		return nil, err
	}
	out := map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p), "restart_required": true}
	if err := s.store.SetOverride(orchestra.OverrideBlob, profile, strings.TrimSuffix(v, ".bin")); err != nil {
		// Конфиг уже с новым blob — не откатываем и не отдаём 500.
		out["warning"] = "Конфиг обновлён, но override не сохранён: " + err.Error()
	}
	return out, nil
}

func (s *Server) postFakeMode(_ *http.Request, p params) (map[string]any, error) {
	profile, err := overrideProfile(p)
	if err != nil {
		return nil, err
	}
	switch v := p.Get("value"); v {
	case orchestra.ModeClassic:
		err = s.store.ClearOverride(orchestra.OverrideMode, profile)
	case orchestra.ModeClone:
		err = s.store.SetOverride(orchestra.OverrideMode, profile, v)
	default:
		return nil, errf(http.StatusBadRequest, "Некорректное значение: "+v)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": false, "restart_required": false}, nil
}

func (s *Server) postCloneSize(_ *http.Request, p params) (map[string]any, error) {
	profile, err := overrideProfile(p)
	if err != nil {
		return nil, err
	}
	v := p.Get("value")
	if v == "" || v == "global" {
		err = s.store.ClearOverride(orchestra.OverrideCloneSize, profile)
	} else {
		if verr := orchestra.OverrideCloneSize.ValidateValue(v); verr != nil {
			return nil, errf(http.StatusBadRequest, fmt.Sprintf("Некорректное значение: %s (допустимо %d..%d)", v, orchestra.CloneSizeMin, orchestra.CloneSizeMax))
		}
		err = s.store.SetOverride(orchestra.OverrideCloneSize, profile, v)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": false, "restart_required": false}, nil
}

func (s *Server) postWGBlob(r *http.Request, p params) (map[string]any, error) {
	v := p.Get("value")
	if !wgBlobFileRe.MatchString(v) {
		return nil, errf(http.StatusBadRequest, "Некорректное значение блоба: "+v)
	}
	if !s.blobExists(v) {
		return nil, errf(http.StatusBadRequest, "Файл блоба не существует: "+v)
	}
	err := s.editConfigs(func(c *config.Config) error {
		return editOpt(c, func(opt string) (string, error) {
			if !wgDeclRe.MatchString(opt) {
				return "", fmt.Errorf("%w: fakewgblob", config.ErrNotFound)
			}
			return wgDeclRe.ReplaceAllString(opt, "${1}"+v), nil
		})
	})
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, "Стратегия WireGuard не найдена в конфиге (нет --blob=fakewgblob:@...)")
		}
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}, nil
}

func (s *Server) postWGRepeats(r *http.Request, p params) (map[string]any, error) {
	v := p.Get("value")
	if !strategyRe.MatchString(v) {
		return nil, errf(http.StatusBadRequest, "Некорректное значение repeats")
	}
	n, _ := strconv.Atoi(v)
	if n < 2 || n > 99 {
		return nil, errf(http.StatusBadRequest, "Значение repeats должно быть от 2 до 99")
	}
	err := s.editConfigs(func(c *config.Config) error {
		return editOpt(c, func(opt string) (string, error) {
			if !wgRepeatsRe.MatchString(opt) {
				return "", fmt.Errorf("%w: repeats", config.ErrNotFound)
			}
			return wgRepeatsRe.ReplaceAllString(opt, "blob=fakewgblob:repeats="+strconv.Itoa(n)), nil
		})
	})
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, "Стратегия WireGuard не найдена в конфиге (нет blob=fakewgblob:repeats=)")
		}
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p)}, nil
}
