package npm

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestPhysicalInstancesAndNearestAncestor(t *testing.T) {
	raw := `{"lockfileVersion":3,"packages":{"node_modules/a":{"version":"1","dependencies":{"x":"*"}},"node_modules/b":{"version":"1","dependencies":{"x":"*"}},"node_modules/a/node_modules/x":{"version":"2"},"node_modules/b/node_modules/x":{"version":"3"},"node_modules/x":{"version":"4"}}}`
	libs, deps, err := NewParser().Parse(nil, strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 5 {
		t.Fatal("lost instances")
	}
	targets := map[string]string{}
	for _, d := range deps {
		for _, r := range d.Requirements {
			targets[d.ID] = r.Resolved
		}
	}
	if targets["node_modules/a"] != "node_modules/a/node_modules/x" || targets["node_modules/b"] != "node_modules/b/node_modules/x" {
		t.Fatalf("wrong ancestor resolution: %v", targets)
	}
}
func TestWorkspaceLinkCycleAndBoundary(t *testing.T) {
	for _, raw := range []string{`{"lockfileVersion":3,"packages":{"node_modules/a":{"link":true,"resolved":"node_modules/b"},"node_modules/b":{"link":true,"resolved":"node_modules/a"}}}`, `{"lockfileVersion":3,"packages":{"../outside":{"version":"1"}}}`} {
		if _, _, err := NewParser().Parse(nil, strings.NewReader(raw)); err == nil {
			t.Fatal("accepted invalid link/path")
		}
	}
}
func FuzzLock(f *testing.F) {
	f.Add([]byte(`{"lockfileVersion":3,"packages":{"node_modules/x":{"version":"1"}}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		NewParser().Parse(nil, bytes.NewReader(b))
	})
}
func BenchmarkPathIndex(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 50000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			packages := map[string]Package{}
			for i := 0; i < n; i++ {
				packages[fmt.Sprintf("node_modules/p%d", i)] = Package{Name: fmt.Sprintf("p%d", i), Version: "1"}
			}
			p := &Parser{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.parseV2(packages)
			}
		})
	}
}
