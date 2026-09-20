package model

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func TestIdentityAndNormalize(t *testing.T) {
	keys := []ComponentKey{{Ecosystem: "npm", Name: "@s/a", Version: "1", Source: "registry:a"}, {Ecosystem: "npm", Name: "@s/a", Version: "1", Source: "registry:b"}, {Ecosystem: "npm", Name: "@s/a", Version: "1"}, {Ecosystem: "python", Name: "@s/a", Version: "1"}, {Ecosystem: "npm", Name: "@s/a", Version: "1", Verification: "sha256:x"}, {Ecosystem: "npm", Name: "@s/a", Version: "1", Verification: "sha256:y"}, {Ecosystem: "npm", Name: "a b", Version: "c"}, {Ecosystem: "npm", Name: "a", Version: "b c"}}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k.ID()] {
			t.Fatal("identity collision")
		}
		seen[k.ID()] = true
	}
	r := Report{Complete: true}
	for _, k := range keys {
		r.Components = append(r.Components, Component{Key: k, Licenses: []string{"MIT"}})
	}
	a := Observation{Component: keys[0].ID(), Snapshot: "one", Project: "a", File: "a/package-lock.json", NativeID: "node_modules/@s/a", Kind: "locked"}
	b := a
	b.Snapshot = "two"
	c := a
	c.NativeID = "node_modules/x/node_modules/@s/a"
	r.Observations = []Observation{a, b, c, a}
	r.Components = append(r.Components, r.Components[0])
	r.Requirements = []Requirement{{From: a.ID(), Target: "@s/missing", Constraint: "^0.2", Operator: "or", Group: "1"}, {From: a.ID(), Target: "b", Resolved: []string{c.ID(), c.ID()}}}
	r.Normalize()
	raw, _ := json.Marshal(r)
	if len(r.Components) != len(keys) || len(r.Observations) != 3 {
		t.Fatalf("lost identity: %s", raw)
	}
	r.Normalize()
	twice, _ := json.Marshal(r)
	if string(raw) != string(twice) {
		t.Fatal("not idempotent")
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		rng.Shuffle(len(r.Components), func(i, j int) { r.Components[i], r.Components[j] = r.Components[j], r.Components[i] })
		rng.Shuffle(len(r.Requirements), func(i, j int) { r.Requirements[i], r.Requirements[j] = r.Requirements[j], r.Requirements[i] })
		r.Normalize()
		got, _ := json.Marshal(r)
		if string(got) != string(raw) {
			t.Fatal("order dependent")
		}
	}
}
