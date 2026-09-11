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
	"context"
	"sort"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	sfvm.RegisterRegionFilter(regionFilterImpl{})
}

// regionFilterImpl adapts the package-level functions to sfvm.RegionFilterFunc.
type regionFilterImpl struct{}

func (regionFilterImpl) FilterContained(target, ctx sfvm.Values, cfg *sfvm.Config) sfvm.Values {
	return filterContained(target, ctx, newFilterOptions(cfg))
}

func (regionFilterImpl) FilterNotContained(target, ctx sfvm.Values, cfg *sfvm.Config) sfvm.Values {
	return filterNotContained(target, ctx, newFilterOptions(cfg))
}

func (regionFilterImpl) FilterOverlap(target sfvm.Values, cfg *sfvm.Config, others ...sfvm.Values) sfvm.Values {
	return filterOverlap(target, newFilterOptions(cfg), others...)
}

func (regionFilterImpl) FilterNotOverlap(target sfvm.Values, cfg *sfvm.Config, others ...sfvm.Values) sfvm.Values {
	return filterNotOverlap(target, newFilterOptions(cfg), others...)
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
// Kept for direct use / tests; the hot path uses regionIndex.contains.
func regionContains(r, t hitRegion) bool {
	return r.path == t.path && r.start <= t.start && t.end <= r.end
}

// regionOverlaps reports whether r and t overlap (same file, strict overlap).
// Kept for direct use / tests; the hot path uses regionIndex.overlaps.
func regionOverlaps(r, t hitRegion) bool {
	return r.path == t.path && t.start < r.end && r.start < t.end
}

// regionIndex provides per-file sorted hit regions for O(log n) contains/overlap
// queries. This replaces the previous O(n*m) linear scans in FilterContained,
// FilterNotContained, FilterOverlap and FilterNotOverlap.
type regionIndex struct {
	// byFile maps a file path to its sorted non-empty hit regions.
	byFile map[string][]hitRegion
}

// newRegionIndex builds an index from a slice of hit regions.
func newRegionIndex(regions []hitRegion) regionIndex {
	idx := regionIndex{byFile: make(map[string][]hitRegion, len(regions))}
	for _, r := range regions {
		idx.byFile[r.path] = append(idx.byFile[r.path], r)
	}
	for path, rs := range idx.byFile {
		if len(rs) <= 1 {
			continue
		}
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].start == rs[j].start {
				return rs[i].end < rs[j].end
			}
			return rs[i].start < rs[j].start
		})
		// Deduplicate exact duplicates to keep searches tight.
		compact := rs[:0]
		for _, r := range rs {
			if len(compact) == 0 || r != compact[len(compact)-1] {
				compact = append(compact, r)
			}
		}
		idx.byFile[path] = compact
	}
	return idx
}

// contains reports whether any region in the same file fully contains t.
func (idx regionIndex) contains(t hitRegion) bool {
	rs, ok := idx.byFile[t.path]
	if !ok || len(rs) == 0 {
		return false
	}
	// Find the rightmost region with start <= t.start. Only that region (and
	// possibly the previous one with same start) can contain t.
	i := sort.Search(len(rs), func(i int) bool { return rs[i].start > t.start })
	if i > 0 {
		cand := rs[i-1]
		if cand.start <= t.start && t.end <= cand.end {
			return true
		}
	}
	return false
}

// overlaps reports whether any region in the same file overlaps t.
func (idx regionIndex) overlaps(t hitRegion) bool {
	rs, ok := idx.byFile[t.path]
	if !ok || len(rs) == 0 {
		return false
	}
	// Find the first region whose end > t.start. That is the only candidate that
	// can overlap t (all earlier regions end before t starts).
	i := sort.Search(len(rs), func(i int) bool { return rs[i].end > t.start })
	if i < len(rs) {
		cand := rs[i]
		if cand.start < t.end && t.start < cand.end {
			return true
		}
	}
	return false
}

// bailContext bundles cancellation/budget callbacks so long-running filters can
// bail out cooperatively. nil-safe: all methods are no-ops when the receiver is
// nil.
type bailContext struct {
	ctx        context.Context
	workBudget *sfvm.RuleWorkBudget
}

// check returns a non-nil error when the rule should stop. It also records one
// work unit against the budget if workBudget is present.
func (b *bailContext) check() error {
	if b == nil {
		return nil
	}
	if b.ctx != nil {
		select {
		case <-b.ctx.Done():
			return b.ctx.Err()
		default:
		}
	}
	if b.workBudget != nil && b.workBudget.EnterWork() {
		return utils.Errorf("work budget exceeded")
	}
	return nil
}

// filterOptions carries optional cancellation/budget through the filter API.
type filterOptions struct {
	bail *bailContext
}

// newFilterOptions builds options from an sfvm.Config. If cfg is nil, returns
// an empty options struct (no cancellation, no budget accounting).
func newFilterOptions(cfg *sfvm.Config) filterOptions {
	if cfg == nil {
		return filterOptions{}
	}
	return filterOptions{bail: &bailContext{
		ctx:        cfg.GetContext(),
		workBudget: cfg.GetWorkBudget(),
	}}
}

// FilterContained keeps target hits that are contained in at least one
// context region (Semgrep pattern-inside). Non-hit values pass through.
// With no context regions the result is empty (nothing is inside nothing).
func FilterContained(target, ctx sfvm.Values) sfvm.Values {
	return filterContained(target, ctx, filterOptions{})
}

func filterContained(target, ctx sfvm.Values, opts filterOptions) sfvm.Values {
	idx := newRegionIndex(collectRegions(ctx))
	if len(idx.byFile) == 0 {
		return sfvm.NewEmptyValues()
	}
	var out []sfvm.ValueOperator
	var n int
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		n++
		if n%1024 == 0 {
			if err := opts.bail.check(); err != nil {
				return err
			}
		}
		if idx.contains(t) {
			out = append(out, v)
		}
		return nil
	})
	return sfvm.NewValues(out)
}

// FilterNotContained drops target hits contained in any context region
// (Semgrep pattern-not-inside). Non-hit values pass through. With no
// context regions everything is kept.
func FilterNotContained(target, ctx sfvm.Values) sfvm.Values {
	return filterNotContained(target, ctx, filterOptions{})
}

func filterNotContained(target, ctx sfvm.Values, opts filterOptions) sfvm.Values {
	idx := newRegionIndex(collectRegions(ctx))
	if len(idx.byFile) == 0 {
		return target
	}
	var out []sfvm.ValueOperator
	var n int
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		n++
		if n%1024 == 0 {
			if err := opts.bail.check(); err != nil {
				return err
			}
		}
		if !idx.contains(t) {
			out = append(out, v)
		}
		return nil
	})
	return sfvm.NewValues(out)
}

// FilterOverlap keeps target hits that overlap at least one hit of EVERY
// other set (Semgrep AND of multiple pattern-regex: range intersection
// non-empty). Non-hit values pass through. An empty others list keeps
// everything; a set with no hit regions contributes no constraint.
func FilterOverlap(target sfvm.Values, others ...sfvm.Values) sfvm.Values {
	return filterOverlap(target, filterOptions{}, others...)
}

func filterOverlap(target sfvm.Values, opts filterOptions, others ...sfvm.Values) sfvm.Values {
	if len(others) == 0 {
		return target
	}
	indexes := make([]regionIndex, 0, len(others))
	for _, other := range others {
		idx := newRegionIndex(collectRegions(other))
		if len(idx.byFile) == 0 {
			// A required AND operand has no hits → nothing can overlap it.
			return sfvm.NewEmptyValues()
		}
		indexes = append(indexes, idx)
	}
	var out []sfvm.ValueOperator
	var n int
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		n++
		if n%1024 == 0 {
			if err := opts.bail.check(); err != nil {
				return err
			}
		}
		for _, idx := range indexes {
			if !idx.overlaps(t) {
				return nil
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
	return filterNotOverlap(target, filterOptions{}, others...)
}

func filterNotOverlap(target sfvm.Values, opts filterOptions, others ...sfvm.Values) sfvm.Values {
	if len(others) == 0 {
		return target
	}
	allRegions := make([]hitRegion, 0)
	for _, other := range others {
		allRegions = append(allRegions, collectRegions(other)...)
	}
	idx := newRegionIndex(allRegions)
	if len(idx.byFile) == 0 {
		return target
	}
	var out []sfvm.ValueOperator
	var n int
	_ = target.Recursive(func(v sfvm.ValueOperator) error {
		t, ok := asHitRegion(v)
		if !ok {
			out = append(out, v)
			return nil
		}
		n++
		if n%1024 == 0 {
			if err := opts.bail.check(); err != nil {
				return err
			}
		}
		if !idx.overlaps(t) {
			out = append(out, v)
		}
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
