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
			if (c.spDerived[value] || c.frameDerived[value]) && c.isFixedFrameAddress(operand.Value, 0, make(map[core.ValueID]bool)) {
				return memorySlotFrame
			}
		}
	}
	return memorySlotOther
}

func (c *converter) isConstantCoreValue(id core.ValueID, depth int, visiting map[core.ValueID]bool) bool {
	if depth > 16 || visiting[id] {
		return false
	}
	if value, ok := c.values[id]; ok && value != nil {
		if _, isConst := ssa.ToConstInst(value); isConst {
			return true
		}
	}
	def, ok := c.defs[id]
	if !ok {
		return false
	}
	visiting[id] = true
	defer delete(visiting, id)
	switch def.Kind {
	case core.OpConstant:
		return true
	case core.OpCopy:
		return len(def.Operands) > 0 && def.Operands[0].Kind == core.OperandValue &&
			c.isConstantCoreValue(def.Operands[0].Value, depth+1, visiting)
	default:
		return false
	}
}

// isFixedFrameAddress reports whether id computes a constant offset from the
// stack or frame pointer (for example rsp + 0x18). Indexed accesses such as
// rsp + rdx + 0x18 return false so array/slice stores stay observable.
func (c *converter) isFixedFrameAddress(id core.ValueID, depth int, visiting map[core.ValueID]bool) bool {
	if depth > 16 || visiting[id] {
		return false
	}
	def, ok := c.defs[id]
	if !ok {
		return false
	}
	visiting[id] = true
	defer delete(visiting, id)
	switch def.Kind {
	case core.OpReadRegister:
		if len(def.Operands) == 0 {
			return false
		}
		canonical := c.registerView(def.Operands[0].Name, def.Operands[0].Type.Bits).canonical
		if canonical == "" {
			return false
		}
		if canonical == c.registerView(c.facts.Frame.StackPointer, 0).canonical {
			return true
		}
		if fp := c.facts.Frame.FramePointer; fp != "" && canonical == c.registerView(fp, 0).canonical {
			return true
		}
		return false
	case core.OpCopy:
		return len(def.Operands) > 0 && def.Operands[0].Kind == core.OperandValue &&
			c.isFixedFrameAddress(def.Operands[0].Value, depth+1, visiting)
	case core.OpAdd:
		if len(def.Operands) < 2 || def.Operands[0].Kind != core.OperandValue || def.Operands[1].Kind != core.OperandValue {
			return false
		}
		a, b := def.Operands[0].Value, def.Operands[1].Value
		if c.isFixedFrameAddress(a, depth+1, visiting) && c.isConstantCoreValue(b, depth+1, visiting) {
			return true
		}
		return c.isFixedFrameAddress(b, depth+1, visiting) && c.isConstantCoreValue(a, depth+1, visiting)
	case core.OpSub, core.OpAnd:
		if len(def.Operands) < 2 || def.Operands[0].Kind != core.OperandValue || def.Operands[1].Kind != core.OperandValue {
			return false
		}
		return c.isFixedFrameAddress(def.Operands[0].Value, depth+1, visiting) &&
			c.isConstantCoreValue(def.Operands[1].Value, depth+1, visiting)
	default:
		return false
	}
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
