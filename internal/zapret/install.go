// Package zapret — загрузка и установка бинарников zapret2 (nfqws2, ip2net,
// mdig) собственным установщиком, без install_easy.sh.
package zapret

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

const (
	// BinNfqws2 и далее — имена устанавливаемых бинарников.
	BinNfqws2 = "nfqws2"
	BinIP2Net = "ip2net"
	BinMdig   = "mdig"

	// VersionFile — файл-маркер установленной сборки в корне zapret2.
	VersionFile = "zator-zapret.version"
)

// binLinks — расположение ссылок на бинарники в стандартной раскладке
// zapret2 (nfq2/nfqws2, ip2net/ip2net, mdig/mdig).
var binLinks = map[string]string{
	BinNfqws2: "nfq2",
	BinIP2Net: "ip2net",
	BinMdig:   "mdig",
}

// Options — параметры установки.
type Options struct {
	// Root — каталог zapret2 (обязателен для DownloadAndInstall; Install
	// подставляет paths.ZapretRoot).
	Root string
	// Arch — архитектура. Пусто — берётся из платформы.
	Arch platform.Arch
	// Source — источник; пусто — DefaultSource (форк).
	Source Source
	// Version — версия без префикса «v»; пусто — DefaultVer.
	Version string
	// Variant — вариант архива; пусто — выбирается по платформе.
	Variant Variant
	// OpenWRT — признак платформы OpenWRT для VariantAuto
	// (Install выставляет сам).
	OpenWRT bool

	// ReleaseBase — переопределение базы релизов (<base>/v<ver>/<asset>).
	ReleaseBase string
	// MirrorBase — зеркало релизов (аналог ZAPRET2_RELEASE_MIRROR_BASE).
	MirrorBase string
	// SHA256 — ожидаемая контрольная сумма архива. Если пусто — при онлайн-установке
	// сумма берётся из sha256sum.txt релиза (fail-closed).
	SHA256 string
	// AllowInsecure — разрешить установку без SHA256 (только осознанный bypass).
	AllowInsecure bool

	// IncludeLua — дополнительно распаковать lua/ из архива
	// (zapret-lib.lua, zapret-antidpi.lua и т.д.).
	IncludeLua bool
	// Force — переустановить, даже если такая же сборка уже стоит.
	Force bool

	// LocalArchive — путь к локальному tar.gz; если задан, сеть не нужна.
	LocalArchive string
	// VendorDir — каталог с архивами релизов (имя файла = AssetName).
	// Также проверяются $ZATOR_VENDOR и <parent(Root)>/vendor.
	VendorDir string
	// Offline — запретить загрузку по сети (ошибка, если локального архива нет).
	Offline bool

	// Client — HTTP-клиент; nil — fetch.NewClient().
	Client *fetch.Client
	// TmpDir — каталог для временных файлов; пусто — рядом с Root.
	TmpDir string
	// Logf — необязательный журнал.
	Logf func(format string, args ...any)
}

// Result — итог установки.
type Result struct {
	Root     string
	Source   Source
	Version  string
	ArchDir  string            // например linux-arm64
	URL      string            // адрес, указанный для загрузки (до зеркал)
	Binaries map[string]string // имя → путь в Root/binaries/<arch>/
	Missing  []string          // необязательные бинарники, которых нет в архиве
	Skipped  bool              // такая же сборка уже установлена
}

// Install — точка входа для internal/app: устанавливает zapret2 в
// p.ZapretRoot для платформы plat.
func Install(ctx context.Context, p paths.Paths, plat platform.Platform, opts Options) (*Result, error) {
	opts.Root = p.ZapretRoot
	if opts.Arch == "" {
		opts.Arch = plat.Arch
	}
	if opts.Variant == VariantAuto && plat.Kind == platform.KindOpenWRT {
		opts.OpenWRT = true
	}
	return DownloadAndInstall(ctx, opts)
}

func (o *Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func (o *Options) embedded() bool {
	switch o.Variant {
	case VariantOpenWRTEmbedded:
		return true
	case VariantStandard:
		return false
	}
	return o.OpenWRT
}

// DownloadAndInstall скачивает архив релиза, извлекает nfqws2, ip2net и mdig
// только для нужной архитектуры и создаёт минимальную раскладку в Root.
func DownloadAndInstall(ctx context.Context, opts Options) (*Result, error) {
	if strings.TrimSpace(opts.Root) == "" {
		return nil, errors.New("zapret: не задан Root")
	}
	if opts.Source == "" {
		opts.Source = DefaultSource
	}
	if !opts.Source.Valid() {
		return nil, fmt.Errorf("zapret: неизвестный источник %q", opts.Source)
	}
	ver, err := ResolveVersion(ctx, opts.Source, opts.Version, nil)
	if err != nil {
		return nil, err
	}
	dirs, err := ArchDirs(opts.Arch)
	if err != nil {
		return nil, err
	}
	root := filepath.Clean(opts.Root)
	embedded := opts.embedded()
	url := ReleaseURL(opts.Source, opts.ReleaseBase, ver, embedded)

	res := &Result{Root: root, Source: opts.Source, Version: ver, URL: url}

	if !opts.Force {
		if info, err := ReadInstalled(root); err == nil &&
			info.Version == ver && info.Source == opts.Source && containsString(dirs, info.ArchDir) &&
			fileExists(filepath.Join(root, "binaries", info.ArchDir, BinNfqws2)) {
			res.ArchDir = info.ArchDir
			res.Binaries = binaryPaths(root, info.ArchDir)
			res.Skipped = true
			opts.logf("zapret2 %s (%s) уже установлен", ver, info.ArchDir)
			return res, nil
		}
	}

	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("zapret: %w", err)
	}
	tmpBase := opts.TmpDir
	if tmpBase == "" {
		tmpBase = parent
	}
	if err := os.MkdirAll(tmpBase, 0o755); err != nil {
		return nil, fmt.Errorf("zapret: %w", err)
	}
	work, err := os.MkdirTemp(tmpBase, ".zapret2-install-")
	if err != nil {
		return nil, fmt.Errorf("zapret: %w", err)
	}
	defer os.RemoveAll(work)

	client := opts.Client
	if client == nil {
		client = fetch.NewClient()
	}
	mirrorBase := opts.MirrorBase
	if mirrorBase == "" {
		mirrorBase = os.Getenv(EnvReleaseMirrorBase)
	}
	if mirrorBase != "" {
		c := *client
		c.Mirrors = append(append([]fetch.Mirror(nil), client.Mirrors...), fetch.ReleaseMirror(mirrorBase))
		client = &c
	}

	asset := AssetName(ver, embedded)
	archive := filepath.Join(work, asset)
	offline := opts.Offline || envTruthy("Z2R_OFFLINE") || envTruthy("ZATOR_OFFLINE")
	local, hasLocal := resolveLocalArchive(opts, asset)

	// Апстрим публикует sha256sum.txt с per-file суммами binaries/…, а не хешем tar.gz.
	// Поэтому: сумма архива — если есть; иначе после распаковки сверяем nfqws2 (+ ip2net/mdig).
	var sumIdx SumIndex
	verifyBins := false
	if opts.SHA256 == "" && !opts.AllowInsecure {
		if hasLocal {
			if sum, ok := resolveLocalArchiveSHA256(local, asset); ok {
				opts.SHA256 = sum
				opts.logf("SHA256 из локального sidecar: %s…", sum[:12])
			} else if offline {
				return nil, fmt.Errorf("zapret: офлайн-установка требует Options.SHA256, файл %s.sha256 или sha256sum.txt рядом с архивом (или AllowInsecure)", filepath.Base(local))
			}
		}
		if opts.SHA256 == "" && !offline {
			idx, err := fetchSumIndex(ctx, client, url)
			if err != nil {
				return nil, fmt.Errorf("zapret: SHA256 обязателен (sha256sum.txt): %w", err)
			}
			sumIdx = idx
			if sum := idx.ArchiveSum(asset); sum != "" {
				opts.SHA256 = sum
				opts.logf("SHA256 архива из sha256sum.txt: %s…", sum[:12])
			} else if idx.HasBinarySums() {
				verifyBins = true
				opts.logf("sha256sum.txt: per-file суммы (проверка после распаковки)")
			} else {
				return nil, fmt.Errorf("zapret: в sha256sum.txt нет суммы для %s и нет binaries/*", asset)
			}
		}
	}
	if opts.SHA256 == "" && opts.AllowInsecure {
		opts.logf("предупреждение: установка без проверки SHA256 (AllowInsecure)")
	}

	if hasLocal {
		opts.logf("локальный архив %s", local)
		if err := copyFile(local, archive, 0o644); err != nil {
			return nil, fmt.Errorf("zapret: копирование локального архива: %w", err)
		}
		if opts.SHA256 != "" {
			if sum, err := fetch.SHA256File(archive); err != nil {
				return nil, err
			} else if !strings.EqualFold(sum, opts.SHA256) {
				return nil, fmt.Errorf("zapret: SHA256 локального архива не совпадает")
			}
		}
		res.URL = "file://" + filepath.ToSlash(local)
	} else {
		if offline {
			return nil, fmt.Errorf("zapret: офлайн-режим, нет локального архива %s (положите в vendor/ или задайте LocalArchive)", asset)
		}
		opts.logf("загрузка %s", url)
		if err := client.Download(ctx, url, archive, opts.SHA256); err != nil {
			return nil, fmt.Errorf("zapret: загрузка архива: %w", err)
		}
	}

	names, err := fetch.ListTarGz(archive)
	if err != nil {
		return nil, fmt.Errorf("zapret: %w", err)
	}
	archDir, found := pickArchDir(names, dirs)
	if archDir == "" {
		return nil, fmt.Errorf("zapret: в архиве нет бинарников для %s (нужны %s)", opts.Arch, strings.Join(dirs, ", "))
	}
	if !found[BinNfqws2] {
		return nil, fmt.Errorf("zapret: в binaries/%s нет %s", archDir, BinNfqws2)
	}
	res.ArchDir = archDir
	for _, b := range []string{BinIP2Net, BinMdig} {
		if !found[b] {
			res.Missing = append(res.Missing, b)
		}
	}

	stage := filepath.Join(work, "stage")
	// Lua/common/init.d нужны для запуска nfqws2 и firewall (не только бинарники).
	includeLua := opts.IncludeLua
	if !includeLua {
		includeLua = true // по умолчанию всегда тянем upstream lua
	}
	err = fetch.SafeUntarFiltered(archive, stage, func(name string, mode os.FileMode) (string, bool) {
		if mode.IsDir() || !mode.IsRegular() {
			return "", false
		}
		return stageRel(name, archDir, includeLua)
	})
	if err != nil {
		return nil, fmt.Errorf("zapret: распаковка: %w", err)
	}
	if !fileExists(filepath.Join(stage, "binaries", archDir, BinNfqws2)) {
		return nil, fmt.Errorf("zapret: %s не распакован", BinNfqws2)
	}
	if verifyBins {
		if err := verifyStagedBinaries(stage, archDir, sumIdx); err != nil {
			return nil, err
		}
	}

	if err := commit(root, stage, archDir, opts.IncludeLua); err != nil {
		return nil, err
	}
	if err := ensureLayout(root); err != nil {
		return nil, err
	}
	if err := linkBinaries(root, archDir); err != nil {
		return nil, err
	}
	if err := writeInstalled(root, Installed{Version: ver, Source: opts.Source, ArchDir: archDir}); err != nil {
		return nil, err
	}
	res.Binaries = binaryPaths(root, archDir)
	opts.logf("zapret2 %s (%s) установлен в %s", ver, archDir, root)
	return res, nil
}

// pickArchDir выбирает первый доступный каталог из candidates и возвращает
// набор найденных в нём бинарников.
func pickArchDir(names, candidates []string) (string, map[string]bool) {
	for _, dir := range candidates {
		found := map[string]bool{}
		for _, n := range names {
			if d, b, ok := splitBinaryEntry(n); ok && d == dir {
				found[b] = true
			}
		}
		if len(found) > 0 {
			return dir, found
		}
	}
	return "", nil
}

// splitBinaryEntry разбирает имя вида [<top>/]binaries/<dir>/<bin>.
func splitBinaryEntry(name string) (dir, bin string, ok bool) {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		if p == "binaries" && len(parts) == i+3 && i <= 1 {
			switch parts[i+2] {
			case BinNfqws2, BinIP2Net, BinMdig:
				return parts[i+1], parts[i+2], true
			}
		}
	}
	return "", "", false
}

// stageRel — отображение записи архива в относительный путь внутри stage.
func stageRel(name, archDir string, lua bool) (string, bool) {
	if d, b, ok := splitBinaryEntry(name); ok && d == archDir {
		return path.Join("binaries", archDir, b), true
	}
	parts := strings.Split(name, "/")
	// Срезаем возможный верхний каталог zapret2-vX/
	start := 0
	if len(parts) > 1 && strings.HasPrefix(parts[0], "zapret2") {
		start = 1
	}
	if start >= len(parts) {
		return "", false
	}
	top := parts[start]
	switch top {
	case "lua":
		if !lua || len(parts) <= start+1 {
			return "", false
		}
		return path.Join(parts[start:]...), true
	case "common", "init.d", "ipset":
		return path.Join(parts[start:]...), true
	case "files":
		// files/fake из апстрима — запасной набор блобов
		if len(parts) > start+1 && parts[start+1] == "fake" {
			return path.Join(parts[start:]...), true
		}
	}
	return "", false
}

// commit переносит распакованное из stage в root, заменяя только
// binaries/ (и lua/ при IncludeLua); остальное содержимое root не трогает.
func commit(root, stage, archDir string, lua bool) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("zapret: %w", err)
	}
	groups := []string{"binaries", "common", "init.d", "ipset", "files"}
	if lua {
		groups = append(groups, "lua")
	}
	for _, g := range groups {
		src := filepath.Join(stage, g)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(root, g)
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("zapret: %w", err)
		}
		if err := os.Rename(src, dst); err != nil {
			if err := copyTree(src, dst); err != nil {
				return fmt.Errorf("zapret: %w", err)
			}
		}
	}
	for _, name := range []string{BinNfqws2, BinIP2Net, BinMdig} {
		p := filepath.Join(root, "binaries", archDir, name)
		if fileExists(p) {
			if err := os.Chmod(p, 0o755); err != nil {
				return fmt.Errorf("zapret: %w", err)
			}
		}
	}
	return nil
}

// ensureLayout создаёт минимальную раскладку каталогов zapret2.
func ensureLayout(root string) error {
	for _, d := range LayoutDirs(root) {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("zapret: %w", err)
		}
	}
	return nil
}

// LayoutDirs — каталоги минимальной раскладки zapret2.
func LayoutDirs(root string) []string {
	return []string{
		root,
		filepath.Join(root, "binaries"),
		filepath.Join(root, "nfq2"),
		filepath.Join(root, "ip2net"),
		filepath.Join(root, "mdig"),
		filepath.Join(root, "lua"),
		filepath.Join(root, "ipset"),
		filepath.Join(root, "files", "fake"),
		filepath.Join(root, "init.d", "sysv", "custom.d"),
		filepath.Join(root, "init.d", "openwrt", "custom.d"),
	}
}

// linkBinaries создаёт nfq2/nfqws2, ip2net/ip2net, mdig/mdig как символические
// ссылки на binaries/<arch>/..., а там, где ссылки недоступны, — копии.
func linkBinaries(root, archDir string) error {
	for bin, dir := range binLinks {
		src := filepath.Join(root, "binaries", archDir, bin)
		if !fileExists(src) {
			continue
		}
		link := filepath.Join(root, dir, bin)
		_ = os.Remove(link)
		rel := filepath.Join("..", "binaries", archDir, bin)
		if err := os.Symlink(rel, link); err == nil {
			continue
		}
		if err := copyFile(src, link, 0o755); err != nil {
			return fmt.Errorf("zapret: %w", err)
		}
	}
	return nil
}

func binaryPaths(root, archDir string) map[string]string {
	out := map[string]string{}
	for _, b := range []string{BinNfqws2, BinIP2Net, BinMdig} {
		p := filepath.Join(root, "binaries", archDir, b)
		if fileExists(p) {
			out[b] = p
		}
	}
	return out
}

// Nfqws2Path возвращает путь к бинарнику nfqws2 в раскладке root
// (через nfq2/nfqws2).
func Nfqws2Path(root string) string { return filepath.Join(root, "nfq2", BinNfqws2) }

// Installed — сведения об установленной сборке.
type Installed struct {
	Version string
	Source  Source
	ArchDir string
}

func writeInstalled(root string, in Installed) error {
	body := fmt.Sprintf("version=%s\nsource=%s\narch=%s\n", in.Version, in.Source, in.ArchDir)
	if err := os.WriteFile(filepath.Join(root, VersionFile), []byte(body), 0o644); err != nil {
		return fmt.Errorf("zapret: %w", err)
	}
	return nil
}

// ReadInstalled читает маркер установленной сборки.
func ReadInstalled(root string) (Installed, error) {
	b, err := os.ReadFile(filepath.Join(root, VersionFile))
	if err != nil {
		return Installed{}, err
	}
	var in Installed
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "version":
			in.Version = v
		case "source":
			in.Source = Source(v)
		case "arch":
			in.ArchDir = v
		}
	}
	if in.Version == "" || in.ArchDir == "" {
		return Installed{}, errors.New("zapret: повреждён файл версии")
	}
	return in, nil
}

// EnvVendorDir — каталог с локальными архивами zapret2.
const EnvVendorDir = "ZATOR_VENDOR"

func resolveLocalArchive(opts Options, asset string) (string, bool) {
	candidates := make([]string, 0, 8)
	if p := strings.TrimSpace(opts.LocalArchive); p != "" {
		candidates = append(candidates, p)
	}
	addDir := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		candidates = append(candidates, filepath.Join(dir, asset))
	}
	addDir(opts.VendorDir)
	addDir(os.Getenv(EnvVendorDir))
	// ZATOR_BUNDLE_DIR/vendor — офлайн-бандл (README: ZATOR_BUNDLE_DIR=… zator install).
	if bd := strings.TrimSpace(os.Getenv("ZATOR_BUNDLE_DIR")); bd != "" {
		addDir(filepath.Join(bd, "vendor"))
	}
	if opts.Root != "" {
		addDir(filepath.Join(filepath.Dir(opts.Root), "vendor"))
		addDir(filepath.Join(opts.Root, "vendor"))
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p, true
		}
	}
	return "", false
}

// resolveLocalArchiveSHA256 ищет сумму рядом с локальным архивом:
// <archive>.sha256 (hex или строка sha256sum) либо sha256sum.txt / SHA256SUMS в том же каталоге.
func resolveLocalArchiveSHA256(archive, asset string) (string, bool) {
	archive = strings.TrimSpace(archive)
	if archive == "" {
		return "", false
	}
	asset = path.Base(asset)
	if asset == "" || asset == "." {
		asset = filepath.Base(archive)
	}
	readSumFile := func(p string) (string, bool) {
		body, err := os.ReadFile(p)
		if err != nil {
			return "", false
		}
		trimmed := strings.TrimSpace(string(body))
		if sha256HexRe.MatchString(trimmed) {
			return strings.ToLower(trimmed), true
		}
		if sum := ParseSHA256SumFile(body).ArchiveSum(asset); sum != "" {
			return sum, true
		}
		// Одна строка «hex  filename» без полного индекса.
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && sha256HexRe.MatchString(fields[0]) {
			return strings.ToLower(fields[0]), true
		}
		return "", false
	}
	if sum, ok := readSumFile(archive + ".sha256"); ok {
		return sum, true
	}
	dir := filepath.Dir(archive)
	for _, name := range []string{"sha256sum.txt", "SHA256SUMS"} {
		if sum, ok := readSumFile(filepath.Join(dir, name)); ok {
			return sum, true
		}
	}
	return "", false
}

func envTruthy(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target, info.Mode().Perm())
	})
}
