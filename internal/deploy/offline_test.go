package deploy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyOfflineBundleSignatureRequired(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	tar := filepath.Join(dir, "zator-full.tar.gz")
	payload := []byte("offline-payload")
	if err := os.WriteFile(tar, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	b := Bundle{Dir: dir, VendorDir: filepath.Join(dir, "vendor"), Tarball: tar}
	if err := VerifyOfflineBundleSignature(b); err == nil {
		t.Fatal("ожидалась ошибка без SHA256SUMS")
	}

	sum := sha256.Sum256(payload)
	sums := []byte(hex.EncodeToString(sum[:]) + "  zator-full.tar.gz\n")
	if err := os.WriteFile(filepath.Join(dir, SHA256SUMSName), sums, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyOfflineBundleSignature(b); err == nil || !strings.Contains(err.Error(), SHA256SUMSSigName) {
		t.Fatalf("один SUMS без .sig: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	SetReleasePublicKeyForTest(pub)
	t.Cleanup(func() { SetReleasePublicKeyForTest(nil) })
	sigLine, err := SignSHA256SUMS(priv, sums)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SHA256SUMSSigName), []byte(sigLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyOfflineBundleSignature(b); err != nil {
		t.Fatal(err)
	}

	// Подмена tar → отказ.
	if err := os.WriteFile(tar, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyOfflineBundleSignature(b); err == nil {
		t.Fatal("ожидался отказ по sha256")
	}
}
