package sfvm

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// SymbolSnapshot captures the named-symbol state of a Values table at one point
// in time, so a child query's result can be checked for NEW named output without
// re-merging the inherited parent vars (the N×M duplicate merge that was the
// #1 allocator on large projects — MergeValues ~463GB / 27%).
//
// It is the corrected mechanism for Opt A's "skip useless merge": snapshot the
// parent's named keys + value dedupKeys ONCE when a sfCheck is created (before
// any path/query runs), then for each child result ask HasNewNamedValue. A child
// that only re-contains inherited parent vars (unchanged) + magic ($__) vars
// has no new named output → skip the symbol merge (the saving). A child that
// produced a new named key, OR a new value (by dedupKey) for an existing key
// (a re-bind, or a later path's values for a key earlier paths added), has new
// output → merge. Snapshotting once (not per-path) is what makes the across-
// path case correct: a key merged by path 1 is NOT in the original snapshot, so
// path 2's re-bind of it is correctly seen as "new" and merged.
type SymbolSnapshot struct {
	keys      map[string]struct{} // named (non-empty, non-__) keys at snapshot time
	dedupKeys map[dedupKey]struct{}
}

// TakeSymbolSnapshot captures the named-symbol state of table. nil table → empty
// snapshot (every child key/value is "new"). Caller is responsible for any
// concurrency locking around table access.
func TakeSymbolSnapshot(table *omap.OrderedMap[string, Values]) *SymbolSnapshot {
	s := &SymbolSnapshot{
		keys:      make(map[string]struct{}),
		dedupKeys: make(map[dedupKey]struct{}),
	}
	if table == nil {
		return s
	}
	table.ForEach(func(key string, vals Values) bool {
		if !isNamedSymbol(key) {
			return true
		}
		s.keys[key] = struct{}{}
		for _, v := range vals {
			if utils.IsNil(v) || v.IsEmpty() {
				continue
			}
			if dk, ok := valueDedupKey(v); ok {
				s.dedupKeys[dk] = struct{}{}
			}
		}
		return true
	})
	return s
}

// isNamedSymbol reports whether key is a named (non-empty, non-magic-`__`-
// prefixed) symbol — the kind that must surface in the parent result. Mirrors
// the `__` prefix convention used by isMatch (sf_config.go) and sanitizeChildResult.
func isNamedSymbol(key string) bool {
	return key != "" && !strings.HasPrefix(key, "__")
}

// HasNewNamedValue reports whether result contains a named key NOT in the
// snapshot (a new var the sub-rule produced), OR a named value whose dedupKey
// is NOT in the snapshot (a new value for an existing key — a re-bind or a
// later path's values). If neither, the child only re-contains inherited
// parent vars (unchanged) + magic vars → no new named output → the symbol merge
// can be skipped.
//
// A value with no dedupKey (nil-id, no Hash) can't be proven "already seen", so
// it's treated as new (merge) — the safe direction; MergeValues would keep it
// too.
func (s *SymbolSnapshot) HasNewNamedValue(result *SFFrameResult) bool {
	if s == nil || result == nil {
		return false
	}
	if result.SymbolTable != nil {
		stop := false
		result.SymbolTable.ForEach(func(key string, vals Values) bool {
			if s.keyOrValuesIsNew(key, vals) {
				stop = true
				return false
			}
			return true
		})
		if stop {
			return true
		}
	}
	if result.AlertSymbolTable != nil {
		stop := false
		result.AlertSymbolTable.ForEach(func(key string, vals Values) bool {
			if s.keyOrValuesIsNew(key, vals) {
				stop = true
				return false
			}
			return true
		})
		if stop {
			return true
		}
	}
	return false
}

// keyOrValuesIsNew reports whether key is a new named key, or any of vals is a
// new named value (dedupKey not in the snapshot).
func (s *SymbolSnapshot) keyOrValuesIsNew(key string, vals Values) bool {
	if !isNamedSymbol(key) {
		return false
	}
	if _, ok := s.keys[key]; !ok {
		return true // new named key
	}
	for _, v := range vals {
		if utils.IsNil(v) || v.IsEmpty() {
			continue
		}
		dk, ok := valueDedupKey(v)
		if !ok {
			return true // can't prove seen → treat as new (safe)
		}
		if _, seen := s.dedupKeys[dk]; !seen {
			return true // new value for an existing key
		}
	}
	return false
}