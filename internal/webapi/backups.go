package webapi

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/backup"
	"github.com/AloofLibra/zator/internal/fetch"
)

var backupNameRe = regexp.MustCompile(`^z2r_backup_(\d{8})_(\d{6})(_\d+)?\.tar$`)

const backupTimeLayout = "20060102_150405"

type backupItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Date string `json:"date"`
}

func (s *Server) backupMgr() *backup.Manager {
	return backup.NewIn(s.paths, s.paths.BackupDir)
}

func backupDate(name string) string {
	m := backupNameRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	t, err := time.Parse(backupTimeLayout, m[1]+"_"+m[2])
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func (s *Server) backupsList() (map[string]any, error) {
	list, err := s.backupMgr().List()
	if err != nil {
		return nil, err
	}
	items := make([]backupItem, 0, len(list))
	for _, info := range list {
		items = append(items, backupItem{Name: info.Name, Size: info.Size, Date: backupDate(info.Name)})
	}
	return map[string]any{"items": items}, nil
}

// GET|POST /cgi-bin/backups.cgi
func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request, p params) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	action := p.Get("action")
	if r.Method == http.MethodGet {
		if action == "download" {
			s.backupDownload(w, p.Get("name"))
			return
		}
		v, err := s.backupsList()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось прочитать список бэкапов")
			return
		}
		writeJSON(w, http.StatusOK, v)
		return
	}
	mgr := s.backupMgr()
	switch action {
	case "create":
		info, err := mgr.Create(backup.CreateOptions{IncludeLists: true, IncludeExtra: true})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось создать бэкап")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": info.Name})
	case "delete":
		name := p.Get("name")
		if name == "" {
			writeError(w, http.StatusBadRequest, "Не указано имя файла")
			return
		}
		if err := mgr.Delete(name); err != nil {
			if errors.Is(err, backup.ErrNotFound) || errors.Is(err, backup.ErrInvalidName) {
				writeError(w, http.StatusNotFound, "Бэкап не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "Не удалось удалить бэкап")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
	case "restore":
		name := p.Get("name")
		if name == "" {
			writeError(w, http.StatusBadRequest, "Не указано имя файла")
			return
		}
		mode := strings.ToLower(strings.TrimSpace(p.Get("mode")))
		if mode == "" {
			mode = "smart"
		}
		n, err := s.restoreBackup(name, mode)
		if err != nil {
			if errors.Is(err, backup.ErrNotFound) || errors.Is(err, backup.ErrInvalidName) {
				writeError(w, http.StatusNotFound, "Бэкап не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "Не удалось восстановить бэкап: "+err.Error())
			return
		}
		restarted := s.maybeRestart(r.Context(), p)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "name": name, "mode": mode, "files": n, "restarted": restarted,
		})
	case "upload":
		s.backupUpload(w, r, p.Get("name"))
	default:
		writeError(w, http.StatusBadRequest, "Неизвестное действие")
	}
}

// restoreBackup восстанавливает через backup.Manager (manager- и webui-/shell-layout).
func (s *Server) restoreBackup(name, mode string) (int, error) {
	bm, err := backup.ParseMode(mode)
	if err != nil {
		return 0, fmt.Errorf("неизвестный режим: %s", mode)
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	blockFull := s.app != nil && s.app.BlockFullRestore()
	res, err := s.backupMgr().Restore(name, backup.RestoreOptions{Mode: bm, BlockFull: blockFull})
	if err != nil {
		return 0, err
	}
	return len(res.Files), nil
}

func (s *Server) backupDownload(w http.ResponseWriter, name string) {
	if name == "" {
		writeError(w, http.StatusBadRequest, "Не указано имя файла")
		return
	}
	path, err := s.backupMgr().Resolve(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Бэкап не найден")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "Бэкап не найден")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	if st != nil {
		w.Header().Set("Content-Length", fmt.Sprint(st.Size()))
	}
	_, _ = io.Copy(w, f)
}

func (s *Server) backupUpload(w http.ResponseWriter, r *http.Request, original string) {
	if r.ContentLength > maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Файл слишком большой (максимум 32 МБ)")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Файл получен не полностью")
		return
	}
	switch {
	case len(data) == 0:
		writeError(w, http.StatusBadRequest, "Пустой файл")
		return
	case len(data) > maxUploadBytes:
		writeError(w, http.StatusRequestEntityTooLarge, "Файл слишком большой (максимум 32 МБ)")
		return
	case r.ContentLength > 0 && int64(len(data)) < r.ContentLength:
		writeError(w, http.StatusBadRequest, "Файл получен не полностью")
		return
	}
	tmp, err := os.CreateTemp("", "zator-backup-upload-*.tar")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить бэкап")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить бэкап")
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить бэкап")
		return
	}
	name, err := s.backupMgr().Import(tmpName, filepath.Base(original))
	if err != nil {
		if errors.Is(err, backup.ErrNotBackup) {
			writeError(w, http.StatusBadRequest, "Файл не является архивом бэкапа")
			return
		}
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить бэкап")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}

// validBackupArchive — smoke-проверка (тесты); Import в Manager делает полную валидацию.
func validBackupArchive(data []byte) bool {
	tr := tar.NewReader(bytes.NewReader(data))
	found := false
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return found
		}
		if err != nil {
			return false
		}
		n := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if n == "config" || strings.HasPrefix(n, "lists/") || strings.HasPrefix(n, "extra_strats/") || strings.HasPrefix(n, "orchestra/") {
			found = true
		}
	}
}

// extractPlainTar безопасно распаковывает uncompressed tar (тесты B4).
func extractPlainTar(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := fetch.CleanEntryName(hdr.Name)
		if err != nil {
			return err
		}
		if name == "." {
			continue
		}
		target := filepath.Join(absDest, filepath.FromSlash(name))
		if rel, err := filepath.Rel(absDest, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %q", fetch.ErrUnsafePath, hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size > fetch.MaxFileSize {
				return fmt.Errorf("backup: файл %q слишком большой (%d байт)", hdr.Name, hdr.Size)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, fetch.MaxFileSize)); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: недопустимый тип записи %q (%c)", fetch.ErrUnsafePath, hdr.Name, hdr.Typeflag)
		}
	}
}
