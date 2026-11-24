package deploy

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/fsutil"
)

const (
	// StrategiesFeedName — ассет каталога стратегий в GitHub Release.
	StrategiesFeedName = "strategies-extra.tsv"
	// StrategiesFeedSigName — Ed25519-подпись каталога (тот же ключ, что SHA256SUMS.sig).
	StrategiesFeedSigName = "strategies-extra.tsv.sig"
)

// StrategiesFeedResult — итог применения подписанного каталога.
type StrategiesFeedResult struct {
	Tag        string   `json:"tag"`
	Applied    int      `json:"applied"`
	Strategies []string `json:"strategies"`
	Path       string   `json:"path"`
}

// ApplyStrategiesFeed скачивает strategies-extra.tsv + .sig, проверяет Ed25519,
// сохраняет в state (fail-closed без валидной подписи).
func (u *Updater) ApplyStrategiesFeed(ctx context.Context) (*StrategiesFeedResult, error) {
	if u.Offline {
		return nil, ErrOffline
	}
	owner, repo := ReleaseOwner(), ReleaseRepo()
	api := strings.TrimRight(GitHubAPIBase, "/") + "/repos/" + owner + "/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	client := u.fetcher().HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLatestBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &fetch.HTTPError{URL: api, Status: resp.StatusCode}
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLatest, err)
	}
	var tsvURL, sigURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case StrategiesFeedName:
			tsvURL = a.BrowserDownloadURL
		case StrategiesFeedSigName:
			sigURL = a.BrowserDownloadURL
		}
	}
	if tsvURL == "" || sigURL == "" {
		return nil, fmt.Errorf("deploy: в релизе %s нет %s / %s", rel.TagName, StrategiesFeedName, StrategiesFeedSigName)
	}
	tsv, err := u.downloadBytes(ctx, tsvURL, 1<<20)
	if err != nil {
		return nil, err
	}
	sig, err := u.downloadBytes(ctx, sigURL, 8<<10)
	if err != nil {
		return nil, err
	}
	pub, err := ReleasePublicKey()
	if err != nil {
		return nil, err
	}
	if err := VerifyStrategiesFeed(pub, tsv, sig); err != nil {
		return nil, err
	}
	dst := filepath.Join(u.Paths.StateDir, StrategiesFeedName)
	if err := fsutil.AtomicWriteFile(dst, tsv, 0o644); err != nil {
		return nil, err
	}
	lines := parseStrategiesFeed(tsv)
	return &StrategiesFeedResult{
		Tag:        strings.TrimSpace(rel.TagName),
		Applied:    len(lines),
		Strategies: lines,
		Path:       dst,
	}, nil
}

// VerifyStrategiesFeed проверяет Ed25519-подпись содержимого strategies-extra.tsv.
func VerifyStrategiesFeed(pub ed25519.PublicKey, tsv, sigFile []byte) error {
	return VerifySHA256SUMS(pub, tsv, sigFile)
}

func parseStrategiesFeed(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (u *Updater) downloadBytes(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	client := u.fetcher().HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &fetch.HTTPError{URL: rawURL, Status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("deploy: слишком большой ассет")
	}
	return data, nil
}
