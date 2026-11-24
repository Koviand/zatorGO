package webapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBlockedImportIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.5.5", true},
		{"169.254.169.254", true},
		{"100.64.1.1", true},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2001:4860:4860::8888", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("parse %s", c.ip)
		}
		if got := blockedImportIP(ip); got != c.want {
			t.Errorf("%s: blocked=%v want %v", c.ip, got, c.want)
		}
	}
}

func TestValidatePublicHTTPURLRejects(t *testing.T) {
	bads := []string{
		"",
		"ftp://example.com/x",
		"http://localhost/list.txt",
		"http://127.0.0.1/x",
		"http://192.168.0.1/x",
		"http://10.1.2.3/x",
		"http://169.254.169.254/latest/",
		"http://user:pass@example.com/x",
		"http://metadata.google.internal/",
		"http://foo.localhost/x",
	}
	for _, raw := range bads {
		if _, err := validatePublicHTTPURL(raw); err == nil {
			t.Errorf("expected reject: %s", raw)
		}
	}
}

func TestValidatePublicHTTPURLAcceptsPublicIPLiteral(t *testing.T) {
	u, err := validatePublicHTTPURL("https://1.1.1.1/path")
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "1.1.1.1" {
		t.Fatalf("%v", u)
	}
}

func TestFetchDomainListURLBlocksLoopbackServer(t *testing.T) {
	e := newEnv(t, envOpts{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("evil.com\n"))
	}))
	t.Cleanup(srv.Close)

	_, err := e.srv.fetchDomainListURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("loopback httptest must be blocked")
	}
}
