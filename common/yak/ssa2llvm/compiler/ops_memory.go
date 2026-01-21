package compiler

// TODO: Phase 2 - Memory Operations
// This file will handle Make, MemberCall, and other memory-related SSA instructions

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// compileMake handles SSA Make instruction (allocates arrays, slices, maps, etc.)
// TODO: Implement memory allocation for different types
//   - Make for slices: allocate {ptr, len, cap} on heap
//   - Make for maps: call runtime map allocation
//   - Make for channels: call runtime channel allocation
func (c *Compiler) compileMake(inst *ssa.Make) error {
	return fmt.Errorf("Make instruction not yet implemented")

	// TODO Phase 2: Implementation
	// switch inst.Type {
	// case SliceType:
	//     return c.compileMakeSlice(inst)
	// case MapType:
	//     return c.compileMakeMap(inst)
	// case ChanType:
	//     return c.compileMakeChan(inst)
	// default:
	//     return fmt.Errorf("unsupported Make type: %v", inst.Type)
	// }
}

// compileMemberCall handles member access (obj.field or obj[index])
// TODO: Implement field access and indexing
//   - Struct field access: GEP (GetElementPtr) instruction
//   - Array/slice indexing: bounds check + GEP
//   - Map indexing: runtime map lookup call
func (c *Compiler) compileMemberCall(inst *ssa.MemberCall) error {
	return fmt.Errorf("MemberCall instruction not yet implemented")

	// TODO Phase 2: Implementation
	// object := c.getValue(inst.Object)
	//
	// switch inst.Kind {
	// case FieldAccess:
	//     return c.compileFieldAccess(object, inst.MemberName)
	// case ArrayIndex:
	//     return c.compileArrayIndex(object, inst.Index)
	// case MapIndex:
	//     return c.compileMapIndex(object, inst.Key)
	// default:
	//     return fmt.Errorf("unsupported member call kind: %v", inst.Kind)
	// }
}

// compileFieldAccess generates LLVM GEP for struct field access
// TODO: Calculate field offset using type layout info
func (c *Compiler) compileFieldAccess(object, fieldName string) error {
	panic("field access not yet implemented")

	// TODO:
	// layout := types.GetLayout(object.Type)
	// offset := layout.FieldOffsets[fieldName]
	// ptr := c.Builder.CreateStructGEP(object, offset, fieldName)
	// value := c.Builder.CreateLoad(ptr, "")
	// c.Values[inst.ID] = value
}

// compileArrayIndex generates bounds-checked array indexing
// TODO: Insert bounds check, then GEP
func (c *Compiler) compileArrayIndex(object, index string) error {
	panic("array indexing not yet implemented")

	// TODO:
	// 1. Load array length
	// 2. Compare index < length
	// 3. If out of bounds, call panic handler
	// 4. Otherwise, GEP to element
	// 5. Load value
}

// compileMapIndex generates runtime map lookup
// TODO: Call runtime function for map access
func (c *Compiler) compileMapIndex(object, key string) error {
	panic("map indexing not yet implemented")

	// TODO:
	// Call runtime function: runtime_mapaccess(map, key) -> value
	// This returns pointer to value in map
}
