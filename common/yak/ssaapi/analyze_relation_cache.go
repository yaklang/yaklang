package ssaapi

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// analyzeRelationCache caches SSA adjacency lists by instruction id.
// It is scoped to a single AnalyzeContext (one topdef/bottomuse traversal).
//
// Important: cache values are SSA nodes (interfaces backed by pointers). We never cache *ssaapi.Value
// because Value carries per-traversal state (runtimeCtx, edges, predecessors) that must not leak across paths.
type analyzeRelationCache struct {
	// ssa.Value(id) -> users (includes pointer targets' users, matching (*Value).GetUsers())
	users map[int64][]ssa.User
	// ssa.Value(id) -> GetValues() (matching i.getValue().GetValues() usages in topdef)
	valueValues map[int64][]ssa.Value
	// ssa.User(id) -> GetValues() (used for return/phi-like nodes that are users)
	userValues map[int64][]ssa.Value
	// ssa.Value(id) -> mask values (ssa.Maskable.GetMask())
	mask map[int64][]ssa.Value

	usersHit      uint64
	usersMiss     uint64
	valueValsHit  uint64
	valueValsMiss uint64
	userValsHit   uint64
	userValsMiss  uint64
	maskHit       uint64
	maskMiss      uint64
}

type analyzeRelationCacheStats struct {
	UsersHit      uint64
	UsersMiss     uint64
	ValueValsHit  uint64
	ValueValsMiss uint64
	UserValsHit   uint64
	UserValsMiss  uint64
	MaskHit       uint64
	MaskMiss      uint64
}

func newAnalyzeRelationCache() *analyzeRelationCache {
	return &analyzeRelationCache{
		users:       make(map[int64][]ssa.User),
		valueValues: make(map[int64][]ssa.Value),
		userValues:  make(map[int64][]ssa.Value),
		mask:        make(map[int64][]ssa.Value),
	}
}

func (a *AnalyzeContext) cache() *analyzeRelationCache {
	if a == nil {
		return nil
	}
	if a.relationCache == nil {
		a.relationCache = newAnalyzeRelationCache()
	}
	return a.relationCache
}

func (a *AnalyzeContext) cachedUsers(v *Value) []ssa.User {
	if a == nil || v == nil {
		return nil
	}
	node := v.getValue()
	if node == nil {
		return nil
	}
	c := a.cache()
	if c == nil {
		return nil
	}
	id := node.GetId()
	if users, ok := c.users[id]; ok {
		atomic.AddUint64(&c.usersHit, 1)
		return users
	}
	atomic.AddUint64(&c.usersMiss, 1)

	seen := make(map[int64]struct{})
	out := make([]ssa.User, 0)
	appendUsers := func(n ssa.Value) {
		if n == nil {
			return
		}
		for _, u := range n.GetUsers() {
			if u == nil {
				continue
			}
			uid := u.GetId()
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}
			out = append(out, u)
		}
	}

	// Match (*Value).GetUsers(): current node + its pointer targets.
	appendUsers(node)
	for _, ref := range node.GetPointer() {
		appendUsers(ref)
	}

	c.users[id] = out
	return out
}

func (a *AnalyzeContext) usersAsValues(base *Value) Values {
	if a == nil || base == nil {
		return nil
	}
	users := a.cachedUsers(base)
	if len(users) == 0 {
		return nil
	}
	out := make(Values, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, base.NewValue(u))
	}
	return out
}

func (a *AnalyzeContext) cachedValueValues(v *Value) []ssa.Value {
	if a == nil || v == nil {
		return nil
	}
	node := v.getValue()
	if node == nil {
		return nil
	}
	c := a.cache()
	if c == nil {
		return nil
	}
	id := node.GetId()
	if vals, ok := c.valueValues[id]; ok {
		atomic.AddUint64(&c.valueValsHit, 1)
		return vals
	}
	atomic.AddUint64(&c.valueValsMiss, 1)
	raw := node.GetValues()
	out := make([]ssa.Value, 0, len(raw))
	for _, sv := range raw {
		if sv == nil {
			continue
		}
		out = append(out, sv)
	}
	c.valueValues[id] = out
	return out
}

func (a *AnalyzeContext) cachedUserValues(u ssa.User) []ssa.Value {
	if a == nil || u == nil {
		return nil
	}
	c := a.cache()
	if c == nil {
		return nil
	}
	id := u.GetId()
	if vals, ok := c.userValues[id]; ok {
		atomic.AddUint64(&c.userValsHit, 1)
		return vals
	}
	atomic.AddUint64(&c.userValsMiss, 1)
	raw := u.GetValues()
	out := make([]ssa.Value, 0, len(raw))
	for _, sv := range raw {
		if sv == nil {
			continue
		}
		out = append(out, sv)
	}
	c.userValues[id] = out
	return out
}

func (a *AnalyzeContext) cachedMaskValues(ownerID int64, m ssa.Maskable) []ssa.Value {
	if a == nil || m == nil || ownerID == 0 {
		return nil
	}
	c := a.cache()
	if c == nil {
		return nil
	}
	if vals, ok := c.mask[ownerID]; ok {
		atomic.AddUint64(&c.maskHit, 1)
		return vals
	}
	atomic.AddUint64(&c.maskMiss, 1)
	raw := m.GetMask()
	out := make([]ssa.Value, 0, len(raw))
	for _, sv := range raw {
		if sv == nil {
			continue
		}
		out = append(out, sv)
	}
	c.mask[ownerID] = out
	return out
}

func (a *AnalyzeContext) relationCacheStats() analyzeRelationCacheStats {
	if a == nil || a.relationCache == nil {
		return analyzeRelationCacheStats{}
	}
	c := a.relationCache
	return analyzeRelationCacheStats{
		UsersHit:      atomic.LoadUint64(&c.usersHit),
		UsersMiss:     atomic.LoadUint64(&c.usersMiss),
		ValueValsHit:  atomic.LoadUint64(&c.valueValsHit),
		ValueValsMiss: atomic.LoadUint64(&c.valueValsMiss),
		UserValsHit:   atomic.LoadUint64(&c.userValsHit),
		UserValsMiss:  atomic.LoadUint64(&c.userValsMiss),
		MaskHit:       atomic.LoadUint64(&c.maskHit),
		MaskMiss:      atomic.LoadUint64(&c.maskMiss),
	}
}
