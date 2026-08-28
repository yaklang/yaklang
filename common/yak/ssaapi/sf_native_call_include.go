package ssaapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

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

type taskLocalSyntaxFlowRuleLibrariesKey struct{}

type taskLocalSyntaxFlowRuleLibraries struct {
	rules map[string]*schema.SyntaxFlowRule
	scope string
}

// WithTaskLocalSyntaxFlowRuleLibraries binds immutable library rules to one
// scan context. Native <include(...)> calls in that context never fall back to
// the shared profile database, including when the requested library is absent.
func WithTaskLocalSyntaxFlowRuleLibraries(
	ctx context.Context,
	rules map[string]*schema.SyntaxFlowRule,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cloned := make(map[string]*schema.SyntaxFlowRule, len(rules))
	for name, rule := range rules {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" && rule != nil {
			cloned[trimmed] = rule
		}
	}
	names := make([]string, 0, len(cloned))
	for name := range cloned {
		names = append(names, name)
	}
	sort.Strings(names)
	hasher := sha256.New()
	for _, name := range names {
		_, _ = hasher.Write([]byte(name))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(taskLocalSyntaxFlowRuleCacheHash(cloned[name])))
		_, _ = hasher.Write([]byte{0})
	}
	return context.WithValue(ctx, taskLocalSyntaxFlowRuleLibrariesKey{}, taskLocalSyntaxFlowRuleLibraries{
		rules: cloned,
		scope: hex.EncodeToString(hasher.Sum(nil)),
	})
}

func resolveSyntaxFlowIncludeRule(
	ctx context.Context,
	ruleName string,
) (*schema.SyntaxFlowRule, bool, error) {
	if ctx != nil {
		if binding, ok := ctx.Value(taskLocalSyntaxFlowRuleLibrariesKey{}).(taskLocalSyntaxFlowRuleLibraries); ok {
			rule, exists := binding.rules[strings.TrimSpace(ruleName)]
			if !exists || rule == nil {
				return nil, true, fmt.Errorf(
					"task-local syntaxflow library %q is not present in the prepared rule snapshot",
					ruleName,
				)
			}
			return rule, true, nil
		}
	}
	rule, err := sfdb.GetLibrary(ruleName)
	return rule, false, err
}

func taskLocalSyntaxFlowRuleScope(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	binding, _ := ctx.Value(taskLocalSyntaxFlowRuleLibrariesKey{}).(taskLocalSyntaxFlowRuleLibraries)
	return binding.scope
}

func taskLocalSyntaxFlowRuleCacheHash(rule *schema.SyntaxFlowRule) string {
	if rule == nil {
		return ""
	}
	// SyntaxFlowRule.CalcHash mutates rule.Hash. Task-local rules preserve the
	// published content_hash as execution metadata, so cache identity must be
	// calculated without overwriting that value.
	return utils.CalcSha256(rule.RuleId, rule.RuleName, rule.Content, rule.Tag)
}

func createIncludeCache() *utils.Cache[Values] {
	return utils.NewTTLCache[Values]()
}

func nativeCallInclude(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (success bool, value sfvm.Values, err error) {
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

	rule, taskLocal, err := resolveSyntaxFlowIncludeRule(frame.GetConfig().GetContext(), ruleName)
	if err != nil {
		log.Warnf("get syntaxflow rule library %v error: %v", ruleName, err)
		return false, nil, err
	}
	cacheKey := ruleName
	if taskLocal {
		cacheKey = taskLocalSyntaxFlowRuleScope(frame.GetConfig().GetContext()) + ":" + ruleName + ":" + taskLocalSyntaxFlowRuleCacheHash(rule)
	}
	hash, ret, shouldCache := GetIncludeCacheValue(parent, cacheKey, inputs)
	if ret != nil {
		return true, ret, nil
	}
	var queryValue sfvm.Values
	if len(inputs) == 0 {
		queryValue = sfvm.ValuesOf(parent)
	} else {
		queryValue = ToSFVMValues(inputs)
	}

	config := frame.GetConfig()
	// Run the included sub-rule under the PARENT rule's ctx + total-work budget.
	// QueryWithSFConfig (WithConfig) already copies ctx + workBudget, but pass
	// them explicitly too so the sub-rule's dataflow descent (AnalyzeContext.check
	// -> ctx.Done()/EnterWork) and per-element native loops (<typeName> etc.)
	// honor the parent's --rule-timeout / --rule-work-limit and BAIL instead of
	// hanging the whole rule (a heavy <include> lib rule like
	// java-write-filename-sink runs <typeName> over tens of thousands of calls;
	// without the parent deadline a single <include> could run 30min+ past the
	// rule budget and exhaust memory building the edge graph).
	result, err := QuerySyntaxflow(
		QueryWithSFConfig(config),
		QueryWithContext(config.GetContext()),
		QueryWithWorkBudget(config.GetWorkBudget()),
		QueryWithProgram(parent),
		QueryWithInitInputValues(queryValue),
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

func CreateIncludeValue(vs Values) sfvm.Values {
	var list []sfvm.ValueOperator
	for _, got := range vs {
		val := got.NewValue(got.getValue())
		val.AppendPredecessor(got, sfvm.WithAnalysisContext_Label("include"))
		list = append(list, val)
	}
	return sfvm.NewValues(list)
}

func GetIncludeCacheValue(program *Program, ruleName string, inputValues Values) (hash string, value sfvm.Values, shouldCache bool) {
	getRetFromCache := func(hash string) sfvm.Values {
		if ret, ok := includeCache.Get(hash); ok {
			return CreateIncludeValue(ret)
		}
		return nil
	}

	if programHash, ok := program.Hash(); ok {
		// Use program hash and rule name to generate a unique hash
		hash = utils.CalcSha256(programHash + ruleName)
		shouldCache = true
		if inputValues != nil && len(inputValues) > 0 {
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
