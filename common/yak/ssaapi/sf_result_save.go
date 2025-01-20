package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func LoadResultByID(resultID uint) (*SyntaxFlowResult, error) {
	res := createEmptyResult()
	result, err := ssadb.GetResultByID(resultID)
	if err != nil {
		return nil, err
	}
	res.dbResult = result
	var rule *schema.SyntaxFlowRule
	if result.RuleName != "" {
		// load rule from db
		rule, err = sfdb.GetRulePure(result.RuleName)
		if err != nil {
			return nil, utils.Errorf("load rule %s error: %v", result.RuleName, err)
		}
	} else {
		// create rule
		rule = &schema.SyntaxFlowRule{}
		rule.Title = result.RuleTitle
		rule.Severity = schema.SyntaxFlowSeverity(result.RuleSeverity)
		rule.Description = result.RuleDesc
		rule.AlertDesc = result.AlertDesc
	}
	res.rule = rule
	prog, err := FromDatabase(result.ProgramName)
	if err != nil {
		return nil, utils.Errorf("load program %s error: %v", result.ProgramName, err)
	}
	res.program = prog
	return res, nil
}

func (r *SyntaxFlowResult) Save(kind schema.SyntaxflowResultKind, TaskIDs ...string) (uint, error) {
	if r == nil || r.memResult == nil || r.program == nil {
		return 0, utils.Error("result or program  is nil")
	}
	// result
	result := ssadb.CreateResult(TaskIDs...)
	result.CheckMsg = r.GetCheckMsg()
	result.Errors = r.GetErrors()

	// rule
	rule := r.memResult.GetRule()
	if rule.ID > 0 {
		// can get from database
		result.RuleName = rule.RuleName
	}
	// save info in result
	result.Kind = kind
	result.RuleTitle = rule.Title
	result.RuleSeverity = string(schema.ValidSeverityType(rule.Severity))
	result.RuleDesc = rule.Description
	result.AlertDesc = rule.AlertDesc
	result.RuleContent = rule.Content
	// program
	result.ProgramName = r.program.GetProgramName()
	// value
	var errs error
	if err := r.saveValue(result); err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	result.RiskCount = uint64(len(r.riskMap))
	result.RiskHashs = lo.MapValues(r.riskMap, func(risk *schema.SSARisk, name string) string {
		return risk.Hash
	})
	if err := ssadb.SaveResult(result); err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	r.dbResult = result
	return result.ID, errs
}

func (r *SyntaxFlowResult) saveValue(result *ssadb.AuditResult) error {
	// result := r.dbResult
	if result == nil {
		return utils.Error("result is nil")
	}
	// values
	var err error
	opts := []SaveValueOption{
		// task
		OptionSaveValue_TaskID(result.TaskID),
		// result
		OptionSaveValue_ResultID(result.ID),
		// rule
		OptionSaveValue_RuleName(result.RuleName),
		OptionSaveValue_RuleTitle(result.RuleTitle),
		// program
		// program
		OptionSaveValue_ProgramName(result.ProgramName),
	}
	saveVariable := func(name string, values Values) {
		opts := append(opts, OptionSaveValue_ResultVariable(name))
		// save un value variable
		if len(values) == 0 {
			result.UnValueVariable = append(result.UnValueVariable, name)
			return
		}
		if msg, ok := r.GetAlertMsg(name); ok {
			opts = append(opts, OptionSaveValue_ResultAlert(msg))
			r.SaveRisk(name, result)
		}
		// save variable that has value
		for _, v := range values {
			e := SaveValue(v, opts...)
			err = utils.JoinErrors(err, e)
		}
	}

	r.GetAllVariable().ForEach(func(name string, value any) {
		values := r.GetValues(name)
		saveVariable(name, values)
	})
	saveVariable("_", r.GetUnNameValues())
	return err
}

func (r *SyntaxFlowResult) GetGRPCModelResult() *ypb.SyntaxFlowResult {
	if r == nil || r.dbResult == nil {
		return nil
	}
	return r.dbResult.ToGRPCModel()
}
