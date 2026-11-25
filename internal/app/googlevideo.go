package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AloofLibra/zator/internal/googlevideo"
	"github.com/AloofLibra/zator/internal/paths"
)

var (
	gvMu       sync.Mutex
	gvListener *googlevideo.Manager
	gvCollect  *googlevideo.Collector
)

// GooglevideoHosts возвращает обёртку над lists/googlevideo-hosts.txt.
func (a *App) GooglevideoHosts() *googlevideo.HostsFile {
	return googlevideo.NewHostsFile(a.Paths.GooglevideoHostsFile())
}

func (a *App) newGooglevideoCollector() *googlevideo.Collector {
	centralPath := filepath.Join(a.Paths.ListsDir, "googlevideo-ipv4.txt")
	var central googlevideo.CentralChecker
	cl := &googlevideo.CentralList{File: centralPath}
	if _, err := cl.Load(); err == nil {
		central = cl
	}
	_ = os.MkdirAll(a.Paths.GooglevideoStateDir(), 0o755)
	return googlevideo.NewCollector(googlevideo.CollectorConfig{
		Dir:       a.Paths.GooglevideoStateDir(),
		HostsFile: a.Paths.GooglevideoHostsFile(),
		Central:   central,
	})
}

// GooglevideoCollector возвращает (и при необходимости создаёт) локальный коллектор.
func (a *App) GooglevideoCollector() *googlevideo.Collector {
	gvMu.Lock()
	defer gvMu.Unlock()
	if gvCollect == nil {
		gvCollect = a.newGooglevideoCollector()
	}
	return gvCollect
}

// SyncGooglevideoHosts сливает SNI-лог, local.tsv и auto_locked (askey=2)
// в googlevideo-hosts.txt. При появлении новых хостов подтягивает ipv4.
func (a *App) SyncGooglevideoHosts() (added int, err error) {
	h := a.GooglevideoHosts()
	n1, err1 := h.MergeSNILog(a.Paths.GooglevideoSNILog())
	if err1 != nil {
		return 0, err1
	}
	n2, err2 := h.MergeLocalRows(a.GooglevideoCollector().Local())
	if err2 != nil {
		return n1, err2
	}
	n3, err3 := h.MergeAutoLocks(a.autoLockedForHosts())
	if err3 != nil {
		return n1 + n2, err3
	}
	added = n1 + n2 + n3
	if added > 0 {
		_, _ = a.RefreshGooglevideoIPv4(context.Background(), false)
	}
	return added, nil
}

// autoLockedForHosts — live+backup auto_locked для askey=2 (googlevideo).
func (a *App) autoLockedForHosts() []googlevideo.AutoLock {
	live, _ := googlevideo.ParseAutoLockedTSV(paths.LiveAutoLockedTSV())
	backup, _ := googlevideo.ParseAutoLockedTSV(a.Paths.AutoLockedTSV)
	return googlevideo.FilterAskey(googlevideo.MergeAutoLocked(live, backup), "2")
}

// GooglevideoCollectionStatus — enabled|disabled для settings.
func (a *App) GooglevideoCollectionStatus() string {
	gvMu.Lock()
	running := gvListener != nil && gvListener.Running()
	gvMu.Unlock()
	if running {
		return "enabled"
	}
	mode := googlevideo.DetectMode(context.Background(), googlevideo.DetectOptions{
		SnifferPath: filepath.Join(a.Paths.ZatorRoot, googlevideo.DefaultSnifferRelPath),
	})
	if mode == googlevideo.ModeOff {
		return "disabled"
	}
	return "enabled"
}

// StartGooglevideoCollection поднимает DNS-listener (если доступен) и
// один раз сливает уже накопленные host в файл. Также стартует seed-воркер
// и mid-stream sync (ipv4/WSSIZE).
func (a *App) StartGooglevideoCollection(ctx context.Context) error {
	_, _ = a.SyncGooglevideoHosts()
	a.EnsureGooglevideoMidstream(ctx)
	a.StartGooglevideoSeeder(ctx)

	gvMu.Lock()
	defer gvMu.Unlock()
	if gvListener != nil && gvListener.Running() {
		return nil
	}
	if gvCollect == nil {
		gvCollect = a.newGooglevideoCollector()
	}
	mode := googlevideo.DetectMode(ctx, googlevideo.DetectOptions{
		SnifferPath: filepath.Join(a.Paths.ZatorRoot, googlevideo.DefaultSnifferRelPath),
	})
	if mode == googlevideo.ModeOff {
		return nil
	}
	mgr, err := googlevideo.NewManager(googlevideo.ManagerConfig{
		Mode:        mode,
		SnifferPath: filepath.Join(a.Paths.ZatorRoot, googlevideo.DefaultSnifferRelPath),
		Observer:    gvCollect,
	})
	if err != nil {
		return err
	}
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("googlevideo listener: %w", err)
	}
	gvListener = mgr
	return nil
}

// StopGooglevideoCollection останавливает listener (для тестов/shutdown).
func (a *App) StopGooglevideoCollection() {
	a.StopGooglevideoSeeder()
	gvMu.Lock()
	defer gvMu.Unlock()
	if gvListener != nil {
		_ = gvListener.Stop()
		gvListener = nil
	}
}

// GooglevideoCentralList — кеш lists/googlevideo-ipv4.txt для WSSIZE mark/ipset.
// По умолчанию только локальный файл (из assets); удалённый feed — через
// Z2R_GOOGLEVIDEO_IPS_URL.
func (a *App) GooglevideoCentralList() *googlevideo.CentralList {
	return &googlevideo.CentralList{
		File: filepath.Join(a.Paths.ListsDir, googlevideo.ListFileName),
		URL:  strings.TrimSpace(os.Getenv(googlevideo.EnvListURL)),
	}
}

// RefreshGooglevideoIPv4 обновляет центральный ipv4-список (TTL или force).
// Нужен для mid-stream: WSSIZE lite маркирует только адреса из этого файла/ipset.
func (a *App) RefreshGooglevideoIPv4(ctx context.Context, force bool) (googlevideo.UpdateResult, error) {
	if err := os.MkdirAll(a.Paths.ListsDir, 0o755); err != nil {
		return googlevideo.UpdateResult{}, err
	}
	cl := a.GooglevideoCentralList()
	// Подмешиваем локально наблюдаемые IP (коллектор) в OnUpdate не нужно —
	// они живут отдельно; central list — публичный CIDR/IP feed.
	return cl.Update(ctx, force)
}
