package deploy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/paths"
)

func withTestReleaseKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	SetReleasePublicKeyForTest(pub)
	t.Cleanup(func() { SetReleasePublicKeyForTest(nil) })
	return priv
}

func signSums(t *testing.T, priv ed25519.PrivateKey, sums string) string {
	t.Helper()
	sig, err := SignSHA256SUMS(priv, []byte(sums))
	if err != nil {
		t.Fatal(err)
	}
	return "# test\n" + sig + "\n"
}

func assetsWithSums(wantName string, size int) []map[string]any {
	return []map[string]any{
		{"name": wantName, "browser_download_url": "REPLACE/" + wantName, "size": size},
		{"name": SHA256SUMSName, "browser_download_url": "REPLACE/" + SHA256SUMSName, "size": 40},
		{"name": SHA256SUMSSigName, "browser_download_url": "REPLACE/" + SHA256SUMSSigName, "size": 40},
	}
}

func TestLatestJSONURLUsesKoviand(t *testing.T) {
	t.Setenv(EnvOwner, "")
	t.Setenv(EnvRepo, "")
	t.Setenv(EnvLatestURL, "")
	got := LatestJSONURL()
	want := "https://github.com/Koviand/zatorGO/releases/download/latest/latest.json"
	if got != want {
		t.Fatalf("LatestJSONURL() = %q, want %q", got, want)
	}
}

func TestLatestJSONURLEnvOverride(t *testing.T) {
	t.Setenv(EnvOwner, "Acme")
	t.Setenv(EnvRepo, "zator")
	t.Setenv(EnvLatestURL, "")
	got := LatestJSONURL()
	want := "https://github.com/Acme/zator/releases/download/latest/latest.json"
	if got != want {
		t.Fatalf("LatestJSONURL() = %q, want %q", got, want)
	}
}

func TestFetchGitHubLatest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Koviand/zatorGO/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.1.5",
			"published_at": "2026-11-13T05:00:00Z",
			"assets":       []any{},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	prev := GitHubAPIBase
	GitHubAPIBase = srv.URL
	defer func() { GitHubAPIBase = prev }()

	t.Setenv(EnvOwner, "Koviand")
	t.Setenv(EnvRepo, "zatorGO")

	u := New(paths.New(t.TempDir(), t.TempDir()))
	u.Offline = false
	l, err := u.FetchGitHubLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if l.Release != "v0.1.5" {
		t.Fatalf("release = %q", l.Release)
	}
}

func TestParseSHA256SUMS(t *testing.T) {
	body := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  zator-linux-amd64\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb *zator-linux-arm64\n")
	if got := parseSHA256SUMS(body, "zator-linux-amd64"); got != strings.Repeat("a", 64) {
		t.Fatalf("amd64 = %q", got)
	}
	if got := parseSHA256SUMS(body, "zator-linux-arm64"); got != strings.Repeat("b", 64) {
		t.Fatalf("arm64 = %q", got)
	}
	if got := parseSHA256SUMS(body, "missing"); got != "" {
		t.Fatalf("missing = %q", got)
	}
}

func TestFindGitHubBinaryURLRequiresSHA256SUMS(t *testing.T) {
	wantName := binaryAssetName()
	payload := []byte("fake-zator-binary")
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])
	priv := withTestReleaseKey(t)
	okSums := good + "  " + wantName + "\n"
	okSig := signSums(t, priv, okSums)
	missingSums := strings.Repeat("a", 64) + "  other-file\n"
	missingSig := signSums(t, priv, missingSums)

	cases := []struct {
		name       string
		assets     []map[string]any
		sumsBody   string
		sigBody    string
		sumsStatus int
		wantErr    string
		wantSHA    string
	}{
		{
			name: "no sums asset",
			assets: []map[string]any{
				{"name": wantName, "browser_download_url": "REPLACE/" + wantName, "size": len(payload)},
			},
			wantErr: SHA256SUMSName,
		},
		{
			name: "no signature asset",
			assets: []map[string]any{
				{"name": wantName, "browser_download_url": "REPLACE/" + wantName, "size": len(payload)},
				{"name": SHA256SUMSName, "browser_download_url": "REPLACE/" + SHA256SUMSName, "size": 40},
			},
			wantErr: SHA256SUMSSigName,
		},
		{
			name:       "sums missing entry",
			assets:     assetsWithSums(wantName, len(payload)),
			sumsBody:   missingSums,
			sigBody:    missingSig,
			wantErr:    "нет суммы",
		},
		{
			name:       "sums http error",
			assets:     assetsWithSums(wantName, len(payload)),
			sumsStatus: http.StatusNotFound,
			wantErr:    "404",
		},
		{
			name:     "bad signature",
			assets:   assetsWithSums(wantName, len(payload)),
			sumsBody: okSums,
			sigBody:  signSums(t, priv, "tampered\n"),
			wantErr:  "подпис",
		},
		{
			name:     "ok",
			assets:   assetsWithSums(wantName, len(payload)),
			sumsBody: okSums,
			sigBody:  okSig,
			wantSHA:  good,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			var srvURL string
			mux.HandleFunc("/repos/Koviand/zatorGO/releases/latest", func(w http.ResponseWriter, r *http.Request) {
				assets := make([]map[string]any, len(tc.assets))
				for i, a := range tc.assets {
					assets[i] = map[string]any{}
					for k, v := range a {
						assets[i][k] = v
					}
					if u, ok := assets[i]["browser_download_url"].(string); ok {
						assets[i]["browser_download_url"] = strings.Replace(u, "REPLACE", srvURL, 1)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"tag_name": "v0.1.20",
					"assets":   assets,
				})
			})
			mux.HandleFunc("/"+wantName, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(payload)
			})
			mux.HandleFunc("/"+SHA256SUMSName, func(w http.ResponseWriter, r *http.Request) {
				if tc.sumsStatus != 0 {
					w.WriteHeader(tc.sumsStatus)
					return
				}
				_, _ = w.Write([]byte(tc.sumsBody))
			})
			mux.HandleFunc("/"+SHA256SUMSSigName, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.sigBody))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			srvURL = srv.URL

			prev := GitHubAPIBase
			GitHubAPIBase = srv.URL
			defer func() { GitHubAPIBase = prev }()

			t.Setenv(EnvOwner, "Koviand")
			t.Setenv(EnvRepo, "zatorGO")

			u := New(paths.New(t.TempDir(), t.TempDir()))
			u.Offline = false
			u.Fetch = fetch.NewClient()
			u.Fetch.HTTP = srv.Client()

			_, _, _, sha, err := u.FindGitHubBinaryURL(context.Background())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sha != tc.wantSHA {
				t.Fatalf("sha = %q, want %q", sha, tc.wantSHA)
			}
		})
	}
}

func TestApplyGitHubBinaryRejectsBadSHA(t *testing.T) {
	payload := []byte("fake-zator-binary")
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])
	bad := strings.Repeat("0", 64)
	if bad == good {
		t.Fatal("unexpected collision")
	}
	wantName := binaryAssetName()
	priv := withTestReleaseKey(t)
	// Подпись валидна для «плохого» манифеста: проверка подписи проходит,
	// отказ — на Download по несовпадению sha256 файла.
	sumsBody := bad + "  " + wantName + "\n"
	sigBody := signSums(t, priv, sumsBody)

	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/repos/Koviand/zatorGO/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.1.16",
			"assets": []map[string]any{
				{
					"name":                 wantName,
					"browser_download_url": srvURL + "/" + wantName,
					"size":                 len(payload),
				},
				{
					"name":                 SHA256SUMSName,
					"browser_download_url": srvURL + "/" + SHA256SUMSName,
					"size":                 80,
				},
				{
					"name":                 SHA256SUMSSigName,
					"browser_download_url": srvURL + "/" + SHA256SUMSSigName,
					"size":                 80,
				},
			},
		})
	})
	mux.HandleFunc("/"+wantName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/"+SHA256SUMSName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumsBody))
	})
	mux.HandleFunc("/"+SHA256SUMSSigName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sigBody))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	prev := GitHubAPIBase
	GitHubAPIBase = srv.URL
	defer func() { GitHubAPIBase = prev }()

	t.Setenv(EnvOwner, "Koviand")
	t.Setenv(EnvRepo, "zatorGO")

	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	u := New(p)
	u.Offline = false
	u.Fetch = fetch.NewClient()
	u.Fetch.HTTP = srv.Client()

	_, tag, err := u.ApplyGitHubBinary(context.Background())
	if err == nil {
		t.Fatal("ожидался отказ по SHA")
	}
	if !errors.Is(err, fetch.ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum (tag=%s)", err, tag)
	}
}
