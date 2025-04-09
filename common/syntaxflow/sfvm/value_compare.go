package sfvm

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/yak/ssa"
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

func (c *StringComparator) Matches(targets ...string) bool {
	if c == nil {
		return false
	}
	// log.Info("StringComparator Matches", c.Conditions, targets)
	switch c.MatchMode {
	case MatchHaveAny:
		for _, condition := range c.Conditions {
			if condition.Matches(targets) {
				return true
			}
		}
		return false
	case MatchHave:
		for _, condition := range c.Conditions {
			if !condition.Matches(targets) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *StringCondition) Matches(target []string) bool {
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
	return slices.ContainsFunc(target, check)
}

type OpcodeComparator struct {
	Context        context.Context
	Opcodes        []ssa.Opcode
	BinAndUnarayOp []string
}

type CheckOpcode func(opcode ssa.Opcode) bool
type CheckBinOrUnaryOpcode func(opcode string) bool

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

func (c *OpcodeComparator) AddBinOrUnaryOpcode(op string) {
	if c == nil {
		return
	}
	c.BinAndUnarayOp = append(c.BinAndUnarayOp, op)
}

func (c *OpcodeComparator) AllSatisfy(opcodeCheck CheckOpcode, check CheckBinOrUnaryOpcode) bool {
	if c == nil {
		return false
	}
	for _, opcode := range c.Opcodes {
		if opcodeCheck(opcode) {
			return true
		}
	}
	for _, opcode := range c.BinAndUnarayOp {
		if check(opcode) {
			return true
		}
	}
	return false
}
