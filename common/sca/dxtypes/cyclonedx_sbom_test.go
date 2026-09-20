package dxtypes

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/sca/model"
	"testing"
)

func TestCycloneDXGraph(t *testing.T) {
	a := &Package{Name: "a", Version: "1", License: []string{"MIT OR Apache-2.0"}, PackageDetails: &PackageDetails{Ecosystem: "npm"}}
	b := &Package{Name: "b", Version: "2", Verification: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PackageDetails: &PackageDetails{Ecosystem: "npm"}}
	c := &Package{Name: "c", Version: "3", PackageDetails: &PackageDetails{Ecosystem: "npm"}}
	a.LinkDepend(b)
	c.LinkDepend(b)
	b.LinkDepend(a)
	bom := CreateCycloneDXSBOMByDXPackages([]*Package{a, c, a})
	if len(bom.Components) != 3 || len(bom.Dependencies) != 3 {
		t.Fatalf("graph loss: %#v", bom)
	}
	edges := map[string][]string{}
	for _, d := range bom.Dependencies {
		edges[d.Ref] = d.DependsOn
	}
	for _, pair := range [][2]*Package{{a, b}, {b, a}, {c, b}} {
		got := edges[pair[0].Identifier()]
		if len(got) != 1 || got[0] != pair[1].Identifier() {
			t.Fatalf("wrong direction: %#v", edges)
		}
	}
	for _, component := range bom.Components {
		if component.Type != "library" || component.BOMRef == "" {
			t.Fatal("invalid component")
		}
		if component.Name == "a" && (len(component.Licenses) != 1 || component.Licenses[0].Expression != "MIT OR Apache-2.0") {
			t.Fatal("lost expression")
		}
	}
	first, err := MarshalCycloneDXBomToJSON(bom)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalCycloneDXBomToJSON(CreateCycloneDXSBOMByDXPackages([]*Package{c, a}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("unstable output:\n%s\n%s", first, second)
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["specVersion"]) != `"1.5"` {
		t.Fatal("wrong version")
	}
}
func TestExactComponentIdentity(t *testing.T) {
	pkgs := []*Package{{Name: "a b", Version: "c"}, {Name: "a", Version: "b c"}, {Name: "a", Version: "b c", PackageDetails: &PackageDetails{Ecosystem: "npm"}}, {Name: "a", Version: "b c", PackageDetails: &PackageDetails{Ecosystem: "python"}}, {Name: "a", Version: "b c", Verification: "sha256:a"}, {Name: "a", Version: "b c", Verification: "sha256:b"}}
	ids := map[string]bool{}
	for _, p := range pkgs {
		if ids[p.Identifier()] {
			t.Fatal("identity collision")
		}
		ids[p.Identifier()] = true
	}
	if got := len(CreateCycloneDXSBOMByDXPackages(pkgs).Components); got != len(pkgs) {
		t.Fatalf("merged distinct evidence: %d", got)
	}
}
func TestAllOccurrencesContributeEdges(t *testing.T) {
	a1 := &Package{Name: "a", Version: "1"}
	a2 := &Package{Name: "a", Version: "1"}
	b := &Package{Name: "b", Version: "1"}
	a2.LinkDepend(b)
	bom := CreateCycloneDXSBOMByDXPackages([]*Package{a1, a2})
	if len(bom.Components) != 2 {
		t.Fatal("lost component")
	}
	for _, d := range bom.Dependencies {
		if d.Ref == a1.Identifier() && (len(d.DependsOn) != 1 || d.DependsOn[0] != b.Identifier()) {
			t.Fatal("lost duplicate record edge")
		}
	}
}

func TestMixedLicensesAndInvalidDigest(t *testing.T) {
	b := CreateCycloneDXSBOMByDXPackages([]*Package{{Name: "a", License: []string{"MIT OR Apache-2.0", "BSD-3-Clause"}, Verification: "sha256:xyz"}})
	c := b.Components[0]
	if len(c.Hashes) != 0 || len(c.Licenses) != 2 {
		t.Fatalf("%+v", c)
	}
	for _, l := range c.Licenses {
		if l.License == nil || l.Expression != "" {
			t.Fatal("invalid mixed license choices")
		}
	}
}

func TestReportSBOMObservationEdges(t *testing.T) {
	k1 := model.ComponentKey{Ecosystem: "npm", Name: "a", Version: "1"}
	k2 := model.ComponentKey{Ecosystem: "npm", Name: "b", Version: "2"}
	o1 := model.Observation{Component: k1.ID(), Project: "a", NativeID: "node_modules/a"}
	o2 := model.Observation{Component: k1.ID(), Project: "b", NativeID: "node_modules/a"}
	o3 := model.Observation{Component: k2.ID(), Project: "b", NativeID: "node_modules/b"}
	r := &model.Report{Complete: true, Components: []model.Component{{Key: k1}, {Key: k2}}, Observations: []model.Observation{o1, o2, o3}, Requirements: []model.Requirement{{From: o2.ID(), Target: "b", Resolved: []string{o3.ID()}}, {From: o1.ID(), Target: "missing"}}}
	b := CreateCycloneDXSBOMFromReport(r)
	if len(b.Components) != 2 {
		t.Fatal("observations became components")
	}
	for _, d := range b.Dependencies {
		if d.Ref == k1.ID() && (len(d.DependsOn) != 1 || d.DependsOn[0] != k2.ID()) {
			t.Fatal("wrong resolved edges")
		}
	}
}
