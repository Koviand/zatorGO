package webapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/sweep"
)

const sweepLastRunFile = "sweep_schedule_last.txt"

// startSweepScheduler — редкий schedule supersweep (выкл при hours=0).
func (s *Server) startSweepScheduler(ctx context.Context) {
	s.schedMu.Lock()
	if s.schedCancel != nil {
		s.schedCancel()
		s.schedCancel = nil
	}
	cctx, cancel := context.WithCancel(ctx)
	s.schedCancel = cancel
	s.schedMu.Unlock()
	go s.sweepScheduleLoop(cctx)
}

func (s *Server) sweepLastRunPath() string {
	return filepath.Join(s.paths.StateDir, sweepLastRunFile)
}

func (s *Server) loadSweepLastRun() time.Time {
	b, err := os.ReadFile(s.sweepLastRunPath())
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func (s *Server) saveSweepLastRun(t time.Time) {
	_ = os.MkdirAll(s.paths.StateDir, 0o755)
	_ = fsutil.WriteIfChanged(s.sweepLastRunPath(),
		[]byte(strconv.FormatInt(t.Unix(), 10)+"\n"), 0o644)
}

func (s *Server) sweepScheduleLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f := sFlags(s)
			if f.SweepScheduleHours <= 0 {
				continue
			}
			lastRun := s.loadSweepLastRun()
			if !lastRun.IsZero() && s.now().Sub(lastRun) < time.Duration(f.SweepScheduleHours)*time.Hour {
				continue
			}
			if s.sweepRunningLocked() {
				continue
			}
			// Только сломанные профили — не перетирать рабочие локи полным прогоном.
			workers := s.brokenWorkers(ctx)
			if len(workers) == 0 {
				s.saveSweepLastRun(s.now())
				log.Printf("webapi: schedule sweep: нет сломанных профилей — skip")
				continue
			}
			if err := s.queueSoftSweep(ctx, workers, "schedule"); err != nil {
				log.Printf("webapi: schedule sweep: %v", err)
				continue
			}
			s.saveSweepLastRun(s.now())
		}
	}
}

// queueSoftSweep запускает supersweep, если не busy. workers nil = все фазы.
func (s *Server) queueSoftSweep(_ context.Context, workers []string, reason string) error {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.relockActive || s.sweepRunningLocked() {
		return sweep.ErrBusy
	}
	sw, err := s.sweeperOrDefault()
	if err != nil {
		return err
	}
	sp := sweep.DefaultParams()
	sp.Scope = orchestra.ScopeDefault
	sp.Workers = workers
	if err := sw.Start(context.Background(), sp); err != nil {
		return err
	}
	log.Printf("webapi: soft-sweep started (%s) workers=%v", reason, workers)
	return nil
}

// postSweepBroken — переподобрать только сломанные профили (BP3).
func (s *Server) postSweepBroken(r *http.Request, p params) (map[string]any, error) {
	scope, err := scopeParam(p)
	if err != nil {
		return nil, err
	}
	broken := s.brokenWorkers(r.Context())
	if len(broken) == 0 {
		return map[string]any{"ok": true, "started": false, "message": "Нет сломанных профилей"}, nil
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.relockActive || s.sweepRunningLocked() {
		return nil, errCode(http.StatusConflict, CodeBusy, "Суперавтопрогон или auto-relock уже выполняется")
	}
	sw, err := s.sweeperOrDefault()
	if err != nil {
		return nil, errf(http.StatusServiceUnavailable, err.Error())
	}
	sp := sweep.DefaultParams()
	sp.Scope = scope
	sp.Workers = broken
	if v := p.Get("tls_pref"); v != "" {
		sp.TLSPref = netcheck.ParseTLSPref(v)
	}
	if err := sw.Start(context.Background(), sp); err != nil {
		if errors.Is(err, sweep.ErrBusy) {
			return nil, errCode(http.StatusConflict, CodeBusy, "Суперавтопрогон уже выполняется")
		}
		return nil, errf(http.StatusInternalServerError, err.Error())
	}
	return map[string]any{"ok": true, "started": true, "workers": broken}, nil
}

func (s *Server) brokenWorkers(ctx context.Context) []string {
	var out []string
	cfg, _ := s.readConfig()
	for _, id := range relockWatchProfiles() {
		if cfg != nil {
			if autoConflict(cfg, id) != nil || profileModeGuard(cfg, id) != nil {
				continue
			}
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		ok := profileCheckOK(s.checker.CheckProfile(cctx, id))
		cancel()
		if ok {
			continue
		}
		if w, ok := sweep.ProfileWorker(id); ok {
			out = append(out, w)
		}
	}
	return out
}

// maybeSweepOnProviderChange — хук после redetect (BP3).
// prevDisplay/newDisplay — строка провайдера до/после; brand для доп. сигнала.
func (s *Server) maybeSweepOnProviderChange(prevDisplay, _prevBrand, newDisplay, _newBrand string) {
	if !sFlags(s).SweepOnProviderChange {
		return
	}
	if strings.EqualFold(strings.TrimSpace(prevDisplay), strings.TrimSpace(newDisplay)) {
		return
	}
	go func() {
		workers := s.brokenWorkers(context.Background())
		if len(workers) == 0 {
			log.Printf("webapi: provider change: нет сломанных — sweep skip")
			return
		}
		if err := s.queueSoftSweep(context.Background(), workers, "provider_change"); err != nil {
			if !errors.Is(err, sweep.ErrBusy) {
				log.Printf("webapi: sweep on provider change: %v", err)
			}
		}
	}()
}
