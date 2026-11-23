package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

func TestServiceWarningsRoundTrip(t *testing.T) {
	root := t.TempDir()
	p := paths.Paths{ZatorRoot: root, StateDir: filepath.Join(root, "state")}
	a := NewWith(p, platform.Platform{})
	a.WriteServiceWarnings([]string{"nfqws2 down", "prepare failed"})
	got := a.ServiceWarnings()
	if len(got) != 2 || got[0] != "nfqws2 down" {
		t.Fatalf("%v", got)
	}
	a.ClearServiceWarnings()
	if len(a.ServiceWarnings()) != 0 {
		t.Fatal("expected cleared")
	}
}

func TestPreUpdateBackupMarkers(t *testing.T) {
	root := t.TempDir()
	p := paths.Paths{
		ZatorRoot:  root,
		StateDir:   filepath.Join(root, "state"),
		BackupDir:  filepath.Join(root, "backup"),
		ZapretRoot: filepath.Join(root, "zapret2"),
		ListsDir:   filepath.Join(root, "lists"),
		ConfigDir:  filepath.Join(root, "config"),
	}
	p.ZapretConfig = filepath.Join(p.ZapretRoot, "config")
	p.BinDir = filepath.Join(root, "bin")
	_ = os.MkdirAll(p.ZapretRoot, 0o755)
	_ = os.MkdirAll(p.ListsDir, 0o755)
	_ = os.MkdirAll(p.BinDir, 0o755)
	_ = os.WriteFile(p.ZapretConfig, []byte("QNUM=200\n"), 0o644)
	_ = os.WriteFile(filepath.Join(p.BinDir, "zator"), []byte("BIN"), 0o755)
	a := NewWith(p, platform.Platform{})
	info, err := a.CreatePreUpdateBackup()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name == "" || a.LastPreUpdateBackup() != info.Name {
		t.Fatalf("name=%q remembered=%q", info.Name, a.LastPreUpdateBackup())
	}
	if !a.BlockFullRestore() {
		t.Fatal("expected block full")
	}
	a.WritePostUpdateVerify(true, "ok")
	st, d := a.PostUpdateVerifyStatus()
	if st != "ok" || d != "ok" {
		t.Fatalf("%s %s", st, d)
	}
}

func TestPreUpdateBinarySnapshotRestore(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	a := NewWith(p, platform.Platform{})
	_ = os.MkdirAll(p.BinDir, 0o755)
	_ = os.MkdirAll(p.StateDir, 0o755)
	_ = os.WriteFile(filepath.Join(p.BinDir, "zator"), []byte("BIN-V1"), 0o755)
	if err := a.snapshotPreUpdateBinary(); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(p.BinDir, "zator"), []byte("BIN-V2"), 0o755)
	if err := a.restorePreUpdateBinary(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(p.BinDir, "zator"))
	if string(got) != "BIN-V1" {
		t.Fatalf("got %q", got)
	}
}

func TestVerifyAfterUpdateRequiresArgs(t *testing.T) {
	root := t.TempDir()
	p := paths.Paths{
		ZatorRoot:  root,
		StateDir:   filepath.Join(root, "state"),
		BinDir:     filepath.Join(root, "bin"),
		ZapretRoot: filepath.Join(root, "zapret2"),
	}
	p.ZapretConfig = filepath.Join(p.ZapretRoot, "config")
	a := NewWith(p, platform.Platform{})
	_ = os.MkdirAll(p.StateDir, 0o755)
	_ = os.MkdirAll(p.BinDir, 0o755)
	_ = os.MkdirAll(p.ZapretRoot, 0o755)
	_ = os.WriteFile(p.ZapretConfig, []byte("QNUM=200\n"), 0o644)
	if err := a.VerifyAfterUpdate(); err == nil {
		t.Fatal("expected fail without bin")
	}
	_ = os.WriteFile(filepath.Join(p.BinDir, "zator"), []byte("x"), 0o755)
	// Пустые args — soft warning, не hard fail (иначе Keenetic rollback после update).
	if err := a.VerifyAfterUpdate(); err != nil {
		t.Fatal(err)
	}
	st, detail := a.PostUpdateVerifyStatus()
	if st != "warn" || !strings.Contains(detail, "nfqws2.args") {
		t.Fatalf("status=%q detail=%q", st, detail)
	}
	_ = os.WriteFile(filepath.Join(p.ZapretRoot, "nfqws2.args"), []byte("--qnum=200\n--new\n"), 0o644)
	if err := a.VerifyAfterUpdate(); err != nil {
		t.Fatal(err)
	}
}

func TestRestartZapretTransactionalRollback(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	a := NewWith(p, platform.Platform{})
	_ = os.MkdirAll(p.ZapretRoot, 0o755)
	_ = os.MkdirAll(p.StateDir, 0o755)
	_ = os.MkdirAll(p.LuaDir, 0o755)
	cfg := `QNUM=200
NFQWS2_OPT="
--new
--filter-tcp=443
"
`
	_ = os.WriteFile(p.ZapretConfig, []byte(cfg), 0o644)
	restarts := 0
	err := a.RestartZapretTransactional(context.Background(), func(context.Context) error {
		restarts++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarts < 1 {
		t.Fatal("expected restart")
	}
	// Apply fail: shell-metachar в OPT → ValidateNfqws2Args.
	_ = os.WriteFile(p.ZapretConfig, []byte("QNUM=200\nNFQWS2_OPT=\"\n--new\n--filter-tcp=443;evil\n\"\n"), 0o644)
	if err := a.RestartZapretTransactional(context.Background(), func(context.Context) error {
		t.Fatal("restart must not run after apply fail")
		return nil
	}); err == nil {
		t.Fatal("expected apply fail")
	}
}

func TestSyncAutoLockedPersist(t *testing.T) {
	root := t.TempDir()
	p := paths.New(filepath.Join(root, "zator"), filepath.Join(root, "zapret2"))
	a := NewWith(p, platform.Platform{})
	_ = os.MkdirAll(filepath.Dir(p.AutoLockedTSV), 0o755)
	body := "host.example\t1\ttls\t3\n"
	_ = os.WriteFile(p.AutoLockedTSV, []byte(body), 0o644)
	// Live path is /tmp — skip if not writable; use Persist from state only.
	a.SyncAutoLockedPersist()
	if got, _ := os.ReadFile(p.AutoLockedTSV); string(got) != body && len(got) == 0 {
		t.Fatalf("state lost: %q", got)
	}
}
