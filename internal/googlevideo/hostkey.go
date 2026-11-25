package googlevideo

import (
	"regexp"
	"strings"
)

// Канон media-CDN YouTube (узлы, на которых рвётся videoplayback):
//
//	^rr\d+---sn-[a-z0-9-]+\.googlevideo\.com$
//
// Пример: rr18---sn-pmcg-bg0s.googlevideo.com
var (
	reRRMediaHost   = regexp.MustCompile(`(?i)^rr\d+---sn-[a-z0-9-]+\.googlevideo\.com$`)
	reRRGooglevideo = regexp.MustCompile(`(?i)^rr\d+---sn-([a-z0-9-]+)\.googlevideo\.com$`)
	reSNGooglevideo = regexp.MustCompile(`(?i)^sn-([a-z0-9-]+)\.googlevideo\.com$`)
)

// IsRRMediaHost — FQDN media-CDN YouTube (rrN---sn-<id>.googlevideo.com).
func IsRRMediaHost(hostname string) bool {
	h := strings.TrimSpace(strings.ToLower(hostname))
	h = strings.Trim(h, ".")
	return h != "" && reRRMediaHost.MatchString(h)
}

var reBareSNKey = regexp.MustCompile(`(?i)^sn-[a-z0-9-]+$`)

// SNIHostKey нормализует media-узел googlevideo к ключу sn-<id>
// (общий для rr1/rr2/rr5 одного POP). Совпадает с slm_googlevideo_hostkey в Lua.
// Не-media хосты (redirector и т.п.) возвращают ("", false).
func SNIHostKey(hostname string) (string, bool) {
	h := strings.TrimSpace(strings.ToLower(hostname))
	h = strings.Trim(h, ".")
	if h == "" {
		return "", false
	}
	if m := reRRGooglevideo.FindStringSubmatch(h); len(m) == 2 && m[1] != "" {
		return "sn-" + m[1], true
	}
	if m := reSNGooglevideo.FindStringSubmatch(h); len(m) == 2 && m[1] != "" {
		return "sn-" + m[1], true
	}
	if reBareSNKey.MatchString(h) {
		return h, true
	}
	return "", false
}

// SyntheticRRHost строит канонический rr1---sn-….googlevideo.com из ключа
// auto_locked (sn-…) или уже полного FQDN. Нужен для googlevideo-hosts.txt.
func SyntheticRRHost(hostOrKey string) (string, bool) {
	h := strings.TrimSpace(strings.ToLower(hostOrKey))
	h = strings.Trim(h, ".")
	if h == "" {
		return "", false
	}
	if IsRRMediaHost(h) {
		return h, true
	}
	key, ok := SNIHostKey(h)
	if !ok || key == "" {
		return "", false
	}
	fqdn := "rr1---" + key + ".googlevideo.com"
	if !IsRRMediaHost(fqdn) {
		return "", false
	}
	return fqdn, true
}
