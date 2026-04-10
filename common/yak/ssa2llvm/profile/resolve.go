package profile

import (
	"fmt"
	"math/rand"
	"path"
	"sort"
)

// Resolution holds the per-obfuscator function assignments produced by Resolve.
type Resolution struct {
	Selections map[string]map[string]struct{}
}

// FuncsFor returns the set of function names assigned to the given obfuscator.
func (r *Resolution) FuncsFor(name string) map[string]struct{} {
	if r == nil {
		return nil
	}
	return r.Selections[name]
}

// Resolve evaluates a Profile against an Inventory and returns concrete
// per-obfuscator function selections.
func Resolve(prof *Profile, inv *Inventory) (*Resolution, error) {
	if prof == nil || inv == nil {
		return &Resolution{Selections: map[string]map[string]struct{}{}}, nil
	}

	res := &Resolution{
		Selections: make(map[string]map[string]struct{}, len(prof.Obfuscators)),
	}

	bodyOwner := make(map[string]string)
	for _, entry := range prof.Obfuscators {
		candidates := filterCandidates(inv, &entry)
		selected := applySelection(candidates, &entry.Selector, prof.SelectionSeed)

		if entry.EffectiveCategory() == CategoryBodyReplace {
			for name := range selected {
				if owner, ok := bodyOwner[name]; ok {
					return nil, fmt.Errorf(
						"conflict: function %q claimed by body-replace obfuscator %q, cannot also be claimed by %q",
						name, owner, entry.Name,
					)
				}
				bodyOwner[name] = entry.Name
			}
		}

		res.Selections[entry.Name] = selected
	}

	return res, nil
}

func filterCandidates(inv *Inventory, entry *ObfEntry) []string {
	sel := &entry.Selector
	var out []string
	for _, fi := range inv.Funcs {
		if fi.IsExtern {
			continue
		}
		if fi.IsEntry && !sel.AllowEntry {
			continue
		}
		if sel.MinBlocks > 0 && fi.BlockCount < sel.MinBlocks {
			continue
		}
		if sel.MinInsts > 0 && fi.InstCount < sel.MinInsts {
			continue
		}
		if !matchIncludeExclude(fi.Name, sel.Include, sel.Exclude) {
			continue
		}
		out = append(out, fi.Name)
	}
	sort.Strings(out)
	return out
}

func matchIncludeExclude(name string, include, exclude []string) bool {
	for _, pat := range exclude {
		if matched, _ := path.Match(pat, name); matched {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, pat := range include {
		if matched, _ := path.Match(pat, name); matched {
			return true
		}
	}
	return false
}

func applySelection(candidates []string, sel *Selector, globalSeed int64) map[string]struct{} {
	result := make(map[string]struct{}, len(candidates))

	if sel.Ratio == nil && sel.Count == nil {
		for _, name := range candidates {
			result[name] = struct{}{}
		}
		return result
	}

	if sel.Count != nil {
		count := *sel.Count
		if count >= len(candidates) {
			for _, name := range candidates {
				result[name] = struct{}{}
			}
			return result
		}
		if count <= 0 {
			return result
		}
		picked := pickN(candidates, count, combineSeed(globalSeed, sel.Seed))
		for _, name := range picked {
			result[name] = struct{}{}
		}
		return result
	}

	ratio := *sel.Ratio
	if ratio >= 1.0 {
		for _, name := range candidates {
			result[name] = struct{}{}
		}
		return result
	}
	if ratio <= 0.0 {
		return result
	}

	count := int(float64(len(candidates)) * ratio)
	if count < 1 {
		count = 1
	}
	picked := pickN(candidates, count, combineSeed(globalSeed, sel.Seed))
	for _, name := range picked {
		result[name] = struct{}{}
	}
	return result
}

func combineSeed(global, local int64) int64 {
	if local != 0 {
		return local
	}
	return global
}

func pickN(sorted []string, n int, seed int64) []string {
	if n >= len(sorted) {
		return sorted
	}
	rng := rand.New(rand.NewSource(seed))
	work := make([]string, len(sorted))
	copy(work, sorted)
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(work)-i)
		work[i], work[j] = work[j], work[i]
	}
	picked := work[:n]
	sort.Strings(picked)
	return picked
}
