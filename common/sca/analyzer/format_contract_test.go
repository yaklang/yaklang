package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/nodejs/pnpm"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/nodejs/yarn"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/php/composer"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/bundler"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/gemspec"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/rust/cargo"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"strings"
	"testing"
)

func TestIndependentNativeIdentity(t *testing.T) {
	tests := []struct {
		name, input string
		p           types.Parser
		count       int
	}{
		{"cargo source ambiguity", `[[package]]
name="app"
version="1"
dependencies=["x"]
[[package]]
name="x"
version="1"
source="registry+a"
[[package]]
name="x"
version="1"
source="registry+b"
`, cargo.NewParser(), 3},
		{"bundler platform ambiguity", "GEM\n  remote: https://example.test/\n  specs:\n    app (1.0)\n      x (>= 1)\n    x (1.0-x86_64-linux)\n    x (1.0-arm64-darwin)\n", bundler.NewParser(), 3},
		{"yarn descriptor instances", "x@^1:\n  version \"1.0.0\"\n  resolved \"https://example.test/a\"\n  dependencies:\n    a \"1\"\n\nx@~1:\n  version \"1.0.0\"\n  resolved \"https://example.test/b\"\n  dependencies:\n    b \"1\"\n", yarn.NewParser(), 2},
		{"pnpm peer instances", "lockfileVersion: '6.0'\npackages:\n  /x@1.0.0(a@1.0.0):\n    resolution: {integrity: abc}\n  /x@1.0.0(a@2.0.0):\n    resolution: {integrity: abc}\n", pnpm.NewParser(), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			libs, deps, err := tt.p.Parse(nil, strings.NewReader(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if len(libs) != tt.count {
				t.Fatalf("lost records: %+v", libs)
			}
			ids := map[string]bool{}
			for _, l := range libs {
				if l.ID == "" || ids[l.ID] {
					t.Fatalf("lost native identity: %+v", libs)
				}
				ids[l.ID] = true
			}
			if strings.Contains(tt.name, "ambiguity") {
				for _, d := range deps {
					for _, ref := range d.DependsOn {
						if ids[ref] {
							t.Fatalf("arbitrarily resolved ambiguous reference: %s", ref)
						}
					}
					for _, q := range d.Requirements {
						if q.Resolved != "" {
							t.Fatal("arbitrary candidate")
						}
					}
				}
			}
			if strings.HasPrefix(tt.name, "yarn") {
				if libs[0].Source == libs[1].Source || len(deps) != 2 {
					t.Fatal("lost provenance/edges")
				}
			}
		})
	}
}
func TestGemspecNeverEvaluatesExpressions(t *testing.T) {
	for _, v := range []string{`ENV["VERSION"]`, `"1" + "2"`, `"#{VERSION}"`, `File.read("version")`} {
		_, _, err := gemspec.NewParser().Parse(nil, strings.NewReader("Gem::Specification.new do |s|\ns.name = \"app\"\ns.version = "+v+"\nend\n"))
		if err == nil {
			t.Fatal("accepted dynamic Ruby", v)
		}
	}
}
func TestComposerPreservesRequirements(t *testing.T) {
	libs, deps, err := composer.NewParser().Parse(nil, strings.NewReader(`{"packages":[{"name":"app","version":"1","source":{"url":"https://example.test/a","reference":"commit"},"require":{"php":">=8","missing/x":"^2"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].Source != "https://example.test/a#commit" || len(deps) != 1 || len(deps[0].Requirements) != 2 {
		t.Fatalf("lost evidence: %+v %+v", libs, deps)
	}
	for _, q := range deps[0].Requirements {
		if q.Resolved != "" || q.Constraint == "" {
			t.Fatal("invented binding or dropped constraint")
		}
	}
}
func TestAPKConstraintAndRawLicense(t *testing.T) {
	for raw, want := range map[string]string{"musl>=1.2": ">=1.2", "so:lib.so=1": "=1", "x<2": "<2"} {
		_, got := trimRequirement(raw)
		if got != want {
			t.Fatalf("%s -> %s", raw, got)
		}
	}
	_ = NewApkAnalyzer().parseLicense("L:MIT OR Apache 2.0")
}
