package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// authReloadMinInterval — минимальный интервал между проверками mtime zator.json.
const authReloadMinInterval = time.Second

const (
	minPasswordLen = 8
	bcryptCost     = bcrypt.DefaultCost
	sessionBytes   = 32
)

var (
	// ErrPasswordTooShort — пароль короче минимума.
	ErrPasswordTooShort = errors.New("app: пароль слишком короткий (минимум 8 символов)")
	// ErrNoPassword — нет заданного пароля.
	ErrNoPassword = errors.New("app: пароль не задан")
)

// AuthRequired сообщает, нужна ли авторизация Web UI.
func (c Config) AuthRequired() bool {
	return c.AuthEnabled
}

// HasPassword — в конфиге есть bcrypt-хеш пароля.
func (c Config) HasPassword() bool {
	return strings.TrimSpace(c.PasswordHash) != ""
}

// SetPassword записывает bcrypt-хеш, ротирует session_token и включает auth.
func (a *App) SetPassword(password string) error {
	password = strings.TrimSpace(password)
	if len(password) < minPasswordLen {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("app: bcrypt: %w", err)
	}
	session, err := randomHex(sessionBytes)
	if err != nil {
		return err
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.Config.PasswordHash = string(hash)
	a.Config.SessionToken = session
	a.Config.AuthEnabled = true
	a.authDirty = true
	return nil
}

// ClearAuth отключает проверку доступа (пароль/session можно оставить для повторного enable).
func (a *App) ClearAuth() {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.Config.AuthEnabled = false
	a.authDirty = true
}

// DisableAuthExplicitly — сознательный отказ от защиты (install -no-auth / auth disable).
func (a *App) DisableAuthExplicitly() {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.Config.AuthEnabled = false
	a.Config.PasswordHash = ""
	a.Config.SessionToken = ""
	a.authDirty = true
}

// EnableAuth включает проверку. Если пароля нет — ошибка ErrNoPassword.
func (a *App) EnableAuth() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if !a.Config.HasPassword() {
		return ErrNoPassword
	}
	if a.Config.SessionToken == "" {
		session, err := randomHex(sessionBytes)
		if err != nil {
			return err
		}
		a.Config.SessionToken = session
	}
	a.Config.AuthEnabled = true
	a.authDirty = true
	return nil
}

// VerifyPassword проверяет plaintext-пароль против bcrypt-хеша.
func (a *App) VerifyPassword(password string) bool {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if !a.Config.HasPassword() {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(a.Config.PasswordHash), []byte(password))
	return err == nil
}

// RotateSession выдаёт новый session_token (logout / смена пароля уже ротирует).
func (a *App) RotateSession() error {
	session, err := randomHex(sessionBytes)
	if err != nil {
		return err
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.Config.SessionToken = session
	a.authDirty = true
	return nil
}

// SessionEqual — constant-time сравнение с session_token.
func (a *App) SessionEqual(got string) bool {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	want := a.Config.SessionToken
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// ReloadAuth перечитывает auth-поля из zator.json (CLI мог сменить пароль).
func (a *App) ReloadAuth() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.reloadAuthUnlocked()
}

// ReloadAuthIfStale перечитывает auth только если mtime ConfigPath изменился
// (или ещё не закэширован). Пропуск при authDirty (несохранённые in-memory
// правки) и не чаще authReloadMinInterval между проверками mtime.
func (a *App) ReloadAuthIfStale() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.authDirty {
		return nil
	}
	now := time.Now()
	if !a.authCheckedAt.IsZero() && now.Sub(a.authCheckedAt) < authReloadMinInterval {
		return nil
	}
	a.authCheckedAt = now
	fi, err := os.Stat(a.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	mt := fi.ModTime()
	if !a.authFileMod.IsZero() && mt.Equal(a.authFileMod) {
		return nil
	}
	return a.reloadAuthUnlocked()
}

// AuthRequiredLocked — потокобезопасное чтение флага auth.
func (a *App) AuthRequiredLocked() bool {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.Config.AuthRequired()
}

func (a *App) reloadAuthUnlocked() error {
	data, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		return err
	}
	var raw struct {
		AuthEnabled  *bool  `json:"auth_enabled"`
		PasswordHash string `json:"password_hash"`
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("app: разбор %s: %w", a.ConfigPath, err)
	}
	a.Config.PasswordHash = raw.PasswordHash
	a.Config.SessionToken = raw.SessionToken
	if raw.AuthEnabled != nil {
		a.Config.AuthEnabled = *raw.AuthEnabled
	} else {
		a.Config.AuthEnabled = strings.TrimSpace(raw.PasswordHash) != ""
	}
	if fi, err := os.Stat(a.ConfigPath); err == nil {
		a.authFileMod = fi.ModTime()
	}
	a.authCheckedAt = time.Now()
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("app: rand: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
