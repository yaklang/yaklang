package ssaapi

import (
	"context"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var ResultCache *utils.CacheWithKey[uint, *SyntaxFlowResult] = utils.NewTTLCacheWithKey[uint, *SyntaxFlowResult](time.Minute * 10)

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
		if result, ok := ResultCache.Get(resultID); ok {
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
	// set cache
	ResultCache.Set(resultID, result)
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

func (r *SyntaxFlowResult) save(ctx context.Context, kind schema.SyntaxflowResultKind, TaskIDs ...string) (uint, error) {
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
	if err := r.saveValue(ctx, result); err != nil {
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

func (r *SyntaxFlowResult) saveValue(ctx context.Context, result *ssadb.AuditResult) error {
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
		OptionSaveValue_ProgramName(result.ProgramName),
		// ctx
		OptionSaveValue_Context(ctx),
	}
	saveVariable := func(name string, values Values) {
		opts := append(opts, OptionSaveValue_ResultVariable(name))

		// save un value variable
		if len(values) == 0 {
			result.UnValueVariable = append(result.UnValueVariable, name)
			return
		}
		//非search才存入到risk数据库中
		if msg, ok := r.GetAlertMsg(name); ok && result.Kind != schema.SFResultKindSearch {
			opts = append(opts, OptionSaveValue_ResultAlert(msg))
		}
		// save variable that has value
		for index, v := range values {
			hash := r.SaveRisk(name, index, v, result)
			if hash != "" {
				opts = append(opts, OptionSaveValue_ResultRiskHash(hash))
			}
			opts = append(opts, OptionSaveValue_ResultIndex(uint(index)))
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
