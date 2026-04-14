package ssaapi

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type reachableOptions struct {
	icfg     bool
	maxDepth int
	maxNodes int
	skipLoopBackedge bool
}

type reachState struct {
	funcID  int64
	blockID int64
	depth   int
}

func resolveICFGCallee(p *Program, call *ssa.Call) *ssa.Function {
	if p == nil || p.Program == nil || call == nil || call.Method <= 0 {
		return nil
	}
	seen := make(map[int64]struct{})
	var tryCalleeValue func(v ssa.Value) *ssa.Function
	tryCalleeValue = func(v ssa.Value) *ssa.Function {
		if v == nil {
			return nil
		}
		id := v.GetId()
		if id > 0 {
			if _, dup := seen[id]; dup {
				return nil
			}
			seen[id] = struct{}{}
		}
		if fn, ok := ssa.ToFunction(v); ok && fn != nil && fn.EnterBlock > 0 {
			return fn
		}
		if phi, ok := ssa.ToPhi(v); ok && phi != nil {
			for _, sub := range phi.GetValues() {
				if sub == nil {
					continue
				}
				if fn := tryCalleeValue(sub); fn != nil {
					return fn
				}
			}
			return nil
		}
		if ref := v.GetReference(); ref != nil {
			return tryCalleeValue(ref)
		}
		return nil
	}
	if method, ok := call.GetValueById(call.Method); ok && method != nil {
		if fn := tryCalleeValue(method); fn != nil {
			return fn
		}
	}
	if fn, err := getFunctionByID(p, call.Method); err == nil && fn != nil && fn.EnterBlock > 0 {
		return fn
	}
	return nil
}

func icfgReturnSuccessors(p *Program, calleeID int64) []reachState {
	if p == nil || p.Program == nil || calleeID <= 0 {
		return nil
	}
	targetFn, _ := getFunctionByID(p, calleeID)
	seen := make(map[reachState]struct{})
	var out []reachState
	p.Program.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		callerID := fn.GetId()
		for _, bid := range fn.Blocks {
			blk, ok := fn.GetBasicBlockByID(bid)
			if !ok || blk == nil {
				continue
			}
			for _, instID := range blk.Insts {
				ins, ok := blk.GetInstructionById(instID)
				if !ok || ins == nil {
					continue
				}
				if lz, ok := ssa.ToLazyInstruction(ins); ok && lz != nil {
					ins = lz.Self()
				}
				call, ok := ssa.ToCall(ins)
				if !ok || call == nil {
					continue
				}
				calleeFn := resolveICFGCallee(p, call)
				if calleeFn == nil {
					continue
				}
				matchesCallee := call.Method == calleeID || calleeFn.GetId() == calleeID
				if !matchesCallee && targetFn != nil && calleeFn.GetName() == targetFn.GetName() {
					matchesCallee = true
				}
				if !matchesCallee {
					continue
				}
				stSame := reachState{funcID: callerID, blockID: blk.GetId(), depth: 0}
				if _, dup := seen[stSame]; !dup {
					seen[stSame] = struct{}{}
					out = append(out, stSame)
				}
				for _, s := range blk.Succs {
					st := reachState{funcID: callerID, blockID: s, depth: 0}
					if _, dup := seen[st]; dup {
						continue
					}
					seen[st] = struct{}{}
					out = append(out, st)
				}
			}
		}
	})
	return out
}

func reachabilitySearch(p *Program, a, b *CfgCtxValue, opt reachableOptions, recordPred bool) (found bool, pred map[reachState]reachState, end reachState) {
	if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
		return false, nil, reachState{}
	}
	start := reachState{funcID: a.FuncID, blockID: a.BlockID, depth: 0}
	targetFuncID, targetBlockID := b.FuncID, b.BlockID

	seen := map[reachState]struct{}{start: {}}
	var predMap map[reachState]reachState
	if recordPred {
		predMap = make(map[reachState]reachState)
	}
	queue := []reachState{start}

	idomDominates := func(idom map[int64]int64, dom, node int64) bool {
		if dom <= 0 || node <= 0 {
			return false
		}
		if dom == node {
			return true
		}
		cur := node
		for step := 0; step < 2048 && cur > 0; step++ {
			parent, ok := idom[cur]
			if !ok || parent <= 0 || parent == cur {
				return false
			}
			if parent == dom {
				return true
			}
			cur = parent
		}
		return false
	}

	tryAdd := func(cur, nxt reachState) {
		if _, ok := seen[nxt]; ok {
			return
		}
		seen[nxt] = struct{}{}
		if predMap != nil {
			predMap[nxt] = cur
		}
		queue = append(queue, nxt)
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.funcID == targetFuncID && cur.blockID == targetBlockID {
			return true, predMap, cur
		}
		if opt.maxNodes > 0 && len(seen) > opt.maxNodes {
			return false, nil, reachState{}
		}
		fn, err := getFunctionByID(p, cur.funcID)
		if err != nil || fn == nil {
			continue
		}
		blk, _ := fn.GetBasicBlockByID(cur.blockID)
		if blk == nil {
			continue
		}

		var idom map[int64]int64
		if opt.skipLoopBackedge {
			if cache := getOrBuildDomCache(p, cur.funcID); cache != nil {
				idom = cache.idom
			}
		}
		for _, s := range blk.Succs {
			if opt.skipLoopBackedge {
				succBlk, _ := fn.GetBasicBlockByID(s)
				if blk.IsBlock(ssa.LoopLatch) && succBlk != nil && succBlk.IsBlock(ssa.LoopHeader) {
					continue
				}
				if idom != nil && idomDominates(idom, s, cur.blockID) {
					continue
				}
			}
			tryAdd(cur, reachState{funcID: cur.funcID, blockID: s, depth: cur.depth})
		}

		if !opt.icfg {
			continue
		}
		if opt.maxDepth > 0 && cur.depth >= opt.maxDepth {
			continue
		}

		for _, instID := range blk.Insts {
			ins, ok := blk.GetInstructionById(instID)
			if !ok || ins == nil {
				continue
			}
			if lz, ok := ssa.ToLazyInstruction(ins); ok && lz != nil {
				ins = lz.Self()
			}
			call, ok := ssa.ToCall(ins)
			if !ok || call == nil {
				continue
			}
			calleeFn := resolveICFGCallee(p, call)
			if calleeFn == nil {
				continue
			}
			tryAdd(cur, reachState{funcID: calleeFn.GetId(), blockID: calleeFn.EnterBlock, depth: cur.depth + 1})
		}

		if isExitLikeBlock(fn, cur.blockID) {
			for _, ret := range icfgReturnSuccessors(p, cur.funcID) {
				tryAdd(cur, reachState{funcID: ret.funcID, blockID: ret.blockID, depth: cur.depth + 1})
			}
		}
	}
	return false, nil, reachState{}
}

func formatReachStateLabel(p *Program, st reachState) string {
	fnName := "?"
	if p != nil {
		if fn, err := getFunctionByID(p, st.funcID); err == nil && fn != nil {
			fnName = fn.GetName()
		}
	}
	return fmt.Sprintf("%s[f=%d,b=%d]", fnName, st.funcID, st.blockID)
}

func cfgReachShortestPathString(p *Program, a, b *CfgCtxValue, opt reachableOptions) string {
	ok, pred, end := reachabilitySearch(p, a, b, opt, true)
	if !ok || pred == nil {
		return ""
	}
	start := reachState{funcID: a.FuncID, blockID: a.BlockID, depth: 0}
	var chain []string
	cur := end
	for {
		chain = append(chain, formatReachStateLabel(p, cur))
		if cur.funcID == start.funcID && cur.blockID == start.blockID {
			break
		}
		pr, ok := pred[cur]
		if !ok {
			return ""
		}
		cur = pr
	}
	slices.Reverse(chain)
	return strings.Join(chain, " -> ")
}

func reachableOptsFromParams(params *sfvm.NativeCallActualParams, icfg bool) reachableOptions {
	if params == nil || !icfg {
		return reachableOptions{icfg: icfg, maxDepth: 0, maxNodes: 0}
	}
	maxDepth := params.GetInt("max_depth", "maxDepth", "depth")
	if maxDepth < 0 {
		maxDepth = 3
	}
	maxNodes := params.GetInt("max_nodes", "maxNodes", "nodes")
	if maxNodes < 0 {
		maxNodes = 5000
	}
	return reachableOptions{icfg: true, maxDepth: maxDepth, maxNodes: maxNodes}
}

func reachableWithOptions(p *Program, a, b *CfgCtxValue, opt reachableOptions) bool {
	ok, _, _ := reachabilitySearch(p, a, b, opt, false)
	return ok
}

func reachable(p *Program, a, b *CfgCtxValue) bool {
	return reachableWithOptions(p, a, b, reachableOptions{icfg: false, maxDepth: 0, maxNodes: 0})
}
