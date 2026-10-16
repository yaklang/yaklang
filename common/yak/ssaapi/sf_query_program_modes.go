package ssaapi

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// queryProgramStructRule runs a struct rule over every application/library unit
// of the program and merges the unit frames into one result.
//
// The caller is a query that already compiled the rule's frame (rule object or
// rule content); base carries its context, runtime, task and sfvm options so
// each unit runs exactly like the caller's query would. The unit runs do not
// finalize (QueryWithNoFinalize), so risks are built and consumers are
// notified once, by the caller, through finalizeProgramRuleResult.
func (p *Program) queryProgramStructRule(base *queryConfig, rule *schema.SyntaxFlowRule) (*SyntaxFlowResult, error) {
	if p == nil || p.Program == nil {
		return nil, utils.Error("struct rule: nil program")
	}
	if rule == nil {
		return nil, utils.Error("struct rule: nil rule")
	}
	if p.IsIncrementalCompile() && !p.IsBaseProgram() && p.GetOverlay() == nil {
		return nil, utils.Error("incremental program overlay unavailable for struct review")
	}
	var accumulated *sfvm.SFFrameResult
	var sharedConfig *ssaconfig.Config
	runUnit := func(source *Program, unit *ssa.CompileUnit, bound *structBound) error {
		if base != nil && base.ctx != nil {
			if err := base.ctx.Err(); err != nil {
				return err
			}
		}
		target := NewStructQueryTarget(source, unit, bound)
		previousBound, previousActive := source.structBound, source.structScanActive
		source.structBound, source.structScanActive = target.bound, true
		defer func() { source.structBound, source.structScanActive = previousBound, previousActive }()

		frame, _, err := sfvm.NewSyntaxFlowVirtualMachine().Load(rule)
		if err != nil {
			return utils.Wrapf(err, "struct rule %s: load frame failed", ruleGetRuleName(rule))
		}
		unitOpts := []QueryOption{
			QueryWithValue(target),
			QueryWithResultProgram(p),
			queryWithStructBound(target.bound),
			QueryWithFrame(frame),
			QueryWithRule(rule),
			QueryWithNoFinalize(),
		}
		if base != nil {
			sharedConfig = base.Config
			unitOpts = append(unitOpts,
				QueryWithContext(base.ctx),
				QueryWithTaskID(base.taskID),
				QueryWithSSAConfig(base.Config),
			)
			for _, fn := range base.onRisk {
				unitOpts = append(unitOpts, QueryWithOnRisk(fn))
			}
			for _, opt := range base.opts {
				unitOpts = append(unitOpts, QueryWithSFOption(opt))
			}
		}
		res, err := QuerySyntaxflow(unitOpts...)
		if err != nil {
			return err
		}
		if res == nil || res.memResult == nil {
			return nil
		}
		if accumulated == nil {
			accumulated = sfvm.NewSFFrameResultAccumulator(res.memResult)
		} else {
			accumulated.MergeByResult(res.memResult)
		}
		return nil
	}
	if overlay := p.GetOverlay(); overlay != nil && overlay.IsTopLayerProgram(p) {
		if err := walkOverlayStructUnits(overlay, runUnit); err != nil {
			return nil, err
		}
	} else {
		units, err := programStructUnits(p)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			return nil, utils.Errorf("struct rule %s: program %s has no application/library unit", ruleGetRuleName(rule), p.GetProgramName())
		}
		for _, unit := range units {
			if unit == nil {
				continue
			}
			if err := runUnit(p, unit, nil); err != nil {
				return nil, err
			}
		}
	}
	if accumulated == nil {
		return nil, nil
	}
	merged := CreateResultFromQuery(accumulated, sharedConfig)
	merged.program = p
	merged.rule = rule
	if base != nil {
		merged.TaskID = base.taskID
		merged.onRisk = base.onRisk
		merged.dbKind = base.kind
		merged.Config = base.Config
	}
	return merged, nil
}

// finalizeProgramRuleResult publishes a merged result the way a normal query
// would. A database result is saved here, which builds its risks. A memory
// result only builds risks. Either way a registered callback receives each
// risk and the result does not write the risk row itself.
func finalizeProgramRuleResult(res *SyntaxFlowResult) error {
	if res == nil {
		return nil
	}
	if res.GetSyntaxFlowResultKind() == ssaconfig.SFResultSaveDatabase {
		kind := res.dbKind
		if kind == "" {
			kind = schema.SFResultKindScan
		}
		_, err := res.Save(kind, res.TaskID)
		return err
	}
	return res.CreateRisk()
}
