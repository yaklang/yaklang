package model

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// Frozen pre-optimization algorithm: JSON defines both equality and order.
func oldExact[T any](in []T) []T {
	m := map[string]T{}
	for _, v := range in {
		raw, _ := json.Marshal(v)
		m[string(raw)] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}
func checkExact[T any](t *testing.T, in []T) {
	t.Helper()
	got, want := exact(in), oldExact(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
func TestExactMatchesFrozenAlgorithm(t *testing.T) {
	for _, ss := range [][]string{nil, {}, {"single"}, {"z", "a", "z"}, {"\x00", "\\", "<", "é", "\xff", "\xfe"}} {
		checkExact(t, ss)
	}
	checkExact(t, []Requirement{{Target: "one"}})
	checkExact(t, []Requirement{{Target: "same", Resolved: nil}, {Target: "same", Resolved: []string{}}})
	rng := rand.New(rand.NewSource(5149))
	vals := []string{"a", "b", "\x00", ":", "\xff", "\xfe", "<", "é", ""}
	for n := 0; n < 80; n++ {
		in := make([]Requirement, n)
		for i := range in {
			in[i] = Requirement{Target: vals[rng.Intn(len(vals))], From: "parent", Constraint: vals[rng.Intn(len(vals))], Resolved: []string{vals[rng.Intn(len(vals))]}}
		}
		checkExact(t, in)
	}
	// Singleton fast paths must not alias the caller's outer slice.
	in := []Requirement{{Target: "a"}}
	out := exact(in)
	out[0].Target = "b"
	if in[0].Target != "a" {
		t.Fatal("normalization aliases caller slice")
	}
}

func frozenNormalize(r *Report) {
	components := map[ComponentKey]map[string]struct{}{}
	for _, c := range r.Components {
		ls := components[c.Key]
		if ls == nil {
			ls = map[string]struct{}{}
			components[c.Key] = ls
		}
		for _, l := range c.Licenses {
			ls[l] = struct{}{}
		}
	}
	r.Components = make([]Component, 0, len(components))
	for k, ls := range components {
		c := Component{Key: k}
		for l := range ls {
			c.Licenses = append(c.Licenses, l)
		}
		sort.Strings(c.Licenses)
		r.Components = append(r.Components, c)
	}
	ids := map[ComponentKey]string{}
	for _, c := range r.Components {
		ids[c.Key] = c.Key.ID()
	}
	sort.Slice(r.Components, func(i, j int) bool { return ids[r.Components[i].Key] < ids[r.Components[j].Key] })
	r.Observations = oldExact(r.Observations)
	for i := range r.Requirements {
		r.Requirements[i].Resolved = oldExact(r.Requirements[i].Resolved)
		r.Requirements[i].Candidates = oldExact(r.Requirements[i].Candidates)
	}
	r.Requirements = oldExact(r.Requirements)
	r.Diagnostics = oldExact(r.Diagnostics)
	for _, d := range r.Diagnostics {
		if d.Incomplete {
			r.Complete = false
		}
	}
}
func TestNormalizeMatchesFrozenAlgorithm(t *testing.T) {
	rng := rand.New(rand.NewSource(5149))
	for n := 0; n < 80; n++ {
		input := Report{Complete: true}
		for i := 0; i < n; i++ {
			key := ComponentKey{Ecosystem: "npm", Name: []string{"a", "b", "x:y"}[rng.Intn(3)], Version: []string{"1", "2"}[rng.Intn(2)], Source: []string{"registry", "local"}[rng.Intn(2)]}
			input.Components = append(input.Components, Component{Key: key, Licenses: []string{[]string{"MIT", "BSD", "MIT"}[rng.Intn(3)]}})
			input.Observations = append(input.Observations, Observation{Component: key.ID(), File: "lock", Kind: "locked"})
			input.Requirements = append(input.Requirements, Requirement{Target: key.Name, Constraint: "^1", Resolved: []string{key.ID(), key.ID()}})
		}
		input.Diagnostics = []Diagnostic{{Code: "evidence_insufficient", Incomplete: n%2 == 0}}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var got, want Report
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		got.Normalize()
		frozenNormalize(&want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("n=%d full normalized report differs", n)
		}
	}
}
