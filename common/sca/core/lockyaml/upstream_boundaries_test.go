// Adversarial inputs retained from go-yaml/yaml v3; see ../../licenses/yaml/LICENSE.
// The finite pnpm grammar rejects aliases and roots other than mappings.
package lockyaml

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"strings"
	"testing"
)

var limitTests = []struct {
	name  string
	data  []byte
	error string
}{
	{
		name:  "1000kb of maps with 100 aliases",
		data:  []byte(`{a: &a [{a}` + strings.Repeat(`,{a}`, 1000*1024/4-100) + `], b: &b [*a` + strings.Repeat(`,*a`, 99) + `]}`),
		error: "yaml: document contains excessive aliasing",
	}, {
		name:  "1000kb of deeply nested slices",
		data:  []byte(strings.Repeat(`[`, 1000*1024)),
		error: "yaml: exceeded max depth of 10000",
	}, {
		name:  "1000kb of deeply nested maps",
		data:  []byte("x: " + strings.Repeat(`{`, 1000*1024)),
		error: "yaml: exceeded max depth of 10000",
	}, {
		name:  "1000kb of deeply nested indents",
		data:  []byte(strings.Repeat(`- `, 1000*1024)),
		error: "yaml: exceeded max depth of 10000",
	}, {
		name: "1000kb of 1000-indent lines",
		data: []byte(strings.Repeat(strings.Repeat(`- `, 1000)+"\n", 1024/2)),
	},
	{name: "1kb of maps", data: []byte(`a: &a [{a}` + strings.Repeat(`,{a}`, 1*1024/4-1) + `]`)},
	{name: "10kb of maps", data: []byte(`a: &a [{a}` + strings.Repeat(`,{a}`, 10*1024/4-1) + `]`)},
	{name: "100kb of maps", data: []byte(`a: &a [{a}` + strings.Repeat(`,{a}`, 100*1024/4-1) + `]`)},
	{name: "1000kb of maps", data: []byte(`a: &a [{a}` + strings.Repeat(`,{a}`, 1000*1024/4-1) + `]`)},
	{name: "1000kb slice nested at max-depth", data: []byte(strings.Repeat(`[`, 10000) + `1` + strings.Repeat(`,1`, 1000*1024/2-20000-1) + strings.Repeat(`]`, 10000))},
	{name: "1000kb slice nested in maps at max-depth", data: []byte("{a,b:\n" + strings.Repeat(" {a,b:", 10000-2) + ` [1` + strings.Repeat(",1", 1000*1024/2-6*10000-1) + `]` + strings.Repeat(`}`, 10000-1))},
	{name: "1000kb of 10000-nested lines", data: []byte(strings.Repeat(`- `+strings.Repeat(`[`, 10000)+strings.Repeat(`]`, 10000)+"\n", 1000*1024/20000))},
}

func TestUpstreamLimits(t *testing.T) {
	for _, tc := range limitTests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := (budget.Limits{MaxResultBytes: 8 << 20, MaxSyntaxDepth: 64}).Normalize()
			if err != nil {
				t.Fatal(err)
			}
			_, err = Parse(budget.Bind(context.Background(), l), tc.data)
			if err == nil {
				t.Fatal("accepted alias/deep/root-sequence input outside finite lock grammar")
			}
		})
	}
	m, err := Parse(context.Background(), []byte("lockfileVersion: 5.4\npackages: {a: {version: '1'}}\n"))
	if err != nil || m["lockfileVersion"] == nil {
		t.Fatalf("positive mapping: %v %v", m, err)
	}
}

func TestUpstreamFuzzCrashers(t *testing.T) {
	cases := []string{
		// runtime error: index out of range
		"\"\\0\\\r\n",

		// should not happen
		"  0: [\n] 0",
		"? ? \"\n\" 0",
		"    - {\n000}0",
		"0:\n  0: [0\n] 0",
		"    - \"\n000\"0",
		"    - \"\n000\"\"",
		"0:\n    - {\n000}0",
		"0:\n    - \"\n000\"0",
		"0:\n    - \"\n000\"\"",

		// runtime error: index out of range
		" \ufeff\n",
		"? \ufeff\n",
		"? \ufeff:\n",
		"0: \ufeff\n",
		"? \ufeff: \ufeff\n",
	}
	for _, data := range cases {
		_, _ = Parse(context.Background(), []byte(data))
	}
}
