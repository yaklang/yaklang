package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var nativeCallEval sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	contextResult, err := frame.GetSFResult()
	if err != nil {
		return false, nil, err
	}
	program, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	exec := func(codeRaw string) (bool, sfvm.ValueOperator, error) {
		newResult, err := QuerySyntaxflow(
			QueryWithProgram(program),
			QueryWithRuleContent(codeRaw),
			QueryWithInitVar(contextResult.SymbolTable),
			QueryWithSFConfig(frame.GetConfig()),
		)
		if err != nil {
			return false, nil, err
		}
		if newResult != nil && newResult.memResult != nil {
			contextResult.MergeByResult(newResult.memResult)
		}
		return true, v, nil
	}

	fromProgram := false
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		_, fromProgram = operator.(*Program)
		return utils.Error("normal exit")
	})
	if !fromProgram {
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			if val.IsConstInst() {
				code := codec.AnyToString(val.GetConstValue())
				_, _, err := exec(code)
				if err != nil {
					log.Warnf("eval code: %v failed: %v", code, err)
				}
			}
			return nil
		})
		return true, v, nil
	}

	var codes string
	var variableName string
	codes = params.GetString(0, "code", "sf", "syntaxflow")
	if codes == "" {
		variableName = params.GetString("var", "name", "variable")
	} else if utils.MatchAnyOfRegexp(codes, `^\$[a-zA-Z_][a-zA-Z_0-9]*$`) {
		variableName = strings.Trim(codes, "$")
		codes = ""
	}
	if variableName != "" {
		newVal, ok := contextResult.SymbolTable.Get(variableName)
		if !ok {
			return false, nil, utils.Error("no code found in <eval(var: " + variableName + ")>")
		}
		firstCode := ""
		_ = newVal.Recursive(func(operator sfvm.ValueOperator) error {
			if raw, ok := operator.(*Value); ok && raw.IsConstInst() {
				firstCode = codec.AnyToString(raw.GetConstValue())
				return utils.Error("abort")
			}
			return nil
		})
		if firstCode == "" {
			return false, nil, utils.Error("no code found (no context result) in <eval(var: " + variableName + ")>")
		}
		codes = firstCode
	}

	if codes == "" {
		return false, nil, utils.Error("no code found in <eval(...)>")
	}
	return exec(codes)
}

var nativeCallFuzztag sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	codes := params.GetString(0, "fuzztag", "f", "tag")
	if codes == "" {
		return false, nil, utils.Error("no fuzztag code found in <eval(...)>")
	}

	parent, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var vals = make(map[string][]sfvm.ValueOperator)
	frame.GetSymbolTable().ForEach(func(name string, value sfvm.ValueOperator) bool {
		value.Recursive(func(operator sfvm.ValueOperator) error {
			if operator.String() == "" {
				return nil
			}
			if raw, ok := operator.(*Value); ok && raw.IsConstInst() {
				existed, ok := vals[name]
				if !ok {
					existed = make([]sfvm.ValueOperator, 0)
					vals[name] = existed
				}
				vals[name] = append(existed, operator)
			}
			return nil
		})
		isNext, results, err := nativeCallName(v, frame, params)
		if err != nil {
			return true
		}
		if isNext {
			existed, ok := vals[name]
			if !ok {
				existed = make([]sfvm.ValueOperator, 0)
				vals[name] = existed
			}
			vals[name] = append(existed, results)
		}
		return true
	})

	var opts []mutate.FuzzConfigOpt
	for rawTagName, valuesRaw := range vals {
		name := rawTagName
		values := valuesRaw
		opt := mutate.Fuzz_WithExtraFuzzTagHandler(name, func(s string) []string {
			results := []string{}
			visited := map[string]struct{}{}
			for _, valIns := range values {
				valIns.Recursive(func(operator sfvm.ValueOperator) error {
					ret := operator.String()
					if ret == "" {
						return nil
					}
					if _, ok := visited[ret]; ok {
						return nil
					}
					visited[ret] = struct{}{}
					if constIns, ok := operator.(*Value); ok && constIns.IsConstInst() {
						ret = codec.AnyToString(constIns.GetConstValue())
					}
					results = append(results, ret)
					return nil
				})
			}
			return results
		})
		opts = append(opts, opt)
	}
	var rets []sfvm.ValueOperator
	results, err := mutate.FuzzTagExec(codes, opts...)
	if err != nil {
		return false, nil, err
	}
	for _, result := range results {
		rets = append(rets, parent.NewConstValue(result))
	}
	if len(rets) == 0 {
		return false, nil, utils.Error("no fuzztag result found")
	}
	return true, sfvm.NewValues(rets), nil
}
