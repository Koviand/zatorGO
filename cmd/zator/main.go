// Команда zator — установка, обновление и управление zapret2 через Web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AloofLibra/zator"
	"github.com/AloofLibra/zator/internal/app"
	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/deploy"
	"github.com/AloofLibra/zator/internal/service"
	"github.com/AloofLibra/zator/internal/webapi"
)

const usage = `zator — управление zapret2 через Web UI

Использование:
  zator [команда] [флаги]

Команды:
  version     показать версию
  status      показать состояние установки и платформу
  install     установить zator (каталоги, Web UI, конфигурация)
  update      обновить ресурсы и бинарник с GitHub (-force — всегда скачать)
  uninstall   удалить runtime (-y); --purge сносит весь каталог
  auth        пароль и включение/отключение проверки доступа
  webui       запустить Web UI (порт по умолчанию 17682)
  serve       синоним webui
  help        эта справка
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return cmdDefault(stdout)
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "zator %s\n", zator.Version())
		return 0
	case "status":
		return cmdStatus(stdout, stderr)
	case "install":
		return cmdInstall(rest, stdout, stderr)
	case "update":
		return cmdUpdate(rest, stdout, stderr)
	case "uninstall":
		return cmdUninstall(rest, stdout, stderr)
	case "auth":
		return cmdAuth(rest, stdout, stderr)
	case "webui", "serve":
		return cmdServe(cmd, rest, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "неизвестная команда %q\n\n%s", cmd, usage)
		return 2
	}
}

func cmdDefault(stdout io.Writer) int {
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stdout, "zator %s: ошибка конфигурации: %v\n", zator.Version(), err)
		return 1
	}
	if a.Installed() {
		fmt.Fprintf(stdout, "zator %s установлен в %s\n", zator.Version(), a.Paths.ZatorRoot)
		fmt.Fprintf(stdout, "Web UI: %s\n", a.WebUIURL(""))
		fmt.Fprintln(stdout, "Подробнее: zator status, запуск: zator webui")
		return 0
	}
	fmt.Fprintf(stdout, "zator %s не установлен.\n", zator.Version())
	fmt.Fprintln(stdout, "Установите командой: zator install")
	return 0
}

func cmdStatus(stdout, stderr io.Writer) int {
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Версия:      %s\n", zator.Version())
	fmt.Fprintf(stdout, "Установлен:  %t\n", a.Installed())
	fmt.Fprintf(stdout, "Платформа:   %s\n", a.Platform)
	fmt.Fprintf(stdout, "ZATOR_ROOT:  %s\n", a.Paths.ZatorRoot)
	fmt.Fprintf(stdout, "ZAPRET2:     %s\n", a.Paths.ZapretRoot)
	fmt.Fprintf(stdout, "Конфиг:      %s\n", a.ConfigPath)
	fmt.Fprintf(stdout, "Web UI:      http://localhost:%d/\n", a.Config.Port)
	fmt.Fprintf(stdout, "Auth:        enabled=%t password=%t\n", a.Config.AuthEnabled, a.Config.HasPassword())
	fmt.Fprintf(stdout, "Источник:    %s\n", a.Config.ZapretSource)
	return 0
}

func cmdInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 0, "порт Web UI")
	mirror := fs.String("mirror", "", "зеркало для загрузок")
	source := fs.String("source", "", "источник zapret: official|fork")
	withZapret := fs.Bool("zapret", true, "скачать и установить zapret2")
	zapretVer := fs.String("zapret-version", "", "версия zapret2 (пусто = latest с GitHub)")
	skipService := fs.Bool("skip-service", false, "не ставить systemd/procd службу")
	password := fs.String("password", "", "пароль Web UI (неинтерактивно; виден в ps — лучше ZATOR_PASSWORD)")
	noAuth := fs.Bool("no-auth", false, "сознательно отключить проверку доступа (нужен -host или -listen-all)")
	hostAck := fs.String("host", "", "при -no-auth: явный bind-хост (напр. 127.0.0.1) как подтверждение риска")
	listenAll := fs.Bool("listen-all", false, "при -no-auth: явное согласие слушать все интерфейсы")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	passFlag := strings.TrimSpace(*password)
	if passFlag == "" {
		passFlag = strings.TrimSpace(os.Getenv("ZATOR_PASSWORD"))
	}
	if passFlag != "" && *noAuth {
		fmt.Fprintln(stderr, "нельзя одновременно -password/ZATOR_PASSWORD и -no-auth")
		return 2
	}
	if *noAuth && strings.TrimSpace(*hostAck) == "" && !*listenAll {
		fmt.Fprintln(stderr, "-no-auth требует -host 127.0.0.1 (или другой) либо явный -listen-all")
		return 2
	}
	if *password != "" {
		fmt.Fprintln(stderr, "предупреждение: -password виден в списке процессов (ps); предпочтительнее ZATOR_PASSWORD")
	}
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	if *port != 0 {
		a.Config.Port = *port
	}
	if *mirror != "" {
		a.Config.Mirror = *mirror
	}
	if *source != "" {
		a.Config.ZapretSource = *source
	}
	if err := a.Config.Validate(); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	switch {
	case *noAuth:
		a.DisableAuthExplicitly()
		if *listenAll || strings.TrimSpace(*hostAck) == "" || strings.TrimSpace(*hostAck) == "0.0.0.0" {
			fmt.Fprintln(stderr, "внимание: проверка доступа отключена (-no-auth); для bind-all укажите zator webui -listen-all")
		} else {
			fmt.Fprintf(stderr, "внимание: проверка доступа отключена (-no-auth); при запуске укажите -host %s\n", strings.TrimSpace(*hostAck))
		}
	case passFlag != "":
		if err := a.SetPassword(passFlag); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	case a.Config.HasPassword():
		// Повторный install / refresh ассетов (в т.ч. из update) — сохраняем auth.
	case a.ConfigExists() && !a.Config.AuthEnabled:
		// Сознательный -no-auth / auth disable в существующем zator.json.
		// Наличие только bin/zator без конфига сюда не попадает — пароль обязателен.
	default:
		pass, err := promptPassword(stderr, "Пароль Web UI: ", "Повтор пароля: ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			fmt.Fprintln(stderr, "подсказка: zator install -password '…' / ZATOR_PASSWORD=… или zator install -no-auth -listen-all")
			return 1
		}
		if err := a.SetPassword(pass); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := a.Install(); err != nil {
		fmt.Fprintf(stderr, "установка не удалась: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "zator установлен в %s\n", a.Paths.ZatorRoot)
	if err := a.InstallSelfBinary(); err != nil {
		fmt.Fprintf(stderr, "предупреждение: бинарник не скопирован: %v\n", err)
	}
	if *withZapret {
		fmt.Fprintln(stdout, "загрузка zapret2 (latest)...")
		zver := *zapretVer
		if zver == "" {
			zver = "latest"
		}
		res, err := a.InstallZapret(context.Background(), app.InstallZapretOptions{
			SkipService: *skipService,
			Version:     zver,
			Log:         stdout,
		})
		if err != nil {
			var soft *app.SoftServiceError
			if errors.As(err, &soft) {
				fmt.Fprintf(stderr, "предупреждение zapret2/службы: %v\n", soft)
				fmt.Fprintln(stderr, "Службы zapret2 могут быть не запущены — смотрите Web UI → Обзор.")
			} else {
				fmt.Fprintf(stderr, "zapret2: %v\n", err)
				return 1
			}
		}
		if res != nil {
			skip := ""
			if res.Skipped {
				skip = " (уже установлен)"
			}
			fmt.Fprintf(stdout, "zapret2 %s (%s) → %s%s\n", res.Version, res.Source, res.Root, skip)
		}
	} else if !*skipService {
		if err := a.StartServices(context.Background(), app.BootstrapOptions{}); err != nil {
			var soft *app.SoftServiceError
			if errors.As(err, &soft) {
				fmt.Fprintf(stderr, "предупреждение служб: %v\n", soft)
				fmt.Fprintln(stderr, "Web UI поднят; zapret2 может требовать ручного start.")
			} else {
				fmt.Fprintf(stderr, "автозапуск служб: %v\n", err)
				return 1
			}
		}
	}
	fmt.Fprintf(stdout, "Web UI: %s\n", a.WebUIURL(""))
	if a.Config.AuthRequired() {
		fmt.Fprintln(stdout, "Вход: пароль, заданный при установке.")
	}
	if !*skipService {
		if warns := a.ServiceWarnings(); len(warns) > 0 {
			fmt.Fprintln(stdout, "Службы: Web UI запущен; zapret2 — с предупреждениями (см. Обзор).")
		} else {
			fmt.Fprintln(stdout, "Службы zapret2 и Web UI установлены и запущены.")
		}
	} else {
		fmt.Fprintln(stdout, "Запуск панели: zator webui")
	}
	return 0
}

func cmdUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "скачать релиз даже если версия совпадает")
	strategies := fs.Bool("strategies", false, "обновить подписанный каталог стратегий (strategies-extra.tsv)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	if !a.Installed() {
		fmt.Fprintf(stderr, "zator не установлен в %s — сначала: zator install\n", a.Paths.ZatorRoot)
		return 1
	}
	if *strategies {
		return cmdUpdateStrategies(a, stdout, stderr)
	}
	info, err := a.CreatePreUpdateBackup()
	if err != nil {
		fmt.Fprintf(stderr, "pre-update backup обязателен: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "pre-update backup: %s\n", info.Name)
	// Сначала локальные встроенные ресурсы, затем (если онлайн) релизный tar/бинарник.
	if err := a.Update(); err != nil {
		fmt.Fprintf(stderr, "обновление ресурсов не удалось: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "zator %s: встроенные ресурсы обновлены\n", zator.Version())

	u := deploy.New(a.Paths)
	off := deploy.ResolveOfflineBundle(os.Getenv)
	rollbackApply := func(cause error) int {
		fmt.Fprintf(stderr, "%v\n", cause)
		if rerr := a.RollbackPreUpdateBackup(); rerr != nil {
			fmt.Fprintf(stderr, "rollback: %v\n", rerr)
		} else {
			fmt.Fprintln(stdout, "выполнен откат из pre-update backup")
		}
		return 1
	}
	if off.Offline {
		if off.Bundle == nil {
			fmt.Fprintln(stdout, "офлайн-режим: бандл не найден (задайте ZATOR_BUNDLE_DIR или положите vendor/+zator-*.tar.gz в cwd при Z2R_OFFLINE=1)")
			return 0
		}
		fmt.Fprintf(stdout, "офлайн-бандл: %s\n", off.Bundle.Tarball)
		ar, err := u.ApplyBundle(*off.Bundle, "")
		if err != nil {
			return rollbackApply(fmt.Errorf("офлайн ApplyBundle: %w", err))
		}
		if ar != nil {
			fmt.Fprintf(stdout, "офлайн: установлено файлов %d\n", ar.Installed)
		}
		if err := a.VerifyAfterUpdate(); err != nil {
			return rollbackApply(fmt.Errorf("post-update verify: %w", err))
		}
		postUpdateRestartServices(a, stdout, stderr)
		return 0
	}
	rep, err := u.Run(context.Background(), deploy.RunOptions{
		Variant: deploy.VariantFull,
		Force:   *force,
	})
	if err != nil {
		return rollbackApply(fmt.Errorf("проверка/применение релиза: %w", err))
	}
	if rep != nil && rep.UpToDate {
		fmt.Fprintln(stdout, "релизы: уже актуальная версия (для принудительного скачивания: zator update -force)")
		return 0
	}
	if rep != nil && rep.Apply != nil {
		fmt.Fprintf(stdout, "релиз применён: установлено файлов %d", rep.Apply.Installed)
		if rep.Archive != "" {
			fmt.Fprintf(stdout, " (%s)", rep.Archive)
		}
		fmt.Fprintln(stdout)
		if err := a.VerifyAfterUpdate(); err != nil {
			return rollbackApply(fmt.Errorf("post-update verify: %w", err))
		}
		fmt.Fprintln(stdout, "post-update verify: ok")
		postUpdateRestartServices(a, stdout, stderr)
	}
	return 0
}

// postUpdateRestartServices — как Web schedulePostUpdateRestarts: args + рестарт zapret2/webui.
func postUpdateRestartServices(a *app.App, stdout, stderr io.Writer) {
	if _, err := a.PrepareZapretRuntime(); err != nil {
		fmt.Fprintf(stderr, "предупреждение: prepare после update: %v\n", err)
	}
	mgr, err := service.New(service.Config{
		Paths:    a.Paths,
		Platform: a.Platform,
		ZatorBin: filepath.Join(a.Paths.BinDir, "zator"),
		ArgsFile: filepath.Join(a.Paths.ZapretRoot, "nfqws2.args"),
	})
	if err != nil {
		fmt.Fprintf(stderr, "предупреждение: службы после update: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := a.RestartZapretTransactional(ctx, func(ctx context.Context) error {
		return mgr.Restart(ctx, service.UnitZapret2)
	}); err != nil {
		fmt.Fprintf(stderr, "предупреждение: рестарт zapret2: %v\n", err)
	} else {
		fmt.Fprintln(stdout, "zapret2 перезапущен")
	}
	_ = mgr.Restart(ctx, service.UnitValidator)
	if err := mgr.Restart(ctx, service.UnitWebUI); err != nil {
		fmt.Fprintf(stderr, "предупреждение: рестарт webui: %v\n", err)
	} else {
		fmt.Fprintln(stdout, "webui перезапущен (если служба была установлена)")
	}
}

// cmdUpdateStrategies — тот же путь, что Web UI setting=strategies_feed:
// verify Ed25519 → state/strategies-extra.tsv → EnsureProfileStrategy (dedupe) → конфиг.
func cmdUpdateStrategies(a *app.App, stdout, stderr io.Writer) int {
	u := deploy.New(a.Paths)
	res, err := u.ApplyStrategiesFeed(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "каталог стратегий: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "каталог стратегий: tag=%s строк=%d → %s\n", res.Tag, res.Applied, res.Path)

	cfg, err := config.Load(a.Paths.ZapretConfig)
	if err != nil {
		fmt.Fprintf(stderr, "конфиг zapret2: %v\n", err)
		return 1
	}
	applied := cfg.ApplyStrategiesFeedLines(res.Strategies)
	if applied.Registered > 0 {
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(stderr, "сохранение конфига: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "регистрация: новых=%d пропущено(dedupe)=%d\n", applied.Registered, applied.Skipped)
	for _, w := range applied.Warnings {
		fmt.Fprintf(stderr, "предупреждение: %s\n", w)
	}
	if applied.Registered > 0 {
		fmt.Fprintln(stdout, "нужен restart zapret2 (Web UI → Обзор или служба), чтобы datapath подхватил стратегии")
	}
	return 0
}

func cmdUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("y", false, "не спрашивать подтверждение")
	purge := fs.Bool("purge", false, "удалить весь каталог zator (config/state/lists)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	a := app.New()
	if !a.Installed() {
		fmt.Fprintln(stdout, "zator не установлен")
		return 0
	}
	if !*yes {
		if *purge {
			fmt.Fprintf(stderr, "будет полностью удалён %s (включая config/state/lists); повторите с -y --purge\n", a.Paths.ZatorRoot)
		} else {
			fmt.Fprintf(stderr, "будут удалены bin/webui/lua и службы; config/state/lists/files сохранятся в %s; повторите с -y (полный снос: -y --purge)\n", a.Paths.ZatorRoot)
		}
		return 1
	}
	if err := a.Uninstall(app.UninstallOptions{Purge: *purge}); err != nil {
		fmt.Fprintf(stderr, "удаление не удалось: %v\n", err)
		return 1
	}
	if *purge {
		fmt.Fprintln(stdout, "zator полностью удалён (--purge)")
	} else {
		fmt.Fprintln(stdout, "runtime zator удалён; config/state/lists/files сохранены")
	}
	return 0
}

// newWebServer собирает Web UI API и статику. Без auth_enabled — предупреждение в stderr.
func newWebServer(a *app.App, stderr io.Writer) (*webapi.Server, error) {
	if !a.ConfigExists() {
		return nil, fmt.Errorf("нет %s: сначала выполните zator install", a.ConfigPath)
	}
	api, err := webapi.New(webapi.Options{App: a})
	if err != nil {
		return nil, err
	}
	if !a.Config.AuthRequired() {
		fmt.Fprintln(stderr, "Web UI без проверки доступа (auth_enabled=false); включите: zator auth enable / set-password")
	}
	return api, nil
}

func cmdServe(name string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 0, "порт (по умолчанию из конфигурации, 17682)")
	host := fs.String("host", "", "адрес привязки (пусто + auth = все интерфейсы; без auth нужен явный -host или -listen-all)")
	listenAll := fs.Bool("listen-all", false, "явно слушать все интерфейсы (обязательно при auth_enabled=false без -host)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	if *port != 0 {
		a.Config.Port = *port
	}
	bindHost := strings.TrimSpace(*host)
	if !a.Config.AuthRequired() {
		if bindHost == "" && !*listenAll {
			fmt.Fprintln(stderr, "auth отключён: укажите -host 127.0.0.1 (или другой) либо явный -listen-all")
			return 2
		}
		if *listenAll && bindHost == "" {
			fmt.Fprintln(stderr, "ВНИМАНИЕ: Web UI без проверки доступа слушает ВСЕ интерфейсы (-listen-all); ограничьте firewall")
		} else if bindHost != "" {
			fmt.Fprintf(stderr, "внимание: Web UI без проверки доступа на %s\n", bindHost)
		}
	} else if bindHost == "" {
		fmt.Fprintf(stderr, "Web UI с auth слушает все интерфейсы (:%d); ограничьте доступ firewall / reverse proxy\n", a.Config.Port)
	}
	// Восстановить стоковые blob, если каталог пуст (ручная замена бинарника / старый uninstall).
	if err := a.EnsureStockFakes(); err != nil {
		fmt.Fprintf(stderr, "предупреждение files/fake: %v\n", err)
	}

	api, err := newWebServer(a, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "ошибка инициализации Web UI: %v\n", err)
		return 1
	}

	addr := net.JoinHostPort(bindHost, strconv.Itoa(a.Config.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Тело: формы до 4 МиБ, бэкап до 32 МиБ; долгие update/restart без WriteTimeout.
		ReadTimeout: 5 * time.Minute,
		IdleTimeout: 2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.StartGooglevideoCollection(ctx); err != nil {
		fmt.Fprintf(stderr, "предупреждение googlevideo-сбор: %v\n", err)
	}
	defer a.StopGooglevideoCollection()
	api.StartBackground(ctx)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	fmt.Fprintf(stdout, "Web UI: %s\n", a.WebUIURL(""))

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "ошибка сервера: %v\n", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}
	return 0
}
