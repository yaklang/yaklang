package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/ssa"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

var DefaultInputVar = "input"

type ResultSaveKind string

const (
	resultSaveNone     ResultSaveKind = "none"     // no save
	resultSaveMemory   ResultSaveKind = "memory"   // in cache
	resultSaveDatabase ResultSaveKind = "database" // in database
)

type queryConfig struct {
	// check if exist in database just use this
	useCache bool
	// input
	program *Program
	value   sfvm.ValueOperator // use this
	//  rule
	ruleContent string
	ruleName    string
	rule        *schema.SyntaxFlowRule // use this

	// runtime
	vm    *sfvm.SyntaxFlowVirtualMachine
	frame *sfvm.SFFrame

	// runtime config
	opts []sfvm.Option // config
	// config       *sfvm.Config
	// parentResult *sfvm.SFFrameResult

	// save
	save   ResultSaveKind
	kind   schema.SyntaxflowResultKind
	taskID string

	// control
	ctx context.Context

	// process
	processCallback func(float64, string)
}

func (config *queryConfig) GetFrame() (*sfvm.SFFrame, error) {
	if frame := config.frame; frame != nil {
		return frame, nil
	}
	// get vm
	vm := config.vm
	if vm == nil {
		vm = sfvm.NewSyntaxFlowVirtualMachine()
	}

	// use rule compiled
	if config.rule != nil {
		frame, resave, err := vm.Load(config.rule)
		if err != nil {
			return nil, utils.Errorf("SyntaxflowQuery: load rule %s error: %v", config.rule.RuleName, err)
		}
		if resave {
			// save rule to db
			sfdb.MigrateSyntaxFlow("", config.rule)
		}
		return frame, nil
	}

	// use rule content
	if config.ruleContent != "" {
		// compile rule
		frame, err := vm.Compile(config.ruleContent)
		if err != nil {
			return nil, utils.Errorf("SyntaxflowQuery: compile rule error: %v", err)
		}
		return frame, nil
	}

	// use rule name
	if config.ruleName != "" {
		rule, err := sfdb.GetRule(config.ruleName)
		if err != nil {
			return nil, utils.Errorf("SyntaxflowQuery: load rule %s from db error: %v", config.ruleName, err)
		}
		frame, resave, err := vm.Load(rule)
		if resave {
			// save rule to db
			sfdb.MigrateSyntaxFlow("", config.rule)
		}
		if err != nil {
			return nil, utils.Errorf("SyntaxflowQuery: load rule %s to sfvm error: %v", config.ruleName, err)
		}
		return frame, nil
	}

	// no rule
	return nil, utils.Errorf("SyntaxflowQuery: rule is nil")
}

func QuerySyntaxflow(opt ...QueryOption) (*SyntaxFlowResult, error) {
	config := &queryConfig{}
	for _, o := range opt {
		o(config)
	}
	process := func(f float64, msg string) {
		if config.processCallback != nil {
			config.processCallback(f, msg)
		}
	}
	process(0, "start query syntaxflow")
	// handler input  value
	value := config.value
	if utils.IsNil(value) {
		return nil, utils.Errorf("SyntaxflowQuery: value is nil")
	}

	// set cache and have programName and kind
	if config.useCache && (config.program != nil && config.kind != "") {
		ruleContent := ""
		if config.ruleContent != "" {
			ruleContent = config.ruleContent
		} else if config.rule != nil {
			ruleContent = config.rule.Content
		} else if config.ruleName != "" {
			rule, err := sfdb.GetRule(config.ruleName)
			if err != nil {
				return nil, utils.Errorf("SyntaxflowQuery: load rule %s from db error: %v", config.ruleName, err)
			}
			config.rule = rule
			ruleContent = rule.Content
		} else {
			return nil, utils.Errorf("SyntaxflowQuery: rule is nil")
		}
		if ret, err := LoadResultByRuleContent(config.program.GetProgramName(), ruleContent, config.kind); err == nil && ret != nil {
			process(1, "get result from database cache")
			return ret, nil
		}
	}

	process(0, "load or compile syntaxflow rule ")
	// get runtime frame
	frame, err := config.GetFrame()
	if err != nil {
		return nil, err
	}

	total := len(frame.Codes) + 1
	handler := 0
	config.opts = append(config.opts, sfvm.WithProcessCallback(func(i int, s string) {
		if handler < i {
			handler = i
		}
		process(float64(handler)/float64(total), s)
	}))

	// runtime
	var res *sfvm.SFFrameResult
	res, err = frame.Feed(value, config.opts...)
	if err != nil {
		return nil, utils.Wrap(err, "SyntaxflowQuery: query rule failed")
	}

	var ret *SyntaxFlowResult
	ret = CreateResultFromQuery(res)

	defer process(1, "end query syntaxflow")
	if config.program != nil {
		ret.program = config.program

		switch config.save {
		case resultSaveDatabase:
			process(float64(total-1)/float64(total), "save result")
			resultID, err := ret.SaveWithContext(config.ctx, config.kind, config.taskID)
			_ = resultID
			if err != nil {
				return ret, utils.Wrap(err, "SyntaxflowQuery: save to DB failed")
			}
			setResultToCache(config.save, ret)
		case resultSaveMemory:
			// save to memory
			id := getResultCacheId()
			ret.SetResultID(id)
			ret.CreateRisk()
			ret.TaskID = config.taskID
			setResultToCache(config.save, ret)
		}
	}

	return ret, nil
}

type QueryOption func(*queryConfig)

func QueryWithProgram(program *Program) QueryOption {
	return func(c *queryConfig) {
		c.program = program
		c.value = program
	}
}

func QueryWithPrograms(programs Programs) QueryOption {
	return func(c *queryConfig) {
		c.value = sfvm.NewValues(lo.Map(programs, func(p *Program, _ int) sfvm.ValueOperator {
			return p
		}))
		c.program, _ = lo.Find(programs, func(item *Program) bool {
			return item.GetProgramKind() == ssa.Application
		})
	}
}

func QueryWithValue(value sfvm.ValueOperator) QueryOption {
	return func(c *queryConfig) {
		c.value = value
		c.program, _ = fetchProgram(value)
	}
}

func QueryWithRule(rule *schema.SyntaxFlowRule) QueryOption {
	return func(c *queryConfig) {
		c.rule = rule
	}
}

func QueryWithRuleContent(rule string) QueryOption {
	return func(c *queryConfig) {
		c.ruleContent = rule
	}
}

func QueryWithVM(vm *sfvm.SyntaxFlowVirtualMachine) QueryOption {
	return func(c *queryConfig) {
		c.vm = vm
	}
}

func QueryWithFrame(f *sfvm.SFFrame) QueryOption {
	return func(c *queryConfig) {
		c.frame = f
	}
}

func QueryWithSave(kind schema.SyntaxflowResultKind) QueryOption {
	return func(c *queryConfig) {
		c.save = resultSaveDatabase
		c.kind = kind
	}
}

func QueryWithMemory() QueryOption {
	return func(c *queryConfig) {
		c.save = resultSaveMemory
	}
}

func QueryWithTaskID(taskID string) QueryOption {
	return func(c *queryConfig) {
		c.taskID = taskID
	}
}

func QueryWithRuleName(names string) QueryOption {
	return func(c *queryConfig) {
		c.ruleName = names
	}
}

func QueryWithSFConfig(config *sfvm.Config) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithConfig(config))
	}
}
func QueryWithInitVar(result *omap.OrderedMap[string, sfvm.ValueOperator]) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithInitialContextVars(result))
	}
}

func QueryWithInitInputVar(value sfvm.ValueOperator) QueryOption {
	return func(c *queryConfig) {
		result := omap.NewOrderedMap(map[string]sfvm.ValueOperator{})
		result.Set(DefaultInputVar, value)
		c.opts = append(c.opts, sfvm.WithInitialContextVars(result))
	}
}

func QueryWithContext(ctx context.Context) QueryOption {
	return func(c *queryConfig) {
		c.ctx = ctx
		c.opts = append(c.opts, sfvm.WithContext(ctx))
	}
}

func QueryWithProcessCallback(cb func(float64, string)) QueryOption {
	return func(c *queryConfig) {
		c.processCallback = cb
	}
}

func QueryWithFailFast(b ...bool) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithFailFast(b...))
	}
}

func QueryWithUseCache(b ...bool) QueryOption {
	return func(c *queryConfig) {
		if len(b) > 0 {
			c.useCache = b[0]
		} else {
			c.useCache = true
		}
	}
}

func QueryWithEnableDebug(b ...bool) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithEnableDebug(b...))
	}
}

func QueryWithStrictMatch(b ...bool) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithStrictMatch(b...))
	}
}

func QueryWithResultCaptured(capture sfvm.ResultCapturedCallback) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithResultCaptured(capture))
	}
}
func QueryWithSyntaxFlowResult(expected string, handler func(*Value) error) QueryOption {
	return func(c *queryConfig) {
		c.opts = append(c.opts, sfvm.WithResultCaptured(func(name string, results sfvm.ValueOperator) error {
			if name != expected {
				return nil
			}
			return results.Recursive(func(operator sfvm.ValueOperator) error {
				result, ok := operator.(*Value)
				if !ok {
					return nil
				}
				err := handler(result)
				if err != nil {
					return err
				}
				return nil
			})
		}))
	}
}

func (p *Program) SyntaxFlowChain(i string, opts ...QueryOption) Values {
	res := p.SyntaxFlow(i, opts...)
	return res.GetAllValuesChain()

}
func (p *Program) SyntaxFlow(rule string, opts ...QueryOption) *SyntaxFlowResult {
	res, err := p.SyntaxFlowWithError(rule, opts...)
	if err != nil {
		log.Errorf("SyntaxFlow: %v", err)
		return nil
	}
	return res
}
func (p *Program) SyntaxFlowWithError(rule string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithProgram(p), QueryWithRuleContent(rule))
	return QuerySyntaxflow(opts...)
}

func (ps Programs) SyntaxFlowWithError(i string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithPrograms(ps), QueryWithRuleContent(i))
	return QuerySyntaxflow(opts...)
}

// func SyntaxFlowWithVMContext(p sfvm.ValueOperator, sfCode string, sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config) (*SyntaxFlowResult, error) {
// 	opts := []QueryOption{
// 		QueryWithValue(p),
// 		QueryWithRuleContent(sfCode),
// 		QueryWithSFConfig(sfConfig),
// 		QueryWithInitVar(sfResult.SymbolTable),
// 	}
// 	return QuerySyntaxflow(opts...)
// }
// func SyntaxFlowWithVMContext(p sfvm.ValueOperator, sfCode string, sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config) (*SyntaxFlowResult, error) {
// 	opts := []QueryOption{
// 		QueryWithValue(p),
// 		QueryWithRuleContent(sfCode),
// 		QueryWithSFConfig(sfConfig),
// 		QueryWithInitVar(sfResult.SymbolTable),
// 	}
// 	return QuerySyntaxflow(opts...)
// }

func (p *Program) SyntaxFlowRuleName(ruleName string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithProgram(p), QueryWithRuleName(ruleName))
	return QuerySyntaxflow(opts...)
}

func (ps Programs) SyntaxFlowRuleName(ruleName string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithPrograms(ps), QueryWithRuleName(ruleName))
	return QuerySyntaxflow(opts...)
}

func (p *Program) SyntaxFlowRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithProgram(p), QueryWithRule(rule))
	return QuerySyntaxflow(opts...)
}

func (ps Programs) SyntaxFlowRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	opts = append(opts, QueryWithPrograms(ps), QueryWithRule(rule))
	return QuerySyntaxflow(opts...)
}
