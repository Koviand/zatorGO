package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/firewall"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/platform"
	"github.com/AloofLibra/zator/internal/uiembed"
)

const packageAMarker = "package_a.v1"

// NfqwsWantRunningPath — флаг «пользователь хочет nfqws2 запущенным» для Entware-watchdog.
func (a *App) NfqwsWantRunningPath() string {
	return filepath.Join(a.Paths.StateDir, "nfqws2.want_running")
}

// migratePackageA приводит live-конфиг к пакету A (Keenetic-hardened):
//   - снимает legacy shell-if у WSSIZE;
//   - один раз (маркер) включает WSSIZE lite, если ещё не мигрировали;
//   - синхронизирует ARGS/SKIP;
//   - гарантирует soft_success_in=0 для circular_quality:key=2.
//
// Повторный вызов не форсит ENABLE обратно, если пользователь выключил WSSIZE.
func (a *App) migratePackageA(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	changed := false
	if cfg.StripWssizeShellIf() {
		changed = true
	}

	marker := filepath.Join(a.Paths.StateDir, packageAMarker)
	first := true
	if _, err := os.Stat(marker); err == nil {
		first = false
	}
	if first {
		_ = os.MkdirAll(a.Paths.StateDir, 0o755)
		if err := cfg.Set("Z2R_WSSIZE_RAMP_ENABLE", "1"); err == nil {
			changed = true
		}
		if err := cfg.Set("Z2R_WSSIZE_RAMP_MODE", "lite"); err == nil {
			changed = true
		}
		_ = fsutil.AtomicWriteFile(marker, []byte("1\n"), 0o644)
	}

	if ok, err := cfg.SyncWssizeRampVars(); err == nil && ok {
		changed = true
	}
	if ok, err := cfg.EnsureProfile2SoftSuccess(); err == nil && ok {
		changed = true
	}
	return changed
}

// ApplyZapretRuntime — единый pipeline: migrate → SyncWssize → Save →
// WriteNfqws2Args → SyncCustomD → queue hook. Не затирает живой config при
// битом NFQWS2_OPT (в отличие от старого wipe-fallback).
func (a *App) ApplyZapretRuntime() (fwHook string, err error) {
	cfgPath := a.Paths.ZapretConfig
	if err := os.MkdirAll(a.Paths.ZapretRoot, 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(cfgPath); err != nil {
		data, e := uiembed.ConfigDefault()
		if e != nil {
			return "", fmt.Errorf("нет %s и нет встроенного шаблона", cfgPath)
		}
		if e := a.seedZapretConfig(data); e != nil {
			return "", e
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", fmt.Errorf("чтение config: %w", err)
	}

	dirty := false
	if changed, e := a.ensureNetworkIfaces(cfg); e == nil && changed {
		dirty = true
	}
	if a.migrateDNSDesyncOff(cfg) {
		dirty = true
	}
	if a.migratePackageA(cfg) {
		dirty = true
	}
	gvLua := filepath.ToSlash(filepath.Join(a.Paths.LuaDir, "googlevideo-collect.lua"))
	if ok, e := cfg.EnsureLuaInit(gvLua); e == nil && ok {
		dirty = true
	}
	if dirty {
		if err := cfg.Save(); err != nil {
			return "", err
		}
		defPath := filepath.Join(a.Paths.ConfigDir, "config.default")
		if def, e := config.Load(defPath); e == nil {
			_ = a.migratePackageA(def)
			_ = def.Save()
		}
	}

	argsFile := filepath.Join(a.Paths.ZapretRoot, "nfqws2.args")
	if err := cfg.WriteNfqws2Args(argsFile); err != nil {
		return "", fmt.Errorf("nfqws2.args: %w (config не затёрт — исправьте NFQWS2_OPT вручную)", err)
	}

	qcfg := firewall.QueueHookFromGetter(func(k string) string { return cfg.GetDefault(k, "") })
	hook, err := firewall.WriteQueueHook(a.Paths.ZapretRoot, qcfg)
	if err != nil {
		return "", err
	}
	_ = a.SyncCustomDHooks(cfg)
	_ = a.ensureHostlistPlaceholders()
	_ = a.deployNfqwsWatchdog()
	a.SyncAutoLockedPersist()
	// Package B: подтянуть googlevideo-ipv4 (best-effort, уважает TTL).
	_, _ = a.RefreshGooglevideoIPv4(context.Background(), false)
	return hook, nil
}

// SetNfqwsWantRunning выставляет/снимает флаг для Entware-watchdog.
func (a *App) SetNfqwsWantRunning(want bool) {
	path := a.NfqwsWantRunningPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if want {
		_ = fsutil.AtomicWriteFile(path, []byte("1\n"), 0o644)
		return
	}
	_ = os.Remove(path)
}

// NfqwsWatchdogRunning — жив ли процесс watchdog (по pidfile).
func (a *App) NfqwsWatchdogRunning() bool {
	b, err := os.ReadFile("/var/run/zator-nfqws-watchdog.pid")
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(b))
	if pid == "" {
		return false
	}
	_, err = os.Stat(filepath.Join("/proc", pid))
	return err == nil
}

// deployNfqwsWatchdog кладёт скрипт watchdog в BinDir (Entware/Keenetic).
func (a *App) deployNfqwsWatchdog() error {
	if a.Platform.InitSystem != platform.InitEntware && a.Platform.Kind != platform.KindEntware {
		return nil
	}
	data, err := fs.ReadFile(uiembed.Init(), "entware/nfqws-watchdog")
	if err != nil || len(data) == 0 {
		return err
	}
	dst := filepath.Join(a.Paths.BinDir, "zator-nfqws-watchdog")
	if err := os.MkdirAll(a.Paths.BinDir, 0o755); err != nil {
		return err
	}
	if err := fsutil.WriteIfChanged(dst, data, 0o755); err != nil {
		return err
	}
	link := "/opt/etc/init.d/S91zator-nfqws-watchdog"
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	if _, err := os.Lstat(link); err != nil {
		if err := os.Symlink(dst, link); err != nil {
			_ = fsutil.WriteIfChanged(link, data, 0o755)
		}
	}
	return nil
}

// StartNfqwsWatchdog запускает watchdog после старта zapret2 (best-effort).
func (a *App) StartNfqwsWatchdog() {
	if a.Platform.InitSystem != platform.InitEntware && a.Platform.Kind != platform.KindEntware {
		return
	}
	_ = a.deployNfqwsWatchdog()
	a.SetNfqwsWantRunning(true)
	bin := filepath.Join(a.Paths.BinDir, "zator-nfqws-watchdog")
	if _, err := os.Stat(bin); err != nil {
		return
	}
	// Не блокируем bootstrap: fork через setsid делает сам скрипт.
	_ = runDetached(bin, "start")
}

// StopNfqwsWatchdog снимает want-флаг и останавливает loop.
func (a *App) StopNfqwsWatchdog() {
	a.SetNfqwsWantRunning(false)
	bin := filepath.Join(a.Paths.BinDir, "zator-nfqws-watchdog")
	if _, err := os.Stat(bin); err != nil {
		return
	}
	_ = runDetached(bin, "stop")
}

// WebUIWantRunningPath — флаг «пользователь хочет Web UI» для Entware-watchdog.
func (a *App) WebUIWantRunningPath() string {
	return filepath.Join(a.Paths.StateDir, "webui.want_running")
}

func (a *App) setWebUIWantRunning(want bool) {
	path := a.WebUIWantRunningPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if want {
		_ = fsutil.AtomicWriteFile(path, []byte("1\n"), 0o644)
		return
	}
	_ = os.Remove(path)
}

func (a *App) deployWebUIWatchdog() error {
	if a.Platform.InitSystem != platform.InitEntware && a.Platform.Kind != platform.KindEntware {
		return nil
	}
	data, err := fs.ReadFile(uiembed.Init(), "entware/webui-watchdog")
	if err != nil || len(data) == 0 {
		return err
	}
	dst := filepath.Join(a.Paths.BinDir, "zator-webui-watchdog")
	if err := os.MkdirAll(a.Paths.BinDir, 0o755); err != nil {
		return err
	}
	if err := fsutil.AtomicWriteFile(dst, data, 0o755); err != nil {
		return err
	}
	link := "/opt/etc/init.d/S93zator-webui-watchdog"
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	_ = os.Remove(link)
	if err := os.Symlink(dst, link); err != nil {
		_ = fsutil.AtomicWriteFile(link, data, 0o755)
	}
	return nil
}

// StartWebUIWatchdog поднимает respawn панели (Entware/Keenetic).
func (a *App) StartWebUIWatchdog() {
	if a.Platform.InitSystem != platform.InitEntware && a.Platform.Kind != platform.KindEntware {
		return
	}
	_ = a.deployWebUIWatchdog()
	a.setWebUIWantRunning(true)
	bin := filepath.Join(a.Paths.BinDir, "zator-webui-watchdog")
	if _, err := os.Stat(bin); err != nil {
		return
	}
	_ = runDetached(bin, "start")
}

// StopWebUIWatchdog снимает want и останавливает loop.
func (a *App) StopWebUIWatchdog() {
	a.setWebUIWantRunning(false)
	bin := filepath.Join(a.Paths.BinDir, "zator-webui-watchdog")
	if _, err := os.Stat(bin); err != nil {
		return
	}
	_ = runDetached(bin, "stop")
}

// WebUIWatchdogRunning — жив ли процесс webui-watchdog.
func (a *App) WebUIWatchdogRunning() bool {
	b, err := os.ReadFile("/var/run/zator-webui-watchdog.pid")
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(b))
	if pid == "" {
		return false
	}
	_, err = os.Stat(filepath.Join("/proc", pid))
	return err == nil
}

func runDetached(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	return cmd.Start()
}
