package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/googlevideo"
	"github.com/AloofLibra/zator/internal/paths"
)

var (
	gvSeederMu sync.Mutex
	gvSeeder   *googlevideo.Seeder

	gvMidstreamMu      sync.Mutex
	gvMidstreamTimer   *time.Timer
	gvMidstreamPending bool
)

const gvMidstreamDebounce = 2 * time.Second

// GooglevideoSeedStatus — статус фонового автопробоя для Domains UI.
func (a *App) GooglevideoSeedStatus() googlevideo.SeedStatus {
	gvSeederMu.Lock()
	s := gvSeeder
	gvSeederMu.Unlock()
	if s == nil {
		return googlevideo.SeedStatus{State: "disabled"}
	}
	return s.Status()
}

// StartGooglevideoSeeder поднимает фоновый seed новых sn-* → auto_locked.
func (a *App) StartGooglevideoSeeder(ctx context.Context) {
	gvSeederMu.Lock()
	defer gvSeederMu.Unlock()
	if gvSeeder != nil {
		return
	}
	// Живой probe через profile-2 lock ломает чужой трафик на 2+ с — по умолчанию
	// только transfer/default seed. Изолированный NfqwsProbe — отдельный контур (supersweep).
	seeder := googlevideo.NewSeeder(googlevideo.SeederConfig{
		LivePath:  paths.LiveAutoLockedTSV(),
		StatePath: a.Paths.AutoLockedTSV,
		ListHosts: func() ([]string, error) {
			_, _ = a.SyncGooglevideoHosts()
			return a.GooglevideoHosts().List()
		},
		Probe:        nil,
		DefaultStrat: 41,
		MaxStrat:     44,
		TailMax:      8,
		Interval:     45 * time.Second,
		Cooldown:     2 * time.Second,
		OnSeeded: func(hostKey string, strategy int) {
			_ = hostKey
			_ = strategy
			a.touchAutoLockedReloadStamp()
			// Один midstream-apply на пачку сидов (не Save+custom.d на каждый sn-*).
			a.scheduleGooglevideoMidstream()
		},
	})
	gvSeeder = seeder
	seeder.Start(ctx)
}

// StopGooglevideoSeeder останавливает seed-воркер.
func (a *App) StopGooglevideoSeeder() {
	gvSeederMu.Lock()
	s := gvSeeder
	gvSeeder = nil
	gvSeederMu.Unlock()
	if s != nil {
		s.Stop()
	}
	gvMidstreamMu.Lock()
	if gvMidstreamTimer != nil {
		gvMidstreamTimer.Stop()
		gvMidstreamTimer = nil
	}
	gvMidstreamPending = false
	gvMidstreamMu.Unlock()
}

// scheduleGooglevideoMidstream — debounce EnsureGooglevideoMidstream после пачки OnSeeded.
func (a *App) scheduleGooglevideoMidstream() {
	gvMidstreamMu.Lock()
	defer gvMidstreamMu.Unlock()
	gvMidstreamPending = true
	if gvMidstreamTimer != nil {
		gvMidstreamTimer.Stop()
	}
	gvMidstreamTimer = time.AfterFunc(gvMidstreamDebounce, func() {
		gvMidstreamMu.Lock()
		run := gvMidstreamPending
		gvMidstreamPending = false
		gvMidstreamTimer = nil
		gvMidstreamMu.Unlock()
		if run {
			a.EnsureGooglevideoMidstream(context.Background())
		}
	})
}

// touchAutoLockedReloadStamp сигналит Lua (slm_merge_auto_locked_from_disk), что TSV обновился.
func (a *App) touchAutoLockedReloadStamp() {
	_ = os.MkdirAll("/tmp", 0o755)
	_ = fsutil.AtomicWriteFile("/tmp/z2r-auto-locked.reload",
		[]byte(fmt.Sprintf("%d\n", time.Now().UnixNano())), 0o644)
}

// EnsureGooglevideoMidstream — sync hosts + ipv4 для WSSIZE mark/ipset + custom.d hook.
func (a *App) EnsureGooglevideoMidstream(ctx context.Context) {
	if a == nil {
		return
	}
	_, _ = a.SyncGooglevideoHosts()
	_, _ = a.RefreshGooglevideoIPv4(ctx, false)
	cfg, err := config.Load(a.Paths.ZapretConfig)
	if err != nil {
		return
	}
	changed, _ := cfg.SyncWssizeRampVars()
	if cfg.StripWssizeShellIf() {
		changed = true
	}
	if changed {
		_ = cfg.Save()
	}
	_ = a.SyncCustomDHooks(cfg)
}
