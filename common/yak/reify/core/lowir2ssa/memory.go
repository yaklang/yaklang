package lowir2ssa

import (
	"fmt"
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

// emitStoreMarker records an externally visible store as a rendered statement.
// A store to the current frame is an internal spill: the memory variable carries
// it to later loads and nothing outside the function can observe it. A store
// through a pointer or to a global address is an effect of the function, so the
// pseudo source prints it instead of dropping the unused memory variable.
func (c *converter) emitStoreMarker(instruction core.Instruction, address core.Operand, value ssa.Value) {
	if value == nil || len(instruction.Operands) < 2 {
		return
	}
	addressValue := c.operandValue(address)
	if addressValue == nil || addressValue.IsUndefined() {
		return
	}
	typ := c.typeFor(instruction.Operands[1].Type)
	if typ == nil || typ.String() == "any" {
		typ = value.GetType()
	}
	markerText := fmt.Sprintf("store:%d:%d", addressValue.GetId(), value.GetId())
	marker := c.newConstant(markerText, ssa.CreateStringType())
	if marker == nil {
		return
	}
	cast := c.builder.EmitTypeCast(marker, typ)
	if cast == nil {
		return
	}
	addressValue.AddUser(cast)
	value.AddUser(cast)
}
