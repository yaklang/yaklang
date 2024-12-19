package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	sfdb.RegisterValid(ValidSyntaxFlowRule)
}

func ValidSyntaxFlowRule(s *schema.SyntaxFlowRule) error {
	fs, err := sfdb.BuildFileSystem(s)
	if err != nil {
		return err
	}
	prog, err := ParseProjectWithFS(fs)
	if err != nil {
		return err
	}
	result, err := prog.SyntaxFlowWithError(s.Content)
	if err != nil {
		return err
	}
	if len(result.GetErrors()) > 0 {
		return utils.Errorf(`runtime error: %v`, result.GetErrors())
	}
	s.Verified = true
	return nil
}

func GetSFIncludeCache() *utils.Cache[sfvm.ValueOperator] {
	return includeCache
}

var includeCache = createIncludeCache()

func createIncludeCache() *utils.Cache[sfvm.ValueOperator] {
	return utils.NewTTLCache[sfvm.ValueOperator]()
}

func nativeCallInclude(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (success bool, value sfvm.ValueOperator, err error) {
	var parent *Program
	v.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *Value:
			parent = ret.ParentProgram
			return utils.Error("abort")
		case *Program:
			parent = ret
		}
		return nil
	})
	if parent == nil {
		return false, nil, utils.Error("no parent program found")
	}
	var ruleName string
	if ret := params.GetString("name", "rule", "rulename"); ret != "" {
		ruleName = ret
	} else if ret := params.GetString("0"); ret != "" {
		ruleName = ret
	}

	if ruleName == "" {
		return false, nil, utils.Error("no rule name found")
	}

	if programName := parent.GetProgramName(); includeCache != nil && programName != "" {
		hash := utils.CalcSha256(ruleName, programName)
		if ret, ok := includeCache.Get(hash); ok {
			return true, ret, nil
		}
		defer func() {
			if !success || value == nil || err != nil {
				return
			}
			includeCache.Set(hash, value)
		}()
	}

	rule, err := sfdb.GetLibrary(ruleName)
	if err != nil {
		log.Warnf("get syntaxflow rule library %v error: %v", ruleName, err)
		return false, nil, err
	}

	config := frame.GetConfig()
	result, err := QuerySyntaxflow(
		QueryWithProgram(parent),
		QueryWithRule(rule),
		QueryWithSFConfig(config),
	)
	if err != nil {
		return false, nil, err
	}
	var vals Values
	for _, name := range result.GetAlertVariables() {
		vs := result.GetValues(name)
		vals = append(vals, vs...)
	}
	if len(vals) > 0 {
		return true, ValuesToSFValueList(vals), nil
	}
	return false, nil, utils.Error("no value found")
}
