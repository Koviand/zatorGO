package zapret

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSHA256SumFileArchiveAndPerFile(t *testing.T) {
	body := []byte(strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  zapret2-v1.0.7.tar.gz",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  zapret2-v1.0.7/binaries/linux-x86_64/nfqws2",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  zapret2-v1.0.7/binaries/linux-arm64/nfqws2",
		"# comment",
		"not-a-hash  skip",
		"",
	}, "\n"))
	idx := ParseSHA256SumFile(body)
	if got := idx.ArchiveSum("zapret2-v1.0.7.tar.gz"); got != strings.Repeat("a", 64) {
		t.Fatalf("ArchiveSum = %q", got)
	}
	if got := idx.BinarySum("linux-x86_64", "nfqws2"); got != strings.Repeat("b", 64) {
		t.Fatalf("BinarySum x86_64 = %q", got)
	}
	if got := idx.BinarySum("linux-arm64", BinNfqws2); got != strings.Repeat("c", 64) {
		t.Fatalf("BinarySum arm64 = %q", got)
	}
	if !idx.HasBinarySums() {
		t.Fatal("HasBinarySums")
	}
	if idx.BinarySum("linux-mipsel", "nfqws2") != "" {
		t.Fatal("unexpected mipsel sum")
	}
}

func TestParseSHA256SumFileUpstreamOnly(t *testing.T) {
	// Как у bol-van / MarkinAlexander: только per-file, без tar.gz.
	body := []byte("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  zapret2-v1.0.7-reasm-fix/binaries/linux-mipsel/nfqws2\n")
	idx := ParseSHA256SumFile(body)
	if idx.ArchiveSum("zapret2-v1.0.7-reasm-fix.tar.gz") != "" {
		t.Fatal("archive sum must be empty for upstream format")
	}
	if !idx.HasBinarySums() {
		t.Fatal("HasBinarySums")
	}
	if got := idx.BinarySum("linux-mipsel", "nfqws2"); got != strings.Repeat("d", 64) {
		t.Fatalf("BinarySum = %q", got)
	}
}

// RM11: fixture реального upstream-формата (без строки *.tar.gz).
func TestParseSHA256SumFileUpstreamFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "sha256sum_upstream.txt"))
	if err != nil {
		t.Fatal(err)
	}
	idx := ParseSHA256SumFile(body)
	if idx.ArchiveSum("anything.tar.gz") != "" {
		t.Fatal("archive sum must stay empty for upstream fixture")
	}
	if !idx.HasBinarySums() {
		t.Fatal("HasBinarySums")
	}
	if got := idx.BinarySum("linux-x86_64", BinNfqws2); got != strings.Repeat("a", 64) {
		t.Fatalf("x86_64=%q", got)
	}
	if got := idx.BinarySum("linux-mips", BinNfqws2); got != strings.Repeat("c", 64) {
		t.Fatalf("mips=%q", got)
	}
}

func TestVerifyStagedBinaries(t *testing.T) {
	dir := t.TempDir()
	arch := "linux-x86_64"
	binDir := filepath.Join(dir, "binaries", arch)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("nfqws2-body")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(binDir, BinNfqws2), payload, 0o755); err != nil {
		t.Fatal(err)
	}
	idx := SumIndex{
		"zapret2-vX/binaries/linux-x86_64/nfqws2": hexSum,
	}
	if err := verifyStagedBinaries(dir, arch, idx); err != nil {
		t.Fatal(err)
	}
	idx["zapret2-vX/binaries/linux-x86_64/nfqws2"] = strings.Repeat("0", 64)
	if err := verifyStagedBinaries(dir, arch, idx); err == nil || !strings.Contains(err.Error(), "не совпадает") {
		t.Fatalf("want mismatch, got %v", err)
	}
}
