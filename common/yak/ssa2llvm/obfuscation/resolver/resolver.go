package resolver

import (
	"fmt"
	"math/rand"
	"path"
	"sort"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/policy"
)

// Resolution holds the per-obfuscator function assignments produced by Resolve.
type Resolution struct {
	// Selections maps obfuscator name → set of selected function names.
	Selections map[string]map[string]struct{}
}

// FuncsFor returns the set of function names assigned to the given obfuscator.
// Returns nil if the obfuscator has no entry (meaning "use all" semantics).
func (r *Resolution) FuncsFor(name string) map[string]struct{} {
	if r == nil {
		return nil
	}
	return r.Selections[name]
}

// Resolve evaluates a Policy against an Inventory and returns concrete
// per-obfuscator function selections. It enforces:
//   - Selector filters (include/exclude globs, min_blocks, min_insts, allow_entry)
//   - Ratio / count random selection (deterministic when seed != 0)
//   - body-replace exclusivity (at most one body-replace obf per function)
func Resolve(pol *policy.Policy, inv *Inventory) (*Resolution, error) {
	if pol == nil || inv == nil {
		return &Resolution{Selections: map[string]map[string]struct{}{}}, nil
	}

	res := &Resolution{
		Selections: make(map[string]map[string]struct{}, len(pol.Obfuscators)),
	}

	// Track body-replace ownership: funcName → obfuscator that owns it.
	bodyOwner := make(map[string]string)

	for _, entry := range pol.Obfuscators {
		candidates := filterCandidates(inv, &entry)

		selected := applySelection(candidates, &entry.Selector, pol.Seed)

		// Enforce body-replace exclusivity.
		if entry.Category == policy.CategoryBodyReplace {
			for name := range selected {
				if owner, ok := bodyOwner[name]; ok {
					return nil, fmt.Errorf(
						"conflict: function %q claimed by body-replace obfuscator %q, "+
							"cannot also be claimed by %q",
						name, owner, entry.Name)
				}
				bodyOwner[name] = entry.Name
			}
		}

		res.Selections[entry.Name] = selected
	}

	return res, nil
}

// filterCandidates applies include/exclude globs and size thresholds.
func filterCandidates(inv *Inventory, entry *policy.ObfEntry) []string {
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
	sort.Strings(out) // deterministic ordering before selection
	return out
}

// matchIncludeExclude returns true if name matches the include/exclude rules.
// Empty include means "match all". Exclude overrides include.
func matchIncludeExclude(name string, include, exclude []string) bool {
	// Check exclude first.
	for _, pat := range exclude {
		if matched, _ := path.Match(pat, name); matched {
			return false
		}
	}
	// Empty include means "all".
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

// applySelection applies ratio or count selection to the sorted candidate list.
func applySelection(candidates []string, sel *policy.Selector, globalSeed int64) map[string]struct{} {
	result := make(map[string]struct{}, len(candidates))

	if sel.Ratio == nil && sel.Count == nil {
		// No selection constraint → all candidates.
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

	// Ratio selection.
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
		count = 1 // select at least 1 if ratio > 0
	}
	picked := pickN(candidates, count, combineSeed(globalSeed, sel.Seed))
	for _, name := range picked {
		result[name] = struct{}{}
	}
	return result
}

func combineSeed(global int64, local int64) int64 {
	if local != 0 {
		return local
	}
	return global
}

// pickN selects n items from a sorted slice using a deterministic shuffle.
func pickN(sorted []string, n int, seed int64) []string {
	if n >= len(sorted) {
		return sorted
	}
	// Fisher-Yates partial shuffle.
	rng := rand.New(rand.NewSource(seed))
	work := make([]string, len(sorted))
	copy(work, sorted)
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(work)-i)
		work[i], work[j] = work[j], work[i]
	}
	picked := work[:n]
	sort.Strings(picked) // return in deterministic order
	return picked
}
