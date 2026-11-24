package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchFromGOARCH(t *testing.T) {
	cases := map[string]Arch{
		"arm": ArchARM, "arm64": ArchARM64, "mips": ArchMIPS, "mipsle": ArchMIPSEL,
		"mips64": ArchMIPS64, "mips64le": ArchMIPSEL64, "ppc64": ArchPPC,
		"386": ArchX86, "amd64": ArchX86_64, "riscv64": ArchRISCV64, "s390x": ArchUnknown,
	}
	for in, want := range cases {
		if got := ArchFromGOARCH(in); got != want {
			t.Errorf("ArchFromGOARCH(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectVPS(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", "ID=debian\nNAME=\"Debian GNU/Linux\"\n")
	writeFile(t, root, "run/systemd/system/.keep", "")
	writeFile(t, root, "usr/sbin/nft", "")
	p := Detector{Root: root, GOARCH: "amd64"}.Detect()
	if p.Kind != KindVPS || p.Hardware != HardwareNone || p.Arch != ArchX86_64 ||
		p.InitSystem != InitSystemd || p.FWType != FWNftables {
		t.Fatalf("неожиданная платформа: %+v", p)
	}
}

func TestDetectOpenWRT(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", "ID=\"openwrt\"\n")
	writeFile(t, root, "sbin/procd", "")
	writeFile(t, root, "sbin/fw4", "")
	p := Detector{Root: root, GOARCH: "mipsle"}.Detect()
	if p.Kind != KindOpenWRT || p.Arch != ArchMIPSEL || p.InitSystem != InitProcd || p.FWType != FWNftables {
		t.Fatalf("неожиданная платформа: %+v", p)
	}
}

func TestDetectEntwareKeenetic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "opt/etc/entware_release", "arch=mipsel\n")
	writeFile(t, root, "etc/ndm/.keep", "")
	p := Detector{Root: root, GOARCH: "mipsle"}.Detect()
	if p.Kind != KindEntware || p.Hardware != HardwareKeenetic || p.InitSystem != InitEntware {
		t.Fatalf("неожиданная платформа: %+v", p)
	}
}

func TestDetectEntwareKeeneticOptNDM(t *testing.T) {
	// Реальный Keenetic Entware: /opt/etc/ndm есть, /etc/ndm нет, os-release — Entware.
	root := t.TempDir()
	writeFile(t, root, "opt/etc/entware_release", "ID=entware\nNAME=Entware\n")
	writeFile(t, root, "opt/etc/ndm/netfilter.d/.keep", "")
	writeFile(t, root, "proc/sys/kernel/osrelease", "4.9-ndm-5\n")
	writeFile(t, root, "proc/version", "Linux version 4.9-ndm-5 (developers@keenetic.com)\n")
	p := Detector{Root: root, GOARCH: "arm64"}.Detect()
	if p.Kind != KindEntware || p.Hardware != HardwareKeenetic {
		t.Fatalf("ожидался Entware/keenetic: %+v", p)
	}
}

func TestDetectEntwareMerlin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "opt/etc/entware_release", "")
	writeFile(t, root, "jffs/.keep", "")
	p := Detector{Root: root, GOARCH: "arm64"}.Detect()
	if p.Kind != KindEntware || p.Hardware != HardwareMerlin || p.Arch != ArchARM64 {
		t.Fatalf("неожиданная платформа: %+v", p)
	}
}

func TestDetectNetcraze(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "opt/etc/entware_release", "")
	writeFile(t, root, "etc/os-release", "NAME=Netcraze\n")
	p := Detector{Root: root, GOARCH: "arm64"}.Detect()
	if p.Hardware != HardwareNetcraze {
		t.Fatalf("ожидался netcraze: %+v", p)
	}
}

func TestDetectLexra(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/openwrt_release", "")
	writeFile(t, root, "proc/cpuinfo", "cpu model : Lexra RLX5281\n")
	p := Detector{Root: root, GOARCH: "mips"}.Detect()
	if p.Arch != ArchLexra {
		t.Fatalf("ожидался lexra: %+v", p)
	}
}

func TestReleaseArchAndBinaryAssetName(t *testing.T) {
	// Lexra: сохраняем endianness текущего бинарника; на amd64/Windows-хосте
	// SelfArch ≠ mips/mipsel → канон ArchMIPS (BE, как upstream linux-lexra).
	gotLex := ReleaseArch(ArchLexra)
	self := SelfArch()
	wantLex := ArchMIPS
	if self == ArchMIPS || self == ArchMIPSEL {
		wantLex = self
	}
	if gotLex != wantLex {
		t.Fatalf("lexra → %q, want %q (self=%q)", gotLex, wantLex, self)
	}
	if got := ReleaseArch(ArchARM64); got != ArchARM64 {
		t.Fatalf("arm64 → %q", got)
	}
	wantAsset := "zator-linux-" + string(wantLex)
	if got := BinaryAssetName(ArchLexra); got != wantAsset {
		t.Fatalf("BinaryAssetName(lexra) = %q, want %q", got, wantAsset)
	}
	if got := BinaryAssetName(ArchRISCV64); got != "zator-linux-riscv64" {
		t.Fatalf("BinaryAssetName(riscv64) = %q", got)
	}
	released := ReleasedArches()
	if len(released) < 10 {
		t.Fatalf("ReleasedArches too short: %v", released)
	}
	for _, a := range released {
		if a == ArchLexra {
			t.Fatal("Lexra must not be in ReleasedArches")
		}
		if BinaryAssetName(a) == "" {
			t.Fatalf("empty asset for %q", a)
		}
	}
}

func TestDetectRealDoesNotPanic(t *testing.T) {
	p := Detect()
	if p.Kind == "" || p.InitSystem == "" || p.FWType == "" {
		t.Fatalf("неполный результат: %+v", p)
	}
	_ = p.String()
}
