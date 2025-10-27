package analyzer

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"golang.org/x/exp/slices"
)

var pkgMaps = make(map[string][]*dxtypes.Package)

func newPackage(name, version, prefix string) *dxtypes.Package {
	p := &dxtypes.Package{
		Name:               name,
		Version:            version,
		IsVersionRange:     strings.ContainsAny(version, "<>="),
		FromFile:           []string{fmt.Sprintf("/path/%s/file", prefix)},
		FromAnalyzer:       []string{fmt.Sprintf("%s-analyzer", prefix)},
		Verification:       "",
		License:            nil,
		UpStreamPackages:   make(map[string]*dxtypes.Package),
		DownStreamPackages: make(map[string]*dxtypes.Package),
	}
	//pkgs = append(pkgs, p)
	list, ok := pkgMaps[prefix]
	if !ok {
		list = make([]*dxtypes.Package, 0)
	}
	list = append(list, p)
	pkgMaps[prefix] = list
	return p
}

// func ShowDot(pkgs []*dxtypes.Package) {
// 	sort.SliceStable(pkgs, func(i, j int) bool {
// 		return pkgs[i].Name+pkgs[i].Version < pkgs[j].Name+pkgs[j].Version
// 	})
// 	for _, pkg := range pkgs {
// 		upstream := lo.MapToSlice(pkg.UpStreamPackages, func(_ string, p *dxtypes.Package) string {
// 			return p.Name + "-" + p.Version
// 		})
// 		sort.Strings(upstream)
// 		downstream := lo.MapToSlice(pkg.DownStreamPackages, func(_ string, p *dxtypes.Package) string {
// 			return p.Name + "-" + p.Version
// 		})
// 		sort.Strings(downstream)
// 		fmt.Printf(`
// 		{
// 	 		ID: "%s-%s",
// 	 		UpStream: %#v,
// 	 		DownStream: %#v,
// 	 	},
// 		`, pkg.Name, pkg.Version, upstream, downstream,
// 		)
// 	}
// }

type testPackage struct {
	ID         string
	DownStream []string // name + version
	UpStream   []string // name + version
}

func Check(t *testing.T, packages []*dxtypes.Package, want []*testPackage) {
	pkgs := CoverPackageToPkg(packages)
	if len(pkgs) != len(want) {
		t.Fatalf("%s: pkgs length error: %d(got) != %d(want)", t.Name(), len(pkgs), len(want))
	}
	for i := 0; i < len(pkgs); i++ {

		if pkgs[i].ID != want[i].ID {
			t.Fatalf("%s: pkgs %d(%s) ID error: %s(got) != %s(want)", t.Name(), i, pkgs[i].ID, pkgs[i].ID, want[i].ID)
		}

		if slices.Compare(pkgs[i].DownStream, want[i].DownStream) != 0 {
			t.Fatalf("%s: pkgs %d(%s) DownStream error: %#v(got) != %#v(want)", t.Name(), i, pkgs[i].ID, pkgs[i].DownStream, want[i].DownStream)
		}
		if slices.Compare(pkgs[i].UpStream, want[i].UpStream) != 0 {
			t.Fatalf("%s: pkgs %d(%s) UpStream error: %#v(got) != %#v(want)", t.Name(), i, pkgs[i].ID, pkgs[i].UpStream, want[i].UpStream)
		}
	}

}

func CoverPackageToPkg(packages []*dxtypes.Package) []*testPackage {
	pkgs := make([]*testPackage, 0)
	sort.SliceStable(packages, func(i, j int) bool {
		return packages[i].Name+packages[i].Version < packages[j].Name+packages[j].Version
	})
	for _, pkg := range packages {
		upstream := lo.MapToSlice(pkg.UpStreamPackages, func(_ string, p *dxtypes.Package) string {
			return p.Name + "-" + p.Version
		})
		sort.Strings(upstream)
		downstream := lo.MapToSlice(pkg.DownStreamPackages, func(_ string, p *dxtypes.Package) string {
			return p.Name + "-" + p.Version
		})
		sort.Strings(downstream)
		p := &testPackage{
			ID:         pkg.Name + "-" + pkg.Version,
			DownStream: downstream,
			UpStream:   upstream,
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

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
	// DrawPackagesDOT(pkgs)

	ret := MergePackages(pkgs)
	wantPkg := []*testPackage{

		{
			ID:         "pa1-0.0.3",
			UpStream:   []string{"pa21-0.0.3", "pa22-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pa21-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa1-0.0.3"},
		},

		{
			ID:         "pa22-0.0.3",
			UpStream:   []string{"pa3-0.0.3", "pb3-0.0.2", "pb3-0.0.3"},
			DownStream: []string{"pa1-0.0.3", "pb1-0.0.3"},
		},

		{
			ID:         "pa3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb1-0.0.3",
			UpStream:   []string{"pa22-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pb3-0.0.2",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},
	}
	// DrawPackagesDOT(ret)
	// ShowDot(ret)
	Check(t, ret, wantPkg)
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

	// DrawPackagesDOT(pkgs, "org.png")
	ret := MergePackages(pkgs)
	wantPkg := []*testPackage{

		{
			ID:         "pa1-0.0.3",
			UpStream:   []string{"pa21-0.0.3", "pa22-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pa21-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa1-0.0.3"},
		},

		{
			ID:         "pa22-0.0.3",
			UpStream:   []string{"pa3-0.0.3", "pa3->0.0.3", "pb3-0.0.2"},
			DownStream: []string{"pa1-0.0.3", "pe1-0.0.3"},
		},

		{
			ID:         "pa3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pa3->0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb3-0.0.2",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pe1-0.0.3",
			UpStream:   []string{"pa22-0.0.3"},
			DownStream: []string{},
		},
	}
	// ShowDot(ret)
	Check(t, ret, wantPkg)
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
	// DrawPackagesDOT(pkgs, "org.png")
	ret := MergePackages(pkgs)
	// _ = ret
	// DrawPackagesDOT(ret, "ret.png")
	wantPkg := []*testPackage{
		{
			ID:         "pa1-0.0.3",
			UpStream:   []string{"pa21-0.0.3", "pa22-0.0.3"},
			DownStream: []string{"pc1-0.0.3"},
		},

		{
			ID:         "pa21-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa1-0.0.3"},
		},

		{
			ID:         "pa22-0.0.3",
			UpStream:   []string{"pa3-0.0.3", "pb3-0.0.2", "pb3-0.0.3"},
			DownStream: []string{"pa1-0.0.3", "pb1-0.0.3"},
		},

		{
			ID:         "pa3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb1-0.0.3",
			UpStream:   []string{"pa22-0.0.3"},
			DownStream: []string{"pc1-0.0.3"},
		},

		{
			ID:         "pb3-0.0.2",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pc1-0.0.3",
			UpStream:   []string{"pa1-0.0.3", "pb1-0.0.3", "pc2-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pc2-0.0.3",
			UpStream:   []string{"pc3-0.0.3"},
			DownStream: []string{"pc1-0.0.3"},
		},

		{
			ID:         "pc3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pc2-0.0.3"},
		},
	}
	// ShowDot(ret)
	Check(t, ret, wantPkg)
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
	// DrawPackagesDOT(pkgs)
	ret := MergePackages(pkgs)
	// DrawPackagesDOT(ret)
	// ShowDot(ret)
	wantPkg := []*testPackage{
		{
			ID:         "pa1-0.0.3",
			UpStream:   []string{"pa21-0.0.3", "pa22-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pa21-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa1-0.0.3"},
		},

		{
			ID:         "pa22-0.0.3",
			UpStream:   []string{"pa3-0.0.3", "pb3-0.0.2", "pb3-0.0.3"},
			DownStream: []string{"pa1-0.0.3", "pb1-0.0.3"},
		},

		{
			ID:         "pa3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb1-0.0.3",
			UpStream:   []string{"pa22-0.0.3"},
			DownStream: []string{},
		},

		{
			ID:         "pb3-0.0.2",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pd1-0.0.3",
			UpStream:   []string{"pd2-0.0.5", "pe1|pf1|pg2-0.0.2|0.0.3|0.0.4"},
			DownStream: []string{},
		},

		{
			ID:         "pd2-0.0.5",
			UpStream:   []string{"pd3-0.0.3"},
			DownStream: []string{"pd1-0.0.3"},
		},

		{
			ID:         "pd3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pd2-0.0.5"},
		},
		{
			ID:         "pe1|pf1|pg2-0.0.2|0.0.3|0.0.4",
			UpStream:   []string{},
			DownStream: []string{"pd1-0.0.3"},
		},
	}
	Check(t, ret, wantPkg)

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
	// DrawPackagesDOT(pkgs)
	ret := MergePackages(pkgs)
	// DrawPackagesDOT(ret)
	// ShowDot(ret)
	wantPkg := []*testPackage{
		{
			ID:         "pa1-0.0.3",
			UpStream:   []string{"pa21-0.0.3", "pa22-0.0.3"},
			DownStream: []string{"pd1-0.0.3"},
		},

		{
			ID:         "pa21-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa1-0.0.3"},
		},

		{
			ID:         "pa22-0.0.3",
			UpStream:   []string{"pa3-0.0.3", "pb3-0.0.2", "pb3-0.0.3"},
			DownStream: []string{"pa1-0.0.3", "pb1-0.0.3"},
		},

		{
			ID:         "pa3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb1-0.0.3",
			UpStream:   []string{"pa22-0.0.3"},
			DownStream: []string{"pd1-0.0.3"},
		},

		{
			ID:         "pb3-0.0.2",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pb3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pa22-0.0.3"},
		},

		{
			ID:         "pd1-0.0.3",
			UpStream:   []string{"pa1-0.0.3", "pb1-0.0.3", "pd2-0.0.5"},
			DownStream: []string{},
		},

		{
			ID:         "pd2-0.0.5",
			UpStream:   []string{"pd3-0.0.3"},
			DownStream: []string{"pd1-0.0.3"},
		},

		{
			ID:         "pd3-0.0.3",
			UpStream:   []string{},
			DownStream: []string{"pd2-0.0.5"},
		},
	}
	Check(t, ret, wantPkg)
}

func TestMregePackagesVersionRangeFirst(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgs = append(pkgs, newPackage("p1", "> 0.0.3", "p1"))
	pkgs = append(pkgs, newPackage("p1", "0.0.4", "p1"))
	pkgs = append(pkgs, newPackage("p3", "0.0.4", "p1"))

	ret := MergePackages(pkgs)
	wantPkg := []*testPackage{
		{
			ID: "p1-0.0.4",
		},
		{
			ID: "p3-0.0.4",
		},
	}
	Check(t, ret, wantPkg)
}

func TestSemverRange(t *testing.T) {
	check := func(semver, want string) {
		got := handlerSemverVersionRange(semver)
		if got != want {
			t.Fatalf("error: %s(org): %s(got) vs %s(want)", semver, got, want)
		}
	}

	check("~3.4.1", ">= 3.4.1 && < 3.5.0")
	check("^3.4.1", ">= 3.4.1 && < 4.0.0")
	check("3.4.1", "3.4.1")
	check("~3.41", "~3.41")
	check("^3.41", "^3.41")
	check("~3.4.1a", "~3.4.1a")
}
