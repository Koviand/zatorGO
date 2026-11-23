package webapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AloofLibra/zator/internal/app"
	"github.com/AloofLibra/zator/internal/netcheck"
	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
	"github.com/AloofLibra/zator/internal/service"
	"github.com/AloofLibra/zator/internal/sweep"
	"github.com/AloofLibra/zator/internal/uiembed"
)

// fakeSvc — заглушка service.Manager.
type fakeSvc struct {
	mu      sync.Mutex
	running bool
	calls   []string
	failOn  string
}

func (f *fakeSvc) rec(a string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, a)
	if f.failOn == a {
		return os.ErrPermission
	}
	switch a {
	case "start", "restart":
		f.running = true
	case "stop":
		f.running = false
	}
	return nil
}
func (f *fakeSvc) Start(context.Context, service.Unit) error   { return f.rec("start") }
func (f *fakeSvc) Stop(context.Context, service.Unit) error    { return f.rec("stop") }
func (f *fakeSvc) Restart(context.Context, service.Unit) error { return f.rec("restart") }
func (f *fakeSvc) Status(_ context.Context, u service.Unit) (service.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return service.Status{Unit: u, Running: f.running}, nil
}
func (f *fakeSvc) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// fakeChecker — заглушка NetChecker без сети.
type fakeChecker struct {
	mu      sync.Mutex
	targets []string
}

func okResult(label, target string) netcheck.TargetResult {
	return netcheck.TargetResult{Label: label, Target: target, TLS12: 1, TLS13: 1, Verdict: netcheck.VerdictOK, Text: "ok"}
}

func (c *fakeChecker) Check(context.Context) netcheck.CheckResponse {
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{okResult("YouTube", "https://www.youtube.com/")}}
}
func (c *fakeChecker) CheckProfile(ctx context.Context, id int) netcheck.CheckResponse {
	return c.CheckProfileOpts(ctx, id, netcheck.ProfileCheckOptions{})
}
func (c *fakeChecker) CheckProfileOpts(_ context.Context, id int, _ netcheck.ProfileCheckOptions) netcheck.CheckResponse {
	if id == 5 || id == 6 {
		return netcheck.CheckResponse{Results: []netcheck.TargetResult{}, Message: netcheck.MessageUDPProfile}
	}
	r := okResult("profile", "https://x/")
	if id == 2 {
		r.Label = "Googlevideo"
		r.Media = &netcheck.MediaProbe{Class: netcheck.MediaTLS, Text: "TLS only"}
	}
	return netcheck.CheckResponse{Results: []netcheck.TargetResult{r}}
}
func (c *fakeChecker) CheckTarget(_ context.Context, label, raw string) netcheck.TargetResult {
	c.mu.Lock()
	c.targets = append(c.targets, raw)
	c.mu.Unlock()
	return okResult(label, raw)
}

// fakeSweeper — заглушка Sweeper.
type fakeSweeper struct {
	started bool
	params  sweep.Params
}

func (f *fakeSweeper) Start(_ context.Context, p sweep.Params) error {
	if f.started {
		return sweep.ErrBusy
	}
	f.started, f.params = true, p
	return nil
}
func (f *fakeSweeper) Cancel() error {
	if !f.started {
		return sweep.ErrNotRunning
	}
	f.started = false
	return nil
}
func (f *fakeSweeper) Report() (sweep.Report, error) { return sweep.IdleReport(), nil }
func (f *fakeSweeper) Running() bool                 { return f.started }

// env — тестовое окружение.
type env struct {
	t       *testing.T
	app     *app.App
	srv     *Server
	ts      *httptest.Server
	svc     *fakeSvc
	checker *fakeChecker
	sweeper *fakeSweeper
}

type envOpts struct {
	noZapret     bool // не создавать config zapret2
	skipInstall  bool
	customStatic bool
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	resetLoginAttempts()
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	a := app.NewWith(p, platform.Platform{})
	if !o.skipInstall {
		if err := a.Install(); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if !o.noZapret {
		if err := os.MkdirAll(p.ZapretRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := uiembed.ConfigDefault()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.ZapretConfig, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Файлы блобов и Lua для настроек.
	for _, f := range []string{"tls_clienthello_5.bin", "custom_tls.bin", "wg_initial_fake_1.bin", "wg_initial_fake_2.bin"} {
		if err := os.MkdirAll(p.FakeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.FakeDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(p.LuaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"rst-guard.lua", "wssize-ramp.lua"} {
		if err := os.WriteFile(filepath.Join(p.LuaDir, f), []byte("--"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	e := &env{t: t, app: a, svc: &fakeSvc{}, checker: &fakeChecker{}, sweeper: &fakeSweeper{}}
	opts := Options{App: a, Service: e.svc, Checker: e.checker, Sweeper: e.sweeper}
	srv, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	e.srv = srv
	e.ts = httptest.NewServer(srv.Handler())
	t.Cleanup(e.ts.Close)
	return e
}

// do выполняет запрос; form — тело POST (urlencoded).
func (e *env) do(method, path string, form url.Values, hdr map[string]string) (*http.Response, []byte) {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, e.ts.URL+path, body)
	if err != nil {
		e.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (e *env) get(path string) (*http.Response, map[string]any) {
	e.t.Helper()
	return e.json(http.MethodGet, path, nil)
}

func (e *env) post(path string, kv ...string) (*http.Response, map[string]any) {
	e.t.Helper()
	form := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		form.Set(kv[i], kv[i+1])
	}
	return e.json(http.MethodPost, path, form)
}

func (e *env) json(method, path string, form url.Values) (*http.Response, map[string]any) {
	e.t.Helper()
	resp, data := e.do(method, path, form, nil)
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		e.t.Fatalf("%s %s: не JSON: %v\n%s", method, path, err, data)
	}
	return resp, out
}

func (e *env) zapretConfig() string {
	e.t.Helper()
	data, err := os.ReadFile(e.app.Paths.ZapretConfig)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
}

func wantError(t *testing.T, resp *http.Response, body map[string]any, status int, msg string) {
	t.Helper()
	wantStatus(t, resp, status)
	if got, _ := body["error"].(string); got != msg {
		t.Fatalf("error = %q, want %q", got, msg)
	}
}

func wantOK(t *testing.T, resp *http.Response, body map[string]any) {
	t.Helper()
	if resp.StatusCode != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v", resp.StatusCode, body)
	}
}
