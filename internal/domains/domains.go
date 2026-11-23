// Package domains управляет пользовательскими списками доменов zator:
// netrogat.txt (исключения), netrogat_substrings.txt (исключения по
// подстроке), TCP_Custom.txt (свои домены для RKN-обработки) и
// TCP_RKN_domains_by_substring.txt (маршрутизация RKN по подстроке).
//
// Все файлы лежат в каталоге lists раскладки paths.Paths. Для списка
// TCP_Custom домену можно сопоставить стратегию orchestra (фиксацию
// locked.tsv); при удалении и переименовании домена фиксации во всех
// client-scope очищаются / переносятся.
package domains

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/paths"
)

// Имена файлов списков (внутри каталога lists).
const (
	NetrogatFile           = "netrogat.txt"
	NetrogatSubstringsFile = "netrogat_substrings.txt"
	CustomFile             = "TCP_Custom.txt"
	RKNSubstringsFile      = "TCP_RKN_domains_by_substring.txt"
)

// Kind — вид списка.
type Kind string

// Поддерживаемые списки.
const (
	// KindNetrogat — netrogat.txt: домены-исключения.
	KindNetrogat Kind = "netrogat"
	// KindNetrogatSubstrings — netrogat_substrings.txt: исключения по подстроке.
	KindNetrogatSubstrings Kind = "netrogat_substrings"
	// KindCustom — TCP_Custom.txt: свои домены для RKN-обработки.
	KindCustom Kind = "custom"
	// KindRKNSubstrings — TCP_RKN_domains_by_substring.txt: маршрутизация по подстроке.
	KindRKNSubstrings Kind = "rkn_substrings"
)

// AllKinds возвращает все поддерживаемые виды списков.
func AllKinds() []Kind {
	return []Kind{KindNetrogat, KindNetrogatSubstrings, KindCustom, KindRKNSubstrings}
}

// FileName возвращает имя файла списка ("" для неизвестного вида).
func (k Kind) FileName() string {
	switch k {
	case KindNetrogat:
		return NetrogatFile
	case KindNetrogatSubstrings:
		return NetrogatSubstringsFile
	case KindCustom:
		return CustomFile
	case KindRKNSubstrings:
		return RKNSubstringsFile
	}
	return ""
}

// IsSubstring сообщает, что список хранит подстроки, а не полные домены.
func (k Kind) IsSubstring() bool {
	return k == KindNetrogatSubstrings || k == KindRKNSubstrings
}

// HasStrategies сообщает, что записям списка можно сопоставить стратегию.
func (k Kind) HasStrategies() bool { return k == KindCustom }

// Ошибки пакета.
var (
	// ErrInvalid — недопустимое значение (вид списка, домен, подстрока, стратегия).
	ErrInvalid = errors.New("domains: invalid argument")
	// ErrNotFound — записи нет в списке.
	ErrNotFound = errors.New("domains: entry not found")
	// ErrExists — запись уже есть в списке (при переименовании в существующее имя).
	ErrExists = errors.New("domains: entry already exists")
)

// MaxEntryLength — максимальная длина домена (RFC 1035).
const MaxEntryLength = 253

var (
	// hostRe — имя хоста из меток [a-z0-9-]; допускает одну метку (netrogat).
	hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
)

// NormalizeDomain приводит ввод пользователя (домен или URL) к имени хоста
// в нижнем регистре: отбрасываются схема, учётные данные, порт, путь, запрос,
// ведущие "www."-префиксы не трогаются. Для требуемого custom-домена нужна
// хотя бы одна точка (requireDot).
func NormalizeDomain(input string, requireDot bool) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("%w: пустой домен", ErrInvalid)
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("%w: %q", ErrInvalid, input)
		}
		s = u.Hostname()
	} else {
		// "host:port/path?query#frag" → "host"
		if i := strings.IndexAny(s, "/?#"); i >= 0 {
			s = s[:i]
		}
		if i := strings.LastIndex(s, "@"); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "]") {
			s = s[:i]
		}
	}
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" || len(s) > MaxEntryLength || !hostRe.MatchString(s) {
		return "", fmt.Errorf("%w: %q", ErrInvalid, input)
	}
	if requireDot && !strings.Contains(s, ".") {
		return "", fmt.Errorf("%w: %q (нужен полный домен)", ErrInvalid, input)
	}
	return s, nil
}

// NormalizeSubstring проверяет подстроку. Регистр сохраняется (контракт Web UI);
// пустые строки и комментарии (#…) отклоняются.
func NormalizeSubstring(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" || len(s) > MaxEntryLength || strings.HasPrefix(s, "#") {
		return "", fmt.Errorf("%w: подстрока %q", ErrInvalid, input)
	}
	return s, nil
}

// Normalize нормализует запись в соответствии с видом списка.
func Normalize(kind Kind, input string) (string, error) {
	switch kind {
	case KindNetrogat:
		return NormalizeDomain(input, false)
	case KindCustom:
		return NormalizeDomain(input, true)
	case KindNetrogatSubstrings, KindRKNSubstrings:
		return NormalizeSubstring(input)
	}
	return "", fmt.Errorf("%w: неизвестный список %q", ErrInvalid, kind)
}

// Manager управляет файлами списков.
type Manager struct {
	// Dir — каталог списков (paths.Paths.ListsDir).
	Dir string
	// ExtraDir — каталог пользовательских стратегий (ExtraStratsDir):
	// TCP_Custom.txt и TCP_RKN_domains_by_substring.txt. Пусто — Dir.
	ExtraDir string
	// Locks — хранилище фиксаций стратегий; nil отключает работу со стратегиями.
	Locks *orchestra.Store

	mu sync.Mutex
}

// New создаёт менеджер для раскладки p (списки в p.ListsDir, custom/substring
// в p.ExtraStratsDir, фиксации в p.StateOrchestraDir()).
func New(p paths.Paths) *Manager {
	return &Manager{
		Dir:      p.ListsDir,
		ExtraDir: p.ExtraStratsDir,
		Locks:    orchestra.NewStore(p.StateOrchestraDir()),
	}
}

// KindFromAPIList мапит имя списка Web API (`list=`) на Kind.
// Неизвестные имена (в т.ч. googlevideo) — false.
func KindFromAPIList(list string) (Kind, bool) {
	switch list {
	case "netrogat":
		return KindNetrogat, true
	case "custom_rkn":
		return KindCustom, true
	case "substring":
		return KindRKNSubstrings, true
	case "netrogat_substring":
		return KindNetrogatSubstrings, true
	}
	return "", false
}

// Path возвращает путь к файлу списка.
func (m *Manager) Path(kind Kind) (string, error) {
	name := kind.FileName()
	if name == "" {
		return "", fmt.Errorf("%w: неизвестный список %q", ErrInvalid, kind)
	}
	dir := m.Dir
	if (kind == KindCustom || kind == KindRKNSubstrings) && m.ExtraDir != "" {
		dir = m.ExtraDir
	}
	return filepath.Join(dir, name), nil
}

// Ensure создаёт каталог и пустой файл списка, если их нет.
func (m *Manager) Ensure(kind Kind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.Path(kind)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fsutil.AtomicWriteFile(path, nil, 0)
}

// readRaw читает строки файла как есть (без перевода строки и "\r").
func readRaw(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		out = append(out, strings.TrimRight(sc.Text(), "\r"))
	}
	return out, sc.Err()
}

// isEntry — строка является записью (не пустая и не комментарий).
func isEntry(line string) bool {
	t := strings.TrimSpace(line)
	return t != "" && !strings.HasPrefix(t, "#")
}

// writeRaw записывает строки атомарно, отбрасывая пустые строки.
func writeRaw(path string, lines []string) error {
	var b strings.Builder
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return fsutil.AtomicWriteFile(path, []byte(b.String()), 0)
}

// List возвращает записи списка (без пустых строк и комментариев) в порядке файла.
func (m *Manager) List(kind Kind) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked(kind)
}

func (m *Manager) listLocked(kind Kind) ([]string, error) {
	path, err := m.Path(kind)
	if err != nil {
		return nil, err
	}
	lines, err := readRaw(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range lines {
		if isEntry(l) {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out, nil
}

// Count возвращает число записей списка.
func (m *Manager) Count(kind Kind) (int, error) {
	l, err := m.List(kind)
	return len(l), err
}

// Contains сообщает, есть ли запись в списке (без учёта регистра).
func (m *Manager) Contains(kind Kind, entry string) (bool, error) {
	n, err := Normalize(kind, entry)
	if err != nil {
		return false, err
	}
	l, err := m.List(kind)
	if err != nil {
		return false, err
	}
	for _, e := range l {
		if strings.EqualFold(e, n) {
			return true, nil
		}
	}
	return false, nil
}

// Add добавляет запись в конец списка. added=false, если запись уже есть
// (сравнение без учёта регистра). Возвращается нормализованное значение.
func (m *Manager) Add(kind Kind, entry string) (normalized string, added bool, err error) {
	normalized, err = Normalize(kind, entry)
	if err != nil {
		return "", false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, _ := m.Path(kind)
	lines, err := readRaw(path)
	if err != nil {
		return "", false, err
	}
	for _, l := range lines {
		if isEntry(l) && strings.EqualFold(strings.TrimSpace(l), normalized) {
			return normalized, false, nil
		}
	}
	if err := writeRaw(path, append(lines, normalized)); err != nil {
		return "", false, err
	}
	return normalized, true, nil
}

// customProto — единственный протокол, допустимый для фиксации стратегии
// пользовательского домена в orchestra.
const customProto = orchestra.ProtoTLS

// AddWithStrategy добавляет домен в TCP_Custom и фиксирует для него TLS-стратегию
// в default-scope. Если домен уже в списке, стратегия всё равно обновляется.
// strategy = orchestra.StrategyAuto снимает фиксацию.
func (m *Manager) AddWithStrategy(domain string, strategy orchestra.Strategy) (added bool, err error) {
	if m.Locks == nil {
		return false, fmt.Errorf("%w: хранилище стратегий не настроено", ErrInvalid)
	}
	norm, added, err := m.Add(KindCustom, domain)
	if err != nil {
		return false, err
	}
	if err := m.Locks.SetLock(orchestra.ScopeDefault, norm, string(customProto), strategy); err != nil {
		return added, err
	}
	return added, nil
}

// SetStrategy фиксирует TLS-стратегию для домена в default-scope.
func (m *Manager) SetStrategy(domain string, strategy orchestra.Strategy) error {
	return m.SetStrategyInScope(orchestra.ScopeDefault, domain, strategy)
}

// SetStrategyInScope фиксирует TLS-стратегию для домена в указанном client-scope.
// Домен должен уже быть в TCP_Custom.
func (m *Manager) SetStrategyInScope(scope, domain string, strategy orchestra.Strategy) error {
	if m.Locks == nil {
		return fmt.Errorf("%w: хранилище стратегий не настроено", ErrInvalid)
	}
	norm, err := Normalize(KindCustom, domain)
	if err != nil {
		return err
	}
	ok, err := m.Contains(KindCustom, norm)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, norm)
	}
	return m.Locks.SetLock(scope, norm, string(customProto), strategy)
}

// ClearStrategy снимает фиксацию TLS-стратегии домена в указанном scope.
func (m *Manager) ClearStrategy(scope, domain string) error {
	if m.Locks == nil {
		return fmt.Errorf("%w: хранилище стратегий не настроено", ErrInvalid)
	}
	norm, err := Normalize(KindCustom, domain)
	if err != nil {
		return err
	}
	return m.Locks.ClearLock(scope, norm, string(customProto))
}

// Strategy возвращает фиксацию TLS-стратегии домена (default-scope);
// orchestra.StrategyAuto, если её нет.
func (m *Manager) Strategy(domain string) (orchestra.Strategy, error) {
	if m.Locks == nil {
		return orchestra.StrategyAuto, nil
	}
	norm, err := Normalize(KindCustom, domain)
	if err != nil {
		return 0, err
	}
	return m.Locks.GetLock(orchestra.ScopeDefault, norm, string(customProto))
}

// Entry — запись списка со стратегией (для KindCustom).
type Entry struct {
	Value string
	// Strategy — фиксация TLS-стратегии; orchestra.StrategyAuto, если её нет
	// (и всегда для списков без стратегий).
	Strategy orchestra.Strategy
}

// ListWithStrategies возвращает записи списка со стратегиями default-scope.
func (m *Manager) ListWithStrategies(kind Kind) ([]Entry, error) {
	return m.ListWithStrategiesInScope(kind, orchestra.ScopeDefault)
}

// ListWithStrategiesInScope возвращает записи со стратегиями в указанном scope.
func (m *Manager) ListWithStrategiesInScope(kind Kind, scope string) ([]Entry, error) {
	items, err := m.List(kind)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		e := Entry{Value: it, Strategy: orchestra.StrategyAuto}
		if kind.HasStrategies() && m.Locks != nil && orchestra.IsCustomDomain(it) {
			st, err := m.Locks.GetLock(scope, it, string(customProto))
			if err != nil {
				return nil, err
			}
			e.Strategy = st
		}
		out = append(out, e)
	}
	return out, nil
}

// Import добавляет несколько записей из сырого текста (разделители: пробел,
// перевод строки, запятая, точка с запятой). Строки с '#' пропускаются.
func (m *Manager) Import(kind Kind, raw string) (added, duplicates, skipped int, err error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ',' || r == ';'
	})
	for _, f := range fields {
		if strings.HasPrefix(f, "#") {
			continue
		}
		_, ok, aerr := m.Add(kind, f)
		if aerr != nil {
			if errors.Is(aerr, ErrInvalid) {
				skipped++
				continue
			}
			return added, duplicates, skipped, aerr
		}
		if ok {
			added++
		} else {
			duplicates++
		}
	}
	return added, duplicates, skipped, nil
}

// Clear удаляет все записи списка. Для KindCustom дополнительно снимает
// фиксации стратегий; lockFailures — число доменов, у которых ClearEverywhere
// вернул ошибку (сам список при этом уже очищен).
func (m *Manager) Clear(kind Kind) (cleared int, lockFailures int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.Path(kind)
	if err != nil {
		return 0, 0, err
	}
	lines, err := readRaw(path)
	if err != nil {
		return 0, 0, err
	}
	var entries []string
	for _, l := range lines {
		if isEntry(l) {
			entries = append(entries, strings.TrimSpace(l))
		}
	}
	if err := writeRaw(path, nil); err != nil {
		return 0, 0, err
	}
	if kind.HasStrategies() && m.Locks != nil {
		for _, e := range entries {
			if !orchestra.IsCustomDomain(e) {
				continue
			}
			if cerr := m.Locks.ClearEverywhere(e, string(customProto)); cerr != nil {
				lockFailures++
			}
		}
	}
	return len(entries), lockFailures, nil
}

// Remove удаляет запись из списка. removed=false, если записи не было.
// Для KindCustom фиксации стратегий домена очищаются во всех scope.
func (m *Manager) Remove(kind Kind, entry string) (removed bool, err error) {
	norm, err := Normalize(kind, entry)
	if err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, _ := m.Path(kind)
	lines, err := readRaw(path)
	if err != nil {
		return false, err
	}
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		if isEntry(l) && strings.EqualFold(strings.TrimSpace(l), norm) {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if removed {
		if err := writeRaw(path, kept); err != nil {
			return false, err
		}
	}
	if kind.HasStrategies() && m.Locks != nil && orchestra.IsCustomDomain(norm) {
		if err := m.Locks.ClearEverywhere(norm, string(customProto)); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// Rename переименовывает запись, сохраняя позицию строки. Для KindCustom
// фиксации стратегий переносятся на новое имя во всех scope.
// ErrNotFound — старой записи нет; ErrExists — новая уже есть в списке.
func (m *Manager) Rename(kind Kind, oldEntry, newEntry string) error {
	oldN, err := Normalize(kind, oldEntry)
	if err != nil {
		return err
	}
	newN, err := Normalize(kind, newEntry)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, _ := m.Path(kind)
	lines, err := readRaw(path)
	if err != nil {
		return err
	}
	idx := -1
	for i, l := range lines {
		if !isEntry(l) {
			continue
		}
		t := strings.TrimSpace(l)
		if strings.EqualFold(t, oldN) && idx < 0 {
			idx = i
		} else if !strings.EqualFold(oldN, newN) && strings.EqualFold(t, newN) {
			return fmt.Errorf("%w: %s", ErrExists, newN)
		}
	}
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, oldN)
	}
	prev := lines[idx]
	lines[idx] = newN
	if err := writeRaw(path, lines); err != nil {
		return err
	}
	if oldN == newN || !kind.HasStrategies() || m.Locks == nil {
		return nil
	}
	if err := m.moveLocks(oldN, newN); err != nil {
		lines[idx] = prev
		_ = writeRaw(path, lines) // откат имени — лок не перенесён
		return err
	}
	return nil
}

// moveLocks переносит фиксации домена old на new во всех scope.
// При ошибке SetLock откатывает уже записанные new-локи и не трогает old.
func (m *Manager) moveLocks(oldN, newN string) error {
	scopes, err := m.Locks.ListScopes()
	if err != nil {
		return err
	}
	type moved struct {
		scope string
	}
	var done []moved
	for _, sc := range scopes {
		st, err := m.Locks.GetLock(sc, oldN, string(customProto))
		if err != nil || st.IsAuto() {
			continue
		}
		if err := m.Locks.SetLock(sc, newN, string(customProto), st); err != nil {
			for _, mvd := range done {
				_ = m.Locks.ClearLock(mvd.scope, newN, string(customProto))
			}
			return err
		}
		done = append(done, moved{scope: sc})
	}
	return m.Locks.ClearEverywhere(oldN, string(customProto))
}

// RestoreFromLocks возвращает в TCP_Custom домены, у которых есть фиксации
// стратегий в default-scope, но которых нет в списке (самолечение после
// затирания списка). Возвращает число восстановленных доменов.
func (m *Manager) RestoreFromLocks() (int, error) {
	if m.Locks == nil {
		return 0, nil
	}
	recs, err := m.Locks.ListLocks(orchestra.ScopeDefault)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	var domains []string
	for _, r := range recs {
		if !orchestra.IsCustomDomain(r.Profile) {
			continue
		}
		d := strings.ToLower(r.Profile)
		if !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	added := 0
	for _, d := range domains {
		_, ok, err := m.Add(KindCustom, d)
		if err != nil {
			continue // фиксация с некорректным именем — пропускаем
		}
		if ok {
			added++
		}
	}
	return added, nil
}
