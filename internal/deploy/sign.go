package deploy

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Имена ассетов подписи SHA256SUMS в GitHub Release.
const (
	SHA256SUMSName    = "SHA256SUMS"
	SHA256SUMSSigName = "SHA256SUMS.sig"
)

// ErrBadSignature — подпись SHA256SUMS не совпала или отсутствует.
var ErrBadSignature = errors.New("deploy: недействительная подпись SHA256SUMS")

// ReleasePublicKeyB64 — встроенный Ed25519 public key (base64 raw 32 байта).
// Пара: секрет GitHub Actions ZATOR_RELEASE_SIGNING_KEY (base64 raw 64-байтного
// private key в формате Go crypto/ed25519).
const ReleasePublicKeyB64 = "zb7FN0Y688ClMPSQeFVgNUqjXfVAASuIgUdzbQ/UtmE="

// releasePublicKeyOverride подменяет pubkey в тестах; nil — встроенный.
var releasePublicKeyOverride ed25519.PublicKey

// SetReleasePublicKeyForTest задаёт pubkey для unit-тестов (сброс: nil).
func SetReleasePublicKeyForTest(pub ed25519.PublicKey) {
	releasePublicKeyOverride = pub
}

// ReleasePublicKey возвращает ключ проверки подписи релизов.
func ReleasePublicKey() (ed25519.PublicKey, error) {
	if len(releasePublicKeyOverride) == ed25519.PublicKeySize {
		return releasePublicKeyOverride, nil
	}
	return ParsePublicKeyB64(ReleasePublicKeyB64)
}

// ParsePublicKeyB64 декодирует raw Ed25519 public key из base64.
func ParsePublicKeyB64(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("%w: pubkey base64: %v", ErrBadSignature, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: pubkey length %d", ErrBadSignature, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKeyB64 декодирует raw Ed25519 private key (64 байта) из base64.
func ParsePrivateKeyB64(s string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("deploy: private key base64: %v", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("deploy: private key length %d", len(raw))
	}
	return ed25519.PrivateKey(raw), nil
}

// SignSHA256SUMS подписывает содержимое SHA256SUMS; возвращает одну строку base64.
func SignSHA256SUMS(priv ed25519.PrivateKey, sums []byte) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("deploy: некорректный private key")
	}
	sig := ed25519.Sign(priv, sums)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// ParseSignature декодирует SHA256SUMS.sig (одна строка base64 или сырой 64 байта).
func ParseSignature(sigFile []byte) ([]byte, error) {
	s := strings.TrimSpace(string(sigFile))
	if s == "" {
		return nil, fmt.Errorf("%w: пустой файл подписи", ErrBadSignature)
	}
	// Игнорируем комментарии (# …) и берём первую непустую строку.
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(line)
		if err != nil {
			return nil, fmt.Errorf("%w: signature base64: %v", ErrBadSignature, err)
		}
		if len(raw) != ed25519.SignatureSize {
			return nil, fmt.Errorf("%w: signature length %d", ErrBadSignature, len(raw))
		}
		return raw, nil
	}
	return nil, fmt.Errorf("%w: нет строки подписи", ErrBadSignature)
}

// VerifySHA256SUMS проверяет Ed25519-подпись содержимого SHA256SUMS.
func VerifySHA256SUMS(pub ed25519.PublicKey, sums, sigFile []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: pubkey", ErrBadSignature)
	}
	sig, err := ParseSignature(sigFile)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, sums, sig) {
		return ErrBadSignature
	}
	return nil
}
