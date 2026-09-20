// Package model defines SCA data without filesystem, parser, or SDK dependencies.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ComponentKey describes a component, not an installation location. Empty
// Version is explicitly unknown; Constraint belongs to a Requirement.
type ComponentKey struct {
	Ecosystem    string `json:"ecosystem"`
	Name         string `json:"name"`
	Version      string `json:"version,omitempty"`
	Source       string `json:"source,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Variant      string `json:"variant,omitempty"`
	Verification string `json:"verification,omitempty"`
}

func (k ComponentKey) ID() string {
	raw, _ := json.Marshal(k)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type Component struct {
	Key      ComponentKey `json:"key"`
	Licenses []string     `json:"licenses,omitempty"`
}
type Observation struct {
	Provides          []string `json:"provides,omitempty"`
	Condition         string   `json:"condition,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	Component         string   `json:"component"`
	Snapshot          string   `json:"snapshot"`
	Project           string   `json:"project"`
	File              string   `json:"file"`
	StartLine         int      `json:"startLine,omitempty"`
	EndLine           int      `json:"endLine,omitempty"`
	NativeID          string   `json:"nativeId,omitempty"`
	Kind              string   `json:"kind"`
	DeclaredIntegrity string   `json:"declaredIntegrity,omitempty"`
}

func (o Observation) ID() string {
	raw, _ := json.Marshal(o)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type Requirement struct {
	Candidates []string `json:"candidates,omitempty"` // possible providers; never an asserted installed edge
	From       string   `json:"from"`                 // observation ID; empty denotes a project declaration
	Target     string   `json:"target"`               // native reference or component name; never reverse-split
	Constraint string   `json:"constraint,omitempty"`
	Resolved   []string `json:"resolved,omitempty"` // observation IDs, not component IDs
	Scope      string   `json:"scope,omitempty"`
	Condition  string   `json:"condition,omitempty"`
	Group      string   `json:"group,omitempty"`
	Operator   string   `json:"operator,omitempty"` // and/or; alternatives stay separate
}
type Diagnostic struct {
	Code       string `json:"code"`
	Stage      string `json:"stage"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Reason     string `json:"reason"`
	Incomplete bool   `json:"incomplete"`
}
type Report struct {
	Components   []Component   `json:"components"`
	Observations []Observation `json:"observations"`
	Requirements []Requirement `json:"requirements"`
	Diagnostics  []Diagnostic  `json:"diagnostics"`
	Complete     bool          `json:"complete"`
}

// Normalize performs exact identity deduplication followed by deterministic
// sorting. It does not solve constraints or bind references across projects.
// Expected cost is O(N + E) for indexes plus O(N log N + E log E) sorting,
// including key lengths and output size. Inputs must already be budgeted.
func (r *Report) Normalize() {
	components := map[ComponentKey]map[string]struct{}{}
	for _, c := range r.Components {
		licenses := components[c.Key]
		if licenses == nil {
			licenses = map[string]struct{}{}
			components[c.Key] = licenses
		}
		for _, l := range c.Licenses {
			licenses[l] = struct{}{}
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
	ids := make(map[ComponentKey]string, len(r.Components))
	for _, c := range r.Components {
		ids[c.Key] = c.Key.ID()
	}
	sort.Slice(r.Components, func(i, j int) bool { return ids[r.Components[i].Key] < ids[r.Components[j].Key] })
	r.Observations = exact(r.Observations)
	for i := range r.Requirements {
		r.Requirements[i].Resolved = exact(r.Requirements[i].Resolved)
		r.Requirements[i].Candidates = exact(r.Requirements[i].Candidates)
	}
	r.Requirements = exact(r.Requirements)
	r.Diagnostics = exact(r.Diagnostics)
	for _, d := range r.Diagnostics {
		if d.Incomplete {
			r.Complete = false
		}
	}
}
func exact[T any](in []T) []T {
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
