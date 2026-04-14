package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func cloneMetaStringAny(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func blockConditionSummaryFromTerminator(pb *ssa.BasicBlock) (ssa.BlockConditionSummary, bool) {
	if pb == nil || len(pb.Insts) == 0 {
		return ssa.BlockConditionSummary{}, false
	}
	for i := len(pb.Insts) - 1; i >= 0; i-- {
		ins, ok := pb.GetInstructionById(pb.Insts[i])
		if !ok || ins == nil {
			continue
		}
		if iff, ok := ssa.ToIfInstruction(ins); ok && iff != nil && iff.Cond > 0 {
			ps := pb.BlockConditionSummary()
			ps.CondInstID = iff.GetId()
			ps.CondValueID = []int64{iff.Cond}
			if ps.Meta == nil {
				ps.Meta = map[string]any{}
			}
			if ps.Meta["source"] == nil || fmt.Sprint(ps.Meta["source"]) == "" {
				ps.Meta["source"] = "if"
			}
			return ps, true
		}
		raw := ins
		if lz, ok := ssa.ToLazyInstruction(raw); ok && lz != nil {
			raw = lz.Self()
		}
		if sw, ok := raw.(*ssa.Switch); ok && sw != nil && sw.Cond > 0 {
			ps := pb.BlockConditionSummary()
			ps.CondInstID = sw.GetId()
			ps.CondValueID = []int64{sw.Cond}
			if ps.Meta == nil {
				ps.Meta = map[string]any{}
			}
			if ps.Meta["source"] == nil || fmt.Sprint(ps.Meta["source"]) == "" {
				ps.Meta["source"] = "switch"
			}
			return ps, true
		}
		if lp, ok := raw.(*ssa.Loop); ok && lp != nil && lp.Cond > 0 {
			ps := pb.BlockConditionSummary()
			ps.CondInstID = lp.GetId()
			ps.CondValueID = []int64{lp.Cond}
			if ps.Meta == nil {
				ps.Meta = map[string]any{}
			}
			if ps.Meta["source"] == nil || fmt.Sprint(ps.Meta["source"]) == "" {
				ps.Meta["source"] = "loop"
			}
			return ps, true
		}
	}
	return ssa.BlockConditionSummary{}, false
}

func enrichBlockConditionSummaryViaIDom(prog *Program, fn *ssa.Function, anchorBlockID int64, direct ssa.BlockConditionSummary) ssa.BlockConditionSummary {
	if len(direct.CondValueID) > 0 {
		return direct
	}
	cache := getOrBuildDomCache(prog, fn.GetId())
	if cache == nil || cache.idom == nil {
		return direct
	}
	cur := anchorBlockID
	for step := 0; step < 512; step++ {
		parent := cache.idom[cur]
		if parent <= 0 || parent == cur {
			break
		}
		pb, ok := fn.GetBasicBlockByID(parent)
		if !ok || pb == nil {
			break
		}
		ps := pb.BlockConditionSummary()
		if len(ps.CondValueID) == 0 {
			if syn, ok := blockConditionSummaryFromTerminator(pb); ok {
				ps = syn
			}
		}
		if len(ps.CondValueID) == 0 {
			cur = parent
			continue
		}
		meta := cloneMetaStringAny(ps.Meta)
		if meta == nil {
			meta = map[string]any{}
		}
		if direct.Meta != nil {
			for k, v := range direct.Meta {
				meta[k] = v
			}
		}
		meta["condition_anchor_block"] = anchorBlockID
		meta["inherited_from_block"] = parent
		meta["inherited_via"] = "idom"
		return ssa.BlockConditionSummary{
			FuncID:      fn.GetId(),
			BlockID:     anchorBlockID,
			CondInstID:  ps.CondInstID,
			CondValueID: append([]int64(nil), ps.CondValueID...),
			Meta:        meta,
		}
	}
	return direct
}

func enrichBlockConditionSummaryViaPredBFS(fn *ssa.Function, anchorBlockID int64, direct ssa.BlockConditionSummary) ssa.BlockConditionSummary {
	if len(direct.CondValueID) > 0 {
		return direct
	}
	seen := make(map[int64]struct{})
	queue := []int64{anchorBlockID}
	for qi := 0; qi < len(queue); qi++ {
		bid := queue[qi]
		if bid <= 0 {
			continue
		}
		if _, dup := seen[bid]; dup {
			continue
		}
		seen[bid] = struct{}{}

		pb, ok := fn.GetBasicBlockByID(bid)
		if !ok || pb == nil {
			continue
		}
		ps := pb.BlockConditionSummary()
		if len(ps.CondValueID) == 0 {
			if syn, ok2 := blockConditionSummaryFromTerminator(pb); ok2 {
				ps = syn
			}
		}
		if len(ps.CondValueID) > 0 {
			meta := cloneMetaStringAny(ps.Meta)
			if meta == nil {
				meta = map[string]any{}
			}
			if direct.Meta != nil {
				for k, v := range direct.Meta {
					meta[k] = v
				}
			}
			meta["condition_anchor_block"] = anchorBlockID
			meta["inherited_from_block"] = bid
			meta["inherited_via"] = "pred_bfs"
			return ssa.BlockConditionSummary{
				FuncID:      fn.GetId(),
				BlockID:     anchorBlockID,
				CondInstID:  ps.CondInstID,
				CondValueID: append([]int64(nil), ps.CondValueID...),
				Meta:        meta,
			}
		}
		for _, p := range pb.Preds {
			if p > 0 {
				queue = append(queue, p)
			}
		}
	}
	return direct
}

func getBlockConditionSummaryByCfgCtx(prog *Program, ctx *CfgCtxValue) (*ssa.BlockConditionSummary, error) {
	if prog == nil || ctx == nil || ctx.IsEmpty() {
		return nil, utils.Error("invalid cfg ctx")
	}
	fn, err := getFunctionByID(prog, ctx.FuncID)
	if err != nil {
		return nil, err
	}
	blk, err := getBlockByID(fn, ctx.BlockID)
	if err != nil {
		return nil, err
	}
	s := blk.BlockConditionSummary()
	if s.FuncID <= 0 {
		s.FuncID = ctx.FuncID
	}
	if s.BlockID <= 0 {
		s.BlockID = ctx.BlockID
	}
	s = enrichBlockConditionSummaryViaIDom(prog, fn, ctx.BlockID, s)
	if len(s.CondValueID) == 0 {
		s = enrichBlockConditionSummaryViaPredBFS(fn, ctx.BlockID, s)
	}
	return &s, nil
}

func cfgCtxForValueMemo(p *Program, v *Value, memo map[int64]*CfgCtxValue) *CfgCtxValue {
	if p == nil || v == nil {
		return nil
	}
	id := v.GetId()
	if memo != nil && id > 0 {
		if c, ok := memo[id]; ok {
			return c
		}
	}
	inst := v.getInstruction()
	if inst == nil {
		if memo != nil && id > 0 {
			memo[id] = nil
		}
		return nil
	}
	fn := inst.GetFunc()
	blk := inst.GetBlock()
	if fn == nil || blk == nil {
		if memo != nil && id > 0 {
			memo[id] = nil
		}
		return nil
	}
	ctx := &CfgCtxValue{
		prog:    p,
		FuncID:  fn.GetId(),
		BlockID: blk.GetId(),
		InstID:  inst.GetId(),
	}
	if memo != nil && id > 0 {
		memo[id] = ctx
	}
	return ctx
}

func cfgConditionForValueMemo(p *Program, v *Value, cfgMemo map[int64]*CfgCtxValue, condMemo map[int64]*ssa.BlockConditionSummary) *ssa.BlockConditionSummary {
	if p == nil || v == nil {
		return nil
	}
	id := v.GetId()
	if condMemo != nil && id > 0 {
		if c, ok := condMemo[id]; ok {
			return c
		}
	}
	cfg := cfgCtxForValueMemo(p, v, cfgMemo)
	if cfg == nil || cfg.IsEmpty() {
		if condMemo != nil && id > 0 {
			condMemo[id] = nil
		}
		return nil
	}
	summary, _ := getBlockConditionSummaryByCfgCtx(p, cfg)
	if condMemo != nil && id > 0 {
		condMemo[id] = summary
	}
	return summary
}

// summaryToCondString renders BlockConditionSummary as the cfgCondition native string.
func summaryToCondString(summary *ssa.BlockConditionSummary) string {
	if summary == nil {
		return ""
	}
	source := ""
	schemaVersion := ""
	if summary.Meta != nil {
		source = fmt.Sprintf("%v", summary.Meta["source"])
		schemaVersion = fmt.Sprintf("%v", summary.Meta["schema_version"])
	}
	return fmt.Sprintf("cond(func=%d,block=%d,inst=%d,values=%v,source=%s,schema=%s)",
		summary.FuncID, summary.BlockID, summary.CondInstID, summary.CondValueID, source, schemaVersion)
}

// appendResolvedCondValues maps CondValueID entries to SSA API values (GetValueById first).
func appendResolvedCondValues(prog *Program, ids []int64, out *[]sfvm.ValueOperator) {
	if prog == nil || out == nil {
		return
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		val, err := prog.GetValueById(id)
		if err == nil && val != nil {
			*out = append(*out, val)
			continue
		}
		ins, ok := prog.Program.GetInstructionById(id)
		if ok && ins != nil {
			val, err = prog.NewValue(ins)
			if err == nil && val != nil {
				*out = append(*out, val)
				continue
			}
		}
		*out = append(*out, prog.NewConstValue(id))
	}
}
