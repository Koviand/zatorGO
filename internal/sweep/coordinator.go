package sweep

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
)

// Ошибки изолированной пробы.
var (
	// ErrProbe — изолированный пробник не запустился или не восстановился.
	ErrProbe = errors.New("sweep: изолированный пробник недоступен")
	// ErrIsolationLost — изоляция проб нарушена повторно; прогон остановлен.
	ErrIsolationLost = errors.New("sweep: изоляция проб нарушена")
)

// Config — конфигурация координатора.
type Config struct {
	// Dir — каталог прогресса (по умолчанию DefaultDir = /tmp/z2r-supersweep).
	Dir string
	// ArchiveDir — каталог с last-report.tsv (пусто — отчёт не сохраняется).
	ArchiveDir string

	// Store — хранилище фиксаций: сюда применяются победители и откатываются
	// прежние локи (обычно *orchestra.Store).
	Store Locker
	// ProbeStore — приватное хранилище пробных локов изолированного режима.
	// nil — пробы пишут прямо в Store (локи потом откатываются/применяются).
	ProbeStore Locker
	// Checker — проверка цели (обычно *netcheck.Checker).
	Checker TargetChecker
	// Probe — изолированный nfqws2; по умолчанию NoopProbe.
	Probe Probe

	// MaxStrategy возвращает число стратегий профиля; по умолчанию
	// config.DefaultMaxStrategy. Обычно cfg.ProfileMaxStrategyOrDefault.
	MaxStrategy func(profile int) int
	// GVURL возвращает URL проверки googlevideo (кластер пользователя);
	// по умолчанию https://rr2---sn-4g5ednly.googlevideo.com/.
	GVURL func(ctx context.Context) string
	// Gate — необязательная предпроверка DNS (например (&netcheck.SpoofGate{}).Check);
	// при подмене прогон не начинается (ErrDNSSpoofed).
	Gate func(ctx context.Context) netcheck.GateResult

	// Sleep и Now подменяются в тестах.
	Sleep SleepFunc
	Now   func() time.Time
	// CancelPoll — период опроса cancel-файла (по умолчанию 200 мс).
	CancelPoll time.Duration
	// Logf — необязательный журнал (строки консоли прогона).
	Logf func(format string, args ...any)
}

// Params — параметры одного суперавтопрогона.
type Params struct {
	// Scope — область локов ("" = default).
	Scope string
	// TLSPref — требование к версиям TLS: any|12|13|both.
	TLSPref netcheck.TLSPref
	// Pause — пауза между проверками YouTube/Googlevideo (включает Settle);
	// DSPause — для Discord (не меньше Pause); RKNPause — для РКН.
	Pause, DSPause, RKNPause time.Duration
	// GreenPause — пауза после зелёной стратегии с обеими TLS (0 — выключена).
	GreenPause time.Duration
	// PhasePause — пауза между фазами.
	PhasePause time.Duration
	// Settle — ожидание после записи лока (TTL кеша Lua).
	Settle time.Duration
	// RKNPar — параллельность РКН (хранится в статусе; проход домены по одному).
	RKNPar int
	// Domains — домены РКН; «example.com» или URL.
	Domains []string
	// MediaStatus — статус медиапроверки googlevideo. Пусто — медиапроверка
	// выключена (обычные ok/warn/fail). "ready" — Checker сам делает
	// медиапробу. Любое другое значение (например resolver_unavailable)
	// помечает результаты googlevideo как unknown (TLS/CDN-тест без медиаданных).
	MediaStatus string
	// Workers — подмножество фаз (yt|gv|ds|rkn). Пусто — все Phases.
	Workers []string
}

// DefaultParams возвращает значения по умолчанию из supersweep.sh.
func DefaultParams() Params {
	return Params{
		Scope:      orchestra.ScopeDefault,
		TLSPref:    netcheck.PrefAny,
		Pause:      DefaultPause,
		DSPause:    3 * DefaultPause,
		RKNPause:   DefaultRKNPause,
		GreenPause: DefaultGreenPause,
		PhasePause: DefaultPhasePause,
		Settle:     DefaultSettle,
		RKNPar:     DefaultRKNPar,
		Domains:    append([]string(nil), DefaultRKNDomains...),
	}
}

// Normalize приводит параметры к допустимым: пауза Discord не меньше общей,
// RKNPar >= 1, TLSPref и Scope — каноничны, отрицательные паузы обнуляются.
func (p Params) Normalize() Params {
	for _, d := range []*time.Duration{&p.Pause, &p.DSPause, &p.RKNPause, &p.GreenPause, &p.PhasePause, &p.Settle} {
		if *d < 0 {
			*d = 0
		}
	}
	if p.DSPause < p.Pause {
		p.DSPause = p.Pause
	}
	if p.RKNPar < 1 {
		p.RKNPar = DefaultRKNPar
	}
	p.TLSPref = netcheck.ParseTLSPref(string(p.TLSPref))
	if p.Scope == "" {
		p.Scope = orchestra.ScopeDefault
	}
	return p
}

// EstimateTotal — ориентировочное время прогона (худший случай, без GreenPause):
// проба ≈ 6 с + пауза на каждую стратегию и межфазовые паузы.
func EstimateTotal(max1, max2, max4, max3, domains int, p Params) time.Duration {
	const probe = 6 * time.Second
	return time.Duration(max1)*(probe+p.Pause) + time.Duration(max2)*(probe+p.Pause) +
		time.Duration(max4)*(probe+p.DSPause) + time.Duration(domains*max3)*(probe+p.RKNPause) +
		3*p.PhasePause
}

type workerSpec struct {
	name    string
	profile int
	protos  []string
	label   string
	url     string
	max     int
	pause   time.Duration
}

// Coordinator — координатор суперавтопрогона: один прогон на каталог Dir.
type Coordinator struct {
	cfg Config

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	final   Report
	err     error
}

// New создаёт координатор.
func New(cfg Config) (*Coordinator, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: не задан Store", ErrConfig)
	}
	if cfg.Checker == nil {
		return nil, fmt.Errorf("%w: не задан Checker", ErrConfig)
	}
	if cfg.Dir == "" {
		cfg.Dir = DefaultDir
	}
	cfg.Dir = filepath.Clean(cfg.Dir)
	if cfg.MaxStrategy == nil {
		cfg.MaxStrategy = config.DefaultMaxStrategy
	}
	if cfg.Probe == nil {
		cfg.Probe = NoopProbe{}
	}
	if cfg.CancelPoll <= 0 {
		cfg.CancelPoll = 200 * time.Millisecond
	}
	return &Coordinator{cfg: cfg}, nil
}

// Dir возвращает каталог прогресса.
func (c *Coordinator) Dir() string { return c.cfg.Dir }

func (c *Coordinator) lockDir() string { return c.cfg.Dir + ".lock" }

// Running сообщает, что прогон выполняется в этом процессе.
func (c *Coordinator) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Report возвращает отчёт по файлам каталога прогона (можно вызывать в любой момент).
func (c *Coordinator) Report() (Report, error) {
	return ReadReport(c.cfg.Dir, ReportOptions{ArchiveDir: c.cfg.ArchiveDir})
}

// Cancel просит активный прогон остановиться: создаёт файл cancel; воркеры
// остановятся на безопасном шаге, откат выполнит координатор. ErrNotRunning —
// если нет живого прогона.
func (c *Coordinator) Cancel() error {
	c.mu.Lock()
	running, cancel := c.running, c.cancel
	c.mu.Unlock()
	if !running {
		// Прогон мог идти в другом процессе.
		rep, err := c.Report()
		if err != nil || !rep.Active {
			return ErrNotRunning
		}
	}
	if err := os.WriteFile(filepath.Join(c.cfg.Dir, fileCancel), nil, 0o644); err != nil {
		return err
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

// Wait блокируется до конца прогона, начатого Start, и возвращает итоговый отчёт.
func (c *Coordinator) Wait() (Report, error) {
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		return Report{}, ErrNotRunning
	}
	<-done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.final, c.err
}

// Run выполняет суперавтопрогон и блокируется до конца. Возвращает итоговый
// отчёт. Отмена (ctx или cancel-файл) — не ошибка: state=cancelled. Ошибка —
// при невалидных параметрах, занятом lock, подмене DNS (до начала прогона)
// либо сбое записи пробного лока/пробника (прогон остановлен и откачен).
func (c *Coordinator) Run(ctx context.Context, p Params) (Report, error) {
	if err := c.Start(ctx, p); err != nil {
		return Report{}, err
	}
	return c.Wait()
}

// Start подготавливает прогон синхронно (проверки, lock, каталог, статус
// starting) и запускает его в горутине; Wait дожидается результата.
func (c *Coordinator) Start(ctx context.Context, p Params) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return ErrBusy
	}
	c.running = true
	c.mu.Unlock()

	r, err := c.prepare(ctx, p)
	if err != nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.mu.Lock()
	c.cancel, c.done, c.final, c.err = cancel, done, Report{}, nil
	c.mu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		rep, err := r.execute(runCtx)
		c.mu.Lock()
		c.final, c.err, c.running, c.cancel = rep, err, false, nil
		c.mu.Unlock()
	}()
	return nil
}

// run — состояние одного прогона.
type run struct {
	c       *Coordinator
	p       Params
	targets []DomainTarget
	maxes   map[int]int
	started time.Time
	state   State
	alive   string

	errMu  sync.Mutex
	err    error
	cancel context.CancelFunc

	prev []prevRow
}

type prevRow struct {
	kind  string // profile|domain
	key   string
	proto string
	value orchestra.Strategy // StrategyAuto — лока не было
}

func (c *Coordinator) now() time.Time {
	if c.cfg.Now != nil {
		return c.cfg.Now()
	}
	return time.Now()
}

func (c *Coordinator) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

func (c *Coordinator) prepare(ctx context.Context, p Params) (*run, error) {
	p = p.Normalize()
	if len(p.Workers) > 0 && len(NormalizeWorkers(p.Workers)) == 0 {
		return nil, ErrInvalidWorkers
	}
	targets, dropped := SanitizeDomains(p.Domains)
	for _, d := range dropped {
		c.logf("Отброшен некорректный домен: %s", d)
	}
	if len(targets) == 0 {
		return nil, ErrNoDomains
	}
	maxes := map[int]int{}
	for _, prof := range []int{1, 2, 4, 3} {
		n := c.cfg.MaxStrategy(prof)
		if n < 1 {
			return nil, fmt.Errorf("%w: профиль %d", ErrNoStrategies, prof)
		}
		maxes[prof] = n
	}
	if c.cfg.Gate != nil {
		if g := c.cfg.Gate(ctx); g.Spoofed() {
			return nil, fmt.Errorf("%w: %s", ErrDNSSpoofed, g.Message)
		}
	}
	if err := c.acquireLock(); err != nil {
		return nil, err
	}
	r := &run{c: c, p: p, targets: targets, maxes: maxes, started: c.now(), state: StateIdle}
	if err := os.RemoveAll(c.cfg.Dir); err != nil {
		c.releaseLock()
		return nil, err
	}
	if err := os.MkdirAll(c.cfg.Dir, 0o755); err != nil {
		c.releaseLock()
		return nil, err
	}
	if err := r.setState(StateStarting); err != nil {
		c.releaseLock()
		return nil, err
	}
	return r, nil
}

// --- lock-каталог ---

func (c *Coordinator) acquireLock() error {
	lock := c.lockDir()
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := os.Mkdir(lock, 0o700)
		if err == nil {
			return os.WriteFile(filepath.Join(lock, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		pid, perr := strconv.Atoi(readFirstLine(filepath.Join(lock, "pid")))
		if perr == nil && pidAlive(pid) {
			return ErrBusy
		}
		if perr != nil {
			// pid ещё не записан соперником — не трогаем свежий lock.
			if st, serr := os.Stat(lock); serr == nil && time.Since(st.ModTime()) < 2*time.Second {
				return ErrBusy
			}
		}
		// Остался после аварии: процесс мёртв.
		_ = os.RemoveAll(lock)
	}
	return ErrBusy
}

func (c *Coordinator) releaseLock() {
	lock := c.lockDir()
	removeIfExists(filepath.Join(lock, "pid"))
	_ = os.Remove(lock)
}

// --- состояние и статус ---

func (r *run) path(name string) string { return filepath.Join(r.c.cfg.Dir, name) }

func (r *run) setState(to State) error {
	if !CanTransition(r.state, to) {
		return fmt.Errorf("%w: %s -> %s", ErrBadTransition, r.state, to)
	}
	r.state = to
	return r.writeStatus()
}

func (r *run) writeStatus() error {
	var urls []string
	for _, t := range r.targets {
		urls = append(urls, t.URL)
	}
	return writeKV(r.path(fileStatus), kv{
		{"state", string(r.state)},
		{"scope", r.p.Scope},
		{"started", strconv.FormatInt(r.started.Unix(), 10)},
		{"updated", strconv.FormatInt(r.c.now().Unix(), 10)},
		{"tls_pref", string(r.p.TLSPref)},
		{"pause", strconv.Itoa(int(r.p.Pause.Seconds()))},
		{"settle", strconv.Itoa(int(r.p.Settle.Seconds()))},
		{"rkn_par", strconv.Itoa(r.p.RKNPar)},
		{"alive", r.alive},
		{"domains", strings.Join(urls, " ")},
	})
}

// fail запоминает первую ошибку и останавливает прогон.
func (r *run) fail(err error) {
	r.errMu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.errMu.Unlock()
	r.c.logf("Прогон остановлен: %v", err)
	_ = os.WriteFile(r.path(fileCancel), nil, 0o644)
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *run) firstErr() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

// --- выполнение ---

func (r *run) isolated() bool { return r.c.cfg.ProbeStore != nil }

func (r *run) probeLocker() Locker {
	if r.c.cfg.ProbeStore != nil {
		return r.c.cfg.ProbeStore
	}
	return r.c.cfg.Store
}

func (r *run) specs(gvURL string) []workerSpec {
	return []workerSpec{
		{WorkerYT, 1, []string{"tls", "http"}, netcheck.LabelYouTube, netcheck.URLYouTube, r.maxes[1], r.p.Pause},
		{WorkerGV, 2, []string{"tls"}, netcheck.LabelGooglevideo, gvURL, r.maxes[2], r.p.Pause},
		{WorkerDS, 4, []string{"tls"}, netcheck.LabelDiscord, netcheck.URLDiscord, r.maxes[4], r.p.DSPause},
	}
}

func (r *run) execute(ctx context.Context) (Report, error) {
	c := r.c
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	defer cancel()

	// Наблюдатель cancel-файла: внешний процесс (WebUI) может остановить прогон.
	watchDone := make(chan struct{})
	go func() {
		t := time.NewTicker(c.cfg.CancelPoll)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if fileExists(r.path(fileCancel)) {
					cancel()
					return
				}
			case <-watchDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	defer close(watchDone)

	defer func() {
		_ = c.cfg.Probe.Stop()
		rep, _ := ReadReport(c.cfg.Dir, ReportOptions{Final: true})
		if c.cfg.ArchiveDir != "" && rep.Available {
			if err := fsutil.AtomicWriteFile(filepath.Join(c.cfg.ArchiveDir, LastReportFile), []byte(rep.TSV()), 0o644); err != nil {
				c.logf("Не удалось сохранить последний отчёт суперавтопрогона: %v", err)
			}
		}
		c.releaseLock()
	}()

	gvURL := "https://" + netcheck.FallbackClusterDomain + "/"
	if c.cfg.GVURL != nil {
		if u := c.cfg.GVURL(ctx); u != "" {
			gvURL = u
		}
	}
	specs := r.specs(gvURL)
	r.writeRegistry(specs)
	r.collectPrev()
	if r.p.MediaStatus != "" {
		_ = fsutil.AtomicWriteFile(r.path(fileMediaStatus), []byte(r.p.MediaStatus+"\n"), 0o644)
	}

	phases := NormalizeWorkers(r.p.Workers)
	if ctx.Err() == nil {
		r.alive = strings.Join(phases, ",")
		if err := r.setState(StateRunning); err != nil {
			return Report{}, err
		}
	}
	cancelled := ctx.Err() != nil
	for i, phase := range phases {
		if ctx.Err() != nil {
			cancelled = true
			break
		}
		r.alive = phase
		_ = r.writeStatus()
		if err := r.runPhase(ctx, phase, specs); err != nil {
			r.fail(err)
		}
		if ctx.Err() != nil {
			cancelled = true
			break
		}
		r.settleWorker(phase)
		if phase != WorkerRKN && i < len(phases)-1 {
			c.logf("Пауза между фазами: %s", r.p.PhasePause)
			if err := r.sleep(ctx, r.p.PhasePause); err != nil {
				cancelled = true
				break
			}
		}
	}

	// До конца отката глушим отмену: откат и сводка не должны прерываться.
	r.alive = ""
	if cancelled {
		r.restoreOnCancel()
		_ = r.setState(StateCancelled)
	} else {
		r.restoreUnapplied()
		_ = r.setState(StateApplying)
		_ = r.setState(StateDone)
	}

	rep, _ := ReadReport(c.cfg.Dir, ReportOptions{Final: true})
	return rep, r.firstErr()
}

func (r *run) sleep(ctx context.Context, d time.Duration) error {
	if r.c.cfg.Sleep != nil {
		return r.c.cfg.Sleep(ctx, d)
	}
	return sleepCtx(ctx, d)
}

func (r *run) writeRegistry(specs []workerSpec) {
	var b strings.Builder
	for _, s := range specs {
		fmt.Fprintf(&b, "%s\t%s\tprofile\t%d\t%s\t%d\n", s.name, s.label, s.profile, s.url, s.max)
	}
	var doms []string
	for _, t := range r.targets {
		doms = append(doms, t.Domain)
	}
	fmt.Fprintf(&b, "%s\t%s\trkn\t3\t%s\t%d\n", WorkerRKN, workerLabels[WorkerRKN], strings.Join(doms, ","), r.maxes[3])
	_ = fsutil.AtomicWriteFile(r.path(fileWorkers), []byte(b.String()), 0o644)
}

// collectPrev запоминает прежние локи для отката (prev.tsv).
func (r *run) collectPrev() {
	add := func(kind, key, proto string) {
		v := orchestra.Strategy(orchestra.StrategyAuto)
		if s, err := r.c.cfg.Store.GetLock(r.p.Scope, key, proto); err == nil {
			v = s
		}
		r.prev = append(r.prev, prevRow{kind, key, proto, v})
	}
	add("profile", "1", "tls")
	add("profile", "1", "http")
	add("profile", "2", "tls")
	add("profile", "4", "tls")
	add("profile", "3", "tls")
	for _, t := range r.targets {
		add("domain", t.Domain, "tls")
	}
	var b strings.Builder
	for _, p := range r.prev {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", p.kind, p.key, p.proto, p.value)
	}
	_ = fsutil.AtomicWriteFile(r.path(filePrev), []byte(b.String()), 0o644)
}

// restorePrev возвращает прежние локи. В изолированном режиме пробы писали
// только в приватное хранилище, рабочие локи трогать не нужно.
func (r *run) restorePrev(kind, key string) {
	if r.isolated() {
		return
	}
	for _, p := range r.prev {
		if (kind != "" && p.kind != kind) || (key != "" && p.key != key) {
			continue
		}
		var err error
		if p.value.IsAuto() {
			err = r.c.cfg.Store.ClearLock(r.p.Scope, p.key, p.proto)
		} else {
			err = r.c.cfg.Store.SetLock(r.p.Scope, p.key, p.proto, p.value)
		}
		if err != nil {
			r.c.logf("Не удалось вернуть прежний лок %s/%s/%s: %v", p.kind, p.key, p.proto, err)
		}
	}
}

// guarded проверяет здоровье изолированного пробника до и после пробы;
// потеря изоляции делает ответ недостоверным — проба повторяется один раз.
type guarded struct {
	r       *run
	inner   TargetChecker
	profile int
}

func (g guarded) CheckTarget(ctx context.Context, label, url string) netcheck.TargetResult {
	probe := g.r.c.cfg.Probe
	for attempt := 0; attempt < 2; attempt++ {
		if !probe.Healthy(ctx) {
			if err := probe.Start(ctx, g.profile); err != nil || !probe.Healthy(ctx) {
				g.r.fail(fmt.Errorf("%w: профиль %d", ErrProbe, g.profile))
				return netcheck.TargetResult{Label: label, Target: url}
			}
		}
		res := g.inner.CheckTarget(ctx, label, url)
		if probe.Healthy(ctx) {
			return res
		}
	}
	g.r.fail(fmt.Errorf("%w: профиль %d", ErrIsolationLost, g.profile))
	return netcheck.TargetResult{Label: label, Target: url}
}

func (r *run) env(profile int) *Env {
	return &Env{
		Locks:   r.probeLocker(),
		Checker: guarded{r: r, inner: r.c.cfg.Checker, profile: profile},
		Scope:   r.p.Scope,
		Settle:  r.p.Settle,
		Sleep:   r.c.cfg.Sleep,
		Now:     r.c.cfg.Now,
	}
}

func (r *run) runPhase(ctx context.Context, phase string, specs []workerSpec) error {
	probeProfile := workerProfiles[phase]
	if err := r.c.cfg.Probe.Start(ctx, probeProfile); err != nil {
		return fmt.Errorf("%w: профиль %d: %v", ErrProbe, probeProfile, err)
	}
	defer func() {
		var last error
		for i := 0; i < 5; i++ {
			last = r.c.cfg.Probe.Stop()
			if last == nil {
				return
			}
			r.c.logf("probe Stop retry %d: %v", i+1, last)
			time.Sleep(200 * time.Millisecond)
		}
		if last != nil {
			r.c.logf("probe Stop incomplete: %v", last)
			_ = fsutil.AtomicWriteFile(r.path("cleanup_incomplete"), []byte(last.Error()+"\n"), 0o644)
		}
	}()
	r.c.logf("=== Фаза %s (профиль %d) ===", phase, probeProfile)

	if phase == WorkerRKN {
		return r.runRKN(ctx)
	}
	var spec workerSpec
	for _, s := range specs {
		if s.name == phase {
			spec = s
		}
	}
	return r.runProfile(ctx, spec)
}

func (r *run) runProfile(ctx context.Context, s workerSpec) error {
	env := r.env(s.profile)
	res, err := env.AutoSweepProfile(ctx, ProfileSweepConfig{
		Name: s.name, Profile: s.profile, Protos: s.protos, Label: s.label, URL: s.url,
		Max: s.max, TLSPref: r.p.TLSPref, Pause: s.pause, GreenPause: r.p.GreenPause,
		MarkUnknown: s.name == WorkerGV && r.p.MediaStatus != "" && r.p.MediaStatus != "ready",
		OnTry: func(n int) {
			_ = fsutil.AtomicWriteFile(r.path(currentFile(s.name)), []byte(s.url+"\t"+strconv.Itoa(n)+"\n"), 0o644)
		},
		OnRow: func(row ProgressRow) {
			_ = appendLine(r.path(progressFile(s.name)), row.TSV())
			_ = r.writeStatus()
		},
		OnPacing: func(n int, d time.Duration) {
			_ = appendLine(r.path(filePacing), fmt.Sprintf("%s\t%d\t%d", s.name, n, int(d.Seconds())))
		},
	})
	bestKind := res.BestKind
	_ = writeKV(r.path(bestFile(s.name)), kv{
		{"best", numOrEmpty(res.Best)},
		{"best_kind", bestKind},
		{"best_short", res.BestShort},
		{"greens", joinInts(res.Greens)},
		{"fulls", joinInts(res.Fulls)},
		{"warns", joinInts(res.Warns)},
		{"n_ok", strconv.Itoa(res.NOK)},
		{"n_warn", strconv.Itoa(res.NWarn)},
		{"n_fail", strconv.Itoa(res.NFail)},
		{"n_unknown", strconv.Itoa(res.NUnknown)},
		{"interrupted", b2s(res.Interrupted)},
	})
	_ = os.WriteFile(r.path(doneFile(s.name)), nil, 0o644)
	removeIfExists(r.path(currentFile(s.name)))
	return err
}

func (r *run) runRKN(ctx context.Context) error {
	yt := map[int]string{}
	for _, row := range readTSV(r.path(progressFile(WorkerYT))) {
		if len(row) >= 4 && isPositiveInt(row[2]) {
			n, _ := strconv.Atoi(row[2])
			yt[n] = row[3]
		}
	}
	env := r.env(3)
	res, err := env.SweepRKN(ctx, RKNSweepConfig{
		Targets: r.targets, Max: r.maxes[3], Order: OrderStrategies(r.maxes[3], yt),
		TLSPref: r.p.TLSPref, Pause: r.p.RKNPause, GreenPause: r.p.GreenPause,
		OnTry: func(domain string, n int) {
			_ = fsutil.AtomicWriteFile(r.path(currentFile(WorkerRKN)), []byte(domain+"\t"+strconv.Itoa(n)+"\n"), 0o644)
		},
		OnRow: func(row ProgressRow, cov CoverageRow) {
			_ = appendLine(r.path(fileCoverage), cov.TSV())
			_ = appendLine(r.path(progressFile(WorkerRKN)), row.TSV())
			_ = r.writeStatus()
		},
		OnPacing: func(n int, d time.Duration) {
			_ = appendLine(r.path(filePacing), fmt.Sprintf("%s\t%d\t%d", WorkerRKN, n, int(d.Seconds())))
		},
	})
	w := res.Winner
	_ = writeKV(r.path(bestFile(WorkerRKN)), kv{
		{"winner", numOrEmpty(w.OK)},
		{"winner_cover", strconv.Itoa(w.OKCover)},
		{"winner_total", strconv.Itoa(w.Total)},
		{"warn_winner", numOrEmpty(w.Warn)},
		{"warn_winner_cover", strconv.Itoa(w.WarnCover)},
		{"reference", res.Reference},
		{"interrupted", b2s(res.Interrupted)},
	})
	_ = os.WriteFile(r.path(doneFile(WorkerRKN)), nil, 0o644)
	removeIfExists(r.path(currentFile(WorkerRKN)))
	return err
}

func numOrEmpty(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// --- применение результатов ---

func (r *run) setLocks(profile string, protos []string, strategy int) error {
	for _, p := range protos {
		if err := r.c.cfg.Store.SetLock(r.p.Scope, profile, p, orchestra.Strategy(strategy)); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) noteApplied(kind, target string, strategy int) {
	_ = appendLine(r.path(fileApplied), fmt.Sprintf("%s\t%s\t%d", kind, target, strategy))
}

// settleWorker применяет результат завершённой фазы через orchestra.Store:
// зелёная лучшая стратегия профиля; для РКН — зелёные победители доменов и
// профиль 3. Жёлтые результаты автоматически не применяются. Неполный проход
// не применяется, прежний лок возвращается.
func (r *run) settleWorker(name string) {
	if !fileExists(r.path(doneFile(name))) || fileExists(r.path(appliedDoneFile(name))) {
		return
	}
	pkey := workerProfiles[name]
	best := readKV(r.path(bestFile(name)))
	marker := "none"
	finish := func() {
		_ = fsutil.AtomicWriteFile(r.path(appliedDoneFile(name)), []byte(marker+"\n"), 0o644)
	}

	if best["interrupted"] == "1" {
		if name != WorkerRKN {
			r.restorePrev("profile", strconv.Itoa(pkey))
		}
		finish()
		return
	}

	var winner int
	if name == WorkerRKN {
		winner = atoi(best["winner"])
		for _, d := range DomainWinners(parseCoverage(readTSV(r.path(fileCoverage)))) {
			if d.Rank != 0 {
				continue
			}
			if err := r.setLocks(d.Domain, []string{"tls"}, d.Strategy); err != nil {
				r.c.logf("РКН: не удалось применить %s=%d: %v", d.Domain, d.Strategy, err)
				continue
			}
			r.noteApplied("domain", d.Domain, d.Strategy)
			r.c.logf("РКН: домен %s — применена стратегия %d", d.Domain, d.Strategy)
		}
		marker = "domains"
	} else if best["best_kind"] != BestKindWarn {
		winner = atoi(best["best"])
	}

	protos := config.ProfileProtos(pkey)
	if name == WorkerYT {
		protos = []string{"tls", "http"}
	}
	switch {
	case winner > 0:
		if err := r.setLocks(strconv.Itoa(pkey), protos, winner); err != nil {
			r.c.logf("Профиль %d: не удалось применить стратегию %d: %v", pkey, winner, err)
			if name != WorkerRKN {
				marker = "error"
			}
		} else {
			if name != WorkerRKN {
				marker = strconv.Itoa(winner)
			}
			r.noteApplied("profile", strconv.Itoa(pkey), winner)
			r.c.logf("Профиль %d: применена стратегия %d", pkey, winner)
		}
	case name == WorkerRKN:
		r.c.logf("РКН: зелёного покрытия нет — профильная строка не менялась")
	default:
		r.restorePrev("profile", strconv.Itoa(pkey))
	}
	finish()
}

// restoreOnCancel — откат при отмене: уже применённые профили сохраняются,
// остальные возвращаются; доменные пробы откатываются.
func (r *run) restoreOnCancel() {
	for _, name := range []string{WorkerYT, WorkerGV, WorkerDS} {
		marker := readFirstLine(r.path(appliedDoneFile(name)))
		switch marker {
		case "", "none", "error":
			r.restorePrev("profile", strconv.Itoa(workerProfiles[name]))
		}
	}
	r.restorePrev("domain", "")
	applied, _ := readApplied(r.path(fileApplied))
	has3 := false
	for _, a := range applied {
		if a.Kind == "profile" && a.Target == "3" {
			has3 = true
		}
	}
	if !has3 {
		r.restorePrev("profile", "3")
	}
}

// restoreUnapplied — штатное завершение: откатить доменные пробы без
// применённого победителя и профили без маркера применения.
func (r *run) restoreUnapplied() {
	applied, _ := readApplied(r.path(fileApplied))
	has := map[string]bool{}
	for _, a := range applied {
		if a.Kind == "domain" {
			has[a.Target] = true
		}
	}
	for _, t := range r.targets {
		if !has[t.Domain] {
			r.restorePrev("domain", t.Domain)
		}
	}
	for _, name := range []string{WorkerYT, WorkerGV, WorkerDS} {
		if !fileExists(r.path(appliedDoneFile(name))) {
			r.restorePrev("profile", strconv.Itoa(workerProfiles[name]))
		}
	}
}
