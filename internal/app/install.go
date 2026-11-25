package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AloofLibra/zator"
	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/deploy"
	"github.com/AloofLibra/zator/internal/uiembed"
)

// Install создаёт раскладку каталогов, распаковывает встроенные ресурсы
// и записывает текущую a.Config (пароль/auth задаёт вызывающий код).
// Вызывающий код должен при необходимости предварительно вызвать Load.
// На роутере сразу ставит системные зависимости (iptables/ipset/модули).
func (a *App) Install() error {
	if err := a.Paths.EnsureDirs(); err != nil {
		return fmt.Errorf("создание каталогов: %w", err)
	}
	if err := a.Config.Validate(); err != nil {
		return err
	}
	if err := a.syncAssets(); err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04")
	ver := zator.Version()
	_ = deploy.WriteVersion(a.Paths, deploy.Version{
		ZatorVersion: ver,
		ZatorDate:    now,
		WebUIVersion: ver,
		WebUIDate:    now,
		Tracking:     "latest",
	})
	if err := a.Save(); err != nil {
		return err
	}
	// Best-effort: ставим iptables/ipset сразу. Жёсткая проверка — в InstallZapret.
	if isRouterHost() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		_, _ = a.EnsureDependencies(ctx)
	}
	return nil
}

// Update обновляет встроенные ресурсы, сохраняя существующую конфигурацию
// (пароль, auth_enabled, legacy token не трогаются).
// На роутере повторно ставит iptables/ipset и подтягивает IFACE.
// Требует завершённый install (zator.json): один bin/zator без конфига —
// неполная установка; DefaultConfig с AuthEnabled=false писать нельзя
// (иначе повторный install воспримет это как сознательный -no-auth).
func (a *App) Update() error {
	if !a.Installed() {
		return fmt.Errorf("zator не установлен в %s", a.Paths.ZatorRoot)
	}
	if !a.ConfigExists() {
		return fmt.Errorf("zator: нет %s — сначала завершите install (пароль/auth)", a.ConfigPath)
	}
	if err := a.Paths.EnsureDirs(); err != nil {
		return err
	}
	if err := a.Load(); err != nil {
		return err
	}
	if err := a.syncAssets(); err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04")
	ver := zator.Version()
	_ = deploy.WriteVersion(a.Paths, deploy.Version{
		ZatorVersion: ver,
		ZatorDate:    now,
		WebUIVersion: ver,
		WebUIDate:    now,
		Tracking:     "latest",
	})
	if err := a.Save(); err != nil {
		return err
	}
	if isRouterHost() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		_, _ = a.EnsureDependencies(ctx)
		_, _, _ = a.ApplyDetectedIfaces()
	}
	return nil
}

// UninstallOptions — поведение удаления.
type UninstallOptions struct {
	// Purge удаляет весь ZatorRoot (включая config/state/lists).
	Purge bool
	// SkipService не трогает unit-файлы служб.
	SkipService bool
}

// Uninstall снимает runtime. Без Purge сохраняет config, state, lists, extra_strats.
// Каталог zapret2 не затрагивается.
func (a *App) Uninstall(opts UninstallOptions) error {
	root := a.Paths.ZatorRoot
	if root == "" || root == "/" || root == filepath.VolumeName(root)+string(filepath.Separator) {
		return fmt.Errorf("небезопасный корневой каталог: %q", root)
	}
	if !opts.SkipService {
		_ = a.StopAndUninstallServices()
	}
	if opts.Purge {
		return os.RemoveAll(root)
	}
	return a.removeRuntime()
}

// removeRuntime удаляет бинарник и встроенные ассеты, оставляя пользовательские данные.
// FakeDir (files/fake) сохраняем: там и стоковые blob, и загруженные пользователем.
func (a *App) removeRuntime() error {
	for _, p := range []string{
		a.Paths.BinDir,
		a.Paths.WebUIDir,
		a.Paths.LuaDir,
		a.Paths.DataDir,
		filepath.Join(a.Paths.ExtraStratsDir, "cache"),
		a.Paths.OrchestraDir,
	} {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("удаление %s: %w", p, err)
		}
	}
	return nil
}

func (a *App) syncAssets() error {
	if err := copyFS(uiembed.WebUI(), a.Paths.WebUIDir); err != nil {
		return fmt.Errorf("распаковка Web UI: %w", err)
	}
	if err := copyFS(uiembed.Lua(), a.Paths.LuaDir); err != nil {
		return fmt.Errorf("распаковка lua: %w", err)
	}
	// strategy-validator.sh должен быть исполняемым (служба zator-strategy-validator).
	_ = os.Chmod(filepath.Join(a.Paths.LuaDir, "strategy-validator.sh"), 0o755)
	if err := copyFS(uiembed.Orchestra(), filepath.Join(a.Paths.ExtraStratsDir, "cache", "orchestra")); err != nil {
		return fmt.Errorf("распаковка orchestra: %w", err)
	}
	// locked.lua должен лежать в /opt/zator/lua (пути в config.default).
	if err := copyFSKeepExisting(uiembed.Orchestra(), a.Paths.LuaDir); err != nil {
		return fmt.Errorf("locked.lua → lua: %w", err)
	}
	if err := a.migrateOrchestraCacheToState(); err != nil {
		return err
	}
	if err := copyFSKeepExisting(uiembed.Lists(), a.Paths.ListsDir); err != nil {
		return fmt.Errorf("распаковка lists: %w", err)
	}
	if err := copyFS(uiembed.Data(), a.Paths.DataDir); err != nil {
		return fmt.Errorf("распаковка data: %w", err)
	}
	if err := a.EnsureStockFakes(); err != nil {
		return err
	}
	if err := copyFSKeepExisting(uiembed.ExtraStrats(), a.Paths.ExtraStratsDir); err != nil {
		return fmt.Errorf("распаковка extra_strats: %w", err)
	}
	if data, err := uiembed.ConfigDefault(); err == nil {
		dst := filepath.Join(a.Paths.ConfigDir, "config.default")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("запись config.default: %w", err)
		}
		if err := a.seedZapretConfig(data); err != nil {
			return err
		}
	}
	return a.ensureOrchestraState()
}

// copyFSKeepExisting копирует файлы, не затирая уже существующие
// (netrogat.txt и пользовательские списки).
func copyFSKeepExisting(src fs.FS, dst string) error {
	return fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(strings.ToLower(name), ".sh") {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}

// seedZapretConfig записывает ZapretConfig из шаблона, если config отсутствует
// и каталог zapret2 существует.
func (a *App) seedZapretConfig(template []byte) error {
	cfg := a.Paths.ZapretConfig
	if cfg == "" {
		return nil
	}
	if _, err := os.Stat(cfg); err == nil {
		return nil
	}
	if st, err := os.Stat(a.Paths.ZapretRoot); err != nil || !st.IsDir() {
		return nil
	}
	if err := os.WriteFile(cfg, template, 0o644); err != nil {
		return fmt.Errorf("запись %s: %w", cfg, err)
	}
	if c, err := config.Load(cfg); err == nil {
		changed, e := a.ensureNetworkIfaces(c)
		if e != nil {
			// Windows/macOS и VPS без отдельного LAN — не валим install.
			if !requireRouterIfaces(a.Platform) {
				return nil
			}
			return fmt.Errorf("автоопределение IFACE_WAN/LAN: %w", e)
		}
		if changed {
			if err := c.Save(); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureOrchestraState создаёт каталоги и пустые файлы состояния orchestra
// (locked.tsv, locked.manual.tsv), чтобы Lua и WebUI видели их с первого запуска.
func (a *App) ensureOrchestraState() error {
	dir := a.Paths.StateOrchestraDir()
	if err := os.MkdirAll(filepath.Join(dir, "scopes"), 0o755); err != nil {
		return fmt.Errorf("каталог orchestra: %w", err)
	}
	for _, p := range []string{a.Paths.LockedTSV, a.Paths.LockedManualTSV, a.Paths.AutoLockedTSV} {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("файл состояния %s: %w", p, err)
		}
		f.Close()
	}
	return nil
}

// migrateOrchestraCacheToState переносит локи из старого пути
// extra_strats/cache/orchestra → state/orchestra (канон для Go и Lua).
func (a *App) migrateOrchestraCacheToState() error {
	oldDir := filepath.Join(a.Paths.ExtraStratsDir, "cache", "orchestra")
	newDir := a.Paths.StateOrchestraDir()
	if err := os.MkdirAll(filepath.Join(newDir, "scopes"), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "locked.lua" || strings.HasSuffix(name, ".lua") {
			continue
		}
		src := filepath.Join(oldDir, name)
		dst := filepath.Join(newDir, name)
		if e.IsDir() {
			if name != "scopes" {
				continue
			}
			if err := copyDirKeepExisting(src, dst); err != nil {
				return err
			}
			continue
		}
		// Пустой/нулевой state (от ensureOrchestraState) не блокирует перенос legacy.
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			continue // непустое state — не затираем
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyDirKeepExisting(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDirKeepExisting(s, d); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Stat(d); err == nil {
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// EnsureStockFakes распаковывает встроенные blob в FakeDir (перезаписывает сток,
// не удаляет пользовательские файлы с другими именами).
func (a *App) EnsureStockFakes() error {
	if err := os.MkdirAll(a.Paths.FakeDir, 0o755); err != nil {
		return fmt.Errorf("создание FakeDir: %w", err)
	}
	if err := copyFS(uiembed.Fake(), a.Paths.FakeDir); err != nil {
		return fmt.Errorf("распаковка files/fake: %w", err)
	}
	return nil
}

// copyFS копирует содержимое src в каталог dst.
func copyFS(src fs.FS, dst string) error {
	return fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(strings.ToLower(name), ".sh") {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}
