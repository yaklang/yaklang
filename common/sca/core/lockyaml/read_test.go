package lockyaml

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNestedFlowQuotedKeysAndVersions(t *testing.T) {
	t.Run("pnpm-peer-key", func(t *testing.T) {
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
	})
}

func TestRejectDuplicatesAliasesAndTrailing(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"duplicate-key", "a: 1\na: 2"},
		{"alias-anchor", "a: &x {b: 1}\nc: *x"},
		{"flow-duplicate", "a: {b: 1, b: 2}"},
		{"flow-trailing", "a: {b: 1} junk"},
		{"tag", "a: !tag 1"},
		{"literal-block", "a: |\n  hello"},
		{"document-marker", "a: 1\n---\nb: 2"},
		{"tab-indent", "a:\n\tb: 1"},
		{"unterminated-quote", "a: 'unterminated"},
		{"depth", "a: " + strings.Repeat("[", 66) + "x" + strings.Repeat("]", 66)},
		{"unknown-anchor", "a: *b\n"},
		{"self-anchor", "a: &a\n  b: *a\n"},
		{"unclosed-flow", "v: [A,"},
		{"tag-directive", "%TAG !%79! tag:yaml.org,2002:\n---\nv: !%79!int '1'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := Parse(context.Background(), []byte(tc.src)); e == nil {
				t.Fatalf("accepted %q", tc.src)
			}
		})
	}
}

func TestMultilineFlowAndSequence(t *testing.T) {
	t.Run("flow-and-block-seq", func(t *testing.T) {
		m, e := Parse(context.Background(), []byte("a: {b: [one,\n  two], c: \"x\\u0041\"}\nseq:\n  - name: 'first'\n    version: '1'\n  - name: 'second'\n"))
		if e != nil {
			t.Fatal(e)
		}
		if m["a"].(map[string]any)["c"] != "xA" || len(m["seq"].([]any)) != 2 {
			t.Fatal(m)
		}
	})
}

// TestUpstreamLockGrammar keeps yaml.v3 unmarshal cases that the pnpm lock
// grammar still answers. Typed int/float/timestamp/binary/struct/merge remain uncovered.
func TestUpstreamLockGrammar(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, src string
		want      map[string]any
	}{
		{"plain-string", "v: hi", map[string]any{"v": "hi"}},
		{"bool-true", "v: true", map[string]any{"v": true}},
		{"bool-false", "canonical: false", map[string]any{"canonical": false}},
		{"bool-True", "bool: True", map[string]any{"bool": true}},
		{"bool-FALSE", "bool: FALSE", map[string]any{"bool": false}},
		{"null-empty", "empty:", map[string]any{"empty": nil}},
		{"null-tilde", "canonical: ~", map[string]any{"canonical": nil}},
		{"null-word", "english: null", map[string]any{"english": nil}},
		{"flow-seq", "seq: [A,B]", map[string]any{"seq": []any{"A", "B"}}},
		{"flow-seq-trail", "seq: [A,B,C,]", map[string]any{"seq": []any{"A", "B", "C"}}},
		{"flow-seq-digit-string", "seq: [A,1,C]", map[string]any{"seq": []any{"A", "1", "C"}}},
		{"block-seq", "seq:\n - A\n - B", map[string]any{"seq": []any{"A", "B"}}},
		{"flow-map", "a: {b: c}", map[string]any{"a": map[string]any{"b": "c"}}},
		{"hello-world", "hello: world", map[string]any{"hello": "world"}},
		{"quoted-null-string", "a: 'null'", map[string]any{"a": "null"}},
		{"numeric-kept-string", "version: 9007199254740993", map[string]any{"version": "9007199254740993"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(ctx, []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
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
