package ssaapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	sfdb.RegisterValid(ValidSyntaxFlowRule)
}

func ValidSyntaxFlowRule(s *schema.SyntaxFlowRule) error {
	if s != nil && s.IsStructMode() {
		// Struct verify lives in sfanalysis (would import-cycle ssaapi).
		s.Verified = true
		return nil
	}
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

var includeCache = utils.NewTTLCache[Values]()

// includeFlight single-flights identical include computations on top of
// includeCache: the first cache miss computes once, concurrent misses share
// the result, and later callers are served from the cache.
var includeFlight = utils.NewSingleFlightCache[string, Values](includeCache.CacheExWithKey)

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

// slowIncludeDuration is the wall-clock threshold after which a single lib
// include sub-query is logged. It diagnoses rules stuck in include without
// rule-perf profiling (e.g. 9 rules stuck 200-350s on include java-servlet-param).
const slowIncludeDuration = 30 * time.Second

// includeFlightValues runs compute once per flightKey across concurrent
// callers and caches the successful result; a waiter can still give up on its
// own rule ctx because the shared single-flight cache is context-aware.
func includeFlightValues(
	ctx context.Context,
	flightKey string,
	ruleName string,
	compute func() (Values, error),
) (Values, error) {
	values, err := includeFlight.DoChan(ctx, flightKey, compute)
	if err != nil && ctx.Err() != nil {
		return nil, utils.Errorf("include %q waiting canceled: %w", ruleName, ctx.Err())
	}
	return values, err
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
	bound := structBoundFromConfig(frame.GetConfig())
	if bound != nil {
		cacheKey = cacheKey + ":struct:" + bound.key
	}
	flightKey, cacheable := includeCacheKey(parent, cacheKey, inputs)

	// A non-cacheable include (temp input values, or a program without a hash)
	// is computed directly; otherwise identical concurrent misses share one
	// computation through includeFlight.
	config := frame.GetConfig()

	// Cycles are rejected when the built-in rule set is synced
	// (sfdb.CheckIncludeCycles). A cycle that reaches a scan another way
	// (user/task-local/bulk rules) waits here until the rule ctx is canceled.
	ctx := config.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}

	compute := func() (Values, error) {
		var queryValue sfvm.Values
		if len(inputs) == 0 {
			queryValue = sfvm.ValuesOf(parent)
		} else {
			queryValue = ToSFVMValues(inputs)
		}
		opts := []QueryOption{
			QueryWithSFConfig(config),
			QueryWithContext(ctx),
			QueryWithWorkBudget(config.GetWorkBudget()),
			QueryWithInitInputValues(queryValue),
			QueryWithRule(rule),
		}
		if bound != nil && rule.IsStructMode() {
			opts = append(opts, QueryWithStruct(bound.compileUnit()))
		} else {
			opts = append(opts, QueryWithProgram(parent))
		}
		start := time.Now()
		result, err := QuerySyntaxflow(opts...)
		if took := time.Since(start); took > slowIncludeDuration {
			log.Warnf("syntaxflow include %q on program %s took %s", ruleName, parent.GetProgramName(), took)
		}
		if err != nil {
			return nil, err
		}
		var gotValues Values
		for _, name := range result.GetAlertVariables() {
			vs := result.GetValues(name)
			gotValues = append(gotValues, vs...)
		}
		if len(gotValues) == 0 {
			return nil, utils.Errorf("no value found")
		}
		return gotValues, nil
	}

	if !cacheable {
		gotValues, err := compute()
		if err != nil {
			return false, nil, err
		}
		return true, CreateIncludeValue(gotValues), nil
	}

	gotValues, err := includeFlightValues(ctx, flightKey, ruleName, compute)
	if err != nil {
		return false, nil, err
	}
	return true, CreateIncludeValue(gotValues), nil
}

// includeCacheKey derives the cache / flight identity of one include. ok is
// false when the result must not be cached or deduped: input values contain
// temp values, or the program has no hash.
func includeCacheKey(program *Program, ruleName string, inputValues Values) (string, bool) {
	// Hash() is unavailable only for a program with neither a stored IR record
	// nor a name; such results are neither cached nor deduped.
	hash, ok := program.Hash()
	if !ok {
		return "", false
	}
	hash = utils.CalcSha256(hash + ruleName)
	if inputValues != nil && len(inputValues) > 0 {
		valueHash, ok := inputValues.Hash()
		if !ok {
			return "", false
		}
		hash = utils.CalcSha256(hash, valueHash)
	}
	return hash, true
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

// GetIncludeCacheValue reports the include cache identity for (program, lib
// rule) and any cached value. Kept for tests that assert cache-key derivation;
// production include lookups go through includeFlight.
func GetIncludeCacheValue(program *Program, ruleName string, inputValues Values) (hash string, value sfvm.Values, shouldCache bool) {
	hash, shouldCache = includeCacheKey(program, ruleName, inputValues)
	if !shouldCache {
		return "", nil, false
	}
	if ret, ok := includeCache.Get(hash); ok {
		value = CreateIncludeValue(ret)
	}
	return hash, value, true
}
