package sca

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

type rawRPMDep struct {
	Name, Version string
	Flags         uint32
}
type rawRPMRecord struct {
	Name, Version, Release, Architecture, License, MD5 string
	Epoch                                              uint32
	Provides, Requires                                 []rawRPMDep
}

func rpmOracleConstraint(d rawRPMDep) string {
	// RPM comparison bits: less=2, greater=4, equal=8. Other flag bits remain
	// evidence in Condition and must not turn into a version operator.
	ops := map[uint32]string{0: "", 2: "<", 4: ">", 6: "<>", 8: "=", 10: "<=", 12: ">=", 14: "<>="}
	return strings.TrimSpace(ops[d.Flags&14] + " " + d.Version)
}
func sortedRPMSet(ss []string) []string {
	set := map[string]bool{}
	for _, s := range ss {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestRPMAllFrozenFields(t *testing.T) {
	var oracle struct {
		SourceSHA256 string `json:"source_sha256"`
		Packages     []rawRPMRecord
	}
	raw, err := os.ReadFile("testdata/full_field/rpm-sqlite-raw-fields.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile("testdata/rpm/rpmdb.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(snapshot)
	if oracle.SourceSHA256 != hex.EncodeToString(sum[:]) || len(oracle.Packages) != 129 {
		t.Fatal("raw oracle provenance/count mismatch")
	}
	r := scanNamed(t, "rpm-all", map[string]string{"var/lib/rpm/rpmdb.sqlite": "testdata/rpm/rpmdb.sqlite"})
	comps, _, byComp := reportIndex(r)
	bom, sbomReqs, sbomObs := parseSBOM(t, r)
	if len(comps) != 129 || len(r.Observations) != 129 || len(bom.Components) != 129 {
		t.Fatalf("inventory: %d/%d/%d", len(comps), len(r.Observations), len(bom.Components))
	}
	if extra5149JSON(t, sbomReqs) != extra5149JSON(t, r.Requirements) || extra5149JSON(t, sbomObs) != extra5149JSON(t, r.Observations) {
		t.Fatal("SBOM dropped requirement/observation fields")
	}
	old := map[string]oldScanRec{}
	for _, p := range loadOldScan(t, "rpm") {
		if !p.Potential {
			old[p.Name+"@"+p.Version] = p
		}
	}
	bomBy := map[string]dxtypes.BOMComponent{}
	for _, c := range bom.Components {
		bomBy[c.BOMRef] = c
	}
	providers := map[string][]string{}
	for _, p := range oracle.Packages {
		c := comps[p.Name+"@"+p.Version]
		want := model.ComponentKey{Ecosystem: "rpm", Name: p.Name, Version: p.Version, Architecture: p.Architecture, Variant: fmt.Sprintf("epoch=%d;release=%s", p.Epoch, p.Release), Verification: "md5:" + p.MD5}
		if c.Key != want || !reflect.DeepEqual(c.Licenses, []string{p.License}) {
			t.Fatalf("%s full component mismatch: got=%+v want=%+v license=%q", p.Name, c, want, p.License)
		}
		prev, ok := old[p.Name+"@"+p.Version]
		if !ok || prev.Verification != want.Verification || !reflect.DeepEqual(prev.FromFile, []string{"var/lib/rpm/rpmdb.sqlite"}) || !reflect.DeepEqual(prev.FromAnalyzer, []string{"rpm-pkg"}) {
			t.Fatalf("%s old comparable fields: %+v", p.Name, prev)
		}
		observations := byComp[c.Key.ID()]
		if len(observations) != 1 {
			t.Fatalf("%s observations=%d", p.Name, len(observations))
		}
		o := observations[0]
		if o.Kind != "installed" || o.File != "var/lib/rpm/rpmdb.sqlite" || o.StartLine != 0 || o.EndLine != 0 || o.Scope != "" || o.Condition != "" || o.DeclaredIntegrity != "" {
			t.Fatalf("%s observation: %+v", p.Name, o)
		}
		expected := []string{}
		providers[p.Name] = append(providers[p.Name], o.ID())
		for _, d := range p.Provides {
			v := d.Name
			if constraint := rpmOracleConstraint(d); constraint != "" {
				v += " " + constraint
			}
			expected = append(expected, v)
			providers[d.Name] = append(providers[d.Name], o.ID())
		}
		if !reflect.DeepEqual(sortedRPMSet(o.Provides), sortedRPMSet(expected)) {
			t.Fatalf("%s provides differ", p.Name)
		}
		b := bomBy[c.Key.ID()]
		if b.Name != p.Name || b.Version != p.Version || len(b.Hashes) != 1 || b.Hashes[0].Algorithm != "MD5" || b.Hashes[0].Value != p.MD5 {
			t.Fatalf("%s SBOM fields/hash: %+v", p.Name, b)
		}
		props := map[string]string{}
		for _, v := range b.Properties {
			props[v.Name] = v.Value
		}
		if props["sca:ecosystem"] != "rpm" || props["sca:architecture"] != p.Architecture || props["sca:variant"] != want.Variant {
			t.Fatalf("%s SBOM identity properties: %v", p.Name, props)
		}
	}
	type reqKey struct{ From, Target, Constraint, Condition string }
	expected := map[reqKey]bool{}
	for _, p := range oracle.Packages {
		c := comps[p.Name+"@"+p.Version]
		o := byComp[c.Key.ID()][0]
		for _, d := range p.Requires {
			expected[reqKey{o.ID(), d.Name, rpmOracleConstraint(d), fmt.Sprintf("rpmflags=%d", d.Flags)}] = true
		}
	}
	if len(r.Requirements) != len(expected) {
		t.Fatalf("requirements count new=%d raw unique=%d", len(r.Requirements), len(expected))
	}
	for _, q := range r.Requirements {
		key := reqKey{q.From, q.Target, q.Constraint, q.Condition}
		if !expected[key] || q.Operator != "and" || q.Scope != "" || q.Group != "" || len(q.Resolved) != 0 {
			t.Fatalf("unexpected or changed requirement: %+v", q)
		}
		if !reflect.DeepEqual(sortedRPMSet(q.Candidates), sortedRPMSet(providers[q.Target])) {
			t.Fatalf("provider candidates for %s: got=%v want=%v", q.Target, q.Candidates, providers[q.Target])
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("lost %d requirements", len(expected))
	}
	t.Logf("validated all 129 packages and %d unique requirements against SQLite raw headers and old comparable fields", len(r.Requirements))
}
