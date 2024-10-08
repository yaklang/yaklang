package ssaapi

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (r *SyntaxFlowResult) Save(ResultID, TaskID string, rule *schema.SyntaxFlowRule, prog *Program) error {
	// result
	db := ssadb.GetDB()
	result := &ssadb.AuditResult{
		TaskID:   TaskID,
		ResultID: ResultID,
		CheckMsg: r.GetCheckMsg(),
		Errors:   r.GetErrors(),
	}
	if rule != nil {
		result.RuleName = rule.RuleName
		result.RuleTitle = rule.Title
		result.RuleSeverity = string(rule.Severity)
		result.RuleType = string(rule.Type)
		result.RuleDesc = rule.Description
	}
	if prog != nil {
		result.ProgramName = prog.GetProgramName()
	}
	// value
	var errs error
	if err := r.saveValue(result); err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	if err := db.Save(result).Error; err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	r.dbResult = result
	return errs
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
		OptionSaveValue_ResultID(result.ResultID),
		// rule
		OptionSaveValue_RuleName(result.RuleName),
		OptionSaveValue_RuleTitle(result.RuleTitle),
		// program
		OptionSaveValue_ProgramName(result.ProgramName),
	}
	saveVariable := func(name string, values Values) {
		newOpts := append(opts, OptionSaveValue_ResultVariable(name))
		if msg, ok := r.GetAlertInfo(name); ok {
			newOpts = append(newOpts, OptionSaveValue_ResultAlert(msg))
		}
		// save un value variable
		if len(values) == 0 {
			result.UnValueVariable = append(result.UnValueVariable, name)
			return
		}
		// save variable that has value
		for _, v := range values {
			e := SaveValue(v, newOpts...)
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
