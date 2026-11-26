package asm2lowir

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// annotateSymbols names LowIR functions and call targets from the image symbol
// table. A call through a GOT slot is resolved by tracing the loaded constant
// address to its relocation symbol.
func annotateSymbols(a *core.Artifact, ir *core.LowIR) {
	if a == nil || a.Image == nil || ir == nil {
		return
	}
	if len(a.Image.Symbols) == 0 && len(a.Image.Relocations) == 0 {
		return
	}
	for functionIndex := range ir.Functions {
		function := &ir.Functions[functionIndex]
		// Value IDs span blocks but start over in each decoded function.
		definitions := make(map[core.ValueID]core.Instruction)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instructions {
				for _, result := range instruction.Results {
					definitions[result.ID] = instruction
				}
			}
		}
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
				if name := relocationCallName(a, instruction.Address); name != "" {
					instruction.Operands[0] = core.Operand{Kind: core.OperandSymbol, Name: name}
					continue
				}
				if name := callSymbolName(a, instruction.Operands[0], definitions); name != "" {
					instruction.Operands[0] = core.Operand{Kind: core.OperandSymbol, Name: name}
				}
			}
		}
	}
}

// relocationCallName returns the symbol a relocatable object's call relocation
// names. The relocation field lies at the instruction address on fixed-width
// targets, one byte into the instruction on x86 where it follows the opcode, and
// one instruction back on RISC-V, where a call is an auipc/jalr pair and the
// relocation sits on the auipc.
func relocationCallName(a *core.Artifact, address uint64) string {
	if a == nil || a.Image == nil {
		return ""
	}
	candidates := []uint64{address, address + 1}
	if a.Target.Architecture == core.ArchRISCV32 || a.Target.Architecture == core.ArchRISCV64 {
		candidates = append(candidates, address-4)
	}
	for _, candidate := range candidates {
		if relocation, ok := a.Image.RelocationAt(candidate); ok && relocation.Symbol != "" {
			return relocation.Symbol
		}
	}
	return ""
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
	// A PowerPC ELFv1 call reaches the function descriptor; the code address it
	// holds is what the symbol names.
	if a != nil && a.Hints != nil {
		if code, ok := a.Hints.OpdDescriptors[address]; ok {
			if name := symbolNameAt(a, code); name != "" {
				return name
			}
		}
	}
	// A local call on PowerPC ELFv2 targets the local entry point, which sits a
	// fixed distance past the symbol.
	if a != nil && a.Hints != nil && a.Hints.LocalEntryOffset > 0 && address > a.Hints.LocalEntryOffset {
		if name := symbolNameAt(a, address-a.Hints.LocalEntryOffset); name != "" {
			return name
		}
	}
	return symbolNameAt(a, address)
}

// symbolNameAt returns the function name defined exactly at one address.
func symbolNameAt(a *core.Artifact, address uint64) string {
	if a == nil || a.Image == nil {
		return ""
	}
	// Several symbols share an address: a mapping symbol marks the start of a
	// run and the function name follows. Skip the marker and keep looking, so a
	// RISC-V call to puts is named puts and not sub_0x<address>.
	for _, symbol := range a.Image.SymbolsAt(address) {
		if symbol.Name == "" || symbol.Kind == sym.SymbolObject {
			continue
		}
		// A mapping symbol marks where one encoding or data run starts: $a and $t
		// on ARM, $x and $d on RISC-V, sometimes with an ISA string appended.
		if strings.HasPrefix(symbol.Name, "$") {
			continue
		}
		return symbol.Name
	}
	return ""
}

func literalValue(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}
