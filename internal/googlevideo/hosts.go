package googlevideo

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/AloofLibra/zator/internal/fsutil"
)

// HostsFileName — список уникальных FQDN googlevideo (без схемы).
const HostsFileName = "googlevideo-hosts.txt"

// SNILogFileName — лог SNI из Lua (host\tunix_ts), пишет nobody.
const SNILogFileName = "sni-hosts.log"

// MaxHostsFileRows — лимит строк googlevideo-hosts.txt.
const MaxHostsFileRows = MaxStateRows

// ParseHostInput принимает FQDN или URL вида https://rr1---sn-….googlevideo.com/…
// и возвращает нормализованный host.
func ParseHostInput(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", false
	}
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	if i := strings.IndexAny(v, "/?#"); i >= 0 {
		v = v[:i]
	}
	if i := strings.LastIndex(v, "@"); i >= 0 {
		v = v[i+1:]
	}
	if i := strings.LastIndex(v, ":"); i >= 0 {
		// порт только если хвост целиком цифры
		port := v[i+1:]
		if port != "" && strings.Trim(port, "0123456789") == "" {
			v = v[:i]
		}
	}
	v = strings.TrimPrefix(v, "*.")
	return NormalizeHostname(v)
}

// HostsFile — дедуплицированный список googlevideo FQDN на диске.
type HostsFile struct {
	path string
	mu   sync.Mutex
}

// NewHostsFile создаёт обёртку над путём (обычно ListsDir/googlevideo-hosts.txt).
func NewHostsFile(path string) *HostsFile {
	return &HostsFile{path: path}
}

// Path возвращает путь к файлу.
func (h *HostsFile) Path() string { return h.path }

// List возвращает отсортированные уникальные host.
func (h *HostsFile) List() ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.readLocked()
}

func (h *HostsFile) readLocked() ([]string, error) {
	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	seen := map[string]struct{}{}
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		host, ok := ParseHostInput(line)
		if !ok {
			continue
		}
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	sort.Strings(out)
	return out, sc.Err()
}

func (h *HostsFile) writeLocked(hosts []string) error {
	if len(hosts) > MaxHostsFileRows {
		hosts = hosts[len(hosts)-MaxHostsFileRows:]
	}
	sort.Strings(hosts)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, host := range hosts {
		b.WriteString(host)
		b.WriteByte('\n')
	}
	return fsutil.AtomicWriteFile(h.path, []byte(b.String()), 0o644)
}

// Add добавляет один host (или URL). ok=false — не googlevideo / дубликат без записи.
func (h *HostsFile) Add(raw string) (added bool, err error) {
	host, ok := ParseHostInput(raw)
	if !ok {
		return false, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.readLocked()
	if err != nil {
		return false, err
	}
	for _, e := range list {
		if e == host {
			return false, nil
		}
	}
	list = append(list, host)
	if err := h.writeLocked(list); err != nil {
		return false, err
	}
	return true, nil
}

// Remove удаляет host.
func (h *HostsFile) Remove(raw string) (removed bool, err error) {
	host, ok := ParseHostInput(raw)
	if !ok {
		return false, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.readLocked()
	if err != nil {
		return false, err
	}
	keep := list[:0]
	for _, e := range list {
		if e == host {
			removed = true
			continue
		}
		keep = append(keep, e)
	}
	if !removed {
		return false, nil
	}
	return true, h.writeLocked(keep)
}

// Clear очищает файл.
func (h *HostsFile) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeLocked(nil)
}

// MergeFrom добавляет hosts из произвольного списка строк (FQDN/URL).
// Возвращает число реально добавленных.
func (h *HostsFile) MergeFrom(raws []string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.readLocked()
	if err != nil {
		return 0, err
	}
	seen := map[string]struct{}{}
	for _, e := range list {
		seen[e] = struct{}{}
	}
	added := 0
	for _, raw := range raws {
		host, ok := ParseHostInput(raw)
		if !ok {
			continue
		}
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		list = append(list, host)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if err := h.writeLocked(list); err != nil {
		return 0, err
	}
	return added, nil
}

// MergeSNILog читает sni-hosts.log (host\tts) и добавляет host в файл.
func (h *HostsFile) MergeSNILog(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	var raws []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		host := line
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			host = line[:i]
		}
		raws = append(raws, host)
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return h.MergeFrom(raws)
}

// MergeLocalRows добавляет host из строк collector local.tsv.
func (h *HostsFile) MergeLocalRows(rows []LocalRow) (int, error) {
	raws := make([]string, 0, len(rows))
	for _, r := range rows {
		raws = append(raws, r.Host)
	}
	return h.MergeFrom(raws)
}

// MergeAutoLocks добавляет синтетические rr1---sn-….googlevideo.com из auto_locked.
func (h *HostsFile) MergeAutoLocks(locks []AutoLock) (int, error) {
	raws := make([]string, 0, len(locks))
	for _, l := range locks {
		fqdn, ok := SyntheticRRHost(l.Host)
		if !ok {
			continue
		}
		raws = append(raws, fqdn)
	}
	return h.MergeFrom(raws)
}
