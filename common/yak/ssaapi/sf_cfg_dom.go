package ssaapi

import (
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// ---- CFG dominator cache (intra-procedural) ----

type cfgCacheKey struct {
	progName string
	funcID   int64
}

type domCache struct {
	idom      map[int64]int64
	postIDom map[int64]int64
}

var cfgDomCache sync.Map // map[cfgCacheKey]*domCache

func getProgNameForCache(p *Program) string {
	if p == nil || p.Program == nil {
		return ""
	}
	return p.Program.GetProgramName()
}

// computeDominators computes immediate dominators for all reachable blocks (block-level).
func computeDominators(fn *ssa.Function) map[int64]int64 {
	if fn == nil || fn.EnterBlock <= 0 {
		return nil
	}

	blocks := fn.Blocks
	if len(blocks) == 0 {
		return nil
	}

	entry := fn.EnterBlock
	all := make(map[int64]struct{}, len(blocks))
	for _, bid := range blocks {
		all[bid] = struct{}{}
	}

	dom := make(map[int64]map[int64]struct{}, len(blocks))
	for _, n := range blocks {
		dom[n] = make(map[int64]struct{}, len(all))
		if n == entry {
			dom[n][entry] = struct{}{}
		} else {
			for k := range all {
				dom[n][k] = struct{}{}
			}
		}
	}

	changed := true
	for changed {
		changed = false
		for _, n := range blocks {
			if n == entry {
				continue
			}
			b, ok := fn.GetBasicBlockByID(n)
			if !ok || b == nil {
				continue
			}
			newDom := make(map[int64]struct{}, len(all))
			newDom[n] = struct{}{}

			first := true
			var inter map[int64]struct{}
			for _, pid := range b.Preds {
				if _, ok := dom[pid]; !ok {
					continue
				}
				if first {
					inter = make(map[int64]struct{}, len(dom[pid]))
					for k := range dom[pid] {
						inter[k] = struct{}{}
					}
					first = false
					continue
				}
				for k := range inter {
					if _, ok := dom[pid][k]; !ok {
						delete(inter, k)
					}
				}
			}
			for k := range inter {
				newDom[k] = struct{}{}
			}

			if len(newDom) != len(dom[n]) {
				dom[n] = newDom
				changed = true
				continue
			}
			for k := range newDom {
				if _, ok := dom[n][k]; !ok {
					dom[n] = newDom
					changed = true
					break
				}
			}
		}
	}

	idom := make(map[int64]int64, len(blocks))
	idom[entry] = 0
	for _, n := range blocks {
		if n == entry {
			continue
		}
		var cands []int64
		for d := range dom[n] {
			if d == n {
				continue
			}
			cands = append(cands, d)
		}
		var best int64
		for _, d := range cands {
			isBest := true
			for _, d2 := range cands {
				if d2 == d {
					continue
				}
				if _, ok := dom[d][d2]; ok {
					isBest = false
					break
				}
			}
			if isBest {
				best = d
				break
			}
		}
		idom[n] = best
	}
	return idom
}

// computePostDominators computes minimal block-level post-dominators using virtual exit.
func computePostDominators(fn *ssa.Function) map[int64]int64 {
	if fn == nil || fn.EnterBlock <= 0 {
		return nil
	}
	blocks := fn.Blocks
	if len(blocks) == 0 {
		return nil
	}

	const vExit int64 = -1

	all := make(map[int64]struct{}, len(blocks)+1)
	for _, bid := range blocks {
		all[bid] = struct{}{}
	}
	all[vExit] = struct{}{}

	getSuccs := func(bid int64) []int64 {
		if bid == vExit {
			return nil
		}
		b, ok := fn.GetBasicBlockByID(bid)
		if !ok || b == nil {
			return nil
		}
		if len(b.Succs) == 0 {
			return []int64{vExit}
		}
		return b.Succs
	}

	pdom := make(map[int64]map[int64]struct{}, len(blocks)+1)
	for k := range all {
		pdom[k] = make(map[int64]struct{}, len(all))
		if k == vExit {
			pdom[k][vExit] = struct{}{}
		} else {
			for a := range all {
				pdom[k][a] = struct{}{}
			}
		}
	}

	changed := true
	for changed {
		changed = false
		for _, n := range blocks {
			succs := getSuccs(n)
			newSet := make(map[int64]struct{}, len(all))
			newSet[n] = struct{}{}
			first := true
			var inter map[int64]struct{}
			for _, s := range succs {
				if _, ok := pdom[s]; !ok {
					continue
				}
				if first {
					inter = make(map[int64]struct{}, len(pdom[s]))
					for k := range pdom[s] {
						inter[k] = struct{}{}
					}
					first = false
					continue
				}
				for k := range inter {
					if _, ok := pdom[s][k]; !ok {
						delete(inter, k)
					}
				}
			}
			for k := range inter {
				newSet[k] = struct{}{}
			}

			if len(newSet) != len(pdom[n]) {
				pdom[n] = newSet
				changed = true
				continue
			}
			for k := range newSet {
				if _, ok := pdom[n][k]; !ok {
					pdom[n] = newSet
					changed = true
					break
				}
			}
		}
	}

	ipdom := make(map[int64]int64, len(blocks))
	for _, n := range blocks {
		var cands []int64
		for d := range pdom[n] {
			if d == n {
				continue
			}
			cands = append(cands, d)
		}
		var best int64
		for _, d := range cands {
			isBest := true
			for _, d2 := range cands {
				if d2 == d {
					continue
				}
				if _, ok := pdom[d][d2]; ok {
					isBest = false
					break
				}
			}
			if isBest {
				best = d
				break
			}
		}
		if best == vExit {
			ipdom[n] = 0
		} else {
			ipdom[n] = best
		}
	}
	return ipdom
}

func getOrBuildDomCache(p *Program, funcID int64) *domCache {
	key := cfgCacheKey{progName: getProgNameForCache(p), funcID: funcID}
	if v, ok := cfgDomCache.Load(key); ok {
		if c, ok := v.(*domCache); ok {
			return c
		}
	}
	fn, err := getFunctionByID(p, funcID)
	if err != nil {
		return &domCache{idom: map[int64]int64{}, postIDom: map[int64]int64{}}
	}
	c := &domCache{
		idom:     computeDominators(fn),
		postIDom: computePostDominators(fn),
	}
	cfgDomCache.Store(key, c)
	return c
}

func cfgInstIndexInBlock(blk *ssa.BasicBlock, instID int64) (idx int, ok bool) {
	if blk == nil || instID <= 0 {
		return 0, false
	}
	for i, iid := range blk.Insts {
		if iid == instID {
			return i, true
		}
	}
	return 0, false
}

func cfgSameBlockOrderedInsts(p *Program, a, b *CfgCtxValue) (bothInst bool, ia, ib int) {
	if p == nil || a == nil || b == nil {
		return false, 0, 0
	}
	if a.FuncID != b.FuncID || a.BlockID != b.BlockID {
		return false, 0, 0
	}
	fn, err := getFunctionByID(p, a.FuncID)
	if err != nil || fn == nil {
		return false, 0, 0
	}
	blk, err := getBlockByID(fn, a.BlockID)
	if err != nil || blk == nil {
		return false, 0, 0
	}
	var okA, okB bool
	ia, okA = cfgInstIndexInBlock(blk, a.InstID)
	ib, okB = cfgInstIndexInBlock(blk, b.InstID)
	if !okA || !okB {
		return false, 0, 0
	}
	return true, ia, ib
}

// dominates implements cfgDominates: whether the frame `target` dominates the pipeline `receiver`
// (graph-theoretically dominates(target, receiver)). Arguments follow SyntaxFlow surface order.
func dominates(p *Program, receiver, target *CfgCtxValue) bool {
	dominator, dominated := target, receiver
	if dominator == nil || dominated == nil || dominator.IsEmpty() || dominated.IsEmpty() {
		return false
	}
	if dominator.FuncID != dominated.FuncID {
		return false
	}
	if both, ia, ib := cfgSameBlockOrderedInsts(p, dominator, dominated); both {
		return ia <= ib
	}
	cache := getOrBuildDomCache(p, dominator.FuncID)
	if cache == nil || cache.idom == nil {
		return false
	}
	cur := dominated.BlockID
	for cur != 0 && cur != dominator.BlockID {
		cur = cache.idom[cur]
	}
	return cur == dominator.BlockID
}

// postDominates implements cfgPostDominates: whether the frame `target` post-dominates the pipeline `receiver`
// (graph-theoretically postDominates(target, receiver)). Arguments follow SyntaxFlow surface order.
func postDominates(p *Program, receiver, target *CfgCtxValue) bool {
	postDom, site := target, receiver
	if postDom == nil || site == nil || postDom.IsEmpty() || site.IsEmpty() {
		return false
	}
	if postDom.FuncID != site.FuncID {
		return false
	}
	if both, ia, ib := cfgSameBlockOrderedInsts(p, postDom, site); both {
		return ia >= ib
	}
	cache := getOrBuildDomCache(p, postDom.FuncID)
	if cache == nil || cache.postIDom == nil {
		return false
	}
	cur := site.BlockID
	for cur != 0 && cur != postDom.BlockID {
		cur = cache.postIDom[cur]
	}
	return cur == postDom.BlockID
}
