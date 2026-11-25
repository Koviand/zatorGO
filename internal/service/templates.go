package service

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// Имена файлов и каталогов служб.
const (
	systemdDir      = "/etc/systemd/system"
	initDir         = "/etc/init.d"
	entwareInitDir  = "/opt/etc/init.d"
	merlinHookFile  = "/jffs/scripts/firewall-start"
	keeneticHookDir = "/opt/etc/ndm/netfilter.d"
	// KeeneticHookName — имя ndm-хука.
	KeeneticHookName = "000-zapret2.sh"

	defaultPath = "/opt/bin:/opt/sbin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// TemplateData — данные для шаблонов unit/init-файлов.
type TemplateData struct {
	Name        string   // имя службы (zapret2, zator-webui)
	Description string   // описание
	Command     []string // исполняемый файл и аргументы
	PIDFile     string   // PID-файл (SysV/Entware)
	LogFile     string   // лог stdout/stderr для SysV/Entware
	InitScript  string   // путь init-скрипта (для restart-fw)
	FWHook      string   // необязательный скрипт межсетевого экрана
	Path        string   // значение PATH
	Start       int      // порядок запуска (procd START=)
}

// ExecStart — командная строка для systemd (аргументы через пробел).
func (d TemplateData) ExecStart() string { return strings.Join(d.Command, " ") }

// ShellCommand — командная строка с экранированием для sh.
func (d TemplateData) ShellCommand() string {
	q := make([]string, len(d.Command))
	for i, a := range d.Command {
		q[i] = shellQuote(a)
	}
	return strings.Join(q, " ")
}

// Exe — первый элемент команды (исполняемый файл).
func (d TemplateData) Exe() string {
	if len(d.Command) == 0 {
		return ""
	}
	return d.Command[0]
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const systemdTemplate = `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=PATH={{.Path}}
{{- if .FWHook}}
ExecStartPre=-{{.FWHook}} start
ExecStopPost=-{{.FWHook}} stop
{{- end}}
ExecStart={{.ExecStart}}
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`

const procdTemplate = `#!/bin/sh /etc/rc.common
# {{.Description}} (создано zator)
START={{.Start}}
STOP=10
USE_PROCD=1
PATH={{.Path}}
export PATH

start_service() {
{{- if .FWHook}}
	[ -x {{shq .FWHook}} ] && {{shq .FWHook}} start
{{- end}}
	procd_open_instance
	procd_set_param command {{.ShellCommand}}
	procd_set_param respawn
	procd_set_param stdout 1
	procd_set_param stderr 1
	procd_close_instance
}

stop_service() {
{{- if .FWHook}}
	[ -x {{shq .FWHook}} ] && {{shq .FWHook}} stop
{{- end}}
	return 0
}
{{- if .FWHook}}

restart_fw() {
	[ -x {{shq .FWHook}} ] && {{shq .FWHook}} restart
}
EXTRA_COMMANDS="restart_fw"
{{- end}}
`

const sysvTemplate = `#!/bin/sh
# {{.Description}} (создано zator)
PATH={{.Path}}
export PATH
NAME={{shq .Name}}
PIDFILE={{shq .PIDFile}}
LOGFILE={{shq .LogFile}}
{{- if .FWHook}}
FWHOOK={{shq .FWHook}}
{{- end}}

is_running() {
	[ -f "$PIDFILE" ] || return 1
	pid="$(cat "$PIDFILE" 2>/dev/null)"
	[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

fw() {
{{- if .FWHook}}
	[ -x "$FWHOOK" ] && "$FWHOOK" "$1"
{{- end}}
	return 0
}

start() {
	if is_running; then
		echo "$NAME уже запущен"
		return 0
	fi
	fw start
	mkdir -p "$(dirname "$PIDFILE")"
	if [ -n "$LOGFILE" ]; then
		mkdir -p "$(dirname "$LOGFILE")"
		# Ротация: не раздувать flash (Keenetic 128–256MB).
		LOG_MAX_BYTES="${Z2R_SERVICE_LOG_MAX:-262144}"
		if [ -f "$LOGFILE" ]; then
			sz=$(wc -c <"$LOGFILE" 2>/dev/null || echo 0)
			sz=$(echo "$sz" | tr -d ' \t')
			if [ -n "$sz" ] && [ "$sz" -gt "$LOG_MAX_BYTES" ] 2>/dev/null; then
				mv -f "$LOGFILE" "$LOGFILE.old" 2>/dev/null || true
			fi
		fi
		: >>"$LOGFILE"
	else
		LOGFILE=/dev/null
	fi
	# Keenetic/Entware: не глотать stderr — иначе webui падает молча, а install врёт «запущен».
	if command -v nohup >/dev/null 2>&1; then
		nohup {{.ShellCommand}} >>"$LOGFILE" 2>&1 &
	else
		{{.ShellCommand}} >>"$LOGFILE" 2>&1 &
	fi
	echo $! > "$PIDFILE"
	sleep 1
	if ! is_running; then
		echo "$NAME не запустился (лог: $LOGFILE)" >&2
		rm -f "$PIDFILE"
		return 1
	fi
	echo "$NAME запущен"
}

stop() {
	if is_running; then
		kill "$(cat "$PIDFILE")" 2>/dev/null
		sleep 1
		if is_running; then
			kill -9 "$(cat "$PIDFILE")" 2>/dev/null
		fi
	fi
	rm -f "$PIDFILE"
	fw stop
	echo "$NAME остановлен"
}

case "$1" in
	start) start ;;
	stop) stop ;;
	restart) stop; sleep 1; start ;;
	restart-fw) fw restart ;;
	status)
		if is_running; then echo "$NAME работает"; else echo "$NAME остановлен"; exit 3; fi
		;;
	*) echo "usage: $0 {start|stop|restart|restart-fw|status}"; exit 1 ;;
esac
`

const keeneticHookTemplate = `#!/bin/sh
# ndm пересобирает firewall при сетевых событиях: возвращаем правила zapret2.
# Debounce + flock: иначе mangle и nat дают два параллельных restart-fw
# (окно без NFQUEUE + гонки iptables без -w).
[ "$table" != "mangle" ] && [ "$table" != "nat" ] && exit 0
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin:$PATH
export PATH
LOCK=/var/run/zator-fw.lock
(
  flock -n 9 || exit 0
  sleep 2
  {{shq .InitScript}} restart-fw
) 9>"$LOCK" &
exit 0
`

var tmplFuncs = template.FuncMap{"shq": shellQuote}

var (
	systemdTmpl      = template.Must(template.New("systemd").Funcs(tmplFuncs).Parse(systemdTemplate))
	procdTmpl        = template.Must(template.New("procd").Funcs(tmplFuncs).Parse(procdTemplate))
	sysvTmpl         = template.Must(template.New("sysv").Funcs(tmplFuncs).Parse(sysvTemplate))
	keeneticHookTmpl = template.Must(template.New("keenetic").Funcs(tmplFuncs).Parse(keeneticHookTemplate))
)

func render(t *template.Template, d TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("service: шаблон %s: %w", t.Name(), err)
	}
	return buf.String(), nil
}

// RenderSystemd формирует содержимое systemd unit.
func RenderSystemd(d TemplateData) (string, error) { return render(systemdTmpl, d) }

// RenderProcd формирует init-скрипт procd (OpenWrt).
func RenderProcd(d TemplateData) (string, error) { return render(procdTmpl, d) }

// RenderSysV формирует init-скрипт SysV/Entware.
func RenderSysV(d TemplateData) (string, error) { return render(sysvTmpl, d) }

// RenderKeeneticHook формирует ndm-хук netfilter.d.
func RenderKeeneticHook(d TemplateData) (string, error) { return render(keeneticHookTmpl, d) }

// MerlinHookLine — строка для /jffs/scripts/firewall-start.
func MerlinHookLine(initScript string) string {
	return shellQuote(filepath.ToSlash(initScript)) + " restart-fw"
}

// AddHookLine добавляет line в содержимое скрипта, если её там ещё нет
// (сравнение построчно, как grep -qxF). Пустое содержимое получает shebang.
func AddHookLine(content, line string) string {
	if content == "" {
		content = "#!/bin/sh\n"
	}
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimRight(l, "\r") == line {
			return content
		}
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + line + "\n"
}

// RemoveHookLine удаляет все вхождения line (целой строкой).
func RemoveHookLine(content, line string) string {
	lines := strings.Split(content, "\n")
	out := lines[:0:0]
	for _, l := range lines {
		if strings.TrimRight(l, "\r") != line {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// templateData собирает данные шаблона для службы.
func (m *manager) templateData(u Unit) TemplateData {
	d := TemplateData{
		Name:   string(u),
		Path:   defaultPath,
		FWHook: slash(m.cfg.FWHook),
	}
	switch u {
	case UnitZapret2:
		d.Description = "zapret2 (nfqws2)"
		d.Command = []string{slash(filepath.Join(m.cfg.Paths.ZapretRoot, "nfq2", "nfqws2")), "@" + slash(m.cfg.ArgsFile)}
		d.Start = 90
	case UnitValidator:
		d.Description = "zator strategy-validator (circular_quality watchdog)"
		script := slash(filepath.Join(m.cfg.Paths.LuaDir, "strategy-validator.sh"))
		d.Command = []string{script, "--daemon"}
		d.Start = 91
		d.FWHook = ""
	case UnitWebUI:
		d.Description = "zator Web UI"
		d.Command = []string{slash(m.cfg.ZatorBin), "webui"}
		d.Start = 95
		d.FWHook = "" // межсетевой экран относится только к zapret2
	}
	runDir := "/var/run"
	if m.cfg.Backend == BackendEntware {
		runDir = "/opt/var/run"
	}
	d.PIDFile = runDir + "/" + string(u) + ".pid"
	if m.cfg.Paths.StateDir != "" {
		d.LogFile = slash(filepath.Join(m.cfg.Paths.StateDir, string(u)+".log"))
	}
	return d
}

func slash(p string) string { return filepath.ToSlash(p) }
