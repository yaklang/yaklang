package sfvm

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"

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

func (c *StringComparator) String() string {
	if c == nil {
		return ""
	}
	var s strings.Builder
	for i, condition := range c.Conditions {
		s.WriteString(condition.Pattern)
		s.WriteString(" ")
		if i > 0 {
			switch c.MatchMode {
			case MatchHave:
				s.WriteString("AND")
			case MatchHaveAny:
				s.WriteString("OR")
			}
			s.WriteString(" ")
		}
	}
	return s.String()
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

type BinaryCondition string

const (
	BinaryConditionEqual    BinaryCondition = "==" // OpEq
	BinaryConditionNotEqual BinaryCondition = "!=" // OpNotEq
	BinaryConditionGt       BinaryCondition = ">"  // OpGt
	BinaryConditionGtEq     BinaryCondition = ">=" // OpGtEq
	BinaryConditionLt       BinaryCondition = "<"  // OpLt
	BinaryConditionLtEq     BinaryCondition = "<=" // OpLtEq
)

type ConstComparator struct {
	ToCompared      string
	BinaryCondition BinaryCondition
}

func NewConstComparator(toComparedConst string, condition BinaryCondition) *ConstComparator {
	return &ConstComparator{
		ToCompared:      toComparedConst,
		BinaryCondition: condition,
	}
}

func (c *ConstComparator) Matches(target string) bool {
	if c == nil {
		return false
	}

	// First try to compare as numbers
	toComparedNum, toComparedErr := strconv.ParseFloat(c.ToCompared, 64)
	targetNum, targetErr := strconv.ParseFloat(target, 64)

	// If both can be parsed as numbers, compare them numerically
	if toComparedErr == nil && targetErr == nil {
		switch c.BinaryCondition {
		case BinaryConditionEqual:
			return targetNum == toComparedNum
		case BinaryConditionNotEqual:
			return targetNum != toComparedNum
		case BinaryConditionGt:
			return targetNum > toComparedNum
		case BinaryConditionGtEq:
			return targetNum >= toComparedNum
		case BinaryConditionLt:
			return targetNum < toComparedNum
		case BinaryConditionLtEq:
			return targetNum <= toComparedNum
		}
	}

	// Try to compare as booleans
	parseBool := func(str string) (bool, error) {
		switch str {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return false, utils.Error("parse bool failed")
	}

	toComparedBool, toComparedErr := parseBool(yakunquote.TryUnquote(c.ToCompared))
	targetBool, targetErr := parseBool(target)
	// If both can be parsed as booleans, compare them
	if toComparedErr == nil && targetErr == nil {
		switch c.BinaryCondition {
		case BinaryConditionEqual:
			return targetBool == toComparedBool
		case BinaryConditionNotEqual:
			return targetBool != toComparedBool
		}
	}

	// Default: compare as strings
	// String()方法会修正string类型的const，因此这里要进行修正
	// Reference:common/yak/ssa/disasmLine.go:160

	switch c.BinaryCondition {
	case BinaryConditionEqual:
		return target == c.ToCompared
	case BinaryConditionNotEqual:
		return target != c.ToCompared
	case BinaryConditionGt:
		return target > c.ToCompared // String comparison
	case BinaryConditionGtEq:
		return target >= c.ToCompared
	case BinaryConditionLt:
		return target < c.ToCompared
	case BinaryConditionLtEq:
		return target <= c.ToCompared
	}

	return false
}
