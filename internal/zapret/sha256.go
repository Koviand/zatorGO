package zapret

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AloofLibra/zator/internal/fetch"
)

var sha256HexRe = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// SumIndex — разобранный sha256sum.txt: ключ = base name или относительный путь.
type SumIndex map[string]string

// ParseSHA256SumFile разбирает текст sha256sum / SHA256SUMS в индекс сумм.
// Поддерживает и сумму архива (имя *.tar.gz), и per-file строки апстрима
// вида «hex  zapret2-vX/binaries/linux-arm64/nfqws2».
func ParseSHA256SumFile(body []byte) SumIndex {
	idx := SumIndex{}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		if !sha256HexRe.MatchString(sum) {
			continue
		}
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*")
		name = path.Clean("/" + filepath.ToSlash(name))
		name = strings.TrimPrefix(name, "/")
		if name == "" || name == "." {
			continue
		}
		idx[name] = sum
		base := path.Base(name)
		if base != name {
			// Не затираем уже записанную сумму архива более специфичным путём.
			if _, ok := idx[base]; !ok {
				idx[base] = sum
			}
		}
	}
	return idx
}

// ArchiveSum возвращает сумму для имени ассета (zapret2-vX.tar.gz), если она есть.
func (idx SumIndex) ArchiveSum(asset string) string {
	if idx == nil {
		return ""
	}
	asset = path.Base(asset)
	if s := idx[asset]; s != "" {
		return s
	}
	return ""
}

// BinarySum ищет сумму для binaries/<archDir>/<bin> (с любым префиксом каталога релиза).
func (idx SumIndex) BinarySum(archDir, bin string) string {
	if idx == nil {
		return ""
	}
	suffix := path.Join("binaries", archDir, bin)
	if s := idx[suffix]; s != "" {
		return s
	}
	for k, s := range idx {
		if k == suffix || strings.HasSuffix(k, "/"+suffix) {
			return s
		}
	}
	return ""
}

// HasBinarySums сообщает, есть ли в индексе хотя бы одна сумма под binaries/.
func (idx SumIndex) HasBinarySums() bool {
	for k := range idx {
		if strings.Contains(k, "/binaries/") || strings.HasPrefix(k, "binaries/") {
			return true
		}
	}
	return false
}

// parseSHA256SumFile — совместимость: сумма для wantName (архив или base).
func parseSHA256SumFile(body []byte, wantName string) string {
	return ParseSHA256SumFile(body).ArchiveSum(wantName)
}

// sha256sumURL рядом с архивом релиза (…/zapret2-vX.tar.gz → …/sha256sum.txt).
func sha256sumURL(archiveURL string) string {
	i := strings.LastIndex(archiveURL, "/")
	if i < 0 {
		return ""
	}
	return archiveURL[:i+1] + "sha256sum.txt"
}

// fetchSumIndex загружает и разбирает sha256sum.txt рядом с archiveURL.
func fetchSumIndex(ctx context.Context, client *fetch.Client, archiveURL string) (SumIndex, error) {
	sumsURL := sha256sumURL(archiveURL)
	if sumsURL == "" {
		return nil, fmt.Errorf("zapret: не удалось построить URL sha256sum.txt")
	}
	if client == nil {
		client = fetch.NewClient()
	}
	tmp, err := os.CreateTemp("", "zator-sha256sum-")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpName)

	if err := client.Download(ctx, sumsURL, tmpName, ""); err != nil {
		return nil, err
	}
	body, err := os.ReadFile(tmpName)
	if err != nil {
		return nil, err
	}
	idx := ParseSHA256SumFile(body)
	if len(idx) == 0 {
		return nil, fmt.Errorf("zapret: пустой или неразборный sha256sum.txt")
	}
	return idx, nil
}

// fetchArchiveSHA256 загружает sha256sum.txt и ищет сумму архива asset.
// Для формата апстрима (только per-file) возвращает ошибку — вызывающий
// код должен использовать fetchSumIndex + verifyStagedBinaries.
func fetchArchiveSHA256(ctx context.Context, client *fetch.Client, archiveURL, asset string) (string, error) {
	idx, err := fetchSumIndex(ctx, client, archiveURL)
	if err != nil {
		return "", err
	}
	sum := idx.ArchiveSum(asset)
	if sum == "" {
		return "", fmt.Errorf("zapret: в sha256sum.txt нет суммы для %s", asset)
	}
	return sum, nil
}

// verifyStagedBinaries сверяет nfqws2 (обязательно) и ip2net/mdig (если есть
// в индексе) с per-file суммами апстрима. Fail-closed для nfqws2.
func verifyStagedBinaries(stage, archDir string, idx SumIndex) error {
	if idx == nil || len(idx) == 0 {
		return fmt.Errorf("zapret: нет сумм для проверки бинарников")
	}
	want := idx.BinarySum(archDir, BinNfqws2)
	if want == "" {
		return fmt.Errorf("zapret: в sha256sum.txt нет суммы для binaries/%s/%s", archDir, BinNfqws2)
	}
	got, err := fetch.SHA256File(filepath.Join(stage, "binaries", archDir, BinNfqws2))
	if err != nil {
		return fmt.Errorf("zapret: проверка %s: %w", BinNfqws2, err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("zapret: SHA256 %s не совпадает", BinNfqws2)
	}
	for _, b := range []string{BinIP2Net, BinMdig} {
		p := filepath.Join(stage, "binaries", archDir, b)
		if !fileExists(p) {
			continue
		}
		w := idx.BinarySum(archDir, b)
		if w == "" {
			continue // необязательные бинарники без строки в sums — не валим
		}
		g, err := fetch.SHA256File(p)
		if err != nil {
			return fmt.Errorf("zapret: проверка %s: %w", b, err)
		}
		if !strings.EqualFold(g, w) {
			return fmt.Errorf("zapret: SHA256 %s не совпадает", b)
		}
	}
	return nil
}
