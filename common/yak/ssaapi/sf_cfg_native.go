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

func resolveCfgTargetFromFrame(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (*CfgCtxValue, error) {
	if frame == nil {
		return nil, utils.Error("cfg*: frame is nil")
	}
	if params == nil {
		return nil, utils.Error("cfg*: params is nil")
	}
	targetVar := params.GetString(0, "target", "var", "against")
	if targetVar == "" {
		return nil, utils.Error("cfg*: 'target' parameter is required (e.g. target=$sinkCfg)")
	}
	targetVar = strings.TrimPrefix(targetVar, "$")
	targetVals, ok := frame.GetSymbolByName(targetVar)
	if !ok || targetVals == nil {
		return nil, utils.Errorf("cfg*: variable '$%s' not found in current frame", targetVar)
	}
	first, err := firstCfgCtxFromSymbolValues(targetVals)
	if err != nil {
		return nil, utils.Wrapf(err, "cfg*: variable '$%s' has no cfg anchor (use <getCfg> or an SSA value with func/block/inst)", targetVar)
	}
	return first, nil
}

// mapCfgCtxAgainstTarget resolves `target` from the frame, then evaluates fn(prog, recv, targ)
// for each cfg ctx recv on the value stack (SyntaxFlow pipeline cfg).
func mapCfgCtxAgainstTarget(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams, opName string, fn func(prog *Program, recv, targ *CfgCtxValue) bool) (bool, sfvm.Values, error) {
	pipe, prog, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	target, err := resolveCfgTargetFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, opName)
	}
	var out []sfvm.ValueOperator
	_ = pipe.Recursive(func(op sfvm.ValueOperator) error {
		receiver, ok := extractCfgCtx(op)
		if !ok || receiver.IsEmpty() {
			return nil
		}
		out = append(out, prog.NewConstValue(fn(prog, receiver, target)))
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

func nativeCallCFGReachable(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	target, err := resolveCfgTargetFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, "cfgReachable")
	}
	pipe, _, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, "cfgReachable")
	}
	icfg := parseBoolParam(params, "icfg", false)
	opt := reachableOptsFromParams(params, icfg)
	return mapCfgCtxValues(pipe, "cfgReachable: no cfg ctx values", func(prog *Program, _ sfvm.ValueOperator, a *CfgCtxValue, out *[]sfvm.ValueOperator) {
		*out = append(*out, prog.NewConstValue(reachableWithOptions(prog, a, target, opt)))
	})
}

func nativeCallCFGReachPath(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	target, err := resolveCfgTargetFromFrame(frame, params)
	if err != nil {
		return false, nil, utils.Wrap(err, "cfgReachPath")
	}
	pipe, _, err := coerceCfgCallInputs(v)
	if err != nil {
		return false, nil, utils.Wrap(err, "cfgReachPath")
	}
	icfg := parseBoolParam(params, "icfg", false)
	opt := reachableOptsFromParams(params, icfg)
	return mapCfgCtxValues(pipe, "cfgReachPath: no cfg ctx values", func(prog *Program, _ sfvm.ValueOperator, a *CfgCtxValue, out *[]sfvm.ValueOperator) {
		s := cfgReachShortestPathString(prog, a, target, opt)
		*out = append(*out, prog.NewConstValue(s))
	})
}

// minimal guard extraction: detect if-in-block dominates sink block and one branch goes to exit.
func nativeCallCFGGuards(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return mapCfgCtxValues(v, "no guards found", func(prog *Program, op sfvm.ValueOperator, ctx *CfgCtxValue, out *[]sfvm.ValueOperator) {
		fn, err := getFunctionByID(prog, ctx.FuncID)
		if err != nil || fn == nil {
			return
		}

		_ = getOrBuildDomCache(prog, ctx.FuncID)

		loopExits, loopLatches := cfgGuardsLoopBreakContinueTargets(fn)

		guards := make([]*GuardPredicateValue, 0, 4)
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

			branchCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: b.GetId(), InstID: instID}
			if !reachable(prog, branchCtx, ctx) {
				continue
			}

			exitID := fn.ExitBlock
			var exitCtx *CfgCtxValue
			if exitID > 0 {
				exitCtx = &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: exitID}
			}

			tCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: tBranch}
			fCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: fBranch}
			targetCtx := &CfgCtxValue{prog: prog, FuncID: ctx.FuncID, BlockID: ctx.BlockID}

			tReach := reachable(prog, tCtx, targetCtx)
			fReach := reachable(prog, fCtx, targetCtx)
			if !tReach && !fReach {
				continue
			}

			tExit := isExitLikeBlock(fn, tBranch)
			fExit := isExitLikeBlock(fn, fBranch)
			if exitCtx != nil {
				tExit = tExit || tBranch == exitID || reachable(prog, tCtx, exitCtx)
				fExit = fExit || fBranch == exitID || reachable(prog, fCtx, exitCtx)
			}

			if tExit && fReach {
				guards = append(guards, &GuardPredicateValue{
					prog:         prog,
					FuncID:       ctx.FuncID,
					GuardBlockID: b.GetId(),
					SinkBlockID:  ctx.BlockID,
					CondInstID:   instID,
					CondValueID:  condValueID,
					Polarity:     false,
					Kind:         cfgGuardsAbortBranchKind(fn, tBranch, loopExits, loopLatches),
					Text:         "",
				})
			} else if fExit && tReach {
				guards = append(guards, &GuardPredicateValue{
					prog:         prog,
					FuncID:       ctx.FuncID,
					GuardBlockID: b.GetId(),
					SinkBlockID:  ctx.BlockID,
					CondInstID:   instID,
					CondValueID:  condValueID,
					Polarity:     true,
					Kind:         cfgGuardsAbortBranchKind(fn, fBranch, loopExits, loopLatches),
					Text:         "",
				})
			}
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
