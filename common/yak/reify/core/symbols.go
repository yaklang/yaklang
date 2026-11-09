package core

import "encoding/binary"

// annotateSymbols names LowIR functions and call targets from the image symbol
// table. A call through a GOT slot is resolved by tracing the loaded constant
// address to its relocation symbol.
func (a *Artifact) annotateSymbols(ir *LowIR) {
	if a == nil || a.Image == nil || len(a.Image.Symbols) == 0 || ir == nil {
		return
	}
	definitions := make(map[ValueID]Instruction)
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
			if symbol, ok := a.Image.SymbolAt(function.Address); ok && symbol.Kind == SymbolFunction {
				function.Name = symbol.Name
			}
		}
		for blockIndex := range function.Blocks {
			block := &function.Blocks[blockIndex]
			for instructionIndex := range block.Instructions {
				instruction := &block.Instructions[instructionIndex]
				if instruction.Kind != OpCall || len(instruction.Operands) == 0 {
					continue
				}
				if name := a.callSymbolName(instruction.Operands[0], definitions); name != "" {
					instruction.Operands[0] = Operand{Kind: OperandSymbol, Name: name}
				}
			}
		}
	}
}

func (a *Artifact) callSymbolName(operand Operand, definitions map[ValueID]Instruction) string {
	switch operand.Kind {
	case OperandConstant:
		return a.functionSymbol(literalValue(operand.Literal))
	case OperandValue:
		id := operand.Value
		for depth := 0; depth < 8; depth++ {
			definition, ok := definitions[id]
			if !ok {
				return ""
			}
			switch definition.Kind {
			case OpCopy:
				if len(definition.Operands) == 0 {
					return ""
				}
				id = definition.Operands[0].Value
			case OpLoad:
				if len(definition.Operands) == 0 {
					return ""
				}
				address := definition.Operands[0]
				if address.Kind == OperandConstant {
					return a.functionSymbol(literalValue(address.Literal))
				}
				if address.Kind != OperandValue {
					return ""
				}
				if constant, ok := constantFromDefinitions(definitions, address.Value); ok {
					return a.functionSymbol(constant)
				}
				return ""
			default:
				return ""
			}
		}
	}
	return ""
}

func constantFromDefinitions(definitions map[ValueID]Instruction, id ValueID) (uint64, bool) {
	for depth := 0; depth < 8; depth++ {
		definition, ok := definitions[id]
		if !ok {
			return 0, false
		}
		switch definition.Kind {
		case OpConstant:
			if len(definition.Operands) == 0 {
				return 0, false
			}
			return literalValue(definition.Operands[0].Literal), true
		case OpCopy:
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

func (a *Artifact) functionSymbol(address uint64) string {
	symbol, ok := a.Image.SymbolAt(address)
	if !ok || symbol.Name == "" || symbol.Kind == SymbolObject {
		return ""
	}
	return symbol.Name
}

func literalValue(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}
