// Package region implements selection of SSA functions / sub-graphs that
// should be lowered into the VM Intermediate Representation (PIR).
//
// The MVP supports two selection modes:
//   - Explicit function-level selection by name.
//   - Select all user-defined functions.
package region

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Candidate represents one SSA function eligible for virtualization.
type Candidate struct {
	Func   *ssa.Function
	Name   string
	Reason string // why this function was selected
}

// Selector chooses which SSA functions should be lowered to PIR.
type Selector interface {
	Select(prog *ssa.Program) []Candidate
}

// ---------------------------------------------------------------------------
// ByName – explicit function-name selector
// ---------------------------------------------------------------------------

// ByName selects functions whose names match the provided list.
type ByName struct {
	Names []string
}

func (s *ByName) Select(prog *ssa.Program) []Candidate {
	nameSet := make(map[string]struct{}, len(s.Names))
	for _, n := range s.Names {
		nameSet[n] = struct{}{}
	}

	var result []Candidate
	prog.EachFunction(func(fn *ssa.Function) {
		name := fn.GetName()
		if _, ok := nameSet[name]; ok {
			result = append(result, Candidate{
				Func:   fn,
				Name:   name,
				Reason: "explicit",
			})
		}
	})
	return result
}

// ---------------------------------------------------------------------------
// All – selects every user-defined function
// ---------------------------------------------------------------------------

// All selects all user-defined functions for protection.
// Useful for testing and full-protection profiles.
type All struct {
	// ExcludeEntry skips the top-level entry function (yak_internal_atmain).
	ExcludeEntry bool
}

func (s *All) Select(prog *ssa.Program) []Candidate {
	var result []Candidate
	prog.EachFunction(func(fn *ssa.Function) {
		name := fn.GetName()
		if s.ExcludeEntry && isEntryName(name) {
			return
		}
		result = append(result, Candidate{
			Func:   fn,
			Name:   name,
			Reason: "all",
		})
	})
	return result
}

func isEntryName(name string) bool {
	return name == "yak_internal_atmain" || name == "main" || name == "@main"
}

// ---------------------------------------------------------------------------
// Analysis helpers
// ---------------------------------------------------------------------------

// IsLowerable checks whether a function's body can be lowered to PIR
// in the MVP scope (basic arithmetic, comparison, branches, calls).
// Functions that use unsupported features (make, panic, recover, typecasts
// on non-integer types) are rejected.
func IsLowerable(fn *ssa.Function) bool {
	if fn == nil {
		return false
	}
	blocks := fn.Blocks
	if len(blocks) == 0 {
		return false
	}
	for _, blockID := range blocks {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok || blockVal == nil {
			return false
		}
		block, ok := blockVal.(*ssa.BasicBlock)
		if !ok {
			return false
		}
		for _, instID := range block.Insts {
			instObj, ok := fn.GetInstructionById(instID)
			if !ok || instObj == nil {
				continue
			}
			if instObj.IsLazy() {
				instObj = instObj.Self()
			}
			if instObj == nil {
				continue
			}
			switch instObj.(type) {
			case *ssa.BinOp, *ssa.Call, *ssa.Return, *ssa.ConstInst,
				*ssa.SideEffect, *ssa.Phi, *ssa.Jump, *ssa.If, *ssa.Loop:
				// supported
			case *ssa.Make, *ssa.Panic, *ssa.Recover, *ssa.ParameterMember:
				return false
			}
		}
	}
	return true
}
