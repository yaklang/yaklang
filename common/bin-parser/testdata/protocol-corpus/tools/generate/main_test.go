package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
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

func TestValidateSpecEvidenceOriginAndDecodeAs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*sourceSpec)
		want   string
	}{
		{"upstream mislabeled generated", func(s *sourceSpec) { s.Captures[0].EvidenceKind = "generated-positive" }, "evidence kind"},
		{"generated mislabeled upstream", func(s *sourceSpec) {
			s.Repositories[0].Origin = "generated"
			s.Repositories[0].Recipe = "tools/recipe.py"
		}, "evidence kind"},
		{"invalid mapping", func(s *sourceSpec) { s.Captures[0].DecodeAs = []string{"--anything"} }, "decode_as"},
		{"duplicate mapping", func(s *sourceSpec) { s.Captures[0].DecodeAs = []string{"udp.port==9995,cflow", "udp.port==9995,cflow"} }, "decode_as"},
		{"recipe traversal", func(s *sourceSpec) { s.Repositories[0].Origin = "generated"; s.Repositories[0].Recipe = "../recipe.py" }, "recipe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validDigestSpec()
			tc.mutate(&spec)
			if err := validateSpec(spec); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
	for _, kind := range []string{"generated-positive", "generated-negative", "generated-identification"} {
		spec := validDigestSpec()
		spec.Repositories[0].Origin = "generated"
		spec.Repositories[0].Recipe = "tools/recipe.py"
		spec.Captures[0].EvidenceKind = kind
		spec.Captures[0].DecodeAs = []string{"tcp.port==7551,edonkey", "udp.port==9995,cflow"}
		if err := validateSpec(spec); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrepareGeneratedRepositoryVerifiesRecipe(t *testing.T) {
	dir := t.TempDir()
	repo := validDigestSpec().Repositories[0]
	repo.Origin = "generated"
	repo.Recipe = "recipe.py"
	repo.LicensePath = "NOTICE"
	recipe := []byte("# deterministic fixture recipe\n")
	license := []byte("CC0 fixture notice\n")
	repo.Commit = fmt.Sprintf("%x", sha1.Sum(recipe))
	repo.LicenseSHA256 = digestHex(license)
	if err := os.WriteFile(filepath.Join(dir, repo.Recipe), recipe, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, repo.LicensePath), license, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareRepositories(dir, []repositorySpec{repo}, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, repo.Recipe), []byte("# changed recipe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareRepositories(dir, []repositorySpec{repo}, false); err == nil || !strings.Contains(err.Error(), "recipe digest mismatch") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestRepresentativeSelectionPreservesDecodeAsArguments(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "args")
	installFixtureTshark(t, dir, "printf '%s\\n' \"$@\" > \"$TSHARK_ARGS\"\nprintf '7\\teth:ip:udp:cflow\\n'\n")
	t.Setenv("TSHARK_ARGS", marker)
	number, protocols, err := selectRepresentativeFrame("path with spaces.pcap", "cflow.version == 10", "udp.port==9995,cflow")
	if err != nil || number != 7 || strings.Join(protocols, ":") != "eth:ip:udp:cflow" {
		t.Fatalf("unexpected representative: %d %v %v", number, protocols, err)
	}
	args, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	want := "-r\npath with spaces.pcap\n-Y\ncflow.version == 10\n-T\nfields\n-e\nframe.number\n-e\nframe.protocols\n-d\nudp.port==9995,cflow\n"
	if string(args) != want {
		t.Fatalf("arguments changed: %q", args)
	}
}

func TestPrepareCapturesRejectsUnmatchedFilterAndKeepsPriorHex(t *testing.T) {
	dir := t.TempDir()
	spec := validDigestSpec()
	repo := spec.Repositories[0]
	var pcap bytes.Buffer
	writer := pcapgo.NewWriter(&pcap)
	if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 14)
	if err := writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), CaptureLength: len(frame), Length: len(frame)}, frame); err != nil {
		t.Fatal(err)
	}
	first, second := spec.Captures[0], spec.Captures[0]
	first.ID = "first"
	second.ID = "second"
	second.DisplayFilter = "no-match"
	first.SourceSHA256 = digestHex(pcap.Bytes())
	second.SourceSHA256 = first.SourceSHA256
	captureDir := filepath.Join(dir, "captures", repo.ID)
	hexDir := filepath.Join(dir, "hex")
	for _, path := range []string{captureDir, hexDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{first.ID, second.ID} {
		if err := os.WriteFile(filepath.Join(captureDir, id+".pcap"), pcap.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prior := []byte("preserve previous complete result\n")
	priorPath := filepath.Join(hexDir, "prior.frame-1.hex")
	if err := os.WriteFile(priorPath, prior, 0o644); err != nil {
		t.Fatal(err)
	}
	installFixtureTshark(t, dir, "for arg do\n if [ \"$arg\" = 'no-match' ]; then exit 0; fi\ndone\nprintf '1\\teth\\n'\n")
	_, err := prepareCaptures(dir, []captureSpec{first, second}, map[string]repositorySpec{repo.ID: repo}, nil, false)
	if err == nil || !strings.Contains(err.Error(), "matched no frame in second") {
		t.Fatalf("unexpected filter validation: %v", err)
	}
	after, err := os.ReadFile(priorPath)
	if err != nil || !bytes.Equal(prior, after) {
		t.Fatalf("prior result lost: %q %v", after, err)
	}
	entries, err := os.ReadDir(hexDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial hex results published: %v %v", entries, err)
	}
	second.DisplayFilter = "frame"
	result, err := prepareCaptures(dir, []captureSpec{first, second}, map[string]repositorySpec{repo.ID: repo}, nil, false)
	if err != nil || len(result) != 2 {
		t.Fatalf("valid complete inventory failed: %v", err)
	}
	if _, err := os.Stat(priorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale generated hex retained: %v", err)
	}
	for _, capture := range result {
		if capture.RepresentativeFrame == nil || capture.RepresentativeFrame.Number != 1 || capture.PacketCount != 1 {
			t.Fatalf("bad inventory result: %+v", capture)
		}
	}
}

func installFixtureTshark(t *testing.T, dir, program string) {
	t.Helper()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tshark"), []byte("#!/bin/sh\n"+program), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
}
