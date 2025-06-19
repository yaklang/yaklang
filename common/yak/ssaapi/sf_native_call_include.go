package ssaapi

import (
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

func GetSFIncludeCache() *utils.Cache[Values] {
	return includeCache
}

var includeCache = createIncludeCache()

func createIncludeCache() *utils.Cache[Values] {
	return utils.NewTTLCache[Values]()
}

func nativeCallInclude(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (success bool, value sfvm.ValueOperator, err error) {
	parent, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var inputs Values
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if ok {
			inputs = append(inputs, val)
		}
		return nil
	})

	var ruleName string
	if ret := params.GetString("name", "rule", "rulename"); ret != "" {
		ruleName = ret
	} else if ret := params.GetString("0"); ret != "" {
		ruleName = ret
	}
	if ruleName == "" {
		return false, nil, utils.Error("no rule name found")
	}

	hash, ret, shouldCache := GetIncludeCacheValue(parent, ruleName, inputs)
	if ret != nil {
		return true, ret, nil
	}

	rule, err := sfdb.GetLibrary(ruleName)
	if err != nil {
		log.Warnf("get syntaxflow rule library %v error: %v", ruleName, err)
		return false, nil, err
	}

	var queryValue sfvm.ValueOperator
	queryValue = inputs
	if inputs.IsEmpty() {
		queryValue = parent
	}

	config := frame.GetConfig()
	result, err := QuerySyntaxflow(
		QueryWithSFConfig(config),
		QueryWithProgram(parent),
		QueryWithInitInputVar(queryValue),
		QueryWithRule(rule),
	)
	if err != nil {
		return false, nil, err
	}
	var gotValues Values
	for _, name := range result.GetAlertVariables() {
		vs := result.GetValues(name)
		gotValues = append(gotValues, vs...)
	}
	if len(gotValues) == 0 {
		return false, nil, utils.Errorf("no value found")
	}
	if shouldCache {
		includeCache.Set(hash, gotValues)
	}
	val := CreateIncludeValue(gotValues)
	return true, val, nil
}

func CreateIncludeValue(vs Values) sfvm.ValueOperator {
	// value := ValuesToSFValueList(vals)
	var list []sfvm.ValueOperator
	for _, got := range vs {
		val := got.NewValue(got.innerValue)
		val.AppendPredecessor(got, sfvm.WithAnalysisContext_Label("include"))
		list = append(list, val)
	}
	return sfvm.NewValues(list)
}

func GetIncludeCacheValue(program *Program, ruleName string, inputValues Values) (hash string, value sfvm.ValueOperator, shouldCache bool) {
	getRetFromCache := func(hash string) sfvm.ValueOperator {
		if ret, ok := includeCache.Get(hash); ok {
			return CreateIncludeValue(ret)
		} else {
			return nil
		}
	}

	if programHash, ok := program.Hash(); ok {
		// Use program hash and rule name to generate a unique hash
		hash = utils.CalcSha256(programHash + ruleName)
		shouldCache = true
		if inputValues != nil && !inputValues.IsEmpty() {
			if valueHash, ok := inputValues.Hash(); ok {
				hash = utils.CalcSha256(hash, valueHash)
			} else {
				// if input param values not empty but have temp value,
				// then the result should not be cached
				shouldCache = false
			}
		}
		if !shouldCache {
			return
		}
		value = getRetFromCache(hash)
		return
	}
	return
}
