package sca

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func cargoLock(deps string, extra string) string {
	return "version = 3\n[[package]]\nname = \"app\"\nversion = \"1.0.0\"\ndependencies = [\n" + deps + "\n]\n" + extra
}

func cargoPkg(name, ver, src string) string {
	return "[[package]]\nname = \"" + name + "\"\nversion = \"" + ver + "\"\nsource = \"" + src + "\"\n"
}

func scanCargoLock(t *testing.T, body string) *model.Report {
	t.Helper()
	r, err := ScanReport(context.Background(), fstest.MapFS{"Cargo.lock": {Data: []byte(body)}}, WithSnapshotID("cargo-qual"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	return r
}

func cargoAppReqs(t *testing.T, r *model.Report) (from string, reqs []model.Requirement) {
	t.Helper()
	app := mustNamed(t, r, "app", "1.0.0")
	var obs model.Observation
	for _, o := range r.Observations {
		if o.Component == app.Key.ID() {
			obs = o
			break
		}
	}
	if obs.Component == "" {
		t.Fatalf("no observation for app: %s", extra5149JSON(t, r.Observations))
	}
	for _, q := range r.Requirements {
		if q.From == obs.ID() {
			reqs = append(reqs, q)
		}
	}
	return obs.ID(), reqs
}

func TestCargoQualifierReportAndSBOM(t *testing.T) {
	present := "registry+https://present.example/index"
	missing := "registry+https://missing.example/index"
	fooPresent := cargoPkg("foo", "1.0.0", present)

	assertNoSynthetic := func(t *testing.T, reqs []model.Requirement) {
		t.Helper()
		for _, q := range reqs {
			if strings.HasPrefix(q.Target, "unresolved-cargo:") {
				t.Fatalf("synthetic target: %+v", q)
			}
		}
	}

	t.Run("name-only-unique", func(t *testing.T) {
		r := scanCargoLock(t, cargoLock(` "foo" `, fooPresent))
		from, reqs := cargoAppReqs(t, r)
		if len(reqs) != 1 {
			t.Fatalf("got %s", extra5149JSON(t, reqs))
		}
		q := reqs[0]
		if q.From != from || q.Target != "foo" || q.Constraint != "" || q.Condition != "foo" {
			t.Fatalf("declaration: %+v", q)
		}
		if len(q.Resolved) != 1 {
			t.Fatalf("unique name-only must resolve via observation: %+v", q)
		}
		var res model.Observation
		for _, o := range r.Observations {
			if o.ID() == q.Resolved[0] {
				res = o
			}
		}
		foo := mustNamed(t, r, "foo", "1.0.0")
		if res.ID() != q.Resolved[0] || res.Component != foo.Key.ID() {
			t.Fatalf("resolved identity %+v want component %s", res, foo.Key.ID())
		}
		if foo.Key.Version != "1.0.0" || foo.Key.Source != present {
			t.Fatalf("locked package %+v", foo.Key)
		}
		assertSBOMReq(t, r, from, "foo", "", "foo", q.Resolved)
	})

	t.Run("explicit-version", func(t *testing.T) {
		r := scanCargoLock(t, cargoLock(` "foo 1.0.0" `, fooPresent))
		from, reqs := cargoAppReqs(t, r)
		if len(reqs) != 1 || reqs[0].Target != "foo" || reqs[0].Constraint != "1.0.0" || reqs[0].Condition != "foo 1.0.0" || len(reqs[0].Resolved) != 1 {
			t.Fatalf("%s", extra5149JSON(t, reqs))
		}
		assertSBOMReq(t, r, from, "foo", "1.0.0", "foo 1.0.0", reqs[0].Resolved)
	})

	t.Run("explicit-source-match", func(t *testing.T) {
		raw := "foo 1.0.0 (registry+https://present.example/index)"
		r := scanCargoLock(t, cargoLock(" \""+raw+"\" ", fooPresent))
		from, reqs := cargoAppReqs(t, r)
		if len(reqs) != 1 || reqs[0].Condition != raw || reqs[0].Constraint != "1.0.0" || len(reqs[0].Resolved) != 1 {
			t.Fatalf("%s", extra5149JSON(t, reqs))
		}
		foo := mustNamed(t, r, "foo", "1.0.0")
		if foo.Key.Source != present {
			t.Fatalf("source %q", foo.Key.Source)
		}
		assertSBOMReq(t, r, from, "foo", "1.0.0", raw, reqs[0].Resolved)
	})

	t.Run("explicit-source-missing", func(t *testing.T) {
		raw := "foo 1.0.0 (registry+https://missing.example/index)"
		r := scanCargoLock(t, cargoLock(" \""+raw+"\" ", fooPresent))
		from, reqs := cargoAppReqs(t, r)
		assertNoSynthetic(t, r.Requirements)
		if len(reqs) != 1 {
			t.Fatalf("one original dependency became %s", extra5149JSON(t, reqs))
		}
		q := reqs[0]
		if q.From != from || q.Target != "foo" || q.Constraint != "1.0.0" || q.Condition != raw {
			t.Fatalf("original qualifier lost: %+v", q)
		}
		if len(q.Resolved) != 0 {
			t.Fatalf("missing source must not uniquely resolve: %+v", q)
		}
		assertSBOMReq(t, r, from, "foo", "1.0.0", raw, nil)
	})

	t.Run("no-match", func(t *testing.T) {
		r := scanCargoLock(t, cargoLock(` "missing" `, fooPresent))
		from, reqs := cargoAppReqs(t, r)
		assertNoSynthetic(t, r.Requirements)
		if len(reqs) != 1 || reqs[0].Target != "missing" || reqs[0].Constraint != "" || reqs[0].Condition != "missing" || len(reqs[0].Resolved) != 0 {
			t.Fatalf("%s", extra5149JSON(t, reqs))
		}
		assertSBOMReq(t, r, from, "missing", "", "missing", nil)
	})

	t.Run("ambiguous-name", func(t *testing.T) {
		r := scanCargoLock(t, cargoLock(` "foo" `, fooPresent+cargoPkg("foo", "2.0.0", present)))
		from, reqs := cargoAppReqs(t, r)
		assertNoSynthetic(t, r.Requirements)
		if len(reqs) != 1 || reqs[0].Target != "foo" || reqs[0].Constraint != "" || reqs[0].Condition != "foo" || len(reqs[0].Resolved) != 0 {
			t.Fatalf("%s", extra5149JSON(t, reqs))
		}
		assertSBOMReq(t, r, from, "foo", "", "foo", nil)
	})

	t.Run("ambiguous-version-two-sources", func(t *testing.T) {
		r := scanCargoLock(t, cargoLock(` "foo 1.0.0" `, fooPresent+cargoPkg("foo", "1.0.0", missing)))
		from, reqs := cargoAppReqs(t, r)
		assertNoSynthetic(t, r.Requirements)
		if len(reqs) != 1 || reqs[0].Target != "foo" || reqs[0].Constraint != "1.0.0" || reqs[0].Condition != "foo 1.0.0" || len(reqs[0].Resolved) != 0 {
			t.Fatalf("%s", extra5149JSON(t, reqs))
		}
		assertSBOMReq(t, r, from, "foo", "1.0.0", "foo 1.0.0", nil)
	})
}

func assertSBOMReq(t *testing.T, r *model.Report, from, target, constraint, condition string, resolved []string) {
	t.Helper()
	bom := dxtypes.CreateCycloneDXSBOMFromReport(r)
	var reqs []model.Requirement
	for _, p := range bom.Properties {
		if p.Name == "sca:requirements" {
			if err := json.Unmarshal([]byte(p.Value), &reqs); err != nil {
				t.Fatal(err)
			}
		}
	}
	found := 0
	for _, q := range reqs {
		if q.From == from && q.Target == target && q.Constraint == constraint && q.Condition == condition {
			found++
			if len(q.Resolved) != len(resolved) {
				t.Fatalf("SBOM resolved %v want %v in %+v", q.Resolved, resolved, q)
			}
			for i := range resolved {
				if q.Resolved[i] != resolved[i] {
					t.Fatalf("SBOM resolved[%d]=%q want %q", i, q.Resolved[i], resolved[i])
				}
			}
		}
	}
	if found != 1 {
		t.Fatalf("SBOM want 1 associated requirement from=%s target=%s constraint=%q condition=%q, got %d in %s", from, target, constraint, condition, found, extra5149JSON(t, reqs))
	}
}
