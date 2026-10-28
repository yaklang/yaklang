package ssaapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
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

// slowIncludeDuration is the wall-clock threshold after which a single lib
// include sub-query is logged. It diagnoses rules stuck in include without
// rule-perf profiling (e.g. 9 rules stuck 200-350s on include java-servlet-param).
const slowIncludeDuration = 30 * time.Second

// includeFlightEntry is one in-flight include computation under the flight
// key (see nativeCallInclude). done is closed once the sub-query finishes
// (successfully or not); values/err are read only after done closes.
type includeFlightEntry struct {
	done   chan struct{}
	values Values
	err    error
}

var includeFlights sync.Map // map[string]*includeFlightEntry

// includeCycleKey marks include flight keys already being loaded by the
// current (nested) sub-query chain; a repeat means a recursive include.
type includeCycleKey struct{}

func releaseIncludeFlight(key string, entry *includeFlightEntry, values Values, err error) {
	entry.values, entry.err = values, err
	close(entry.done)
	// Delete only if this exact entry is still registered: between close(done)
	// and Delete a re-entrant miss may already have installed a fresh entry.
	includeFlights.CompareAndDelete(key, entry)
}

// acquireIncludeFlight returns the in-flight entry for key. The first caller
// gets (entry, true) and must run the loader, then releaseIncludeFlight.
// Later callers get (entry, false) and must wait on entry.done — bounded by
// their own ctx so a runaway include cannot hang the waiting rules too.
func acquireIncludeFlight(key string) (*includeFlightEntry, bool) {
	entry := &includeFlightEntry{done: make(chan struct{})}
	actual, loaded := includeFlights.LoadOrStore(key, entry)
	if loaded {
		return actual.(*includeFlightEntry), false
	}
	return entry, true
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
	flightKey, ret := getIncludeCacheValueWithFlightKey(parent, cacheKey, inputs)
	if ret != nil {
		return true, ret, nil
	}

	// flightKey is the cache hash when the result is cacheable. When the
	// program hash is unavailable it degenerates to a nocache: program-name
	// key: still unique per lib/program, so two different libs or two
	// different programs never share a flight entry, but the result is not
	// cached. An empty flightKey (temp input values, or an unnamed program)
	// means neither cache nor dedup is safe: compute directly.
	//
	// The lib include may be requested by many rules of the same program
	// concurrently (e.g. 9+ rules include java-servlet-param on the same
	// 1.6M-line program). Without coordination every cache miss re-runs the
	// whole lib query in parallel — same result, N times the DB load, and the
	// analyze phase crawls. Single-flight on the flight key: the first miss
	// computes once, everyone else waits (bounded by its own rule ctx) and
	// reuses the result.
	config := frame.GetConfig()

	// Recursive includes (a lib including itself, directly or through another
	// lib) used to recurse until the stack died; on a flight entry they would
	// deadlock the loader on itself. Detect the cycle via the nested
	// sub-query ctx before anything else.
	//
	// The cycle key is the rule identity (cacheKey), NOT the flight key: a
	// self-including lib that transforms its input values produces a fresh
	// input hash each level, so keying the cycle on inputs would let the
	// recursion slip past detection. Wrapped as CriticalError so the frame
	// aborts the whole rule — an ordinary error is only a statement-level skip
	// (frame exec skips to expression end) and the rule would silently
	// "succeed" with empty results.
	ctx := config.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}
	var active map[string]struct{}
	if v, ok := ctx.Value(includeCycleKey{}).(map[string]struct{}); ok {
		active = v
		if _, cycling := active[cacheKey]; cycling {
			return false, nil, utils.Wrapf(
				sfvm.CriticalError,
				"recursive include %q detected", ruleName,
			)
		}
	}
	nextActive := map[string]struct{}{cacheKey: {}}
	for k := range active {
		nextActive[k] = struct{}{}
	}
	ctx = context.WithValue(ctx, includeCycleKey{}, nextActive)

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

	if flightKey == "" {
		gotValues, err := compute()
		if err != nil {
			return false, nil, err
		}
		return true, CreateIncludeValue(gotValues), nil
	}

	entry, isLoader := acquireIncludeFlight(flightKey)
	if !isLoader {
		select {
		case <-entry.done:
		case <-ctx.Done():
			return false, nil, utils.Errorf("include %q waiting canceled: %w", ruleName, ctx.Err())
		}
		if entry.err != nil {
			return false, nil, entry.err
		}
		if len(entry.values) == 0 {
			return false, nil, utils.Errorf("no value found")
		}
		return true, CreateIncludeValue(entry.values), nil
	}

	// compute may panic; free the waiters first, then propagate.
	var panicked any
	gotValues, computeErr := func() (v Values, e error) {
		defer func() {
			if r := recover(); r != nil {
				panicked = r
				e = utils.Errorf("include %q panicked: %v", ruleName, r)
			}
		}()
		return compute()
	}()
	if computeErr != nil {
		releaseIncludeFlight(flightKey, entry, nil, computeErr)
		if panicked != nil {
			panic(panicked)
		}
		return false, nil, computeErr
	}
	// Only the hash-derived flight key identifies a cacheable result; the
	// nocache: variant must stay out of the shared include cache.
	if !strings.HasPrefix(flightKey, "nocache:") {
		includeCache.Set(flightKey, gotValues)
	}
	releaseIncludeFlight(flightKey, entry, gotValues, nil)
	return true, CreateIncludeValue(gotValues), nil
}

// getIncludeCacheValueWithFlightKey is the cache-read half of nativeCallInclude.
// It returns (flightKey, cachedValues): a non-empty flightKey means the result
// is cacheable under that key (and identical concurrent misses single-flight
// on it), an empty one means the result must not be cached. cachedValues is
// non-nil only on a hit.
func getIncludeCacheValueWithFlightKey(program *Program, ruleName string, inputValues Values) (string, sfvm.Values) {
	hash, ok := program.Hash()
	if !ok {
		// No program hash (unnamed program): the result must not be cached, but
		// concurrent misses of the same lib still single-flight on the program
		// name if there is one.
		if name := program.GetProgramName(); name != "" {
			return "nocache:" + utils.CalcSha256(name, ruleName), nil
		}
		return "", nil
	}
	hash = utils.CalcSha256(hash + ruleName)
	if inputValues != nil && len(inputValues) > 0 {
		valueHash, ok := inputValues.Hash()
		if !ok {
			// input values contain temp values: the result is neither cacheable
			// nor safe to dedup (same as legacy behavior).
			return "", nil
		}
		hash = utils.CalcSha256(hash, valueHash)
	}
	if ret, ok := includeCache.Get(hash); ok {
		return hash, CreateIncludeValue(ret)
	}
	return hash, nil
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
