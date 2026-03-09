package tlsutils

import (
	"bytes"
	"testing"
)

func TestRSASuiteEncryptDecrypt_Blocking(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GeneratePrivateAndPublicKeyPEMWithPrivateFormatterWithSize("pkcs1", 1024)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}

	plaintext := bytes.Repeat([]byte("yaklang-rsa-suite-js-compat-"), 64)
	ciphertext, err := RSAEncryptWithPKCS1v15Block(string(publicKeyPEM), plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatalf("empty ciphertext")
	}

	decrypted, err := RSADecryptWithPKCS1v15Block(string(privateKeyPEM), ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("plaintext mismatch after decrypt")
	}
}

func TestRSASuiteSignVerify(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GeneratePrivateAndPublicKeyPEMWithPrivateFormatterWithSize("pkcs1", 1024)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}

	data := []byte("yaklang-rsa-sign-verify")
	signature256, err := RSASignWithPKCS1v15Digest(string(privateKeyPEM), data, "SHA256withRSA")
	if err != nil {
		t.Fatalf("sign sha256 failed: %v", err)
	}

	ok, err := RSAVerifyWithPKCS1v15Digest(string(publicKeyPEM), data, signature256, "sha-256")
	if err != nil {
		t.Fatalf("verify sha256 failed: %v", err)
	}
	if !ok {
		t.Fatalf("verify sha256 returned false")
	}

	signature512, err := RSASignWithPKCS1v15Digest(string(privateKeyPEM), data, "sha512")
	if err != nil {
		t.Fatalf("sign sha512 failed: %v", err)
	}

	ok, err = RSAVerifyWithPKCS1v15Digest(string(publicKeyPEM), data, signature512, "RSA-SHA512")
	if err != nil {
		t.Fatalf("verify sha512 failed: %v", err)
	}
	if !ok {
		t.Fatalf("verify sha512 returned false")
	}
}

func TestRSASuiteSign_UnsupportedAlgo(t *testing.T) {
	privateKeyPEM, _, err := GeneratePrivateAndPublicKeyPEMWithPrivateFormatterWithSize("pkcs1", 1024)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}

	if _, err := RSASignWithPKCS1v15Digest(string(privateKeyPEM), []byte("hello"), "md5"); err == nil {
		t.Fatalf("expected unsupported algo error")
	}
}
