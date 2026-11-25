package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrScanUnsupported — на системе нет ни procfs, ни pidof.
var ErrScanUnsupported = errors.New("service: сканирование процессов недоступно")

// Matcher проверяет процесс по имени (comm) и аргументам командной строки.
type Matcher func(comm string, cmdline []string) bool

// MatchNfqws2 выбирает процессы live nfqws2 (не изолированный probe qnum=65535).
func MatchNfqws2(comm string, cmdline []string) bool {
	isNfq := comm == "nfqws2" || (len(cmdline) > 0 && exeBase(cmdline[0]) == "nfqws2")
	if !isNfq {
		return false
	}
	// SuperSweep/relock probe: --qnum=65535 — не считать zapret2 «работающим».
	for _, a := range cmdline {
		if a == "--qnum=65535" || strings.HasPrefix(a, "--qnum=65535") {
			return false
		}
	}
	return true
}

// MatchWebUI выбирает процессы «zator webui|serve».
func MatchWebUI(comm string, cmdline []string) bool {
	if comm == "zator-webui" {
		return true
	}
	if len(cmdline) < 2 {
		return false
	}
	switch exeBase(cmdline[0]) {
	case "zator", "zator-webui":
	default:
		return false
	}
	for _, a := range cmdline[1:] {
		if a == "webui" || a == "serve" {
			return true
		}
	}
	return false
}

// MatchValidator выбирает strategy-validator.sh --daemon.
func MatchValidator(comm string, cmdline []string) bool {
	base := exeBase(comm)
	if strings.Contains(base, "strategy-validator") {
		return true
	}
	for _, a := range cmdline {
		if strings.Contains(a, "strategy-validator") {
			return true
		}
	}
	return false
}

func exeBase(p string) string {
	// Унифицируем разделители, чтобы MatchWebUI видел `C:\zator.exe` и на Linux.
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.TrimSuffix(strings.ToLower(filepath.Base(p)), ".exe")
	return p
}

// Scanner ищет процессы через procfs, а при её отсутствии — через pidof.
type Scanner struct {
	ProcRoot string // по умолчанию /proc
	Runner   Runner // для pidof; по умолчанию os/exec
}

// DefaultScanner — сканер по умолчанию.
func DefaultScanner() Scanner { return Scanner{ProcRoot: "/proc", Runner: ExecRunner()} }

// Find возвращает PID процессов, удовлетворяющих match. pidofName —
// имя для запасного пути через pidof.
func (s Scanner) Find(ctx context.Context, match Matcher, pidofName string) ([]int, error) {
	root := s.ProcRoot
	if root == "" {
		root = "/proc"
	}
	entries, err := os.ReadDir(root)
	if err == nil && hasPIDDirs(entries) {
		return scanProc(root, entries, match), nil
	}
	// procfs отсутствует либо пуст (например, тестовый каталог без процессов).
	if err == nil && !isRealProc(root) {
		return nil, nil
	}
	return s.pidof(ctx, pidofName)
}

// isRealProc отличает настоящий procfs (есть self) от произвольного каталога.
func isRealProc(root string) bool {
	_, err := os.Stat(filepath.Join(root, "self"))
	return err == nil
}

func hasPIDDirs(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() && isPID(e.Name()) {
			return true
		}
	}
	return false
}

func isPID(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

func scanProc(root string, entries []os.DirEntry, match Matcher) []int {
	var pids []int
	for _, e := range entries {
		if !e.IsDir() || !isPID(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		comm := ""
		if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			comm = strings.TrimSpace(string(b))
		}
		var cmdline []string
		if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, part := range strings.Split(strings.TrimRight(string(b), "\x00"), "\x00") {
				if part != "" {
					cmdline = append(cmdline, part)
				}
			}
		}
		if match(comm, cmdline) {
			pid, _ := strconv.Atoi(e.Name())
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	return pids
}

func (s Scanner) pidof(ctx context.Context, name string) ([]int, error) {
	if name == "" {
		return nil, ErrScanUnsupported
	}
	r := s.Runner
	if r == nil {
		r = ExecRunner()
	}
	out, err := r.Run(ctx, "pidof", name)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// pidof завершается кодом 1, если процессов нет.
			return nil, nil
		}
		return nil, ErrScanUnsupported
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(f); err == nil && n > 0 {
			pids = append(pids, n)
		}
	}
	sort.Ints(pids)
	return pids, nil
}
