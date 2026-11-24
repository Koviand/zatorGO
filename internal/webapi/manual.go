package webapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/orchestra"
)

const maxBlobUploadBytes = 2 << 20 // 2 MiB

// postStrategyRegister — POST settings.cgi setting=strategy_register
// profile=1..10, desync=<строка>, lock=0|1 (опц.), restart=0|1.
func (s *Server) postStrategyRegister(r *http.Request, p params) (map[string]any, error) {
	profile := p.Get("profile")
	if !profileRe.MatchString(profile) {
		return nil, errf(http.StatusBadRequest, "Некорректный профиль")
	}
	id, _ := strconv.Atoi(profile)
	if !config.ValidProfile(id) {
		return nil, errf(http.StatusBadRequest, "Некорректный профиль")
	}
	desync := strings.TrimSpace(p.Raw("desync"))
	if desync == "" {
		desync = strings.TrimSpace(p.Raw("value"))
	}
	if desync == "" {
		return nil, errf(http.StatusBadRequest, "Укажите строку desync")
	}
	cfg, _ := s.readConfig()
	if err := autoConflict(cfg, id); err != nil {
		return nil, err
	}
	if err := profileModeGuard(cfg, id); err != nil {
		return nil, err
	}

	var assigned int
	err := s.editConfigs(func(c *config.Config) error {
		n, err := c.AppendProfileStrategy(id, desync)
		if err != nil {
			return err
		}
		assigned = n
		return nil
	})
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, errf(http.StatusInternalServerError, "Блок профиля не найден в конфиге")
		}
		return nil, errf(http.StatusBadRequest, err.Error())
	}

	locked := false
	lockWarning := ""
	if p.Get("lock") == "1" {
		sc, err := scopeParam(p)
		if err != nil {
			return nil, err
		}
		if err := s.applyProfileLock(sc, id, orchestra.Strategy(assigned)); err != nil {
			// Стратегия уже в конфиге — не откатываем и не отдаём 500.
			lockWarning = "Стратегия добавлена, но зафиксировать не удалось: " + err.Error()
		} else {
			locked = true
			_ = s.lockOrigin.Set(id, OriginManual)
		}
	}

	return map[string]any{
		"ok":           true,
		"profile":      id,
		"strategy":     assigned,
		"max_strategy": assigned,
		"locked":       locked,
		"warning":      lockWarning,
		"restarted":    s.maybeRestart(r.Context(), p),
	}, nil
}

// profileModeGuard запрещает ручные стратегии для выключенных режимов.
func profileModeGuard(cfg *config.Config, id int) error {
	switch id {
	case config.ProfileFallbackT, config.ProfileFallbackH:
		if cfg.FallbackText() != "включен" {
			return errf(http.StatusConflict, "Сначала включите безразборный режим")
		}
	case config.ProfileUDPGames:
		if cfg.UDPGamesText() != "Включен" {
			return errf(http.StatusConflict, "Сначала включите игровой UDP")
		}
	case config.ProfileDNS:
		if cfg.DNSDesyncText() != "Включен" {
			return errf(http.StatusConflict, "Сначала включите антиспуф DNS")
		}
	}
	return nil
}

// POST /cgi-bin/blob-upload.cgi?kind=tls|wg&name=….bin[&select=1]
// Тело — сырые байты (application/octet-stream) или multipart file.
func (s *Server) handleBlobUpload(w http.ResponseWriter, r *http.Request, p params) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	kind := strings.ToLower(p.Get("kind"))
	if kind == "" {
		kind = "tls"
	}
	name := filepath.Base(p.Get("name"))
	data, err := readBlobBody(r, &name)
	if err != nil {
		writeErr(w, err)
		return
	}
	re := tlsBlobFileRe
	if kind == "wg" {
		re = wgBlobFileRe
	} else if kind != "tls" {
		writeError(w, http.StatusBadRequest, "kind должен быть tls или wg")
		return
	}
	if name == "" || !re.MatchString(name) {
		hint := "tls_*.bin или custom_tls.bin"
		if kind == "wg" {
			hint = "wg_initial_fake_*.bin"
		}
		writeError(w, http.StatusBadRequest, "Некорректное имя файла (ожидается "+hint+")")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "Пустой файл")
		return
	}
	if err := os.MkdirAll(s.paths.FakeDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать каталог fake")
		return
	}
	dst := filepath.Join(s.paths.FakeDir, name)
	if err := fsutil.AtomicWriteFile(dst, data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить blob")
		return
	}

	out := map[string]any{
		"ok":              true,
		"name":            name,
		"kind":            kind,
		"available_blobs": s.listFake(re),
		"restarted":       false,
	}
	if p.Get("select") == "1" {
		pp := params{"value": {name}, "restart": {p.Get("restart")}}
		var apply map[string]any
		var aerr error
		if kind == "wg" {
			apply, aerr = s.postWGBlob(r, pp)
		} else {
			apply, aerr = s.postTLSBlob(r, pp)
		}
		if aerr != nil {
			// Файл уже сохранён — не маскируем успех загрузки ошибкой select.
			out["selected"] = false
			out["select_error"] = aerr.Error()
			out["warning"] = "Файл сохранён, но выбрать текущим не удалось: " + aerr.Error()
		} else {
			out["selected"] = true
			if v, ok := apply["restarted"]; ok {
				out["restarted"] = v
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func readBlobBody(r *http.Request, name *string) ([]byte, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		r.Body = http.MaxBytesReader(nil, r.Body, maxBlobUploadBytes+64<<10)
		if err := r.ParseMultipartForm(maxBlobUploadBytes); err != nil {
			return nil, errf(http.StatusBadRequest, "Некорректный multipart")
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			f, hdr, err = r.FormFile("blob")
		}
		if err != nil {
			return nil, errf(http.StatusBadRequest, "Файл не передан (поле file)")
		}
		defer f.Close()
		if *name == "" && hdr != nil {
			*name = filepath.Base(hdr.Filename)
		}
		data, err := io.ReadAll(io.LimitReader(f, maxBlobUploadBytes+1))
		if err != nil {
			return nil, errf(http.StatusBadRequest, "Не удалось прочитать файл")
		}
		if len(data) > maxBlobUploadBytes {
			return nil, errf(http.StatusRequestEntityTooLarge, "Файл слишком большой (макс. 2 МБ)")
		}
		return data, nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBlobUploadBytes+1)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errf(http.StatusBadRequest, "Не удалось прочитать тело запроса")
	}
	if len(data) > maxBlobUploadBytes {
		return nil, errf(http.StatusRequestEntityTooLarge, "Файл слишком большой (макс. 2 МБ)")
	}
	return data, nil
}
