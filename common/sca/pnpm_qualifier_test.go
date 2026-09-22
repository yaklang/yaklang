package sca

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestPnpmSpecifierNotLockedVersion(t *testing.T) {
	raw := []byte(`lockfileVersion: 5.4

specifiers:
  lodash: ^4.17.21

dependencies:
  lodash: 4.17.21

packages:

  /lodash/4.17.21:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
`)
	r, err := ScanReport(context.Background(), fstest.MapFS{"pnpm-lock.yaml": {Data: raw}}, WithSnapshotID("pnpm-spec"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	c := mustNamed(t, r, "lodash", "4.17.21")
	if c.Key.Architecture != "" || len(c.Licenses) != 0 {
		t.Fatalf("invented arch/license %+v", c)
	}
	found := false
	for _, q := range r.Requirements {
		if q.Target != "lodash" {
			continue
		}
		found = true
		if q.Constraint != "^4.17.21" {
			t.Fatalf("original specifier lost: %+v", q)
		}
		if q.Constraint == "4.17.21" {
			t.Fatal("locked version used as specifier")
		}
	}
	if !found {
		t.Fatalf("missing lodash specifier: %s", extra5149JSON(t, r.Requirements))
	}
	from := obsIDFor(t, r, c.Key.ID())
	assertSBOMReq(t, r, from, "lodash", "^4.17.21", "", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "lodash" && q.Constraint == "^4.17.21" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestPnpmOptionalAfterDependenciesReport(t *testing.T) {
	lock := `lockfileVersion: 5.4
specifiers:
  app: 1.0.0
dependencies:
  app: 1.0.0
packages:
  /app/1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dependencies:
      foo: 1.0.0
    optionalDependencies:
      bar: 2.0.0
    dev: false
  /foo/1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
  /bar/2.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"pnpm-lock.yaml": {Data: []byte(lock)}}, WithSnapshotID("pnpm-opt"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	app := mustNamed(t, r, "app", "1.0.0")
	foo := mustNamed(t, r, "foo", "1.0.0")
	bar := mustNamed(t, r, "bar", "2.0.0")
	from := obsIDFor(t, r, app.Key.ID())
	var sawFoo, sawBar bool
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		switch {
		case q.Target == "foo" && q.Constraint == "1.0.0":
			sawFoo = true
			if q.Scope != "" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != foo.Key.ID() {
				t.Fatalf("foo %+v", q)
			}
		case q.Target == "bar" && q.Constraint == "2.0.0":
			sawBar = true
			if q.Scope != "optional" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != bar.Key.ID() {
				t.Fatalf("bar %+v", q)
			}
		}
	}
	if !sawFoo || !sawBar {
		t.Fatalf("missing foo/bar: %s", extra5149JSON(t, r.Requirements))
	}
	assertSBOMReq(t, r, from, "bar", "2.0.0", "bar@2.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "bar" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestPnpmV6ImporterDeclarations(t *testing.T) {
	lock := `lockfileVersion: '6.0'
importers:
  .:
    dependencies:
      foo:
        specifier: ^1.0.0
        version: 1.2.0
    optionalDependencies:
      bar:
        specifier: ~2.0.0
        version: 2.0.1
packages:
  /foo@1.2.0:
    resolution: {integrity: ''}
  /bar@2.0.1:
    resolution: {integrity: ''}
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"pnpm-lock.yaml": {Data: []byte(lock)}}, WithSnapshotID("pnpm-imp"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	imp := mustNamed(t, r, ".", "")
	foo := mustNamed(t, r, "foo", "1.2.0")
	bar := mustNamed(t, r, "bar", "2.0.1")
	from := obsIDFor(t, r, imp.Key.ID())
	var sawFoo, sawBar bool
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		switch {
		case q.Target == "foo" && q.Constraint == "^1.0.0":
			sawFoo = true
			if q.Scope != "" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != foo.Key.ID() {
				t.Fatalf("foo %+v", q)
			}
		case q.Target == "bar" && q.Constraint == "~2.0.0":
			sawBar = true
			if q.Scope != "optional" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != bar.Key.ID() {
				t.Fatalf("bar %+v", q)
			}
		}
	}
	if !sawFoo || !sawBar {
		t.Fatalf("importer declarations missing: %s", extra5149JSON(t, r.Requirements))
	}
	assertSBOMReq(t, r, from, "foo", "^1.0.0", "foo@^1.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "foo" {
				return q.Resolved
			}
		}
		return nil
	}())
	assertSBOMReq(t, r, from, "bar", "~2.0.0", "bar@~2.0.0", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "bar" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestPnpmMultiImporterDoesNotMix(t *testing.T) {
	lock := `lockfileVersion: '6.0'
importers:
  .:
    dependencies:
      foo:
        specifier: ^1.0.0
        version: 1.2.0
  packages/app:
    dependencies:
      foo:
        specifier: ^2.0.0
        version: 2.0.0
packages:
  /foo@1.2.0:
    resolution: {integrity: ''}
  /foo@2.0.0:
    resolution: {integrity: ''}
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"pnpm-lock.yaml": {Data: []byte(lock)}}, WithSnapshotID("pnpm-multi"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	root := mustNamed(t, r, ".", "")
	nested := mustNamed(t, r, "packages/app", "")
	foo1 := mustNamed(t, r, "foo", "1.2.0")
	foo2 := mustNamed(t, r, "foo", "2.0.0")
	fromRoot := obsIDFor(t, r, root.Key.ID())
	fromNested := obsIDFor(t, r, nested.Key.ID())
	for _, q := range r.Requirements {
		if q.From == fromRoot && q.Target == "foo" {
			if q.Constraint != "^1.0.0" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != foo1.Key.ID() {
				t.Fatalf("root foo %+v", q)
			}
		}
		if q.From == fromNested && q.Target == "foo" {
			if q.Constraint != "^2.0.0" || len(q.Resolved) != 1 || obsComponent(r, q.Resolved[0]) != foo2.Key.ID() {
				t.Fatalf("nested foo %+v", q)
			}
		}
	}
}
