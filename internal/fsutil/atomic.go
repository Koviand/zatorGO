// Package fsutil содержит небольшие файловые утилиты, общие для пакетов zator.
package fsutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultFileMode — права по умолчанию для новых файлов.
const DefaultFileMode os.FileMode = 0o644

// renameAttempts и renameDelay — повторы os.Rename: на Windows замена файла,
// который в этот момент открыт читателем, может кратковременно завершаться
// ошибкой доступа.
const (
	renameAttempts = 8
	renameDelay    = 15 * time.Millisecond
)

// WriteIfChanged атомарно пишет data только если содержимое файла отличается
// (или файла нет). Снижает износ флеша на роутерах при повторных apply.
func WriteIfChanged(path string, data []byte, perm os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return AtomicWriteFile(path, data, perm)
}

// AtomicWriteFile записывает data в path атомарно: данные пишутся во временный
// файл в том же каталоге, синхронизируются на диск и переименовываются поверх
// целевого файла. Если файл уже существует и perm == 0, сохраняются его права.
// Недостающие родительские каталоги создаются.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if perm == 0 {
		perm = DefaultFileMode
		if st, err := os.Stat(path); err == nil {
			perm = st.Mode().Perm()
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return err
	}
	if err := renameReplace(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func renameReplace(from, to string) error {
	var err error
	for i := 0; i < renameAttempts; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(renameDelay * time.Duration(i+1))
	}
	return err
}
