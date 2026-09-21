package sca

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/model"
)

func TestYarnQualifierReportAndSBOM(t *testing.T) {
	lock := `# yarn lockfile v1

js-tokens@^2.0.0:
  version "2.0.0"
  resolved "https://registry.yarnpkg.com/js-tokens/-/js-tokens-2.0.0.tgz#79903f5563ee778cc1162e6dcf1a0027c97f9cb5"
  integrity sha512-kk4RjoztHFeAlCNPdTr/uF04oN5z53L18LJfADBQ7YHkO3iR2rP2ZwOH0n33r2FRw0psRmjTDEkOWPE//zmujg==

"js-tokens@^3.0.0 || ^4.0.0":
  version "4.0.0"
  resolved "https://registry.yarnpkg.com/js-tokens/-/js-tokens-4.0.0.tgz#19203fb59991df98e3a287050d4647cdeaf32499"
  integrity sha512-RdJUflcE3cUzKiMqQgsCu06FPu9UdIJO0beYbPhHN4k6apgJtifcoCtT9bcxOpYBtpD2kCM6Sbzg4CausW/PKQ==

loose-envify@^1.1.0, loose-envify@^1.4.0:
  version "1.4.0"
  resolved "https://registry.yarnpkg.com/loose-envify/-/loose-envify-1.4.0.tgz#71ee51fa7be4caec1a63839f7e682d8132d30caf"
  integrity sha512-lyuxPGr/Wfhrlem2CL/UcnUc1zcqKAImBDzukY7Y5F/yQiNdko6+fRLevlw1HgMySw7f611UIY408EtxRSoK3Q==
  dependencies:
    js-tokens "^3.0.0 || ^4.0.0"
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"yarn.lock": {Data: []byte(lock)}}, WithSnapshotID("yarn-qual"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	loose := mustNamed(t, r, "loose-envify", "1.4.0")
	js4 := mustNamed(t, r, "js-tokens", "4.0.0")
	js2 := mustNamed(t, r, "js-tokens", "2.0.0")
	var from string
	for _, o := range r.Observations {
		if o.Component == loose.Key.ID() {
			from = o.ID()
		}
	}
	if from == "" {
		t.Fatal("no loose-envify observation")
	}
	n := 0
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		n++
		if q.Target != "js-tokens" {
			t.Fatalf("Target is native id: %+v", q)
		}
		if q.Constraint != "^3.0.0 || ^4.0.0" {
			t.Fatalf("original range lost or replaced with lock version: %+v", q)
		}
		if q.Condition != "js-tokens@^3.0.0 || ^4.0.0" {
			t.Fatalf("raw descriptor: %+v", q)
		}
		if len(q.Resolved) != 1 {
			t.Fatalf("resolved %v", q.Resolved)
		}
		var resComp string
		for _, o := range r.Observations {
			if o.ID() == q.Resolved[0] {
				resComp = o.Component
			}
		}
		if resComp != js4.Key.ID() {
			t.Fatalf("resolved to %s want js-tokens@4.0.0 %s not 2.0.0 %s", resComp, js4.Key.ID(), js2.Key.ID())
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 original dep, got %d %s", n, extra5149JSON(t, r.Requirements))
	}
	assertSBOMReq(t, r, from, "js-tokens", "^3.0.0 || ^4.0.0", "js-tokens@^3.0.0 || ^4.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == from {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestYarnMissingDescriptorDoesNotPickByOrder(t *testing.T) {
	lock := `# yarn lockfile v1

app@1.0.0:
  version "1.0.0"
  dependencies:
    js-tokens "^3.0.0 || ^4.0.0"

js-tokens@^2.0.0:
  version "2.0.0"
  resolved "https://registry.yarnpkg.com/js-tokens/-/js-tokens-2.0.0.tgz#79903f5563ee778cc1162e6dcf1a0027c97f9cb5"
  integrity sha512-kk4RjoztHFeAlCNPdTr/uF04oN5z53L18LJfADBQ7YHkO3iR2rP2ZwOH0n33r2FRw0psRmjTDEkOWPE//zmujg==
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"yarn.lock": {Data: []byte(lock)}}, WithSnapshotID("yarn-miss"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	app := mustNamed(t, r, "app", "1.0.0")
	var from string
	for _, o := range r.Observations {
		if o.Component == app.Key.ID() {
			from = o.ID()
		}
	}
	n := 0
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		n++
		if q.Target != "js-tokens" || q.Constraint != "^3.0.0 || ^4.0.0" || len(q.Resolved) != 0 {
			t.Fatalf("must not uniquely resolve by scan order: %+v", q)
		}
		if strings.HasPrefix(q.Target, "[") || strings.HasPrefix(q.Target, "unresolved-yarn:") {
			t.Fatalf("native or synthetic target: %+v", q)
		}
	}
	if n != 1 {
		t.Fatalf("duplicate or missing original: %s", extra5149JSON(t, r.Requirements))
	}
}

func TestYarnOptionalAfterDependenciesReport(t *testing.T) {
	lock := `# yarn lockfile v1

app@1.0.0:
  version "1.0.0"
  dependencies:
    foo "^1.0.0"
  optionalDependencies:
    bar "^2.0.0"

foo@^1.0.0:
  version "1.0.0"
  resolved "https://example/foo-1.0.0.tgz"

bar@^2.0.0:
  version "2.0.0"
  resolved "https://example/bar-2.0.0.tgz"
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"yarn.lock": {Data: []byte(lock)}}, WithSnapshotID("yarn-opt"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	app := mustNamed(t, r, "app", "1.0.0")
	foo := mustNamed(t, r, "foo", "1.0.0")
	bar := mustNamed(t, r, "bar", "2.0.0")
	var from string
	for _, o := range r.Observations {
		if o.Component == app.Key.ID() {
			from = o.ID()
		}
	}
	var sawFoo, sawBar bool
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		if strings.HasPrefix(q.Target, "[") || strings.HasPrefix(q.Target, "unresolved-yarn:") {
			t.Fatalf("native or synthetic %+v", q)
		}
		switch {
		case q.Target == "foo" && q.Constraint == "^1.0.0":
			sawFoo = true
			if q.Scope != "" || q.Condition != "foo@^1.0.0" || len(q.Resolved) != 1 {
				t.Fatalf("foo %+v", q)
			}
			if obsComponent(r, q.Resolved[0]) != foo.Key.ID() {
				t.Fatalf("foo resolved to %s", obsComponent(r, q.Resolved[0]))
			}
		case q.Target == "bar" && q.Constraint == "^2.0.0":
			sawBar = true
			if q.Scope != "optional" || q.Condition != "bar@^2.0.0" || len(q.Resolved) != 1 {
				t.Fatalf("bar %+v", q)
			}
			if obsComponent(r, q.Resolved[0]) != bar.Key.ID() {
				t.Fatalf("bar resolved to %s", obsComponent(r, q.Resolved[0]))
			}
		default:
			t.Fatalf("unexpected %+v", q)
		}
	}
	if !sawFoo || !sawBar {
		t.Fatalf("missing foo/bar: %s", extra5149JSON(t, r.Requirements))
	}
	assertSBOMReq(t, r, from, "bar", "^2.0.0", "bar@^2.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "bar" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestYarnBerryFrozenProtocolRanges(t *testing.T) {
	raw, err := os.ReadFile("testdata/node_yarn/positive_protocol/yarn.lock")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ScanReport(context.Background(), fstest.MapFS{"yarn.lock": {Data: raw}}, WithSnapshotID("yarn-berry"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	for _, name := range []string{"c", "package1", "package2", "util1", "yarn-workspace-test"} {
		for _, c := range r.Components {
			if c.Key.Name == name {
				t.Fatalf("workspace protocol package leaked: %+v", c.Key)
			}
		}
	}
	js4 := mustNamed(t, r, "js-tokens", "4.0.0")
	js8 := mustNamed(t, r, "js-tokens", "8.0.1")
	loose := mustNamed(t, r, "loose-envify", "1.4.0")
	odd := mustNamed(t, r, "is-odd", "3.0.1")
	n6 := mustNamed(t, r, "is-number", "6.0.0")
	n7 := mustNamed(t, r, "is-number", "7.0.0")
	if loose.Key.Architecture != "" || len(loose.Licenses) != 0 {
		t.Fatalf("berry lock has no arch/license: %+v", loose)
	}
	if loose.Key.Verification != "" {
		t.Fatalf("berry checksum is not the classic integrity field: %q", loose.Key.Verification)
	}
	if loose.Key.Source != "" {
		t.Fatalf("berry resolution descriptor is not a URL source: %q", loose.Key.Source)
	}
	fromLoose := obsIDFor(t, r, loose.Key.ID())
	fromOdd := obsIDFor(t, r, odd.Key.ID())
	n := 0
	for _, q := range r.Requirements {
		if q.From != fromLoose {
			continue
		}
		n++
		if q.Target != "js-tokens" || q.Constraint != "^3.0.0 || ^4.0.0" || q.Condition != "js-tokens@^3.0.0 || ^4.0.0" {
			t.Fatalf("berry original range lost: %+v", q)
		}
		if len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != js4.Key.ID() {
			t.Fatalf("berry resolved %v want js-tokens@4.0.0 not 8.0.1 %s", q.Resolved, js8.Key.ID())
		}
	}
	if n != 1 {
		t.Fatalf("loose-envify deps %d: %s", n, extra5149JSON(t, r.Requirements))
	}
	n = 0
	for _, q := range r.Requirements {
		if q.From != fromOdd {
			continue
		}
		n++
		if q.Target != "is-number" || q.Constraint != "^6.0.0" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != n6.Key.ID() {
			t.Fatalf("must resolve is-number@^6.0.0 to 6.0.0 not 7.0.0: %+v want %s not %s", q, n6.Key.ID(), n7.Key.ID())
		}
	}
	if n != 1 {
		t.Fatalf("is-odd deps %d", n)
	}
	assertSBOMReq(t, r, fromLoose, "js-tokens", "^3.0.0 || ^4.0.0", "js-tokens@^3.0.0 || ^4.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == fromLoose && q.Target == "js-tokens" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func obsIDFor(t *testing.T, r *model.Report, component string) string {
	t.Helper()
	for _, o := range r.Observations {
		if o.Component == component {
			return o.ID()
		}
	}
	t.Fatalf("no observation for %s", component)
	return ""
}

func obsComponent(r *model.Report, id string) string {
	for _, o := range r.Observations {
		if o.ID() == id {
			return o.Component
		}
	}
	return ""
}
