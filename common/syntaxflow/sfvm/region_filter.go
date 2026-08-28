package sfvm

// region_filter.go — delegation hook for the region algebra over pattern hits.
//
// The containment/overlap implementation lives in sfpattern (it operates on
// SimpleValue hits and belongs with the pattern-matching code), but sfvm
// cannot import sfpattern (sfpattern imports sfvm). The VM side therefore
// delegates through a registered RegionFilterFunc — the same injection
// pattern as the FileFilter matcher. sfpattern registers its implementation
// in init().

// RegionFilterFunc implements the region algebra over pattern hits.
type RegionFilterFunc interface {
	FilterContained(target, ctx Values) Values
	FilterNotContained(target, ctx Values) Values
	FilterOverlap(target Values, others ...Values) Values
	FilterNotOverlap(target Values, others ...Values) Values
	AllSimpleHits(vs Values) bool
}

var regionFilter RegionFilterFunc

// RegisterRegionFilter installs the region algebra implementation
// (called by sfpattern's init).
func RegisterRegionFilter(fn RegionFilterFunc) {
	regionFilter = fn
}

// RegionContained delegates to the registered implementation. Defensive
// fallbacks keep the VM usable when sfpattern is not linked in.
func RegionContained(target, ctx Values) Values {
	if regionFilter == nil {
		return NewEmptyValues()
	}
	return regionFilter.FilterContained(target, ctx)
}

// RegionNotContained delegates to the registered implementation.
func RegionNotContained(target, ctx Values) Values {
	if regionFilter == nil {
		return target
	}
	return regionFilter.FilterNotContained(target, ctx)
}

// RegionOverlap delegates to the registered implementation.
func RegionOverlap(target Values, others ...Values) Values {
	if regionFilter == nil {
		return target
	}
	return regionFilter.FilterOverlap(target, others...)
}

// RegionNotOverlap delegates to the registered implementation.
func RegionNotOverlap(target Values, others ...Values) Values {
	if regionFilter == nil {
		return target
	}
	return regionFilter.FilterNotOverlap(target, others...)
}

// RegionAllSimpleHits delegates to the registered implementation.
func RegionAllSimpleHits(vs Values) bool {
	if regionFilter == nil {
		return false
	}
	return regionFilter.AllSimpleHits(vs)
}
