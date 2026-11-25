package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AloofLibra/zator/internal/config"
	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

func TestEnsureNetworkIfacesSetsFromRoute(t *testing.T) {
	// Unit-тест только SetIfaces / NeedsIfaceAutodetect — Detect требует /proc.
	c := config.Parse(`#IFACE_LAN=eth0
#IFACE_WAN=eth1
`)
	if !c.NeedsIfaceAutodetect() {
		t.Fatal("expected needs autodetect")
	}
	if err := c.SetIfaces("br0", "eth3"); err != nil {
		t.Fatal(err)
	}
	if c.NeedsIfaceAutodetect() {
		t.Fatal("after set should not need")
	}
	lan, _ := c.Get(config.IfaceLAN)
	wan, _ := c.Get(config.IfaceWAN)
	if lan != "br0" || wan != "eth3" {
		t.Fatalf("lan=%q wan=%q", lan, wan)
	}
}

func TestRequireRouterIfaces(t *testing.T) {
	if !requireRouterIfaces(platform.Platform{Kind: platform.KindEntware}) {
		t.Fatal("Entware must require ifaces")
	}
	if !requireRouterIfaces(platform.Platform{Kind: platform.KindOpenWRT}) {
		t.Fatal("OpenWRT must require ifaces")
	}
	if requireRouterIfaces(platform.Platform{Kind: platform.KindVPS}) {
		t.Fatal("VPS must soft-fail ifaces")
	}
}

func TestApplyDetectedIfacesOpenWRTSkip(t *testing.T) {
	root := t.TempDir()
	zator := filepath.Join(root, "zator")
	zapret := filepath.Join(root, "zapret2")
	_ = os.MkdirAll(zapret, 0o755)
	cfgPath := filepath.Join(zapret, "config")
	if err := os.WriteFile(cfgPath, []byte("#IFACE_LAN=eth0\n#IFACE_WAN=eth1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewWith(paths.New(zator, zapret), platform.Platform{Kind: platform.KindOpenWRT})
	a.Paths.ZapretConfig = cfgPath
	_, _, err := a.ApplyDetectedIfaces()
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// OpenWrt: ensureNetworkIfaces no-op → всё ещё needs detect
	if !c.NeedsIfaceAutodetect() {
		t.Fatal("OpenWrt should leave IFACE commented")
	}
}
