// Package sweep — подбор стратегий: одиночная проба профиля (Try), автопрогон
// одного профиля (AutoSweepProfile) и координатор суперавтопрогона
// (Coordinator) с файлами прогресса для WebUI. Порт lib/supersweep.sh.
//
// Координатор последовательно проходит фазы yt (профиль 1), gv (2), ds (4) и
// rkn (3, домены по одному), пишет прогресс в каталог Dir (по умолчанию
// /tmp/z2r-supersweep), применяет победителей через orchestra.Store и умеет
// отдавать отчёт (Report) по одним лишь файлам — из любого процесса.
// Реальный изолированный nfqws2 подключается через интерфейс Probe
// (по умолчанию — заглушка NoopProbe).
package sweep

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/orchestra"
)

// Значения по умолчанию (Z2R_SUPERSWEEP_*).
const (
	DefaultDir        = "/tmp/z2r-supersweep"
	DefaultSettle     = 2 * time.Second  // TTL кеша Lua после записи лока
	DefaultPause      = 5 * time.Second  // пауза между проверками
	DefaultRKNPause   = 60 * time.Second // пауза между проверками РКН
	DefaultPhasePause = 30 * time.Second // пауза между фазами
	DefaultGreenPause = 5 * time.Second  // пауза после зелёной (обе TLS) стратегии
	DefaultRKNPar     = 2
)

// DefaultRKNDomains — эталонные домены РКН (Z2R_SUPERSWEEP_RKN_DOMAINS).
var DefaultRKNDomains = []string{"xhamster.com", "anidub.com", "amnezia.org"}

// Имена воркеров (фаз).
const (
	WorkerYT  = "yt"
	WorkerGV  = "gv"
	WorkerDS  = "ds"
	WorkerRKN = "rkn"
)

// Phases — порядок фаз суперавтопрогона.
var Phases = []string{WorkerYT, WorkerGV, WorkerDS, WorkerRKN}

// ProfileWorker возвращает имя фазы supersweep для профиля 1/2/3/4.
func ProfileWorker(profile int) (string, bool) {
	w, ok := profileWorkers[profile]
	return w, ok
}

// NormalizeWorkers фильтрует и упорядочивает имена фаз.
// nil/пустой список → все Phases (полный прогон).
// Непустой ввод без валидных yt|gv|ds|rkn → nil (не подменять полным прогоном).
func NormalizeWorkers(workers []string) []string {
	if len(workers) == 0 {
		return append([]string(nil), Phases...)
	}
	want := map[string]bool{}
	for _, w := range workers {
		w = strings.ToLower(strings.TrimSpace(w))
		switch w {
		case WorkerYT, WorkerGV, WorkerDS, WorkerRKN:
			want[w] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	var out []string
	for _, p := range Phases {
		if want[p] {
			out = append(out, p)
		}
	}
	return out
}

// Токены результата одной стратегии.
const (
	TokenOK      = "ok"
	TokenWarn    = "warn"
	TokenFail    = "fail"
	TokenUnknown = "unknown" // TLS/CDN-тест без медиапроверки (googlevideo)
)

// Ошибки пакета.
var (
	// ErrBusy — суперавтопрогон уже выполняется (живой lock).
	ErrBusy = errors.New("sweep: суперавтопрогон уже выполняется")
	// ErrNotRunning — нет активного прогона (для Cancel).
	ErrNotRunning = errors.New("sweep: суперавтопрогон не запущен")
	// ErrNoDomains — после проверки не осталось корректных доменов РКН.
	ErrNoDomains = errors.New("sweep: нет корректных доменов РКН")
	// ErrInvalidWorkers — workers= задан, но нет валидных yt|gv|ds|rkn.
	ErrInvalidWorkers = errors.New("sweep: некорректные workers (ожидаются yt|gv|ds|rkn)")
	// ErrNoStrategies — не удалось определить число стратегий профиля.
	ErrNoStrategies = errors.New("sweep: не удалось определить число стратегий профиля")
	// ErrDNSSpoofed — предпроверка DNS обнаружила подмену; прогон не начат.
	ErrDNSSpoofed = errors.New("sweep: системный DNS подменён или сломан")
	// ErrBadTransition — недопустимый переход состояния.
	ErrBadTransition = errors.New("sweep: недопустимый переход состояния")
	// ErrConfig — некорректная конфигурация.
	ErrConfig = errors.New("sweep: некорректная конфигурация")
)

// Locker — подмножество *orchestra.Store, нужное пробам.
type Locker interface {
	GetLock(scope, profile, proto string) (orchestra.Strategy, error)
	SetLock(scope, profile, proto string, strategy orchestra.Strategy) error
	ClearLock(scope, profile, proto string) error
}

var _ Locker = (*orchestra.Store)(nil)

// TargetChecker — проверка цели; реализует *netcheck.Checker.
type TargetChecker interface {
	CheckTarget(ctx context.Context, label, url string) netcheck.TargetResult
}

var _ TargetChecker = (*netcheck.Checker)(nil)

// Probe — изолированная тестовая очередь nfqws2 одной фазы. Реальная
// реализация поднимает отдельный nfqws2/правила firewall; координатору важны
// только Start/Healthy/Stop.
type Probe interface {
	// Start запускает пробник для профиля фазы.
	Start(ctx context.Context, profile int) error
	// Healthy сообщает, что изоляция цела.
	Healthy(ctx context.Context) bool
	// Stop снимает пробник и правила.
	Stop() error
}

// NoopProbe — заглушка Probe: всегда «здорова», ничего не запускает.
type NoopProbe struct{}

// Start — заглушка.
func (NoopProbe) Start(context.Context, int) error { return nil }

// Healthy — заглушка.
func (NoopProbe) Healthy(context.Context) bool { return true }

// Stop — заглушка.
func (NoopProbe) Stop() error { return nil }

// SleepFunc — прерываемый сон (подменяется в тестах).
type SleepFunc func(ctx context.Context, d time.Duration) error

// sleepCtx — SleepFunc по умолчанию.
func sleepCtx(ctx context.Context, d time.Duration) error {
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

// State — состояние суперавтопрогона (поле state отчёта).
type State string

// Состояния.
const (
	StateIdle        State = "idle"
	StateStarting    State = "starting"
	StateRunning     State = "running"
	StateApplying    State = "applying"
	StateDone        State = "done"
	StateCancelled   State = "cancelled"
	StateInterrupted State = "interrupted"
)

// Valid сообщает, что s — одно из известных состояний.
func (s State) Valid() bool {
	switch s {
	case StateIdle, StateStarting, StateRunning, StateApplying, StateDone, StateCancelled, StateInterrupted:
		return true
	}
	return false
}

// InProgress — незавершённые состояния (нужен живой владелец).
func (s State) InProgress() bool {
	return s == StateStarting || s == StateRunning || s == StateApplying
}

// Terminal — прогон завершён (успешно, отменой или обрывом).
func (s State) Terminal() bool {
	return s == StateDone || s == StateCancelled || s == StateInterrupted
}

var transitions = map[State][]State{
	StateIdle:        {StateStarting},
	StateStarting:    {StateRunning, StateCancelled, StateInterrupted},
	StateRunning:     {StateApplying, StateCancelled, StateInterrupted},
	StateApplying:    {StateDone, StateCancelled, StateInterrupted},
	StateDone:        {StateStarting},
	StateCancelled:   {StateStarting},
	StateInterrupted: {StateStarting},
}

// CanTransition сообщает, допустим ли переход from -> to (повтор того же
// состояния допустим: status обновляется при смене данных).
func CanTransition(from, to State) bool {
	if from == to && from.Valid() {
		return true
	}
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}
