package webapi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AloofLibra/zator/internal/fsutil"
)

const (
	journalFile     = "strategy_journal.jsonl"
	journalMaxBytes = 256 << 10 // 256 KiB кольцо
	journalMaxLines = 400
)

// journalEntry — одна запись локального журнала успеха/fail (без секретов).
type journalEntry struct {
	Time     time.Time `json:"ts"`
	Profile  int       `json:"profile"`
	Strategy int       `json:"strategy"`
	Result   string    `json:"result"` // ok|fail|warn
	Source   string    `json:"source"`
}

type journalStore struct {
	mu   sync.Mutex
	path string
}

func newJournalStore(stateDir string) *journalStore {
	return &journalStore{path: filepath.Join(stateDir, journalFile)}
}

func (s *Server) appendJournal(e journalEntry) {
	if s == nil || s.journal == nil {
		return
	}
	// BP11: opt-in. Без journal_enabled на диск не пишем (роутер / флеш).
	if !sFlags(s).JournalEnabled {
		return
	}
	_ = s.journal.append(e)
}

func (j *journalStore) append(e journalEntry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(j.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	_ = f.Close()
	if werr != nil {
		return werr
	}
	return j.rotateLocked()
}

func (j *journalStore) rotateLocked() error {
	st, err := os.Stat(j.path)
	if err != nil || st.Size() < journalMaxBytes {
		return nil
	}
	lines, err := j.readLinesLocked(journalMaxLines / 2)
	if err != nil {
		return err
	}
	var b []byte
	for _, ln := range lines {
		b = append(b, ln...)
		b = append(b, '\n')
	}
	return fsutil.AtomicWriteFile(j.path, b, 0o644)
}

func (s *Server) journalRecentGet() map[string]any {
	f := sFlags(s)
	entries := []journalEntry{}
	if s.journal != nil && f.JournalEnabled {
		entries = s.journal.recent(30)
	}
	if entries == nil {
		entries = []journalEntry{}
	}
	return map[string]any{
		"enabled": f.JournalEnabled,
		"entries": entries,
	}
}

func (j *journalStore) recent(n int) []journalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines, err := j.readLinesLocked(n)
	if err != nil {
		return nil
	}
	out := make([]journalEntry, 0, len(lines))
	for _, ln := range lines {
		var e journalEntry
		if json.Unmarshal(ln, &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (j *journalStore) readLinesLocked(n int) ([][]byte, error) {
	f, err := os.Open(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		all = append(all, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
