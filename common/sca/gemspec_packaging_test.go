package sca

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/model"
)

func TestGemspecRuntimeAndDevReport(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0.0"
  s.homepage = "https://example.test/app"
  s.add_runtime_dependency(%q<foo>.freeze, [">= 1.0", "< 2.0"])
  s.add_runtime_dependency "empty"
  s.add_development_dependency(%q<bar>.freeze, [">= 0"])
  s.add_runtime_dependency(%q<foo>.freeze, ["~> 3.0"])
end
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"gems/specifications/app.gemspec": {Data: []byte(src)}}, WithSnapshotID("gemspec-req"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	c := mustNamed(t, r, "app", "1.0.0")
	if c.Key.Source != "https://example.test/app" || c.Key.Architecture != "" || c.Key.Verification != "" {
		t.Fatalf("%+v", c)
	}
	from := obsIDFor(t, r, c.Key.ID())
	var fooMulti, fooTilde, empty, bar bool
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		if q.Target == "foo" && q.Constraint == ">= 1.0, < 2.0" && q.Scope == "" {
			fooMulti = true
			assertSBOMReq(t, r, from, "foo", ">= 1.0, < 2.0", "foo@>= 1.0, < 2.0", nil)
		}
		if q.Target == "foo" && q.Constraint == "~> 3.0" {
			fooTilde = true
		}
		if q.Target == "empty" && q.Constraint == "" && q.Condition == "empty" {
			empty = true
		}
		if q.Target == "bar" && q.Scope == "dev" && q.Constraint == ">= 0" {
			bar = true
			assertSBOMReq(t, r, from, "bar", ">= 0", "bar@>= 0", nil)
		}
	}
	if !fooMulti || !fooTilde || !empty || !bar {
		t.Fatalf("original gemspec deps lost: %s", extra5149JSON(t, r.Requirements))
	}
}

func TestGemspecVarargsRangesReport(t *testing.T) {
	scan := func(t *testing.T, line string) (*model.Report, error) {
		t.Helper()
		src := "Gem::Specification.new do |s|\n s.name = \"app\"\n s.version = \"1.0.0\"\n " + line + "\nend\n"
		return ScanReport(context.Background(), fstest.MapFS{"gems/specifications/app.gemspec": {Data: []byte(src)}}, WithSnapshotID("gemspec-varargs"))
	}
	r, err := scan(t, `s.add_dependency "rack", ">= 1", "< 2"`)
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("varargs complete=%v err=%v", r != nil && r.Complete, err)
	}
	c := mustNamed(t, r, "app", "1.0.0")
	from := obsIDFor(t, r, c.Key.ID())
	found := false
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "rack" && q.Constraint == ">= 1, < 2" && q.Scope == "" && q.Condition == "rack@>= 1, < 2" {
			found = true
			if q.Constraint == ">= 1" && !strings.Contains(q.Constraint, "< 2") {
				t.Fatal("dropped varargs range")
			}
			assertSBOMReq(t, r, from, "rack", ">= 1, < 2", "rack@>= 1, < 2", nil)
		}
	}
	if !found {
		t.Fatalf("varargs constraint lost: %s", extra5149JSON(t, r.Requirements))
	}

	r, err = scan(t, `s.add_dependency "rack", ">= 1" + dynamic_value`)
	if r == nil || err == nil || r.Complete {
		t.Fatalf("dynamic concat complete=%v err=%v reqs=%s", r != nil && r.Complete, err, extra5149JSON(t, r))
	}
	if scanerr.CodeOf(err) != scanerr.UnsupportedSyntax {
		t.Fatalf("dynamic code %q err=%v", scanerr.CodeOf(err), err)
	}
	for _, q := range r.Requirements {
		if q.Target == "rack" {
			t.Fatalf("dynamic concat guessed static range: %+v", q)
		}
	}

	r, err = scan(t, `s.add_dependency("rack", [">= 1", "< 2"]`)
	if r == nil || err == nil || r.Complete {
		t.Fatalf("truncated paren complete=%v err=%v reqs=%s", r != nil && r.Complete, err, extra5149JSON(t, r))
	}
	if scanerr.CodeOf(err) != scanerr.MalformedInput {
		t.Fatalf("truncated code %q err=%v", scanerr.CodeOf(err), err)
	}
	for _, q := range r.Requirements {
		if q.Target == "rack" {
			t.Fatalf("truncated call guessed range: %+v", q)
		}
	}
}

func TestGemspecIllegalNotCompleteEmpty(t *testing.T) {
	cases := []struct {
		name, src, code string
	}{
		{"empty-name", mustRead(t, "testdata/ruby_gemspec/negative/empty_name.gemspec"), scanerr.MalformedInput},
		{"truncated", "Gem::Specification.new do |s|\ns.name = \"app\"\n", scanerr.MalformedInput},
		{"dynamic-version", "Gem::Specification.new do |s|\ns.name = \"app\"\ns.version = ENV[\"V\"]\nend\n", scanerr.UnsupportedSyntax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ScanReport(context.Background(), fstest.MapFS{"gems/specifications/x.gemspec": {Data: []byte(tc.src)}}, WithSnapshotID("gemspec-bad"))
			if r == nil || err == nil || r.Complete {
				t.Fatalf("complete=%v err=%v report=%s", r != nil && r.Complete, err, extra5149JSON(t, r))
			}
			if scanerr.CodeOf(err) != tc.code {
				t.Fatalf("code %q want %q err=%v diags=%s", scanerr.CodeOf(err), tc.code, err, extra5149JSON(t, r.Diagnostics))
			}
			if len(r.Components) != 0 && tc.name != "dynamic-dep-kept" {
				t.Fatalf("illegal identity still produced components: %s", extra5149JSON(t, r.Components))
			}
		})
	}
}

func TestPackagingRequiresDistReport(t *testing.T) {
	raw, err := os.ReadFile("analyzer/dep-parser/python/packaging/testdata/zipp-3.12.1.METADATA")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ScanReport(context.Background(), fstest.MapFS{"zipp-3.12.1.dist-info/METADATA": {Data: raw}}, WithSnapshotID("packaging-req"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	c := mustNamed(t, r, "zipp", "3.12.1")
	if c.Key.Source != "https://github.com/jaraco/zipp" {
		t.Fatalf("home-page %q", c.Key.Source)
	}
	from := obsIDFor(t, r, c.Key.ID())
	found := false
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "sphinx" && q.Constraint == ">=3.5" && q.Condition == "extra == 'docs'" {
			found = true
			assertSBOMReq(t, r, from, "sphinx", ">=3.5", "extra == 'docs'", nil)
		}
	}
	if !found {
		t.Fatalf("Requires-Dist extras/markers lost: %s", extra5149JSON(t, r.Requirements))
	}
}

func TestPackagingDiscoveryAndIllegal(t *testing.T) {
	meta, err := os.ReadFile("testdata/python_packaging/dist-info/METADATA")
	if err != nil {
		t.Fatal(err)
	}
	egg, err := os.ReadFile("testdata/python_packaging/egg-info/PKG-INFO")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"x.dist-info/METADATA", "x.egg-info/PKG-INFO", "EGG-INFO/PKG-INFO"} {
		data := meta
		if strings.Contains(path, "egg") || strings.Contains(path, "EGG") {
			data = egg
		}
		r, err := ScanReport(context.Background(), fstest.MapFS{path: {Data: data}}, WithSnapshotID("packaging-"+path))
		if r == nil || err != nil || !r.Complete {
			t.Fatalf("%s complete=%v err=%v", path, r != nil && r.Complete, err)
		}
		mustNamed(t, r, "distlib", "0.3.1")
	}
	r, err := ScanReport(context.Background(), fstest.MapFS{"x.dist-info/METADATA": {Data: []byte("Name: foo\n")}}, WithSnapshotID("packaging-trunc"))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("truncated complete=%v err=%v", r != nil && r.Complete, err)
	}
	if scanerr.CodeOf(err) != scanerr.MalformedInput {
		t.Fatalf("code %q err=%v", scanerr.CodeOf(err), err)
	}
	r, err = ScanReport(context.Background(), fstest.MapFS{"x.dist-info/METADATA": {Data: []byte("Name: foo\nName: bar\nVersion: 1.0\n")}}, WithSnapshotID("packaging-dup"))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("duplicate Name complete=%v err=%v", r != nil && r.Complete, err)
	}
}

func TestPackagingFoldedRequiresDistReport(t *testing.T) {
	src := "Name: app\nVersion: 1.0\nHome-page: https://example.test/app\nRequires-Dist: requests (>=2.0)\n  ; extra == 'docs'\nProvides-Extra: docs\n"
	r, err := ScanReport(context.Background(), fstest.MapFS{"app-1.0.dist-info/METADATA": {Data: []byte(src)}}, WithSnapshotID("packaging-fold"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	if hasName(r, "docs") && namedComponent(r, "docs") {
		t.Fatalf("Provides-Extra became a component: %s", extra5149JSON(t, r.Components))
	}
	c := mustNamed(t, r, "app", "1.0")
	from := obsIDFor(t, r, c.Key.ID())
	found := false
	for _, q := range r.Requirements {
		if q.From == from && q.Target == "requests" && q.Constraint == ">=2.0" && q.Condition == "extra == 'docs'" {
			found = true
		}
	}
	if !found {
		t.Fatalf("folded Requires-Dist lost: %s", extra5149JSON(t, r.Requirements))
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func namedComponent(r *model.Report, name string) bool {
	for _, c := range r.Components {
		if c.Key.Name == name {
			return true
		}
	}
	return false
}

func TestGemspecLicenseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		assignment string
		want       []string
	}{
		{`s.license = "Custom, License".freeze # literal comma`, []string{"Custom, License"}},
		{`s.licenses = ["Custom", "License".freeze,].freeze`, []string{"Custom", "License"}},
		{`s.licenses = [%q<Custom, License>, "MIT"]`, []string{"Custom, License", "MIT"}},
	} {
		t.Run(tc.assignment, func(t *testing.T) {
			src := "Gem::Specification.new do |s|\ns.name = 'app'\ns.version = '1'\ns.homepage = 'https://example.test'\n" + tc.assignment + "\ns.add_runtime_dependency 'dep', '>= 1'\nend\n"
			r, err := ScanReport(context.Background(), fstest.MapFS{"gems/specifications/app.gemspec": {Data: []byte(src)}})
			if err != nil || !r.Complete {
				t.Fatalf("report=%+v err=%v", r, err)
			}
			c := mustNamed(t, r, "app", "1")
			if !reflect.DeepEqual(c.Licenses, tc.want) || c.Key.Source != "https://example.test" {
				t.Fatalf("component %+v want %q", c, tc.want)
			}
			if len(r.Requirements) != 1 || r.Requirements[0].Target != "dep" || r.Requirements[0].Constraint != ">= 1" {
				t.Fatalf("requirements %+v", r.Requirements)
			}
			bom, _, _ := parseSBOM(t, r)
			var got []string
			for _, choice := range bom.Components[0].Licenses {
				if choice.License == nil || choice.Expression != "" {
					t.Fatalf("choice %+v", choice)
				}
				got = append(got, choice.License.Name)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BOM licenses %q want %q", got, tc.want)
			}
		})
	}
	for _, assignment := range []string{
		`s.licenses = ["MIT" "BSD"]`, `s.licenses = [, "MIT"]`, `s.licenses = ["MIT",, "BSD"]`,
		`s.licenses = ["MIT"] + dynamic`, `s.license = "#{dynamic}"`, `s.license = "MIT" + dynamic`,
	} {
		t.Run(assignment, func(t *testing.T) {
			src := "Gem::Specification.new do |s|\ns.name = 'app'\ns.version = '1'\n" + assignment + "\nend\n"
			r, _ := ScanReport(context.Background(), fstest.MapFS{"gems/specifications/app.gemspec": {Data: []byte(src)}})
			if r == nil || r.Complete || len(r.Diagnostics) == 0 {
				t.Fatalf("silently accepted %s: %+v", assignment, r)
			}
		})
	}
}
