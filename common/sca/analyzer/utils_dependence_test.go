package analyzer

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"reflect"
	"testing"
)

var pkgMaps = map[string][]*dxtypes.Package{}

func newPackage(name, version, prefix string) *dxtypes.Package {
	p := &dxtypes.Package{Name: name, Version: version, FromFile: []string{"/path/" + prefix + "/file"}, FromAnalyzer: []string{prefix + "-analyzer"}}
	pkgMaps[prefix] = append(pkgMaps[prefix], p)
	return p
}

// These are the original seven graph inputs from d33a21b6. Expectations now
// preserve every distinct version and every OR declaration.
func TestMergePackagesNormal(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgMaps = make(map[string][]*dxtypes.Package)

	pa1 := newPackage("pa1", "0.0.3", "pa")
	pa22 := newPackage("pa22", "0.0.3", "pa")
	pa21 := newPackage("pa21", "0.0.3", "pa")
	pa3 := newPackage("pa3", "0.0.3", "pa")
	pa3b := newPackage("pb3", "0.0.2", "pa")
	pb1 := newPackage("pb1", "0.0.3", "pb")
	pb2 := newPackage("pa22", "0.0.3", "pb")
	pb3 := newPackage("pb3", "0.0.3", "pb")
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)

	//             -> pa3b
	// pa1 -> pa22 -> pa3
	//     -> pa21
	pa1.LinkDepend(pa22)
	pa1.LinkDepend(pa21)
	pa22.LinkDepend(pa3)
	pa22.LinkDepend(pa3b)

	// pb1 -> pb2(pa22) -> pb3
	pb1.LinkDepend(pb2)
	pb2.LinkDepend(pb3)

	assertExactGraph(t, pkgs, 7)
}
func TestMergePackagesVersionRange(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgMaps = make(map[string][]*dxtypes.Package)
	pa1 := newPackage("pa1", "0.0.3", "pa")
	pa22 := newPackage("pa22", "0.0.3", "pa")
	pa21 := newPackage("pa21", "0.0.3", "pa")
	pa3 := newPackage("pa3", "0.0.3", "pa")
	pa3b := newPackage("pb3", "0.0.2", "pa")
	pe1 := newPackage("pe1", "0.0.3", "pe")
	pe2 := newPackage("pa22", "<0.0.5", "pe")
	pe3 := newPackage("pa3", ">0.0.3", "pe")
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pe"]...)

	//             -> pa3b
	// pa1 -> pa22 -> pa3
	//     -> pa21
	pa1.LinkDepend(pa22)
	pa1.LinkDepend(pa21)
	pa22.LinkDepend(pa3)
	pa22.LinkDepend(pa3b)

	// pe1 -> pe2(pa2<0.0.5) -> pe3
	pe1.LinkDepend(pe2)
	pe2.LinkDepend(pe3)

	assertExactGraph(t, pkgs, 8)
}
func TestMergePackagesOrPackage(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgMaps = make(map[string][]*dxtypes.Package)
	pa1 := newPackage("pa1", "0.0.3", "pa")
	pa22 := newPackage("pa22", "0.0.3", "pa")
	pa21 := newPackage("pa21", "0.0.3", "pa")
	pa3 := newPackage("pa3", "0.0.3", "pa")
	pa3b := newPackage("pb3", "0.0.2", "pa")
	pb1 := newPackage("pb1", "0.0.3", "pb")
	pb2 := newPackage("pa22", "0.0.3", "pb")
	pb3 := newPackage("pb3", "0.0.3", "pb")
	pc1 := newPackage("pc1", "0.0.3", "pc")
	pc2 := newPackage("pc2", "0.0.3", "pc")
	pc3 := newPackage("pc3", "0.0.3", "pc")
	pcor := newPackage("pa1|pb1|pb2", "0.0.3|0.0.3|0.0.3", "pc")
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	pkgs = append(pkgs, pkgMaps["pc"]...)

	//             -> pa3b
	// pa1 -> pa22 -> pa3
	//     -> pa21
	pa1.LinkDepend(pa22)
	pa1.LinkDepend(pa21)
	pa22.LinkDepend(pa3)
	pa22.LinkDepend(pa3b)

	// pb1 -> pb2(pa22) -> pb3
	pb1.LinkDepend(pb2)
	pb2.LinkDepend(pb3)

	// pc1 -> pc2 -> pc3
	//     -> pa1|pc4|pb2
	pc1.LinkDepend(pc2)
	pc1.LinkDepend(pcor)
	pc2.LinkDepend(pc3)
	assertExactGraph(t, pkgs, 11)
}
func TestMergePackagesOrPackageNotMatch(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgMaps = make(map[string][]*dxtypes.Package)
	pa1 := newPackage("pa1", "0.0.3", "pa")
	pa22 := newPackage("pa22", "0.0.3", "pa")
	pa21 := newPackage("pa21", "0.0.3", "pa")
	pa3 := newPackage("pa3", "0.0.3", "pa")
	pa3b := newPackage("pb3", "0.0.2", "pa")
	pb1 := newPackage("pb1", "0.0.3", "pb")
	pb2a := newPackage("pa22", "0.0.3", "pb")
	pb3 := newPackage("pb3", "0.0.3", "pb")
	pd1 := newPackage("pd1", "0.0.3", "pd")
	pd2 := newPackage("pd2", "0.0.5", "pd")
	pd3 := newPackage("pd3", "0.0.3", "pd")
	// not match
	pdor := newPackage("pe1|pf1|pg2", "0.0.2|0.0.3|0.0.4", "pd")
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	pkgs = append(pkgs, pkgMaps["pd"]...)

	//             -> pa3b
	// pa1 -> pa22 -> pa3a
	//     -> pa21
	pa1.LinkDepend(pa22)
	pa1.LinkDepend(pa21)
	pa22.LinkDepend(pa3)
	pa22.LinkDepend(pa3b)

	// pb1 -> pb2a(pa22) -> pb3
	pb1.LinkDepend(pb2a)
	pb2a.LinkDepend(pb3)

	// pd1 -> pd2 -> pd3
	//     -> pe1|pf1|pg2
	pd1.LinkDepend(pd2)
	pd2.LinkDepend(pd3)
	pd1.LinkDepend(pdor)
	assertExactGraph(t, pkgs, 11)
}
func TestMergePackagesOrPackageVersionRange(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgMaps = make(map[string][]*dxtypes.Package)
	pa1 := newPackage("pa1", "0.0.3", "pa")
	pa22 := newPackage("pa22", "0.0.3", "pa")
	pa21 := newPackage("pa21", "0.0.3", "pa")
	pa3 := newPackage("pa3", "0.0.3", "pa")
	pa3b := newPackage("pb3", "0.0.2", "pa")
	pb1 := newPackage("pb1", "0.0.3", "pb")
	pb2a := newPackage("pa22", "0.0.3", "pb")
	pb3 := newPackage("pb3", "0.0.3", "pb")
	pd1 := newPackage("pd1", "0.0.3", "pd")
	pd2 := newPackage("pd2", "0.0.5", "pd")
	pd3 := newPackage("pd3", "0.0.3", "pd")
	pdor := newPackage("pa1|pb1|pb2", ">0.0.2|>=0.0.3|<0.0.4", "pd")
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	pkgs = append(pkgs, pkgMaps["pd"]...)

	//             -> pa3b
	// pa1 -> pa22 -> pa3a
	//     -> pa21
	pa1.LinkDepend(pa22)
	pa1.LinkDepend(pa21)
	pa22.LinkDepend(pa3)
	pa22.LinkDepend(pa3b)

	// pb1 -> pb2a(pa22) -> pb3
	pb1.LinkDepend(pb2a)
	pb2a.LinkDepend(pb3)

	// pd1 -> pd2 -> pd3
	//     -> pa1|pb1|pb2
	pd1.LinkDepend(pd2)
	pd2.LinkDepend(pd3)
	pd1.LinkDepend(pdor)
	assertExactGraph(t, pkgs, 11)
}
func TestMregePackagesVersionRangeFirst(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgs = append(pkgs, newPackage("p1", "> 0.0.3", "p1"))
	pkgs = append(pkgs, newPackage("p1", "0.0.4", "p1"))
	pkgs = append(pkgs, newPackage("p3", "0.0.4", "p1"))

	assertExactGraph(t, pkgs, 3)
}
func TestSemverRange(t *testing.T) {
	for _, tc := range []struct{ semver, want string }{
		{"~3.4.1", ">= 3.4.1 && < 3.5.0"},
		{"^0.2.3", ">= 0.2.3 && < 0.3.0"},
		{"^0.0.3", ">= 0.0.3 && < 0.0.4"},
		{"^3.4.1", ">= 3.4.1 && < 4.0.0"},
		{"3.4.1", "3.4.1"},
		{"~3.41", "~3.41"},
		{"^3.41", "^3.41"},
		{"~3.4.1a", "~3.4.1a"},
	} {
		t.Run(tc.semver, func(t *testing.T) {
			got := handlerSemverVersionRange(tc.semver)
			if got != tc.want {
				t.Fatalf("error: %s(org): %s(got) vs %s(want)", tc.semver, got, tc.want)
			}
		})
	}
}

func graphEvidence(pkgs []*dxtypes.Package) map[string]map[string]bool {
	label := func(p *dxtypes.Package) string {
		b, _ := json.Marshal([]string{p.Name, p.Version, p.Details().Ecosystem, p.Details().Source, p.Details().Architecture, p.Details().Variant, p.Details().Snapshot, p.Details().ProjectRoot, p.Details().Instance, p.Verification, fmt.Sprint(p.Potential)})
		return string(b)
	}
	result := map[string]map[string]bool{}
	for _, p := range pkgs {
		key := label(p)
		if result[key] == nil {
			result[key] = map[string]bool{}
		}
		for _, up := range p.UpStreamPackages {
			result[key][label(up)] = true
		}
	}
	return result
}
func assertExactGraph(t *testing.T, pkgs []*dxtypes.Package, n int) {
	t.Helper()
	before := graphEvidence(pkgs)
	got := MergePackages(pkgs)
	if len(got) != n {
		t.Fatalf("component count %d want %d", len(got), n)
	}
	if after := graphEvidence(got); !reflect.DeepEqual(before, after) {
		t.Fatalf("lost or invented graph relation: before=%v after=%v", before, after)
	}
	again := MergePackages(got)
	if len(again) != len(got) || !reflect.DeepEqual(before, graphEvidence(again)) {
		t.Fatal("not idempotent")
	}
}
func TestExactMergeSeparatesContexts(t *testing.T) {
	a := &dxtypes.Package{Name: "a", Version: "1", PackageDetails: &dxtypes.PackageDetails{ProjectRoot: "one"}}
	b := &dxtypes.Package{Name: "a", Version: "1", PackageDetails: &dxtypes.PackageDetails{ProjectRoot: "two"}}
	c := &dxtypes.Package{Name: "a", Version: "1", Verification: "hash:other", PackageDetails: &dxtypes.PackageDetails{ProjectRoot: "one"}}
	assertExactGraph(t, []*dxtypes.Package{a, b, c, a}, 3)
}
