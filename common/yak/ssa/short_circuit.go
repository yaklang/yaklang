package ssa

import (
	"fmt"
	"sync/atomic"
)

var shortCircuitSeq atomic.Uint64

// BuildShortCircuit lowers boolean && / || without evaluating the right
// operand when the left operand already decides the result. The merged value
// is a Phi so SyntaxFlow can still follow both branches.
func (b *FunctionBuilder) BuildShortCircuit(left, right func() Value, isAnd bool) Value {
	if b == nil || b.CurrentBlock == nil {
		return nil
	}
	name := fmt.Sprintf("__short_circuit_%d", shortCircuitSeq.Add(1))
	b.AssignVariable(b.CreateLocalVariable(name), b.EmitConstInstNil())
	assign := func(value Value) {
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}
	leftValue := left()
	if constant, ok := ToConstInst(leftValue); ok && constant.IsBoolean() {
		if isAnd {
			if !constant.Boolean() {
				return constant
			}
			return right()
		}
		if constant.Boolean() {
			return constant
		}
		return right()
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() Value {
		if leftValue == nil {
			leftValue = b.EmitConstInst(true)
		}
		return leftValue
	}, func() {
		if isAnd {
			assign(right())
		} else {
			assign(b.EmitConstInst(true))
		}
	})
	ifBuilder.SetElse(func() {
		if isAnd {
			assign(b.EmitConstInst(false))
		} else {
			assign(right())
		}
	})
	ifBuilder.Build()
	return b.ReadValue(name)
}

// BuildLuaLogical lowers Lua's value-producing `and` / `or`. The left operand
// is always evaluated once; the right operand only runs when it can change the
// result. Unlike boolean && / ||, the result is one of the operand values.
func (b *FunctionBuilder) BuildLuaLogical(left Value, right func() Value, isAnd bool) Value {
	if b == nil || b.CurrentBlock == nil {
		return nil
	}
	name := fmt.Sprintf("__lua_logical_%d", shortCircuitSeq.Add(1))
	if left == nil {
		left = b.EmitConstInstNil()
	}
	b.AssignVariable(b.CreateLocalVariable(name), left)
	assign := func(value Value) {
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}
	if constant, ok := ToConstInst(left); ok && constant.IsBoolean() {
		if isAnd {
			if !constant.Boolean() {
				return constant
			}
			return right()
		}
		if constant.Boolean() {
			return constant
		}
		return right()
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() Value { return b.ReadValue(name) }, func() {
		if isAnd {
			assign(right())
		}
	})
	ifBuilder.SetElse(func() {
		if !isAnd {
			assign(right())
		}
	})
	ifBuilder.Build()
	return b.ReadValue(name)
}
