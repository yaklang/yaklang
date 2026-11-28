package ssa2source

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// AnnotateAddresses adds string and symbol comments to address constants. It
// is part of the SSA rendering stage, not the command entry point.
func AnnotateAddresses(program *ssa.Program, stringAt func(uint64) (string, bool), symbolAt func(uint64) (string, bool)) {
	if program == nil {
		return
	}
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		annotateFunctionAddresses(function, stringAt, symbolAt)
		return true
	})
}

func annotateFunctionAddresses(function *ssa.Function, stringAt func(uint64) (string, bool), symbolAt func(uint64) (string, bool)) {
	if function == nil || function.IsExtern() {
		return
	}
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, instructionID := range block.Insts {
			annotateInstructionAddresses(function, instructionID, stringAt, symbolAt)
		}
		for _, instructionID := range block.Phis {
			annotateInstructionAddresses(function, instructionID, stringAt, symbolAt)
		}
	}
}

func annotateInstructionAddresses(function *ssa.Function, instructionID int64, stringAt func(uint64) (string, bool), symbolAt func(uint64) (string, bool)) {
	instruction, ok := function.GetInstructionById(instructionID)
	if !ok || instruction == nil {
		return
	}
	node, ok := ssa.ToNode(instruction)
	if !ok || node == nil {
		return
	}
	for _, value := range node.GetValues() {
		constant, ok := ssa.ToConstInst(value)
		if !ok || constant == nil || constant.GetVerboseName() != "" {
			continue
		}
		address, ok := addressConstant(constant)
		if !ok {
			continue
		}
		if text, ok := stringAt(address); ok {
			constant.SetVerboseName(fmt.Sprintf("/* %q */", text))
			continue
		}
		if name, ok := symbolAt(address); ok && name != "" {
			constant.SetVerboseName("/* " + name + " */")
		}
	}
}

// AnnotateFunctions attaches string and symbol comments to only the provided functions.
func AnnotateFunctions(functions []*ssa.Function, artifact *core.Artifact) {
	if len(functions) == 0 || artifact == nil {
		return
	}
	stringAt := func(address uint64) (string, bool) { return artifact.StringAt(address, 128) }
	symbolAt := func(address uint64) (string, bool) {
		if artifact.Image == nil {
			return "", false
		}
		symbol, ok := artifact.Image.SymbolAt(address)
		if !ok {
			return "", false
		}
		return symbol.Name, symbol.Name != ""
	}
	for _, fn := range functions {
		annotateFunctionAddresses(fn, stringAt, symbolAt)
	}
}

func addressConstant(constant *ssa.ConstInst) (uint64, bool) {
	switch value := constant.GetRawValue().(type) {
	case int64:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case uint64:
		return value, true
	case int:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case uint32:
		return uint64(value), true
	default:
		return 0, false
	}
}

// AnnotateArtifact attaches string and symbol comments using an Artifact.
func AnnotateArtifact(program *ssa.Program, artifact *core.Artifact) {
	if program == nil || artifact == nil {
		return
	}
	stringAt := func(address uint64) (string, bool) { return artifact.StringAt(address, 128) }
	symbolAt := func(address uint64) (string, bool) {
		if artifact.Image == nil {
			return "", false
		}
		symbol, ok := artifact.Image.SymbolAt(address)
		if !ok {
			return "", false
		}
		return symbol.Name, symbol.Name != ""
	}
	AnnotateAddresses(program, stringAt, symbolAt)
}
