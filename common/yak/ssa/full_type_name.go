package ssa

import (
	"github.com/samber/lo"
)

// maxFullTypeNameEntries caps the number of fullTypeNames stored per type.
// On large Java projects (e.g. Hadoop), types can accumulate hundreds of
// names through inheritance chains and method signatures, causing the
// extra_information JSON to balloon to 3.7GB across 110K types.
// Capping at 200 preserves the most relevant names (the type itself +
// direct ancestors) while preventing unbounded growth.
const maxFullTypeNameEntries = 200

func fullTypeNameAdd(target *[]string, name string, owner Type) bool {
	if target == nil || name == "" {
		return false
	}
	if len(*target) >= maxFullTypeNameEntries {
		return false
	}
	if lo.Contains(*target, name) {
		return false
	}
	*target = append(*target, name)
	return true
}

func fullTypeNameAddList(target *[]string, names []string, owner Type) bool {
	if target == nil {
		return false
	}
	changed := false
	for _, name := range names {
		if fullTypeNameAdd(target, name, owner) {
			changed = true
		}
	}
	if changed {
		return true
	}
	return false
}

func fullTypeNameSet(target *[]string, names []string, owner Type) bool {
	if target == nil {
		return false
	}
	if len(names) <= 8 && len(*target) == len(names) {
		same := true
		for i, name := range names {
			if (*target)[i] != name {
				same = false
				break
			}
		}
		// Existing lists may be edited through GetFullTypeNames, so exact
		// equality alone is insufficient if duplicate names were introduced.
		if same {
			unique := true
			for i, name := range names {
				if lo.Contains(names[:i], name) {
					unique = false
					break
				}
			}
			if unique {
				return false
			}
		}
	}
	// Stop after the same first 200 distinct names as before. Cleaning the
	// entire input first retains its oversized backing array after slicing.
	// Stack storage also avoids allocations when the effective list is unchanged.
	var scratch [maxFullTypeNameEntries]string
	cleaned := scratch[:0]
	seen := make(map[string]struct{}, min(len(names), maxFullTypeNameEntries))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
		if len(cleaned) == maxFullTypeNameEntries {
			break
		}
	}
	if len(*target) == len(cleaned) {
		same := true
		for i, name := range cleaned {
			if (*target)[i] != name {
				same = false
				break
			}
		}
		if same {
			return false
		}
	}
	// Do not share input storage: callers may mutate their slice afterwards.
	*target = append([]string(nil), cleaned...)
	return true
}
