package asm2lowir

import (
	"encoding/binary"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// annotateSymbols names LowIR functions and call targets from the image symbol
// table. A call through a GOT slot is resolved by tracing the loaded constant
// address to its relocation symbol.
func annotateSymbols(a *core.Artifact, ir *core.LowIR) {
	if a == nil || a.Image == nil || len(a.Image.Symbols) == 0 || ir == nil {
		return
	}
	definitions := make(map[core.ValueID]core.Instruction)
	for functionIndex := range ir.Functions {
		for blockIndex := range ir.Functions[functionIndex].Blocks {
			for _, instruction := range ir.Functions[functionIndex].Blocks[blockIndex].Instructions {
				for _, result := range instruction.Results {
					definitions[result.ID] = instruction
				}
			}
		}
	}
	for functionIndex := range ir.Functions {
		function := &ir.Functions[functionIndex]
		if function.Name == "" {
			if symbol, ok := a.Image.SymbolAt(function.Address); ok && symbol.Kind == sym.SymbolFunction {
				function.Name = symbol.Name
			}
		}
		for blockIndex := range function.Blocks {
			block := &function.Blocks[blockIndex]
			for instructionIndex := range block.Instructions {
				instruction := &block.Instructions[instructionIndex]
				if instruction.Kind != core.OpCall || len(instruction.Operands) == 0 {
					continue
				}
				if name := callSymbolName(a, instruction.Operands[0], definitions); name != "" {
					instruction.Operands[0] = core.Operand{Kind: core.OperandSymbol, Name: name}
				}
			}
		}
	}
}

func callSymbolName(a *core.Artifact, operand core.Operand, definitions map[core.ValueID]core.Instruction) string {
	switch operand.Kind {
	case core.OperandConstant:
		return functionSymbol(a, literalValue(operand.Literal))
	case core.OperandValue:
		id := operand.Value
		for depth := 0; depth < 8; depth++ {
			definition, ok := definitions[id]
			if !ok {
				return ""
			}
			switch definition.Kind {
			case core.OpCopy:
				if len(definition.Operands) == 0 {
					return ""
				}
				id = definition.Operands[0].Value
			case core.OpLoad:
				if len(definition.Operands) == 0 {
					return ""
				}
				address := definition.Operands[0]
				if address.Kind == core.OperandConstant {
					return functionSymbol(a, literalValue(address.Literal))
				}
				if address.Kind != core.OperandValue {
					return ""
				}
				if constant, ok := constantFromDefinitions(definitions, address.Value); ok {
					return functionSymbol(a, constant)
				}
				return ""
			default:
				return ""
			}
		}
	}
	return ""
}

func constantFromDefinitions(definitions map[core.ValueID]core.Instruction, id core.ValueID) (uint64, bool) {
	for depth := 0; depth < 8; depth++ {
		definition, ok := definitions[id]
		if !ok {
			return 0, false
		}
		switch definition.Kind {
		case core.OpConstant:
			if len(definition.Operands) == 0 {
				return 0, false
			}
			return literalValue(definition.Operands[0].Literal), true
		case core.OpCopy:
			if len(definition.Operands) == 0 {
				return 0, false
			}
			id = definition.Operands[0].Value
		default:
			return 0, false
		}
	}
	return 0, false
}

func functionSymbol(a *core.Artifact, address uint64) string {
	symbol, ok := a.Image.SymbolAt(address)
	if !ok || symbol.Name == "" || symbol.Kind == sym.SymbolObject {
		return ""
	}
	return symbol.Name
}

func literalValue(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}
