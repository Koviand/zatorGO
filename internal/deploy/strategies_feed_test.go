package deploy

import (
	"crypto/ed25519"
	"testing"
)

func TestVerifyStrategiesFeed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tsv := []byte("# profile\tdesync\n1\t--lua-desync=fake\n")
	sig, err := SignSHA256SUMS(priv, tsv)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyStrategiesFeed(pub, tsv, []byte(sig+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStrategiesFeed(pub, tsv, []byte("bad\n")); err == nil {
		t.Fatal("expected bad signature error")
	}
	if err := VerifyStrategiesFeed(pub, append(tsv, 'x'), []byte(sig+"\n")); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestParseStrategiesFeed(t *testing.T) {
	lines := parseStrategiesFeed([]byte("# c\n\n1\t--x\n2\t--y\n"))
	if len(lines) != 2 {
		t.Fatalf("%v", lines)
	}
}
