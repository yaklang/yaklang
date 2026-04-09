package ssaapi

import (
	"fmt"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const (
	NativeCall_GetCFG       = "getCfg"
	NativeCall_CFGGuards    = "cfgGuards"
	NativeCall_CFGDominates = "cfgDominates"
	NativeCall_CFGPostDom   = "cfgPostDominates"
	NativeCall_CFGReachable = "cfgReachable"
	NativeCall_CFGBlockInfo = "cfgBlock"
	NativeCall_CFGInstInfo  = "cfgInst"
)

// ---- CFG cache (intra-procedural) ----

type cfgCacheKey struct {
	progName string
	funcID   int64
}

type domCache struct {
	// idom[blockID] = immediate dominator blockID, 0 for entry or unknown
	idom map[int64]int64
	// postIDom[blockID] = immediate post-dominator blockID (virtual exit supported)
	postIDom map[int64]int64
}

var cfgDomCache sync.Map // map[cfgCacheKey]*domCache

func getProgNameForCache(p *Program) string {
	if p == nil || p.Program == nil {
		return ""
	}
	return p.Program.GetProgramName()
}

func getFunctionByID(p *Program, funcID int64) (*ssa.Function, error) {
	if p == nil || p.Program == nil || funcID <= 0 {
		return nil, utils.Error("invalid program/function")
	}
	ins, ok := p.Program.GetInstructionById(funcID)
	if !ok || ins == nil {
		return nil, utils.Errorf("function id %d not found", funcID)
	}
	fn, ok := ssa.ToFunction(ins)
	if !ok || fn == nil {
		return nil, utils.Errorf("instruction %d is not a function", funcID)
	}
	return fn, nil
}

func getBlockByID(fn *ssa.Function, blockID int64) (*ssa.BasicBlock, error) {
	if fn == nil || blockID <= 0 {
		return nil, utils.Error("invalid function/block")
	}
	b, ok := fn.GetBasicBlockByID(blockID)
	if !ok || b == nil {
		return nil, utils.Errorf("block id %d not found", blockID)
	}
	return b, nil
}

// computeDominators computes immediate dominators for all reachable blocks (block-level).
// Algorithm: classic iterative dataflow on sets, then derive idom.
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

	// dom[n] as set
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
			// newDom = {n} U (intersection of dom[p] for all preds)
			newDom := make(map[int64]struct{}, len(all))
			newDom[n] = struct{}{}

			// start intersection with first pred that exists
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
				// inter = inter ∩ dom[pid]
				for k := range inter {
					if _, ok := dom[pid][k]; !ok {
						delete(inter, k)
					}
				}
			}
			for k := range inter {
				newDom[k] = struct{}{}
			}

			// compare
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

	// derive idom: idom(entry)=0; for n!=entry choose d in dom[n]\{n} that is not dominated by any other in dom[n]\{n}.
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
		// pick the closest: a candidate d such that no other candidate d2 (d2!=d) dominates d.
		var best int64
		for _, d := range cands {
			isBest := true
			for _, d2 := range cands {
				if d2 == d {
					continue
				}
				// d2 dominates d ?
				if _, ok := dom[d][d2]; ok {
					// if d2 in dom[d], then d2 dominates d
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

// computePostDominators: minimal block-level post-dominators using virtual exit.
func computePostDominators(fn *ssa.Function) map[int64]int64 {
	if fn == nil || fn.EnterBlock <= 0 {
		return nil
	}
	blocks := fn.Blocks
	if len(blocks) == 0 {
		return nil
	}

	// virtual exit node id: -1
	const vExit int64 = -1

	all := make(map[int64]struct{}, len(blocks)+1)
	for _, bid := range blocks {
		all[bid] = struct{}{}
	}
	all[vExit] = struct{}{}

	// build reverse preds (postdom uses succs in forward graph; we'll intersect over succs)
	getSuccs := func(bid int64) []int64 {
		if bid == vExit {
			return nil
		}
		b, ok := fn.GetBasicBlockByID(bid)
		if !ok || b == nil {
			return nil
		}
		// Treat blocks with no succs as exiting to vExit.
		if len(b.Succs) == 0 {
			return []int64{vExit}
		}
		return b.Succs
	}

	// pdom[n] as set
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

	// derive immediate post-dominator (ipdom) from pdom sets (exclude self).
	ipdom := make(map[int64]int64, len(blocks))
	for _, n := range blocks {
		var cands []int64
		for d := range pdom[n] {
			if d == n {
				continue
			}
			cands = append(cands, d)
		}
		// pick closest: d such that no other candidate d2 is post-dominated by d (mirror of idom derivation)
		var best int64
		for _, d := range cands {
			isBest := true
			for _, d2 := range cands {
				if d2 == d {
					continue
				}
				// d2 post-dominates d ? equivalently d2 in pdom[d]
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
		// ignore virtual exit in result mapping
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

func dominates(p *Program, a, b *CfgCtxValue) bool {
	if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a.FuncID != b.FuncID {
		return false
	}
	cache := getOrBuildDomCache(p, a.FuncID)
	if cache == nil || cache.idom == nil {
		return false
	}
	// walk from b up to entry
	cur := b.BlockID
	for cur != 0 && cur != a.BlockID {
		cur = cache.idom[cur]
	}
	return cur == a.BlockID
}

func postDominates(p *Program, a, b *CfgCtxValue) bool {
	if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a.FuncID != b.FuncID {
		return false
	}
	cache := getOrBuildDomCache(p, a.FuncID)
	if cache == nil || cache.postIDom == nil {
		return false
	}
	cur := b.BlockID
	for cur != 0 && cur != a.BlockID {
		cur = cache.postIDom[cur]
	}
	return cur == a.BlockID
}

func reachable(p *Program, a, b *CfgCtxValue) bool {
	if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a.FuncID != b.FuncID {
		return false
	}
	fn, err := getFunctionByID(p, a.FuncID)
	if err != nil {
		return false
	}
	start, err := getBlockByID(fn, a.BlockID)
	if err != nil || start == nil {
		return false
	}
	target := b.BlockID
	seen := map[int64]struct{}{start.GetId(): {}}
	queue := []int64{start.GetId()}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == target {
			return true
		}
		blk, _ := fn.GetBasicBlockByID(cur)
		if blk == nil {
			continue
		}
		for _, s := range blk.Succs {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			queue = append(queue, s)
		}
	}
	return false
}

// ---- native calls ----

func extractCfgCtx(v sfvm.ValueOperator) (*CfgCtxValue, bool) {
	c, ok := v.(*CfgCtxValue)
	return c, ok
}

func fetchProgramFromCfgValues(v sfvm.Values) (*Program, error) {
	// Prefer existing program resolution for normal SSA Values.
	if p, err := fetchProgram(v); err == nil && p != nil {
		return p, nil
	}
	// Fallback: resolve from cfg ctx carrier.
	var prog *Program
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		if c, ok := op.(*CfgCtxValue); ok && c != nil && c.prog != nil {
			prog = c.prog
			return utils.Error("abort")
		}
		return nil
	})
	if prog != nil {
		return prog, nil
	}
	return nil, utils.Error("no parent program found")
}

func nativeCallGetCFG(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}
	var out []sfvm.ValueOperator
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		val, ok := op.(*Value)
		if !ok || val == nil || val.IsNil() {
			return nil
		}
		inst := val.getInstruction()
		if inst == nil {
			return nil
		}
		fn := inst.GetFunc()
		blk := inst.GetBlock()
		if fn == nil || blk == nil {
			return nil
		}
		ctx := &CfgCtxValue{
			prog:    prog,
			FuncID:  fn.GetId(),
			BlockID: blk.GetId(),
			InstID:  inst.GetId(),
		}
		// propagate anchor bits
		ctx.SetAnchorBitVector(op.GetAnchorBitVector())
		out = append(out, ctx)
		return nil
	})
	if len(out) == 0 {
		return false, nil, utils.Error("no cfg ctx produced")
	}
	return true, sfvm.NewValues(out), nil
}

func nativeCallCFGBlock(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgramFromCfgValues(v)
	if err != nil {
		return false, nil, err
	}
	var out []sfvm.ValueOperator
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		ctx, ok := extractCfgCtx(op)
		if !ok || ctx.IsEmpty() {
			return nil
		}
		s := fmt.Sprintf("func=%d block=%d", ctx.FuncID, ctx.BlockID)
		out = append(out, prog.NewConstValue(s))
		return nil
	})
	if len(out) == 0 {
		return false, nil, utils.Error("no block info")
	}
	return true, sfvm.NewValues(out), nil
}

func nativeCallCFGInst(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgramFromCfgValues(v)
	if err != nil {
		return false, nil, err
	}
	var out []sfvm.ValueOperator
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		ctx, ok := extractCfgCtx(op)
		if !ok || ctx.IsEmpty() {
			return nil
		}
		s := fmt.Sprintf("func=%d block=%d inst=%d", ctx.FuncID, ctx.BlockID, ctx.InstID)
		out = append(out, prog.NewConstValue(s))
		return nil
	})
	if len(out) == 0 {
		return false, nil, utils.Error("no inst info")
	}
	return true, sfvm.NewValues(out), nil
}

func resolveCfgTargetFromFrame(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (*CfgCtxValue, error) {
	if frame == nil {
		return nil, utils.Error("cfg*: frame is nil")
	}
	targetVar := params.GetString(0, "target", "var", "against")
	if targetVar == "" {
		return nil, utils.Error("cfg*: 'target' parameter is required (e.g. target=$sinkCfg)")
	}
	targetVar = strings.TrimPrefix(targetVar, "$")
	targetOp, ok := frame.GetSymbolByName(targetVar)
	if !ok || targetOp == nil {
		return nil, utils.Errorf("cfg*: variable '$%s' not found in current frame", targetVar)
	}
	var first *CfgCtxValue
	_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
		if c, ok := operator.(*CfgCtxValue); ok && c != nil && !c.IsEmpty() {
			first = c
			return utils.Error("abort")
		}
		return nil
	})
	if first == nil {
		return nil, utils.Errorf("cfg*: variable '$%s' contains no cfg ctx value (did you call <getCfg>?)", targetVar)
	}
	return first, nil
}

func nativeCallCFGRel(opName string, rel func(p *Program, a, b *CfgCtxValue) bool) sfvm.NativeCallFunc {
	return sfvm.NativeCallFunc(func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
		prog, err := fetchProgramFromCfgValues(v)
		if err != nil {
			return false, nil, err
		}
		b, err := resolveCfgTargetFromFrame(frame, params)
		if err != nil {
			return false, nil, utils.Wrap(err, opName)
		}
		var out []sfvm.ValueOperator
		_ = v.Recursive(func(op sfvm.ValueOperator) error {
			a, ok := extractCfgCtx(op)
			if !ok || a.IsEmpty() {
				return nil
			}
			out = append(out, prog.NewConstValue(rel(prog, a, b)))
			return nil
		})
		if len(out) == 0 {
			return false, nil, utils.Errorf("%s: no cfg ctx values", opName)
		}
		return true, sfvm.NewValues(out), nil
	})
}

// minimal guard extraction: detect if-in-block dominates sink block and one branch goes to exit.
func nativeCallCFGGuards(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgramFromCfgValues(v)
	if err != nil {
		return false, nil, err
	}
	var out []sfvm.ValueOperator
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		ctx, ok := extractCfgCtx(op)
		if !ok || ctx.IsEmpty() {
			return nil
		}
		fn, err := getFunctionByID(prog, ctx.FuncID)
		if err != nil || fn == nil {
			return nil
		}

		// precompute dom cache for dominates checks
		_ = getOrBuildDomCache(prog, ctx.FuncID)

		guards := make([]string, 0, 4)
		for _, bid := range fn.Blocks {
			b, _ := fn.GetBasicBlockByID(bid)
			if b == nil || len(b.Insts) == 0 {
				continue
			}
			last := b.LastInst()
			if last == nil {
				continue
			}
			ifInst, ok := ssa.ToIfInstruction(last)

			// Prefer structured IfInstruction, but allow Yak lowering variants where the
			// terminator isn't directly an IfInstruction (fallback: 2-way succs).
			condText := ""
			instID := last.GetId()
			var tBranch, fBranch int64
			if ok && ifInst != nil {
				instID = ifInst.GetId()
				tBranch, fBranch = ifInst.True, ifInst.False

				condVal, _ := fn.GetValueById(ifInst.Cond)
				if condVal != nil {
					if r := condVal.GetRange(); r != nil {
						condText = r.GetTextContext(0)
					}
				}
				if condText == "" {
					condText = fmt.Sprintf("cond@%d", ifInst.Cond)
				}
			} else if len(b.Succs) == 2 {
				tBranch, fBranch = b.Succs[0], b.Succs[1]
				condText = fmt.Sprintf("cond@block%d", b.GetId())
			} else {
				continue
			}

			// Minimal phase-1 heuristic: only require the branch block can reach the target block.
			// (Dominance can be added later when CFG termination/exit semantics are standardized.)
			branchCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: b.GetId(), InstID: instID}
			if !reachable(prog, branchCtx, ctx) {
				continue
			}

			// if one branch is exit block (or reaches no succ), other branch reaches target
			exitID := fn.ExitBlock
			var exitCtx *CfgCtxValue
			if exitID > 0 {
				exitCtx = &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: exitID}
			}

			// Determine which branch can reach target.
			tCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: tBranch}
			fCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: fBranch}
			targetCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: ctx.BlockID}

			tReach := reachable(prog, tCtx, targetCtx)
			fReach := reachable(prog, fCtx, targetCtx)
			if !tReach && !fReach {
				continue
			}

			isExitLike := func(bid int64) bool {
				if bid <= 0 {
					return false
				}
				blk, _ := fn.GetBasicBlockByID(bid)
				if blk == nil {
					return false
				}
				if len(blk.Succs) == 0 {
					return true
				}
				if last := blk.LastInst(); last != nil {
					if _, ok := ssa.ToReturn(last); ok {
						return true
					}
				}
				return false
			}

			tExit := isExitLike(tBranch)
			fExit := isExitLike(fBranch)
			if exitCtx != nil {
				tExit = tExit || tBranch == exitID || reachable(prog, tCtx, exitCtx)
				fExit = fExit || fBranch == exitID || reachable(prog, fCtx, exitCtx)
			}

			// guard pattern: if (cond) return; => fallthrough requires !cond (when cond branch exits)
			if tExit && fReach {
				guards = append(guards, fmt.Sprintf("not(%s)", condText))
			} else if fExit && tReach {
				guards = append(guards, condText)
			}
		}
		for _, g := range lo.Uniq(guards) {
			out = append(out, prog.NewConstValue(g, (*memedit.Range)(nil)))
		}
		return nil
	})

	if len(out) == 0 {
		return false, nil, utils.Error("no guards found")
	}
	return true, sfvm.NewValues(out), nil
}
