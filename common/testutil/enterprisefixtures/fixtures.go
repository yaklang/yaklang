package enterprisefixtures

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	archiveRelativePath = "testdata/enterprise-traffic-analysis/fixtures.tar.gz"
	fixtureRootRelative = "testdata/enterprise-traffic-analysis"
	archiveOnlyEnv      = "YAKLANG_ENTERPRISE_FIXTURES_ARCHIVE_ONLY"
)

var requiredExpandedFiles = []string{
	"dns-tunnel-c2/capture.pcap",
	"host-user-identification/Wireshark-tutorial-identifying-hosts-and-users-1-of-5.pcap",
	"http-dga-c2/capture.pcapng",
	"https-keylog-dridex/capture.pcap",
	"lateral-movement-psexec/capture.pcapng",
	"redline-infostealer-2023-07/capture.pcap",
}

// Root returns the expanded fixture tree. Local development trees keep using
// their existing expanded files; fresh checkouts extract the tracked archive
// into the caller-provided temporary directory.
func Root(t testing.TB) string {
	t.Helper()
	repo, err := repositoryRoot()
	if err != nil {
		t.Fatalf("locate enterprise fixture repository: %v", err)
	}
	expanded := filepath.Join(repo, filepath.FromSlash(fixtureRootRelative))
	if os.Getenv(archiveOnlyEnv) != "1" && hasRequiredFiles(expanded) {
		return expanded
	}
	destination := t.TempDir()
	if err := ExtractArchive(filepath.Join(repo, filepath.FromSlash(archiveRelativePath)), destination); err != nil {
		t.Fatalf("extract enterprise fixtures: %v", err)
	}
	return destination
}

// Path returns a safe absolute path inside the materialized fixture tree.
func Path(t testing.TB, relative string) string {
	t.Helper()
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		t.Fatalf("unsafe enterprise fixture path: %q", relative)
	}
	return filepath.Join(Root(t), clean)
}

// ReadFile reads one fixture after restoring the archive when necessary.
func ReadFile(t testing.TB, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(Path(t, relative))
	if err != nil {
		t.Fatalf("read enterprise fixture %s: %v", relative, err)
	}
	return data
}

// ExtractArchive safely extracts a gzip-compressed tar archive.
func ExtractArchive(archivePath, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	source, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer source.Close()
	compressed, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("fixture archive has negative size for %s", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("fixture archive has unsupported entry %q", header.Name)
		}
	}
}

func safeArchiveTarget(destination, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture archive has unsafe path %q", name)
	}
	target := filepath.Join(destination, clean)
	relative, err := filepath.Rel(destination, target)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture archive escapes destination: %q", name)
	}
	return target, nil
}

func repositoryRoot() (string, error) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", err
	}
	return root, nil
}

func hasRequiredFiles(root string) bool {
	for _, relative := range requiredExpandedFiles {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}
