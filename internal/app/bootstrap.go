package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/firewall"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/service"
)

// BootstrapOptions — автозапуск служб после установки.
type BootstrapOptions struct {
	// SkipService не ставит и не запускает службы.
	SkipService bool
	// StartWebUI ставит и запускает zator-webui (по умолчанию true).
	StartWebUI *bool
}

func (o BootstrapOptions) webUI() bool {
	if o.StartWebUI == nil {
		return true
	}
	return *o.StartWebUI
}

// InstallSelfBinary копирует текущий исполняемый файл в BinDir/zator.
func (a *App) InstallSelfBinary() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("путь к бинарнику: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	dst := filepath.Join(a.Paths.BinDir, "zator")
	if sameFile(exe, dst) {
		return nil
	}
	if err := a.Paths.EnsureDirs(); err != nil {
		return err
	}
	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Windows / busy FS: попробовать перезапись
		_ = os.Remove(dst)
		if err2 := os.Rename(tmp, dst); err2 != nil {
			os.Remove(tmp)
			return err2
		}
	}
	_ = os.Chmod(dst, 0o755)
	return nil
}

func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// PrepareZapretRuntime готовит config → nfqws2.args и firewall-хук.
// Делегирует пакету A (единый Apply: migrate WSSIZE/profile2 + args + custom.d).
func (a *App) PrepareZapretRuntime() (fwHook string, err error) {
	return a.ApplyZapretRuntime()
}

// SyncCustomDHooks пишет wssize + client-scope хуки в init.d/*/custom.d (sysv и openwrt).
func (a *App) SyncCustomDHooks(cfg *config.Config) error {
	if cfg == nil {
		var err error
		cfg, err = config.Load(a.Paths.ZapretConfig)
		if err != nil {
			return err
		}
	}
	root := a.Paths.ZatorRoot
	wcfg, err := firewall.WssizeConfigFromGetter(cfg.Get, root)
	if err != nil {
		return err
	}
	wscript, err := firewall.RenderWssizeHook(wcfg, root)
	if err != nil {
		return err
	}
	ccfg, err := firewall.ClientScopeConfigFromGetter(cfg.Get, root)
	if err != nil {
		return err
	}
	cscript, err := firewall.RenderClientScopeHook(ccfg)
	if err != nil {
		return err
	}
	for _, flavor := range []firewall.CustomDFlavor{firewall.CustomDSysV, firewall.CustomDOpenWrt} {
		if _, err := firewall.WriteCustomD(a.Paths.ZapretRoot, flavor, firewall.CustomDWssizeName, wscript); err != nil {
			return err
		}
		if _, err := firewall.WriteCustomD(a.Paths.ZapretRoot, flavor, firewall.CustomDClientScopeName, cscript); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ensureHostlistPlaceholders() error {
	if a.Paths.ExtraStratsDir == "" || a.Paths.ListsDir == "" {
		return fmt.Errorf("app: ExtraStratsDir/ListsDir не заданы — отказ писать hostlist в cwd")
	}
	files := []string{
		filepath.Join(a.Paths.ExtraStratsDir, "TCP_RKN_list.txt"),
		filepath.Join(a.Paths.ExtraStratsDir, "TCP_Custom.txt"),
		filepath.Join(a.Paths.ExtraStratsDir, "TCP_Discord.txt"),
		filepath.Join(a.Paths.ExtraStratsDir, "TCP_YT_list.txt"),
		filepath.Join(a.Paths.ListsDir, "netrogat.txt"),
		filepath.Join(a.Paths.ListsDir, "netrogat_substrings.txt"),
		filepath.Join(a.Paths.ListsDir, "googlevideo-ipv4.txt"),
	}
	for _, p := range files {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("app: относительный hostlist-путь запрещён: %s", p)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := fsutil.AtomicWriteFile(p, []byte{}, 0o644); err != nil {
			return err
		}
	}
	return nil
}

const dnsDesyncOffMarker = "dns_desync_default_off.v1"

// migrateDNSDesyncOff один раз выключает антиспуф DNS на обновлениях со старым
// небезопасным дефолтом. Маркер в state/ не даёт повторно гасить ручное включение.
func (a *App) migrateDNSDesyncOff(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	marker := filepath.Join(a.Paths.StateDir, dnsDesyncOffMarker)
	if _, err := os.Stat(marker); err == nil {
		return false
	}
	_ = os.MkdirAll(a.Paths.StateDir, 0o755)
	changed := false
	if cfg.DNSDesyncText() == "Включен" {
		if err := cfg.SetDNSDesync(false); err == nil {
			changed = true
		}
	}
	_ = fsutil.AtomicWriteFile(marker, []byte("1\n"), 0o644)
	return changed
}

// StartServices ставит в автозагрузку и запускает zapret2 и Web UI.
// Ошибки prepare/zapret не блокируют установку Web UI: панель должна
// подняться даже при битом /opt/zapret2/config после wipe.
func (a *App) StartServices(ctx context.Context, opts BootstrapOptions) error {
	if opts.SkipService {
		return nil
	}
	if err := a.InstallSelfBinary(); err != nil {
		// не фатально на Windows-сборочной машине
		_ = err
	}
	// На Windows службы роутера недоступны — только подготовка runtime (для тестов/сборки).
	if runtime.GOOS == "windows" {
		_, _ = a.PrepareZapretRuntime()
		return nil
	}
	// Повторная страховка: зависимости + IFACE (если ставили без InstallZapret).
	if rep, err := a.EnsureDependencies(ctx); err != nil {
		_ = rep // отчёт уже мог быть напечатан в InstallZapret; здесь не блокируем Web UI
	} else {
		_, _, _ = a.ApplyDetectedIfaces()
	}
	fwHook, prepErr := a.PrepareZapretRuntime()
	mgr, err := service.New(service.Config{
		Paths:    a.Paths,
		Platform: a.Platform,
		FWHook:   fwHook,
		ZatorBin: filepath.Join(a.Paths.BinDir, "zator"),
		ArgsFile: filepath.Join(a.Paths.ZapretRoot, "nfqws2.args"),
	})
	if err != nil {
		return fmt.Errorf("службы: %w", err)
	}

	var soft, hard []string
	if prepErr != nil {
		soft = append(soft, "prepare zapret: "+prepErr.Error())
	}

	zapretUnits := []service.Unit{service.UnitZapret2, service.UnitValidator}
	if prepErr == nil {
		for _, u := range zapretUnits {
			if err := mgr.Install(ctx, u); err != nil {
				soft = append(soft, fmt.Sprintf("install %s: %v", u, err))
				continue
			}
			if err := mgr.Restart(ctx, u); err != nil {
				soft = append(soft, fmt.Sprintf("start %s: %v", u, err))
			}
		}
		time.Sleep(800 * time.Millisecond)
		if st, err := mgr.Status(ctx, service.UnitZapret2); err == nil && !st.Running {
			soft = append(soft, "nfqws2 не запущен после start (проверьте /opt/zapret2/nfqws2.args и логи)")
		} else {
			if err := a.EnsureFirewallApplied(ctx); err != nil {
				soft = append(soft, "fw ensure: "+err.Error())
			}
			a.SyncAutoLockedPersist()
			a.StartNfqwsWatchdog()
		}
	}

	if opts.webUI() {
		u := service.UnitWebUI
		if err := mgr.Install(ctx, u); err != nil {
			hard = append(hard, fmt.Sprintf("install %s: %v", u, err))
		} else if err := mgr.Restart(ctx, u); err != nil {
			hard = append(hard, fmt.Sprintf("start %s: %v", u, err))
		} else {
			time.Sleep(500 * time.Millisecond)
			st, err := mgr.Status(ctx, u)
			if err != nil || !st.Running {
				logHint := filepath.ToSlash(filepath.Join(a.Paths.StateDir, string(u)+".log"))
				hard = append(hard, "zator-webui не запущен после start (лог: "+logHint+")")
			} else {
				a.StartWebUIWatchdog()
			}
		}
	}

	if len(hard) > 0 {
		all := append(append([]string{}, hard...), soft...)
		a.WriteServiceWarnings(all)
		return fmt.Errorf("%s", joinErrors(all))
	}
	if len(soft) > 0 {
		// Web UI реально жив — zapret-проблемы не валят install (иначе launcher die).
		a.WriteServiceWarnings(soft)
		return &SoftServiceError{Msgs: soft}
	}
	a.ClearServiceWarnings()
	return nil
}

// SoftServiceError — zapret/runtime предупреждения при живом Web UI.
type SoftServiceError struct {
	Msgs []string
}

func (e *SoftServiceError) Error() string { return joinErrors(e.Msgs) }
func (e *SoftServiceError) Soft() bool    { return true }

func joinErrors(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}

// StopAndUninstallServices останавливает и снимает unit-файлы zator.
func (a *App) StopAndUninstallServices() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	mgr, err := service.New(service.Config{
		Paths:    a.Paths,
		Platform: a.Platform,
		ZatorBin: filepath.Join(a.Paths.BinDir, "zator"),
		ArgsFile: filepath.Join(a.Paths.ZapretRoot, "nfqws2.args"),
	})
	if err != nil {
		return err
	}
	a.StopNfqwsWatchdog()
	var errs []string
	for _, u := range service.Units() {
		_ = mgr.Stop(ctx, u)
		if err := mgr.Uninstall(ctx, u); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", u, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", joinErrors(errs))
	}
	return nil
}
