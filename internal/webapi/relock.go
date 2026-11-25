package webapi

import (
	"context"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/platform"
	"github.com/AloofLibra/zator/internal/telemetry"
)

const (
	relockIntervalDefault = 90 * time.Second
	relockSettleDefault   = 2300 * time.Millisecond
	relockCheckTimeout    = 45 * time.Second
	relockProbeTimeout    = 8 * time.Minute
	relockMaxCandidates   = 12
	// relockCooldownDefault — пауза после любой попытки tryRelock (успех/fail),
	// чтобы при офлайне/остановленном nfqws2 не крутить 8‑минутный probe каждые 90 с.
	relockCooldownDefault = 30 * time.Minute
)

// relockMonitor — фоновый монитор TLS для профилей с ручным/рекомендованным локом.
type relockMonitor struct {
	s *Server

	mu             sync.Mutex
	fails          map[int]int
	degraded       map[int]bool
	warning        string
	lastRelock     time.Time // момент последней попытки tryRelock (любой исход)
	nextProfileIdx int       // round-robin, чтобы один профиль не голодал остальные
	busyProfile    int
	cancel         context.CancelFunc
	interval       time.Duration
	cooldown       time.Duration
}

func newRelockMonitor(s *Server) *relockMonitor {
	return &relockMonitor{
		s:        s,
		fails:    map[int]int{},
		degraded: map[int]bool{},
		interval: relockIntervalDefault,
		cooldown: relockCooldownDefault,
	}
}

// StartBackground запускает мониторы (auto-relock, schedule). Идемпотентно.
func (s *Server) StartBackground(ctx context.Context) {
	if s.relock == nil {
		s.relock = newRelockMonitor(s)
	}
	s.relock.start(ctx)
	s.startSweepScheduler(ctx)
}

func (m *relockMonitor) start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	interval := m.interval
	m.mu.Unlock()
	go m.loop(cctx, interval)
}

func (m *relockMonitor) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m *relockMonitor) loop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *relockMonitor) snapshot() (fails map[int]int, degraded map[int]bool, warning string, busy int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fails = make(map[int]int, len(m.fails))
	for k, v := range m.fails {
		fails[k] = v
	}
	degraded = make(map[int]bool, len(m.degraded))
	for k, v := range m.degraded {
		degraded[k] = v
	}
	return fails, degraded, m.warning, m.busyProfile
}

func (m *relockMonitor) setWarning(msg string) {
	m.mu.Lock()
	m.warning = msg
	m.mu.Unlock()
}

func (m *relockMonitor) tick(ctx context.Context) {
	flags := sFlags(m.s)
	if !flags.AutoRelock {
		return
	}
	if m.s.runtimeBusy() {
		return
	}
	// Не крутить probe, если служба остановлена или пользователь не хочет nfqws2.
	if !m.s.zapretWantedAndRunning(ctx) {
		return
	}
	threshold := flags.AutoRelockFails
	profiles := relockWatchProfiles()
	if len(profiles) == 0 {
		return
	}

	m.mu.Lock()
	start := m.nextProfileIdx % len(profiles)
	inCooldown := !m.lastRelock.IsZero() && m.s.now().Sub(m.lastRelock) < m.cooldown
	m.mu.Unlock()

	for i := 0; i < len(profiles); i++ {
		if ctx.Err() != nil {
			return
		}
		id := profiles[(start+i)%len(profiles)]
		if !m.profileHasFixedLock(id) {
			m.mu.Lock()
			delete(m.fails, id)
			delete(m.degraded, id)
			m.mu.Unlock()
			continue
		}
		ok := m.checkProfileOK(ctx, id)
		m.mu.Lock()
		if ok {
			m.fails[id] = 0
			delete(m.degraded, id)
			m.mu.Unlock()
			continue
		}
		m.fails[id]++
		n := m.fails[id]
		if n >= threshold {
			m.degraded[id] = true
		}
		m.mu.Unlock()
		if n >= threshold {
			if inCooldown {
				m.setWarning("auto-relock: cooldown после предыдущей попытки")
				// Сдвигаем указатель, чтобы следующий тик смотрел другие профили.
				m.mu.Lock()
				m.nextProfileIdx = (start + i + 1) % len(profiles)
				m.mu.Unlock()
				return
			}
			m.mu.Lock()
			m.nextProfileIdx = (start + i + 1) % len(profiles)
			m.mu.Unlock()
			m.tryRelock(ctx, id)
			return
		}
	}
	m.mu.Lock()
	m.nextProfileIdx = (start + 1) % len(profiles)
	m.mu.Unlock()
}

func relockWatchProfiles() []int {
	// TLS-проверяемые профили с фиксацией (не UDP 5/6, не DNS 10).
	return []int{1, 2, 3, 4}
}

func (m *relockMonitor) profileHasFixedLock(id int) bool {
	lock, _ := m.s.effectiveLock(orchestra.ScopeDefault, id)
	st, err := orchestra.ParseStrategy(lock)
	if err != nil {
		return false
	}
	// auto / OFF(0) — не фиксированный рабочий lock; relock не трогает.
	return !st.IsAuto() && !st.IsOff()
}

func (m *relockMonitor) checkProfileOK(ctx context.Context, id int) bool {
	cctx, cancel := context.WithTimeout(ctx, relockCheckTimeout)
	defer cancel()
	resp := m.s.checker.CheckProfile(cctx, id)
	return profileCheckOK(resp)
}

func profileCheckOK(resp netcheck.CheckResponse) bool {
	if len(resp.Results) == 0 {
		return false
	}
	primary := resp.Results[0]
	return primary.Verdict == netcheck.VerdictOK || primary.Verdict == netcheck.VerdictWarn
}

// tryRelock — узкий изолированный probe (не трогает live lock до успеха).
func (m *relockMonitor) tryRelock(ctx context.Context, id int) {
	m.mu.Lock()
	if m.busyProfile != 0 {
		m.mu.Unlock()
		return
	}
	m.busyProfile = id
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.busyProfile = 0
		// Любая попытка включает cooldown (успех или fail) — нет шторма при offline.
		m.lastRelock = m.s.now()
		m.mu.Unlock()
	}()

	// #2: сериализация с supersweep / service через runtimeMu + relockActive.
	if !m.s.beginRelock() {
		m.setWarning("auto-relock: пропуск — занят supersweep или служба")
		return
	}
	defer m.s.endRelock()

	if !m.s.zapretWantedAndRunning(ctx) {
		m.setWarning("auto-relock: пропуск — zapret2 не запущен")
		return
	}

	cfg, installed := m.s.readConfig()
	if !installed || cfg == nil {
		m.setWarning("auto-relock: конфиг zapret2 не установлен")
		return
	}
	// #6: те же guards, что у set-lock.
	if err := autoConflict(cfg, id); err != nil {
		m.setWarning("auto-relock: профиль " + strconv.Itoa(id) + " — " + err.Error())
		return
	}
	if err := profileModeGuard(cfg, id); err != nil {
		m.setWarning("auto-relock: профиль " + strconv.Itoa(id) + " — " + err.Error())
		return
	}

	scope := orchestra.ScopeDefault
	prevLabel := m.s.profileLockLabel(scope, id)
	prevSt := parseStrategyLabel(prevLabel)
	candidates := m.relockCandidates(ctx, id, prevSt)

	pctx, cancel := context.WithTimeout(ctx, relockProbeTimeout)
	defer cancel()

	best, ok, err := m.s.strategyProberOrDefault().Probe(pctx, id, candidates)
	if err != nil {
		msg := "auto-relock: профиль " + strconv.Itoa(id) + " — probe недоступен: " + err.Error()
		m.setWarning(msg)
		log.Printf("webapi: %s", msg)
		return
	}
	if !ok || best < 1 {
		msg := "auto-relock: профиль " + strconv.Itoa(id) + " — подбор не удался, прежний lock сохранён"
		m.setWarning(msg)
		m.s.appendJournal(journalEntry{
			Time: m.s.now().UTC(), Profile: id, Strategy: int(prevSt),
			Result: "fail", Source: OriginAutoRelock,
		})
		log.Printf("webapi: %s", msg)
		return
	}

	// Успех probe → одна запись в live store (предыдущий лок ещё на месте до этой строки).
	if err := m.s.applyProfileLock(scope, id, orchestra.Strategy(best)); err != nil {
		msg := "auto-relock: не удалось применить strategy " + strconv.Itoa(best) + ": " + err.Error()
		m.setWarning(msg)
		log.Printf("webapi: %s", msg)
		return
	}
	_ = sleepBrief(ctx, relockSettleDefault)

	liveOK := m.checkProfileOK(ctx, id)
	if !liveOK && sFlags(m.s).BlobFallback {
		// BP9: blob только после применения победителя probe, на live.
		if m.s.tryBlobFallback(ctx, id) {
			liveOK = true
			log.Printf("webapi: auto-relock profile %d → strategy %d (+blob fallback)", id, best)
		}
	}
	if !liveOK {
		_ = m.s.applyProfileLock(scope, id, prevSt)
		msg := "auto-relock: профиль " + strconv.Itoa(id) + " — live verify fail, откат на " + prevLabel
		m.setWarning(msg)
		m.s.appendJournal(journalEntry{
			Time: m.s.now().UTC(), Profile: id, Strategy: int(prevSt),
			Result: "fail", Source: OriginAutoRelock,
		})
		log.Printf("webapi: %s", msg)
		return
	}

	_ = m.s.lockOrigin.Set(id, OriginAutoRelock)
	m.s.appendJournal(journalEntry{
		Time: m.s.now().UTC(), Profile: id, Strategy: best,
		Result: "ok", Source: OriginAutoRelock,
	})
	m.mu.Lock()
	m.fails[id] = 0
	delete(m.degraded, id)
	m.warning = ""
	m.mu.Unlock()
	log.Printf("webapi: auto-relock profile %d → strategy %d (probe)", id, best)
}

// zapretWantedAndRunning — пользователь хочет nfqws2 и процесс жив (не probe).
func (s *Server) zapretWantedAndRunning(ctx context.Context) bool {
	if s.app != nil {
		entware := s.app.Platform.Kind == platform.KindEntware ||
			s.app.Platform.InitSystem == platform.InitEntware
		if entware {
			if _, err := os.Stat(s.app.NfqwsWantRunningPath()); err != nil {
				return false
			}
		}
	}
	return s.zapretRunning(ctx)
}

// beginRelock помечает auto-relock активным, если нет supersweep/уже идущего relock.
func (s *Server) beginRelock() bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.relockActive || s.sweepRunningLocked() {
		return false
	}
	s.relockActive = true
	return true
}

func (s *Server) endRelock() {
	s.runtimeMu.Lock()
	s.relockActive = false
	s.runtimeMu.Unlock()
}

// runtimeBusy — supersweep или auto-relock заняты (для tick/schedule).
func (s *Server) runtimeBusy() bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.relockActive || s.sweepRunningLocked()
}

// relockBlocksRuntime — auto-relock в процессе (вызывать под runtimeMu или через helper).
func (s *Server) relockBlocksRuntime() bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.relockActive
}

func (m *relockMonitor) relockCandidates(ctx context.Context, id int, prev orchestra.Strategy) []int {
	var out []int
	seen := map[int]bool{}
	cfg, _ := m.s.readConfig()
	max := 12
	if cfg != nil {
		max = cfg.ProfileMaxStrategyOrDefault(id)
	}
	add := func(n int) {
		if n < 1 || n > max || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	c := telemetry.New(m.s.paths)
	rec, err := c.LoadRecommendations()
	_ = ctx
	if err == nil && rec != nil && rec.Status == telemetry.StatusReady {
		for _, r := range rec.Top(id) {
			add(r.Strategy)
		}
	}
	if !prev.IsAuto() && !prev.IsOff() {
		p := int(prev)
		for _, d := range []int{1, -1, 2, -2, 3, -3} {
			add(p + d)
		}
	}
	for i := 1; i <= max && len(out) < relockMaxCandidates; i++ {
		add(i)
	}
	if len(out) > relockMaxCandidates {
		out = out[:relockMaxCandidates]
	}
	return out
}

func parseStrategyLabel(label string) orchestra.Strategy {
	if label == "" || label == "auto" {
		return orchestra.StrategyAuto
	}
	n, err := strconv.Atoi(label)
	if err != nil || n < 0 {
		return orchestra.StrategyAuto
	}
	return orchestra.Strategy(n)
}

func sleepBrief(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sFlags(s *Server) BypassFlags {
	if s == nil || s.flags == nil {
		return DefaultBypassFlags()
	}
	return s.flags.get()
}

// healthRelockPayload — поля auto-relock для health JSON.
func (s *Server) healthRelockPayload() map[string]any {
	f := sFlags(s)
	out := map[string]any{
		"auto_relock":       f.AutoRelock,
		"auto_relock_fails": f.AutoRelockFails,
	}
	if s.relock == nil {
		out["enabled"] = f.AutoRelock
		out["degraded_profiles"] = []int{}
		return out
	}
	_, degraded, warning, busy := s.relock.snapshot()
	var deg []int
	for id, on := range degraded {
		if on {
			deg = append(deg, id)
		}
	}
	sort.Ints(deg)
	out["enabled"] = f.AutoRelock
	out["degraded_profiles"] = deg
	out["busy_profile"] = busy
	if warning != "" {
		out["warning"] = warning
	}
	return out
}
