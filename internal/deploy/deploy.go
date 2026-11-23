// Package deploy — самообновление zator из релизных tar.gz-архивов.
//
// Поток обновления:
//
//  1. CheckLatest скачивает latest.json (URL настраивается) и сравнивает
//     SHA сборки с установленной версией (version.env).
//  2. DownloadAsset скачивает архив варианта (core|webui|full) и проверяет
//     размер и sha256 из latest.json.
//  3. Unpack безопасно распаковывает архив в staging-каталог
//     (fetch.SafeUntar), а Apply сверяет sha256 всех файлов с манифестом
//     (extra_strats/cache/deploy/manifest.tsv) и только затем раскладывает
//     файлы по назначению. Классы файлов манифеста:
//     auto / payload — всегда заменяются (при совпадении sha256 пропускаются),
//     keep-if-exists — кладутся, только если файла нет (пользовательские списки).
//     При ошибке в середине применения заменённые файлы возвращаются обратно.
//
// Офлайн-режим: Z2R_OFFLINE=1 и/или ZATOR_BUNDLE_DIR (vendor/ и
// zator-*.tar.gz) отключают сетевые операции; бандл применяется через
// ApplyBundle.
package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/fsutil"
	"github.com/AloofLibra/zator/internal/paths"
)

// Источники и настройки по умолчанию.
const (
	// DefaultOwner и DefaultRepo — репозиторий релизов ZatorGO.
	DefaultOwner = "Koviand"
	DefaultRepo  = "zatorGO"
	// EnvOwner / EnvRepo переопределяют репозиторий релизов (как в лаунчере).
	EnvOwner = "ZATOR_OWNER"
	EnvRepo  = "ZATOR_REPO"
	// EnvLatestURL переопределяет адрес latest.json.
	EnvLatestURL = "ZATOR_LATEST_URL"
	// ManifestRel — путь манифеста внутри архива.
	ManifestRel = "extra_strats/cache/deploy/manifest.tsv"
	// VersionEnvRel — путь version.env внутри архива.
	VersionEnvRel = "extra_strats/cache/deploy/version.env"

	// Классы файлов манифеста.
	ClassAuto         = "auto"
	ClassKeepIfExists = "keep-if-exists"
	ClassPayload      = "payload"

	// Варианты архива.
	VariantCore  = "core"
	VariantWebUI = "webui"
	VariantFull  = "full"

	// SchemaVersion — поддерживаемая версия формата latest.json.
	SchemaVersion = 1

	maxLatestBytes = 1 << 20
)

// DefaultLatestURL — адрес latest.json по умолчанию (репозиторий Koviand/zatorGO).
var DefaultLatestURL = fetch.GitHubReleaseURL(DefaultOwner, DefaultRepo, "latest", "latest.json")

// ReleaseOwner / ReleaseRepo возвращают owner/repo релизов с учётом env.
func ReleaseOwner() string {
	if v := strings.TrimSpace(os.Getenv(EnvOwner)); v != "" {
		return v
	}
	return DefaultOwner
}

func ReleaseRepo() string {
	if v := strings.TrimSpace(os.Getenv(EnvRepo)); v != "" {
		return v
	}
	return DefaultRepo
}

// LatestJSONURL — URL latest.json для текущего owner/repo (или ZATOR_LATEST_URL).
func LatestJSONURL() string {
	if v := strings.TrimSpace(os.Getenv(EnvLatestURL)); v != "" {
		return v
	}
	return fetch.GitHubReleaseURL(ReleaseOwner(), ReleaseRepo(), "latest", "latest.json")
}

// Ошибки пакета.
var (
	// ErrOffline — сетевая операция в офлайн-режиме.
	ErrOffline = errors.New("deploy: офлайн-режим, сеть недоступна")
	// ErrInvalidLatest — latest.json не прошёл проверку.
	ErrInvalidLatest = errors.New("deploy: некорректный latest.json")
	// ErrUnknownVariant — неизвестный вариант архива.
	ErrUnknownVariant = errors.New("deploy: неизвестный вариант архива")
	// ErrNoManifest — в архиве нет манифеста.
	ErrNoManifest = errors.New("deploy: в архиве нет манифеста")
	// ErrBadManifest — манифест некорректен или содержит недопустимые пути.
	ErrBadManifest = errors.New("deploy: некорректный манифест")
	// ErrVerify — файл staging не совпал с sha256 манифеста.
	ErrVerify = errors.New("deploy: сверка staging не пройдена")
	// ErrSize — размер скачанного архива не совпал с latest.json.
	ErrSize = errors.New("deploy: размер архива не совпал")
)

// ValidVariant сообщает, что variant — core, webui или full.
func ValidVariant(v string) bool {
	return v == VariantCore || v == VariantWebUI || v == VariantFull
}

// ---------------------------------------------------------------------------
// latest.json

// Asset — архив релиза в latest.json.
type Asset struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	UnpackedSize int64  `json:"unpackedSize"`
	URL          string `json:"url,omitempty"`
}

// Latest — указатель последней сборки (latest.json).
type Latest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Release       string           `json:"release"`
	BuildDate     string           `json:"buildDate"`
	Commit        string           `json:"commit"`
	ZatorSHA      string           `json:"zatorSha"`
	WebUISHA      string           `json:"webuiSha"`
	ZatorDate     string           `json:"zatorDate"`
	WebUIDate     string           `json:"webuiDate"`
	Assets        map[string]Asset `json:"assets"`
}

var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ParseLatest разбирает и проверяет latest.json.
func ParseLatest(data []byte) (*Latest, error) {
	var l Latest
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLatest, err)
	}
	if l.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: schemaVersion %d", ErrInvalidLatest, l.SchemaVersion)
	}
	if strings.TrimSpace(l.Release) == "" {
		return nil, fmt.Errorf("%w: пустой release", ErrInvalidLatest)
	}
	for name, a := range l.Assets {
		if !ValidVariant(name) {
			continue
		}
		if a.Size < 0 || a.UnpackedSize < 0 || (a.SHA256 != "" && !sha256Re.MatchString(a.SHA256)) {
			return nil, fmt.Errorf("%w: ассет %s", ErrInvalidLatest, name)
		}
		if !safeBaseName(a.Name) {
			return nil, fmt.Errorf("%w: имя ассета %q", ErrInvalidLatest, a.Name)
		}
		if a.URL != "" {
			u, err := url.Parse(a.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, fmt.Errorf("%w: url ассета %s", ErrInvalidLatest, name)
			}
		}
	}
	return &l, nil
}

func safeBaseName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `/\`) && !strings.ContainsRune(n, 0) && filepath.Base(n) == n
}

// Asset возвращает ассет варианта.
func (l *Latest) Asset(variant string) (Asset, error) {
	if !ValidVariant(variant) {
		return Asset{}, fmt.Errorf("%w: %q", ErrUnknownVariant, variant)
	}
	a, ok := l.Assets[variant]
	if !ok || a.Name == "" || a.SHA256 == "" || a.Size <= 0 {
		return Asset{}, fmt.Errorf("%w: в latest.json нет полного ассета %q", ErrInvalidLatest, variant)
	}
	return a, nil
}

// ---------------------------------------------------------------------------
// Updater

// Updater выполняет проверку, загрузку и применение обновлений.
type Updater struct {
	Paths paths.Paths
	// Fetch — загрузчик (повторы, зеркала, sha256). По умолчанию fetch.NewClient().
	Fetch *fetch.Client
	// LatestURL — адрес latest.json (по умолчанию DefaultLatestURL или
	// ZATOR_LATEST_URL).
	LatestURL string
	// ScriptDest — путь, куда ставится запись манифеста с dest=/opt/z2r.sh.
	// Пусто — такие записи пропускаются.
	ScriptDest string
	// Remap преобразует относительный путь внутри /opt/zator в путь относительно
	// ZatorRoot (nil — без изменений). Пустой результат пропускает файл.
	Remap func(rel string) string
	// Offline отключает сетевые операции.
	Offline bool
	// WebUIInstalled сообщает, установлен ли веб-интерфейс (по умолчанию —
	// WebUIDir существует и не пуст).
	WebUIInstalled func() bool
	// Now — источник времени (по умолчанию time.Now).
	Now func() time.Time
}

// New создаёт обновлятор для раскладки p. Адрес latest.json берётся из
// ZATOR_LATEST_URL / ZATOR_OWNER+ZATOR_REPO, офлайн-флаг — из Z2R_OFFLINE.
func New(p paths.Paths) *Updater {
	return &Updater{
		Paths:     p,
		Fetch:     fetch.NewClient(),
		LatestURL: LatestJSONURL(),
		Offline:   DetectOffline(os.Getenv, "").Offline,
	}
}

func (u *Updater) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}

func (u *Updater) fetcher() *fetch.Client {
	if u.Fetch != nil {
		return u.Fetch
	}
	return fetch.NewClient()
}

func (u *Updater) latestURL() string {
	if u.LatestURL != "" {
		return u.LatestURL
	}
	return LatestJSONURL()
}

func (u *Updater) webUIInstalled() bool {
	if u.WebUIInstalled != nil {
		return u.WebUIInstalled()
	}
	ents, err := os.ReadDir(u.Paths.WebUIDir)
	return err == nil && len(ents) > 0
}

func readFileOpt(p string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// CheckResult — итог проверки обновлений.
type CheckResult struct {
	Latest  *Latest
	Current Version
	// UpdateZator — ядро устарело; UpdateWebUI — устарела веб-панель
	// (учитывается только при установленной панели).
	UpdateZator bool
	UpdateWebUI bool
	// VersionNewer — тег релиза новее установленной версии (справочно).
	VersionNewer bool
	CheckedAt    time.Time
}

// UpdateAvailable сообщает, что есть что обновлять.
func (r *CheckResult) UpdateAvailable() bool { return r != nil && (r.UpdateZator || r.UpdateWebUI) }

// CompareLatest сравнивает установленную версию с latest. Основной признак —
// несовпадение SHA содержимого (как в shell-версии); при неизвестном SHA ядра
// используется сравнение версий. SHA сравниваются только если заданы с обеих
// сторон — иначе GitHub API без SHA не даёт «вечный update».
func CompareLatest(cur Version, l *Latest, webUIInstalled bool) CheckResult {
	r := CheckResult{Latest: l, Current: cur}
	if l == nil {
		return r
	}
	switch {
	case cur.ZatorSHA != "" && l.ZatorSHA != "":
		r.UpdateZator = !strings.EqualFold(cur.ZatorSHA, l.ZatorSHA)
	case knownVersion(cur.ZatorVersion):
		r.UpdateZator = IsNewer(cur.ZatorVersion, l.Release)
	default:
		// Чистая установка без version.env — предложить обновление, если есть тег.
		r.UpdateZator = knownVersion(l.Release)
	}
	if webUIInstalled {
		switch {
		case cur.WebUISHA != "" && l.WebUISHA != "":
			r.UpdateWebUI = !strings.EqualFold(cur.WebUISHA, l.WebUISHA)
		case knownVersion(cur.WebUIVersion):
			r.UpdateWebUI = IsNewer(cur.WebUIVersion, l.Release)
		case knownVersion(cur.ZatorVersion):
			// В binary-релизах Web UI едет вместе с ядром.
			r.UpdateWebUI = IsNewer(cur.ZatorVersion, l.Release)
		default:
			r.UpdateWebUI = knownVersion(l.Release)
		}
	}
	r.VersionNewer = knownVersion(cur.ZatorVersion) && IsNewer(cur.ZatorVersion, l.Release)
	return r
}

func knownVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "unknown" && v != "branch"
}

// FetchLatest скачивает и разбирает latest.json. Если ассета нет — fallback
// на GitHub Releases API репозитория Koviand/zatorGO (или ZATOR_OWNER/REPO).
// В офлайн-режиме — ErrOffline.
func (u *Updater) FetchLatest(ctx context.Context) (*Latest, []byte, error) {
	if u.Offline {
		return nil, nil, ErrOffline
	}
	dir := Dir(u.Paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	tmp, err := os.CreateTemp(dir, "latest-*.json.part")
	if err != nil {
		return nil, nil, err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)
	if err := u.fetcher().Download(ctx, u.latestURL(), tmpName, ""); err == nil {
		f, err := os.Open(tmpName)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxLatestBytes+1))
		if err != nil {
			return nil, nil, err
		}
		if len(data) > maxLatestBytes {
			return nil, nil, fmt.Errorf("%w: слишком большой файл", ErrInvalidLatest)
		}
		l, err := ParseLatest(data)
		if err != nil {
			return nil, nil, err
		}
		return l, data, nil
	}
	// Нет latest.json в релизе — берём тег из GitHub API своего репозитория.
	l, err := u.FetchGitHubLatest(ctx)
	if err != nil {
		return nil, nil, err
	}
	raw, _ := json.Marshal(l)
	return l, raw, nil
}

// CheckLatest проверяет наличие обновления и кэширует latest.json.
func (u *Updater) CheckLatest(ctx context.Context) (*CheckResult, error) {
	l, raw, err := u.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	cur, err := ReadVersion(u.Paths)
	if err != nil {
		return nil, err
	}
	// Если version.env ещё нет — сравниваем с версией бинарника из status API
	// через ZATOR_VERSION env / файл VERSION недоступен здесь; оставляем
	// CompareLatest: без SHA и без версии обновиться предложит при любом релизе.
	res := CompareLatest(cur, l, u.webUIInstalled())
	res.CheckedAt = u.now()
	if len(raw) > 0 {
		_ = fsutil.AtomicWriteFile(filepath.Join(Dir(u.Paths), LatestCacheName), raw, 0o644)
	}
	return &res, nil
}

// AssetURL возвращает адрес архива ассета: url из latest.json либо
// <каталог latest.json>/<name>.
func (u *Updater) AssetURL(a Asset) (string, error) {
	if a.URL != "" {
		return a.URL, nil
	}
	base, err := url.Parse(u.latestURL())
	if err != nil {
		return "", err
	}
	base.Path = path.Join(path.Dir(base.Path), a.Name)
	base.RawQuery = ""
	return base.String(), nil
}

// DownloadAsset скачивает архив варианта в StateDir/deploy и проверяет размер
// и sha256 из latest.json. Возвращает путь к файлу.
func (u *Updater) DownloadAsset(ctx context.Context, l *Latest, variant string) (string, error) {
	if u.Offline {
		return "", ErrOffline
	}
	a, err := l.Asset(variant)
	if err != nil {
		return "", err
	}
	link, err := u.AssetURL(a)
	if err != nil {
		return "", err
	}
	dir := Dir(u.Paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, a.Name)
	if err := u.fetcher().Download(ctx, link, dest, a.SHA256); err != nil {
		return "", err
	}
	st, err := os.Stat(dest)
	if err != nil {
		return "", err
	}
	if st.Size() != a.Size {
		os.Remove(dest)
		return "", fmt.Errorf("%w: ожидалось %d, получено %d", ErrSize, a.Size, st.Size())
	}
	return dest, nil
}

// Unpack безопасно распаковывает архив в новый staging-каталог внутри
// StateDir/deploy и возвращает его путь (вызывающий удаляет каталог).
func (u *Updater) Unpack(archive string) (string, error) {
	dir := Dir(u.Paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(dir, "staging-")
	if err != nil {
		return "", err
	}
	if err := fetch.SafeUntar(archive, staging); err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	return staging, nil
}

// ---------------------------------------------------------------------------
// Манифест

// ManifestEntry — строка манифеста: path|dest|class|sha256|size|exec.
type ManifestEntry struct {
	Path   string
	Dest   string
	Class  string
	SHA256 string
	Size   int64
	Exec   bool
}

// ParseManifest разбирает manifest.tsv.
func ParseManifest(data []byte) ([]ManifestEntry, error) {
	var out []ManifestEntry
	seen := map[string]bool{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != 6 {
			return nil, fmt.Errorf("%w: строка %d: %d полей", ErrBadManifest, i+1, len(f))
		}
		rel, err := fetch.CleanEntryName(f[0])
		if err != nil || rel == "." {
			return nil, fmt.Errorf("%w: строка %d: путь %q", ErrBadManifest, i+1, f[0])
		}
		switch f[2] {
		case ClassAuto, ClassKeepIfExists, ClassPayload:
		default:
			return nil, fmt.Errorf("%w: строка %d: класс %q", ErrBadManifest, i+1, f[2])
		}
		if !sha256Re.MatchString(f[3]) {
			return nil, fmt.Errorf("%w: строка %d: sha256", ErrBadManifest, i+1)
		}
		var size int64
		if _, err := fmt.Sscanf(f[4], "%d", &size); err != nil || size < 0 {
			return nil, fmt.Errorf("%w: строка %d: размер %q", ErrBadManifest, i+1, f[4])
		}
		if f[5] != "0" && f[5] != "1" {
			return nil, fmt.Errorf("%w: строка %d: exec %q", ErrBadManifest, i+1, f[5])
		}
		if seen[rel] {
			return nil, fmt.Errorf("%w: дубликат пути %q", ErrBadManifest, rel)
		}
		seen[rel] = true
		out = append(out, ManifestEntry{
			Path: rel, Dest: f[1], Class: f[2], SHA256: strings.ToLower(f[3]), Size: size, Exec: f[5] == "1",
		})
	}
	return out, nil
}

// ScriptManifestDest — dest записи с лаунчером z2r.sh.
const ScriptManifestDest = "/opt/z2r.sh"

const zatorManifestPrefix = "/opt/zator/"

// resolveDest преобразует dest манифеста в путь на диске. skip=true — запись
// не применяется (нет ScriptDest или Remap вернул пусто).
func (u *Updater) resolveDest(dest string) (target string, skip bool, err error) {
	if dest == "" || strings.ContainsRune(dest, 0) || strings.Contains(dest, `\`) {
		return "", false, fmt.Errorf("%w: dest %q", ErrBadManifest, dest)
	}
	for _, seg := range strings.Split(dest, "/") {
		if seg == ".." {
			return "", false, fmt.Errorf("%w: dest %q", ErrBadManifest, dest)
		}
	}
	switch {
	case dest == ScriptManifestDest:
		if u.ScriptDest == "" {
			return "", true, nil
		}
		return u.ScriptDest, false, nil
	case strings.HasPrefix(dest, zatorManifestPrefix):
		rel := path.Clean(strings.TrimPrefix(dest, zatorManifestPrefix))
		if rel == "." || rel == "" || strings.HasPrefix(rel, "/") {
			return "", false, fmt.Errorf("%w: dest %q", ErrBadManifest, dest)
		}
		if u.Remap != nil {
			rel = u.Remap(rel)
			if rel == "" {
				return "", true, nil
			}
			if rel = path.Clean(rel); rel == "." || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") {
				return "", false, fmt.Errorf("%w: remap %q", ErrBadManifest, dest)
			}
		}
		root := u.Paths.ZatorRoot
		target = filepath.Join(root, filepath.FromSlash(rel))
		if r, err := filepath.Rel(root, target); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", false, fmt.Errorf("%w: dest %q", ErrBadManifest, dest)
		}
		return target, false, nil
	}
	return "", false, fmt.Errorf("%w: недопустимый dest %q", ErrBadManifest, dest)
}

// LoadManifest читает манифест из staging.
func LoadManifest(staging string) ([]ManifestEntry, error) {
	data, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(ManifestRel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoManifest
		}
		return nil, err
	}
	return ParseManifest(data)
}

// VerifyStaging сверяет sha256 и размер каждого файла манифеста в staging.
func VerifyStaging(staging string, entries []ManifestEntry) error {
	var bad []string
	for _, e := range entries {
		p := filepath.Join(staging, filepath.FromSlash(e.Path))
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			bad = append(bad, e.Path)
			continue
		}
		sum, err := fetch.SHA256File(p)
		if err != nil || sum != e.SHA256 || (e.Size > 0 && st.Size() != e.Size) {
			bad = append(bad, e.Path)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("%w: повреждено файлов %d из %d (%s)", ErrVerify, len(bad), len(entries), strings.Join(bad, ", "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Применение

// ApplyOptions настраивает Apply.
type ApplyOptions struct {
	// Tracking — значение TRACKING в version.env ("latest" или тег; пусто —
	// сохраняется прежнее, иначе latest).
	Tracking string
	// Meta — данные latest.json; используются для version.env, если в архиве
	// нет extra_strats/cache/deploy/version.env.
	Meta *Latest
}

// ApplyResult — итог применения.
type ApplyResult struct {
	Installed int // записано файлов
	Unchanged int // пропущено: sha256 совпал
	Kept      int // пропущено: keep-if-exists и файл есть
	Skipped   int // пропущено: dest вне раскладки (нет ScriptDest, Remap → "")
	// Files — пути записанных файлов.
	Files []string
	// HasCore и HasWebUI — что именно принёс архив.
	HasCore, HasWebUI bool
	// ScriptUpdated — обновлён лаунчер (ScriptDest).
	ScriptUpdated bool
	Version       Version
}

type undoOp struct {
	target string
	backup string // "" — файла не было
}

// Apply проверяет staging по манифесту и раскладывает файлы. До проверки всех
// файлов на диск ничего не пишется. При ошибке заменённые файлы возвращаются
// на место, созданные — удаляются.
func (u *Updater) Apply(staging string, opts ApplyOptions) (*ApplyResult, error) {
	entries, err := LoadManifest(staging)
	if err != nil {
		return nil, err
	}
	if err := VerifyStaging(staging, entries); err != nil {
		return nil, err
	}
	type planned struct {
		e      ManifestEntry
		target string
	}
	var plan []planned
	res := &ApplyResult{}
	for _, e := range entries {
		target, skip, err := u.resolveDest(e.Dest)
		if err != nil {
			return nil, err
		}
		if skip {
			res.Skipped++
			continue
		}
		if strings.HasPrefix(e.Path, "webui/") {
			res.HasWebUI = true
		} else {
			res.HasCore = true
		}
		plan = append(plan, planned{e, target})
	}

	backupDir, err := os.MkdirTemp(Dir(u.Paths), "rollback-")
	if err != nil {
		if mkErr := os.MkdirAll(Dir(u.Paths), 0o755); mkErr != nil {
			return nil, mkErr
		}
		if backupDir, err = os.MkdirTemp(Dir(u.Paths), "rollback-"); err != nil {
			return nil, err
		}
	}
	defer os.RemoveAll(backupDir)

	var undo []undoOp
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			op := undo[i]
			if op.backup == "" {
				os.Remove(op.target)
				continue
			}
			if data, err := os.ReadFile(op.backup); err == nil {
				_ = fsutil.AtomicWriteFile(op.target, data, 0)
			}
		}
	}

	for i, pl := range plan {
		e, target := pl.e, pl.target
		st, statErr := os.Lstat(target)
		exists := statErr == nil
		if exists && st.Mode()&os.ModeSymlink != 0 {
			rollback()
			return nil, fmt.Errorf("deploy: %s — символическая ссылка, замена отклонена", target)
		}
		if exists && e.Class == ClassKeepIfExists {
			res.Kept++
			continue
		}
		if exists && st.Mode().IsRegular() {
			if sum, err := fetch.SHA256File(target); err == nil && sum == e.SHA256 {
				res.Unchanged++
				continue
			}
		}
		if exists && !st.Mode().IsRegular() {
			rollback()
			return nil, fmt.Errorf("deploy: %s не является обычным файлом", target)
		}
		data, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(e.Path)))
		if err != nil {
			rollback()
			return nil, err
		}
		op := undoOp{target: target}
		if exists {
			op.backup = filepath.Join(backupDir, fmt.Sprintf("%d", i))
			old, err := os.ReadFile(target)
			if err != nil {
				rollback()
				return nil, err
			}
			if err := os.WriteFile(op.backup, old, 0o600); err != nil {
				rollback()
				return nil, err
			}
		}
		perm := os.FileMode(0o644)
		if e.Exec {
			perm = 0o755
		}
		undo = append(undo, op)
		if err := fsutil.AtomicWriteFile(target, data, perm); err != nil {
			rollback()
			return nil, fmt.Errorf("deploy: установка %s: %w", target, err)
		}
		if err := os.Chmod(target, perm); err != nil && perm == 0o755 {
			rollback()
			return nil, err
		}
		res.Installed++
		res.Files = append(res.Files, target)
		if u.ScriptDest != "" && target == u.ScriptDest {
			res.ScriptUpdated = true
		}
	}

	ver, err := u.mergeVersion(staging, res, opts)
	if err != nil {
		rollback()
		return nil, err
	}
	res.Version = ver
	if err := WriteVersion(u.Paths, ver); err != nil {
		rollback()
		return nil, err
	}
	return res, nil
}

// mergeVersion строит итоговую version.env: данные архива заменяют только те
// части (ядро / веб-панель), которые архив содержал.
func (u *Updater) mergeVersion(staging string, res *ApplyResult, opts ApplyOptions) (Version, error) {
	old, err := ReadVersion(u.Paths)
	if err != nil {
		return Version{}, err
	}
	var nv Version
	if data, err := readFileOpt(filepath.Join(staging, filepath.FromSlash(VersionEnvRel))); err != nil {
		return Version{}, err
	} else if data != nil {
		nv = ParseVersionEnv(string(data))
	} else if m := opts.Meta; m != nil {
		nv = Version{
			ZatorVersion: m.Release, ZatorDate: m.ZatorDate, ZatorCommit: m.Commit, ZatorSHA: m.ZatorSHA,
			WebUIVersion: m.Release, WebUIDate: m.WebUIDate, WebUISHA: m.WebUISHA,
		}
	}
	out := old
	if res.HasCore {
		out.ZatorVersion, out.ZatorDate, out.ZatorCommit, out.ZatorSHA = nv.ZatorVersion, nv.ZatorDate, nv.ZatorCommit, nv.ZatorSHA
	}
	if res.HasWebUI {
		out.WebUIVersion, out.WebUIDate, out.WebUISHA = nv.WebUIVersion, nv.WebUIDate, nv.WebUISHA
	}
	switch {
	case opts.Tracking != "":
		out.Tracking = opts.Tracking
	case out.Tracking == "":
		out.Tracking = TrackingLatest
	}
	return out, nil
}

// ApplyArchive распаковывает архив во временный staging и применяет его.
func (u *Updater) ApplyArchive(archive string, opts ApplyOptions) (*ApplyResult, error) {
	staging, err := u.Unpack(archive)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	return u.Apply(staging, opts)
}

// RunOptions настраивает Run.
type RunOptions struct {
	// Variant — core | webui | full (по умолчанию full).
	Variant string
	// Force применяет архив даже при отсутствии обновления.
	Force bool
	// Tracking — значение TRACKING (по умолчанию latest).
	Tracking string
	// KeepArchive не удаляет скачанный архив после успешного применения.
	KeepArchive bool
}

// Report — итог Run.
type Report struct {
	Check    *CheckResult
	Variant  string
	Archive  string
	UpToDate bool
	Apply    *ApplyResult
}

// Run выполняет полный цикл: проверка → загрузка → проверка sha256 →
// staging → применение. Без обновления (и без Force) возвращает Report с
// UpToDate=true и ничего не скачивает.
func (u *Updater) Run(ctx context.Context, opts RunOptions) (*Report, error) {
	variant := opts.Variant
	if variant == "" {
		variant = VariantFull
	}
	if !ValidVariant(variant) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownVariant, variant)
	}
	check, err := u.CheckLatest(ctx)
	if err != nil && check == nil {
		return nil, err
	}
	rep := &Report{Check: check, Variant: variant}
	if !check.UpdateAvailable() && !opts.Force {
		rep.UpToDate = true
		return rep, nil
	}
	// Релиз без tar/latest.json (только zator-linux-*): обновляем бинарник с GitHub.
	if check.Latest != nil {
		if _, aerr := check.Latest.Asset(variant); aerr != nil {
			ar, tag, berr := u.ApplyGitHubBinary(ctx)
			if berr != nil {
				return rep, berr
			}
			rep.Apply = ar
			rep.Archive = tag
			return rep, nil
		}
	}
	archive, err := u.DownloadAsset(ctx, check.Latest, variant)
	if err != nil {
		return rep, err
	}
	rep.Archive = archive
	if !opts.KeepArchive {
		defer os.Remove(archive)
	}
	res, err := u.ApplyArchive(archive, ApplyOptions{Tracking: opts.Tracking, Meta: check.Latest})
	if err != nil {
		return rep, err
	}
	rep.Apply = res
	return rep, nil
}
