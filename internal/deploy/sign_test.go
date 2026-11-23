package deploy

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestSignVerifySHA256SUMS(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sums := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  zator-linux-amd64\n")
	sigLine, err := SignSHA256SUMS(priv, sums)
	if err != nil {
		t.Fatal(err)
	}
	sigFile := []byte("# comment\n" + sigLine + "\n")
	if err := VerifySHA256SUMS(pub, sums, sigFile); err != nil {
		t.Fatal(err)
	}
	// Подмена SUMS → отказ.
	if err := VerifySHA256SUMS(pub, append(sums, 'x'), sigFile); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered sums: %v", err)
	}
	// Битая подпись → отказ.
	bad := []byte(strings.Replace(sigLine, "A", "B", 1) + "\n")
	if err := VerifySHA256SUMS(pub, sums, bad); err == nil {
		t.Fatal("expected bad signature error")
	}
}

func TestReleasePublicKeyParses(t *testing.T) {
	pub, err := ReleasePublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("len=%d", len(pub))
	}
}

// TestEmbeddedKeyRoundTrip — опционально: если задан ZATOR_RELEASE_SIGNING_KEY,
// проверяем, что он соответствует вшитому ReleasePublicKeyB64.
func TestEmbeddedKeyRoundTrip(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("ZATOR_RELEASE_SIGNING_KEY"))
	if raw == "" {
		t.Skip("ZATOR_RELEASE_SIGNING_KEY не задан")
	}
	priv, err := ParsePrivateKeyB64(raw)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ReleasePublicKey()
	if err != nil {
		t.Fatal(err)
	}
	sums := []byte("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  zator-linux-x86_64\n")
	sig, err := SignSHA256SUMS(priv, sums)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256SUMS(pub, sums, []byte(sig+"\n")); err != nil {
		t.Fatal(err)
	}
}
