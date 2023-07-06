package sca

import (
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/types"
)

// func TestLoadDockerImageFromContext(t *testing.T) {
// 	pkgs, err := LoadDockerImageFromContext("5d0da3dc9764")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	for _, pkg := range pkgs {
// 		fmt.Printf(`{
// Name: %#v,
// Version: %#v,
// },
// `, pkg.Name, pkg.Version)
// 	}
// }

func TestLoadDockerImageFromFile(t *testing.T) {
	pkgs, err := LoadDockerImageFromFile("./sca_dockertest.tar")
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

var wantpkgs = []types.Package{}

func init() {
	wantpkgs = append(wantpkgs, analyzer.ApkWantPkgs...)
	wantpkgs = append(wantpkgs, analyzer.RpmWantPkgs...)
	wantpkgs = append(wantpkgs, analyzer.DpkgWantPkgs...)
}
