package webapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	domainImportMaxBytes = 2 << 20
	domainImportTimeout  = 20 * time.Second
	domainImportMaxRedir = 3
)

// lookupIPFn — DNS для validatePublicHTTPURL (подмена в тестах pin/rebinding).
var lookupIPFn = net.LookupIP

// blockedImportHosts — имена, которые нельзя тянуть с роутера (SSRF).
var blockedImportHosts = map[string]struct{}{
	"localhost":                {},
	"localhost.localdomain":    {},
	"metadata":                 {},
	"metadata.google.internal": {},
	"metadata.goog":            {},
}

// validatePublicHTTPURL проверяет URL и возвращает закреплённые публичные IP
// первого resolve (защита от DNS rebinding при Dial).
func validatePublicHTTPURL(raw string) (*url.URL, []net.IP, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, fmt.Errorf("пустой URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("некорректный URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, nil, fmt.Errorf("нужен http(s) URL")
	}
	if u.User != nil {
		return nil, nil, fmt.Errorf("URL с учётными данными запрещён")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return nil, nil, fmt.Errorf("URL без хоста")
	}
	if _, bad := blockedImportHosts[host]; bad {
		return nil, nil, fmt.Errorf("хост %s запрещён", host)
	}
	if strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, nil, fmt.Errorf("локальный хост запрещён")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedImportIP(ip) {
			return nil, nil, fmt.Errorf("внутренний IP запрещён")
		}
		return u, []net.IP{ip}, nil
	}
	// DNS: все ответы должны быть публичными; пиним этот набор для Dial.
	ips, err := lookupIPFn(host)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось разрешить хост: %w", err)
	}
	if len(ips) == 0 {
		return nil, nil, fmt.Errorf("хост не резолвится")
	}
	pinned := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if blockedImportIP(ip) {
			return nil, nil, fmt.Errorf("хост резолвится во внутренний адрес")
		}
		pinned = append(pinned, ip)
	}
	return u, pinned, nil
}

func blockedImportIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64/10 (не входит в net.IP.IsPrivate).
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// domainImportHTTPClient — HTTP-клиент с Dial только по уже проверенным IP
// (без повторного Lookup, который мог бы уехать в private при rebinding).
func domainImportHTTPClient(initialPins []net.IP) *http.Client {
	return domainImportHTTPClientDial(initialPins, nil)
}

func domainImportHTTPClientDial(initialPins []net.IP, dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 0}
	if dial == nil {
		dial = dialer.DialContext
	}
	var mu sync.Mutex
	pins := append([]net.IP(nil), initialPins...)

	tr := &http.Transport{
		Proxy:                 nil, // не ходим через env-proxy во внутреннюю сеть
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			mu.Lock()
			cur := append([]net.IP(nil), pins...)
			mu.Unlock()
			if len(cur) == 0 {
				return nil, fmt.Errorf("нет закреплённых адресов")
			}
			var last error
			for _, ip := range cur {
				if blockedImportIP(ip) {
					last = fmt.Errorf("внутренний IP запрещён")
					continue
				}
				conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			if last == nil {
				last = fmt.Errorf("нет публичных адресов")
			}
			return nil, last
		},
	}
	return &http.Client{
		Timeout:   domainImportTimeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= domainImportMaxRedir {
				return fmt.Errorf("слишком много редиректов")
			}
			_, newPins, err := validatePublicHTTPURL(req.URL.String())
			if err != nil {
				return fmt.Errorf("редирект запрещён: %w", err)
			}
			mu.Lock()
			pins = newPins
			mu.Unlock()
			return nil
		},
	}
}
