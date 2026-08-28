package sfpattern

// hit_region.go — region algebra over SimpleValue pattern hits.
//
// Source-mode rules (sfpattern) produce sfvm.SimpleValue hits that carry
// (path, start, end) byte ranges. These helpers implement the Semgrep
// context semantics on top of those ranges:
//
//   - FilterContained    — pattern-inside: keep hits contained in a context region
//   - FilterNotContained — pattern-not-inside: drop hits contained in a context region
//   - FilterOverlap      — multiple pattern-regex AND: keep hits overlapping every
//     other positive hit set (Semgrep range-intersection semantics)
//
// Non-SimpleValue values are passed through unchanged (defensive: these
// operators are only meaningful for pattern hits; SSA values keep the
// ID-based set operations in sfvm/values.go).
//
// sfvm cannot import sfpattern (sfpattern imports sfvm), so the VM side
// delegates through sfvm.RegisterRegionFilter — the same injection pattern
// as the FileFilter matcher. init() below performs the registration.

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	sfvm.RegisterRegionFilter(regionFilterImpl{})
}

// regionFilterImpl adapts the package-level functions to sfvm.RegionFilterFunc.
type regionFilterImpl struct{}

func (regionFilterImpl) FilterContained(target, ctx sfvm.Values) sfvm.Values {
	return FilterContained(target, ctx)
}

func (regionFilterImpl) FilterNotContained(target, ctx sfvm.Values) sfvm.Values {
	return FilterNotContained(target, ctx)
}

func (regionFilterImpl) FilterOverlap(target sfvm.Values, others ...sfvm.Values) sfvm.Values {
	return FilterOverlap(target, others...)
}

func (regionFilterImpl) FilterNotOverlap(target sfvm.Values, others ...sfvm.Values) sfvm.Values {
	return FilterNotOverlap(target, others...)
}

func (regionFilterImpl) AllSimpleHits(vs sfvm.Values) bool {
	return allSimpleHits(vs)
}

// hitRegion is the minimal range view used by the region algebra.
type hitRegion struct {
	path  string
	start int
	end   int
}

// asHitRegion extracts a region from a value; ok=false for non-hits
// (consts, SSA values, nil).
func asHitRegion(v sfvm.ValueOperator) (hitRegion, bool) {
	if utils.IsNil(v) {
		return hitRegion{}, false
	}
	sv, ok := v.(*sfvm.SimpleValue)
	if !ok || sv == nil || sv.Path() == "" {
		return hitRegion{}, false
	}
	return hitRegion{path: sv.Path(), start: sv.Start(), end: sv.End()}, true
}

// collectRegions gathers all hit regions from a value set.
func collectRegions(vs sfvm.Values) []hitRegion {
	if vs.IsEmpty() {
		return nil
	}
	var out []hitRegion
	_ = vs.Recursive(func(v sfvm.ValueOperator) error {
		if r, ok := asHitRegion(v); ok {
			out = append(out, r)
		}
		return nil
	})
	return out
}

// regionContains reports whether r contains t (same file, inclusive bounds).
func regionContains(r, t hitRegion) bool {
	return r.path == t.path && r.start <= t.start && t.end <= r.end
}

// regionOverlaps reports whether r and t overlap (same file, strict overlap).
func regionOverlaps(r, t hitRegion) bool {
	return r.path == t.path && t.start < r.end && r.start < t.end
}

// FilterContained keeps target hits that are contained in at least one
// context region (Semgrep pattern-inside). Non-hit values pass through.
// With no context regions the result is empty (nothing is inside nothing).
func FilterContained(target, ctx sfvm.Values) sfvm.Values {
	regions := collectRegions(ctx)
	if len(regions) == 0 {
		return sfvm.NewEmptyValues()
	}
	var out []sfvm.ValueOperator
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		for _, r := range regions {
			if regionContains(r, t) {
				out = append(out, v)
				return nil
			}
		}
		return nil
	})
	return sfvm.NewValues(out)
}

// FilterNotContained drops target hits contained in any context region
// (Semgrep pattern-not-inside). Non-hit values pass through. With no
// context regions everything is kept.
func FilterNotContained(target, ctx sfvm.Values) sfvm.Values {
	regions := collectRegions(ctx)
	if len(regions) == 0 {
		return target
	}
	var out []sfvm.ValueOperator
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		for _, r := range regions {
			if regionContains(r, t) {
				return nil // dropped
			}
		}
		out = append(out, v)
		return nil
	})
	return sfvm.NewValues(out)
}

// FilterOverlap keeps target hits that overlap at least one hit of EVERY
// other set (Semgrep AND of multiple pattern-regex: range intersection
// non-empty). Non-hit values pass through. An empty others list keeps
// everything; a set with no hit regions contributes no constraint.
func FilterOverlap(target sfvm.Values, others ...sfvm.Values) sfvm.Values {
	if len(others) == 0 {
		return target
	}
	regionSets := make([][]hitRegion, 0, len(others))
	for _, other := range others {
		regionSets = append(regionSets, collectRegions(other))
	}
	var out []sfvm.ValueOperator
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		for _, regions := range regionSets {
			matched := false
			for _, r := range regions {
				if regionOverlaps(r, t) {
					matched = true
					break
				}
			}
			if !matched {
				return nil // fails this AND constraint
			}
		}
		out = append(out, v)
		return nil
	})
	return sfvm.NewValues(out)
}

// FilterNotOverlap keeps target hits that overlap NO hit of any other set
// (Semgrep pattern-not-regex: the match must not overlap a negative match).
// Non-hit values pass through. An empty others list keeps everything.
func FilterNotOverlap(target sfvm.Values, others ...sfvm.Values) sfvm.Values {
	if len(others) == 0 {
		return target
	}
	var regions []hitRegion
	for _, other := range others {
		regions = append(regions, collectRegions(other)...)
	}
	if len(regions) == 0 {
		return target
	}
	var out []sfvm.ValueOperator
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		for _, r := range regions {
			if regionOverlaps(r, t) {
				return nil // dropped
			}
		}
		out = append(out, v)
		return nil
	})
	return sfvm.NewValues(out)
}

// allSimpleHits reports whether every value in the set is a SimpleValue
// pattern hit (path non-empty). Used for type-aware dispatch: overlap-based
// set semantics apply only when both operands are pattern hits; SSA values
// keep the ID-based operations in sfvm/values.go.
func allSimpleHits(vs sfvm.Values) bool {
	if vs.IsEmpty() {
		return false
	}
	all := true
	_ = vs.Recursive(func(v sfvm.ValueOperator) error {
		if _, ok := asHitRegion(v); !ok {
			all = false
		}
		return nil
	})
	return all
}
