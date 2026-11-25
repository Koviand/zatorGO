package webapi

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/firewall"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/sweep"
)

// StrategyProber — изолированный подбор стратегии профиля (без записи в live store).
// Реализация по умолчанию: NfqwsProbe + ProbeStore; в тестах — заглушка.
type StrategyProber interface {
	// Probe перебирает candidates в изоляции. ok=false — ни одна не прошла;
	// strategy=0 при неуспехе. Ошибка инфраструктуры (probe не стартовал) ≠ «все fail».
	Probe(ctx context.Context, profile int, candidates []int) (strategy int, ok bool, err error)
}

// relockProbeDir — отдельный каталог, чтобы не пересекаться с supersweep-probe.
func (s *Server) relockProbeDir() string {
	return filepath.Join(s.paths.StateDir, "relock-probe")
}

type nfqwsStrategyProber struct {
	s *Server
}

func (s *Server) strategyProberOrDefault() StrategyProber {
	if s.prober != nil {
		return s.prober
	}
	return nfqwsStrategyProber{s: s}
}

func (p nfqwsStrategyProber) Probe(ctx context.Context, profile int, candidates []int) (int, bool, error) {
	if len(candidates) == 0 {
		return 0, false, nil
	}
	cfgLive, installed := p.s.readConfig()
	if !installed || cfgLive == nil {
		return 0, false, fmt.Errorf("конфиг zapret2 не установлен")
	}
	label, rawURL, ok := profileProbeTarget(ctx, p.s, profile)
	if !ok {
		return 0, false, fmt.Errorf("нет TLS-цели для профиля %d", profile)
	}

	workDir := p.s.relockProbeDir()
	probeStore := orchestra.NewStore(filepath.Join(workDir, "orchestra"))
	probeStore.MaxStrategy = p.s.maxStrategy

	probe, err := sweep.NewNfqwsProbe(sweep.NfqwsProbeConfig{
		Paths:        p.s.paths,
		WorkDir:      workDir,
		OrchestraDir: probeStore.Dir(),
		Config:       cfgLive,
	})
	if err != nil {
		return 0, false, err
	}
	if err := probe.Start(ctx, profile); err != nil {
		_ = probe.Stop()
		return 0, false, err
	}
	defer func() { _ = probe.Stop() }()

	opts := netcheck.DefaultOptions()
	opts.WaitBoth = true
	opts.ProbeGap = 50 * time.Millisecond
	opts.DialContext = netcheck.BindPortRangeDialer(
		firewall.SuperSweepPortFirst, firewall.SuperSweepPortLast)
	checker := netcheck.NewChecker(opts)
	if base, ok := p.s.checker.(*netcheck.Checker); ok {
		checker.RedirectorURL = base.RedirectorURL
		checker.DNS = base.DNS
	}

	env := &sweep.Env{
		Locks:   probeStore,
		Checker: checker,
		Scope:   orchestra.ScopeDefault,
		Settle:  relockSettleDefault,
		Now:     p.s.now,
	}

	for _, st := range candidates {
		if ctx.Err() != nil {
			return 0, false, ctx.Err()
		}
		if st < 1 {
			continue
		}
		// Пока probe жив — не трогаем live store.
		if !probe.Healthy(ctx) {
			return 0, false, fmt.Errorf("%w: профиль %d", sweep.ErrIsolationLost, profile)
		}
		tr, err := env.Try(ctx, sweep.TryRequest{
			Profile:  profile,
			Protos:   config.ProfileProtos(profile),
			Strategy: st,
			Label:    label,
			URL:      rawURL,
			TLSPref:  netcheck.PrefAny,
		})
		if err != nil {
			return 0, false, err
		}
		if tr.Verdict == sweep.TokenOK || tr.Verdict == sweep.TokenWarn {
			return st, true, nil
		}
	}
	return 0, false, nil
}

func profileProbeTarget(ctx context.Context, s *Server, profile int) (label, rawURL string, ok bool) {
	if c, is := s.checker.(*netcheck.Checker); is {
		t, ok := c.ProfileTarget(ctx, profile)
		return t.Label, t.URL, ok
	}
	// Fallback без *netcheck.Checker (тесты с fakeChecker).
	switch profile {
	case 1:
		return netcheck.LabelYouTube, netcheck.URLYouTube, true
	case 2:
		return netcheck.LabelGooglevideo, "https://" + netcheck.FallbackClusterDomain + "/", true
	case 3:
		return netcheck.LabelBlockedSites, netcheck.URLMeduza, true
	case 4:
		return netcheck.LabelDiscord, netcheck.URLDiscord, true
	}
	return "", "", false
}

// stubStrategyProber — для unit-тестов (без nfqws2).
type stubStrategyProber struct {
	// Result — стратегия при успехе; 0 → все fail.
	Result int
	Err    error
	Calls  int
}

func (p *stubStrategyProber) Probe(_ context.Context, _ int, candidates []int) (int, bool, error) {
	p.Calls++
	if p.Err != nil {
		return 0, false, p.Err
	}
	if p.Result <= 0 {
		return 0, false, nil
	}
	for _, c := range candidates {
		if c == p.Result {
			return p.Result, true, nil
		}
	}
	if len(candidates) > 0 {
		return p.Result, true, nil
	}
	return 0, false, nil
}
