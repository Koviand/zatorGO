package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	p := paths.New(filepath.Join(t.TempDir(), "zator"), filepath.Join(t.TempDir(), "zapret2"))
	return NewWith(p, platform.Platform{Kind: platform.KindVPS})
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	a := newTestApp(t)
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	if a.Config.Port != DefaultPort || a.Config.ZapretSource != ZapretSourceFork {
		t.Fatalf("defaults: %+v", a.Config)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	a := newTestApp(t)
	a.Config = Config{Port: 18000, Mirror: "https://m.example", ZapretSource: ZapretSourceFork}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	b := NewWith(a.Paths, a.Platform)
	if err := b.Load(); err != nil {
		t.Fatal(err)
	}
	if b.Config != a.Config {
		t.Fatalf("got %+v, want %+v", b.Config, a.Config)
	}
}

func TestValidate(t *testing.T) {
	c := Config{Port: 70000}
	if err := c.Validate(); err == nil {
		t.Error("ожидалась ошибка порта")
	}
	c = Config{ZapretSource: "bogus"}
	if err := c.Validate(); err == nil {
		t.Error("ожидалась ошибка источника")
	}
	c = Config{}
	if err := c.Validate(); err != nil || c.Port != DefaultPort || c.ZapretSource != ZapretSourceFork {
		t.Errorf("значения по умолчанию: %+v, err=%v", c, err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	a := newTestApp(t)
	if err := os.MkdirAll(filepath.Dir(a.ConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.ConfigPath, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Load(); err == nil {
		t.Fatal("ожидалась ошибка разбора")
	}
}

func TestUpdateRejectsPartialInstallWithoutConfig(t *testing.T) {
	a := newTestApp(t)
	if err := os.MkdirAll(a.Paths.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(a.Paths.BinDir, "zator")
	if err := os.WriteFile(bin, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !a.Installed() {
		t.Fatal("Installed() должен быть true при наличии bin/zator")
	}
	if a.ConfigExists() {
		t.Fatal("zator.json не должен существовать")
	}
	err := a.Update()
	if err == nil {
		t.Fatal("Update без zator.json должен вернуть ошибку")
	}
	if !strings.Contains(err.Error(), "завершите install") {
		t.Fatalf("unexpected err: %v", err)
	}
	if a.ConfigExists() {
		t.Fatal("Update не должен создавать zator.json с AuthEnabled=false")
	}
}

func TestInstallUpdateUninstall(t *testing.T) {
	a := newTestApp(t)
	if a.Installed() {
		t.Fatal("не должно быть установлено")
	}
	if err := a.Update(); err == nil {
		t.Fatal("update без установки должен вернуть ошибку")
	}
	if err := a.SetPassword("test-pass"); err != nil {
		t.Fatal(err)
	}
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if !a.Installed() {
		t.Fatalf("установка неполная: %+v", a.Config)
	}
	if !a.Config.AuthEnabled || !a.Config.HasPassword() {
		t.Fatalf("ожидался пароль после install: %+v", a.Config)
	}
	if _, err := os.Stat(filepath.Join(a.Paths.WebUIDir, "index.html")); err != nil {
		t.Errorf("index.html не распакован: %v", err)
	}
	hash := a.Config.PasswordHash
	if err := a.Update(); err != nil {
		t.Fatal(err)
	}
	if a.Config.PasswordHash != hash || !a.Config.AuthEnabled {
		t.Error("update не должен снимать auth")
	}
	if err := a.Uninstall(UninstallOptions{Purge: true, SkipService: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Paths.ZatorRoot); !os.IsNotExist(err) {
		t.Errorf("каталог должен быть удалён: %v", err)
	}
}

func TestWebUIURL(t *testing.T) {
	a := newTestApp(t)
	if got := a.WebUIURL(""); got != "http://localhost:17682/" {
		t.Errorf("got %q", got)
	}
	if got := a.WebUIURL("1.2.3.4"); got != "http://1.2.3.4:17682/" {
		t.Errorf("got %q", got)
	}
}

func TestInstallUnpacksStockFakes(t *testing.T) {
	a := newTestApp(t)
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(a.Paths.FakeDir)
	if err != nil {
		t.Fatal(err)
	}
	var tls, wg int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(strings.ToLower(n), "tls_") || n == "custom_tls.bin" {
			tls++
		}
		if strings.HasPrefix(strings.ToLower(n), "wg_initial_fake_") {
			wg++
		}
	}
	if tls < 30 {
		t.Fatalf("ожидалось ≥30 tls blob, got %d (всего %d)", tls, len(entries))
	}
	if wg < 4 {
		t.Fatalf("ожидалось ≥4 wg blob, got %d", wg)
	}
	// Soft uninstall не должен сносить FakeDir.
	if err := a.Uninstall(UninstallOptions{SkipService: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Paths.FakeDir, "tls_clienthello_5.bin")); err != nil {
		t.Fatalf("FakeDir должен сохраняться при uninstall без purge: %v", err)
	}
	// EnsureStockFakes восстанавливает после полной очистки каталога.
	_ = os.RemoveAll(a.Paths.FakeDir)
	if err := a.EnsureStockFakes(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Paths.FakeDir, "custom_tls.bin")); err != nil {
		t.Fatalf("EnsureStockFakes не восстановил blob: %v", err)
	}
}

func TestInstallCreatesOrchestraState(t *testing.T) {
	a := newTestApp(t)
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a.Paths.LockedTSV, a.Paths.LockedManualTSV} {
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			t.Errorf("нет файла состояния %s: %v", p, err)
		}
	}
	for _, d := range []string{a.Paths.StateOrchestraDir(), filepath.Join(a.Paths.StateOrchestraDir(), "scopes"), a.Paths.ExtraStratsDir, a.Paths.ConfigDir} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("нет каталога %s: %v", d, err)
		}
	}
	if _, err := os.Stat(a.Paths.ZatorConfig); err != nil {
		t.Errorf("нет zator.json: %v", err)
	}
	// Каталога zapret2 нет — живой config не создаётся.
	if _, err := os.Stat(a.Paths.ZapretConfig); err == nil {
		t.Error("config zapret2 создан без каталога zapret2")
	}
}

func TestInstallKeepsLocksAndSeedsZapretConfig(t *testing.T) {
	a := newTestApp(t)
	if err := os.MkdirAll(a.Paths.ZapretRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(a.Paths.ZapretConfig); err != nil || st.Size() == 0 {
		t.Fatalf("config zapret2 не создан из шаблона: %v", err)
	}
	// Повторная установка не затирает существующие locked.tsv и config.
	if err := os.WriteFile(a.Paths.LockedTSV, []byte("1\ttls\t5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.Paths.ZapretConfig, []byte("CUSTOM=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(a.Paths.LockedTSV); string(data) != "1\ttls\t5\n" {
		t.Errorf("locked.tsv затёрт: %q", data)
	}
	if data, _ := os.ReadFile(a.Paths.ZapretConfig); string(data) != "CUSTOM=1\n" {
		t.Errorf("config затёрт: %q", data)
	}
}
