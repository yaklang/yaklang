// Package model defines SCA data without filesystem, parser, or SDK dependencies.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
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
	var scratch [512]byte
	raw, ok := componentJSON(scratch[:0], k)
	if !ok {
		raw, _ = json.Marshal(k)
	}
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
	var scratch [768]byte
	raw, ok := observationJSON(scratch[:0], o)
	if !ok {
		raw, _ = json.Marshal(o)
	}
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
	// Decorate once instead of hashing the seven-field key for every sort
	// comparison. Keep the same ComponentKey.ID ordering as before.
	type keyedComponent struct {
		component Component
		id        string
	}
	ordered := make([]keyedComponent, 0, len(components))
	for k, ls := range components {
		c := Component{Key: k}
		for l := range ls {
			c.Licenses = append(c.Licenses, l)
		}
		sort.Strings(c.Licenses)
		ordered = append(ordered, keyedComponent{c, k.ID()})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].id < ordered[j].id })
	r.Components = make([]Component, len(ordered))
	for i, c := range ordered {
		r.Components[i] = c.component
	}
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
	// Preserve the detached, non-nil result without serializing singleton
	// relationships (the common case for Resolved and Candidates).
	if len(in) < 2 {
		out := make([]T, len(in))
		copy(out, in)
		return out
	}
	// Sort one decorated slice; keep the last input for equal JSON keys,
	// exactly as the frozen map-based algorithm did. This avoids a hash
	// index and a separate key list.
	type entry struct {
		key   string
		index int
	}
	keys := make([]entry, len(in))
	for i := range in {
		if o, ok := any(&in[i]).(*Observation); ok {
			var scratch [768]byte
			if raw, plain := observationJSON(scratch[:0], *o); plain {
				keys[i] = entry{string(raw), i}
				continue
			}
		}
		if q, ok := any(&in[i]).(*Requirement); ok {
			var scratch [768]byte
			if raw, plain := requirementJSON(scratch[:0], *q); plain {
				keys[i] = entry{string(raw), i}
				continue
			}
		}
		raw, _ := json.Marshal(&in[i])
		keys[i] = entry{string(raw), i}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].key != keys[j].key {
			return keys[i].key < keys[j].key
		}
		return keys[i].index > keys[j].index
	})
	out := make([]T, 0, len(keys))
	for i, k := range keys {
		if i == 0 || k.key != keys[i-1].key {
			out = append(out, in[k.index])
		}
	}
	return out
}

// SortDiagnostics orders an already bounded failure list without reserializing
// or copying an otherwise normalized component graph. Duplicate failure records
// are harmless evidence; ordinary Normalize still performs exact deduplication.
func (r *Report) SortDiagnostics() {
	slices.SortFunc(r.Diagnostics, func(a, b Diagnostic) int {
		for _, pair := range [][2]string{{a.Code, b.Code}, {a.Stage, b.Stage}, {a.File, b.File}, {a.Reason, b.Reason}} {
			if c := strings.Compare(pair[0], pair[1]); c != 0 {
				return c
			}
		}
		if a.Line < b.Line {
			return -1
		}
		if a.Line > b.Line {
			return 1
		}
		if a.Incomplete == b.Incomplete {
			return 0
		}
		if a.Incomplete {
			return 1
		}
		return -1
	})
}
