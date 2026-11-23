package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/paths"
)

const liveConfig = `FWTYPE=iptables
FLOWOFFLOAD=none
MODE_FILTER=none
NFQWS2_PORTS_TCP=80,443
NFQWS2_PORTS_UDP=443
CLIENT_SCOPE_ENABLE=0
#Z2R_AUTO_MODE=0
NFQWS2_OPT="
--filter-tcp=443 --lua-desync=fake:strategy=1
"
`

const oldConfig = `FWTYPE=nftables
FLOWOFFLOAD=software
MODE_FILTER=autohostlist
NFQWS2_PORTS_TCP=80,443,8443
NFQWS2_PORTS_UDP=443,50000-50099
CLIENT_SCOPE_ENABLE=1
#Z2R_AUTO_MODE=1
NFQWS2_OPT="
--filter-tcp=443 --lua-desync=fake:strategy=99
"
`

type env struct {
	p   paths.Paths
	m   *Manager
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	e := &env{p: p, now: time.Date(2026, 5, 4, 10, 30, 0, 0, time.UTC)}
	e.m = NewIn(p, filepath.Join(root, "backups"))
	e.m.Now = func() time.Time { return e.now }
	return e
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// seed заполняет состояние установки.
func (e *env) seed(t *testing.T) {
	t.Helper()
	p := e.p
	write(t, p.ZapretConfig, oldConfig)
	write(t, filepath.Join(p.ListsDir, "netrogat.txt"), "a.com\nb.com\n")
	write(t, filepath.Join(p.ListsDir, "netrogat_substrings.txt"), "google\n")
	write(t, filepath.Join(p.ExtraStratsDir, "TCP_Custom.txt"), "my.example.org\n")
	write(t, filepath.Join(p.ListsDir, "russia-youtube.txt"), "youtube.com\n") // не входит без IncludeLists
	write(t, p.LockedTSV, "1\ttls\t7\nmy.example.org\ttls\t3\n")
	write(t, p.LockedManualTSV, "2\ttls\t4\n")
	write(t, filepath.Join(p.StateOrchestraDir(), "mode_override.tsv"), "1\tclone\n")
	write(t, filepath.Join(p.StateOrchestraDir(), "blob_override.tsv"), "2\tmaxru\n")
	write(t, filepath.Join(p.StateOrchestraDir(), "scopes", "mark_2.tsv"), "1\ttls\t5\n")
	write(t, filepath.Join(p.StateOrchestraDir(), "history.tsv"), "history must not be backed up\n")
}

func names(t *testing.T, m *Manager, name string) []string {
	t.Helper()
	got, err := m.Contents(name)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range got {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func TestDefaultDir(t *testing.T) {
	def := paths.New("", "")
	if got := DefaultDir(def); got != filepath.FromSlash("/opt/zator_backup") {
		t.Errorf("default root dir = %s", got)
	}
	custom := paths.New(filepath.Join(t.TempDir(), "z"), "")
	if got := DefaultDir(custom); got != filepath.Join(custom.StateDir, "backups") {
		t.Errorf("custom root dir = %s", got)
	}
	if m := New(custom); m.Dir != DefaultDir(custom) {
		t.Errorf("New dir = %s", m.Dir)
	}
}

func TestValidName(t *testing.T) {
	good := []string{"z2r_backup_20260504_103000.tar", "z2r_backup_20260504_103000_2.tar"}
	bad := []string{"", "../z2r_backup_20260504_103000.tar", "z2r_backup_20260504_103000.tar.gz",
		"backup.tar", "z2r_backup_2026_103000.tar", "sub/z2r_backup_20260504_103000.tar", `a\z2r_backup_20260504_103000.tar`}
	for _, n := range good {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false", n)
		}
	}
	for _, n := range bad {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true", n)
		}
	}
}

func TestCreateContentsAndName(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, err := e.m.Create(CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "z2r_backup_20260504_103000.tar" || info.Size <= 0 || filepath.Dir(info.Path) != e.m.Dir {
		t.Errorf("info = %+v", info)
	}
	want := []string{
		"config",
		"lists/TCP_Custom.txt", "lists/netrogat.txt", "lists/netrogat_substrings.txt",
		"orchestra/blob_override.tsv", "orchestra/locked.manual.tsv", "orchestra/locked.tsv",
		"orchestra/mode_override.tsv", "orchestra/scopes/mark_2.tsv",
	}
	if got := names(t, e.m, info.Name); !reflect.DeepEqual(got, want) {
		t.Errorf("contents =\n%v\nwant\n%v", got, want)
	}
}

func TestCreateOptions(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, err := e.m.Create(CreateOptions{IncludeLists: true, SkipConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, e.m, info.Name)
	has := func(n string) bool {
		for _, g := range got {
			if g == n {
				return true
			}
		}
		return false
	}
	if has("config") {
		t.Error("SkipConfig must exclude config")
	}
	if !has("lists/russia-youtube.txt") {
		t.Errorf("IncludeLists must add all lists: %v", got)
	}
	if has("orchestra/history.tsv") {
		t.Error("history must not be backed up")
	}
}

func TestCreateEmptyAndCollisions(t *testing.T) {
	e := newEnv(t)
	if _, err := e.m.Create(CreateOptions{}); err == nil {
		t.Fatal("empty installation must fail")
	}
	e.seed(t)
	a, err := e.m.Create(CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.m.Create(CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name == b.Name || b.Name != "z2r_backup_20260504_103000_1.tar" {
		t.Errorf("names = %s, %s", a.Name, b.Name)
	}
	// Временных файлов не остаётся.
	ents, _ := os.ReadDir(e.m.Dir)
	for _, en := range ents {
		if strings.HasSuffix(en.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", en.Name())
		}
	}
}

func TestListSortedAndFiltered(t *testing.T) {
	e := newEnv(t)
	if l, err := e.m.List(); err != nil || len(l) != 0 {
		t.Fatalf("List on missing dir = %v, %v", l, err)
	}
	e.seed(t)
	e.now = time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	first, _ := e.m.Create(CreateOptions{})
	e.now = e.now.Add(time.Hour)
	second, _ := e.m.Create(CreateOptions{})
	write(t, filepath.Join(e.m.Dir, "notes.txt"), "x")
	write(t, filepath.Join(e.m.Dir, "z2r_backup_bad.tar"), "x")
	l, err := e.m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 || l[0].Name != second.Name || l[1].Name != first.Name {
		t.Errorf("List = %+v", l)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	if err := e.m.Delete(info.Name); err != nil {
		t.Fatal(err)
	}
	if exists(info.Path) {
		t.Error("archive still exists")
	}
	if err := e.m.Delete(info.Name); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete err = %v", err)
	}
	for _, bad := range []string{"../../etc/passwd", "notes.txt", "", "sub/z2r_backup_20260504_103000.tar"} {
		if err := e.m.Delete(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Delete(%q) err = %v, want ErrInvalidName", bad, err)
		}
	}
	// Файл вне каталога бэкапов не удаляется даже при совпадении шаблона.
	outside := filepath.Join(filepath.Dir(e.m.Dir), "z2r_backup_20260504_103000.tar")
	write(t, outside, "keep")
	if err := e.m.Delete("..\\z2r_backup_20260504_103000.tar"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("err = %v", err)
	}
	if !exists(outside) {
		t.Error("file outside backup dir must survive")
	}
}

func TestRestoreFullRoundTrip(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, err := e.m.Create(CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Портим состояние.
	write(t, e.p.ZapretConfig, liveConfig)
	write(t, filepath.Join(e.p.ListsDir, "netrogat.txt"), "changed.com\n")
	os.Remove(filepath.Join(e.p.ExtraStratsDir, "TCP_Custom.txt"))
	write(t, e.p.LockedTSV, "")
	os.Remove(filepath.Join(e.p.StateOrchestraDir(), "mode_override.tsv"))
	os.RemoveAll(filepath.Join(e.p.StateOrchestraDir(), "scopes"))

	res, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeFull})
	if err != nil {
		t.Fatal(err)
	}
	if !res.ConfigReplaced || len(res.Files) != 8 {
		t.Errorf("result = %+v", res)
	}
	if read(t, e.p.ZapretConfig) != oldConfig {
		t.Error("config not restored")
	}
	if read(t, filepath.Join(e.p.ListsDir, "netrogat.txt")) != "a.com\nb.com\n" {
		t.Error("netrogat not restored")
	}
	if read(t, filepath.Join(e.p.ExtraStratsDir, "TCP_Custom.txt")) != "my.example.org\n" {
		t.Error("TCP_Custom not restored")
	}
	if read(t, e.p.LockedTSV) != "1\ttls\t7\nmy.example.org\ttls\t3\n" {
		t.Error("locked.tsv not restored")
	}
	// Восстановленное читается стором orchestra.
	st := orchestra.NewStore(e.p.StateOrchestraDir())
	if v, _ := st.GetLock(orchestra.ScopeDefault, "1", "tls"); v != 7 {
		t.Errorf("lock 1/tls = %v", v)
	}
	if v, _, _ := st.GetOverride(orchestra.OverrideMode, "1"); v != "clone" {
		t.Errorf("mode override = %q", v)
	}
	if v, _ := st.GetLock("mark:2", "1", "tls"); v != 5 {
		t.Errorf("scope lock = %v", v)
	}
	// Не входящее в бэкап осталось нетронутым.
	if read(t, filepath.Join(e.p.ListsDir, "russia-youtube.txt")) != "youtube.com\n" {
		t.Error("unrelated list changed")
	}
}

func TestRestoreFullBlockedAndErrors(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	write(t, e.p.ZapretConfig, liveConfig)
	if _, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeFull, BlockFull: true}); !errors.Is(err, ErrFullBlocked) {
		t.Errorf("err = %v, want ErrFullBlocked", err)
	}
	if read(t, e.p.ZapretConfig) != liveConfig {
		t.Error("blocked restore must not touch config")
	}
	if _, err := e.m.Restore(info.Name, RestoreOptions{}); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("zero mode err = %v", err)
	}
	if _, err := e.m.Restore("z2r_backup_20990101_000000.tar", RestoreOptions{Mode: ModeLists}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing archive err = %v", err)
	}
	if _, err := e.m.Restore("../x.tar", RestoreOptions{Mode: ModeLists}); !errors.Is(err, ErrInvalidName) {
		t.Errorf("invalid name err = %v", err)
	}
}

func TestRestoreListsKeepsConfig(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	write(t, e.p.ZapretConfig, liveConfig)
	write(t, filepath.Join(e.p.ListsDir, "netrogat.txt"), "other\n")
	res, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeLists})
	if err != nil {
		t.Fatal(err)
	}
	if res.ConfigReplaced || len(res.SmartApplied) != 0 {
		t.Errorf("result = %+v", res)
	}
	if read(t, e.p.ZapretConfig) != liveConfig {
		t.Error("config must stay untouched")
	}
	if read(t, filepath.Join(e.p.ListsDir, "netrogat.txt")) != "a.com\nb.com\n" {
		t.Error("lists not restored")
	}
}

func TestRestoreFullWithoutConfigInArchive(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{SkipConfig: true})
	write(t, e.p.ZapretConfig, liveConfig)
	if _, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeFull}); !errors.Is(err, ErrNoConfig) {
		t.Errorf("err = %v, want ErrNoConfig", err)
	}
	if read(t, e.p.ZapretConfig) != liveConfig {
		t.Error("config must stay untouched")
	}
	// Режим списков работает и без config в архиве.
	if _, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeLists}); err != nil {
		t.Errorf("lists mode err = %v", err)
	}
}

func TestRestoreSmart(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	write(t, e.p.ZapretConfig, liveConfig)
	write(t, e.p.LockedTSV, "")

	res, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeSmart, BlockFull: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ConfigReplaced {
		t.Error("smart must not replace config")
	}
	wantApplied := []string{"NFQWS2_PORTS_TCP", "NFQWS2_PORTS_UDP", "FWTYPE", "FLOWOFFLOAD", "MODE_FILTER", "CLIENT_SCOPE_ENABLE", "AUTO_MODE"}
	got := append([]string(nil), res.SmartApplied...)
	sort.Strings(got)
	sort.Strings(wantApplied)
	if !reflect.DeepEqual(got, wantApplied) {
		t.Errorf("applied = %v, want %v", got, wantApplied)
	}

	cfg, err := config.Load(e.p.ZapretConfig)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"FWTYPE": "nftables", "FLOWOFFLOAD": "software", "MODE_FILTER": "autohostlist",
		"NFQWS2_PORTS_TCP": "80,443,8443", "NFQWS2_PORTS_UDP": "443,50000-50099", "CLIENT_SCOPE_ENABLE": "1",
	} {
		if got, _ := cfg.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if on, ok := cfg.AutoModeFlag(); !ok || !on {
		t.Errorf("auto mode flag = %v, %v", on, ok)
	}
	// Стратегии в NFQWS2_OPT живого config остались прежними.
	if opt, _ := cfg.NFQWS2Opt(); !strings.Contains(opt, "strategy=1") || strings.Contains(opt, "strategy=99") {
		t.Errorf("NFQWS2_OPT changed: %q", opt)
	}
	// Состояние orchestra восстановлено.
	if read(t, e.p.LockedTSV) != "1\ttls\t7\nmy.example.org\ttls\t3\n" {
		t.Error("locks not restored in smart mode")
	}
}

func TestRestoreSmartIdempotentAndNoLiveConfig(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	// Живой config совпадает с архивным — ничего не применяется и файл не переписывается.
	res, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeSmart})
	if err != nil || len(res.SmartApplied) != 0 {
		t.Fatalf("identical config: %+v, %v", res, err)
	}
	os.Remove(e.p.ZapretConfig)
	if _, err := e.m.Restore(info.Name, RestoreOptions{Mode: ModeSmart}); !errors.Is(err, ErrNoLiveConfig) {
		t.Errorf("err = %v, want ErrNoLiveConfig", err)
	}
	// Нет config в архиве.
	write(t, e.p.ZapretConfig, liveConfig)
	noCfg, _ := e.m.Create(CreateOptions{SkipConfig: true})
	if _, err := e.m.Restore(noCfg.Name, RestoreOptions{Mode: ModeSmart}); !errors.Is(err, ErrNoConfig) {
		t.Errorf("err = %v, want ErrNoConfig", err)
	}
}

// buildTar собирает tar из записей name→content (typeflag по умолчанию — файл).
type tarEntry struct {
	name, body, link string
	typ              byte
}

func buildTar(t *testing.T, entries []tarEntry, gz bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w interface {
		Write([]byte) (int, error)
	} = &buf
	var gw *gzip.Writer
	if gz {
		gw = gzip.NewWriter(&buf)
		w = gw
	}
	tw := tar.NewWriter(w)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Linkname: e.link}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	if gw != nil {
		gw.Close()
	}
	return buf.Bytes()
}

func TestRestoreRejectsUnsafeArchives(t *testing.T) {
	cases := map[string][]tarEntry{
		"dotdot":    {{name: "../evil.txt", body: "x"}},
		"nested":    {{name: "lists/../../evil.txt", body: "x"}},
		"absolute":  {{name: "/etc/evil", body: "x"}},
		"symlink":   {{name: "lists/link", typ: tar.TypeSymlink, link: "/etc/passwd"}},
		"backslash": {{name: `lists\..\evil`, body: "x"}},
	}
	for name, entries := range cases {
		for _, gz := range []bool{false, true} {
			e := newEnv(t)
			write(t, e.p.ZapretConfig, liveConfig)
			arch := filepath.Join(t.TempDir(), "evil.tar")
			if err := os.WriteFile(arch, buildTar(t, entries, gz), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := e.m.RestoreFile(arch, RestoreOptions{Mode: ModeFull})
			if err == nil {
				t.Errorf("%s (gz=%v): unsafe archive accepted", name, gz)
			}
			if read(t, e.p.ZapretConfig) != liveConfig {
				t.Errorf("%s (gz=%v): config modified", name, gz)
			}
			if exists(filepath.Join(filepath.Dir(e.p.ZatorRoot), "evil.txt")) {
				t.Errorf("%s: file escaped extraction dir", name)
			}
		}
	}
	// Ошибка распаковки сопровождается ErrUnsafePath для plain tar.
	e := newEnv(t)
	arch := filepath.Join(t.TempDir(), "evil.tar")
	os.WriteFile(arch, buildTar(t, cases["dotdot"], false), 0o644)
	if _, err := e.m.RestoreFile(arch, RestoreOptions{Mode: ModeLists}); !errors.Is(err, fetch.ErrUnsafePath) {
		t.Errorf("err = %v, want ErrUnsafePath", err)
	}
}

func TestRestoreIgnoresUnknownEntries(t *testing.T) {
	e := newEnv(t)
	arch := filepath.Join(t.TempDir(), "mixed.tar")
	os.WriteFile(arch, buildTar(t, []tarEntry{
		{name: "lists/netrogat.txt", body: "ok.com\n"},
		{name: "bin/zator", body: "malware"},
		{name: "random.txt", body: "x"},
		{name: "lists/", typ: tar.TypeDir},
	}, false), 0o644)
	res, err := e.m.RestoreFile(arch, RestoreOptions{Mode: ModeLists})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 || read(t, filepath.Join(e.p.ListsDir, "netrogat.txt")) != "ok.com\n" {
		t.Errorf("result = %+v", res)
	}
	if exists(filepath.Join(e.p.BinDir, "zator")) || exists(filepath.Join(e.p.ZatorRoot, "random.txt")) {
		t.Error("unknown entries must be ignored")
	}
}

func TestRestoreGzipArchive(t *testing.T) {
	e := newEnv(t)
	arch := filepath.Join(t.TempDir(), "b.tar.gz")
	os.WriteFile(arch, buildTar(t, []tarEntry{
		{name: "config", body: oldConfig},
		{name: "lists/netrogat.txt", body: "gz.com\n"},
		{name: "orchestra/locked.tsv", body: "1\ttls\t9\n"},
	}, true), 0o644)
	res, err := e.m.RestoreFile(arch, RestoreOptions{Mode: ModeFull})
	if err != nil {
		t.Fatal(err)
	}
	if !res.ConfigReplaced || read(t, e.p.ZapretConfig) != oldConfig ||
		read(t, e.p.LockedTSV) != "1\ttls\t9\n" || read(t, filepath.Join(e.p.ListsDir, "netrogat.txt")) != "gz.com\n" {
		t.Errorf("gzip restore failed: %+v", res)
	}
}

func TestRestoreLegacyShellLayout(t *testing.T) {
	e := newEnv(t)
	arch := filepath.Join(t.TempDir(), "legacy.tar")
	os.WriteFile(arch, buildTar(t, []tarEntry{
		{name: "./config", body: oldConfig},
		{name: "./lists/netrogat.txt", body: "legacy.com\n"},
		{name: "./extra_strats/TCP_Custom.txt", body: "c.example.org\n"},
		{name: "./extra_strats/TCP_RKN_domains_by_substring.txt", body: "rkn\n"},
		{name: "./extra_strats/cache/orchestra/locked.tsv", body: "1\ttls\t2\n"},
		{name: "./extra_strats/cache/orchestra/mode_override.tsv", body: "1\tclone\n"},
	}, false), 0o644)
	if _, err := e.m.RestoreFile(arch, RestoreOptions{Mode: ModeFull}); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		e.p.ZapretConfig: oldConfig,
		filepath.Join(e.p.ListsDir, "netrogat.txt"):                           "legacy.com\n",
		filepath.Join(e.p.ExtraStratsDir, "TCP_Custom.txt"):                   "c.example.org\n",
		filepath.Join(e.p.ExtraStratsDir, "TCP_RKN_domains_by_substring.txt"): "rkn\n",
		e.p.LockedTSV: "1\ttls\t2\n",
		filepath.Join(e.p.StateOrchestraDir(), "mode_override.tsv"): "1\tclone\n",
	}
	for path, want := range checks {
		if got := read(t, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestImport(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	info, _ := e.m.Create(CreateOptions{})
	// Экспортируем во внешний файл и импортируем под тем же именем после удаления.
	data := read(t, info.Path)
	ext := filepath.Join(t.TempDir(), "upload.tmp")
	os.WriteFile(ext, []byte(data), 0o644)
	e.m.Delete(info.Name)

	name, err := e.m.Import(ext, info.Name)
	if err != nil || name != info.Name {
		t.Fatalf("Import = %q, %v", name, err)
	}
	// Имя занято — строится новое.
	name2, err := e.m.Import(ext, info.Name)
	if err != nil || name2 == info.Name || !ValidName(name2) {
		t.Fatalf("second Import = %q, %v", name2, err)
	}
	// Некорректное исходное имя — новое имя по времени.
	name3, err := e.m.Import(ext, "../../evil.tar")
	if err != nil || !ValidName(name3) {
		t.Fatalf("Import with bad name = %q, %v", name3, err)
	}
	if l, _ := e.m.List(); len(l) != 3 {
		t.Errorf("List = %d archives", len(l))
	}

	// Не архив и посторонний архив отвергаются.
	junk := filepath.Join(t.TempDir(), "junk")
	os.WriteFile(junk, []byte("not a tar archive at all"), 0o644)
	if _, err := e.m.Import(junk, ""); !errors.Is(err, ErrNotBackup) {
		t.Errorf("junk err = %v", err)
	}
	other := filepath.Join(t.TempDir(), "other.tar")
	os.WriteFile(other, buildTar(t, []tarEntry{{name: "readme.txt", body: "x"}}, false), 0o644)
	if _, err := e.m.Import(other, ""); !errors.Is(err, ErrNotBackup) {
		t.Errorf("foreign archive err = %v", err)
	}
	evil := filepath.Join(t.TempDir(), "evil.tar")
	os.WriteFile(evil, buildTar(t, []tarEntry{{name: "../config", body: "x"}}, false), 0o644)
	if _, err := e.m.Import(evil, ""); !errors.Is(err, ErrNotBackup) {
		t.Errorf("unsafe archive err = %v", err)
	}
}

func TestParseModeAndString(t *testing.T) {
	for in, want := range map[string]Mode{"full": ModeFull, "1": ModeFull, "LISTS": ModeLists, "2": ModeLists, " smart ": ModeSmart, "3": ModeSmart} {
		got, err := ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = %v, %v", in, got, err)
		}
		if got.String() == "unknown" {
			t.Errorf("String(%v)", got)
		}
	}
	if _, err := ParseMode("x"); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("err = %v", err)
	}
	if Mode(0).String() != "unknown" {
		t.Error("zero Mode string")
	}
}

func TestCreateThenRestoreIntoFreshRoot(t *testing.T) {
	src := newEnv(t)
	src.seed(t)
	info, err := src.m.Create(CreateOptions{IncludeLists: true})
	if err != nil {
		t.Fatal(err)
	}
	dst := newEnv(t)
	res, err := dst.m.RestoreFile(info.Path, RestoreOptions{Mode: ModeLists})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst.p.ListsDir, "russia-youtube.txt")) != "youtube.com\n" {
		t.Errorf("extra list not restored: %+v", res)
	}
	if exists(dst.p.ZapretConfig) {
		t.Error("lists mode must not create config")
	}
}
