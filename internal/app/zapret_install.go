package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/AloofLibra/zator/internal/deploy"
	"github.com/AloofLibra/zator/internal/uiembed"
	"github.com/AloofLibra/zator/internal/zapret"
)

// InstallZapretOptions — параметры установки zapret2 поверх раскладки zator.
type InstallZapretOptions struct {
	Force       bool
	Version     string
	IncludeLua  bool
	SkipService bool
	Timeout     time.Duration
	// Log — куда писать ход проверки зависимостей и IFACE (обычно stdout).
	Log io.Writer
}

// InstallZapret скачивает последнюю (или указанную) сборку zapret2, раскладывает
// runtime (bin/common/init.d/lua), ставит config и поднимает службы zapret2+Web UI.
func (a *App) InstallZapret(ctx context.Context, opts InstallZapretOptions) (*zapret.Result, error) {
	if err := a.Paths.EnsureDirs(); err != nil {
		return nil, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	src, err := zapret.ParseSource(a.Config.ZapretSource)
	if err != nil {
		src = zapret.DefaultSource
	}
	force := opts.Force
	// Старые установки могли положить только binaries без init.d/common —
	// тогда nfqws2 не поднимается. Переустанавливаем runtime.
	if !force {
		fn := filepath.Join(a.Paths.ZapretRoot, "init.d", "sysv", "functions")
		if _, err := os.Stat(fn); err != nil {
			force = true
		}
	}
	zopts := zapret.Options{
		Source:     src,
		Version:    opts.Version,
		Force:      force,
		IncludeLua: true,
		MirrorBase: a.Config.Mirror,
	}
	// Офлайн-бандл (ZATOR_BUNDLE_DIR / Z2R_OFFLINE): vendor/ с архивом zapret2.
	off := deploy.ResolveOfflineBundle(os.Getenv)
	zopts.Offline = off.Offline
	if off.Bundle != nil {
		zopts.VendorDir = off.Bundle.VendorDir
	}
	log := opts.Log
	if log == nil {
		log = io.Discard
	}
	if off.Offline {
		fmt.Fprintf(log, "офлайн-режим (%s)", off.Reason)
		if off.Bundle != nil {
			fmt.Fprintf(log, ", vendor=%s", off.Bundle.VendorDir)
		}
		fmt.Fprintln(log)
	}
	// Сначала системные зависимости (iptables/ipset/модули) — иначе firewall
	// после старта служб молча не поднимется.
	fmt.Fprintln(log, "проверка зависимостей под", a.Platform.String(), "...")
	depCtx, depCancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Minute)
	rep, depErr := a.EnsureDependencies(depCtx)
	depCancel()
	WriteDepsReport(log, rep)
	if depErr != nil {
		return nil, fmt.Errorf("зависимости: %w", depErr)
	}

	res, err := zapret.Install(cctx, a.Paths, a.Platform, zopts)
	if err != nil {
		return res, err
	}
	if data, err := uiembed.ConfigDefault(); err == nil {
		_ = os.WriteFile(filepath.Join(a.Paths.ZapretRoot, "config.default"), data, 0o644)
		if err := a.seedZapretConfig(data); err != nil {
			return res, err
		}
	}
	if err := a.syncAssets(); err != nil {
		return res, fmt.Errorf("ресурсы zator: %w", err)
	}
	if err := a.deployEntwareInit(); err != nil {
		return res, err
	}

	lan, wan, ifaceErr := a.ApplyDetectedIfaces()
	if ifaceErr != nil {
		// Hard-fail IFACE только на роутерах (Entware/OpenWRT): без LAN/WAN
		// firewall бесполезен. На VPS — warning, Web UI всё равно поднимаем.
		if requireRouterIfaces(a.Platform) {
			return res, fmt.Errorf("IFACE_WAN/LAN: %w", ifaceErr)
		}
		fmt.Fprintf(log, "предупреждение IFACE: %v\n", ifaceErr)
	} else if lan != "" || wan != "" {
		fmt.Fprintf(log, "интерфейсы: IFACE_LAN=%s IFACE_WAN=%s\n", lan, wan)
	}

	if opts.SkipService {
		if _, err := a.PrepareZapretRuntime(); err != nil {
			return res, err
		}
		return res, nil
	}
	// Отдельный бюджет на службы: download мог съесть почти весь cctx.
	sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer scancel()
	if err := a.StartServices(sctx, BootstrapOptions{}); err != nil {
		return res, err
	}
	return res, nil
}

// deployEntwareInit подменяет init-скрипт zapret2 оверрайдом Keenetic/Entware
// (если он встроен в бинарник). Апстримный init.d остаётся в каталоге;
// оверрайд использует те же functions из EXEDIR.
func (a *App) deployEntwareInit() error {
	src := uiembed.Init()
	data, err := fs.ReadFile(src, "entware/zapret")
	if err != nil {
		return nil
	}
	dst := filepath.Join(a.Paths.ZapretRoot, "init.d", "sysv", "zapret2")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return fmt.Errorf("Entware init: %w", err)
	}
	_ = a.deployNfqwsWatchdog()
	return nil
}
