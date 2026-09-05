package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSpecRejectsMissingAndInvalidDigests(t *testing.T) {
	if err := validateSpec(validDigestSpec()); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*sourceSpec)
		want   string
	}{
		{
			name: "missing license_sha256",
			mutate: func(spec *sourceSpec) {
				spec.Repositories[0].LicenseSHA256 = ""
			},
			want: "invalid repository specification",
		},
		{
			name: "invalid license_sha256",
			mutate: func(spec *sourceSpec) {
				spec.Repositories[0].LicenseSHA256 = strings.Repeat("g", 64)
			},
			want: "invalid repository specification",
		},
		{
			name: "missing source_sha256",
			mutate: func(spec *sourceSpec) {
				spec.Captures[0].SourceSHA256 = ""
			},
			want: `capture "sample-capture"`,
		},
		{
			name: "invalid source_sha256",
			mutate: func(spec *sourceSpec) {
				spec.Captures[0].SourceSHA256 = strings.Repeat("g", 64)
			},
			want: `capture "sample-capture"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validDigestSpec()
			test.mutate(&spec)
			err := validateSpec(spec)
			if err == nil {
				t.Fatal("validateSpec accepted an absent or malformed digest")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrepareRepositoriesRejectsLicenseDigestMismatch(t *testing.T) {
	corpusDir := t.TempDir()
	repo := validDigestSpec().Repositories[0]
	repo.LicenseSHA256 = digestHex([]byte("expected license contents"))

	licenseDir := filepath.Join(corpusDir, "licenses")
	if err := os.MkdirAll(licenseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	licenseFile := filepath.Join(licenseDir, repo.ID+"-"+filepath.Base(repo.LicensePath)+".txt")
	if err := os.WriteFile(licenseFile, []byte("different local license contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := prepareRepositories(corpusDir, []repositorySpec{repo}, false)
	if err == nil {
		t.Fatal("prepareRepositories accepted license bytes that did not match the pin")
	}
	want := "license digest mismatch for " + repo.ID
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareCapturesRejectsCaptureDigestMismatchBeforeTshark(t *testing.T) {
	corpusDir := t.TempDir()
	spec := validDigestSpec()
	repo := spec.Repositories[0]
	capture := spec.Captures[0]
	capture.SourceSHA256 = digestHex([]byte("expected capture contents"))

	captureDir := filepath.Join(corpusDir, "captures", repo.ID)
	if err := os.MkdirAll(captureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	captureFile := filepath.Join(captureDir, capture.ID+filepath.Ext(capture.UpstreamPath))
	if err := os.WriteFile(captureFile, []byte("different local capture contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(corpusDir, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(corpusDir, "tshark-called")
	fakeTshark := filepath.Join(fakeBin, "tshark")
	fakeProgram := []byte("#!/bin/sh\n: > \"$TSHARK_CALLED_MARKER\"\nexit 97\n")
	if err := os.WriteFile(fakeTshark, fakeProgram, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	t.Setenv("TSHARK_CALLED_MARKER", marker)

	_, err := prepareCaptures(
		corpusDir,
		[]captureSpec{capture},
		map[string]repositorySpec{repo.ID: repo},
		nil,
		false,
	)
	if err == nil {
		t.Fatal("prepareCaptures accepted capture bytes that did not match the pin")
	}
	want := "capture digest mismatch for " + capture.ID
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("tshark was invoked before digest verification; marker stat: %v", statErr)
	}
}

func validDigestSpec() sourceSpec {
	return sourceSpec{
		Schema:        "./source-spec.schema.json",
		SchemaVersion: 1,
		Repositories: []repositorySpec{
			{
				ID:            "sample-repository",
				Repository:    "example/protocol-samples",
				Commit:        strings.Repeat("a", 40),
				License:       "MIT",
				LicensePath:   "LICENSE",
				LicenseSHA256: strings.Repeat("b", 64),
			},
		},
		Captures: []captureSpec{
			{
				ID:            "sample-capture",
				RepositoryID:  "sample-repository",
				UpstreamPath:  "captures/sample.pcap",
				Protocol:      "Sample Protocol",
				DisplayFilter: "frame",
				EvidenceKind:  "upstream-positive",
				SourceSHA256:  strings.Repeat("c", 64),
			},
		},
	}
}

func digestHex(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
