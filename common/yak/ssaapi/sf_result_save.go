package ssaapi

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func CreateResultByID(resultID string) (*SyntaxFlowResult, error) {
	res := createEmptyResult()
	result, err := ssadb.GetResultByID(resultID)
	if err != nil {
		return nil, err
	}
	res.dbResult = result
	var rule *schema.SyntaxFlowRule
	if result.RuleName != "" {
		// load rule from db
		rule, err = sfdb.GetRule(result.RuleName)
		if err != nil {
			return nil, err
		}
	} else {
		// create rule
		rule = &schema.SyntaxFlowRule{
			Title:       result.RuleTitle,
			Severity:    schema.SyntaxFlowSeverity(result.RuleSeverity),
			Type:        schema.SyntaxFlowRuleType(result.RuleType),
			Description: result.RuleDesc,
			AlertDesc:   result.AlertDesc,
		}
	}
	res.rule = rule
	return res, nil
}

func (r *SyntaxFlowResult) Save(ResultID, TaskID string) error {
	if r == nil || r.memResult == nil {
		return utils.Error("result is nil")
	}
	// result
	db := ssadb.GetDB()
	result := &ssadb.AuditResult{
		TaskID:   TaskID,
		ResultID: ResultID,
		CheckMsg: r.GetCheckMsg(),
		Errors:   r.GetErrors(),
	}
	rule := r.memResult.GetRule()
	if rule.ID > 0 {
		// can get from database
		result.RuleName = rule.RuleName
	} else {
		// save info in result
		result.RuleTitle = rule.Title
		result.RuleSeverity = string(rule.Severity)
		result.RuleType = string(rule.Type)
		result.RuleDesc = rule.Description
		result.AlertDesc = rule.AlertDesc
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
	}
	saveVariable := func(name string, values Values) {
		opts := append(opts, OptionSaveValue_ResultVariable(name))
		if msg, ok := r.GetAlertInfo(name); ok {
			opts = append(opts, OptionSaveValue_ResultAlert(msg))
		}
		// save un value variable
		if len(values) == 0 {
			result.UnValueVariable = append(result.UnValueVariable, name)
			return
		}
		// save variable that has value
		for _, v := range values {
			opts := append(opts, OptionSaveValue_ProgramName(v.GetProgramName()))
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
