package ssa

import (
	"github.com/samber/lo"
)

func fullTypeNameAdd(target *[]string, name string, owner Type) bool {
	if target == nil || name == "" {
		return false
	}
	if lo.Contains(*target, name) {
		return false
	}
	*target = append(*target, name)
	// notifyFullTypeNameChanged(owner)
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
	// notifyFullTypeNameChanged(owner)
	return true
}

// func notifyFullTypeNameChanged(owner Type) {
// 	if owner == nil {
// 		return
// 	}
// 	saveTypeWithValue(value, owner)
// }
