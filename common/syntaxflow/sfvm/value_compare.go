package sfvm

import (
	"context"
	"fmt"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
)

// CompareItems is used to compare string or opcode
type CompareItems struct {
	// compare mode
	Mode *CompareItemsMode
	// compare string Mode
	StringMode CompareStringMode
	// ConditionCache is the cache for regex and glob that has been compiled
	ConditionCache map[string]any
	// Items include the string to be compared and the condition mode
	Items   []*CompareItem
	context context.Context
}

type CompareItemsMode struct {
	Mode       CompareMode
	StringMode CompareStringMode
}

type CompareItem struct {
	ToCompared    string
	ConditionMode ConditionFilterMode

	CompareItems *CompareItems
}

func NewCompareStringItems(mode CompareStringMode, context context.Context) *CompareItems {
	return &CompareItems{
		Mode: &CompareItemsMode{
			Mode:       CompareModeString,
			StringMode: mode,
		},
		ConditionCache: make(map[string]any),
		Items:          make([]*CompareItem, 0),
		context:        context,
	}
}

func NewCompareOpcodeItems(context context.Context) *CompareItems {
	return &CompareItems{
		Mode: &CompareItemsMode{
			Mode: CompareModeOpcode,
		},
		ConditionCache: make(map[string]any),
		Items:          make([]*CompareItem, 0),
		context:        context,
	}
}

func (c *CompareItems) getContext() context.Context {
	if c == nil {
		return context.Background()
	}
	return c.context
}

func (c *CompareItems) AddStringCompareItem(s string, m ConditionFilterMode) {
	if c == nil {
		return
	}
	c.Items = append(c.Items, &CompareItem{
		CompareItems:  c,
		ToCompared:    s,
		ConditionMode: m,
	})
}

func (c *CompareItems) AddOpcodeCompareItem(opcode string) {
	if c == nil {
		return
	}
	c.Items = append(c.Items, &CompareItem{
		CompareItems: c,
		ToCompared:   opcode,
	})
}

func (c *CompareItems) CompareMode() CompareMode {
	if c == nil || c.Mode == nil {
		return -1
	}
	return c.Mode.Mode
}

func (c *CompareItems) CompareStringMode() CompareStringMode {
	if c == nil || c.Mode == nil {
		return -1
	}
	return c.Mode.StringMode
}

func (c *CompareItems) CompareString(s string) bool {
	if c == nil {
		return false
	}
	switch c.CompareStringMode() {
	// CompareStringAnyMode means that if any of the items is matched, the result is true
	case CompareStringAnyMode:
		for _, item := range c.Items {
			if item.CompareString(s, c.ConditionCache) {
				return true
			}
		}
		return false
	// CompareStringAllMode means that all items must be matched to return true
	case CompareStringHaveMode:
		for _, item := range c.Items {
			if !item.CompareString(s, c.ConditionCache) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *CompareItems) CompareOpcode(opcode string, binOp string) bool {
	if c == nil || opcode == "" {
		return false
	}
	for _, item := range c.Items {
		if item.ToCompared == opcode {
			return true
		} else if rets := BinOpRegexp.FindStringSubmatch(item.ToCompared); len(rets) > 2 {
			if opcode == rets[1] {
				switch ret := binOp; ret {
				case ssa.OpAdd:
					if rets[2] == "+" {
						return true
					}
				}
			}
		}
	}
	return false
}

// MatchComparedValues is used to search the value that matches the condition from ssadb.
// It is used to be called by `Program`.
func (c *CompareItems) MatchComparedValues(value ValueOperator) ValueOperator {
	switch c.CompareMode() {
	case CompareModeString:
		return c.matchStringValue(value)
	case CompareModeOpcode:
		return c.matchOpcodeValue(value)
	}
	return nil
}

func (c *CompareItems) matchStringValue(value ValueOperator) ValueOperator {
	if c == nil {
		return nil
	}

	context := c.getContext()

	switch c.CompareStringMode() {
	case CompareStringAnyMode:
		set := NewValueSet()
		for _, item := range c.Items {
			v := item.matchStringValue(context, value)
			if v == nil {
				return nil
			}
			v.Recursive(func(vo ValueOperator) error {
				if ret, ok := vo.(ssa.GetIdIF); ok {
					id := ret.GetId()
					set.Add(id, vo)
				}
				return nil
			})
		}
		return NewValues(set.List())
	case CompareStringHaveMode:
		set := NewValueSet()
		for i, item := range c.Items {
			v := item.matchStringValue(context, value)
			if v == nil {
				return nil
			}
			otherSet := NewValueSet()
			v.Recursive(func(vo ValueOperator) error {
				if ret, ok := vo.(ssa.GetIdIF); ok {
					id := ret.GetId()
					if i == 0 {
						set.Add(id, vo)
					} else {
						otherSet.Add(id, vo)
					}
				}
				return nil
			})
			if i != 0 {
				set = set.And(otherSet)
			}
		}
		return NewValues(set.List())
	}
	return nil
}

func (c *CompareItems) matchOpcodeValue(value ValueOperator) ValueOperator {
	if c == nil {
		return nil
	}

	context := c.getContext()
	set := NewValueSet()
	for _, item := range c.Items {
		v := item.matchOpcodeValue(value, context)
		if v == nil {
			return nil
		}
		v.Recursive(func(vo ValueOperator) error {
			if ret, ok := vo.(ssa.GetIdIF); ok {
				id := ret.GetId()
				set.Add(id, vo)
			}
			return nil
		})
	}
	return NewValues(set.List())
}

func (c *CompareItem) matchStringValue(ctx context.Context, value ValueOperator) ValueOperator {
	if c == nil {
		return nil
	}
	var (
		ok  bool
		v   ValueOperator
		err error
	)
	switch c.ConditionMode {
	case GlobalConditionFilter:
		ok, v, err = value.GlobMatch(ctx, ssadb.NameMatch, c.ToCompared)
	case RegexpConditionFilter:
		ok, v, err = value.RegexpMatch(ctx, ssadb.NameMatch, c.ToCompared)
	case ExactConditionFilter:
		ok, v, err = value.RegexpMatch(ctx, ssadb.NameMatch, fmt.Sprintf(".*%s.*", c.ToCompared))
	}
	if err == nil && ok {
		return v
	}
	return nil
}

func (c *CompareItem) matchOpcodeValue(value ValueOperator, context context.Context) ValueOperator {
	if c == nil {
		return nil
	}
	ok, v, err := value.OpcodeMatch(context, c.ToCompared)
	if err != nil || !ok {
		return nil
	}
	return v
}

func (c *CompareItem) CompareString(s string, conditionCache map[string]any) bool {
	if c == nil {
		return false
	}
	condition, ok := conditionCache[codec.Md5(c.ToCompared)]
	switch c.ConditionMode {
	case GlobalConditionFilter:
		var global glob.Glob
		if ok {
			if _global, ok := condition.(glob.Glob); ok {
				global = _global
			}
		}
		if global == nil {
			compile, err := glob.Compile(c.ToCompared)
			if err != nil {
				log.Errorf("global compile fail: %s", err)
				return false
			}
			conditionCache[codec.Md5(c.ToCompared)] = compile
			global = compile
		}
		return global.Match(s)
	case RegexpConditionFilter:
		var regexpCondition *regexp.Regexp
		if ok {
			if r, ok := condition.(*regexp.Regexp); ok {
				regexpCondition = r
			}
		}
		if regexpCondition == nil {
			compile, err := regexp.Compile(c.ToCompared)
			if err != nil {
				log.Errorf("regexp compile fail: %s", err)
				return false
			}
			conditionCache[codec.Md5(c.ToCompared)] = compile
			regexpCondition = compile
		}
		return regexpCondition.MatchString(s)
	case ExactConditionFilter:
		return strings.Contains(s, c.ToCompared)
	default:
		return false
	}
}
