package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Признаки офлайн-режима.
const (
	// EnvOffline — Z2R_OFFLINE=1 (так же называется флаг в shell-версии;
	// выставляется офлайн-лаунчером после проверки бандла).
	EnvOffline = "Z2R_OFFLINE"
	// EnvOfflineAlias — синоним ZATOR_OFFLINE=1.
	EnvOfflineAlias = "ZATOR_OFFLINE"
	// EnvBundleDir — каталог офлайн-бандла (vendor/ + zator-*.tar.gz).
	EnvBundleDir = "ZATOR_BUNDLE_DIR"
	// BundleVendorDir — подкаталог бандла с зависимостями.
	BundleVendorDir = "vendor"
	// BundleTarballGlob — шаблон архива zator внутри бандла.
	BundleTarballGlob = "zator-*.tar.gz"
)

// Bundle — офлайн-бандл: каталог с vendor/ и архивом zator-<variant>.tar.gz.
type Bundle struct {
	Dir       string
	VendorDir string
	// Tarball — архив zator; при нескольких предпочитается zator-full.tar.gz.
	Tarball string
}

// FindBundle ищет офлайн-бандл в каталоге dir.
func FindBundle(dir string) (Bundle, bool) {
	if dir == "" {
		return Bundle{}, false
	}
	vendor := filepath.Join(dir, BundleVendorDir)
	if st, err := os.Stat(vendor); err != nil || !st.IsDir() {
		return Bundle{}, false
	}
	matches, err := filepath.Glob(filepath.Join(dir, BundleTarballGlob))
	if err != nil || len(matches) == 0 {
		return Bundle{}, false
	}
	var files []string
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.Mode().IsRegular() {
			files = append(files, m)
		}
	}
	if len(files) == 0 {
		return Bundle{}, false
	}
	sort.Strings(files)
	tar := files[len(files)-1]
	for _, f := range files {
		if filepath.Base(f) == "zator-full.tar.gz" {
			tar = f
		}
	}
	return Bundle{Dir: dir, VendorDir: vendor, Tarball: tar}, true
}

// OfflineStatus — результат определения офлайн-режима.
type OfflineStatus struct {
	Offline bool
	// Reason — "env" (Z2R_OFFLINE=1), "bundle" (найден бандл) или "".
	Reason string
	Bundle *Bundle
}

// DetectOffline определяет офлайн-режим: по переменной окружения (getenv, nil
// — os.Getenv) и по наличию бандла в каталоге bundleDir ("" — не искать).
func DetectOffline(getenv func(string) string, bundleDir string) OfflineStatus {
	if getenv == nil {
		getenv = os.Getenv
	}
	var st OfflineStatus
	if b, ok := FindBundle(bundleDir); ok {
		st.Bundle = &b
	}
	for _, k := range []string{EnvOffline, EnvOfflineAlias} {
		if envTruthy(getenv(k)) {
			st.Offline, st.Reason = true, "env"
			return st
		}
	}
	if st.Bundle != nil {
		st.Offline, st.Reason = true, "bundle"
	}
	return st
}

// ApplyBundle применяет архив офлайн-бандла без обращения к сети. Версия
// записывается с TRACKING = "offline-<tag>" (tag пуст — "offline").
// Если в каталоге бандла есть SHA256SUMS и SHA256SUMS.sig — обязательна
// проверка Ed25519 и sha256 архива (fail-closed при битой/частичной подписи).
func (u *Updater) ApplyBundle(b Bundle, tag string) (*ApplyResult, error) {
	if err := VerifyOfflineBundleSignature(b); err != nil {
		return nil, err
	}
	tracking := "offline"
	if tag = strings.TrimSpace(tag); tag != "" {
		tracking = "offline-" + tag
	}
	return u.ApplyArchive(b.Tarball, ApplyOptions{Tracking: tracking})
}

// VerifyOfflineBundleSignature проверяет Ed25519 SHA256SUMS[.sig] рядом с tar.
// Нет обоих файлов — ошибка (не расширяем AllowInsecure). Один без другого — ошибка.
func VerifyOfflineBundleSignature(b Bundle) error {
	sumsPath := filepath.Join(b.Dir, SHA256SUMSName)
	sigPath := filepath.Join(b.Dir, SHA256SUMSSigName)
	sums, errSums := os.ReadFile(sumsPath)
	sig, errSig := os.ReadFile(sigPath)
	hasSums := errSums == nil
	hasSig := errSig == nil
	switch {
	case !hasSums && !hasSig:
		return fmt.Errorf("deploy: офлайн-бандл без %s/%s — отказоустойчивый режим не расширяется; пересоберите pack-offline с подписью", SHA256SUMSName, SHA256SUMSSigName)
	case hasSums != hasSig:
		return fmt.Errorf("deploy: офлайн-бандл: нужны оба файла %s и %s", SHA256SUMSName, SHA256SUMSSigName)
	}
	pub, err := ReleasePublicKey()
	if err != nil {
		return err
	}
	if err := VerifySHA256SUMS(pub, sums, sig); err != nil {
		return fmt.Errorf("deploy: офлайн SHA256SUMS.sig: %w", err)
	}
	wantName := filepath.Base(b.Tarball)
	want := parseSHA256SUMS(sums, wantName)
	if want == "" {
		return fmt.Errorf("deploy: в SHA256SUMS нет суммы для %s", wantName)
	}
	f, err := os.Open(b.Tarball)
	if err != nil {
		return fmt.Errorf("deploy: офлайн tar: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("deploy: офлайн tar hash: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("deploy: sha256 офлайн-архива не совпал (ожидалось %s, получено %s)", want, got)
	}
	return nil
}

// ResolveOfflineBundle ищет офлайн-бандл.
//
// Каталоги:
//   - ZATOR_BUNDLE_DIR — всегда (явный путь);
//   - cwd и каталог бинарника — только при Z2R_OFFLINE=1 / ZATOR_OFFLINE=1
//     (иначе случайный zator-*.tar.gz в /tmp или ~ применялся бы от root).
//
// Offline=true также при Z2R_OFFLINE=1 без бандла (сеть отключена).
func ResolveOfflineBundle(getenv func(string) string) OfflineStatus {
	if getenv == nil {
		getenv = os.Getenv
	}
	st := DetectOffline(getenv, "")
	dirs := make([]string, 0, 4)
	if v := strings.TrimSpace(getenv(EnvBundleDir)); v != "" {
		dirs = append(dirs, v)
	}
	if st.Offline && st.Reason == "env" {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			dirs = append(dirs, cwd)
		}
		if ex, err := os.Executable(); err == nil && ex != "" {
			dirs = append(dirs, filepath.Dir(ex))
		}
	}
	for _, d := range dirs {
		if b, ok := FindBundle(d); ok {
			bb := b
			st.Bundle = &bb
			if !st.Offline {
				st.Offline, st.Reason = true, "bundle"
			}
			return st
		}
	}
	return st
}

// envTruthy — как zapret.envTruthy: 1/true/yes/on (без учёта регистра).
func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
