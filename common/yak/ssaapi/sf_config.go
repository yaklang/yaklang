package ssaapi

import (
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

type sfCheck struct {
	contextResult *sf.SFFrameResult
	config        *sf.Config
	vm            *sf.SyntaxFlowVirtualMachine

	matchItem []*checkItem
	untilItem []*checkItem
}

type checkItem struct {
	*sf.RecursiveConfigItem
	frame *sf.SFFrame
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

func (c *sfCheck) AppendItems(items ...*sf.RecursiveConfigItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		frame, err := c.vm.Compile(item.Value)
		if err == nil {
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
}

func CreateCheckFromNativeCallParam(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	params *sf.NativeCallActualParams,
) *sfCheck {

	check := CreateCheck(sfResult, config)
	if rule := params.GetString("exclude"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Exclude, Value: rule, SyntaxFlowRule: true}
		check.AppendItems(configItem)
	}
	if rule := params.GetString("include"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Include, Value: rule, SyntaxFlowRule: true}
		check.AppendItems(configItem)
	}

	if rule := params.GetString("hook"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Hook, Value: rule, SyntaxFlowRule: true}
		check.AppendItems(configItem)
	}
	if rule := params.GetString("until"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Until, Value: rule, SyntaxFlowRule: true}
		check.AppendItems(configItem)
	}

	return check
}

func (r *sfCheck) CheckMatch(path sf.ValueOperator) bool {
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
func (r *sfCheck) CheckUntil(path sf.ValueOperator) bool {
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
	path sf.ValueOperator,
	item []*checkItem,
	fn func(string, bool) bool,
) {
	opt := []QueryOption{
		QueryWithVM(r.vm),
		QueryWithInitVar(r.contextResult.SymbolTable),
		QueryWithValue(path),
	}
	for _, item := range item {
		res, err := item.check(path, opt...)
		if err != nil {
			log.Errorf("check path value %v fail: %v", path.String(), err)
			continue
		}

		match := isMatch(res)
		r.clearup(res.GetSFResult())
		if !fn(item.Key, match) {
			return
		}
	}
}

func (item *checkItem) check(value sf.ValueOperator, opt ...QueryOption) (*SyntaxFlowResult, error) {
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
	return (res), nil
}

func (r *sfCheck) clearup(sfres *sf.SFFrameResult) {
	if sfres == nil {
		return
	}
	r.config.Mutex.Lock()
	sfres.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	sfres.SymbolTable.Delete(sf.RecursiveMagicVariable)
	r.contextResult.MergeByResult(sfres)
	r.config.Mutex.Unlock()
}

func isMatch(result *SyntaxFlowResult) bool {
	if result.GetVariableNum() == 0 {
		// check un-name value
		if len(result.GetUnNameValues()) != 0 {
			return true
		}
	} else if result.GetVariableNum() == 1 {
		match := false
		// if only one variable, check its value
		if ret := result.GetAllVariable(); ret.Len() == 1 {
			ret.ForEach(func(key string, value any) {
				num := value.(int)
				if num != 0 {
					match = true
				}
			})
		}
		return match
	} else {
		// multiple variable, check magic variable
		if len(result.GetValues(sf.RecursiveMagicVariable)) != 0 {
			return true
		}
	}
	return false
}
