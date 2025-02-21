package sfvm

import (
	"context"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"regexp"
	"strings"
)

type StringComparator struct {
	Context    context.Context
	MatchMode  StringMatchMode
	Conditions []*StringCondition
}

type StringCondition struct {
	Pattern    string
	FilterMode ConditionFilterMode
}

func NewStringComparator(mode StringMatchMode, ctx context.Context) *StringComparator {
	return &StringComparator{
		MatchMode:  mode,
		Context:    ctx,
		Conditions: make([]*StringCondition, 0),
	}
}

func (c *StringComparator) AddCondition(pattern string, filterMode ConditionFilterMode) {
	if c == nil {
		return
	}
	c.Conditions = append(c.Conditions, &StringCondition{
		Pattern:    pattern,
		FilterMode: filterMode,
	})
}

func (c *StringComparator) Matches(target string) bool {
	if c == nil {
		return false
	}
	switch c.MatchMode {
	case MatchHaveAny:
		for _, condition := range c.Conditions {
			if condition.Matches(target) {
				return true
			}
		}
		return false
	case MatchHave:
		for _, condition := range c.Conditions {
			if !condition.Matches(target) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *StringCondition) Matches(target string) bool {
	if c == nil {
		return false
	}
	var check func(string) bool
	switch c.FilterMode {
	case GlobalConditionFilter:
		matcher, err := glob.Compile(c.Pattern)
		if err != nil {
			return false
		}
		check = func(s string) bool { return matcher.Match(s) }
	case RegexpConditionFilter:
		matcher, err := regexp.Compile(c.Pattern)
		if err != nil {
			return false
		}
		check = func(s string) bool { return matcher.MatchString(s) }
	case ExactConditionFilter:
		check = func(s string) bool { return strings.Contains(s, c.Pattern) }
	default:
		return false
	}
	return check(target)
}

type OpcodeComparator struct {
	Context   context.Context
	Opcodes   []ssa.Opcode
	BinOpcode []string
}

type OpcodeCheck func(opcode ssa.Opcode) bool

type BinOpcodeCheck func(opcode string) bool

func NewOpcodeComparator(ctx context.Context) *OpcodeComparator {
	return &OpcodeComparator{
		Context: ctx,
		Opcodes: make([]ssa.Opcode, 0),
	}
}

func (c *OpcodeComparator) AddOpcode(opcode ssa.Opcode) {
	if c == nil {
		return
	}
	c.Opcodes = append(c.Opcodes, opcode)
}

func (c *OpcodeComparator) AddBinOpcode(binOp string) {
	if c == nil {
		return
	}
	c.BinOpcode = append(c.BinOpcode, binOp)
}

func (c *OpcodeComparator) AllSatisfy(opcodeCheck OpcodeCheck, binOpCheck BinOpcodeCheck) bool {
	if c == nil {
		return false
	}
	for _, opcode := range c.Opcodes {
		if opcodeCheck(opcode) {
			return true
		}
	}
	for _, binOpcode := range c.BinOpcode {
		if binOpCheck(binOpcode) {
			return true
		}
	}
	return false
}
