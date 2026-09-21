package cargo

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParseKeepsOriginalDependencyNames(t *testing.T) {
	f, err := os.Open("testdata/cargo_v3.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, deps, err := NewParser().Parse(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	foundMemchr, foundRegex := false, false
	for _, d := range deps {
		for _, q := range d.Requirements {
			if q.Target == "" || q.Target[0] == '[' || strings.HasPrefix(q.Target, "unresolved-cargo:") {
				t.Fatalf("requirement target is not the lock name: %+v", q)
			}
			if q.Target == "memchr" && q.Constraint == "2.5.0" {
				foundMemchr = true
				if q.Condition != "memchr 2.5.0" {
					t.Fatalf("explicit version raw form: %+v", q)
				}
			}
			if q.Target == "regex" {
				foundRegex = true
				if q.Constraint != "" {
					t.Fatalf("name-only regex invented constraint %q", q.Constraint)
				}
				if q.Condition != "regex" {
					t.Fatalf("name-only raw form: %+v", q)
				}
				if q.Resolved == "" {
					t.Fatalf("unique name-only regex should still resolve: %+v", q)
				}
			}
		}
	}
	if !foundMemchr || !foundRegex {
		t.Fatalf("missing original memchr 2.5.0 or name-only regex: %+v", deps)
	}
}

func TestParseDependencyQualifiers(t *testing.T) {
	lock := func(deps string, extra string) string {
		return "version = 3\n[[package]]\nname = \"app\"\nversion = \"1.0.0\"\ndependencies = [\n" + deps + "\n]\n" + extra
	}
	foo := func(ver, src string) string {
		return "[[package]]\nname = \"foo\"\nversion = \"" + ver + "\"\nsource = \"" + src + "\"\n"
	}
	present := "registry+https://present.example/index"
	missing := "registry+https://missing.example/index"
	reqOf := func(t *testing.T, src string) []types.Requirement {
		t.Helper()
		_, deps, err := NewParser().Parse(nil, bytes.NewReader([]byte(src)))
		if err != nil {
			t.Fatal(err)
		}
		var out []types.Requirement
		for _, d := range deps {
			out = append(out, d.Requirements...)
			for _, id := range d.DependsOn {
				if strings.HasPrefix(id, "unresolved-cargo:") {
					t.Fatalf("synthetic unresolved target in DependsOn: %q", id)
				}
			}
		}
		return out
	}
	t.Run("name-only-unique", func(t *testing.T) {
		qs := reqOf(t, lock(` "foo" `, foo("1.0.0", present)))
		if len(qs) != 1 {
			t.Fatalf("got %+v", qs)
		}
		q := qs[0]
		if q.Target != "foo" || q.Constraint != "" || q.Condition != "foo" || q.Resolved == "" {
			t.Fatalf("name-only unique: %+v", q)
		}
	})
	t.Run("explicit-version", func(t *testing.T) {
		qs := reqOf(t, lock(` "foo 1.0.0" `, foo("1.0.0", present)))
		if len(qs) != 1 || qs[0].Target != "foo" || qs[0].Constraint != "1.0.0" || qs[0].Condition != "foo 1.0.0" || qs[0].Resolved == "" {
			t.Fatalf("explicit version: %+v", qs)
		}
	})
	t.Run("explicit-source-match", func(t *testing.T) {
		raw := "foo 1.0.0 (registry+https://present.example/index)"
		qs := reqOf(t, lock(" \""+raw+"\" ", foo("1.0.0", present)))
		if len(qs) != 1 || qs[0].Target != "foo" || qs[0].Constraint != "1.0.0" || qs[0].Condition != raw || qs[0].Resolved == "" {
			t.Fatalf("source match: %+v", qs)
		}
	})
	t.Run("explicit-source-missing", func(t *testing.T) {
		raw := "foo 1.0.0 (registry+https://missing.example/index)"
		qs := reqOf(t, lock(" \""+raw+"\" ", foo("1.0.0", present)))
		if len(qs) != 1 {
			t.Fatalf("duplicate requirements: %+v", qs)
		}
		q := qs[0]
		if q.Target != "foo" || q.Constraint != "1.0.0" || q.Condition != raw || q.Resolved != "" {
			t.Fatalf("missing source must stay one unresolved original: %+v", q)
		}
	})
	t.Run("no-match", func(t *testing.T) {
		qs := reqOf(t, lock(` "missing" `, foo("1.0.0", present)))
		if len(qs) != 1 || qs[0].Target != "missing" || qs[0].Constraint != "" || qs[0].Condition != "missing" || qs[0].Resolved != "" {
			t.Fatalf("no match: %+v", qs)
		}
	})
	t.Run("ambiguous-name", func(t *testing.T) {
		qs := reqOf(t, lock(` "foo" `, foo("1.0.0", present)+foo("2.0.0", present)))
		if len(qs) != 1 || qs[0].Target != "foo" || qs[0].Constraint != "" || qs[0].Resolved != "" {
			t.Fatalf("ambiguous name: %+v", qs)
		}
	})
	t.Run("ambiguous-version-two-sources", func(t *testing.T) {
		qs := reqOf(t, lock(` "foo 1.0.0" `, foo("1.0.0", present)+foo("1.0.0", missing)))
		if len(qs) != 1 || qs[0].Target != "foo" || qs[0].Constraint != "1.0.0" || qs[0].Condition != "foo 1.0.0" || qs[0].Resolved != "" {
			t.Fatalf("ambiguous version/source: %+v", qs)
		}
	})
}
