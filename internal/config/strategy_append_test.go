package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeDesyncLine(t *testing.T) {
	got, err := NormalizeDesyncLine("fake:blob=fake_default_tls:repeats=2", 45)
	if err != nil {
		t.Fatal(err)
	}
	want := "--lua-desync=fake:blob=fake_default_tls:repeats=2:strategy=45"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got, err = NormalizeDesyncLine("--lua-desync=multisplit:pos=1:strategy=9 --lua-desync=fake:blob=x:strategy=9", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ":strategy=50") || strings.Contains(got, ":strategy=9") {
		t.Fatalf("strategy rewrite failed: %q", got)
	}
}

func TestAppendProfileStrategyCommon(t *testing.T) {
	src := filepath.Join("..", "..", "assets", "config.default")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	c := Parse(string(raw))
	before := c.ProfileMaxStrategyOrDefault(1)
	n, err := c.AppendProfileStrategy(1, "fake:blob=fake_default_tls:repeats=2 --lua-desync=multisplit:pos=1,midsld")
	if err != nil {
		t.Fatal(err)
	}
	if n != before+1 {
		t.Fatalf("n=%d want %d", n, before+1)
	}
	if c.ProfileMaxStrategyOrDefault(1) != n {
		t.Fatalf("max after=%d want %d", c.ProfileMaxStrategyOrDefault(1), n)
	}
	opt, _ := c.NFQWS2Opt()
	if !strings.Contains(opt, ":strategy="+strconv.Itoa(n)) {
		t.Fatalf("opt missing strategy line")
	}
}

func TestAppendProfileStrategyDiscord(t *testing.T) {
	src := filepath.Join("..", "..", "assets", "config.default")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	c := Parse(string(raw))
	before := c.ProfileMaxStrategyOrDefault(4)
	n, err := c.AppendProfileStrategy(4, "hostfakesplit:tcp_md5")
	if err != nil {
		t.Fatal(err)
	}
	if n != before+1 {
		t.Fatalf("n=%d want %d", n, before+1)
	}
	if c.ProfileMaxStrategyOrDefault(4) != n {
		t.Fatalf("discord max not updated")
	}
}

func TestEnsureProfileStrategyDedupe(t *testing.T) {
	src := filepath.Join("..", "..", "assets", "config.default")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	c := Parse(string(raw))
	desync := "fake:blob=feed_tls:repeats=1"
	n1, added, err := c.EnsureProfileStrategy(1, desync)
	if err != nil || !added {
		t.Fatalf("first: n=%d added=%v err=%v", n1, added, err)
	}
	max1 := c.ProfileMaxStrategyOrDefault(1)
	n2, added, err := c.EnsureProfileStrategy(1, desync)
	if err != nil {
		t.Fatal(err)
	}
	if added || n2 != n1 {
		t.Fatalf("dedupe: n=%d added=%v want n=%d added=false", n2, added, n1)
	}
	if c.ProfileMaxStrategyOrDefault(1) != max1 {
		t.Fatalf("max grew on dedupe")
	}
	// с уже проставленным :strategy= — тоже dedupe
	n3, added, err := c.EnsureProfileStrategy(1, desync+":strategy=99")
	if err != nil || added || n3 != n1 {
		t.Fatalf("strategy-stripped dedupe: n=%d added=%v", n3, added)
	}
}

func TestApplyStrategiesFeedLines(t *testing.T) {
	src := filepath.Join("..", "..", "assets", "config.default")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	c := Parse(string(raw))
	lines := []string{
		"# comment",
		"1\tfake:blob=feed_a:repeats=1",
		"1\tfake:blob=feed_a:repeats=1",
		"4\thostfakesplit:tcp_md5_feed",
		"bad",
		"99\tx",
	}
	r := c.ApplyStrategiesFeedLines(lines)
	if r.Registered != 2 {
		t.Fatalf("registered=%d want 2 (warns=%v)", r.Registered, r.Warnings)
	}
	if r.Skipped != 1 {
		t.Fatalf("skipped=%d want 1", r.Skipped)
	}
	if len(r.Warnings) < 2 {
		t.Fatalf("warnings=%v", r.Warnings)
	}
	r2 := c.ApplyStrategiesFeedLines(lines)
	// две одинаковые строки profile 1 + одна profile 4 → 3 skipped
	if r2.Registered != 0 || r2.Skipped != 3 {
		t.Fatalf("reapply: %+v", r2)
	}
}
