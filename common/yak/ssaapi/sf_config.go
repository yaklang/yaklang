package ssaapi

import (
	"strings"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

type sfCheck struct {
	contextResult *sf.SFFrameResult
	config        *sf.Config
	vm            *sf.SyntaxFlowVirtualMachine

	matchItem []*checkItem
	untilItem []*checkItem

	// beforeMergeHook runs on a child SFFrameResult before MergeByResult (e.g. dataflow
	// only_reachable post-mode pruning include-rule captures). parent is the accumulated
	// context result before this child merge.
	beforeMergeHook func(parent *sf.SFFrameResult, child *sf.SFFrameResult)
}

type checkItem struct {
	*sf.RecursiveConfigItem
	frame *sf.SFFrame
}

// appendSubRuleFromNativeParam adds one include/exclude/hook/until sub-rule from a native-call param if non-empty.
func appendSubRuleFromNativeParam(check *sfCheck, params *sf.NativeCallActualParams, paramKey string, cfgKey sf.RecursiveConfigKey) {
	if check == nil || params == nil {
		return
	}
	if rule := params.GetString(paramKey); rule != "" {
		check.AppendItems(&sf.RecursiveConfigItem{
			Key:            string(cfgKey),
			Value:          rule,
			SyntaxFlowRule: true,
		})
	}
}

func recursiveCheckKeyLabel(key string) string {
	switch key {
	case sf.RecursiveConfig_Include, sf.RecursiveConfig_Exclude, sf.RecursiveConfig_Until, sf.RecursiveConfig_Hook:
		return string(key)
	default:
		return "unknown"
	}
}

func CreateCheck(
	contextResult *sf.SFFrameResult,
	config *sf.Config,
	configItems ...*sf.RecursiveConfigItem,
) *sfCheck {
	res := &sfCheck{
		contextResult: contextResult,
		config:        config,
		vm:            sf.NewSyntaxFlowVirtualMachine(),
		matchItem:     make([]*checkItem, 0, len(configItems)),
	}
	contextResult.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	contextResult.SymbolTable.Delete(sf.RecursiveMagicVariable)
	res.vm.SetConfig(config)
	res.AppendItems(configItems...)
	return res
}

func (c *sfCheck) Empty() bool {
	return len(c.matchItem) == 0 && len(c.untilItem) == 0
}

func (c *sfCheck) SetBeforeMergeHook(f func(parent *sf.SFFrameResult, child *sf.SFFrameResult)) {
	c.beforeMergeHook = f
}

func (c *sfCheck) AppendItems(items ...*sf.RecursiveConfigItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		frame, err := c.vm.Compile(item.Value)
		if err != nil {
			keyName := recursiveCheckKeyLabel(item.Key)
			// 暴露编译错误，添加到结果中以便前端可以获取
			errorMsg := utils.Errorf("SyntaxFlow compile error for %s rule [%s]: %v", keyName, item.Value, err).Error()
			log.Errorf(errorMsg)
			if c.contextResult != nil {
				c.contextResult.Errors = append(c.contextResult.Errors, errorMsg)
			}
			continue
		}
		checkItem := &checkItem{
			RecursiveConfigItem: item,
			frame:               frame,
		}
		switch item.Key {
		case sf.RecursiveConfig_Include, sf.RecursiveConfig_Exclude:
			c.matchItem = append(c.matchItem, checkItem)
		case sf.RecursiveConfig_Until, sf.RecursiveConfig_Hook:
			c.untilItem = append(c.untilItem, checkItem)
		}
	}
}

func CreateCheckFromNativeCallParam(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	params *sf.NativeCallActualParams,
) *sfCheck {

	check := CreateCheck(sfResult, config)
	// Order preserved: exclude / include (path match) before hook / until (walk).
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamExclude, sf.RecursiveConfig_Exclude)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamInclude, sf.RecursiveConfig_Include)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamHook, sf.RecursiveConfig_Hook)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamUntil, sf.RecursiveConfig_Until)

	return check
}

func (r *sfCheck) CheckMatch(path sf.Values) bool {
	if r.Empty() {
		return true
	}
	ret := true
	r.check(path, r.matchItem, func(key string, match bool) bool {
		switch key {
		case sf.RecursiveConfig_Include:
			if !match {
				// RecursiveConfig_Include: If match, continue; if not, stop.
				ret = false
				return false // stop
			}
		case sf.RecursiveConfig_Exclude:
			if match {
				// RecursiveConfig_Exclude: If match, stop; if not, continue.
				ret = false
				return false // stop
			}
		}
		return true // continue
	})
	return ret
}
func (r *sfCheck) CheckUntil(path sf.Values) bool {
	if r.Empty() {
		return false
	}
	until := false
	r.check(path, r.untilItem, func(key string, match bool) bool {
		switch key {
		case sf.RecursiveConfig_Until:
			if match {
				// RecursiveConfig_Until: If match, stop; if not, continue.
				until = true
				return false // stop
			}
		case sf.RecursiveConfig_Hook:
			// RecursiveConfig_Hook: Always continue.
		}
		return true // continue
	})
	return until
}
func (r *sfCheck) check(
	path sf.Values,
	items []*checkItem,
	fn func(string, bool) bool,
) {
	opt := []QueryOption{
		QueryWithVM(r.vm),
		QueryWithInitVar(r.contextResult.SymbolTable),
		QueryWithValues(path),
	}
	for _, it := range items {
		res, err := it.check(path, opt...)
		if err != nil {
			log.Errorf("check path value %v fail: %v", path.String(), err)
			continue
		}

		match := isMatch(res)
		r.clearup(res.GetSFResult())
		if !fn(it.Key, match) {
			return
		}
	}
}

func (item *checkItem) check(value sf.Values, opt ...QueryOption) (*SyntaxFlowResult, error) {
	if item.frame == nil {
		return nil, utils.Errorf("syntaxflow frame is nil")
	}

	var res *SyntaxFlowResult
	var err error
	opt = append(opt, QueryWithFrame(item.frame))
	res, err = QuerySyntaxflow(opt...)
	if err != nil {
		return nil, utils.Errorf("syntaxflow rule exec fail: %v", err)
	}
	return res, nil
}

func (r *sfCheck) clearup(sfres *sf.SFFrameResult) {
	if sfres == nil {
		return
	}
	r.sanitizeChildResult(sfres)
	r.runBeforeMergeHook(sfres)
	r.mergeChildResult(sfres)
}

func (r *sfCheck) sanitizeChildResult(sfres *sf.SFFrameResult) {
	r.config.Mutex.Lock()
	sfres.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	sfres.SymbolTable.Delete(sf.RecursiveMagicVariable)
	r.config.Mutex.Unlock()
}

func (r *sfCheck) runBeforeMergeHook(sfres *sf.SFFrameResult) {
	if r.beforeMergeHook == nil {
		return
	}
	// Run hook outside config lock:
	// hook logic may trigger CFG/reachability helpers that re-enter VM/config paths and
	// attempt to lock the same mutex again. Holding the lock here can deadlock (non-reentrant mutex).
	r.beforeMergeHook(r.contextResult, sfres)
}

func (r *sfCheck) mergeChildResult(sfres *sf.SFFrameResult) {
	r.config.Mutex.Lock()
	r.contextResult.MergeByResult(sfres)
	r.config.Mutex.Unlock()
}

func isMatch(result *SyntaxFlowResult) bool {
	if result == nil {
		return false
	}

	effectiveVarNum := 0
	matchedSingle := false
	if vars := result.GetAllVariable(); vars != nil {
		vars.ForEach(func(key string, value any) {
			// Ignore VM internal temporary symbols when deciding include/exclude/until match.
			if strings.HasPrefix(key, "__") && key != sf.RecursiveMagicVariable {
				return
			}
			effectiveVarNum++
			if num, ok := value.(int); ok && num != 0 {
				matchedSingle = true
			}
		})
	}

	if effectiveVarNum == 0 {
		// check un-name value
		if len(result.GetUnNameValues()) != 0 {
			return true
		}
	} else if effectiveVarNum == 1 {
		return matchedSingle
	} else {
		// multiple variable, check magic variable
		if len(result.GetValues(sf.RecursiveMagicVariable)) != 0 {
			return true
		}
	}
	return false
}
