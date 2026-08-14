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
	cleaned := clean(names)
	// Cap the list size
	if len(cleaned) > maxFullTypeNameEntries {
		cleaned = cleaned[:maxFullTypeNameEntries]
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
	*target = cleaned
	return true
}
