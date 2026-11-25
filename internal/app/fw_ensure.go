package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/platform"
	"github.com/AloofLibra/zator/internal/service"
)

const (
	fwEnsureStateFile = "fw_ensure.txt"
	keeneticHookPath  = "/opt/etc/ndm/netfilter.d/000-zapret2.sh"
)

// FwHookPresent — установлен ли Keenetic/Merlin fw-хук или доступен init restart-fw.
func (a *App) FwHookPresent() bool {
	if _, err := os.Stat(keeneticHookPath); err == nil {
		return true
	}
	switch service.DetectHook(a.Platform) {
	case service.HookKeenetic, service.HookMerlin:
		return true
	}
	return a.resolveZapretInit() != ""
}

// ArgsPresent — есть ли непустой nfqws2.args.
func (a *App) ArgsPresent() bool {
	st, err := os.Stat(filepath.Join(a.Paths.ZapretRoot, "nfqws2.args"))
	return err == nil && st.Size() > 0
}

// FwEnsureStatus читает итог последнего EnsureFirewallApplied: "ok"|"skip"|"fail"|"".
func (a *App) FwEnsureStatus() (status, detail string) {
	b, err := os.ReadFile(filepath.Join(a.Paths.StateDir, fwEnsureStateFile))
	if err != nil {
		return "", ""
	}
	lines := splitTwo(string(b))
	return lines[0], lines[1]
}

func (a *App) writeFwEnsure(status, detail string) {
	_ = os.MkdirAll(a.Paths.StateDir, 0o755)
	_ = fsutil.AtomicWriteFile(filepath.Join(a.Paths.StateDir, fwEnsureStateFile),
		[]byte(status+"\n"+detail+"\n"), 0o644)
}

func (a *App) resolveZapretInit() string {
	cands := []string{
		filepath.Join(a.Paths.ZapretRoot, "init.d", "sysv", "zapret2"),
		"/opt/etc/init.d/S90-zapret2",
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// firewallRulesPresent — дешёвая проверка: цепочка/правила NFQUEUE уже на месте.
// Если да — повторный restart-fw не нужен (ndm/watchdog не штормят).
func (a *App) firewallRulesPresent(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// ZATOR_NFQ / Z2R_WS_MARK — наши цепочки; иначе любой NFQUEUE в mangle
	// (live qnum бывает 200 или 300 — не завязываемся на одно число).
	for _, args := range [][]string{
		{"-w", "-t", "mangle", "-L", "ZATOR_NFQ", "-n"},
		{"-w", "-t", "mangle", "-L", "Z2R_WS_MARK", "-n"},
	} {
		cmd := exec.CommandContext(cctx, "iptables", args...)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	cmd := exec.CommandContext(cctx, "iptables", "-w", "-t", "mangle", "-S")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "NFQUEUE")
}

// EnsureFirewallApplied вызывает init restart-fw на Entware/Keenetic (best-effort).
// На других платформах — skip. Если правила уже есть и недавний ok — no-op.
func (a *App) EnsureFirewallApplied(ctx context.Context) error {
	if a.Platform.InitSystem != platform.InitEntware && a.Platform.Kind != platform.KindEntware &&
		service.DetectHook(a.Platform) == service.HookNone {
		a.writeFwEnsure("skip", "not entware/keenetic")
		return nil
	}
	init := a.resolveZapretInit()
	if init == "" {
		a.writeFwEnsure("skip", "нет init-скрипта zapret2")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Не дёргать restart-fw, если правила уже на месте.
	if a.firewallRulesPresent(ctx) {
		if st, _ := a.FwEnsureStatus(); st != "ok" {
			a.writeFwEnsure("ok", "rules already present")
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, init, "restart-fw")
	out, err := cmd.CombinedOutput()
	detail := string(out)
	if len(detail) > 200 {
		detail = detail[:200]
	}
	if err != nil {
		msg := fmt.Sprintf("%s restart-fw: %v %s", init, err, detail)
		a.writeFwEnsure("fail", msg)
		return fmt.Errorf("fw ensure: %w", err)
	}
	a.writeFwEnsure("ok", init)
	return nil
}

func splitTwo(s string) [2]string {
	var out [2]string
	i := 0
	start := 0
	for n := 0; n < len(s) && i < 2; n++ {
		if s[n] == '\n' {
			out[i] = trimSpaceASCII(s[start:n])
			i++
			start = n + 1
		}
	}
	if i < 2 && start < len(s) {
		out[i] = trimSpaceASCII(s[start:])
	}
	return out
}

func trimSpaceASCII(s string) string {
	return strings.TrimSpace(s)
}
