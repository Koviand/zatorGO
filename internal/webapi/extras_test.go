package webapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestConfigRawGetAndPost(t *testing.T) {
	e := newEnv(t, envOpts{})
	cfgPath := e.app.Paths.ZapretConfig
	if err := os.WriteFile(cfgPath, []byte("NFQWS2_OPT=\"\n--qnum=200\n\"\nFWTYPE=iptables\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, out := e.get("/cgi-bin/settings.cgi?setting=config_raw")
	wantStatus(t, resp, 200)
	if out["exists"] != true {
		t.Fatalf("expected exists: %v", out)
	}
	resp, out = e.post("/cgi-bin/settings.cgi",
		"setting", "config_raw",
		"value", "NFQWS2_OPT=\"\n--qnum=201\n\"\nFWTYPE=nftables\n",
	)
	wantStatus(t, resp, 200)
	raw := e.zapretConfig()
	if !strings.Contains(raw, "FWTYPE=nftables") || !strings.Contains(raw, "--qnum=201") {
		t.Fatalf("config not updated: %s", raw)
	}
}

func TestConfigRawRejectsWithoutNFQWS2(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.post("/cgi-bin/settings.cgi", "setting", "config_raw", "value", "FWTYPE=iptables\n")
	wantStatus(t, resp, http.StatusBadRequest)
	if out["error"] == nil {
		t.Fatal("expected error")
	}
}

func TestConfigRawRejectsShellBreakout(t *testing.T) {
	e := newEnv(t, envOpts{})
	evil := "NFQWS2_OPT=\"\n--new\n\"\nFOO=\"\n\"; reboot; echo \"\n"
	resp, out := e.post("/cgi-bin/settings.cgi", "setting", "config_raw", "value", evil)
	wantStatus(t, resp, http.StatusBadRequest)
	if out["error"] == nil {
		t.Fatal("expected shell-safety error")
	}
}

func TestConfigRawWritesBak(t *testing.T) {
	e := newEnv(t, envOpts{})
	cfgPath := e.app.Paths.ZapretConfig
	prev := "NFQWS2_OPT=\"\n--qnum=200\n\"\nFWTYPE=iptables\n"
	if err := os.WriteFile(cfgPath, []byte(prev), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, _ := e.post("/cgi-bin/settings.cgi",
		"setting", "config_raw",
		"value", "NFQWS2_OPT=\"\n--qnum=201\n\"\nFWTYPE=nftables\n",
	)
	wantStatus(t, resp, 200)
	bak, err := os.ReadFile(cfgPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != prev {
		t.Fatalf("bak mismatch: %q", bak)
	}
}

func TestNfqws2ErrorsGet(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, out := e.get("/cgi-bin/settings.cgi?setting=nfqws2_errors")
	wantStatus(t, resp, 200)
	if _, ok := out["available"]; !ok {
		t.Fatalf("missing available: %v", out)
	}
}

func TestFwtypeAndFlowoffloadPost(t *testing.T) {
	e := newEnv(t, envOpts{})
	resp, _ := e.post("/cgi-bin/settings.cgi", "setting", "fwtype", "value", "nftables")
	wantStatus(t, resp, 200)
	if !strings.Contains(e.zapretConfig(), "FWTYPE=nftables") {
		t.Fatal("fwtype not set")
	}
	resp, _ = e.post("/cgi-bin/settings.cgi", "setting", "flowoffload", "value", "software")
	wantStatus(t, resp, 200)
	if !strings.Contains(e.zapretConfig(), "FLOWOFFLOAD=software") {
		t.Fatal("flowoffload not set")
	}
}
