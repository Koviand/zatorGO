package webapi

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/service"
)

const (
	validatorHealthOK       = "ok"
	validatorHealthDegraded = "degraded"
	validatorHealthOff      = "off"

	// Недавние FAIL в очереди circular_quality считаем деградацией обхода.
	validatorFailFresh = 15 * time.Minute
)

// validatorHealth — статус strategy-validator для health JSON (BP10).
// Значения: ok|degraded|off.
//
//	off      — нет скрипта или unit не запущен;
//	ok       — unit жив, нет деградации обхода;
//	degraded — unit жив, но relock пометил профили и/или в очереди свежие FAIL
//	           (связь с BP1 без дублирования TLS-проверок).
func (s *Server) validatorHealth() string {
	if !s.validatorUnitAlive() {
		return validatorHealthOff
	}
	if s.relockHasDegraded() || validatorQueueHasFreshFail() {
		return validatorHealthDegraded
	}
	return validatorHealthOK
}

func (s *Server) validatorUnitAlive() bool {
	if s == nil || s.app == nil {
		return false
	}
	script := filepath.Join(s.paths.LuaDir, "strategy-validator.sh")
	if _, err := os.Stat(script); err != nil {
		return false
	}
	if s.svc == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, err := s.svc.Status(ctx, service.UnitValidator)
	if err != nil {
		return false
	}
	return st.Running
}

func (s *Server) relockHasDegraded() bool {
	if s == nil || s.relock == nil {
		return false
	}
	_, degraded, _, _ := s.relock.snapshot()
	for _, on := range degraded {
		if on {
			return true
		}
	}
	return false
}

func validatorQueueDir() string {
	if v := strings.TrimSpace(os.Getenv("Z2R_VALIDATION_QUEUE")); v != "" {
		return v
	}
	return "/tmp/z2r-strategy-validation"
}

// validatorQueueHasFreshFail — result.* со статусом FAIL младше validatorFailFresh.
func validatorQueueHasFreshFail() bool {
	dir := validatorQueueDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	cutoff := time.Now().Add(-validatorFailFresh)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "result.") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if resultFileIsFail(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func resultFileIsFail(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return false
	}
	// id\tSTATUS\taskey\thostkey\tstrategy
	parts := strings.Split(sc.Text(), "\t")
	if len(parts) < 2 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parts[1]), "FAIL")
}
