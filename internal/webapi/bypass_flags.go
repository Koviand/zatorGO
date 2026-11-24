package webapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/AloofLibra/zator/internal/fsutil"
)

const bypassFlagsFile = "bypass_flags.json"

// BypassFlags — локальные флаги обхода DPI (не в zapret-конфиге).
type BypassFlags struct {
	// AutoRelock — монитор TLS → узкий relock деградировавших профилей.
	AutoRelock bool `json:"auto_relock"`
	// AutoRelockFails — подряд fail check до запуска relock (default 3).
	AutoRelockFails int `json:"auto_relock_fails"`
	// SweepOnProviderChange — soft-sweep после смены ASN/бренда.
	SweepOnProviderChange bool `json:"sweep_on_provider_change"`
	// SweepScheduleHours — редкий schedule supersweep; 0 = выкл.
	SweepScheduleHours int `json:"sweep_schedule_hours"`
	// BlobFallback — при fail после смены strategy пробовать blob/clone (BP9).
	BlobFallback bool `json:"blob_fallback"`
	// JournalEnabled — локальный JSONL успехов/fail (BP11).
	JournalEnabled bool `json:"journal_enabled"`
}

// DefaultBypassFlags возвращает значения по умолчанию (всё выкл / безопасные лимиты).
func DefaultBypassFlags() BypassFlags {
	return BypassFlags{AutoRelockFails: 3}
}

func (f BypassFlags) normalized() BypassFlags {
	if f.AutoRelockFails < 1 {
		f.AutoRelockFails = 3
	}
	if f.AutoRelockFails > 10 {
		f.AutoRelockFails = 10
	}
	if f.SweepScheduleHours < 0 {
		f.SweepScheduleHours = 0
	}
	if f.SweepScheduleHours > 168 {
		f.SweepScheduleHours = 168
	}
	return f
}

type bypassFlagsStore struct {
	mu   sync.Mutex
	path string
	cur  BypassFlags
}

func newBypassFlagsStore(stateDir string) *bypassFlagsStore {
	s := &bypassFlagsStore{
		path: filepath.Join(stateDir, bypassFlagsFile),
		cur:  DefaultBypassFlags(),
	}
	_ = s.load()
	return s
}

func (s *bypassFlagsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cur = DefaultBypassFlags()
			return nil
		}
		return err
	}
	var f BypassFlags
	if err := json.Unmarshal(data, &f); err != nil {
		s.cur = DefaultBypassFlags()
		return err
	}
	s.cur = f.normalized()
	return nil
}

func (s *bypassFlagsStore) get() BypassFlags {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *bypassFlagsStore) set(f BypassFlags) error {
	f = f.normalized()
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.AtomicWriteFile(s.path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	s.cur = f
	return nil
}

func (s *bypassFlagsStore) patch(fn func(*BypassFlags)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cur
	fn(&cur)
	f := cur.normalized()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.AtomicWriteFile(s.path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	s.cur = f
	return nil
}
