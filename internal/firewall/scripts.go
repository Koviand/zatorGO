package firewall

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AloofLibra/zator/internal/fsutil"
)

// osReadFile — os.ReadFile (отдельная переменная для единообразия подмены).
var osReadFile = os.ReadFile

//go:embed tmpl/wssize-ramp.sh.tmpl
var wssizeHookTmpl string

//go:embed tmpl/client-scope.sh.tmpl
var clientScopeHookTmpl string

// Имена файлов custom.d (префикс задаёт порядок загрузки скриптов zapret2).
const (
	// CustomDWssizeName — тот же файл, что ставит z2r.sh.
	CustomDWssizeName = "90-z2r-wssize-ramp.sh"
	// CustomDClientScopeName — хук client scope. Номер выше, чем у wssize: при
	// iptables переход client scope вставляется в PREROUTING позже и оказывается
	// первым.
	CustomDClientScopeName = "95-zator-client-scope.sh"
)

// CustomDFlavor — подкаталог init.d zapret2 с custom.d.
type CustomDFlavor string

// Поддерживаемые варианты init.d zapret2.
const (
	CustomDSysV    CustomDFlavor = "sysv"
	CustomDOpenWrt CustomDFlavor = "openwrt"
)

// ErrUnsafeScriptValue — значение нельзя безопасно подставить в shell-шаблон.
var ErrUnsafeScriptValue = errors.New("firewall: unsafe value for shell template")

var (
	scriptPathRe  = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)
	scriptTokenRe = regexp.MustCompile(`@@[A-Z_]+@@`)
)

func checkScriptPath(name, v string) error {
	if !scriptPathRe.MatchString(v) {
		return fmt.Errorf("%w: %s=%q (absolute path of [A-Za-z0-9_./-] expected)", ErrUnsafeScriptValue, name, v)
	}
	return nil
}

func renderTemplate(tmpl string, vars map[string]string) (string, error) {
	tmpl = strings.ReplaceAll(tmpl, "\r\n", "\n")
	oldnew := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		oldnew = append(oldnew, "@@"+k+"@@", v)
	}
	out := strings.NewReplacer(oldnew...).Replace(tmpl)
	if left := scriptTokenRe.FindString(out); left != "" {
		return "", fmt.Errorf("firewall: unresolved template token %s", left)
	}
	return out, nil
}

// RenderWssizeHook возвращает custom.d-скрипт wssize_ramp для zapret2
// (определяет zapret_custom_firewall, zapret_custom_firewall_nft,
// zapret_custom_firewall_nft_flush и zapret_custom_daemons). Значения по
// умолчанию — метка, список IPv4, пути — берутся из cfg; переменные zapret2
// (Z2R_WSSIZE_*, QNUM, DESYNC_MARK ...) по-прежнему читаются во время работы.
func RenderWssizeHook(cfg WssizeConfig, zatorRoot string) (string, error) {
	cfg = cfg.withDefaults()
	root := path.Clean(filepath.ToSlash(zatorRoot))
	if err := checkScriptPath("zatorRoot", root); err != nil {
		return "", err
	}
	if !nftNameRe.MatchString(cfg.SetName) {
		return "", fmt.Errorf("%w: set name %q", ErrUnsafeScriptValue, cfg.SetName)
	}
	if err := cfg.Marks.CheckWssize(); err != nil {
		return "", err
	}
	return renderTemplate(wssizeHookTmpl, map[string]string{
		"SET":             cfg.SetName,
		"WSSIZE_MARK":     FormatMark(cfg.Marks.Wssize),
		"SUPERSWEEP_MARK": FormatMark(cfg.Marks.superSweep()),
		"ZATOR_ROOT":      root,
	})
}

// RenderClientScopeHook возвращает custom.d-скрипт client scope для iptables и
// nftables. TSV читается скриптом во время работы; cfg задаёт значения по
// умолчанию (имена, маска, сдвиг, максимум, путь файла карты).
func RenderClientScopeHook(cfg ClientScopeConfig) (string, error) {
	cfg = cfg.withDefaults()
	for _, f := range []struct {
		name, value string
		re          *regexp.Regexp
	}{
		{"iptables chain", cfg.IptablesChain, iptNameRe},
		{"iptables table", cfg.IptablesTable, iptTableRe},
		{"nft chain", cfg.NftChain, nftNameRe},
	} {
		if !f.re.MatchString(f.value) {
			return "", fmt.Errorf("%w: %s=%q", ErrUnsafeScriptValue, f.name, f.value)
		}
	}
	if !nftCommentRe.MatchString(cfg.Comment) || strings.ContainsAny(cfg.Comment, `"'$`+"`\\") {
		return "", fmt.Errorf("%w: comment=%q", ErrUnsafeScriptValue, cfg.Comment)
	}
	if err := checkScriptPath("MapFile", cfg.MapFile); err != nil {
		return "", err
	}
	if cfg.Shift > 31 {
		return "", fmt.Errorf("%w: shift=%d", ErrUnsafeScriptValue, cfg.Shift)
	}
	return renderTemplate(clientScopeHookTmpl, map[string]string{
		"IPT_CHAIN": cfg.IptablesChain,
		"IPT_TABLE": cfg.IptablesTable,
		"NFT_TABLE": ClientScopeNftTable,
		"NFT_CHAIN": cfg.NftChain,
		"COMMENT":   cfg.Comment,
		"MAP_FILE":  cfg.MapFile,
		"SHIFT":     fmt.Sprint(cfg.Shift),
		"MAX":       fmt.Sprint(cfg.Max),
		"MASK":      FormatMark(cfg.Mask),
	})
}

// CustomDDir возвращает каталог custom.d: <zapretRoot>/init.d/<flavor>/custom.d.
func CustomDDir(zapretRoot string, flavor CustomDFlavor) string {
	return filepath.Join(zapretRoot, "init.d", string(flavor), "custom.d")
}

// CustomDPath возвращает путь файла хука в custom.d.
func CustomDPath(zapretRoot string, flavor CustomDFlavor, name string) string {
	return filepath.Join(CustomDDir(zapretRoot, flavor), name)
}

// WriteCustomD атомарно записывает хук в custom.d (права 0755, недостающие
// каталоги создаются) и возвращает путь файла. name — простое имя файла.
func WriteCustomD(zapretRoot string, flavor CustomDFlavor, name, content string) (string, error) {
	if flavor != CustomDSysV && flavor != CustomDOpenWrt {
		return "", fmt.Errorf("firewall: unknown custom.d flavor %q", flavor)
	}
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("firewall: invalid custom.d file name %q", name)
	}
	p := CustomDPath(zapretRoot, flavor, name)
	if err := fsutil.WriteIfChanged(p, []byte(content), 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// RemoveCustomD удаляет хук из custom.d; отсутствие файла не ошибка.
func RemoveCustomD(zapretRoot string, flavor CustomDFlavor, name string) error {
	if name == "" || name != filepath.Base(name) {
		return fmt.Errorf("firewall: invalid custom.d file name %q", name)
	}
	err := os.Remove(CustomDPath(zapretRoot, flavor, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// FlavorForInit подбирает вариант custom.d по имени платформы
// (platform.KindOpenWRT → openwrt, остальные → sysv).
func FlavorForInit(openwrt bool) CustomDFlavor {
	if openwrt {
		return CustomDOpenWrt
	}
	return CustomDSysV
}
