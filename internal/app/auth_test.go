package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetPasswordVerifyAndUpdatePreserves(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetPassword("secret12"); err != nil {
		t.Fatal(err)
	}
	if !a.Config.AuthEnabled || !a.Config.HasPassword() || a.Config.SessionToken == "" {
		t.Fatalf("after set: %+v", a.Config)
	}
	if !a.VerifyPassword("secret12") || a.VerifyPassword("wrong") {
		t.Fatal("verify")
	}
	session := a.Config.SessionToken
	hash := a.Config.PasswordHash
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if err := a.Update(); err != nil {
		t.Fatal(err)
	}
	if a.Config.PasswordHash != hash || a.Config.SessionToken != session || !a.Config.AuthEnabled {
		t.Fatalf("update cleared auth: %+v", a.Config)
	}
}

func TestDisableEnableAuth(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetPassword("secret12"); err != nil {
		t.Fatal(err)
	}
	a.ClearAuth()
	if a.Config.AuthRequired() || !a.Config.HasPassword() {
		t.Fatalf("clear should disable but keep hash: %+v", a.Config)
	}
	if err := a.EnableAuth(); err != nil {
		t.Fatal(err)
	}
	if !a.Config.AuthRequired() {
		t.Fatal("enable failed")
	}
	a.DisableAuthExplicitly()
	if a.Config.HasPassword() || a.Config.AuthRequired() {
		t.Fatalf("explicit disable should wipe: %+v", a.Config)
	}
	if err := a.EnableAuth(); err != ErrNoPassword {
		t.Fatalf("enable without password: %v", err)
	}
}

func TestSavePreservesDiskAuthWhenNotDirty(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetPassword("secret12"); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	cli := NewWith(a.Paths, a.Platform)
	cli.ConfigPath = a.ConfigPath
	if err := cli.Load(); err != nil {
		t.Fatal(err)
	}
	if err := cli.SetPassword("newpass12"); err != nil {
		t.Fatal(err)
	}
	if err := cli.Save(); err != nil {
		t.Fatal(err)
	}
	// Имитация живого webui: в памяти старый пароль, Save порта не должен затереть CLI.
	a.Config.Port = 18080
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if err := a.ReloadAuth(); err != nil {
		t.Fatal(err)
	}
	if !a.VerifyPassword("newpass12") || a.VerifyPassword("secret12") {
		t.Fatalf("disk auth lost after Save: enabled=%v", a.Config.AuthEnabled)
	}
	if a.Config.Port != 18080 {
		t.Fatalf("port=%d", a.Config.Port)
	}
}

func TestConfigExists(t *testing.T) {
	a := newTestApp(t)
	if a.ConfigExists() {
		t.Fatal("expected no config")
	}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if !a.ConfigExists() {
		t.Fatal("expected config after Save")
	}
}

func TestReloadAuthIfStale(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetPassword("secret12"); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if err := a.ReloadAuthIfStale(); err != nil {
		t.Fatal(err)
	}
	prev := a.authFileMod
	if err := a.ReloadAuthIfStale(); err != nil {
		t.Fatal(err)
	}
	if !a.authFileMod.Equal(prev) {
		t.Fatal("повторный ReloadAuthIfStale не должен перечитывать без смены mtime")
	}
	cli := NewWith(a.Paths, a.Platform)
	cli.ConfigPath = a.ConfigPath
	if err := cli.Load(); err != nil {
		t.Fatal(err)
	}
	if err := cli.SetPassword("newpass12"); err != nil {
		t.Fatal(err)
	}
	if err := cli.Save(); err != nil {
		t.Fatal(err)
	}
	// Форсируем более новый mtime (на FS с грубой гранулярностью).
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(a.ConfigPath, newer, newer); err != nil {
		t.Fatal(err)
	}
	a.authCheckedAt = time.Time{}
	if err := a.ReloadAuthIfStale(); err != nil {
		t.Fatal(err)
	}
	if !a.VerifyPassword("newpass12") {
		t.Fatal("stale reload не подхватил новый пароль")
	}
	if !a.AuthRequiredLocked() {
		t.Fatal("AuthRequiredLocked")
	}
}


func TestAuthEnabledFromPasswordHash(t *testing.T) {
	a := newTestApp(t)
	if err := os.MkdirAll(filepath.Dir(a.ConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"port":17682,"password_hash":"$2a$10$abcdefghijklmnopqrstuu","zapret_source":"fork"}` + "\n"
	if err := os.WriteFile(a.ConfigPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	if !a.Config.AuthEnabled || !a.Config.HasPassword() {
		t.Fatalf("migration from password_hash: %+v", a.Config)
	}
}

func TestMigrateOrchestraCacheToState(t *testing.T) {
	a := newTestApp(t)
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(a.Paths.ExtraStratsDir, "cache", "orchestra")
	if err := os.MkdirAll(filepath.Join(old, "scopes"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "9\ttls\t4\n"
	if err := os.WriteFile(filepath.Join(old, "locked.tsv"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	// state пустой после truncate
	if err := os.WriteFile(a.Paths.LockedTSV, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Remove(a.Paths.LockedTSV)
	if err := a.migrateOrchestraCacheToState(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(a.Paths.LockedTSV)
	if err != nil || string(got) != legacy {
		t.Fatalf("migrated = %q err=%v", got, err)
	}
}

func TestUninstallKeepVsPurge(t *testing.T) {
	a := newTestApp(t)
	if err := a.SetPassword("secret12"); err != nil {
		t.Fatal(err)
	}
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Paths.ListsDir, "netrogat.txt"), []byte("keep.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Uninstall(UninstallOptions{SkipService: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Paths.ZatorConfig); err != nil {
		t.Fatalf("config должен остаться: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.Paths.ListsDir, "netrogat.txt")); err != nil {
		t.Fatalf("lists должны остаться: %v", err)
	}
	if _, err := os.Stat(a.Paths.WebUIDir); !os.IsNotExist(err) {
		t.Errorf("webui должен быть удалён: %v", err)
	}
	if err := a.Uninstall(UninstallOptions{Purge: true, SkipService: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Paths.ZatorRoot); !os.IsNotExist(err) {
		t.Errorf("purge должен снести корень: %v", err)
	}
}
