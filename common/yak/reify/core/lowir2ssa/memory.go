package lowir2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// memorySlotKind classifies an address expression for alias decisions. Two
// frame slots with different keys cannot overlap, and neither can two constant
// addresses. Every other pair is treated as possibly aliasing, because the
// conversion performs no points-to analysis.
type memorySlotKind int

const (
	memorySlotOther memorySlotKind = iota
	memorySlotFrame
	memorySlotConstant
)

func (c *converter) classifyMemorySlot(key string, operand core.Operand) memorySlotKind {
	if strings.HasPrefix(key, "const:") {
		return memorySlotConstant
	}
	if operand.Kind == core.OperandValue {
		if value, ok := c.values[operand.Value]; ok && value != nil {
			if c.spDerived[value] || c.frameDerived[value] {
				return memorySlotFrame
			}
		}
	}
	return memorySlotOther
}

func memorySlotsMayAlias(key string, kind memorySlotKind, other string, otherKind memorySlotKind) bool {
	if key == other {
		return true
	}
	switch {
	case kind == memorySlotFrame && otherKind == memorySlotFrame:
		return false
	case kind == memorySlotConstant && otherKind == memorySlotConstant:
		return false
	case kind == memorySlotFrame && otherKind == memorySlotConstant:
		return false
	case kind == memorySlotConstant && otherKind == memorySlotFrame:
		return false
	}
	return true
}

// recordMemoryStore marks the written slot as current and invalidates every slot
// a later read can no longer trust. Without the invalidation a second store
// through a different pointer would leave the first pointer's cached value in
// place, and a reload through it would fold to a stale constant.
func (c *converter) recordMemoryStore(key string, operand core.Operand) {
	kind := c.classifyMemorySlot(key, operand)
	for other, otherKind := range c.memoryKind {
		if other == key {
			continue
		}
		if memorySlotsMayAlias(key, kind, other, otherKind) {
			c.memoryValid[other] = false
		}
	}
	c.memoryKind[key] = kind
	c.memoryValid[key] = true
}

func (c *converter) memoryValidAt(key string) bool {
	valid, ok := c.memoryValid[key]
	return ok && valid
}

// emitMemoryRead records a read whose slot holds no known value. The address is
// the operand of the instruction, so the use chain carries the dependency: type
// inference marks the address as a pointer and the source pass renders the
// dereference from it.
func (c *converter) emitMemoryRead(instruction core.Instruction, address core.Operand, op ssa.UnaryOpcode) ssa.Value {
	addressValue := c.values[address.Value]
	if addressValue == nil || addressValue.IsUndefined() {
		if len(instruction.Results) == 0 {
			return nil
		}
		return c.builder.EmitUndefined("memory")
	}
	value := c.builder.EmitUnOp(op, addressValue)
	if value == nil {
		return nil
	}
	if len(instruction.Results) > 0 {
		value.SetType(c.typeFor(instruction.Results[0].Type))
	}
	return value
}

// emitMemoryWrite records a write outside the current frame. A write to the
// current frame is an internal spill: the memory variable carries it to later
// reads and nothing outside the function observes it. Both the address and the
// stored value are operands, so the instruction stream keeps the write together
// with everything it depends on.
func (c *converter) emitMemoryWrite(instruction core.Instruction, address core.Operand, value ssa.Value) {
	if value == nil || value.IsUndefined() || len(instruction.Operands) < 2 {
		return
	}
	addressValue := c.operandValue(address)
	if addressValue == nil || addressValue.IsUndefined() {
		return
	}
	store := c.builder.EmitBinOp(ssa.OpStore, addressValue, value)
	if store == nil {
		return
	}
	typ := c.typeFor(instruction.Operands[1].Type)
	if typ == nil || typ.String() == "any" {
		typ = value.GetType()
	}
	store.SetType(typ)
}
