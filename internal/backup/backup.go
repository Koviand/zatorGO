// Package backup создаёт, перечисляет, удаляет и восстанавливает резервные
// копии состояния zator: конфигурации zapret2, пользовательских списков
// доменов, фиксаций стратегий и переопределений orchestra.
//
// Архив — несжатый tar (z2r_backup_YYYYMMDD_HHMMSS.tar, как в shell-версии)
// со следующей раскладкой записей:
//
//	config                     — файл конфигурации zapret2
//	lists/<файл>               — пользовательские списки (ZatorRoot/lists)
//	orchestra/<файл>           — состояние orchestra (StateDir/orchestra)
//
// Архивы shell-версии (extra_strats/…) принимаются при восстановлении и
// отображаются на новую раскладку. Распаковка выполняется во временный
// каталог с проверкой имён (fetch.CleanEntryName); gzip-архивы
// распаковываются через fetch.SafeUntar.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/paths"
)

// Имена и расположение.
const (
	// ArchivePrefix и ArchiveExt образуют имя z2r_backup_<ts>.tar.
	ArchivePrefix = "z2r_backup_"
	ArchiveExt    = ".tar"
	// LegacyDir — каталог бэкапов shell-версии.
	LegacyDir = "/opt/zator_backup"
	// DirName — каталог бэкапов внутри StateDir.
	DirName = "backups"

	timeLayout = "20060102_150405"

	// Имена записей архива.
	entryConfig   = "config"
	entryLists    = "lists"
	entryOrch     = "orchestra"
	autoLockedTSV = "auto_locked.tsv"
)

// Ошибки пакета.
var (
	// ErrInvalidName — недопустимое имя архива.
	ErrInvalidName = errors.New("backup: недопустимое имя архива")
	// ErrNotFound — архив не найден.
	ErrNotFound = errors.New("backup: архив не найден")
	// ErrNotBackup — файл не похож на архив бэкапа zator.
	ErrNotBackup = errors.New("backup: файл не является бэкапом zator")
	// ErrFullBlocked — полное восстановление запрещено (контекст обновления).
	ErrFullBlocked = errors.New("backup: полное восстановление заблокировано")
	// ErrNoConfig — в архиве нет config.
	ErrNoConfig = errors.New("backup: в архиве нет config")
	// ErrNoLiveConfig — живой config zapret2 не найден (нужен умному переносу).
	ErrNoLiveConfig = errors.New("backup: живой config zapret2 не найден")
	// ErrInvalidMode — неизвестный режим восстановления.
	ErrInvalidMode = errors.New("backup: неизвестный режим восстановления")
)

var nameRe = regexp.MustCompile(`^` + ArchivePrefix + `[0-9]{8}_[0-9]{6}(_[0-9]+)?` + ArchiveExt + `$`)

// ValidName сообщает, что name — допустимое имя архива бэкапа.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// DefaultDir возвращает каталог бэкапов: LegacyDir для установки в корень по
// умолчанию, иначе StateDir/backups.
func DefaultDir(p paths.Paths) string {
	if p.ZatorRoot == filepath.Clean(paths.DefaultZatorRoot) {
		return filepath.FromSlash(LegacyDir)
	}
	return filepath.Join(p.StateDir, DirName)
}

// stateOrchestraFiles — файлы orchestra, входящие в бэкап (как в
// z2r_backup_state_files shell-версии).
func stateOrchestraFiles() []string {
	return []string{
		orchestra.LockedFileName,
		orchestra.LockedManualFileName,
		autoLockedTSV,
		orchestra.BlobOverrideFileName,
		orchestra.SNIOverrideFileName,
		orchestra.ModeOverrideFileName,
		orchestra.CloneSizeFileName,
	}
}

// stateListFiles — списки в ListsDir, входящие в бэкап всегда.
func stateListFiles() []string {
	return []string{
		"netrogat.txt",
		"netrogat_substrings.txt",
	}
}

// stateExtraFiles — пользовательские списки в ExtraStratsDir (с fallback в ListsDir).
func stateExtraFiles() []string {
	return []string{
		"TCP_Custom.txt",
		"TCP_RKN_domains_by_substring.txt",
	}
}

func (m *Manager) extraOrListsPath(name string) string {
	if m.Paths.ExtraStratsDir != "" {
		p := filepath.Join(m.Paths.ExtraStratsDir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return filepath.Join(m.Paths.ListsDir, name)
}

func (m *Manager) extraDest(name string) string {
	if m.Paths.ExtraStratsDir != "" {
		return filepath.Join(m.Paths.ExtraStratsDir, name)
	}
	return filepath.Join(m.Paths.ListsDir, name)
}

// Info описывает архив бэкапа.
type Info struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// Manager управляет бэкапами раскладки Paths в каталоге Dir.
type Manager struct {
	Paths paths.Paths
	// Dir — каталог архивов (по умолчанию DefaultDir(Paths)).
	Dir string
	// Now — источник времени (по умолчанию time.Now).
	Now func() time.Time
}

// New создаёт менеджер с каталогом по умолчанию.
func New(p paths.Paths) *Manager { return &Manager{Paths: p, Dir: DefaultDir(p)} }

// NewIn создаёт менеджер с явным каталогом архивов.
func NewIn(p paths.Paths, dir string) *Manager { return &Manager{Paths: p, Dir: dir} }

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) dir() string {
	if m.Dir != "" {
		return m.Dir
	}
	return DefaultDir(m.Paths)
}

// ---------------------------------------------------------------------------
// Создание

// CreateOptions настраивает создание бэкапа.
type CreateOptions struct {
	// IncludeLists добавляет в архив все файлы каталога lists (а не только
	// пользовательские списки доменов).
	IncludeLists bool
	// IncludeExtra добавляет все обычные файлы ExtraStratsDir как extra_strats/…
	// (UI-бэкап). Известные TCP_Custom / RKN substring и так попадают в lists/.
	IncludeExtra bool
	// SkipConfig не включает config zapret2 в архив.
	SkipConfig bool
}

type fileEntry struct {
	name string // имя записи архива
	src  string // путь на диске
}

// collect собирает список файлов бэкапа (существующие обычные файлы).
func (m *Manager) collect(opts CreateOptions) ([]fileEntry, error) {
	var out []fileEntry
	seen := map[string]bool{}
	add := func(name, src string) {
		if seen[name] {
			return
		}
		if st, err := os.Stat(src); err == nil && st.Mode().IsRegular() {
			seen[name] = true
			out = append(out, fileEntry{name: name, src: src})
		}
	}
	if !opts.SkipConfig {
		add(entryConfig, m.Paths.ZapretConfig)
	}
	for _, f := range stateListFiles() {
		add(entryLists+"/"+f, filepath.Join(m.Paths.ListsDir, f))
	}
	for _, f := range stateExtraFiles() {
		// В архиве канон — lists/<file>; источник — ExtraStratsDir при наличии.
		add(entryLists+"/"+f, m.extraOrListsPath(f))
	}
	if opts.IncludeLists {
		err := filepath.WalkDir(m.Paths.ListsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(m.Paths.ListsDir, p)
			if err != nil {
				return err
			}
			add(entryLists+"/"+filepath.ToSlash(rel), p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if opts.IncludeExtra && m.Paths.ExtraStratsDir != "" {
		err := filepath.WalkDir(m.Paths.ExtraStratsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(m.Paths.ExtraStratsDir, p)
			if err != nil {
				return err
			}
			base := path.Base(filepath.ToSlash(rel))
			// Уже учтены как lists/TCP_*.txt — не дублируем.
			if base == "TCP_Custom.txt" || base == "TCP_RKN_domains_by_substring.txt" {
				if !strings.Contains(filepath.ToSlash(rel), "/") {
					return nil
				}
			}
			add("extra_strats/"+filepath.ToSlash(rel), p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	orchDir := m.Paths.StateOrchestraDir()
	for _, f := range stateOrchestraFiles() {
		add(entryOrch+"/"+f, filepath.Join(orchDir, f))
	}
	// Файлы client-scope (scopes/mark_N.tsv).
	if ents, err := os.ReadDir(filepath.Join(orchDir, orchestra.ScopesDirName)); err == nil {
		for _, e := range ents {
			if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".tsv") {
				add(entryOrch+"/"+orchestra.ScopesDirName+"/"+e.Name(), filepath.Join(orchDir, orchestra.ScopesDirName, e.Name()))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// Create создаёт архив бэкапа и возвращает его описание. Каталог бэкапов
// создаётся при необходимости. Имя при коллизии получает суффикс _N.
func (m *Manager) Create(opts CreateOptions) (Info, error) {
	files, err := m.collect(opts)
	if err != nil {
		return Info{}, err
	}
	if len(files) == 0 {
		return Info{}, fmt.Errorf("backup: нечего сохранять (нет config, списков и состояния orchestra)")
	}
	dir := m.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Info{}, err
	}
	ts := m.now().Format(timeLayout)
	name := ArchivePrefix + ts + ArchiveExt
	for n := 1; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s%s_%d%s", ArchivePrefix, ts, n, ArchiveExt)
	}
	final := filepath.Join(dir, name)

	tmp, err := os.CreateTemp(dir, ".backup-*.tmp")
	if err != nil {
		return Info{}, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	tw := tar.NewWriter(tmp)
	mod := m.now()
	for _, f := range files {
		if err := writeTarFile(tw, f, mod); err != nil {
			return Info{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return Info{}, err
	}
	if err := tmp.Sync(); err != nil {
		return Info{}, err
	}
	if err := tmp.Close(); err != nil {
		return Info{}, err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return Info{}, err
	}
	_ = os.Remove(final)
	if err := os.Rename(tmpName, final); err != nil {
		return Info{}, err
	}
	ok = true
	return statInfo(final)
}

func writeTarFile(tw *tar.Writer, f fileEntry, mod time.Time) error {
	in, err := os.Open(f.src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:     f.name,
		Mode:     0o644,
		Size:     st.Size(),
		ModTime:  mod,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	n, err := io.Copy(tw, in)
	if err != nil {
		return err
	}
	if n != st.Size() {
		return fmt.Errorf("backup: %s изменился во время чтения", f.src)
	}
	return nil
}

func statInfo(p string) (Info, error) {
	st, err := os.Stat(p)
	if err != nil {
		return Info{}, err
	}
	return Info{Name: filepath.Base(p), Path: p, Size: st.Size(), ModTime: st.ModTime()}, nil
}

// ---------------------------------------------------------------------------
// Список, удаление, импорт

// List возвращает архивы каталога бэкапов, новейшие первыми (по имени).
// Отсутствующий каталог даёт пустой список.
func (m *Manager) List() ([]Info, error) {
	ents, err := os.ReadDir(m.dir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range ents {
		if !e.Type().IsRegular() || !ValidName(e.Name()) {
			continue
		}
		info, err := statInfo(filepath.Join(m.dir(), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Resolve возвращает путь архива по имени, проверяя имя (без каталогов).
func (m *Manager) Resolve(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	p := filepath.Join(m.dir(), name)
	st, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return p, nil
}

// Delete удаляет архив по имени.
func (m *Manager) Delete(name string) error {
	p, err := m.Resolve(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Contents возвращает имена записей архива (после проверки безопасности).
func (m *Manager) Contents(name string) ([]string, error) {
	p, err := m.Resolve(name)
	if err != nil {
		return nil, err
	}
	return listArchive(p)
}

// Import проверяет внешний архив и копирует его в каталог бэкапов. origName
// сохраняется, если подходит под шаблон и свободен; иначе имя строится из
// текущего времени. Возвращает итоговое имя.
func (m *Manager) Import(src, origName string) (string, error) {
	names, err := listArchive(src)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotBackup, err)
	}
	if !looksLikeBackup(names) {
		return "", ErrNotBackup
	}
	dir := m.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := ""
	if ValidName(origName) {
		if _, err := os.Stat(filepath.Join(dir, origName)); errors.Is(err, os.ErrNotExist) {
			name = origName
		}
	}
	if name == "" {
		ts := m.now().Format(timeLayout)
		name = ArchivePrefix + ts + ArchiveExt
		for n := 1; ; n++ {
			if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
				break
			}
			name = fmt.Sprintf("%s%s_%d%s", ArchivePrefix, ts, n, ArchiveExt)
		}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := fsutil.AtomicWriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

func looksLikeBackup(names []string) bool {
	for _, n := range names {
		if n == entryConfig || strings.HasPrefix(n, entryLists+"/") || strings.HasPrefix(n, entryOrch+"/") ||
			strings.HasPrefix(n, "extra_strats/") || strings.HasPrefix(n, "lists/") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Безопасная распаковка

// openTar открывает tar либо tar.gz (определяется по сигнатуре gzip).
func openTar(archive string) (*tar.Reader, io.Closer, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, nil, err
	}
	var magic [2]byte
	n, _ := io.ReadFull(f, magic[:])
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, nil, err
	}
	if n == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		return tar.NewReader(gz), multiCloser{gz, f}, nil
	}
	return tar.NewReader(f), f, nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var first error
	for _, c := range m {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func isGzip(archive string) bool {
	f, err := os.Open(archive)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [2]byte
	n, _ := io.ReadFull(f, magic[:])
	return n == 2 && magic[0] == 0x1f && magic[1] == 0x8b
}

// listArchive возвращает безопасные имена записей (обычные файлы и каталоги).
func listArchive(archive string) ([]string, error) {
	if isGzip(archive) {
		return fetch.ListTarGz(archive)
	}
	tr, c, err := openTar(archive)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var out []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		name, err := fetch.CleanEntryName(hdr.Name)
		if err != nil {
			return nil, err
		}
		if name != "." {
			out = append(out, name)
		}
	}
}

// extract безопасно распаковывает архив в dest: только обычные файлы и
// каталоги, имена проверяются fetch.CleanEntryName, размер файла ограничен
// fetch.MaxFileSize. Ссылки и спецфайлы отвергаются.
func extract(archive, dest string) error {
	if isGzip(archive) {
		return fetch.SafeUntar(archive, dest)
	}
	tr, c, err := openTar(archive)
	if err != nil {
		return err
	}
	defer c.Close()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := fetch.CleanEntryName(hdr.Name)
		if err != nil {
			return err
		}
		if name == "." {
			continue
		}
		target := filepath.Join(absDest, filepath.FromSlash(name))
		if rel, err := filepath.Rel(absDest, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %q", fetch.ErrUnsafePath, hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size > fetch.MaxFileSize {
				return fmt.Errorf("backup: файл %q слишком большой (%d байт)", hdr.Name, hdr.Size)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, fetch.MaxFileSize)); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: недопустимый тип записи %q (%c)", fetch.ErrUnsafePath, hdr.Name, hdr.Typeflag)
		}
	}
}

// ---------------------------------------------------------------------------
// Восстановление

// Mode — режим восстановления.
type Mode int

const (
	// ModeFull — config + списки + состояние orchestra.
	ModeFull Mode = iota + 1
	// ModeLists — только списки доменов и состояние orchestra (config не трогается).
	ModeLists
	// ModeSmart — умный перенос: живой config не заменяется, из бэкапа
	// переносятся порты, FWTYPE/FLOWOFFLOAD/MODE_FILTER, client-scope и флаг
	// авторотации; плюс списки и состояние orchestra.
	ModeSmart
)

// String возвращает имя режима.
func (m Mode) String() string {
	switch m {
	case ModeFull:
		return "full"
	case ModeLists:
		return "lists"
	case ModeSmart:
		return "smart"
	}
	return "unknown"
}

// ParseMode разбирает имя режима (full|lists|smart или 1|2|3).
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full", "1":
		return ModeFull, nil
	case "lists", "2":
		return ModeLists, nil
	case "smart", "3":
		return ModeSmart, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrInvalidMode, s)
}

// RestoreOptions настраивает восстановление.
type RestoreOptions struct {
	Mode Mode
	// BlockFull запрещает ModeFull (после обновления, чтобы архив не затёр
	// свежий config).
	BlockFull bool
}

// Result — итог восстановления.
type Result struct {
	Mode Mode
	// ConfigReplaced — живой config заменён файлом из архива (ModeFull).
	ConfigReplaced bool
	// SmartApplied — имена переменных/флагов, перенесённых в живой config
	// (ModeSmart).
	SmartApplied []string
	// Files — восстановленные файлы (пути на диске).
	Files []string
}

// smartVars — переменные config, переносимые умным режимом как есть.
var smartVars = []string{
	"NFQWS2_PORTS_TCP", "NFQWS2_PORTS_UDP",
	"FWTYPE", "FLOWOFFLOAD", "MODE_FILTER",
	"CLIENT_SCOPE_ENABLE", "CLIENT_SCOPE_MARK_MASK", "CLIENT_SCOPE_MARK_SHIFT", "CLIENT_SCOPE_MARK_MAX",
	"CLIENT_SCOPE_CHAIN", "CLIENT_SCOPE_TABLE",
}

// Restore восстанавливает состояние из архива по имени.
func (m *Manager) Restore(name string, opts RestoreOptions) (*Result, error) {
	p, err := m.Resolve(name)
	if err != nil {
		return nil, err
	}
	return m.RestoreFile(p, opts)
}

// RestoreFile восстанавливает состояние из архива по пути (tar либо tar.gz).
// Остановка и запуск сервиса — на стороне вызывающего.
// Package C: перед записью снимается snapshot затронутых путей; при ошибке — откат.
func (m *Manager) RestoreFile(archive string, opts RestoreOptions) (*Result, error) {
	switch opts.Mode {
	case ModeFull, ModeLists, ModeSmart:
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidMode, int(opts.Mode))
	}
	if opts.Mode == ModeFull && opts.BlockFull {
		return nil, ErrFullBlocked
	}

	tmp, err := os.MkdirTemp("", "z2r_restore_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := extract(archive, tmp); err != nil {
		return nil, fmt.Errorf("backup: распаковка %s: %w", filepath.Base(archive), err)
	}

	snapRoot, err := os.MkdirTemp("", "z2r_restore_snap_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(snapRoot)

	res := &Result{Mode: opts.Mode}
	oldCfg := filepath.Join(tmp, entryConfig)
	var undo []snapEntry

	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			e := undo[i]
			if e.missing {
				_ = os.Remove(e.dst)
				continue
			}
			data, err := os.ReadFile(e.snap)
			if err != nil {
				continue
			}
			_ = fsutil.AtomicWriteFile(e.dst, data, 0o644)
		}
	}

	snapshot := func(dst string) error {
		se := snapEntry{dst: dst}
		data, err := os.ReadFile(dst)
		if errors.Is(err, os.ErrNotExist) {
			se.missing = true
			undo = append(undo, se)
			return nil
		}
		if err != nil {
			return err
		}
		se.snap = filepath.Join(snapRoot, fmt.Sprintf("%d.bin", len(undo)))
		if err := os.WriteFile(se.snap, data, 0o644); err != nil {
			return err
		}
		undo = append(undo, se)
		return nil
	}

	writeLocked := func(dst string, data []byte) error {
		if err := snapshot(dst); err != nil {
			return err
		}
		return fsutil.WithDirLock(dst, 0, 0, func() error {
			return fsutil.AtomicWriteFile(dst, data, 0o644)
		})
	}

	// Config (до записи файлов: ошибка не должна оставить частичное состояние).
	switch opts.Mode {
	case ModeFull:
		data, err := os.ReadFile(oldCfg)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrNoConfig
			}
			return nil, err
		}
		if err := config.ValidateSourceSafe(string(data)); err != nil {
			return nil, fmt.Errorf("backup: опасный config в архиве: %w", err)
		}
		if err := writeLocked(m.Paths.ZapretConfig, data); err != nil {
			rollback()
			return nil, err
		}
		res.ConfigReplaced = true
	case ModeSmart:
		if err := snapshot(m.Paths.ZapretConfig); err != nil {
			return nil, err
		}
		applied, err := m.smartApply(oldCfg, m.Paths.ZapretConfig)
		if err != nil {
			rollback()
			return nil, err
		}
		res.SmartApplied = applied
	}

	files, err := m.restoreStateSnap(tmp, snapshot)
	res.Files = files
	if err != nil {
		rollback()
		return res, err
	}
	return res, nil
}

type snapEntry struct {
	dst     string
	snap    string
	missing bool
}

// restoreStateSnap как restoreState, но перед каждой записью вызывает snapshot.
func (m *Manager) restoreStateSnap(root string, snapshot func(string) error) ([]string, error) {
	var restored []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := m.destFor(filepath.ToSlash(rel))
		if dst == "" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := snapshot(dst); err != nil {
			return err
		}
		if err := fsutil.AtomicWriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("backup: восстановление %s: %w", rel, err)
		}
		restored = append(restored, dst)
		return nil
	})
	sort.Strings(restored)
	return restored, err
}

// destFor отображает имя записи архива на путь на диске ("" — игнорировать).
func (m *Manager) destFor(name string) string {
	name = path.Clean(name)
	switch {
	case strings.HasPrefix(name, entryLists+"/"):
		rel := strings.TrimPrefix(name, entryLists+"/")
		base := path.Base(rel)
		if base == "TCP_Custom.txt" || base == "TCP_RKN_domains_by_substring.txt" {
			return m.extraDest(base)
		}
		return filepath.Join(m.Paths.ListsDir, filepath.FromSlash(rel))
	case strings.HasPrefix(name, entryOrch+"/"):
		return filepath.Join(m.Paths.StateOrchestraDir(), filepath.FromSlash(strings.TrimPrefix(name, entryOrch+"/")))
	case strings.HasPrefix(name, "orchestra_state/"):
		// Старый webui-layout.
		return filepath.Join(m.Paths.StateOrchestraDir(), filepath.FromSlash(strings.TrimPrefix(name, "orchestra_state/")))
	// Раскладка shell-версии / webui.
	case strings.HasPrefix(name, "extra_strats/cache/orchestra/"):
		return filepath.Join(m.Paths.StateOrchestraDir(), filepath.FromSlash(strings.TrimPrefix(name, "extra_strats/cache/orchestra/")))
	case name == "extra_strats/TCP_Custom.txt" || name == "extra_strats/TCP_RKN_domains_by_substring.txt":
		return m.extraDest(path.Base(name))
	case strings.HasPrefix(name, "extra_strats/"):
		return filepath.Join(m.Paths.ExtraStratsDir, filepath.FromSlash(strings.TrimPrefix(name, "extra_strats/")))
	}
	return ""
}

// restoreState копирует списки и состояние orchestra из распакованного
// архива на их места.
func (m *Manager) restoreState(root string) ([]string, error) {
	var restored []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := m.destFor(filepath.ToSlash(rel))
		if dst == "" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := fsutil.AtomicWriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("backup: восстановление %s: %w", rel, err)
		}
		restored = append(restored, dst)
		return nil
	})
	sort.Strings(restored)
	return restored, err
}

// smartApply переносит настройки из config бэкапа (oldPath) в живой config
// (livePath), не заменяя его. Возвращает имена применённых настроек.
func (m *Manager) smartApply(oldPath, livePath string) ([]string, error) {
	old, err := config.Load(oldPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoConfig
		}
		return nil, err
	}
	live, err := config.Load(livePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoLiveConfig
		}
		return nil, err
	}
	var applied []string
	for _, name := range smartVars {
		raw, ok := old.GetRaw(name)
		if !ok {
			continue
		}
		if cur, has := live.GetRaw(name); has && cur == raw {
			continue
		}
		if err := live.Set(name, raw); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	// Флаг авторотации (#Z2R_AUTO_MODE).
	if oldOn, ok := old.AutoModeFlag(); ok {
		if liveOn, ok := live.AutoModeFlag(); ok && liveOn != oldOn {
			if err := live.SetAutoModeFlag(oldOn); err != nil {
				return nil, err
			}
			applied = append(applied, "AUTO_MODE")
		}
	}
	if len(applied) > 0 {
		if err := live.Save(); err != nil {
			return nil, err
		}
	}
	return applied, nil
}
