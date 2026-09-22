package pnpm

import (
	"bytes"
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParsePnpmOriginalSpecifiers(t *testing.T) {
	reqOf := func(t *testing.T, src string) ([]types.Library, []types.Requirement) {
		t.Helper()
		libs, deps, err := NewParser().Parse(nil, bytes.NewReader([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		var out []types.Requirement
		for _, d := range deps {
			out = append(out, d.Requirements...)
			for _, id := range d.DependsOn {
				if strings.HasPrefix(id, "unresolved-") {
					t.Fatalf("synthetic unresolved %q", id)
				}
			}
		}
		return libs, out
	}

	t.Run("v5-specifier-not-locked-version", func(t *testing.T) {
		src := `lockfileVersion: 5.4
specifiers:
  lodash: ^4.17.21
dependencies:
  lodash: 4.17.21
packages:
  /lodash/4.17.21:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
`
		libs, _ := reqOf(t, src)
		if len(libs) != 1 || libs[0].Name != "lodash" || libs[0].Version != "4.17.21" {
			t.Fatalf("%+v", libs)
		}
		if libs[0].DeclaredVersion != "^4.17.21" {
			t.Fatalf("original specifier lost: %q", libs[0].DeclaredVersion)
		}
		if libs[0].DeclaredVersion == "4.17.21" {
			t.Fatal("locked version used as specifier")
		}
	})

	t.Run("v6-nested-specifier", func(t *testing.T) {
		src := `lockfileVersion: '6.0'
dependencies:
  jquery:
    specifier: ^3.6.0
    version: 3.6.0
packages:
  /jquery@3.6.0:
    resolution: {integrity: sha512-JVzAR/AjBvVt2BmYhxRCSYysDsPcssdmTFnzyLEts9qNwmjmu4JTAMYubEfwVOSwpQ1I1sKKFcxhZCI2buerfw==}
    dev: false
`
		libs, _ := reqOf(t, src)
		if len(libs) != 1 || libs[0].DeclaredVersion != "^3.6.0" || libs[0].Version != "3.6.0" {
			t.Fatalf("%+v", libs)
		}
	})

	t.Run("optional-after-dependencies", func(t *testing.T) {
		src := `lockfileVersion: 5.4
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
		_, qs := reqOf(t, src)
		var foo, bar types.Requirement
		for _, q := range qs {
			switch q.Target {
			case "foo":
				foo = q
			case "bar":
				bar = q
			}
		}
		if foo.Constraint != "1.0.0" || foo.Scope != "" || foo.Resolved == "" {
			t.Fatalf("foo %+v", foo)
		}
		if bar.Constraint != "2.0.0" || bar.Scope != "optional" || bar.Resolved == "" {
			t.Fatalf("bar %+v", bar)
		}
	})

	t.Run("missing-and-empty-and-ambiguous-peer", func(t *testing.T) {
		src := `lockfileVersion: 5.4
specifiers:
  app: 1.0.0
dependencies:
  app: 1.0.0
packages:
  /app/1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dependencies:
      missing: 9.9.9
      foo: 1.0.0
    dev: false
  /foo/1.0.0_a@1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
  /foo/1.0.0_b@1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
    dev: false
`
		libs, qs := reqOf(t, src)
		if len(libs) != 3 {
			t.Fatalf("peer variants collapsed: %+v", libs)
		}
		var missing, foo types.Requirement
		for _, q := range qs {
			switch q.Target {
			case "missing":
				missing = q
			case "foo":
				foo = q
			}
		}
		if missing.Constraint != "9.9.9" || missing.Resolved != "" {
			t.Fatalf("missing %+v", missing)
		}
		if foo.Constraint != "1.0.0" || foo.Resolved != "" {
			t.Fatalf("ambiguous peer uniquely resolved: %+v", foo)
		}
	})

	t.Run("v6-importers-root-optional", func(t *testing.T) {
		src := `lockfileVersion: '6.0'
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
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
  /bar@2.0.1:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
`
		libs, deps, err := NewParser().Parse(nil, bytes.NewReader([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		var imp types.Dependency
		for _, d := range deps {
			if d.ID == "importer:." {
				imp = d
			}
		}
		if imp.ID == "" {
			t.Fatalf("importer declarations missing: %+v", deps)
		}
		got := map[string]types.Requirement{}
		for _, q := range imp.Requirements {
			got[q.Target] = q
		}
		fooID, barID := "", ""
		for _, lib := range libs {
			switch lib.Name {
			case "foo":
				fooID = lib.ID
			case "bar":
				barID = lib.ID
			}
		}
		if got["foo"].Constraint != "^1.0.0" || got["foo"].Scope != "" || got["foo"].Resolved != fooID {
			t.Fatalf("foo %+v want resolved %q", got["foo"], fooID)
		}
		if got["bar"].Constraint != "~2.0.0" || got["bar"].Scope != "optional" || got["bar"].Resolved != barID {
			t.Fatalf("bar %+v want resolved %q", got["bar"], barID)
		}
	})

	t.Run("multi-importer-not-global", func(t *testing.T) {
		src := `lockfileVersion: '6.0'
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
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
  /foo@2.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
`
		_, deps, err := NewParser().Parse(nil, bytes.NewReader([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		var root, nested types.Requirement
		for _, d := range deps {
			for _, q := range d.Requirements {
				if q.Target != "foo" {
					continue
				}
				switch d.ID {
				case "importer:.":
					root = q
				case "importer:packages/app":
					nested = q
				}
			}
		}
		if root.Constraint != "^1.0.0" || nested.Constraint != "^2.0.0" {
			t.Fatalf("importers mixed: root=%+v nested=%+v", root, nested)
		}
		if root.Resolved == nested.Resolved || root.Resolved == "" || nested.Resolved == "" {
			t.Fatalf("lock refs mixed: %q %q", root.Resolved, nested.Resolved)
		}
	})

	t.Run("empty-specifier-and-missing-package", func(t *testing.T) {
		src := `lockfileVersion: '6.0'
importers:
  .:
    dependencies:
      foo:
        specifier: ""
        version: 1.0.0
      missing:
        specifier: workspace:*
        version: link:packages/missing
packages:
  /foo@1.0.0:
    resolution: {integrity: sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==}
`
		_, deps, err := NewParser().Parse(nil, bytes.NewReader([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		var empty, missing types.Requirement
		for _, d := range deps {
			if d.ID != "importer:." {
				continue
			}
			for _, q := range d.Requirements {
				switch q.Target {
				case "foo":
					empty = q
				case "missing":
					missing = q
				}
			}
		}
		if empty.Constraint != "" || empty.Resolved == "" {
			t.Fatalf("empty specifier %+v", empty)
		}
		if missing.Constraint != "workspace:*" || missing.Resolved != "" {
			t.Fatalf("workspace/link must stay unresolved: %+v", missing)
		}
	})

	t.Run("tarball-source-not-architecture", func(t *testing.T) {
		src := `lockfileVersion: '6.0'
dependencies:
  debug:
    specifier: https://github.com/debug-js/debug/tarball/4.3.4
    version: '@github.com/debug-js/debug/tarball/4.3.4'
packages:
  '@github.com/debug-js/debug/tarball/4.3.4':
    resolution: {tarball: https://github.com/debug-js/debug/tarball/4.3.4}
    name: debug
    version: 4.3.4
    dev: false
`
		libs, _ := reqOf(t, src)
		if len(libs) != 1 || libs[0].Name != "debug" || libs[0].Version != "4.3.4" {
			t.Fatalf("%+v", libs)
		}
		if libs[0].Source != "https://github.com/debug-js/debug/tarball/4.3.4" {
			t.Fatalf("tarball source %q", libs[0].Source)
		}
		if libs[0].DeclaredVersion != "https://github.com/debug-js/debug/tarball/4.3.4" {
			t.Fatalf("original specifier %q", libs[0].DeclaredVersion)
		}
	})
}

func TestV9SharedMetadataBudget(t *testing.T) {
	lock := LockFile{Packages: map[string]PackageInfo{"x@1.0.0": {Resolution: PackageResolution{Integrity: strings.Repeat("x", 4096)}}}, Snapshots: map[string]PackageInfo{}}
	for _, peer := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"} {
		lock.Snapshots["x@1.0.0(peer@"+peer+")"] = PackageInfo{}
	}
	limits, _ := (budget.Limits{MaxResultBytes: 200000}).Normalize()
	ctx := budget.Bind(context.Background(), limits)
	if _, err := joinSnapshots(ctx, lock); !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("shared metadata fanout escaped budget: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := joinSnapshots(budget.Ensure(canceled), lock); !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot join ignored cancel: %v", err)
	}
}
