package tlsutils

import (
	"bytes"
	"crypto/rsa"
	"fmt"
	"strings"

	cryptorand "crypto/rand"
)

// RSAEncryptWithPKCS1v15Block encrypts plaintext with PKCS#1 v1.5 and automatically chunks long plaintext.
func RSAEncryptWithPKCS1v15Block(pubKeyPem string, data []byte) ([]byte, error) {
	pub, err := GetRSAPubKey([]byte(pubKeyPem))
	if err != nil {
		return nil, fmt.Errorf("parse public key failed: %w", err)
	}

	if len(data) == 0 {
		return []byte{}, nil
	}

	maxPlainBlockSize := pub.Size() - 11
	if maxPlainBlockSize <= 0 {
		return nil, fmt.Errorf("invalid rsa public key size: %d", pub.Size())
	}

	var encrypted bytes.Buffer
	for start := 0; start < len(data); start += maxPlainBlockSize {
		end := start + maxPlainBlockSize
		if end > len(data) {
			end = len(data)
		}

		chunk, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pub, data[start:end])
		if err != nil {
			return nil, fmt.Errorf("encrypt block %d failed: %w", start/maxPlainBlockSize, err)
		}
		encrypted.Write(chunk)
	}
	return encrypted.Bytes(), nil
}

// RSADecryptWithPKCS1v15Block decrypts ciphertext with PKCS#1 v1.5 and automatically chunks by key size.
func RSADecryptWithPKCS1v15Block(privKeyPem string, ciphertext []byte) ([]byte, error) {
	pri, err := GetRSAPrivateKey([]byte(privKeyPem))
	if err != nil {
		return nil, fmt.Errorf("parse private key failed: %w", err)
	}

	if len(ciphertext) == 0 {
		return []byte{}, nil
	}

	blockSize := pri.Size()
	if blockSize <= 0 {
		return nil, fmt.Errorf("invalid rsa private key size: %d", pri.Size())
	}
	if len(ciphertext)%blockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length %d: not multiple of rsa block size %d", len(ciphertext), blockSize)
	}

	var plaintext bytes.Buffer
	for start := 0; start < len(ciphertext); start += blockSize {
		chunk, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, ciphertext[start:start+blockSize])
		if err != nil {
			return nil, fmt.Errorf("decrypt block %d failed: %w", start/blockSize, err)
		}
		plaintext.Write(chunk)
	}
	return plaintext.Bytes(), nil
}

// RSASignWithPKCS1v15Digest signs data using PKCS#1 v1.5 with sha256/sha512.
func RSASignWithPKCS1v15Digest(privKeyPem string, data []byte, algo string) ([]byte, error) {
	switch normalizeRSASignAlgo(algo) {
	case "sha256":
		return PemSignSha256WithRSA([]byte(privKeyPem), data)
	case "sha512":
		return PemSignSha512WithRSA([]byte(privKeyPem), data)
	default:
		return nil, fmt.Errorf("unsupported rsa sign algorithm: %s", algo)
	}
}

// RSAVerifyWithPKCS1v15Digest verifies RSA signature using PKCS#1 v1.5 with sha256/sha512.
func RSAVerifyWithPKCS1v15Digest(pubKeyPem string, data []byte, signature []byte, algo string) (bool, error) {
	var err error
	switch normalizeRSASignAlgo(algo) {
	case "sha256":
		err = PemVerifySignSha256WithRSA([]byte(pubKeyPem), data, signature)
	case "sha512":
		err = PemVerifySignSha512WithRSA([]byte(pubKeyPem), data, signature)
	default:
		return false, fmt.Errorf("unsupported rsa verify algorithm: %s", algo)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RSAEncryptWithJSEncryptStyle is an alias for PKCS#1 v1.5 block encryption compatibility.
func RSAEncryptWithJSEncryptStyle(pubKeyPem string, data []byte) ([]byte, error) {
	return RSAEncryptWithPKCS1v15Block(pubKeyPem, data)
}

// RSADecryptWithJSEncryptStyle is an alias for PKCS#1 v1.5 block decryption compatibility.
func RSADecryptWithJSEncryptStyle(privKeyPem string, ciphertext []byte) ([]byte, error) {
	return RSADecryptWithPKCS1v15Block(privKeyPem, ciphertext)
}

func normalizeRSASignAlgo(algo string) string {
	normalized := strings.ToLower(strings.TrimSpace(algo))
	if normalized == "" {
		return "sha256"
	}

	var compact strings.Builder
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			compact.WriteRune(r)
		}
	}

	switch compact.String() {
	case "sha256", "sha256withrsa", "rsasha256":
		return "sha256"
	case "sha512", "sha512withrsa", "rsasha512":
		return "sha512"
	default:
		return normalized
	}
}
