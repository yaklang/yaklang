package ssaapi

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// mapCfgCtxValues runs fn for each non-empty CfgCtxValue under v; emptyErr is returned if nothing was appended.
func mapCfgCtxValues(v sfvm.Values, emptyErr string, fn func(prog *Program, op sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator)) (bool, sfvm.Values, error) {
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
		fn(prog, op, ctx, &out)
		return nil
	})
	if len(out) == 0 {
		return false, nil, utils.Error(emptyErr)
	}
	return true, sfvm.NewValues(out), nil
}

func extractCfgCtx(v sfvm.ValueOperator) (*CfgCtxValue, bool) {
	c, ok := v.(*CfgCtxValue)
	return c, ok
}

// valuesPipeHasCfgCtx reports whether the value pipe already carries at least one non-empty CfgCtxValue.
func valuesPipeHasCfgCtx(v sfvm.Values) bool {
	found := false
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		if c, ok := extractCfgCtx(op); ok && c != nil && !c.IsEmpty() {
			found = true
			return utils.Error("abort")
		}
		return nil
	})
	return found
}

// expandValuesToCfgCtxList maps each SSA *Value under v to a CfgCtxValue (same rules as <getCfg>).
func expandValuesToCfgCtxList(v sfvm.Values) ([]sfvm.ValueOperator, *Program, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return nil, nil, err
	}
	var outs []sfvm.ValueOperator
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
		ctx.SetAnchorBitVector(op.GetAnchorBitVector())
		outs = append(outs, ctx)
		return nil
	})
	if len(outs) == 0 {
		return nil, nil, utils.Error("no cfg ctx produced")
	}
	return outs, prog, nil
}

// coerceCfgCallInputs ensures the pipe carries CfgCtxValue: if it already does, returns v unchanged;
// otherwise applies implicit <getCfg>-style expansion from SSA values.
func coerceCfgCallInputs(v sfvm.Values) (sfvm.Values, *Program, error) {
	if valuesPipeHasCfgCtx(v) {
		prog, err := fetchProgramFromCfgValues(v)
		if err != nil {
			return nil, nil, err
		}
		return v, prog, nil
	}
	outs, prog, err := expandValuesToCfgCtxList(v)
	if err != nil {
		return nil, nil, err
	}
	return sfvm.NewValues(outs), prog, nil
}

func fetchProgramFromCfgValues(v sfvm.Values) (*Program, error) {
	if p, err := fetchProgram(v); err == nil && p != nil {
		return p, nil
	}
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
	out, _, err := expandValuesToCfgCtxList(v)
	if err != nil {
		return false, nil, err
	}
	return true, sfvm.NewValues(out), nil
}

func nativeCallCFGBlock(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxValues(v, "no block info", func(prog *Program, _ sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		s := fmt.Sprintf("func=%d block=%d", ctx.FuncID, ctx.BlockID)
		*out = append(*out, prog.NewConstValue(s))
	})
}

func nativeCallCFGInst(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxValues(v, "no inst info", func(prog *Program, _ sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		s := fmt.Sprintf("func=%d block=%d inst=%d", ctx.FuncID, ctx.BlockID, ctx.InstID)
		*out = append(*out, prog.NewConstValue(s))
	})
}

func nativeCallCFGCondition(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxValues(v, "cfgCondition: no condition info", func(prog *Program, _ sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		summary, err := getBlockConditionSummaryByCfgCtx(prog, ctx)
		if err != nil || summary == nil {
			return
		}
		*out = append(*out, prog.NewConstValue(summaryToCondString(summary)))
	})
}

func nativeCallCFGConditionValues(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxValues(v, "cfgConditionValues: no condition values", func(prog *Program, _ sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		summary, err := getBlockConditionSummaryByCfgCtx(prog, ctx)
		if err != nil || summary == nil {
			return
		}
		appendResolvedCondValues(prog, summary.CondValueID, out)
	})
}

func firstCfgCtxFromSymbolValues(vals sfvm.Values) (*CfgCtxValue, error) {
	var first *CfgCtxValue
	_ = vals.Recursive(func(operator sfvm.ValueOperator) error {
		if c, ok := operator.(*CfgCtxValue); ok && c != nil && !c.IsEmpty() {
			first = c
			return utils.Error("abort")
		}
		return nil
	})
	if first != nil {
		return first, nil
	}
	outs, _, err := expandValuesToCfgCtxList(vals)
	if err != nil {
		return nil, err
	}
	c0, ok := outs[0].(*CfgCtxValue)
	if !ok || c0 == nil {
		return nil, utils.Error("cfg*: internal: expected CfgCtxValue from SSA value expansion")
	}
	return c0, nil
}

// allCfgCtxFromSymbolValues collects every distinct cfg anchor in vals (e.g. every Sprintf in
// `fmt.Sprint* as $unsafe`); firstCfgCtxFromSymbolValues only took the first, which made
// cfgDominates/cfgPostDominates compare all receivers against a single Sprintf.
func allCfgCtxFromSymbolValues(vals sfvm.Values) ([]*CfgCtxValue, error) {
	seen := make(map[struct{ f, b, i int64 }]struct{}, 8)
	var out []*CfgCtxValue
	appendDistinct := func(c *CfgCtxValue) {
		if c == nil || c.IsEmpty() {
			return
		}
		k := struct{ f, b, i int64 }{c.FuncID, c.BlockID, c.InstID}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, c)
	}
	_ = vals.Recursive(func(operator sfvm.ValueOperator) error {
		if c, ok := operator.(*CfgCtxValue); ok {
			appendDistinct(c)
		}
		return nil
	})
	if len(out) > 0 {
		return out, nil
	}
	outs, _, err := expandValuesToCfgCtxList(vals)
	if err != nil {
		return nil, err
	}
	for _, op := range outs {
		c, ok := op.(*CfgCtxValue)
		if !ok {
			continue
		}
		appendDistinct(c)
	}
	if len(out) == 0 {
		return nil, utils.Error("cfg*: no cfg anchor")
	}
	return out, nil
}

// parseCfgTargetParam 从 frame 中解析 <cfg* target="$var"> 指向的符号名与值（cfgDominates / cfgReachable / reachabilityGuard 等共用）。
func parseCfgTargetParam(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (targetVar string, targetVals sfvm.Values, err error) {
	if frame == nil {
		return "", nil, utils.Error("cfg*: frame is nil")
	}
	if params == nil {
		return "", nil, utils.Error("cfg*: params is nil")
	}
	targetVar = params.GetString(0, "target", "var", "against")
	if targetVar == "" {
		return "", nil, utils.Error("cfg*: 'target' parameter is required (e.g. target=$sinkCfg)")
	}
	targetVar = strings.TrimPrefix(targetVar, "$")
	targetVals, ok := frame.GetSymbolByName(targetVar)
	if !ok || targetVals == nil {
		return "", nil, utils.Errorf("cfg*: variable '$%s' not found in current frame", targetVar)
	}
	return targetVar, targetVals, nil
}

func resolveCfgTargetFromFrame(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (*CfgCtxValue, error) {
	targetVar, targetVals, err := parseCfgTargetParam(frame, params)
	if err != nil {
		return nil, err
	}
	first, err := firstCfgCtxFromSymbolValues(targetVals)
	if err != nil {
		return nil, utils.Wrapf(err, "cfg*: variable '$%s' has no cfg anchor (use <getCfg> or an SSA value with func/block/inst)", targetVar)
	}
	return first, nil
}

func resolveAllCfgTargetsFromFrame(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) ([]*CfgCtxValue, error) {
	targetVar, targetVals, err := parseCfgTargetParam(frame, params)
	if err != nil {
		return nil, err
	}
	all, err := allCfgCtxFromSymbolValues(targetVals)
	if err != nil {
		return nil, utils.Wrapf(err, "cfg*: variable '$%s' has no cfg anchor (use <getCfg> or an SSA value with func/block/inst)", targetVar)
	}
	return all, nil
}

// mapCfgCtxAgainstTarget resolves `target` from the frame, then evaluates fn(prog, recv, targ)
// for each cfg ctx recv on the value stack (SyntaxFlow pipeline cfg).
// If the target variable binds multiple cfg points, we take OR over targets in the same
// function as the receiver (others already yield false in fn), indexed by FuncID to avoid
// O(|recv| * |all targets|) cross-function work.
func mapCfgCtxAgainstTarget(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams, opName string, fn func(prog *Program, recv, targ *CfgCtxValue) bool) (bool, sfvm.Values, error) {
	pipe, prog, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	targets, err := resolveAllCfgTargetsFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	byFunc := lo.GroupBy(targets, func(t *CfgCtxValue) int64 { return t.FuncID })
	var out []sfvm.ValueOperator
	_ = pipe.Recursive(func(op sfvm.ValueOperator) error {
		receiver, ok := extractCfgCtx(op)
		if !ok || receiver.IsEmpty() {
			return nil
		}
		cands := byFunc[receiver.FuncID]
		hit := false
		for _, t := range cands {
			if fn(prog, receiver, t) {
				hit = true
				break
			}
		}
		val := prog.NewConstValue(hit)
		// Boolean results must keep the same anchor bits as the cfg receiver so
		// `?{ *<cfgDominates...> }` and other filter sub-expressions can buildFilterMask
		// (see sfvm/condition_exec.go buildFilterMask).
		if ab := op.GetAnchorBitVector(); ab != nil && !ab.IsEmpty() && val != nil {
			val.SetAnchorBitVector(ab)
		}
		out = append(out, val)
		return nil
	})
	if len(out) == 0 {
		return false, nil, utils.Errorf("%s: no cfg ctx values", opName)
	}
	return true, sfvm.NewValues(out), nil
}

func nativeCallCFGDominates(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxAgainstTarget(v, frame, params, "cfgDominates", dominates)
}

func nativeCallCFGPostDominates(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxAgainstTarget(v, frame, params, "cfgPostDominates", postDominates)
}

func parseBoolParam(params *sfvm.NativeCallActualParams, key string, defaultValue bool) bool {
	if params == nil {
		return defaultValue
	}
	raw := strings.TrimSpace(params.GetString(key))
	if raw == "" {
		return defaultValue
	}
	switch strings.ToLower(raw) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func mapCfgWithReachableTarget(
	v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams, opName, empty string,
	compute func(prog *Program, a, target *CfgCtxValue, opt reachableOptions) any,
) (bool, sfvm.Values, error) {
	target, err := resolveCfgTargetFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	pipe, _, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	icfg := parseBoolParam(params, "icfg", false)
	opt := reachableOptsFromParams(params, icfg)
	return mapCfgCtxValues(pipe, empty, func(prog *Program, _ sfvm.ValueOperator, a *CfgCtxValue, out *[]sfvm.ValueOperator) {
		*out = append(*out, prog.NewConstValue(compute(prog, a, target, opt)))
	})
}

func nativeCallCFGReachable(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgWithReachableTarget(v, frame, params, "cfgReachable", "cfgReachable: no cfg ctx values",
		func(prog *Program, a, target *CfgCtxValue, opt reachableOptions) any {
			return reachableWithOptions(prog, a, target, opt)
		},
	)
}

func nativeCallCFGReachPath(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgWithReachableTarget(v, frame, params, "cfgReachPath", "cfgReachPath: no cfg ctx values",
		func(prog *Program, a, target *CfgCtxValue, opt reachableOptions) any {
			return cfgReachShortestPathString(prog, a, target, opt)
		},
	)
}

func newGuardPredicate(
	prog *Program, sinkCtx *CfgCtxValue, guardBlockID, instID, condValueID int64,
	polarity bool, kind string,
) *GuardPredicateValue {
	return &GuardPredicateValue{
		prog:         prog,
		FuncID:       sinkCtx.FuncID,
		GuardBlockID: guardBlockID,
		SinkBlockID:  sinkCtx.BlockID,
		CondInstID:   instID,
		CondValueID:  condValueID,
		Polarity:     polarity,
		Kind:         kind,
		Text:         "",
	}
}

// ifRejoinsWithoutElsePayload：无实质 else 时，Yak 常把 then 接一条边到汇合 M，而 **False 直接是 M 块**
//（M 上还有到后续 if 的出边，故不能用「两枝块都单后继且相同」来判）。此时该 if 对更后面的 sink 不增加分支约束
//（与 for 内 if(continue) vs 落到 sink 不同，后者 false 块不是 then 的唯一下一跳）。
func ifRejoinsWithoutElsePayload(fn *ssa.Function, tBranch, fBranch int64) bool {
	if fn == nil || tBranch <= 0 || fBranch <= 0 {
		return false
	}
	tb, ok1 := fn.GetBasicBlockByID(tBranch)
	if !ok1 || tb == nil {
		return false
	}
	if len(tb.Succs) != 1 {
		return false
	}
	merge := tb.Succs[0]
	if merge <= 0 {
		return false
	}
	if fBranch == merge {
		return true
	}
	fb, ok2 := fn.GetBasicBlockByID(fBranch)
	if !ok2 || fb == nil {
		return false
	}
	if len(fb.Succs) == 1 && fb.Succs[0] == merge {
		return true
	}
	return false
}

// computeCfgGuardsPredicates is the shared scan behind cfgGuards / reachabilityGuard conditional
// condition values: every if-header on some path to sink where exactly one side reaches the sink.
func computeCfgGuardsPredicates(prog *Program, fn *ssa.Function, sinkCtx *CfgCtxValue) []*GuardPredicateValue {
	if prog == nil || fn == nil || sinkCtx == nil || sinkCtx.IsEmpty() {
		return nil
	}
	_ = getOrBuildDomCache(prog, sinkCtx.FuncID)
	loopExits, loopLatches := cfgGuardsLoopBreakContinueTargets(fn)
	guards := make([]*GuardPredicateValue, 0, 8)
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

		instID := last.GetId()
		condValueID := int64(0)
		if summary := b.BlockConditionSummary(); summary.CondInstID > 0 || len(summary.CondValueID) > 0 {
			if summary.CondInstID > 0 {
				instID = summary.CondInstID
			}
			if len(summary.CondValueID) > 0 {
				condValueID = summary.CondValueID[0]
			}
		}
		var tBranch, fBranch int64
		if ok && ifInst != nil {
			instID = ifInst.GetId()
			tBranch, fBranch = ifInst.True, ifInst.False
			if condValueID <= 0 {
				condValueID = ifInst.Cond
			}
		} else if len(b.Succs) == 2 {
			tBranch, fBranch = b.Succs[0], b.Succs[1]
		} else {
			continue
		}

		branchCtx := &CfgCtxValue{prog: prog, FuncID: sinkCtx.FuncID, BlockID: b.GetId(), InstID: instID}
		if !reachable(prog, branchCtx, sinkCtx) {
			continue
		}

		exitID := fn.ExitBlock
		var exitCtx *CfgCtxValue
		if exitID > 0 {
			exitCtx = &CfgCtxValue{prog: prog, FuncID: sinkCtx.FuncID, BlockID: exitID}
		}

		tCtx := &CfgCtxValue{prog: prog, FuncID: sinkCtx.FuncID, BlockID: tBranch}
		fCtx := &CfgCtxValue{prog: prog, FuncID: sinkCtx.FuncID, BlockID: fBranch}
		targetCtx := &CfgCtxValue{prog: prog, FuncID: sinkCtx.FuncID, BlockID: sinkCtx.BlockID}

		tReach := reachable(prog, tCtx, targetCtx)
		fReach := reachable(prog, fCtx, targetCtx)
		if !tReach && !fReach {
			continue
		}

		// tExit/fExit：含「沿该分支能到达函数出口」，供 switch 区分 return/continue/break 等（for 内 if 等）
		tExit := isExitLikeBlock(fn, tBranch)
		fExit := isExitLikeBlock(fn, fBranch)
		if exitCtx != nil {
			tExit = tExit || tBranch == exitID || reachable(prog, tCtx, exitCtx)
			fExit = fExit || fBranch == exitID || reachable(prog, fCtx, exitCtx)
		}
		if tReach && fReach && !tExit && !fExit {
			continue
		}

		gb := b.GetId()
		switch {
		case !tReach && fReach:
			guards = append(guards, newGuardPredicate(prog, sinkCtx, gb, instID, condValueID, false, cfgGuardsAbortBranchKindFromRegion(fn, tBranch, loopExits, loopLatches)))
		case tExit && fReach && tReach:
			guards = append(guards, newGuardPredicate(prog, sinkCtx, gb, instID, condValueID, false, cfgGuardsAbortBranchKind(fn, tBranch, loopExits, loopLatches)))
		case tReach && !fReach:
			guards = append(guards, newGuardPredicate(prog, sinkCtx, gb, instID, condValueID, true, cfgGuardsAbortBranchKindFromRegion(fn, fBranch, loopExits, loopLatches)))
		case fExit && tReach:
			guards = append(guards, newGuardPredicate(prog, sinkCtx, gb, instID, condValueID, true, cfgGuardsAbortBranchKind(fn, fBranch, loopExits, loopLatches)))
		}
	}
	return guards
}

// minimal guard extraction: detect if-in-block dominates sink block and one branch goes to exit.
func nativeCallCFGGuards(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	pipe, _, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, "cfgGuards")
	}
	return mapCfgCtxValues(pipe, "no guards found", func(prog *Program, op sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		fn, err := getFunctionByID(prog, ctx.FuncID)
		if err != nil || fn == nil {
			return
		}

		guards := computeCfgGuardsPredicates(prog, fn, ctx)
		if len(guards) == 0 && ctx.FuncID > 0 && ctx.BlockID > 0 {
			guards = append(guards, newGuardPredicate(prog, ctx, ctx.BlockID, 0, 0, false, GuardKindNone))
		}
		for _, g := range lo.UniqBy(guards, func(v *GuardPredicateValue) string {
			if v == nil {
				return ""
			}
			if h, ok := v.Hash(); ok {
				return h
			}
			return v.String()
		}) {
			if g == nil || g.IsEmpty() {
				continue
			}
			g.SetAnchorBitVector(op.GetAnchorBitVector())
			*out = append(*out, g)
		}
	})
}

const (
	// reachabilityGuardParamMode：分析模式；当前仅支持 mustExecute。
	reachabilityGuardParamMode = "mode"
	// reachabilityGuardModeMustExecute：从函数入口到出口的全路径上是否必经过 target（必达分支依赖 cfgPostDominates；见 nc_desc）。
	reachabilityGuardModeMustExecute = "mustExecute"
)

func cfgCtxForFunctionEntry(p *Program, funcID int64) (*CfgCtxValue, error) {
	if p == nil || funcID <= 0 {
		return nil, utils.Errorf("%s: invalid program or func id", NativeCall_ReachabilityGuard)
	}
	fn, err := getFunctionByID(p, funcID)
	if err != nil || fn == nil {
		return nil, utils.Wrapf(err, "%s: get function %d", NativeCall_ReachabilityGuard, funcID)
	}
	enter, ok := fn.GetBasicBlockByID(fn.EnterBlock)
	if !ok || enter == nil {
		return nil, utils.Errorf("%s: function %s has no enter block", NativeCall_ReachabilityGuard, fn.GetName())
	}
	var instID int64
	for _, id := range enter.Insts {
		ins, ok := enter.GetInstructionById(id)
		if !ok || ins == nil {
			continue
		}
		if ssa.IsControlInstruction(ins) {
			continue
		}
		instID = id
		break
	}
	if instID <= 0 && len(enter.Insts) > 0 {
		instID = enter.Insts[0]
	}
	if instID <= 0 {
		return nil, utils.Errorf("%s: enter block has no instructions in %s", NativeCall_ReachabilityGuard, fn.GetName())
	}
	return &CfgCtxValue{
		prog:    p,
		FuncID:  funcID,
		BlockID: enter.GetId(),
		InstID:  instID,
	}, nil
}

func firstValueFromPipeForReachability(v sfvm.Values) *Value {
	var out *Value
	_ = v.Recursive(func(op sfvm.ValueOperator) error {
		if val, ok := op.(*Value); ok && val != nil && !val.IsNil() {
			out = val
			return utils.Error("abort")
		}
		return nil
	})
	return out
}

// --- reachabilityGuard: const-folded CFG edges (if cond is provably true/false, only follow that side)

func normalizeSSAInst(ins ssa.Instruction) ssa.Instruction {
	if ins == nil {
		return nil
	}
	if lz, ok := ins.(*ssa.LazyInstruction); ok && lz != nil {
		return lz.Self()
	}
	return ins
}

func ensureInstProgramForFold(ins ssa.Instruction, sp *ssa.Program) {
	if ins == nil || sp == nil {
		return
	}
	if ins.GetProgram() == nil {
		ins.SetProgram(sp)
	}
}

func constForReachabilityTruthy(c *ssa.ConstInst) (truthy bool, ok bool) {
	if c == nil {
		return false, false
	}
	if c.IsBoolean() {
		return c.Boolean(), true
	}
	// NewConst(true/false) may set *Const.value to a Go bool while anValue.stored type is not BooleanTypeKind
	// (SetType/saveTypeWithValue path); still treat as provable truth for CFG / or-and phi short-circuit folds.
	if c.Const != nil {
		if rv := c.Const.GetRawValue(); rv != nil {
			if b, ok := rv.(bool); ok {
				return b, true
			}
		}
	}
	if c.IsNumber() {
		return c.Number() != 0, true
	}
	return false, false
}

func sameConstInstForReach(a, b *ssa.ConstInst) bool {
	if a == nil || b == nil {
		return false
	}
	if a.IsNumber() && b.IsNumber() {
		return a.Number() == b.Number()
	}
	if a.IsBoolean() && b.IsBoolean() {
		return a.Boolean() == b.Boolean()
	}
	if a.IsString() && b.IsString() {
		return a.VarString() == b.VarString()
	}
	if a.IsFloat() && b.IsFloat() {
		return a.Float() == b.Float()
	}
	return false
}

// tryFoldValueToConstInst folds comparisons/bool ops on const operands (and phis of identical const) for reachability.
func tryFoldValueToConstInst(prog *Program, valueID int64) (*ssa.ConstInst, bool) {
	if prog == nil || valueID <= 0 || prog.Program == nil {
		return nil, false
	}
	seen := make(map[int64]struct{})
	return tryFoldValueToConstInstRec(prog, valueID, seen)
}

func tryFoldValueToConstInstRec(prog *Program, valueID int64, seen map[int64]struct{}) (*ssa.ConstInst, bool) {
	if valueID <= 0 {
		return nil, false
	}
	if _, ok := seen[valueID]; ok {
		return nil, false
	}
	seen[valueID] = struct{}{}
	defer delete(seen, valueID)

	sp := prog.Program
	ins, ok := sp.GetInstructionById(valueID)
	if !ok || ins == nil {
		return nil, false
	}
	ins = normalizeSSAInst(ins)
	ensureInstProgramForFold(ins, sp)
	if se, ok := ssa.ToSideEffect(ins); ok && se != nil && se.Value > 0 {
		// a = 1; if a < 0: compare operands may be SideEffect (assignment chain), not a bare ConstInst.
		return tryFoldValueToConstInstRec(prog, se.Value, seen)
	}
	if tc, ok := ssa.ToTypeCast(ins); ok && tc != nil && tc.Value > 0 {
		return tryFoldValueToConstInstRec(prog, tc.Value, seen)
	}
	if c, ok := ssa.ToConstInst(ins); ok && c != nil {
		return c, true
	}
	if bop, ok := ssa.ToBinOp(ins); ok && bop != nil {
		ensureInstProgramForFold(bop, sp)
		if cx, xok := tryFoldValueToConstInstRec(prog, bop.X, seen); xok {
			if cy, yok := tryFoldValueToConstInstRec(prog, bop.Y, seen); yok {
				if v := ssa.CalcConstBinary(cx, cy, bop.Op); v != nil {
					if c, ok := ssa.ToConstInst(v); ok && c != nil {
						return c, true
					}
				}
			}
		}
		if out := ssa.HandlerBinOp(bop); out != nil {
			if c, ok := ssa.ToConstInst(out); ok && c != nil {
				return c, true
			}
		}
		_, _ = tryFoldValueToConstInstRec(prog, bop.X, seen)
		_, _ = tryFoldValueToConstInstRec(prog, bop.Y, seen)
		if out := ssa.HandlerBinOp(bop); out != nil {
			if c, ok := ssa.ToConstInst(out); ok && c != nil {
				return c, true
			}
		}
	}
	if uop, ok := ssa.ToUnOp(ins); ok && uop != nil {
		ensureInstProgramForFold(uop, sp)
		_, _ = tryFoldValueToConstInstRec(prog, uop.X, seen)
		if out := ssa.HandlerUnOp(uop); out != nil {
			if c, ok := ssa.ToConstInst(out); ok && c != nil {
				return c, true
			}
		}
	}
	if phi, ok := ssa.ToPhi(ins); ok && phi != nil && len(phi.Edge) >= 2 {
		logicalVn := logicalYakOrAndPhiName(prog, valueID, ins)
		if logicalVn == ssa.OrExpressionVariable || logicalVn == ssa.AndExpressionVariable {
			if c, ok2 := tryFoldShortCircuitAndOrPhiRec(prog, phi, logicalVn, seen); ok2 {
				return c, true
			}
			// 识别为 &&/|| phi 但未能折叠：不拦截其它 phi 规则（如各边同常量）
		}
	}
	if phi, ok := ssa.ToPhi(ins); ok && phi != nil && len(phi.Edge) > 0 {
		var first *ssa.ConstInst
		for _, eid := range phi.Edge {
			c, ok2 := tryFoldValueToConstInstRec(prog, eid, seen)
			if !ok2 || c == nil {
				return nil, false
			}
			if first == nil {
				first = c
			} else if !sameConstInstForReach(first, c) {
				return nil, false
			}
		}
		return first, true
	}
	return nil, false
}

// logicalYakOrAndPhiName 结合 ssaapi Value.String 与 IR 上 meta：同一 id 的裸 Instruction 在 GetName 上可能
// 与 runSyntaxFlow 里结果 Value 的展示名不一致，故以 GetValueById 的 disasm 串为准最稳。
func logicalYakOrAndPhiName(prog *Program, valueID int64, ins ssa.Instruction) string {
	if prog != nil && valueID > 0 {
		if v, err := prog.GetValueById(valueID); err == nil && v != nil {
			s := v.String()
			if strings.Contains(s, "phi("+ssa.OrExpressionVariable+")") {
				return ssa.OrExpressionVariable
			}
			if strings.Contains(s, "phi("+ssa.AndExpressionVariable+")") {
				return ssa.AndExpressionVariable
			}
		}
	}
	return logicalAndOrPhiVariableNameFromIns(ins)
}

// logicalAndOrPhiVariableNameFromIns 用 IR 上 GetVerboseName/Variable/name/LineDisASM 作后备。
func logicalAndOrPhiVariableNameFromIns(ins ssa.Instruction) string {
	if ins == nil {
		return ""
	}
	ins = normalizeSSAInst(ins)
	// Disasm 在 phi 上最准；须先于 GetVerboseName：Yak 或式 phi 的 VerboseName 在部分情况下可能与 and_expression 混用，若先信 VerboseName 会误走 && 折叠。
	if p, ok := ssa.ToPhi(ins); ok && p != nil {
		line := ssa.LineDisASM(p)
		if strings.Contains(line, "phi("+ssa.OrExpressionVariable+")") {
			return ssa.OrExpressionVariable
		}
		if strings.Contains(line, "phi("+ssa.AndExpressionVariable+")") {
			return ssa.AndExpressionVariable
		}
	}
	vn := ins.GetVerboseName()
	if vn == ssa.OrExpressionVariable || vn == ssa.AndExpressionVariable {
		return vn
	}
	if val, ok := ins.(ssa.Value); ok && val != nil {
		if val.GetVariable(ssa.OrExpressionVariable) != nil {
			return ssa.OrExpressionVariable
		}
		if val.GetVariable(ssa.AndExpressionVariable) != nil {
			return ssa.AndExpressionVariable
		}
	}
	if n := ins.GetName(); n != "" {
		if strings.Contains(n, "||") {
			return ssa.OrExpressionVariable
		}
		if strings.Contains(n, "&&") {
			return ssa.AndExpressionVariable
		}
	}
	return ""
}

// tryFoldShortCircuitAndOrPhiRec 对 yak2ssa 的 or_expression/and_expression phi 做常量折叠（与 builder_ast
// handlerJumpExpression 一致：|| 的 Edge[0]=a、Edge[1]=b；&& 的 Edge[0]=b、Edge[1]=a）。
// 若两边均可折成布尔真值，用命题式 t(a)||t(b) / t(a)&&t(b)（与短路在常量上同效），避免仅一边能折时整体不折。
func tryFoldShortCircuitAndOrPhiRec(prog *Program, phi *ssa.Phi, vn string, seen map[int64]struct{}) (*ssa.ConstInst, bool) {
	if prog == nil || phi == nil || len(phi.Edge) < 2 {
		return nil, false
	}
	switch vn {
	case ssa.OrExpressionVariable:
		ca, oa := tryFoldValueToConstInstRec(prog, phi.Edge[0], seen)
		cb, ob := tryFoldValueToConstInstRec(prog, phi.Edge[1], seen)
		if oa && ob && ca != nil && cb != nil {
			ta, okA := constForReachabilityTruthy(ca)
			tb, okB := constForReachabilityTruthy(cb)
			if okA && okB {
				return ssa.NewConst(ta || tb), true
			}
		}
		// 短路：a 为真则整体真；a 为假则整体为 b
		if oa && ca != nil {
			if t, tOk := constForReachabilityTruthy(ca); tOk {
				if t {
					return ssa.NewConst(true), true
				}
				return tryFoldValueToConstInstRec(prog, phi.Edge[1], seen)
			}
		}
		return nil, false
	case ssa.AndExpressionVariable:
		ca, oa := tryFoldValueToConstInstRec(prog, phi.Edge[1], seen) // a
		cb, ob := tryFoldValueToConstInstRec(prog, phi.Edge[0], seen) // b
		if oa && ob && ca != nil && cb != nil {
			ta, okA := constForReachabilityTruthy(ca)
			tb, okB := constForReachabilityTruthy(cb)
			if okA && okB {
				return ssa.NewConst(ta && tb), true
			}
		}
		if oa && ca != nil {
			if t, tOk := constForReachabilityTruthy(ca); tOk {
				if !t {
					return ca, true
				}
				return tryFoldValueToConstInstRec(prog, phi.Edge[0], seen)
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func evalIfCondConstTruth(prog *Program, condID int64) (truthy bool, ok bool) {
	c, ok2 := tryFoldValueToConstInst(prog, condID)
	if !ok2 || c == nil {
		return false, false
	}
	return constForReachabilityTruthy(c)
}

// dropFalsyConstGuardConds 去掉会折叠为「假」的常量 if 条件（<boolean> false 等），避免把该 SSA 值当作结果，
// 与 mustExecute 的 false 混淆。恒真条件（1>0 等）保留：合并块上的 cc 仍挂在「条件为真」的叙述下，
// 与 if/else 两端均可达时的 true 表示一致，且比清空 conds 后走 reachableFuncExitAvoidingBlock 更稳（后者对汇合块易误判）。
func dropFalsyConstGuardConds(prog *Program, conds []*Value) []*Value {
	if prog == nil || len(conds) == 0 {
		return conds
	}
	out := make([]*Value, 0, len(conds))
	for _, v := range conds {
		if v == nil {
			continue
		}
		if k, ok := tryFoldValueToConstInst(prog, v.GetId()); ok && k != nil {
			truthy, tOk := constForReachabilityTruthy(k)
			if tOk && !truthy {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// findIfControllingTwoSucc returns the *ssa.If in block b whose True/False match b.Succs[0,1] when possible.
func findIfControllingTwoSucc(fn *ssa.Function, b *ssa.BasicBlock) *ssa.If {
	if fn == nil || b == nil || len(b.Succs) != 2 {
		return nil
	}
	s0, s1 := b.Succs[0], b.Succs[1]
	for _, iid := range b.Insts {
		ins, ok := b.GetInstructionById(iid)
		if !ok || ins == nil {
			continue
		}
		ins = normalizeSSAInst(ins)
		if ifInst, ok := ssa.ToIfInstruction(ins); ok && ifInst != nil {
			if ifInst.True == s0 && ifInst.False == s1 {
				return ifInst
			}
		}
	}
	for _, iid := range b.Insts {
		ins, ok := b.GetInstructionById(iid)
		if !ok || ins == nil {
			continue
		}
		ins = normalizeSSAInst(ins)
		if ifInst, ok := ssa.ToIfInstruction(ins); ok && ifInst != nil {
			return ifInst
		}
	}
	return nil
}

// constFoldFilteredSuccs 展开块的后继，并在双后继 if 上按可证明的真值只保留可达边（与 blockReachableConstFiltered / reachableFuncExitAvoiding 共用）。
func constFoldFilteredSuccs(prog *Program, fn *ssa.Function, bid int64) []int64 {
	b, ok := fn.GetBasicBlockByID(bid)
	if !ok || b == nil {
		return nil
	}
	succs := b.Succs
	if len(succs) == 2 {
		succs = filterSuccsForConstFoldIfExitSearch(prog, fn, bid, succs)
	}
	return succs
}

// filterSuccsForConstFoldIfExitSearch: 若 if 条件可常量化，只保留条件真/假对应的后继，必须用 ifInst.True/False，
// 因 b.Succs[0,1] 与 (true, false) 的排列未必一致（仅按 succ[0] 为真会误判短路为左真等形）。
func filterSuccsForConstFoldIfExitSearch(prog *Program, fn *ssa.Function, blockID int64, succs []int64) []int64 {
	if len(succs) != 2 || prog == nil || fn == nil {
		return succs
	}
	b, ok := fn.GetBasicBlockByID(blockID)
	if !ok || b == nil {
		return succs
	}
	var ifInst *ssa.If
	if last := b.LastInst(); last != nil {
		if toIf, o := ssa.ToIfInstruction(normalizeSSAInst(last)); o && toIf != nil {
			ifInst = toIf
		}
	}
	if ifInst == nil {
		ifInst = findIfControllingTwoSucc(fn, b)
	}
	if ifInst == nil || ifInst.Cond <= 0 {
		return succs
	}
	t, o := evalIfCondConstTruth(prog, ifInst.Cond)
	if !o {
		return succs
	}
	if t {
		return []int64{ifInst.True}
	}
	return []int64{ifInst.False}
}

// blockReachableConstFilteredIf is like plain CFG BFS to toBlock but only follows feasible 2-exit if edges.
func blockReachableConstFilteredIf(prog *Program, fn *ssa.Function, fromBlockID, toBlockID int64) bool {
	if prog == nil || fn == nil || fromBlockID <= 0 || toBlockID <= 0 {
		return false
	}
	if fromBlockID == toBlockID {
		return true
	}
	queue := []int64{fromBlockID}
	seen := make(map[int64]struct{}, 32)
	seen[fromBlockID] = struct{}{}
	for qi := 0; qi < len(queue); qi++ {
		bid := queue[qi]
		if bid == toBlockID {
			return true
		}
		for _, s := range constFoldFilteredSuccs(prog, fn, bid) {
			if s <= 0 {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			queue = append(queue, s)
		}
	}
	return false
}

// reachableFuncExitAvoidingBlock reports whether there is a CFG path from startBlockID
// to the function exit block that never enters avoidBlockID (used to refute vacuous postDom).
func reachableFuncExitAvoidingBlock(prog *Program, funcID int64, startBlockID, avoidBlockID int64) bool {
	if prog == nil || funcID <= 0 || startBlockID <= 0 || avoidBlockID <= 0 {
		return false
	}
	fn, err := getFunctionByID(prog, funcID)
	if err != nil || fn == nil {
		return false
	}
	exitID := fn.ExitBlock
	if exitID <= 0 {
		return false
	}
	if startBlockID == avoidBlockID {
		return false
	}
	queue := []int64{startBlockID}
	seen := make(map[int64]struct{}, 32)
	seen[startBlockID] = struct{}{}
	for qi := 0; qi < len(queue); qi++ {
		bid := queue[qi]
		if bid == exitID || isExitLikeBlock(fn, bid) {
			return true
		}
		for _, s := range constFoldFilteredSuccs(prog, fn, bid) {
			if s <= 0 || s == avoidBlockID {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			queue = append(queue, s)
		}
	}
	return false
}

func reachabilityGuardReturn(prog *Program, anchor *Value, vals []*Value) (bool, sfvm.Values, error) {
	return true, sfvm.NewValues(buildReachabilityGuardValues(prog, anchor, vals)), nil
}

func reachabilityGuardConst(prog *Program, anchor *Value, b bool) (bool, sfvm.Values, error) {
	return reachabilityGuardReturn(prog, anchor, []*Value{prog.NewConstValue(b)})
}

func appendCondsFromCondValueIDs(prog *Program, ids []int64, conds []*Value) []*Value {
	if len(ids) == 0 {
		return conds
	}
	var raw []sfvm.ValueOperator
	appendResolvedCondValues(prog, ids, &raw)
	for _, op := range raw {
		if vv, ok := op.(*Value); ok && vv != nil {
			conds = append(conds, vv)
		}
	}
	return conds
}

func nativeCallReachabilityGuard(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	if frame == nil || params == nil {
		return false, nil, utils.Errorf("%s: nil frame or params", NativeCall_ReachabilityGuard)
	}
	mode := strings.TrimSpace(params.GetString(reachabilityGuardParamMode, 0))
	if mode == "" {
		mode = reachabilityGuardModeMustExecute
	}
	if !strings.EqualFold(mode, reachabilityGuardModeMustExecute) {
		return false, nil, utils.Errorf("%s: unsupported mode %q (only %s)", NativeCall_ReachabilityGuard, mode, reachabilityGuardModeMustExecute)
	}

	toCtx, err := resolveCfgTargetFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, NativeCall_ReachabilityGuard)
	}
	if toCtx == nil || toCtx.IsEmpty() {
		return false, nil, utils.Errorf("%s: empty target cfg", NativeCall_ReachabilityGuard)
	}
	prog := toCtx.prog
	if prog == nil {
		return false, nil, utils.Errorf("%s: target has no program", NativeCall_ReachabilityGuard)
	}
	var fnAtTarget *ssa.Function
	if toCtx.FuncID > 0 {
		fnAtTarget, _ = getFunctionByID(prog, toCtx.FuncID)
	}

	fromVal := firstValueFromPipeForReachability(v)
	anchor := anchorValForReachabilityGuard(fromVal, toCtx, prog)
	if fromVal != nil {
		if fromVal.ParentProgram != nil && fromVal.ParentProgram != prog {
			return reachabilityGuardConst(prog, anchor, false)
		}
		ins := fromVal.getInstruction()
		if ins != nil {
			fnFrom := ins.GetFunc()
			if fnAtTarget != nil && fnFrom != nil && fnFrom.GetId() != fnAtTarget.GetId() {
				return reachabilityGuardConst(prog, anchor, false)
			}
		}
	}

	enterCtx, err := cfgCtxForFunctionEntry(prog, toCtx.FuncID)
	if err != nil {
		return reachabilityGuardConst(prog, anchor, false)
	}

	reachOpt := reachableOptions{icfg: false, maxDepth: 0, maxNodes: 0}
	if !reachableWithOptions(prog, enterCtx, toCtx, reachOpt) {
		return reachabilityGuardConst(prog, anchor, false)
	}
	if fnAtTarget != nil && toCtx.BlockID > 0 {
		if !blockReachableConstFilteredIf(prog, fnAtTarget, enterCtx.BlockID, toCtx.BlockID) {
			return reachabilityGuardConst(prog, anchor, false)
		}
	}

	mustExec := postDominates(prog, enterCtx, toCtx)
	if mustExec && toCtx.BlockID > 0 {
		// postDom 在死块/不可达边上可能过强：若存在不经过 target 所在块即可到出口的路径，则不应视为 mustExecute。
		if reachableFuncExitAvoidingBlock(prog, toCtx.FuncID, enterCtx.BlockID, toCtx.BlockID) {
			mustExec = false
		}
	}
	if mustExec {
		return reachabilityGuardConst(prog, anchor, true)
	}

	var conds []*Value
	if fnAtTarget != nil {
		guards := computeCfgGuardsPredicates(prog, fnAtTarget, toCtx)
		seenCond := make(map[int64]struct{}, len(guards))
		for _, g := range guards {
			if g == nil || g.CondValueID <= 0 {
				continue
			}
			if _, dup := seenCond[g.CondValueID]; dup {
				continue
			}
			// 仅 reachability：去掉「无 else、仅汇合」的前置 if 的谓词（gt(aa) 等），不修改 cfgGuards 对 panic/continue 的判定。
			if gb, okb := fnAtTarget.GetBasicBlockByID(g.GuardBlockID); okb && gb != nil {
				if last := gb.LastInst(); last != nil {
					if ifInst, ok2 := ssa.ToIfInstruction(normalizeSSAInst(last)); ok2 && ifInst != nil {
						if ifRejoinsWithoutElsePayload(fnAtTarget, ifInst.True, ifInst.False) {
							continue
						}
					}
				}
			}
			seenCond[g.CondValueID] = struct{}{}
			conds = appendCondsFromCondValueIDs(prog, []int64{g.CondValueID}, conds)
		}
	}
	if len(conds) == 0 {
		summary, err2 := getBlockConditionSummaryByCfgCtx(prog, toCtx)
		if err2 == nil && summary != nil {
			conds = appendCondsFromCondValueIDs(prog, summary.CondValueID, conds)
		}
	}
	conds = dropFalsyConstGuardConds(prog, conds)
	if len(conds) == 0 {
		// 无枚举到的分支谓词时：若存在「不进入 target 所在块即可到出口」的路径，则 target 非全路径必达（含死代码上的 target）。
		if toCtx.BlockID > 0 && reachableFuncExitAvoidingBlock(prog, toCtx.FuncID, enterCtx.BlockID, toCtx.BlockID) {
			return reachabilityGuardConst(prog, anchor, false)
		}
		return reachabilityGuardConst(prog, anchor, true)
	}

	return reachabilityGuardReturn(prog, anchor, conds)
}

func anchorValForReachabilityGuard(fromVal *Value, toCtx *CfgCtxValue, prog *Program) *Value {
	if fromVal != nil {
		return fromVal
	}
	if toCtx == nil || prog == nil {
		return nil
	}
	v, err := prog.GetValueById(toCtx.InstID)
	if err != nil {
		return nil
	}
	return v
}

// buildReachabilityGuardValues 将 reachabilityGuard 的返回值压栈：每项为 bool 常量或一条到达 target 时沿路径收集的条件 SSA value（可多项，如嵌套 if）。
func buildReachabilityGuardValues(prog *Program, anchor *Value, vals []*Value) []sfvm.ValueOperator {
	if prog == nil {
		return nil
	}
	ab := anchorBitVecForReachabilityGuard(anchor)
	out := make([]sfvm.ValueOperator, 0, len(vals))
	for _, val := range vals {
		if val == nil {
			continue
		}
		if ab != nil && !ab.IsEmpty() {
			val.SetAnchorBitVector(ab)
		}
		out = append(out, val)
	}
	return out
}

func anchorBitVecForReachabilityGuard(v *Value) *utils.BitVector {
	if v == nil {
		return nil
	}
	return v.GetAnchorBitVector()
}
