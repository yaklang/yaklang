package ssaapi

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

// queryConfigOf applies query options to a throwaway config. A helper that
// builds its own result uses it to keep the caller's scan wiring (runtime,
// task, callbacks) instead of starting a second one.
func queryConfigOf(opts ...QueryOption) *queryConfig {
	cfg := &queryConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// queryProgramSourceRule runs a source rule against the program's own source
// snapshot (FileList / ExtraFile / IrSource editors), so a compiled program can
// execute source rules without a live source path.
func (p *Program) queryProgramSourceRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	if p == nil || p.Program == nil {
		return nil, utils.Error("source rule: nil program")
	}
	target := NewSourceQueryTargetFromProgram(p)
	if target == nil || len(target.Files()) == 0 {
		return nil, utils.Errorf(
			"source rule %s: program %s has no source snapshot to scan",
			ruleGetRuleName(rule), p.GetProgramName(),
		)
	}
	return target.SyntaxFlowRule(rule, opts...)
}

// queryProgramStructRule runs a struct rule over every application/library unit
// of the program. Each unit runs without finalizing, and the unit frames merge
// into one result so the rule reports a single result per program.
func (p *Program) queryProgramStructRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	if p == nil || p.Program == nil {
		return nil, utils.Error("struct rule: nil program")
	}
	units := programStructUnits(p)
	if len(units) == 0 {
		return nil, utils.Errorf(
			"struct rule %s: program %s has no application/library unit",
			ruleGetRuleName(rule), p.GetProgramName(),
		)
	}
	base := queryConfigOf(opts...)
	var accumulated *sfvm.SFFrameResult
	for _, unit := range units {
		if unit == nil {
			continue
		}
		frame, _, err := sfvm.NewSyntaxFlowVirtualMachine().Load(rule)
		if err != nil {
			return nil, utils.Wrapf(err, "struct rule %s: load frame failed", ruleGetRuleName(rule))
		}
		unitOpts := append([]QueryOption{}, opts...)
		unitOpts = append(unitOpts,
			QueryWithValue(NewStructQueryTarget(p, unit, nil)),
			QueryWithResultProgram(p),
			QueryWithStruct(unit),
			QueryWithFrame(frame),
			QueryWithRule(rule),
			QueryWithNoFinalize(),
		)
		res, err := QuerySyntaxflow(unitOpts...)
		if err != nil {
			return nil, err
		}
		if res == nil || res.memResult == nil {
			continue
		}
		if accumulated == nil {
			accumulated = sfvm.NewSFFrameResultAccumulator(res.memResult)
		} else {
			accumulated.MergeByResult(res.memResult)
		}
	}
	if accumulated == nil {
		return nil, nil
	}
	merged := CreateResultFromQuery(accumulated, base.Config)
	merged.program = p
	merged.rule = rule
	merged.TaskID = base.taskID
	merged.scanRuntime = base.scanRuntime
	return merged, nil
}

// finalizeProgramRuleResult publishes a merged result the way a normal query
// would: a scan runtime owns the risk decision and the consumers, and a result
// outside a scan keeps its risks in memory only. The result is emitted before
// its risks are built so a database consumer can write the row first and the
// stored risk carries the row id.
func finalizeProgramRuleResult(res *SyntaxFlowResult) error {
	if res == nil {
		return nil
	}
	if res.scanRuntime != nil {
		if err := res.scanRuntime.EmitResult(res); err != nil {
			return err
		}
	}
	return res.CreateRisk()
}
