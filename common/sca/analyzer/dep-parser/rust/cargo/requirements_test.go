package cargo

import (
	"os"
	"testing"
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
	found := false
	for _, d := range deps {
		for _, q := range d.Requirements {
			if q.Target == "memchr" && q.Constraint == "2.5.0" {
				found = true
			}
			if q.Target == "" || q.Target[0] == '[' {
				t.Fatalf("requirement target is not the lock name: %+v", q)
			}
		}
	}
	if !found {
		t.Fatalf("missing original memchr 2.5.0 requirement: %+v", deps)
	}
}
