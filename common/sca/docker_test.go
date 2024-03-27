package sca

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

//go:embed testdata/sca_dockertest.tar.gz
var dockerGzipFile []byte

func TestLoadDockerImageFromFile(t *testing.T) {
	check(t, "docker", DockerWantpkgs)
	br := bytes.NewReader(dockerGzipFile)

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

	testPkgs(t, DockerWantpkgs, pkgs)
}

var DockerWantpkgs = []*dxtypes.Package{}

func init() {
	DockerWantpkgs = append(DockerWantpkgs, APKWantPkgs...)
	DockerWantpkgs = append(DockerWantpkgs, RPMWantPkgs...)
	DockerWantpkgs = append(DockerWantpkgs, DPKGWantPkgs...)
	DockerWantpkgs = analyzer.MergePackages(DockerWantpkgs)
}
