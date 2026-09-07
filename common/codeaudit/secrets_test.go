package codeaudit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSecrets_AwsKey(t *testing.T) {
	dir := t.TempDir()
	content := `package com.example;
class Config {
    String key = "AKIAIOSFODNN7EXAMPLE";
}`
	if err := os.WriteFile(filepath.Join(dir, "Config.java"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report := ScanSecrets(dir, WithLanguage("java"))

	foundAwsKey := false
	for _, f := range report.Findings {
		if f.ID == "secret.aws_access_key" {
			foundAwsKey = true
		}
	}
	if !foundAwsKey {
		t.Errorf("expected to find secret.aws_access_key finding, got: %+v", report.Findings)
	}
}

func TestScanSecrets_JdbcCredential(t *testing.T) {
	dir := t.TempDir()
	content := `spring.datasource.url=jdbc:mysql://root:secret123@localhost:3306/db`
	if err := os.WriteFile(filepath.Join(dir, "application.properties"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report := ScanSecrets(dir, WithLanguage("java"))

	foundJdbc := false
	for _, f := range report.Findings {
		if f.ID == "secret.jdbc_inline_credential" {
			foundJdbc = true
		}
	}
	if !foundJdbc {
		t.Errorf("expected to find secret.jdbc_inline_credential finding, got: %+v", report.Findings)
	}
}

func TestScanSecrets_PlaceholderFilter(t *testing.T) {
	dir := t.TempDir()
	content := `password = changeme`
	if err := os.WriteFile(filepath.Join(dir, "app.properties"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report := ScanSecrets(dir, WithLanguage("java"))

	// The value "changeme" should be filtered as a placeholder
	for _, f := range report.Findings {
		if f.ID == "config.password.property" || f.ID == "secret.password_assignment" {
			t.Errorf("expected placeholder 'changeme' to be filtered, but found: %s", f.ID)
		}
	}
}

func TestScanSecrets_PrivateKey(t *testing.T) {
	dir := t.TempDir()
	content := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890
-----END RSA PRIVATE KEY-----`
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report := ScanSecrets(dir, WithLanguage("java"))

	foundPrivateKey := false
	for _, f := range report.Findings {
		if f.ID == "secret.private_key_block" {
			foundPrivateKey = true
		}
	}
	if !foundPrivateKey {
		t.Errorf("expected to find secret.private_key_block finding, got: %+v", report.Findings)
	}
}
