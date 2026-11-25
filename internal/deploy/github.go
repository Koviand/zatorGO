package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AloofLibra/zator/internal/fetch"
	"github.com/AloofLibra/zator/internal/platform"
)

// GitHubAPIBase — базовый URL GitHub API (переопределяется в тестах).
var GitHubAPIBase = "https://api.github.com"

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// FetchGitHubLatest читает /repos/{owner}/{repo}/releases/latest и строит
// Latest для сравнения версий. Если в релизе есть latest.json — не сюда:
// основной путь остаётся FetchLatest через ассет latest.json.
func (u *Updater) FetchGitHubLatest(ctx context.Context) (*Latest, error) {
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
	if len(body) > maxLatestBytes {
		return nil, fmt.Errorf("%w: слишком большой ответ GitHub API", ErrInvalidLatest)
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLatest, err)
	}
	tag := strings.TrimSpace(rel.TagName)
	if tag == "" {
		return nil, fmt.Errorf("%w: пустой tag_name", ErrInvalidLatest)
	}
	buildDate := ""
	if !rel.PublishedAt.IsZero() {
		buildDate = rel.PublishedAt.UTC().Format("2006-01-02 15:04")
	}
	return &Latest{
		SchemaVersion: SchemaVersion,
		Release:       tag,
		BuildDate:     buildDate,
		ZatorDate:     buildDate,
		WebUIDate:     buildDate,
		Assets:        map[string]Asset{},
	}, nil
}

// binaryAssetName — имя ассета бинарника для текущей архитектуры устройства.
// Учитывает ReleaseArch (Lexra → SelfArch/mips BE) и детект платформы, а не
// только GOARCH бинарника (на Lexra SelfArch() без cpuinfo не увидит Lexra).
func binaryAssetName() string {
	arch := platform.Detect().Arch
	if arch == platform.ArchUnknown {
		arch = platform.SelfArch()
	}
	if arch == platform.ArchUnknown {
		switch runtime.GOARCH {
		case "amd64":
			arch = platform.ArchX86_64
		case "386":
			arch = platform.ArchX86
		case "arm64":
			arch = platform.ArchARM64
		case "arm":
			arch = platform.ArchARM
		case "riscv64":
			arch = platform.ArchRISCV64
		default:
			arch = platform.Arch(runtime.GOARCH)
		}
	}
	if name := platform.BinaryAssetName(arch); name != "" {
		return name
	}
	return "zator-linux-" + string(platform.ReleaseArch(arch))
}

// FindGitHubBinaryURL ищет URL zator-linux-<arch> в latest-релизе.
// sha — обязательный hex из ассета SHA256SUMS (fail-closed без суммы).
func (u *Updater) FindGitHubBinaryURL(ctx context.Context) (tag, url string, size int64, sha string, err error) {
	if u.Offline {
		return "", "", 0, "", ErrOffline
	}
	owner, repo := ReleaseOwner(), ReleaseRepo()
	api := strings.TrimRight(GitHubAPIBase, "/") + "/repos/" + owner + "/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", "", 0, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	client := u.fetcher().HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLatestBytes+1))
	if err != nil {
		return "", "", 0, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, "", &fetch.HTTPError{URL: api, Status: resp.StatusCode}
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", "", 0, "", err
	}
	tag = strings.TrimSpace(rel.TagName)
	want := binaryAssetName()
	var sumsURL, sigURL string
	for _, a := range rel.Assets {
		switch {
		case a.Name == want && a.BrowserDownloadURL != "":
			url, size = a.BrowserDownloadURL, a.Size
		case a.Name == SHA256SUMSName && a.BrowserDownloadURL != "":
			sumsURL = a.BrowserDownloadURL
		case a.Name == SHA256SUMSSigName && a.BrowserDownloadURL != "":
			sigURL = a.BrowserDownloadURL
		}
	}
	if url == "" {
		return tag, "", 0, "", fmt.Errorf("deploy: в релизе нет ассета %s", want)
	}
	if sumsURL == "" {
		return tag, "", 0, "", fmt.Errorf("deploy: в релизе нет ассета %s", SHA256SUMSName)
	}
	if sigURL == "" {
		return tag, "", 0, "", fmt.Errorf("deploy: в релизе нет ассета %s", SHA256SUMSSigName)
	}
	sha, err = u.fetchVerifiedAssetSHA256(ctx, sumsURL, sigURL, want)
	if err != nil {
		return tag, "", 0, "", err
	}
	return tag, url, size, sha, nil
}

func (u *Updater) httpGetLimited(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// fetchVerifiedAssetSHA256 скачивает SHA256SUMS + .sig, проверяет Ed25519,
// затем возвращает hex-сумму для wantName (fail-closed).
func (u *Updater) fetchVerifiedAssetSHA256(ctx context.Context, sumsURL, sigURL, wantName string) (string, error) {
	sums, err := u.httpGetLimited(ctx, sumsURL, 1<<20)
	if err != nil {
		return "", err
	}
	sig, err := u.httpGetLimited(ctx, sigURL, 1<<16)
	if err != nil {
		return "", err
	}
	pub, err := ReleasePublicKey()
	if err != nil {
		return "", err
	}
	if err := VerifySHA256SUMS(pub, sums, sig); err != nil {
		return "", err
	}
	sum := parseSHA256SUMS(sums, wantName)
	if sum == "" {
		return "", fmt.Errorf("deploy: в SHA256SUMS нет суммы для %s", wantName)
	}
	return sum, nil
}

func parseSHA256SUMS(body []byte, wantName string) string {
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*") // binary mode marker
		if name == wantName || filepath.Base(name) == wantName {
			sum := strings.ToLower(fields[0])
			if sha256Re.MatchString(sum) {
				return sum
			}
		}
	}
	return ""
}

// ApplyGitHubBinary скачивает zator-linux-<arch>, заменяет бинарник и
// запускает «zator install -zapret=false», чтобы подтянуть встроенные assets.
func (u *Updater) ApplyGitHubBinary(ctx context.Context) (*ApplyResult, string, error) {
	tag, link, size, sha, err := u.FindGitHubBinaryURL(ctx)
	if err != nil {
		return nil, "", err
	}
	if link == "" {
		return nil, tag, fmt.Errorf("deploy: бинарник для этой arch не найден в релизе %s", tag)
	}
	dir := Dir(u.Paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, tag, err
	}
	dest := filepath.Join(dir, binaryAssetName())
	if err := u.fetcher().Download(ctx, link, dest, sha); err != nil {
		return nil, tag, err
	}
	if size > 0 {
		st, err := os.Stat(dest)
		if err != nil {
			return nil, tag, err
		}
		if st.Size() != size {
			os.Remove(dest)
			return nil, tag, fmt.Errorf("%w: ожидалось %d, получено %d", ErrSize, size, st.Size())
		}
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return nil, tag, err
	}
	binDir := u.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, tag, err
	}
	final := filepath.Join(binDir, "zator")
	// Снимок текущего бинарника: если install нового упадёт — вернём старый.
	prev := final + ".prev"
	hadPrev := false
	if old, rerr := os.ReadFile(final); rerr == nil && len(old) > 0 {
		if werr := os.WriteFile(prev, old, 0o755); werr == nil {
			hadPrev = true
		}
	}
	restorePrev := func() {
		if !hadPrev {
			return
		}
		if data, err := os.ReadFile(prev); err == nil {
			_ = os.WriteFile(final, data, 0o755)
		}
		_ = os.Remove(prev)
	}
	tmp := final + ".new"
	data, err := os.ReadFile(dest)
	if err != nil {
		restorePrev()
		return nil, tag, err
	}
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		restorePrev()
		return nil, tag, err
	}
	os.Remove(dest)
	if err := os.Rename(tmp, final); err != nil {
		// ETXTBSY / занятый inode: перезапись поверх (Linux обычно допускает rename
		// запущенного бинарника; fallback — WriteFile).
		if werr := os.WriteFile(final, data, 0o755); werr != nil {
			_ = os.Remove(tmp)
			restorePrev()
			return nil, tag, fmt.Errorf("deploy: замена бинарника: %v; fallback: %w", err, werr)
		}
		_ = os.Remove(tmp)
	}
	// Новый бинарник сам разложит актуальные assets (webui и пр.).
	// -skip-service: не трогаем units; auth из существующего zator.json сохраняется.
	cmd := exec.CommandContext(ctx, final, "install", "-zapret=false", "-skip-service")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		restorePrev()
		return nil, tag, fmt.Errorf("deploy: %s install: %w (%s)", final, err, strings.TrimSpace(string(out)))
	}
	_ = os.Remove(prev)
	ver := strings.TrimPrefix(tag, "v")
	now := time.Now().UTC().Format("2006-01-02 15:04")
	_ = WriteVersion(u.Paths, Version{
		ZatorVersion: ver,
		ZatorDate:    now,
		WebUIVersion: ver,
		WebUIDate:    now,
		Tracking:     "latest",
	})
	return &ApplyResult{Installed: 1}, tag, nil
}
