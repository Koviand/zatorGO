package zapret

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/AloofLibra/zator/internal/fetch"
)

var sha256HexRe = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// parseSHA256SumFile ищет hex-сумму для wantName в тексте sha256sum / SHA256SUMS.
func parseSHA256SumFile(body []byte, wantName string) string {
	wantName = path.Base(wantName)
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*")
		if name == wantName || path.Base(name) == wantName {
			sum := strings.ToLower(fields[0])
			if sha256HexRe.MatchString(sum) {
				return sum
			}
		}
	}
	return ""
}

// sha256sumURL рядом с архивом релиза (…/zapret2-vX.tar.gz → …/sha256sum.txt).
func sha256sumURL(archiveURL string) string {
	i := strings.LastIndex(archiveURL, "/")
	if i < 0 {
		return ""
	}
	return archiveURL[:i+1] + "sha256sum.txt"
}

// fetchArchiveSHA256 загружает sha256sum.txt рядом с archiveURL (через fetch.Client,
// чтобы работали зеркала) и ищет сумму asset.
func fetchArchiveSHA256(ctx context.Context, client *fetch.Client, archiveURL, asset string) (string, error) {
	sumsURL := sha256sumURL(archiveURL)
	if sumsURL == "" {
		return "", fmt.Errorf("zapret: не удалось построить URL sha256sum.txt")
	}
	if client == nil {
		client = fetch.NewClient()
	}
	tmp, err := os.CreateTemp("", "zator-sha256sum-")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpName)

	if err := client.Download(ctx, sumsURL, tmpName, ""); err != nil {
		return "", err
	}
	body, err := os.ReadFile(tmpName)
	if err != nil {
		return "", err
	}
	sum := parseSHA256SumFile(body, asset)
	if sum == "" {
		return "", fmt.Errorf("zapret: в sha256sum.txt нет суммы для %s", asset)
	}
	return sum, nil
}
