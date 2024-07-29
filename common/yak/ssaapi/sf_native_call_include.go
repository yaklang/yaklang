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
	prog, err := ParseProject(fs)
	if err != nil {
		return err
	}
	result, err := prog.SyntaxFlowWithError(s.Content)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return utils.Errorf(`runtime error: %v`, result.Errors)
	}
	s.Verified = true
	return nil
}

func nativeCallInclude(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
	rule, err := sfdb.GetLibrary(ruleName)
	if err != nil {
		log.Warnf("get syntaxflow rule library %v error: %v", ruleName, err)
		return false, nil, err
	}

	result, err := SyntaxFlowWithOldEnv(parent, rule.Content, sfvm.NewSFResult(rule.Content), frame.GetVM().GetConfig())
	if err != nil {
		return false, nil, err
	}
	var vals []sfvm.ValueOperator
	for _, val := range result.AlertSymbolTable {
		val.Recursive(func(operator sfvm.ValueOperator) error {
			vals = append(vals, operator)
			return nil
		})
	}
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
}
