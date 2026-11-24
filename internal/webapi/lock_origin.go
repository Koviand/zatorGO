package webapi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/AloofLibra/zator/internal/fsutil"
)

const lockOriginFile = "lock_origin.tsv"

// Источники apply-лока (sidecar; не путать с orchestra.LockSource).
const (
	OriginManual          = "manual"
	OriginRecommendations = "recommendations"
	OriginAutoRelock      = "auto_relock"
	OriginSupersweep      = "supersweep"
	OriginFixDiscord      = "fix_discord"
)

type lockOriginStore struct {
	mu   sync.Mutex
	path string
}

func newLockOriginStore(orchestraDir string) *lockOriginStore {
	return &lockOriginStore{path: filepath.Join(orchestraDir, lockOriginFile)}
}

func (s *lockOriginStore) Get(profile int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return ""
	}
	return m[profile]
}

func (s *lockOriginStore) Set(profile int, origin string) error {
	if profile < 1 {
		return fmt.Errorf("lock_origin: некорректный профиль %d", profile)
	}
	origin = strings.TrimSpace(origin)
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if m == nil {
		m = map[int]string{}
	}
	if origin == "" {
		delete(m, profile)
	} else {
		m[profile] = origin
	}
	return s.writeLocked(m)
}

func (s *lockOriginStore) Map() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return map[int]string{}
	}
	return m
}

func (s *lockOriginStore) readLocked() (map[int]string, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[int]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || id < 1 {
			continue
		}
		out[id] = strings.TrimSpace(parts[1])
	}
	return out, sc.Err()
}

func (s *lockOriginStore) writeLocked(m map[int]string) error {
	var b strings.Builder
	b.WriteString("# profile\torigin\n")
	for id := 1; id <= 10; id++ {
		if v, ok := m[id]; ok && v != "" {
			fmt.Fprintf(&b, "%d\t%s\n", id, v)
		}
	}
	return fsutil.AtomicWriteFile(s.path, []byte(b.String()), 0o644)
}
