package sfvm

// region_filter.go — delegation hook for the region algebra over pattern hits.
//
// The containment/overlap implementation lives in sfpattern (it operates on
// SimpleValue hits and belongs with the pattern-matching code), but sfvm
// cannot import sfpattern (sfpattern imports sfvm). The VM side therefore
// delegates through a registered RegionFilterFunc — the same injection pattern
// as the FileFilter matcher. sfpattern registers its implementation in init().

// RegionFilterFunc implements the region algebra over pattern hits. The
// implementation receives the per-rule sfvm.Config so it can honor context
// cancellation and the per-rule work budget.
type RegionFilterFunc interface {
	FilterContained(target, ctx Values, cfg *Config) Values
	FilterNotContained(target, ctx Values, cfg *Config) Values
	FilterOverlap(target Values, cfg *Config, others ...Values) Values
	FilterNotOverlap(target Values, cfg *Config, others ...Values) Values
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
func RegionContained(target, ctx Values, cfg ...*Config) Values {
	if regionFilter == nil {
		return NewEmptyValues()
	}
	var c *Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return regionFilter.FilterContained(target, ctx, c)
}

// RegionNotContained delegates to the registered implementation.
func RegionNotContained(target, ctx Values, cfg ...*Config) Values {
	if regionFilter == nil {
		return target
	}
	var c *Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return regionFilter.FilterNotContained(target, ctx, c)
}

// RegionOverlap delegates to the registered implementation.
func RegionOverlap(target Values, cfg *Config, others ...Values) Values {
	if regionFilter == nil {
		return target
	}
	return regionFilter.FilterOverlap(target, cfg, others...)
}

// RegionNotOverlap delegates to the registered implementation.
func RegionNotOverlap(target Values, cfg *Config, others ...Values) Values {
	if regionFilter == nil {
		return target
	}
	return regionFilter.FilterNotOverlap(target, cfg, others...)
}

// RegionAllSimpleHits delegates to the registered implementation.
func RegionAllSimpleHits(vs Values) bool {
	if regionFilter == nil {
		return false
	}
	return regionFilter.AllSimpleHits(vs)
}
