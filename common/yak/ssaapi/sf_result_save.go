package ssaapi

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func LoadResultByRuleContent(programName, ruleContent string, kind schema.SyntaxflowResultKind) (*SyntaxFlowResult, error) {
	result := ssadb.GetResultByRuleContent(programName, ruleContent, kind)
	if result == nil {
		return nil, utils.Error("result not found")
	}
	return loadResult(result)
}

func LoadResultByID(resultID uint, force ...bool) (*SyntaxFlowResult, error) {
	// if set force=true not use cache
	if len(force) > 0 && force[0] {
		// Skip cache when force is true
	} else {
		// check cache
		if result := CreateResultFromCache(resultSaveDatabase, uint64(resultID)); result != nil {
			return result, nil
		}
	}

	resultdb, err := ssadb.GetResultByID(resultID)
	if err != nil {
		return nil, err
	}
	result, err := loadResult(resultdb)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func loadResult(result *ssadb.AuditResult) (*SyntaxFlowResult, error) {
	res := createEmptyResult()
	res.dbResult = result
	var rule *schema.SyntaxFlowRule
	if result.RuleName != "" {
		// load rule from db
		var err error
		rule, err = sfdb.GetRulePure(result.RuleName)
		if err != nil {
			return nil, utils.Errorf("load rule %s error: %v", result.RuleName, err)
		}
	} else {
		// create rule
		rule = &schema.SyntaxFlowRule{}
		rule.RuleName = result.RuleName
		rule.Title = result.RuleTitle
		rule.TitleZh = result.RuleTitleZh
		rule.Purpose = schema.SyntaxFlowRulePurposeType(result.RulePurpose)
		rule.Severity = schema.SyntaxFlowSeverity(result.RuleSeverity)
		rule.Description = result.RuleDesc
		rule.Content = result.RuleContent
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

func YieldSyntaxFlowResult(db *gorm.DB) chan *SyntaxFlowResult {
	ch := make(chan *SyntaxFlowResult)
	go func() {
		defer close(ch)
		results := ssadb.YieldAuditResults(db, context.Background())
		for result := range results {
			res, err := loadResult(result)
			if err != nil {
				continue
			}
			ch <- res
		}
	}()
	return ch
}

func CountSyntaxFlowResult(db *gorm.DB) (int, error) {
	return ssadb.CountAuditResults(db)
}

func (r *SyntaxFlowResult) Save(kind schema.SyntaxflowResultKind, TaskIDs ...string) (uint, error) {
	return r.save(context.Background(), kind, TaskIDs...)
}

func (r *SyntaxFlowResult) SaveWithContext(ctx context.Context, kind schema.SyntaxflowResultKind, TaskIDs ...string) (uint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, kind, TaskIDs...)
}

func (r *SyntaxFlowResult) save(
	ctx context.Context,
	kind schema.SyntaxflowResultKind,
	TaskIDs ...string,
) (uint, error) {
	if r == nil || r.memResult == nil || r.program == nil {
		return 0, utils.Error("result or program  is nil")
	}
	if len(TaskIDs) > 0 {
		r.TaskID = TaskIDs[0]
	}
	// result
	result := ssadb.CreateResult(TaskIDs...)
	r.id = result.ID
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
	if err := r.saveValue(ctx, result); err != nil {
		errs = utils.JoinErrors(errs, err)
	}

	r.variable.ForEach(func(key string, value any) {
		r.riskCountMap[key] = int64(value.(int))
	})
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
func (r *SyntaxFlowResult) CreateRisk() error {
	if r == nil {
		return utils.Errorf("SyntaxFlowResult is nil")
	}

	r.GetAlertValues().ForEach(func(i string, v Values) bool {
		for index, v := range v {
			r.SaveRisk(i, index, v, false)
		}
		return true
	})

	return nil
}
func (r *SyntaxFlowResult) saveValue(ctx context.Context, result *ssadb.AuditResult) error {
	// result := r.dbResult
	if result == nil {
		return utils.Error("result is nil")
	}

	database := newAuditDatabase(ctx, ssadb.GetDB())
	defer database.Close() // wait for save finish
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
		OptionSaveValue_ProgramName(result.ProgramName),
		// ctx
		OptionSaveValue_Context(ctx),
		// database
		OptionSaveValue_Database(database),
		OptionSaveValue_IsMemoryCompile(r.IsProgMemoryCompile()),
	}
	saveVariable := func(name string, values Values) {
		opts := append(opts, OptionSaveValue_ResultVariable(name))

		// save no value variable
		if len(values) == 0 {
			result.UnValueVariable = append(result.UnValueVariable, name)
			return
		}

		switch result.Kind {
		case schema.SFResultKindDebug:
			// debug模式下，所有变量都存数据库
			if msg, ok := r.GetAlertMsg(name); ok {
				opts = append(opts, OptionSaveValue_ResultAlert(msg))
			}
		case schema.SFResultKindSearch:
			// search模式下，不保存到risk数据库
		case schema.SFResultKindScan:
			// scan模式下，只存有风险的变量
			if msg, ok := r.GetAlertMsg(name); ok {
				opts = append(opts, OptionSaveValue_ResultAlert(msg))
			} else {
				result.UnValueVariable = append(result.UnValueVariable, name)
				return
			}
		}

		// save variable that has value
		for index, v := range values {
			hash := r.SaveRisk(name, index, v, true)
			if hash != "" {
				opts = append(opts, OptionSaveValue_ResultRiskHash(hash))
			}
			opts = append(opts, OptionSaveValue_ResultIndex(uint(index)))

			v.ShowDot()

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
	if r == nil {
		return nil
	}
	var ret *ypb.SyntaxFlowResult
	if r.dbResult != nil {
		ret = r.dbResult.ToGRPCModel()
	}
	if r.memResult != nil {
		ret = r.memResult.ToGRPCModel()
	}
	if ret != nil {
		ret.ResultID = uint64(r.id)
		ret.TaskID = r.TaskID
		ret.SaveKind = string(r.GetResultSaveKind())
	}
	return ret
}
