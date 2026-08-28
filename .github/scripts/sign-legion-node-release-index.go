// Command sign-legion-node-release-index signs the exact unified release
// index with the Ed25519 key held by the protected Yaklang release job.
package main

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	privateKeyFile := flag.String("private-key-file", "", "file containing a base64 Ed25519 seed or private key")
	expectedPublicKey := flag.String("expected-public-key", "", "approved base64 Ed25519 public key")
	input := flag.String("input", "", "release index to sign")
	output := flag.String("output", "", "base64 signature output")
	printPublicKey := flag.Bool("print-public-key", false, "print the derived public key and exit")
	flag.Parse()
	if strings.TrimSpace(*privateKeyFile) == "" {
		fatal("--private-key-file is required")
	}
	raw, err := os.ReadFile(*privateKeyFile)
	if err != nil {
		fatal("read private key: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		fatal("private key must be standard base64: %v", err)
	}
	var privateKey ed25519.PrivateKey
	switch len(decoded) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(decoded)
	case ed25519.PrivateKeySize:
		derived := ed25519.NewKeyFromSeed(decoded[:ed25519.SeedSize])
		if subtle.ConstantTimeCompare(decoded, derived) != 1 {
			fatal("64-byte Ed25519 private key is internally inconsistent")
		}
		privateKey = derived
	default:
		fatal("private key must decode to a 32-byte seed or 64-byte Ed25519 private key")
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKey)
	if *printPublicKey {
		fmt.Println(publicKeyBase64)
		return
	}
	approved, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*expectedPublicKey))
	if err != nil || len(approved) != ed25519.PublicKeySize || subtle.ConstantTimeCompare(publicKey, approved) != 1 {
		fatal("private key does not match the approved release public key")
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		fatal("--input and --output are required when signing")
	}
	payload, err := os.ReadFile(*input)
	if err != nil {
		fatal("read input: %v", err)
	}
	signature := ed25519.Sign(privateKey, payload)
	if err := os.WriteFile(*output, []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0o600); err != nil {
		fatal("write signature: %v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[legion-node-release-sign][error] "+format+"\n", args...)
	os.Exit(1)
}
