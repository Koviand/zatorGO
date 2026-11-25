package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/paths"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.1", -1},
		{"v1.2.0", "1.2.0", 0},
		{"1.0.0-rc1", "1.0.0", -1},
		{"2.0.0", "1.9.9", 1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q,%q)=%d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseLatestAndRun(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	payload := []byte("hello-zator\n")
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])
	rel := "lua/hello.lua"
	manifest := fmt.Sprintf("%s|/opt/zator/%s|auto|%s|%d|0\n", rel, rel, sumHex, len(payload))
	verEnv := "ZATOR_VERSION=\"1.2.3\"\nZATOR_SHA=\"abc\"\nWEBUI_VERSION=\"1.2.3\"\nWEBUI_SHA=\"def\"\nTRACKING=\"latest\"\n"

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeTar := func(name string, data []byte) {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
		_, _ = tw.Write(data)
	}
	writeTar(rel, payload)
	writeTar(ManifestRel, []byte(manifest))
	writeTar(VersionEnvRel, []byte(verEnv))
	_ = tw.Close()
	_ = gz.Close()
	archiveBytes := buf.Bytes()
	archSum := sha256.Sum256(archiveBytes)
	archHex := hex.EncodeToString(archSum[:])

	latest := Latest{
		SchemaVersion: 1,
		Release:       "1.2.3",
		ZatorSHA:      "newsha",
		WebUISHA:      "newweb",
		ZatorDate:     "2026-01-01",
		WebUIDate:     "2026-01-01",
		Assets: map[string]Asset{
			VariantFull: {
				Name:   "zator-full.tar.gz",
				Size:   int64(len(archiveBytes)),
				SHA256: archHex,
			},
		},
	}
	latestJSON, _ := json.Marshal(latest)

	mux := http.NewServeMux()
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(latestJSON)
	})
	mux.HandleFunc("/zator-full.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u := New(p)
	u.LatestURL = srv.URL + "/latest.json"
	u.Offline = false

	ctx := context.Background()
	check, err := u.CheckLatest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !check.UpdateAvailable() {
		t.Fatalf("expected update available: %+v", check)
	}

	rep, err := u.Run(ctx, RunOptions{Variant: VariantFull, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Apply == nil || rep.Apply.Installed < 1 {
		t.Fatalf("apply: %+v", rep.Apply)
	}
	got, err := os.ReadFile(filepath.Join(p.ZatorRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("file content = %q", got)
	}
}

func TestDetectOfflineBundle(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	_ = os.MkdirAll(vendor, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "zator-full.tar.gz"), []byte("x"), 0o644)
	st := DetectOffline(func(string) string { return "" }, dir)
	if !st.Offline || st.Reason != "bundle" || st.Bundle == nil {
		t.Fatalf("status = %+v", st)
	}
}

func TestDetectOfflineTruthyAliases(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		st := DetectOffline(func(k string) string {
			if k == EnvOffline {
				return v
			}
			return ""
		}, "")
		if !st.Offline || st.Reason != "env" {
			t.Fatalf("v=%q status=%+v", v, st)
		}
	}
	st := DetectOffline(func(k string) string {
		if k == EnvOffline {
			return "false"
		}
		return ""
	}, "")
	if st.Offline {
		t.Fatalf("false must not be offline: %+v", st)
	}
}

func TestResolveOfflineBundleEnv(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "zator-full.tar.gz"), []byte("x"), 0o644)
	st := ResolveOfflineBundle(func(k string) string {
		if k == EnvBundleDir {
			return dir
		}
		return ""
	})
	if !st.Offline || st.Bundle == nil || st.Bundle.Dir != dir {
		t.Fatalf("%+v", st)
	}
}

func TestResolveOfflineBundleIgnoresCwdWithoutFlag(t *testing.T) {
	// Без ZATOR_BUNDLE_DIR и без Z2R_OFFLINE бандл не подхватывается
	// (даже если бы он лежал в cwd — мы его сюда не передаём).
	st := ResolveOfflineBundle(func(string) string { return "" })
	if st.Offline || st.Bundle != nil {
		t.Fatalf("unexpected offline auto-detect: %+v", st)
	}
}

func TestResolveOfflineBundleCwdOnlyWithEnvFlag(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "zator-full.tar.gz"), []byte("x"), 0o644)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	st := ResolveOfflineBundle(func(k string) string {
		if k == EnvOffline {
			return "1"
		}
		return ""
	})
	if !st.Offline || st.Bundle == nil || st.Bundle.Dir != dir {
		t.Fatalf("want cwd bundle under Z2R_OFFLINE: %+v", st)
	}
}

func TestKeepIfExists(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	_ = p.EnsureDirs()
	existing := []byte("user-list\n")
	targetRel := "lists/netrogat.txt"
	_ = os.MkdirAll(filepath.Join(p.ZatorRoot, "lists"), 0o755)
	_ = os.WriteFile(filepath.Join(p.ZatorRoot, filepath.FromSlash(targetRel)), existing, 0o644)

	newPayload := []byte("new-list\n")
	sum := sha256.Sum256(newPayload)
	manifest := fmt.Sprintf("%s|/opt/zator/%s|keep-if-exists|%s|%d|0\n", targetRel, targetRel, hex.EncodeToString(sum[:]), len(newPayload))
	staging := t.TempDir()
	_ = os.MkdirAll(filepath.Join(staging, "lists"), 0o755)
	_ = os.MkdirAll(filepath.Join(staging, filepath.FromSlash(filepath.Dir(ManifestRel))), 0o755)
	_ = os.WriteFile(filepath.Join(staging, filepath.FromSlash(targetRel)), newPayload, 0o644)
	_ = os.WriteFile(filepath.Join(staging, filepath.FromSlash(ManifestRel)), []byte(manifest), 0o644)

	u := New(p)
	res, err := u.Apply(staging, ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kept != 1 {
		t.Fatalf("kept=%d installed=%d", res.Kept, res.Installed)
	}
	got, _ := os.ReadFile(filepath.Join(p.ZatorRoot, filepath.FromSlash(targetRel)))
	if !bytes.Equal(got, existing) {
		t.Fatalf("keep-if-exists overwritten: %q", got)
	}
}

func TestSafeAssetURL(t *testing.T) {
	u := &Updater{LatestURL: "https://example.com/releases/latest.json"}
	link, err := u.AssetURL(Asset{Name: "zator-core.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(link, "/zator-core.tar.gz") {
		t.Fatalf("url=%s", link)
	}
	_ = io.Discard
}

func TestCompareLatestGitHubNoSHA(t *testing.T) {
	// Binary-only релизы: в Latest нет SHA — нельзя предлагать update при равных версиях.
	cur := Version{ZatorVersion: "0.1.6", WebUIVersion: "0.1.6"}
	same := &Latest{SchemaVersion: 1, Release: "v0.1.6"}
	res := CompareLatest(cur, same, true)
	if res.UpdateAvailable() {
		t.Fatalf("same version must not update: %+v", res)
	}
	newer := &Latest{SchemaVersion: 1, Release: "v0.1.7"}
	res = CompareLatest(cur, newer, true)
	if !res.UpdateZator || !res.UpdateWebUI {
		t.Fatalf("newer tag must update both: %+v", res)
	}
	// Пустой SHA только у latest при непустом cur SHA — fallback на версии.
	curSHA := Version{ZatorVersion: "0.1.6", ZatorSHA: "abc"}
	res = CompareLatest(curSHA, same, false)
	if res.UpdateZator {
		t.Fatalf("empty latest SHA must not force update: %+v", res)
	}
	if res = CompareLatest(cur, nil, true); res.UpdateAvailable() {
		t.Fatalf("nil latest: %+v", res)
	}
}

func TestReadCachedLatest(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	l, err := ReadCachedLatest(p)
	if err != nil || l != nil {
		t.Fatalf("empty cache: l=%v err=%v", l, err)
	}
	raw, _ := json.Marshal(Latest{SchemaVersion: 1, Release: "v0.1.6", ZatorDate: "2026-01-01"})
	if err := os.MkdirAll(Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LatestCachePath(p), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCachedLatest(p)
	if err != nil || got == nil || got.Release != "v0.1.6" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
