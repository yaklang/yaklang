package sca

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed testdata/sca_git_test.tar.gz
var gitGzipFile []byte

func testPkgs(t *testing.T, wantPkgs []*dxtypes.Package, pkgs []*dxtypes.Package) bool {
	t.Helper()

	if len(pkgs) != len(wantPkgs) {
		t.Fatalf("pkgs length error: %d(got) != %d(want)", len(pkgs), len(wantPkgs))
	}
	sort.Slice(pkgs, func(i, j int) bool {
		c := strings.Compare(pkgs[i].Name, pkgs[j].Name)
		if c == 0 {
			return strings.Compare(pkgs[i].Version, pkgs[j].Version) > 0
		}
		return c > 0
	})
	sort.Slice(wantPkgs, func(i, j int) bool {
		c := strings.Compare(wantPkgs[i].Name, wantPkgs[j].Name)
		if c == 0 {
			return strings.Compare(wantPkgs[i].Version, wantPkgs[j].Version) > 0
		}
		return c > 0
	})

	for i := 0; i < len(pkgs); i++ {
		if pkgs[i].Name != wantPkgs[i].Name {
			t.Fatalf("pkgs %d name error: %s(got) != %s(want)", i, pkgs[i].Name, wantPkgs[i].Name)
		}
		if pkgs[i].Version != wantPkgs[i].Version {
			t.Fatalf("pkgs %d(%s) version error: %s(got) != %s(want)", i, pkgs[i].Name, pkgs[i].Version, wantPkgs[i].Version)
		}
	}
	return true
}

func unCompressTarGz(r io.Reader, destinationDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzr)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("ExtractTarGz: Next() failed: %s", err.Error())
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filepath.Join(destinationDir, header.Name), 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(filepath.Join(destinationDir, header.Name))
			if err != nil {
				return err
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
		default:

		}
	}
	return nil
}

func TestScanGitRepo(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), utils.RandStringBytes(16))
	err := os.MkdirAll(tmpDir, 0o777)
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(tmpDir)
	}()
	br := bytes.NewBuffer(gitGzipFile)

	err = unCompressTarGz(br, tmpDir)
	require.NoError(t, err)

	pkgs, err := ScanGitRepo(tmpDir)
	require.NoError(t, err)

	testPkgs(t, GitWantPkgs, pkgs)
}

var GitWantPkgs = []*dxtypes.Package{}

func init() {
	GitWantPkgs = append(GitWantPkgs, APKWantPkgs...)
}
