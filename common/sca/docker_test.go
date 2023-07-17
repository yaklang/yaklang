package sca

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

var (
	//go:embed testdata/sca_dockertest.tar.gz
	gzipFile []byte
)

func TestLoadDockerImageFromFile(t *testing.T) {
	check(t, "docker", wantpkgs)
	br := bytes.NewReader(gzipFile)

	r, err := gzip.NewReader(br)
	if err != nil {
		t.Fatalf("can't new gzip Reader: %v", err)
	}
	tmp, err := os.CreateTemp("", "docker_test_")
	if err != nil {
		t.Fatalf("can't open tmp file: %v", err)
	}
	if _, err := io.Copy(tmp, r); err != nil {
		t.Fatalf("can't copy gzip data to tmpfile: %v", err)
	}
	defer func() {
		name := tmp.Name()
		tmp.Close()
		os.Remove(name)
	}()

	pkgs, err := ScanDockerImageFromFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(pkgs) != len(wantpkgs) {
		t.Fatalf("pkgs length error: %d(got) != %d(want)", len(pkgs), len(wantpkgs))
	}
	sort.Slice(pkgs, func(i, j int) bool {
		c := strings.Compare(pkgs[i].Name, pkgs[j].Name)
		if c == 0 {
			return strings.Compare(pkgs[i].Version, pkgs[j].Version) > 0
		}
		return c > 0
	})
	sort.Slice(wantpkgs, func(i, j int) bool {
		c := strings.Compare(wantpkgs[i].Name, wantpkgs[j].Name)
		if c == 0 {
			return strings.Compare(wantpkgs[i].Version, wantpkgs[j].Version) > 0
		}
		return c > 0
	})

	for i := 0; i < len(pkgs); i++ {
		if pkgs[i].Name != wantpkgs[i].Name {
			t.Fatalf("pkgs %d name error: %s(got) != %s(want)", i, pkgs[i].Name, wantpkgs[i].Name)
		}
		if pkgs[i].Version != wantpkgs[i].Version {
			t.Fatalf("pkgs %d(%s) version error: %s(got) != %s(want)", i, pkgs[i].Name, pkgs[i].Version, wantpkgs[i].Version)
		}
	}
}

var wantpkgs = []*dxtypes.Package{}

func init() {
	wantpkgs = append(wantpkgs, APKWantPkgs...)
	wantpkgs = append(wantpkgs, RPMWantPkgs...)
	wantpkgs = append(wantpkgs, DPKGWantPkgs...)
	wantpkgs = analyzer.MergePackages(wantpkgs)
}
