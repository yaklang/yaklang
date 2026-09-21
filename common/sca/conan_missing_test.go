package sca

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/model"
)

func TestConanMissingNodeReportAndSBOM(t *testing.T) {
	scan := func(child string) (*model.Report, error) {
		t.Helper()
		body := `{"graph_lock":{"nodes":{"0":{"requires":["1"]},"1":{"ref":"app/1","requires":["2"]},` + child + `}},"version":"0.4"}`
		return ScanReport(context.Background(), fstest.MapFS{"conan.lock": &fstest.MapFile{Data: []byte(body)}}, WithSnapshotID("conan-missing"))
	}
	r, err := scan(`"2":{"ref":"zlib/1.2.12"}`)
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("valid child complete=%v err=%v", r != nil && r.Complete, err)
	}
	comps, obsByID, byComp := reportIndex(r)
	app := comps["app@1"]
	zlib := comps["zlib@1.2.12"]
	if app.Key.Name == "" || zlib.Key.Name == "" {
		t.Fatalf("valid identities: %s", extra5149JSON(t, r.Components))
	}
	from := byComp[app.Key.ID()][0].ID()
	q := requireEdge(t, r, from, "zlib", "1.2.12", "", "zlib/1.2.12")
	if len(q.Resolved) != 1 || obsByID[q.Resolved[0]].Component != zlib.Key.ID() {
		t.Fatalf("resolved %+v", q)
	}
	for _, q := range r.Requirements {
		if q.Target == "2" {
			t.Fatalf("native id as name: %+v", q)
		}
	}
	_, sbomReqs, _ := parseSBOM(t, r)
	ok := false
	for _, q := range sbomReqs {
		if q.From == from && q.Target == "zlib" && q.Constraint == "1.2.12" && len(q.Resolved) == 1 {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("SBOM lost zlib: %s", extra5149JSON(t, sbomReqs))
	}

	for _, child := range []string{`"2":{"ref":""}`, `"3":{"ref":"unused/1"}`} {
		r, err = scan(child)
		if r == nil {
			t.Fatal(err)
		}
		if r.Complete || err == nil {
			t.Fatalf("%s treated as complete: complete=%v err=%v", child, r.Complete, err)
		}
		if hasName(r, "zlib") || hasName(r, "2") {
			t.Fatalf("%s invented package: %s", child, extra5149JSON(t, r.Components))
		}
		if !hasName(r, "app") {
			t.Fatalf("%s dropped app: %s", child, extra5149JSON(t, r.Components))
		}
		unresolved, named2 := 0, 0
		for _, q := range r.Requirements {
			if q.Target == "2" {
				named2++
			}
			if q.Condition == "2" && q.Target == "" && len(q.Resolved) == 0 {
				unresolved++
			}
		}
		if named2 != 0 || unresolved != 1 {
			t.Fatalf("%s reqs named2=%d unresolved=%d %s", child, named2, unresolved, extra5149JSON(t, r.Requirements))
		}
		found := false
		for _, d := range r.Diagnostics {
			if d.Code == "evidence_insufficient" && d.Incomplete {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s diagnostics: %s", child, extra5149JSON(t, r.Diagnostics))
		}
		bom, sbomReqs, _ := parseSBOM(t, r)
		complete := false
		for _, p := range bom.Properties {
			if p.Name == "sca:complete" && p.Value == "true" {
				complete = true
			}
		}
		if complete {
			t.Fatalf("%s SBOM complete=true", child)
		}
		ok = false
		for _, q := range sbomReqs {
			if q.Condition == "2" && q.Target == "" {
				ok = true
			}
			if q.Target == "2" {
				t.Fatalf("%s SBOM native id as name: %+v", child, q)
			}
		}
		if !ok {
			t.Fatalf("%s SBOM lost raw node: %s", child, extra5149JSON(t, sbomReqs))
		}
	}

	r, err = scan(`"2":{"ref":"zlib/1.2.12"}`)
	if r == nil || !r.Complete || err != nil {
		t.Fatalf("retry valid: complete=%v err=%v", r != nil && r.Complete, err)
	}
}
