package app

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/uiembed"
)

func TestNfqwsWatchdogHasBackoffAndRestartFW(t *testing.T) {
	data, err := fs.ReadFile(uiembed.Init(), "entware/nfqws-watchdog")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, needle := range []string{
		"restart-fw",
		"MAX_INTERVAL",
		"MAX_PER_HOUR",
		"bump_backoff",
		"rate-limit",
		"LOG_MAX_BYTES",
		"--qnum=65535", // ignore supersweep/relock probe
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("watchdog missing %q", needle)
		}
	}
}

func TestWebUIWatchdogAsset(t *testing.T) {
	data, err := fs.ReadFile(uiembed.Init(), "entware/webui-watchdog")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, needle := range []string{"S92zator-webui", "MAX_PER_HOUR", "webui.want_running", "bump_backoff"} {
		if !strings.Contains(s, needle) {
			t.Errorf("webui-watchdog missing %q", needle)
		}
	}
}
