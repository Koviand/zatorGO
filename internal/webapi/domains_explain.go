package webapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/domains"
)

var googlevideoHostRe = regexp.MustCompile(`(?i)(^|\.)googlevideo\.com$`)

// explainDomain — BP7: какой list/kind/role матчит домен (или none).
func (s *Server) explainDomain(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errf(http.StatusBadRequest, "Укажите domain=")
	}
	norm, err := domains.Normalize(domains.KindCustom, raw)
	if err != nil {
		norm = strings.ToLower(raw)
	}
	matches := []map[string]any{}

	addExact := func(list, kind, role string) {
		matches = append(matches, map[string]any{
			"list": list, "kind": kind, "match": "exact", "role": role,
		})
	}
	addSub := func(list, kind, role, entry string) {
		matches = append(matches, map[string]any{
			"list": list, "kind": kind, "match": "substring", "role": role, "entry": entry,
		})
	}

	// Manager-списки (CRUD).
	if ok, err := s.domains.Contains(domains.KindNetrogat, norm); err == nil && ok {
		addExact("netrogat", string(domains.KindNetrogat), "exclude")
	}
	if ok, err := s.domains.Contains(domains.KindCustom, norm); err == nil && ok {
		addExact("custom_rkn", string(domains.KindCustom), "hostlist")
	}
	for _, kind := range []domains.Kind{domains.KindNetrogatSubstrings, domains.KindRKNSubstrings} {
		listName := "netrogat_substrings"
		role := "exclude_substring"
		if kind == domains.KindRKNSubstrings {
			listName = "substring"
			role = "route_substring"
		}
		entries, err := s.domains.List(kind)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e != "" && strings.Contains(norm, strings.ToLower(e)) {
				addSub(listName, string(kind), role, e)
				break
			}
		}
	}

	// Файловые hostlist'ы extra_strats (вне CRUD Manager).
	extra := s.paths.ExtraStratsDir
	if hostlistFileContains(filepath.Join(extra, "TCP_RKN_list.txt"), norm) {
		addExact("rkn_list", "rkn_list", "hostlist")
	}
	if hostlistFileContains(filepath.Join(extra, "TCP_Discord.txt"), norm) {
		matches = append(matches, map[string]any{
			"list": "discord", "kind": "discord", "match": "exact", "role": "hostlist",
			"note": "профиль Discord; также hostlist-exclude для RKN",
		})
	}
	if hostlistFileContains(filepath.Join(extra, "TCP_YT_list.txt"), norm) {
		addExact("yt_list", "yt_list", "hostlist")
	}

	// Autohostlist (zapret2 ipset), если файл есть.
	autoPath := filepath.Join(s.paths.ZapretRoot, "ipset", "zapret-hosts-auto.txt")
	if hostlistFileContains(autoPath, norm) {
		addExact("autohostlist", "autohostlist", "autohostlist")
	}

	if googlevideoHostRe.MatchString(norm) {
		matches = append(matches, map[string]any{
			"list": "googlevideo", "kind": "googlevideo", "match": "pattern", "role": "hostlist",
		})
	} else if hostlistFileContains(s.paths.GooglevideoHostsFile(), norm) {
		addExact("googlevideo", "googlevideo", "hostlist")
	}

	cfg, _ := s.readConfig()
	out := map[string]any{
		"ok":            true,
		"domain":        norm,
		"matches":       matches,
		"matched":       len(matches) > 0,
		"hostlist_mode": cfg.HostlistText(),
	}
	if len(matches) == 0 {
		out["reason"] = "none"
	}
	if hint := s.customRKNStaleHint(); hint != "" {
		out["custom_rkn_hint"] = hint
	}
	return out, nil
}

// hostlistFileContains — точное совпадение строки (без учёта регистра), построчно.
func hostlistFileContains(path, domain string) bool {
	if path == "" || domain == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	want := strings.ToLower(strings.TrimSpace(domain))
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.EqualFold(line, want) {
			return true
		}
	}
	return false
}

func (s *Server) customRKNStaleHint() string {
	path := filepath.Join(s.paths.StateDir, "custom_rkn_last_ok.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return ""
	}
	days := int(s.now().Sub(t).Hours() / 24)
	if days >= 7 {
		return fmt.Sprintf("custom_rkn не открывался %d дн.", days)
	}
	return ""
}

func (s *Server) fetchDomainListURL(ctx context.Context, rawURL string) (string, error) {
	u, pins, err := validatePublicHTTPURL(rawURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "zator-domain-import/1")
	client := domainImportHTTPClient(pins)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, domainImportMaxBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
