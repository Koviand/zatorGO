//go:build ignore

// Command sign-sha256sums подписывает SHA256SUMS ключом Ed25519 (Go raw format).
//
//	ZATOR_RELEASE_SIGNING_KEY=<base64-64-byte-priv> go run ./scripts/sign-sha256sums.go \
//	  -in out/SHA256SUMS -out out/SHA256SUMS.sig
//
// Или: go run ./scripts/sign-sha256sums.go -key-file .secrets/release.ed25519.b64 ...
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AloofLibra/zator/internal/deploy"
)

func main() {
	inPath := flag.String("in", "SHA256SUMS", "путь к SHA256SUMS")
	outPath := flag.String("out", "SHA256SUMS.sig", "путь к файлу подписи")
	keyFile := flag.String("key-file", "", "файл с base64 private key")
	keyEnv := flag.String("key-env", "ZATOR_RELEASE_SIGNING_KEY", "имя env с base64 private key")
	flag.Parse()

	keyB64 := ""
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			fatal(err)
		}
		keyB64 = string(b)
	} else {
		keyB64 = os.Getenv(*keyEnv)
	}
	if keyB64 == "" {
		fatal(fmt.Errorf("нет ключа: задайте %s или -key-file", *keyEnv))
	}
	priv, err := deploy.ParsePrivateKeyB64(keyB64)
	if err != nil {
		fatal(err)
	}
	sums, err := os.ReadFile(*inPath)
	if err != nil {
		fatal(err)
	}
	sig, err := deploy.SignSHA256SUMS(priv, sums)
	if err != nil {
		fatal(err)
	}
	body := "# zator ed25519 signature of SHA256SUMS\n" + sig + "\n"
	if err := os.WriteFile(*outPath, []byte(body), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "signed %s → %s\n", *inPath, *outPath)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "sign-sha256sums: %v\n", err)
	os.Exit(1)
}
