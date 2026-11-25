package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AloofLibra/zator/internal/fsutil"
)

// ---------------------------------------------------------------- вспомогательное

func (m *manager) writeFile(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	// WriteIfChanged: update/apply не долбят флеш, если хук/init уже актуальные.
	if err := fsutil.WriteIfChanged(path, []byte(content), mode); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

// exitCode разделяет «команда вернула ненулевой код» и «команду не удалось запустить».
func queryResult(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return false, err
}

func (m *manager) queryCmd(ctx context.Context, name string, args ...string) (bool, error) {
	_, err := m.cfg.Runner.Run(ctx, name, args...)
	return queryResult(err)
}

// ---------------------------------------------------------------- systemd

type systemdBackend struct{ m *manager }

func (b systemdBackend) unitFile(u Unit) string {
	return b.m.sys(systemdDir + "/" + string(u) + ".service")
}

func (b systemdBackend) install(ctx context.Context, u Unit) error {
	content, err := RenderSystemd(b.m.templateData(u))
	if err != nil {
		return err
	}
	if err := b.m.writeFile(b.unitFile(u), content, 0o644); err != nil {
		return err
	}
	if err := b.m.run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return b.m.run(ctx, "systemctl", "enable", string(u)+".service")
}

func (b systemdBackend) uninstall(ctx context.Context, u Unit) error {
	_ = b.m.run(ctx, "systemctl", "disable", "--now", string(u)+".service")
	if err := os.Remove(b.unitFile(u)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: %w", err)
	}
	_ = b.m.run(ctx, "systemctl", "daemon-reload")
	return nil
}

func (b systemdBackend) control(ctx context.Context, u Unit, action string) error {
	return b.m.run(ctx, "systemctl", action, string(u)+".service")
}

func (b systemdBackend) query(ctx context.Context, u Unit) (bool, error) {
	return b.m.queryCmd(ctx, "systemctl", "is-active", "--quiet", string(u)+".service")
}

// ---------------------------------------------------------------- procd (OpenWrt)

type procdBackend struct{ m *manager }

func (b procdBackend) script(u Unit) string { return initDir + "/" + string(u) }

func (b procdBackend) install(ctx context.Context, u Unit) error {
	content, err := RenderProcd(b.m.templateData(u))
	if err != nil {
		return err
	}
	if err := b.m.writeFile(b.m.sys(b.script(u)), content, 0o755); err != nil {
		return err
	}
	return b.m.run(ctx, b.script(u), "enable")
}

func (b procdBackend) uninstall(ctx context.Context, u Unit) error {
	_ = b.m.run(ctx, b.script(u), "stop")
	_ = b.m.run(ctx, b.script(u), "disable")
	if err := os.Remove(b.m.sys(b.script(u))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func (b procdBackend) control(ctx context.Context, u Unit, action string) error {
	return b.m.run(ctx, b.script(u), action)
}

func (b procdBackend) query(ctx context.Context, u Unit) (bool, error) {
	return b.m.queryCmd(ctx, b.script(u), "running")
}

// ---------------------------------------------------------------- SysV

type sysvBackend struct{ m *manager }

func (b sysvBackend) script(u Unit) string { return initDir + "/" + string(u) }

func (b sysvBackend) install(ctx context.Context, u Unit) error {
	content, err := RenderSysV(b.m.templateData(u))
	if err != nil {
		return err
	}
	if err := b.m.writeFile(b.m.sys(b.script(u)), content, 0o755); err != nil {
		return err
	}
	// Включение автозапуска зависит от дистрибутива: пробуем доступные
	// утилиты, ошибки не фатальны.
	if err := b.m.run(ctx, "update-rc.d", string(u), "defaults"); err != nil {
		_ = b.m.run(ctx, "chkconfig", "--add", string(u))
	}
	return nil
}

func (b sysvBackend) uninstall(ctx context.Context, u Unit) error {
	_ = b.m.run(ctx, b.script(u), "stop")
	if err := b.m.run(ctx, "update-rc.d", "-f", string(u), "remove"); err != nil {
		_ = b.m.run(ctx, "chkconfig", "--del", string(u))
	}
	if err := os.Remove(b.m.sys(b.script(u))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func (b sysvBackend) control(ctx context.Context, u Unit, action string) error {
	return b.m.run(ctx, b.script(u), action)
}

func (b sysvBackend) query(ctx context.Context, u Unit) (bool, error) {
	return b.m.queryCmd(ctx, b.script(u), "status")
}

// ---------------------------------------------------------------- Entware

type entwareBackend struct{ sysvBackend }

// initScriptFor возвращает путь init-скрипта, который вызывается для службы
// (для zapret2 — скрипт в каталоге zapret2, на который указывает S90-симлинк).
func (m *manager) initScriptFor(u Unit) string {
	switch {
	case m.cfg.Backend == BackendEntware && u == UnitZapret2:
		return filepath.Join(m.cfg.Paths.ZapretRoot, "init.d", "sysv", "zapret2")
	case m.cfg.Backend == BackendEntware:
		return entwareInitDir + "/S92" + string(u)
	}
	return initDir + "/" + string(u)
}

func (b entwareBackend) s90Link() string {
	return b.m.sys(entwareInitDir + "/S90-" + string(UnitZapret2))
}

func (b entwareBackend) merlinFile() string { return b.m.sys(merlinHookFile) }
func (b entwareBackend) keeneticHook() string {
	return b.m.sys(keeneticHookDir + "/" + KeeneticHookName)
}

func (b entwareBackend) install(ctx context.Context, u Unit) error {
	d := b.m.templateData(u)
	script := b.m.initScriptFor(u)
	d.InitScript = filepath.ToSlash(script)
	scriptOnDisk := script
	if u != UnitZapret2 {
		scriptOnDisk = b.m.sys(script)
	}
	// zapret2: не затирать апстримный/Entware init (deployEntwareInit уже положил
	// оверрайд). RenderSysV — только запасной вариант, если скрипта ещё нет.
	if u == UnitZapret2 {
		if _, err := os.Stat(scriptOnDisk); err != nil {
			content, err := RenderSysV(d)
			if err != nil {
				return err
			}
			if err := b.m.writeFile(scriptOnDisk, content, 0o755); err != nil {
				return err
			}
		} else if err := os.Chmod(scriptOnDisk, 0o755); err != nil {
			return fmt.Errorf("service: %w", err)
		}
		if err := b.linkS90(scriptOnDisk); err != nil {
			return err
		}
		return b.installHook(d)
	}
	content, err := RenderSysV(d)
	if err != nil {
		return err
	}
	return b.m.writeFile(scriptOnDisk, content, 0o755)
}

// linkS90 создаёт /opt/etc/init.d/S90-zapret2 → init-скрипт (копия, если
// символические ссылки недоступны).
func (b entwareBackend) linkS90(target string) error {
	link := b.s90Link()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	_ = os.Remove(link)
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return b.m.writeFile(link, string(data), 0o755)
}

func (b entwareBackend) installHook(d TemplateData) error {
	switch b.m.cfg.Hook {
	case HookKeenetic:
		content, err := RenderKeeneticHook(d)
		if err != nil {
			return err
		}
		return b.m.writeFile(b.keeneticHook(), content, 0o755)
	case HookMerlin:
		path := b.merlinFile()
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("service: %w", err)
		}
		updated := AddHookLine(string(old), MerlinHookLine(d.InitScript))
		return b.m.writeFile(path, updated, 0o755)
	}
	return nil
}

func (b entwareBackend) removeHook() error {
	switch b.m.cfg.Hook {
	case HookKeenetic:
		if err := os.Remove(b.keeneticHook()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("service: %w", err)
		}
	case HookMerlin:
		path := b.merlinFile()
		old, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("service: %w", err)
		}
		line := MerlinHookLine(filepath.ToSlash(b.m.initScriptFor(UnitZapret2)))
		return b.m.writeFile(path, RemoveHookLine(string(old), line), 0o755)
	}
	return nil
}

func (b entwareBackend) uninstall(ctx context.Context, u Unit) error {
	script := b.m.initScriptFor(u)
	_ = b.m.run(ctx, script, "stop")
	if u == UnitZapret2 {
		// Не удаляем upstream init в /opt/zapret2 — только S90/хуки zator.
		if err := os.Remove(b.s90Link()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("service: %w", err)
		}
		if err := b.removeHook(); err != nil {
			return err
		}
		return nil
	}
	if err := os.Remove(b.m.sys(script)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func (b entwareBackend) control(ctx context.Context, u Unit, action string) error {
	return b.m.run(ctx, filepath.ToSlash(b.m.initScriptFor(u)), action)
}

func (b entwareBackend) query(ctx context.Context, u Unit) (bool, error) {
	return b.m.queryCmd(ctx, filepath.ToSlash(b.m.initScriptFor(u)), "status")
}
