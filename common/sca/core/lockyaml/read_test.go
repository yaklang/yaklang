package lockyaml

import (
	"context"
	"strings"
	"testing"
)

func TestNestedFlowQuotedKeysAndVersions(t *testing.T) {
	b := []byte("lockfileVersion: '6.0'\npackages:\n  '/@scope/name@1.2.3(peer@4)':\n    resolution: {integrity: 'sha512:a#b', tarball: https://example.org/a}\n    os: [darwin, 'win32']\n    version: 9007199254740993\n    dev: false\n")
	m, e := Parse(context.Background(), b)
	if e != nil {
		t.Fatal(e)
	}
	pkg := m["packages"].(map[string]any)["/@scope/name@1.2.3(peer@4)"].(map[string]any)
	if pkg["version"] != "9007199254740993" || pkg["dev"] != false {
		t.Fatal(pkg)
	}
	if pkg["resolution"].(map[string]any)["integrity"] != "sha512:a#b" {
		t.Fatal(pkg)
	}
}
func TestRejectDuplicatesAliasesAndTrailing(t *testing.T) {
	for _, s := range []string{"a: 1\na: 2", "a: &x {b: 1}\nc: *x", "a: {b: 1, b: 2}", "a: {b: 1} junk", "a: !tag 1", "a: |\n  hello", "a: 1\n---\nb: 2", "a:\n\tb: 1", "a: 'unterminated", "a: " + strings.Repeat("[", 66) + "x" + strings.Repeat("]", 66)} {
		if _, e := Parse(context.Background(), []byte(s)); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
func TestMultilineFlowAndSequence(t *testing.T) {
	m, e := Parse(context.Background(), []byte("a: {b: [one,\n  two], c: \"x\\u0041\"}\nseq:\n  - name: 'first'\n    version: '1'\n  - name: 'second'\n"))
	if e != nil {
		t.Fatal(e)
	}
	if m["a"].(map[string]any)["c"] != "xA" || len(m["seq"].([]any)) != 2 {
		t.Fatal(m)
	}
}
func FuzzLockYAML(f *testing.F) {
	f.Add([]byte("lockfileVersion: 5.4\npackages: {}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		Parse(context.Background(), b)
	})
}
