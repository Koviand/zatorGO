// Package platform определяет тип устройства, архитектуру и init-систему.
package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Kind — тип окружения.
type Kind string

const (
	KindVPS     Kind = "VPS"
	KindOpenWRT Kind = "OpenWRT"
	KindEntware Kind = "Entware"
)

// Hardware — производитель/семейство роутера (пустая строка — не роутер/неизвестно).
type Hardware string

const (
	HardwareNone     Hardware = ""
	HardwareKeenetic Hardware = "keenetic"
	HardwareMerlin   Hardware = "merlin"
	HardwareNetcraze Hardware = "netcraze"
)

// Arch — имя архитектуры в терминах zator/zapret2.
type Arch string

const (
	ArchARM      Arch = "arm"
	ArchARM64    Arch = "arm64"
	ArchMIPS     Arch = "mips"
	ArchMIPS64   Arch = "mips64"
	ArchMIPSEL   Arch = "mipsel"
	ArchMIPSEL64 Arch = "mipsel64"
	ArchPPC      Arch = "ppc"
	ArchX86      Arch = "x86"
	ArchX86_64   Arch = "x86_64"
	ArchRISCV64  Arch = "riscv64"
	ArchLexra    Arch = "lexra"
	ArchUnknown  Arch = ""
)

// AllArches возвращает все известные архитектуры платформы (включая Lexra).
func AllArches() []Arch {
	return []Arch{
		ArchARM, ArchARM64, ArchMIPS, ArchMIPS64, ArchMIPSEL, ArchMIPSEL64,
		ArchPPC, ArchX86, ArchX86_64, ArchRISCV64, ArchLexra,
	}
}

// ReleasedArches — архитектуры, для которых публикуется zator-linux-<arch>.
// Lexra отдельного Go-таргета не имеет (см. ReleaseArch).
func ReleasedArches() []Arch {
	return []Arch{
		ArchARM, ArchARM64, ArchMIPS, ArchMIPS64, ArchMIPSEL, ArchMIPSEL64,
		ArchPPC, ArchX86, ArchX86_64, ArchRISCV64,
	}
}

// ReleaseArch возвращает имя ассета релиза для архитектуры устройства.
// Lexra (RLX): отдельного GOARCH нет. Upstream zapret собирает linux-lexra
// big-endian (EB); zator-ассет должен совпадать с endianness уже
// установленного бинарника (SelfArch), иначе update подменит BE→LE.
// Если SelfArch неизвестен — канон ArchMIPS (BE softfloat).
func ReleaseArch(a Arch) Arch {
	switch a {
	case ArchLexra:
		if self := SelfArch(); self == ArchMIPS || self == ArchMIPSEL {
			return self
		}
		return ArchMIPS
	case ArchUnknown:
		return ArchUnknown
	default:
		return a
	}
}

// BinaryAssetName — имя файла релиза zator-linux-<arch> для данной arch.
func BinaryAssetName(a Arch) string {
	a = ReleaseArch(a)
	if a == ArchUnknown {
		return ""
	}
	return "zator-linux-" + string(a)
}

// InitSystem — система инициализации.
type InitSystem string

const (
	InitSystemd InitSystem = "systemd"
	InitProcd   InitSystem = "procd"
	InitSysV    InitSystem = "sysv"
	InitEntware InitSystem = "entware"
)

// FWType — тип межсетевого экрана.
type FWType string

const (
	FWIptables FWType = "iptables"
	FWNftables FWType = "nftables"
)

// Platform — результат определения платформы.
type Platform struct {
	Kind       Kind
	Hardware   Hardware
	Arch       Arch
	InitSystem InitSystem
	FWType     FWType
	OSID       string // значение ID из /etc/os-release
}

// String возвращает краткое описание платформы.
func (p Platform) String() string {
	hw := string(p.Hardware)
	if hw == "" {
		hw = "-"
	}
	return string(p.Kind) + "/" + hw + "/" + string(p.Arch) + "/" + string(p.InitSystem) + "/" + string(p.FWType)
}

// Detector читает файлы платформы относительно Root (для тестов; в продакшене "/").
type Detector struct {
	Root   string
	GOARCH string
}

// Detect определяет платформу текущей системы.
func Detect() Platform {
	return Detector{Root: "/", GOARCH: runtime.GOARCH}.Detect()
}

// ArchFromGOARCH сопоставляет runtime.GOARCH с именем архитектуры zator.
// Для неподдерживаемых значений возвращает ArchUnknown.
func ArchFromGOARCH(goarch string) Arch {
	switch goarch {
	case "arm":
		return ArchARM
	case "arm64":
		return ArchARM64
	case "mips":
		return ArchMIPS
	case "mipsle":
		return ArchMIPSEL
	case "mips64":
		return ArchMIPS64
	case "mips64le":
		return ArchMIPSEL64
	case "ppc64", "ppc64le", "ppc":
		return ArchPPC
	case "386":
		return ArchX86
	case "amd64":
		return ArchX86_64
	case "riscv64":
		return ArchRISCV64
	}
	return ArchUnknown
}

// SelfArch возвращает архитектуру текущего бинарника по runtime.GOARCH.
func SelfArch() Arch { return ArchFromGOARCH(runtime.GOARCH) }

// Detect определяет платформу.
func (d Detector) Detect() Platform {
	if d.Root == "" {
		d.Root = "/"
	}
	goarch := d.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	osRelease := parseKV(d.read("etc/os-release"))
	cpuinfo := strings.ToLower(d.read("proc/cpuinfo"))

	p := Platform{OSID: strings.ToLower(osRelease["ID"])}
	p.Kind = d.detectKind(osRelease)
	p.Hardware = d.detectHardware(p.Kind, osRelease, cpuinfo)
	p.Arch = detectArch(goarch, cpuinfo)
	p.InitSystem = d.detectInit(p.Kind)
	p.FWType = d.detectFW(p.Kind)
	return p
}

func (d Detector) detectKind(osRelease map[string]string) Kind {
	id := strings.ToLower(osRelease["ID"])
	idLike := strings.ToLower(osRelease["ID_LIKE"])
	if id == "openwrt" || strings.Contains(idLike, "openwrt") ||
		d.exists("etc/openwrt_release") || d.exists("etc/openwrt_version") {
		return KindOpenWRT
	}
	if d.exists("opt/etc/entware_release") || (d.exists("opt/bin/opkg") && d.exists("jffs")) {
		return KindEntware
	}
	return KindVPS
}

func (d Detector) detectHardware(kind Kind, osRelease map[string]string, cpuinfo string) Hardware {
	if kind == KindVPS {
		return HardwareNone
	}
	osrelease := strings.ToLower(d.read("proc/sys/kernel/osrelease"))
	procVer := strings.ToLower(d.read("proc/version"))
	joined := strings.ToLower(osRelease["ID"] + " " + osRelease["NAME"] + " " +
		osRelease["PRETTY_NAME"] + " " + osRelease["HOME_URL"] + " " + cpuinfo + " " +
		osrelease + " " + procVer)
	switch {
	case strings.Contains(joined, "netcraze"):
		return HardwareNetcraze
	// Keenetic Entware: NDM живёт в /opt/etc/ndm (не /etc/ndm), ядро вида 4.9-ndm-*.
	case strings.Contains(joined, "keenetic") ||
		d.exists("etc/ndm") || d.exists("tmp/ndm") || d.exists("opt/etc/ndm") ||
		strings.Contains(osrelease, "-ndm"):
		return HardwareKeenetic
	case strings.Contains(joined, "merlin") || strings.Contains(joined, "asuswrt") || d.exists("jffs"):
		return HardwareMerlin
	}
	return HardwareNone
}

func detectArch(goarch, cpuinfo string) Arch {
	arch := ArchFromGOARCH(goarch)
	// Lexra (RLX) — MIPS-подобные ядра без части инструкций; в Go выглядят как mips/mipsle.
	if (arch == ArchMIPS || arch == ArchMIPSEL) &&
		(strings.Contains(cpuinfo, "lexra") || strings.Contains(cpuinfo, "rlx")) {
		return ArchLexra
	}
	return arch
}

func (d Detector) detectInit(kind Kind) InitSystem {
	switch {
	case d.exists("run/systemd/system"):
		return InitSystemd
	case kind == KindOpenWRT && (d.exists("sbin/procd") || d.exists("etc/rc.common")):
		return InitProcd
	case kind == KindEntware:
		return InitEntware
	case d.exists("sbin/procd"):
		return InitProcd
	}
	return InitSysV
}

func (d Detector) detectFW(kind Kind) FWType {
	switch kind {
	case KindOpenWRT:
		if d.exists("sbin/fw4") || d.exists("usr/sbin/nft") {
			return FWNftables
		}
		return FWIptables
	case KindEntware:
		return FWIptables
	}
	if d.exists("usr/sbin/nft") || d.exists("sbin/nft") {
		if !d.exists("usr/sbin/iptables") && !d.exists("sbin/iptables") {
			return FWNftables
		}
	}
	if d.exists("usr/sbin/iptables") || d.exists("sbin/iptables") {
		return FWIptables
	}
	if d.exists("usr/sbin/nft") || d.exists("sbin/nft") {
		return FWNftables
	}
	return FWIptables
}

func (d Detector) path(rel string) string {
	return filepath.Join(d.Root, filepath.FromSlash(rel))
}

func (d Detector) exists(rel string) bool {
	_, err := os.Stat(d.path(rel))
	return err == nil
}

func (d Detector) read(rel string) string {
	b, err := os.ReadFile(d.path(rel))
	if err != nil {
		return ""
	}
	return string(b)
}

// parseKV разбирает файл формата KEY=VALUE (os-release), снимая кавычки.
func parseKV(content string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[strings.TrimSpace(k)] = v
	}
	return out
}
