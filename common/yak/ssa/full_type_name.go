package ssa

import "github.com/yaklang/yaklang/common/utils"

// maxFullTypeNameEntries caps the number of fullTypeNames stored per type.
// On large Java projects (e.g. Hadoop), types can accumulate hundreds of
// names through inheritance chains and method signatures, causing the
// extra_information JSON to balloon to 3.7GB across 110K types.
// Capping at 200 preserves the most relevant names (the type itself +
// direct ancestors) while preventing unbounded growth.
const maxFullTypeNameEntries = 200

// orderedTypeNames is an ordered list of names, not a map: callers walk it
// from the type itself toward ancestors and never look a name up by key.
// The slice is shared by compile and persist workers, so it cannot be a bare
// []string. SafeSlice is the lock; omap would invent keys this list does not have.
type orderedTypeNames struct {
	items utils.SafeSlice[string]
}

func (n *orderedTypeNames) add(name string) bool {
	if n == nil || name == "" {
		return false
	}
	return n.items.Update(func(cur []string) ([]string, bool) {
		if len(cur) >= maxFullTypeNameEntries {
			return nil, false
		}
		for _, existing := range cur {
			if existing == name {
				return nil, false
			}
		}
		return append(cur, name), true
	})
}

func (n *orderedTypeNames) addList(names []string) bool {
	if n == nil {
		return false
	}
	changed := false
	for _, name := range names {
		if n.add(name) {
			changed = true
		}
	}
	return changed
}

func (n *orderedTypeNames) set(names []string) bool {
	if n == nil {
		return false
	}
	cleaned := dedupFullTypeNames(names)
	return n.items.Update(func(cur []string) ([]string, bool) {
		if len(cur) == len(cleaned) {
			same := true
			for i := range cur {
				if cur[i] != cleaned[i] {
					same = false
					break
				}
			}
			if same {
				return nil, false
			}
		}
		return cleaned, true
	})
}

func (n *orderedTypeNames) list() []string {
	if n == nil {
		return nil
	}
	return n.items.Snapshot()
}

// rewrite replaces names under the slice lock. fn must not retain cur and must
// not call back into this orderedTypeNames. A nil result keeps the current names.
func (n *orderedTypeNames) rewrite(fn func([]string) []string) {
	if n == nil || fn == nil {
		return
	}
	n.items.Update(func(cur []string) ([]string, bool) {
		next := fn(cur)
		if next == nil {
			return nil, false
		}
		return next, true
	})
}

// dedupFullTypeNames keeps the first occurrence of each name, including empty
// strings, and stops at the cap. The result does not share the input array.
func dedupFullTypeNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, min(len(names), maxFullTypeNameEntries))
	out := make([]string, 0, min(len(names), maxFullTypeNameEntries))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
		if len(out) == maxFullTypeNameEntries {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
