package model

import "github.com/yaklang/yaklang/common/sca/core/budget"

// NormalizeBudget reserves all controlled normalization containers and JSON
// keys before Normalize starts. Bounds cover map buckets at low occupancy,
// decorated sorting slices, detached output records, and simultaneously live
// JSON bytes/string keys (up to six escaped bytes per source byte, plus growth).
// Runtime stacks, allocator size classes and GC remain outside logical budgets.
func (r *Report) NormalizeBudget(st *budget.State) error {
	var total int64
	add := func(n int64) error {
		var err error
		total, err = budget.SizeAdd(total, n)
		return err
	}
	strings := func(values ...string) error {
		for _, s := range values {
			n, err := budget.SizeMul(len(s), 24)
			if err != nil {
				return err
			}
			if err = add(n); err != nil {
				return err
			}
		}
		return nil
	}
	for _, c := range r.Components {
		if err := add(1024); err != nil {
			return err
		}
		k := c.Key
		if err := strings(k.Ecosystem, k.Name, k.Version, k.Source, k.Architecture, k.Variant, k.Verification); err != nil {
			return err
		}
		for _, l := range c.Licenses {
			if err := add(256); err != nil {
				return err
			}
			if err := strings(l); err != nil {
				return err
			}
		}
	}
	for _, o := range r.Observations {
		if err := add(1024); err != nil {
			return err
		}
		if err := strings(o.Condition, o.Scope, o.Component, o.Snapshot, o.Project, o.File, o.NativeID, o.Kind, o.DeclaredIntegrity); err != nil {
			return err
		}
		for _, p := range o.Provides {
			if err := add(128); err != nil {
				return err
			}
			if err := strings(p); err != nil {
				return err
			}
		}
	}
	for _, q := range r.Requirements {
		if err := add(1024); err != nil {
			return err
		}
		if err := strings(q.From, q.Target, q.Constraint, q.Scope, q.Condition, q.Group, q.Operator); err != nil {
			return err
		}
		for _, list := range [][]string{q.Resolved, q.Candidates} {
			for _, id := range list {
				if err := add(256); err != nil {
					return err
				}
				if err := strings(id); err != nil {
					return err
				}
			}
		}
	}
	for _, d := range r.Diagnostics {
		if err := add(512); err != nil {
			return err
		}
		if err := strings(d.Code, d.Stage, d.File, d.Reason); err != nil {
			return err
		}
	}
	if err := st.Working(total); err != nil {
		return err
	}
	r.Normalize()
	return nil
}
