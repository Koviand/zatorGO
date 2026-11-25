package zapret

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

type fakeFile struct {
	name string
	body string
	mode int64
}

func buildArchive(t *testing.T, top string, extra ...fakeFile) []byte {
	t.Helper()
	files := []fakeFile{
		{top + "/install_bin.sh", "#!/bin/sh\n", 0o755},
		{top + "/docs/readme.txt", "docs", 0o644},
		{top + "/lua/zapret-lib.lua", "-- lib", 0o644},
	}
	for _, arch := range []string{"linux-x86_64", "linux-arm64", "linux-mipsel"} {
		for _, b := range []string{"nfqws2", "ip2net", "mdig"} {
			files = append(files, fakeFile{top + "/binaries/" + arch + "/" + b, "BIN:" + arch + ":" + b, 0o755})
		}
	}
	files = append(files, extra...)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Typeflag: tar.TypeReg, Mode: f.mode, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f.body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newServer(t *testing.T, version string, archive []byte, embedded bool, hits *int32) *httptest.Server {
	t.Helper()
	return newServerSums(t, version, archive, embedded, hits, false)
}

// newServerSums поднимает mock-релиз. perFileOnly=true — формат апстрима
// (только binaries/…, без суммы tar.gz).
func newServerSums(t *testing.T, version string, archive []byte, embedded bool, hits *int32, perFileOnly bool) *httptest.Server {
	t.Helper()
	asset := AssetName(version, embedded)
	want := "/v" + version + "/" + asset
	sumsPath := "/v" + version + "/sha256sum.txt"
	var sumsBody string
	if perFileOnly {
		// Считаем суммы тел бинарников из buildArchive (BIN:<arch>:<bin>).
		var b strings.Builder
		top := "zapret2-v" + version
		for _, arch := range []string{"linux-x86_64", "linux-arm64", "linux-mipsel"} {
			for _, name := range []string{"nfqws2", "ip2net", "mdig"} {
				body := "BIN:" + arch + ":" + name
				sum := sha256.Sum256([]byte(body))
				fmt.Fprintf(&b, "%s  %s/binaries/%s/%s\n", hex.EncodeToString(sum[:]), top, arch, name)
			}
		}
		sumsBody = b.String()
	} else {
		sum := sha256.Sum256(archive)
		sumsBody = hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		switch r.URL.Path {
		case want:
			w.Write(archive)
		case sumsPath:
			w.Write([]byte(sumsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fastClient() *fetch.Client {
	c := fetch.NewClient()
	c.RetryDelay = 0
	return c
}

func TestResolveArch(t *testing.T) {
	cases := map[platform.Arch]string{
		platform.ArchARM:      "linux-arm",
		platform.ArchARM64:    "linux-arm64",
		platform.ArchMIPS:     "linux-mips",
		platform.ArchMIPSEL:   "linux-mipsel",
		platform.ArchMIPS64:   "linux-mips64",
		platform.ArchMIPSEL64: "linux-mipsel64",
		platform.ArchPPC:      "linux-ppc",
		platform.ArchX86:      "linux-x86",
		platform.ArchX86_64:   "linux-x86_64",
		platform.ArchRISCV64:  "linux-riscv64",
		platform.ArchLexra:    "linux-lexra",
	}
	for a, want := range cases {
		got, err := ResolveArch(a)
		if err != nil || got != want {
			t.Errorf("%s: %q, %v (ожидалось %q)", a, got, err, want)
		}
	}
	// Все поддерживаемые архитектуры платформы должны иметь отображение.
	for _, a := range platform.AllArches() {
		if _, err := ResolveArch(a); err != nil {
			t.Errorf("нет отображения для %s: %v", a, err)
		}
	}
	if _, err := ResolveArch(platform.ArchUnknown); err == nil {
		t.Error("для неизвестной архитектуры ожидалась ошибка")
	}
	if d, _ := ArchDirs(platform.ArchMIPSEL64); len(d) != 2 || d[1] != "linux-mipsel" {
		t.Errorf("запасной вариант mipsel64: %v", d)
	}
}

func TestDefaults(t *testing.T) {
	if DefaultSource != SourceFork {
		t.Fatal("источник по умолчанию должен быть форк")
	}
	if DefaultVer != "1.0.7-reasm-fix" {
		t.Fatalf("DefaultVer = %s", DefaultVer)
	}
	if s, err := ParseSource(""); err != nil || s != SourceFork {
		t.Fatal(s, err)
	}
	if s, _ := ParseSource("Official"); s != SourceOfficial {
		t.Fatal(s)
	}
	if _, err := ParseSource("x"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestNormalizeVersion(t *testing.T) {
	if v, err := NormalizeVersion(SourceOfficial, ""); err != nil || v != DefaultVersion(SourceOfficial) {
		t.Fatal(v, err)
	}
	if v, err := NormalizeVersion(SourceOfficial, "v1.0.5.1"); err != nil || v != "1.0.5.1" {
		t.Fatal(v, err)
	}
	if _, err := NormalizeVersion(SourceOfficial, "1.0.5.1-reasm-fix"); err == nil {
		t.Fatal("суффикс допустим только у форка")
	}
	if v, err := NormalizeVersion(SourceFork, "1.0.5.1-reasm-fix"); err != nil || v != "1.0.5.1-reasm-fix" {
		t.Fatal(v, err)
	}
	for _, bad := range []string{"../x", "1.0;rm", "abc", strings.Repeat("1", 41)} {
		if _, err := NormalizeVersion(SourceFork, bad); err == nil {
			t.Errorf("%q должна быть отвергнута", bad)
		}
	}
}

func TestReleaseURL(t *testing.T) {
	t.Setenv(EnvReleaseBase, "")
	t.Setenv(EnvForkReleaseBase, "")
	if got := ReleaseURL(SourceFork, "", "0.8.2", false); got !=
		"https://github.com/MarkinAlexander/zapret2/releases/download/v0.8.2/zapret2-v0.8.2.tar.gz" {
		t.Fatal(got)
	}
	if got := ReleaseURL(SourceOfficial, "", "0.8.2", true); got !=
		"https://github.com/bol-van/zapret2/releases/download/v0.8.2/zapret2-v0.8.2-openwrt-embedded.tar.gz" {
		t.Fatal(got)
	}
	if got := ReleaseURL(SourceOfficial, "https://m.example/r/", "1.0", false); got !=
		"https://m.example/r/v1.0/zapret2-v1.0.tar.gz" {
		t.Fatal(got)
	}
	t.Setenv(EnvForkReleaseBase, "https://env.example")
	if got := ReleaseURL(SourceFork, "", "1.0", false); got != "https://env.example/v1.0/zapret2-v1.0.tar.gz" {
		t.Fatal(got)
	}
}

func TestDownloadAndInstall(t *testing.T) {
	ver := "0.8.2"
	srv := newServer(t, ver, buildArchive(t, "zapret2-v"+ver), false, nil)
	root := filepath.Join(t.TempDir(), "opt", "zapret2")

	var logs []string
	res, err := DownloadAndInstall(context.Background(), Options{
		Root:        root,
		Arch:        platform.ArchX86_64,
		Version:     ver,
		ReleaseBase: srv.URL,
		Client:      fastClient(),
		Logf:        func(f string, a ...any) { logs = append(logs, f) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceFork || res.Version != ver || res.ArchDir != "linux-x86_64" || res.Skipped {
		t.Fatalf("результат: %+v", res)
	}
	if len(res.Missing) != 0 || len(logs) == 0 {
		t.Fatalf("missing=%v logs=%v", res.Missing, logs)
	}

	for _, b := range []string{"nfqws2", "ip2net", "mdig"} {
		got, err := os.ReadFile(filepath.Join(root, "binaries", "linux-x86_64", b))
		if err != nil || string(got) != "BIN:linux-x86_64:"+b {
			t.Fatalf("%s: %q %v", b, got, err)
		}
	}
	// Ссылки/копии в стандартной раскладке читаются как бинарники.
	for bin, dir := range map[string]string{"nfqws2": "nfq2", "ip2net": "ip2net", "mdig": "mdig"} {
		got, err := os.ReadFile(filepath.Join(root, dir, bin))
		if err != nil || string(got) != "BIN:linux-x86_64:"+bin {
			t.Fatalf("%s/%s: %q %v", dir, bin, got, err)
		}
	}
	if got, err := os.ReadFile(Nfqws2Path(root)); err != nil || !strings.HasPrefix(string(got), "BIN:") {
		t.Fatal(got, err)
	}
	// Только нужная архитектура и никаких лишних файлов архива.
	for _, p := range []string{
		filepath.Join(root, "binaries", "linux-arm64"),
		filepath.Join(root, "binaries", "linux-mipsel"),
		filepath.Join(root, "install_bin.sh"),
		filepath.Join(root, "docs"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("не должно существовать: %s", p)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "lua")); len(entries) != 0 {
		t.Error("lua не запрашивалась")
	}
	// Минимальная раскладка.
	for _, d := range LayoutDirs(root) {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("нет каталога %s", d)
		}
	}
	if runtimeExecBit() {
		st, _ := os.Stat(filepath.Join(root, "binaries", "linux-x86_64", "nfqws2"))
		if st.Mode()&0o111 == 0 {
			t.Error("nfqws2 не исполняемый")
		}
	}
	in, err := ReadInstalled(root)
	if err != nil || in.Version != ver || in.Source != SourceFork || in.ArchDir != "linux-x86_64" {
		t.Fatalf("ReadInstalled: %+v %v", in, err)
	}
	// Временные каталоги убраны.
	entries, _ := os.ReadDir(filepath.Dir(root))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".zapret2-install-") {
			t.Errorf("остался временный каталог %s", e.Name())
		}
	}
}

func runtimeExecBit() bool { return os.PathSeparator == '/' }

func TestInstallWithUpstreamPerFileSums(t *testing.T) {
	// Реальный sha256sum.txt апстрима не содержит суммы tar.gz — только binaries/*.
	ver := "0.8.2"
	srv := newServerSums(t, ver, buildArchive(t, "zapret2-v"+ver), false, nil, true)
	root := filepath.Join(t.TempDir(), "opt", "zapret2")
	res, err := DownloadAndInstall(context.Background(), Options{
		Root: root, Arch: platform.ArchX86_64, Version: ver,
		ReleaseBase: srv.URL, Client: fastClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchDir != "linux-x86_64" || res.Skipped {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "binaries", "linux-x86_64", BinNfqws2)); err != nil {
		t.Fatal(err)
	}
}

func TestInstallFromLocalArchive(t *testing.T) {
	ver := "0.8.2"
	dir := t.TempDir()
	asset := AssetName(ver, false)
	archPath := filepath.Join(dir, "vendor", asset)
	if err := os.MkdirAll(filepath.Dir(archPath), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "opt", "zapret2")
	raw := buildArchive(t, "zapret2-v"+ver)
	sum := sha256.Sum256(raw)
	if err := os.WriteFile(archPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := DownloadAndInstall(context.Background(), Options{
		Root: root, Arch: platform.ArchX86_64, Version: ver,
		VendorDir: filepath.Join(dir, "vendor"),
		Offline:   true,
		SHA256:    hex.EncodeToString(sum[:]),
		Client:    fastClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URL, "file://") || res.Skipped {
		t.Fatalf("local: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "binaries", "linux-x86_64", BinNfqws2)); err != nil {
		t.Fatal(err)
	}
	// Offline без локального архива — ошибка, без сети.
	if _, err := DownloadAndInstall(context.Background(), Options{
		Root: filepath.Join(t.TempDir(), "z"), Arch: platform.ArchX86_64, Version: ver,
		Offline: true, AllowInsecure: true, Client: fastClient(),
	}); err == nil || !strings.Contains(err.Error(), "офлайн") {
		t.Fatalf("offline without archive: %v", err)
	}
}

func TestInstallFromLocalArchiveSidecarSHA256(t *testing.T) {
	ver := "0.8.2"
	dir := t.TempDir()
	asset := AssetName(ver, false)
	vendor := filepath.Join(dir, "vendor")
	archPath := filepath.Join(vendor, asset)
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := buildArchive(t, "zapret2-v"+ver)
	sum := sha256.Sum256(raw)
	hexSum := hex.EncodeToString(sum[:])
	if err := os.WriteFile(archPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archPath+".sha256", []byte(hexSum+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "opt", "zapret2")
	res, err := DownloadAndInstall(context.Background(), Options{
		Root: root, Arch: platform.ArchX86_64, Version: ver,
		VendorDir: vendor, Offline: true, Client: fastClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URL, "file://") {
		t.Fatalf("local sidecar: %+v", res)
	}
	// Offline + local без sidecar и без Options.SHA256 — ошибка.
	bare := t.TempDir()
	bareVendor := filepath.Join(bare, "vendor")
	if err := os.MkdirAll(bareVendor, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareVendor, asset), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadAndInstall(context.Background(), Options{
		Root: filepath.Join(bare, "z"), Arch: platform.ArchX86_64, Version: ver,
		VendorDir: bareVendor, Offline: true, Client: fastClient(),
	}); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("offline without sidecar: %v", err)
	}
}

func TestResolveLocalArchiveBundleDir(t *testing.T) {
	ver := "0.8.2"
	asset := AssetName(ver, false)
	bundle := t.TempDir()
	vendor := filepath.Join(bundle, "vendor")
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := buildArchive(t, "zapret2-v"+ver)
	if err := os.WriteFile(filepath.Join(vendor, asset), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZATOR_BUNDLE_DIR", bundle)
	t.Setenv(EnvVendorDir, "")
	got, ok := resolveLocalArchive(Options{Root: filepath.Join(t.TempDir(), "zapret2")}, asset)
	if !ok || !strings.Contains(got, filepath.Join("vendor", asset)) {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestResolveLocalArchiveSHA256(t *testing.T) {
	dir := t.TempDir()
	asset := "zapret2-v1.tar.gz"
	arch := filepath.Join(dir, asset)
	want := strings.Repeat("ab", 32)
	if err := os.WriteFile(arch, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(arch+".sha256", []byte(want+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveLocalArchiveSHA256(arch, asset)
	if !ok || got != want {
		t.Fatalf("sidecar hex: got=%q ok=%v", got, ok)
	}
	dir2 := t.TempDir()
	arch2 := filepath.Join(dir2, asset)
	if err := os.WriteFile(arch2, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	sumLine := want + "  " + asset + "\n"
	if err := os.WriteFile(filepath.Join(dir2, "sha256sum.txt"), []byte(sumLine), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok = resolveLocalArchiveSHA256(arch2, asset)
	if !ok || got != want {
		t.Fatalf("sha256sum.txt: got=%q ok=%v", got, ok)
	}
}

func TestInstallSkipAndForce(t *testing.T) {
	ver := "0.8.2"
	var hits int32
	srv := newServer(t, ver, buildArchive(t, "zapret2-v"+ver), false, &hits)
	opts := Options{Root: filepath.Join(t.TempDir(), "z"), Arch: platform.ArchARM64, Version: ver,
		ReleaseBase: srv.URL, Client: fastClient()}

	if _, err := DownloadAndInstall(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	h1 := atomic.LoadInt32(&hits)
	res, err := DownloadAndInstall(context.Background(), opts)
	if err != nil || !res.Skipped {
		t.Fatalf("ожидался пропуск: %+v %v", res, err)
	}
	if atomic.LoadInt32(&hits) != h1 {
		t.Fatal("повторная установка не должна качать архив")
	}
	opts.Force = true
	res, err = DownloadAndInstall(context.Background(), opts)
	if err != nil || res.Skipped || atomic.LoadInt32(&hits) == h1 {
		t.Fatalf("Force: %+v %v", res, err)
	}
}

func TestInstallPreservesUserFilesAndReplacesBinaries(t *testing.T) {
	ver := "0.8.2"
	srv := newServer(t, ver, buildArchive(t, "zapret2-v"+ver), false, nil)
	root := filepath.Join(t.TempDir(), "z")
	os.MkdirAll(filepath.Join(root, "binaries", "linux-mipsel"), 0o755)
	os.WriteFile(filepath.Join(root, "binaries", "linux-mipsel", "nfqws2"), []byte("old"), 0o755)
	os.WriteFile(filepath.Join(root, "config"), []byte("USER=1"), 0o644)

	_, err := DownloadAndInstall(context.Background(), Options{Root: root, Arch: platform.ArchX86_64, Version: ver,
		ReleaseBase: srv.URL, Client: fastClient()})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "config")); string(b) != "USER=1" {
		t.Fatal("пользовательский config затронут")
	}
	if _, err := os.Stat(filepath.Join(root, "binaries", "linux-mipsel")); !os.IsNotExist(err) {
		t.Fatal("старые бинарники другой архитектуры должны быть удалены")
	}
}

func TestInstallIncludeLua(t *testing.T) {
	ver := "0.8.2"
	srv := newServer(t, ver, buildArchive(t, "zapret2-v"+ver), false, nil)
	root := filepath.Join(t.TempDir(), "z")
	_, err := DownloadAndInstall(context.Background(), Options{Root: root, Arch: platform.ArchX86_64, Version: ver,
		ReleaseBase: srv.URL, Client: fastClient(), IncludeLua: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "lua", "zapret-lib.lua")); err != nil || string(b) != "-- lib" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestInstallMirrorFallback(t *testing.T) {
	ver := "0.8.2"
	archive := buildArchive(t, "zapret2-v"+ver)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer dead.Close()
	mirror := newServer(t, ver, archive, false, nil)

	_, err := DownloadAndInstall(context.Background(), Options{
		Root:        filepath.Join(t.TempDir(), "z"),
		Arch:        platform.ArchX86_64,
		Source:      SourceOfficial,
		Version:     ver,
		ReleaseBase: dead.URL,
		MirrorBase:  mirror.URL,
		Client:      fastClient(),
	})
	// ReleaseBase не github.com, поэтому ReleaseMirror не применим: ошибка загрузки.
	if err == nil {
		t.Fatal("для нестандартной базы зеркало не должно переписывать URL")
	}

	// Клиент с HostMirror для тестового хоста подменяет адрес.
	c := fastClient()
	c.Mirrors = []fetch.Mirror(nil)
	c.Mirrors = append(c.Mirrors, fetch.HostMirror(strings.TrimPrefix(dead.URL, "http://"), mirror.URL))
	_, err = DownloadAndInstall(context.Background(), Options{
		Root:        filepath.Join(t.TempDir(), "z2"),
		Arch:        platform.ArchX86_64,
		Version:     ver,
		ReleaseBase: dead.URL,
		Client:      c,
	})
	if err != nil {
		t.Fatalf("зеркало должно сработать: %v", err)
	}
}

func TestInstallErrors(t *testing.T) {
	ver := "0.8.2"
	good := buildArchive(t, "zapret2-v"+ver)
	ctx := context.Background()
	base := func(srv *httptest.Server, arch platform.Arch) Options {
		return Options{Root: filepath.Join(t.TempDir(), "z"), Arch: arch, Version: ver,
			ReleaseBase: srv.URL, Client: fastClient()}
	}

	t.Run("sha mismatch", func(t *testing.T) {
		srv := newServer(t, ver, good, false, nil)
		o := base(srv, platform.ArchX86_64)
		o.SHA256 = strings.Repeat("0", 64)
		if _, err := DownloadAndInstall(ctx, o); err == nil {
			t.Fatal("ожидалась ошибка")
		}
		if _, err := os.Stat(filepath.Join(o.Root, VersionFile)); !os.IsNotExist(err) {
			t.Fatal("маркер версии не должен появиться")
		}
	})
	t.Run("sha ok", func(t *testing.T) {
		srv := newServer(t, ver, good, false, nil)
		o := base(srv, platform.ArchX86_64)
		h := sha256.Sum256(good)
		o.SHA256 = hex.EncodeToString(h[:])
		if _, err := DownloadAndInstall(ctx, o); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("arch absent from archive", func(t *testing.T) {
		srv := newServer(t, ver, good, false, nil)
		if _, err := DownloadAndInstall(ctx, base(srv, platform.ArchRISCV64)); err == nil || !strings.Contains(err.Error(), "linux-riscv64") {
			t.Fatalf("получено %v", err)
		}
	})
	t.Run("unknown arch", func(t *testing.T) {
		srv := newServer(t, ver, good, false, nil)
		if _, err := DownloadAndInstall(ctx, base(srv, platform.ArchUnknown)); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("traversal in archive", func(t *testing.T) {
		evil := buildArchive(t, "zapret2-v"+ver, fakeFile{"../evil", "x", 0o644})
		srv := newServer(t, ver, evil, false, nil)
		o := base(srv, platform.ArchX86_64)
		if _, err := DownloadAndInstall(ctx, o); err == nil || !strings.Contains(err.Error(), "небезопасный") {
			t.Fatalf("получено %v", err)
		}
		if _, err := os.Stat(filepath.Join(o.Root, "binaries")); !os.IsNotExist(err) {
			t.Fatal("ничего не должно быть установлено")
		}
	})
	t.Run("not found", func(t *testing.T) {
		srv := newServer(t, "9.9.9", good, false, nil)
		if _, err := DownloadAndInstall(ctx, base(srv, platform.ArchX86_64)); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("missing root", func(t *testing.T) {
		if _, err := DownloadAndInstall(ctx, Options{Arch: platform.ArchX86_64}); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("bad version", func(t *testing.T) {
		if _, err := DownloadAndInstall(ctx, Options{Root: t.TempDir(), Arch: platform.ArchX86_64, Version: "../x"}); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
	t.Run("no nfqws2", func(t *testing.T) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "zapret2-v" + ver + "/binaries/linux-x86_64/mdig", Typeflag: tar.TypeReg, Mode: 0o755, Size: 1})
		tw.Write([]byte("x"))
		tw.Close()
		gz.Close()
		srv := newServer(t, ver, buf.Bytes(), false, nil)
		if _, err := DownloadAndInstall(ctx, base(srv, platform.ArchX86_64)); err == nil || !strings.Contains(err.Error(), "nfqws2") {
			t.Fatalf("получено %v", err)
		}
	})
}

func TestInstallWrapperOpenWRTEmbedded(t *testing.T) {
	ver := "0.8.2"
	// Сервер отдаёт только embedded-вариант.
	srv := newServer(t, ver, buildArchive(t, "zapret2-v"+ver), true, nil)
	zr := filepath.Join(t.TempDir(), "zapret2")
	p := paths.New(filepath.Join(t.TempDir(), "zator"), zr)
	plat := platform.Platform{Kind: platform.KindOpenWRT, Arch: platform.ArchARM64, InitSystem: platform.InitProcd}

	res, err := Install(context.Background(), p, plat, Options{Version: ver,
		ReleaseBase: srv.URL, Client: fastClient()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Root != zr || res.ArchDir != "linux-arm64" || !strings.HasSuffix(res.URL, "-openwrt-embedded.tar.gz") {
		t.Fatalf("%+v", res)
	}
	if res.Binaries[BinNfqws2] == "" {
		t.Fatal("нет пути nfqws2")
	}

	// На VPS — стандартный вариант, который сервер не отдаёт.
	vps := platform.Platform{Kind: platform.KindVPS, Arch: platform.ArchARM64}
	if _, err := Install(context.Background(), paths.New(t.TempDir(), filepath.Join(t.TempDir(), "z")), vps, Options{Version: ver,
		ReleaseBase: srv.URL, Client: fastClient()}); err == nil {
		t.Fatal("ожидалась ошибка: стандартного архива нет")
	}
	// Явный вариант перекрывает автоопределение.
	if _, err := Install(context.Background(), paths.New(t.TempDir(), filepath.Join(t.TempDir(), "z")), vps,
		Options{Version: ver,
			ReleaseBase: srv.URL, Client: fastClient(), Variant: VariantOpenWRTEmbedded}); err != nil {
		t.Fatal(err)
	}
}
