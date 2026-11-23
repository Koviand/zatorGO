package webapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/deploy"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/service"
)

const (
	maxConfigRawBytes = 2 << 20 // 2 MiB
	maxErrLogBytes    = 256 << 10
)

// configRawGet — GET settings.cgi?setting=config_raw (п.7 CLI).
func (s *Server) configRawGet() map[string]any {
	path := s.paths.ZapretConfig
	out := map[string]any{
		"path":    path,
		"content": "",
		"exists":  false,
		"size":    0,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			out["error"] = err.Error()
		}
		return out
	}
	out["exists"] = true
	out["size"] = len(data)
	out["content"] = string(data)
	return out
}

func (s *Server) postConfigRaw(r *http.Request, p params) (map[string]any, error) {
	// Raw — без TrimSpace, иначе съедаются края многострочного config.
	content := p.Raw("value")
	if strings.TrimSpace(content) == "" {
		return nil, errf(http.StatusBadRequest, "Пустой конфиг")
	}
	if len(content) > maxConfigRawBytes {
		return nil, errf(http.StatusBadRequest, "Конфиг слишком большой")
	}
	if !utf8.ValidString(content) {
		return nil, errf(http.StatusBadRequest, "Конфиг должен быть UTF-8 текстом")
	}
	if !strings.Contains(content, "NFQWS2_OPT") {
		return nil, errf(http.StatusBadRequest, "В конфиге нет NFQWS2_OPT — отказ в записи")
	}
	if err := config.ValidateSourceSafe(content); err != nil {
		return nil, errf(http.StatusBadRequest, err.Error())
	}
	path := s.paths.ZapretConfig
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Снимок для отката «Вручную» (Manual) — до замены рабочего config.
	if prev, err := os.ReadFile(path); err == nil && len(prev) > 0 {
		_ = fsutil.AtomicWriteFile(path+".bak", prev, 0o644)
	}
	if err := fsutil.AtomicWriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "restarted": s.maybeRestart(r.Context(), p), "size": len(content)}, nil
}

// nfqws2ErrorsGet — журнал ошибок последнего запуска (п.666 CLI).
func (s *Server) nfqws2ErrorsGet() map[string]any {
	candidates := []string{
		"/tmp/nfqws2_1.err",
		"/opt/tmp/nfqws2_1.err",
		filepath.Join(os.TempDir(), "nfqws2_1.err"),
	}
	out := map[string]any{"available": false, "path": "", "content": "", "lines": 0}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if len(data) > maxErrLogBytes {
			data = data[len(data)-maxErrLogBytes:]
		}
		text := string(data)
		n := 0
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		out["available"] = true
		out["path"] = p
		out["content"] = text
		out["lines"] = n
		return out
	}
	return out
}

func (s *Server) postUpdateApply(r *http.Request, _ params) (map[string]any, error) {
	out := map[string]any{"ok": true, "up_to_date": false, "installed": 0, "restart_required": false}
	// Package C: product-backup до Apply (fail-closed — иначе откат на устаревший архив).
	if s.app != nil {
		info, err := s.app.CreatePreUpdateBackup()
		if err != nil {
			return nil, errf(http.StatusInternalServerError, "pre-update backup обязателен: "+err.Error())
		}
		out["pre_update_backup"] = info.Name
	}
	u := deploy.New(s.paths)
	off := deploy.ResolveOfflineBundle(os.Getenv)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	var (
		installed int
		upToDate  bool
		err       error
	)
	if off.Offline && off.Bundle != nil {
		out["offline"] = true
		out["offline_reason"] = off.Reason
		ar, aerr := u.ApplyBundle(*off.Bundle, "")
		if aerr != nil {
			return nil, errf(http.StatusBadGateway, "Офлайн-обновление не удалось: "+aerr.Error())
		}
		if ar != nil {
			installed = ar.Installed
			out["installed"] = installed
		}
		out["up_to_date"] = false
	} else {
		if off.Offline {
			u.Offline = true
		}
		rep, rerr := u.Run(ctx, deploy.RunOptions{Variant: deploy.VariantFull})
		err = rerr
		if err != nil {
			return nil, errf(http.StatusBadGateway, "Обновление не удалось: "+err.Error())
		}
		if rep != nil {
			upToDate = rep.UpToDate
			out["up_to_date"] = upToDate
			if rep.Apply != nil {
				installed = rep.Apply.Installed
				out["installed"] = installed
			}
			if rep.Check != nil && rep.Check.Latest != nil {
				out["release"] = rep.Check.Latest.Release
			}
		}
	}
	if installed > 0 && !upToDate {
		verifyOK := true
		if s.app != nil {
			if err := s.app.VerifyAfterUpdate(); err != nil {
				verifyOK = false
				out["post_update_verify"] = "fail"
				out["post_update_verify_detail"] = err.Error()
				if rerr := s.app.RollbackPreUpdateBackup(); rerr != nil {
					out["rollback"] = "fail"
					out["rollback_detail"] = rerr.Error()
					s.app.WriteServiceWarnings([]string{err.Error(), "rollback: " + rerr.Error()})
				} else {
					out["rollback"] = "ok"
					out["post_update_verify"] = "ok"
					out["post_update_verify_detail"] = "restored from pre-update backup"
					verifyOK = true
				}
			} else {
				out["post_update_verify"] = "ok"
			}
		}
		if verifyOK {
			out["restart_required"] = true
			s.schedulePostUpdateRestarts()
			out["webui_restart_scheduled"] = true
			out["zapret2_restart_scheduled"] = true
		} else {
			out["restart_required"] = false
			out["restart_skipped"] = "post-update verify failed — рестарт отложен; откат из pre_update_backup не удался"
		}
	}
	return out, nil
}

// schedulePostUpdateRestarts — отложенный рестарт webui + zapret2 (новый Lua в nfqws2).
func (s *Server) schedulePostUpdateRestarts() {
	if s.svc == nil {
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if s.app != nil {
			s.runtimeMu.Lock()
			if err := s.app.RestartZapretTransactional(ctx, func(ctx context.Context) error {
				return s.svc.Restart(ctx, service.UnitZapret2)
			}); err != nil {
				s.app.WriteServiceWarnings([]string{"post-update restart: " + err.Error()})
			} else {
				var warns []string
				if st, d := s.app.FwEnsureStatus(); st == "fail" && d != "" {
					warns = append(warns, "fw ensure: "+d)
				}
				s.app.StartNfqwsWatchdog()
				if len(warns) > 0 {
					s.app.WriteServiceWarnings(warns)
				} else {
					s.app.ClearServiceWarnings()
				}
			}
			s.runtimeMu.Unlock()
		} else {
			_ = s.svc.Restart(ctx, service.UnitZapret2)
		}
		_ = s.svc.Restart(ctx, service.UnitValidator)
		_ = s.svc.Restart(ctx, service.UnitWebUI)
		if s.app != nil {
			s.app.StartWebUIWatchdog()
		}
	}()
}
