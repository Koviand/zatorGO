package webapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	domainImportMaxBytes = 2 << 20
	domainImportTimeout  = 20 * time.Second
	domainImportMaxRedir = 3
)

// blockedImportHosts — имена, которые нельзя тянуть с роутера (SSRF).
var blockedImportHosts = map[string]struct{}{
	"localhost":               {},
	"localhost.localdomain":   {},
	"metadata":                {},
	"metadata.google.internal": {},
	"metadata.goog":           {},
}

func validatePublicHTTPURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("пустой URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("некорректный URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("нужен http(s) URL")
	}
	if u.User != nil {
		return nil, fmt.Errorf("URL с учётными данными запрещён")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return nil, fmt.Errorf("URL без хоста")
	}
	if _, bad := blockedImportHosts[host]; bad {
		return nil, fmt.Errorf("хост %s запрещён", host)
	}
	if strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, fmt.Errorf("локальный хост запрещён")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedImportIP(ip) {
			return nil, fmt.Errorf("внутренний IP запрещён")
		}
		return u, nil
	}
	// DNS: все ответы должны быть публичными (иначе SSRF через приватный A-запись).
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("не удалось разрешить хост: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("хост не резолвится")
	}
	for _, ip := range ips {
		if blockedImportIP(ip) {
			return nil, fmt.Errorf("хост резолвится во внутренний адрес")
		}
	}
	return u, nil
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

func domainImportHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 0}
	tr := &http.Transport{
		Proxy:                 nil, // не ходим через env-proxy во внутреннюю сеть
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ia := range ips {
				if blockedImportIP(ia.IP) {
					last = fmt.Errorf("внутренний IP запрещён")
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ia.IP.String(), port))
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
			if _, err := validatePublicHTTPURL(req.URL.String()); err != nil {
				return fmt.Errorf("редирект запрещён: %w", err)
			}
			return nil
		},
	}
}
