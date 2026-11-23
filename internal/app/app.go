// Package app содержит общий контекст приложения zator и его конфигурацию.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AloofLibra/zator/internal/paths"
	"github.com/AloofLibra/zator/internal/platform"
)

const (
	// DefaultPort — порт Web UI по умолчанию.
	DefaultPort = 17682

	// ZapretSourceOfficial — официальный репозиторий zapret2.
	ZapretSourceOfficial = "official"
	// ZapretSourceFork — форк zapret2.
	ZapretSourceFork = "fork"
)

// Config — содержимое zator.json.
type Config struct {
	Port         int    `json:"port"`
	AuthEnabled  bool   `json:"auth_enabled"`
	PasswordHash string `json:"password_hash,omitempty"` // bcrypt; plaintext не храним
	SessionToken string `json:"session_token,omitempty"` // cookie / Bearer после логина
	Mirror       string `json:"mirror"`
	ZapretSource string `json:"zapret_source"`
}

// DefaultConfig возвращает конфигурацию по умолчанию.
func DefaultConfig() Config {
	return Config{
		Port:         DefaultPort,
		ZapretSource: ZapretSourceFork,
		AuthEnabled:  false,
	}
}

// Validate проверяет корректность значений и подставляет значения по умолчанию.
func (c *Config) Validate() error {
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("app: некорректный порт %d", c.Port)
	}
	switch c.ZapretSource {
	case "":
		c.ZapretSource = ZapretSourceFork
	case ZapretSourceOfficial, ZapretSourceFork:
	default:
		return fmt.Errorf("app: неизвестный источник zapret %q (official|fork)", c.ZapretSource)
	}
	return nil
}

// App — общий контекст: пути, платформа и конфигурация.
type App struct {
	Paths      paths.Paths
	Platform   platform.Platform
	ConfigPath string // путь к zator.json
	Config     Config

	authMu        sync.Mutex // Config auth-поля + Save
	authDirty     bool       // true — Save не подмешивает auth с диска (CLI/login)
	authFileMod   time.Time  // mtime ConfigPath на момент последнего ReloadAuth
	authCheckedAt time.Time  // когда последний раз смотрели mtime
}

// New создаёт App с раскладкой из переменных окружения, определённой
// платформой и конфигурацией по умолчанию (без чтения с диска).
func New() *App {
	return NewWith(paths.FromEnv(), platform.Detect())
}

// NewWith создаёт App из готовых путей и платформы.
func NewWith(p paths.Paths, pl platform.Platform) *App {
	return &App{
		Paths:      p,
		Platform:   pl,
		ConfigPath: p.ZatorConfig,
		Config:     DefaultConfig(),
	}
}

// Load создаёт App и загружает zator.json, если файл существует.
func Load() (*App, error) {
	a := New()
	if err := a.Load(); err != nil {
		return a, err
	}
	return a, nil
}

// Load читает конфигурацию из ConfigPath. Если файла нет, остаётся
// конфигурация по умолчанию и ошибка не возвращается.
func (a *App) Load() error {
	data, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			a.Config = DefaultConfig()
			return nil
		}
		return fmt.Errorf("app: чтение %s: %w", a.ConfigPath, err)
	}
	var raw struct {
		Port         int    `json:"port"`
		AuthEnabled  *bool  `json:"auth_enabled"`
		PasswordHash string `json:"password_hash"`
		SessionToken string `json:"session_token"`
		Mirror       string `json:"mirror"`
		ZapretSource string `json:"zapret_source"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("app: разбор %s: %w", a.ConfigPath, err)
	}
	cfg := DefaultConfig()
	cfg.Port = raw.Port
	cfg.PasswordHash = raw.PasswordHash
	cfg.SessionToken = raw.SessionToken
	cfg.Mirror = raw.Mirror
	cfg.ZapretSource = raw.ZapretSource
	if raw.AuthEnabled != nil {
		cfg.AuthEnabled = *raw.AuthEnabled
	} else {
		// Старые конфиги без auth_enabled: наличие пароля = защита включена.
		cfg.AuthEnabled = strings.TrimSpace(raw.PasswordHash) != ""
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	a.Config = cfg
	return nil
}

// ConfigExists — на диске есть zator.json (отличие от «только bin/zator»).
func (a *App) ConfigExists() bool {
	st, err := os.Stat(a.ConfigPath)
	return err == nil && !st.IsDir()
}

// Save атомарно записывает конфигурацию в ConfigPath (права 0600 — секреты).
// Если auth не менялся в этом процессе (authDirty=false), auth-поля
// подтягиваются с диска — чтобы Save из Web UI не затирал `zator auth …`.
func (a *App) Save() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if !a.authDirty {
		if err := a.reloadAuthUnlocked(); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := a.Config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.Config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(a.ConfigPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".zator-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, 0o600)
	if err := os.Rename(tmpName, a.ConfigPath); err != nil {
		return err
	}
	a.authDirty = false
	return nil
}

// WebUIURL возвращает URL Web UI (без секретов в query).
func (a *App) WebUIURL(host string) string {
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d/", host, a.Config.Port)
}

// Installed сообщает, установлен ли zator.
func (a *App) Installed() bool { return a.Paths.Installed() }
