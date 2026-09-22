package gemspec_test

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/gemspec"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParseGemspecOriginalDeclarations(t *testing.T) {
	src, err := fixtures.ReadFile("testdata/multiple_licenses.gemspec")
	if err != nil {
		t.Fatal(err)
	}
	libs, deps, err := gemspec.NewParser().Parse(nil, strings.NewReader(string(src)))
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].Name != "test-unit" || libs[0].Version != "3.3.7" {
		t.Fatalf("%+v", libs)
	}
	if libs[0].License != "Ruby, BSDL, PSFL" {
		t.Fatalf("licenses %q", libs[0].License)
	}
	if libs[0].Source != "http://test-unit.github.io/" {
		t.Fatalf("homepage source %q", libs[0].Source)
	}
	if libs[0].Locations == nil || libs[0].Locations[0].StartLine != 4 || libs[0].Locations[0].EndLine < 40 {
		t.Fatalf("location %+v", libs[0].Locations)
	}
	if len(deps) != 1 || deps[0].ID != libs[0].ID {
		t.Fatalf("deps %+v", deps)
	}
	got := map[string]types.Requirement{}
	for _, q := range deps[0].Requirements {
		if _, ok := got[q.Target]; ok {
			t.Fatalf("if/else duplicated %s: %+v", q.Target, deps[0].Requirements)
		}
		got[q.Target] = q
		if strings.HasPrefix(q.Target, "unresolved-") {
			t.Fatalf("synthetic %q", q.Target)
		}
	}
	if len(got) != 6 {
		t.Fatalf("want 6 unique deps, got %d: %+v", len(got), deps[0].Requirements)
	}
	pa := got["power_assert"]
	if pa.Constraint != ">= 0" || pa.Scope != "" || pa.Condition != "power_assert@>= 0" || pa.Resolved != "" {
		t.Fatalf("runtime %+v", pa)
	}
	for _, name := range []string{"bundler", "rake", "yard", "kramdown", "packnga"} {
		q := got[name]
		if q.Constraint != ">= 0" || q.Scope != "dev" || q.Condition != name+"@>= 0" || q.Resolved != "" {
			t.Fatalf("dev %s %+v", name, q)
		}
	}
}

func TestParseGemspecDeclarationForms(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0.0"
  s.homepage = "https://example.test/app".freeze
  s.add_runtime_dependency(%q<foo>.freeze, [">= 1.0", "< 2.0"])
  s.add_runtime_dependency "bar"
  s.add_development_dependency('baz', '~> 3.0')
  s.add_dependency(%q<quux>.freeze, [">= 0"])
end
`
	libs, deps, err := gemspec.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].Source != "https://example.test/app" {
		t.Fatalf("homepage %q", libs[0].Source)
	}
	got := map[string]types.Requirement{}
	for _, q := range deps[0].Requirements {
		got[q.Target] = q
	}
	if got["foo"].Constraint != ">= 1.0, < 2.0" || got["foo"].Scope != "" {
		t.Fatalf("multi range %+v", got["foo"])
	}
	if got["bar"].Constraint != "" || got["bar"].Condition != "bar" {
		t.Fatalf("empty range %+v", got["bar"])
	}
	if got["baz"].Constraint != "~> 3.0" || got["baz"].Scope != "dev" {
		t.Fatalf("quoted dev %+v", got["baz"])
	}
	if got["quux"].Constraint != ">= 0" || got["quux"].Scope != "" {
		t.Fatalf("add_dependency %+v", got["quux"])
	}
}

func TestParseGemspecIfElsePrefersScoped(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0"
  if s.respond_to? :add_runtime_dependency then
    s.add_runtime_dependency(%q<foo>.freeze, [">= 0"])
    s.add_development_dependency(%q<bar>.freeze, [">= 1"])
  else
    s.add_dependency(%q<foo>.freeze, [">= 0"])
    s.add_dependency(%q<bar>.freeze, [">= 1"])
  end
end
`
	_, deps, err := gemspec.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]types.Requirement{}
	for _, q := range deps[0].Requirements {
		if _, ok := got[q.Target]; ok {
			t.Fatalf("duplicated %+v", deps[0].Requirements)
		}
		got[q.Target] = q
	}
	if got["foo"].Scope != "" || got["bar"].Scope != "dev" || len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestParseGemspecSameNameDifferentRanges(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0"
  s.add_runtime_dependency "foo", ">= 1.0"
  s.add_runtime_dependency "foo", "< 2.0"
end
`
	_, deps, err := gemspec.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(deps[0].Requirements) != 2 {
		t.Fatalf("distinct ranges dropped: %+v", deps[0].Requirements)
	}
}

func TestParseGemspecDynamicIsDiagnosed(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0"
  s.add_runtime_dependency gem_name
  s.add_runtime_dependency ENV["GEM"]
  s.add_runtime_dependency("foo", version_req)
end
`
	libs, deps, err := gemspec.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].Name != "app" {
		t.Fatalf("%+v", libs)
	}
	if len(deps) != 0 {
		t.Fatalf("guessed dynamic deps: %+v", deps)
	}
	n := 0
	for _, d := range libs[0].Diagnostics {
		if d.Code == "unsupported_syntax" && d.Incomplete {
			n++
		}
	}
	if n < 3 {
		t.Fatalf("dynamic Ruby not diagnosed: %+v", libs[0].Diagnostics)
	}
}

func TestParseGemspecTruncatedDependency(t *testing.T) {
	src := `Gem::Specification.new do |s|
  s.name = "app"
  s.version = "1.0"
  s.add_runtime_dependency(%q<foo
end
`
	libs, _, err := gemspec.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range libs[0].Diagnostics {
		if d.Code == "malformed_input" && d.Incomplete {
			found = true
		}
	}
	if !found {
		t.Fatalf("truncated dep not diagnosed: %+v", libs[0].Diagnostics)
	}
}

func TestParseGemspecMissingIdentity(t *testing.T) {
	_, _, err := gemspec.NewParser().Parse(nil, strings.NewReader("Gem::Specification.new do |s|\nend\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") {
		t.Fatalf("empty spec: %v", err)
	}
}

func TestParseGemspecVarargsAndCallSuffix(t *testing.T) {
	parse := func(line string) ([]types.Library, []types.Dependency, error) {
		src := "Gem::Specification.new do |s|\n  s.name = \"app\"\n  s.version = \"1.0.0\"\n  " + line + "\nend\n"
		return gemspec.NewParser().Parse(nil, strings.NewReader(src))
	}
	req := func(t *testing.T, line string) types.Requirement {
		t.Helper()
		libs, deps, err := parse(line)
		if err != nil || len(libs) != 1 {
			t.Fatalf("%s: %+v %v", line, libs, err)
		}
		if len(deps) != 1 || len(deps[0].Requirements) != 1 {
			t.Fatalf("%s deps %+v", line, deps)
		}
		return deps[0].Requirements[0]
	}
	for _, line := range []string{
		`s.add_dependency "rack", ">= 1", "< 2"`,
		`s.add_dependency("rack", ">= 1", "< 2")`,
		`s.add_dependency "rack", [">= 1", "< 2"]`,
		`s.add_dependency("rack", [">= 1", "< 2"])`,
		`s.add_dependency(%q<rack>.freeze, [">= 1".freeze, "< 2".freeze])`,
	} {
		q := req(t, line)
		if q.Target != "rack" || q.Constraint != ">= 1, < 2" || q.Scope != "" || q.Condition != "rack@>= 1, < 2" {
			t.Fatalf("%s -> %+v", line, q)
		}
	}
	empty := req(t, `s.add_dependency "rack"`)
	if empty.Target != "rack" || empty.Constraint != "" || empty.Condition != "rack" {
		t.Fatalf("no range %+v", empty)
	}
	single := req(t, `s.add_runtime_dependency "rack", "~> 1.0"`)
	if single.Constraint != "~> 1.0" || single.Scope != "" {
		t.Fatalf("single %+v", single)
	}

	dynamic := []string{
		`s.add_dependency "rack", ">= 1" + dynamic_value`,
		`s.add_dependency "rack", ">= 1", dynamic_value`,
		`s.add_dependency "rack", GEM_VERSION`,
	}
	for _, line := range dynamic {
		libs, deps, err := parse(line)
		if err != nil {
			t.Fatalf("%s parse err %v", line, err)
		}
		if len(deps) != 0 {
			t.Fatalf("%s guessed %+v", line, deps)
		}
		found := false
		for _, d := range libs[0].Diagnostics {
			if d.Code == "unsupported_syntax" && d.Incomplete {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s not diagnosed: %+v", line, libs[0].Diagnostics)
		}
	}
	truncated := []string{
		`s.add_dependency("rack", [">= 1", "< 2"]`,
		`s.add_dependency "rack", ">= 1",`,
		`s.add_dependency(%q<rack>`,
	}
	for _, line := range truncated {
		libs, deps, err := parse(line)
		if err != nil {
			t.Fatalf("%s parse err %v", line, err)
		}
		if len(deps) != 0 {
			t.Fatalf("%s guessed truncated %+v", line, deps)
		}
		found := false
		for _, d := range libs[0].Diagnostics {
			if d.Code == "malformed_input" && d.Incomplete {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s truncated not diagnosed: %+v", line, libs[0].Diagnostics)
		}
	}
	libs, deps, err := parse(`s.add_dependency "rack", ">= 1" leftover`)
	if err != nil || len(deps) != 0 {
		t.Fatalf("garbage tail %+v %v", deps, err)
	}
	found := false
	for _, d := range libs[0].Diagnostics {
		if d.Incomplete && (d.Code == "malformed_input" || d.Code == "unsupported_syntax") {
			found = true
		}
	}
	if !found {
		t.Fatalf("garbage tail not diagnosed: %+v", libs[0].Diagnostics)
	}
}
