package firewall

import (
	"path/filepath"
	"strings"

	"github.com/AloofLibra/zator/internal/fsutil"
)

// QueueHookName — имя скрипта NFQUEUE в ZapretRoot.
const QueueHookName = "zator-nfqueue.sh"

// QueueHookConfig — параметры хука межсетевого экрана для nfqws2.
type QueueHookConfig struct {
	QNum     string // номер очереди (по умолчанию 200)
	PortsTCP string // multiport TCP
	PortsUDP string // multiport UDP (без диапазонов вида a-b для простоты — режем)
	Mark     string // DESYNC_MARK, по умолчанию 0x40000000
}

// DefaultQueueHook заполняет поля по умолчанию.
func DefaultQueueHook() QueueHookConfig {
	return QueueHookConfig{
		QNum:     "200",
		PortsTCP: "80,443,2053,2083,2087,2096,8443",
		PortsUDP: "443,2408,2053,2083,2087,2096,8443",
		Mark:     "0x40000000",
	}
}

// RenderQueueHook возвращает POSIX sh-скрипт start|stop|restart для iptables NFQUEUE.
func RenderQueueHook(cfg QueueHookConfig) string {
	if cfg.QNum == "" {
		cfg.QNum = "200"
	}
	if cfg.Mark == "" {
		cfg.Mark = "0x40000000"
	}
	tcp := sanitizePorts(cfg.PortsTCP)
	udp := sanitizePorts(cfg.PortsUDP)
	var b strings.Builder
	b.WriteString(`#!/bin/sh
# zator: NFQUEUE → nfqws2 (создано автоматически)
QNUM=`)
	b.WriteString(shellQuote(cfg.QNum))
	b.WriteString(`
MARK=`)
	b.WriteString(shellQuote(cfg.Mark))
	b.WriteString(`
TCP_PORTS=`)
	b.WriteString(shellQuote(tcp))
	b.WriteString(`
UDP_PORTS=`)
	b.WriteString(shellQuote(udp))
	b.WriteString(`
CHAIN=ZATOR_NFQ

have_ipt() { command -v iptables >/dev/null 2>&1; }

stop_rules() {
	have_ipt || return 0
	iptables -t mangle -D POSTROUTING -j "$CHAIN" 2>/dev/null
	iptables -t mangle -D PREROUTING -j "$CHAIN" 2>/dev/null
	iptables -t mangle -F "$CHAIN" 2>/dev/null
	iptables -t mangle -X "$CHAIN" 2>/dev/null
	return 0
}

start_rules() {
	have_ipt || return 0
	stop_rules
	iptables -t mangle -N "$CHAIN" 2>/dev/null || iptables -t mangle -F "$CHAIN"
	if [ -n "$TCP_PORTS" ]; then
		iptables -t mangle -A "$CHAIN" -p tcp -m multiport --dports "$TCP_PORTS" \
			-m mark ! --mark "$MARK/$MARK" \
			-j NFQUEUE --queue-num "$QNUM" --queue-bypass 2>/dev/null || true
		iptables -t mangle -A "$CHAIN" -p tcp -m multiport --sports "$TCP_PORTS" \
			-m mark ! --mark "$MARK/$MARK" \
			-j NFQUEUE --queue-num "$QNUM" --queue-bypass 2>/dev/null || true
	fi
	if [ -n "$UDP_PORTS" ]; then
		iptables -t mangle -A "$CHAIN" -p udp -m multiport --dports "$UDP_PORTS" \
			-m mark ! --mark "$MARK/$MARK" \
			-j NFQUEUE --queue-num "$QNUM" --queue-bypass 2>/dev/null || true
		iptables -t mangle -A "$CHAIN" -p udp -m multiport --sports "$UDP_PORTS" \
			-m mark ! --mark "$MARK/$MARK" \
			-j NFQUEUE --queue-num "$QNUM" --queue-bypass 2>/dev/null || true
	fi
	iptables -t mangle -I POSTROUTING 1 -j "$CHAIN" 2>/dev/null || \
		iptables -t mangle -A POSTROUTING -j "$CHAIN"
	iptables -t mangle -I PREROUTING 1 -j "$CHAIN" 2>/dev/null || \
		iptables -t mangle -A PREROUTING -j "$CHAIN"
}

case "$1" in
	start) start_rules ;;
	stop) stop_rules ;;
	restart) stop_rules; start_rules ;;
	*) echo "usage: $0 {start|stop|restart}"; exit 1 ;;
esac
exit 0
`)
	return b.String()
}

func sanitizePorts(s string) string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// multiport не любит слишком длинные списки и сложные диапазоны —
		// оставляем одиночные порты и простые a:b → a-b уже в конфиге как a-b.
		if strings.Count(p, "-") > 1 {
			continue
		}
		out = append(out, p)
		if len(out) >= 15 { // лимит multiport
			break
		}
	}
	return strings.Join(out, ",")
}

// WriteQueueHook атомарно пишет скрипт в zapretRoot/QueueHookName (только если изменился).
func WriteQueueHook(zapretRoot string, cfg QueueHookConfig) (string, error) {
	path := filepath.Join(zapretRoot, QueueHookName)
	if err := fsutil.WriteIfChanged(path, []byte(RenderQueueHook(cfg)), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// QueueHookFromGetter читает порты/qnum/mark из config-подобного Getter.
func QueueHookFromGetter(get func(key string) string) QueueHookConfig {
	cfg := DefaultQueueHook()
	if v := strings.TrimSpace(get("QNUM")); v != "" {
		cfg.QNum = v
	}
	if v := strings.TrimSpace(get("NFQWS2_PORTS_TCP")); v != "" {
		cfg.PortsTCP = v
	}
	if v := strings.TrimSpace(get("NFQWS2_PORTS_UDP")); v != "" {
		cfg.PortsUDP = v
	}
	if v := strings.TrimSpace(get("DESYNC_MARK")); v != "" {
		cfg.Mark = v
	}
	if cfg.QNum == "" {
		cfg.QNum = "200"
	}
	return cfg
}
