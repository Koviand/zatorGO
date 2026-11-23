package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AloofLibra/zator/internal/backup"
	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/deploy"
	"github.com/AloofLibra/zator/internal/fsutil"
)

const preUpdateBinRel = "pre_update_bin/zator"

// ClearPreUpdateRollbackState снимает имя архива и снимок бинарника,
// чтобы неудачный backup не оставлял откат на устаревшее состояние.
func (a *App) ClearPreUpdateRollbackState() {
	_ = os.Remove(filepath.Join(a.Paths.StateDir, postUpdateBackupFile))
	_ = os.Remove(a.preUpdateBinaryPath())
}

// CreatePreUpdateBackup снимает product-backup перед update (config+lists+orchestra)
// и снимок бинарника bin/zator (ModeSmart его не откатывает).
// При ошибке маркеры отката очищены — RollbackPreUpdateBackup не возьмёт старый архив.
func (a *App) CreatePreUpdateBackup() (backup.Info, error) {
	_ = os.MkdirAll(a.Paths.StateDir, 0o755)
	a.ClearPreUpdateRollbackState()
	mgr := backup.NewIn(a.Paths, a.Paths.BackupDir)
	info, err := mgr.Create(backup.CreateOptions{IncludeLists: true})
	if err != nil {
		return backup.Info{}, err
	}
	if err := a.snapshotPreUpdateBinary(); err != nil {
		a.ClearPreUpdateRollbackState()
		return backup.Info{}, fmt.Errorf("снимок bin/zator: %w", err)
	}
	a.RememberPreUpdateBackup(info.Name)
	return info, nil
}

func (a *App) preUpdateBinaryPath() string {
	return filepath.Join(a.Paths.StateDir, filepath.FromSlash(preUpdateBinRel))
}

func (a *App) snapshotPreUpdateBinary() error {
	src := filepath.Join(a.Paths.BinDir, "zator")
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	dst := a.preUpdateBinaryPath()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (a *App) restorePreUpdateBinary() error {
	src := a.preUpdateBinaryPath()
	st, err := os.Stat(src)
	if err != nil || st.Size() == 0 {
		return fmt.Errorf("нет снимка bin/zator")
	}
	dst := filepath.Join(a.Paths.BinDir, "zator")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(a.Paths.BinDir, 0o755)
	return fsutil.AtomicWriteFile(dst, data, 0o755)
}

// VerifyAfterUpdate — проверка после Apply.
// Жёсткий fail только без бинарника zator (иначе rollback откатывал рабочий
// релиз из‑за временного/мягкого состояния nfqws2.args на Keenetic).
// Проблемы zapret2/config и args пишутся в warnings и detail, но update не валят.
func (a *App) VerifyAfterUpdate() error {
	bin := filepath.Join(a.Paths.BinDir, "zator")
	if _, err := os.Stat(bin); err != nil {
		msg := "нет бинарника zator"
		a.WritePostUpdateVerify(false, msg)
		return fmt.Errorf("post-update verify: %s", msg)
	}

	var soft []string
	if _, err := os.Stat(a.Paths.ZapretConfig); err != nil {
		soft = append(soft, "нет zapret2/config")
	}
	argsPath := filepath.Join(a.Paths.ZapretRoot, "nfqws2.args")
	if st, err := os.Stat(argsPath); err != nil || st.Size() == 0 {
		soft = append(soft, "нет или пуст nfqws2.args")
	} else if raw, err := os.ReadFile(argsPath); err != nil {
		soft = append(soft, "не читается nfqws2.args")
	} else {
		var toks []string
		for _, ln := range strings.Split(string(raw), "\n") {
			ln = strings.TrimSpace(ln)
			if ln != "" && !strings.HasPrefix(ln, "#") {
				toks = append(toks, ln)
			}
		}
		if err := config.ValidateNfqws2Args(toks); err != nil {
			soft = append(soft, "nfqws2.args: "+err.Error())
		}
	}
	_, _ = deploy.ReadVersion(a.Paths) // version.env опционален

	if len(soft) > 0 {
		detail := "warn: " + strings.Join(soft, "; ")
		// Не ok — UI не должен показывать зелёный успех; rollback не делаем.
		a.WritePostUpdateVerifyStatus("warn", detail)
		a.WriteServiceWarnings(soft)
		return nil
	}
	a.WritePostUpdateVerify(true, "ok")
	return nil
}

// RollbackPreUpdateBackup: smart restore архива + откат bin/zator из снимка.
func (a *App) RollbackPreUpdateBackup() error {
	name := a.LastPreUpdateBackup()
	if name == "" {
		return fmt.Errorf("нет pre-update backup для отката")
	}
	mgr := backup.NewIn(a.Paths, a.Paths.BackupDir)
	if _, err := mgr.Restore(name, backup.RestoreOptions{Mode: backup.ModeSmart}); err != nil {
		return fmt.Errorf("rollback %s: %w", name, err)
	}
	binMsg := ""
	if err := a.restorePreUpdateBinary(); err != nil {
		binMsg = "; bin: " + err.Error()
	}
	a.ClearBlockFullRestore()
	if err := a.VerifyAfterUpdate(); err != nil {
		a.WriteServiceWarnings([]string{"rollback выполнен, verify всё ещё fail: " + err.Error() + binMsg})
		return err
	}
	a.WriteServiceWarnings([]string{"выполнен smart-откат из pre-update backup " + name + binMsg})
	return nil
}
