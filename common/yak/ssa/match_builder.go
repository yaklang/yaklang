package ssa

import (
	"fmt"
	"sync/atomic"
)

// MatchCase is one ordered arm of a match/when expression. Condition is
// evaluated in the current scope; only when it is true is Body evaluated and
// assigned as the candidate result.
type MatchCase struct {
	Condition func() Value
	Body      func() Value
}

var matchValueSeq atomic.Uint64

// BuildMatchValue lowers an ordered multi-way match into a chain of if/else
// blocks and merges the arm results through a variable, so the final read is a
// Phi. Cases are evaluated in source order and later conditions are not
// evaluated after the first match. A nil Body leaves the arm result nil.
func (b *FunctionBuilder) BuildMatchValue(cases []MatchCase, otherwise func() Value) Value {
	if b == nil || b.CurrentBlock == nil {
		return nil
	}
	name := fmt.Sprintf("__match_value_%d", matchValueSeq.Add(1))
	b.AssignVariable(b.CreateLocalVariable(name), b.EmitConstInstNil())

	assign := func(body func() Value) {
		value := Value(nil)
		if body != nil {
			value = body()
		}
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}

	var emit func(index int)
	emit = func(index int) {
		if b.CurrentBlock == nil || b.CurrentBlock.finish {
			return
		}
		if index >= len(cases) {
			if otherwise != nil {
				assign(otherwise)
			}
			return
		}
		item := cases[index]
		if item.Condition != nil {
			if constant, ok := ToConstInst(item.Condition()); ok && constant.IsBoolean() {
				if constant.Boolean() {
					assign(item.Body)
				} else {
					emit(index + 1)
				}
				return
			}
		}
		ifBuilder := b.CreateIfBuilder()
		ifBuilder.SetCondition(item.Condition, func() {
			assign(item.Body)
		})
		if index+1 < len(cases) || otherwise != nil {
			ifBuilder.SetElse(func() {
				emit(index + 1)
			})
		}
		ifBuilder.Build()
	}

	emit(0)
	return b.ReadValue(name)
}
