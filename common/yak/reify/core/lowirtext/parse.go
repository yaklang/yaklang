package lowirtext

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

var (
	targetLine      = regexp.MustCompile(`^target: ([^/]+)/([^ ]+) \((\d+)-bit, ([a-z]+)-endian\)$`)
	functionLine    = regexp.MustCompile(`^function (\S+) @ (0x[0-9a-fA-F]+)$`)
	blockLine       = regexp.MustCompile(`^block b(\d+) @ (0x[0-9a-fA-F]+)$`)
	instructionLine = regexp.MustCompile(`^  (0x[0-9a-fA-F]+)  (.*)$`)
)

// Parse builds LowIR from the text produced by Format. It is used by tests and
// tools that need a compact way to describe machine-level programs.
func Parse(text string) (*core.LowIR, error) {
	ir := &core.LowIR{}
	var function *core.Function
	var block *core.Block
	for lineNumber, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if match := targetLine.FindStringSubmatch(line); match != nil {
			bits, _ := strconv.Atoi(match[3])
			ir.Target = core.Target{
				OS:           core.OperatingSystem(match[1]),
				Architecture: core.Architecture(match[2]),
				Bits:         bits,
				ByteOrder:    core.ByteOrder(match[4]),
			}
			continue
		}
		if match := functionLine.FindStringSubmatch(line); match != nil {
			address, _ := strconv.ParseUint(match[2], 0, 64)
			ir.Functions = append(ir.Functions, core.Function{
				ID:      core.FunctionID(len(ir.Functions) + 1),
				Name:    match[1],
				Address: address,
			})
			function = &ir.Functions[len(ir.Functions)-1]
			block = nil
			continue
		}
		if match := blockLine.FindStringSubmatch(line); match != nil {
			if function == nil {
				return nil, fmt.Errorf("lowirtext: line %d: block before function", lineNumber+1)
			}
			id, _ := strconv.Atoi(match[1])
			address, _ := strconv.ParseUint(match[2], 0, 64)
			function.Blocks = append(function.Blocks, core.Block{ID: core.BlockID(id), Address: address})
			block = &function.Blocks[len(function.Blocks)-1]
			continue
		}
		if match := instructionLine.FindStringSubmatch(line); match != nil {
			if block == nil {
				return nil, fmt.Errorf("lowirtext: line %d: instruction before block", lineNumber+1)
			}
			instruction, err := parseInstruction(match[1], match[2])
			if err != nil {
				return nil, fmt.Errorf("lowirtext: line %d: %w", lineNumber+1, err)
			}
			block.Instructions = append(block.Instructions, instruction)
			continue
		}
		return nil, fmt.Errorf("lowirtext: line %d: unsupported %q", lineNumber+1, line)
	}
	if ir.Target.ByteOrder == "" {
		ir.Target.ByteOrder = core.LittleEndian
	}
	return ir, nil
}

func parseInstruction(addressText, body string) (core.Instruction, error) {
	address, _ := strconv.ParseUint(addressText, 0, 64)
	instruction := core.Instruction{Address: address}
	if index := strings.LastIndex(body, " ["); index >= 0 && strings.HasSuffix(body, "]") {
		effects, err := parseEffects(body[index+2 : len(body)-1])
		if err != nil {
			return core.Instruction{}, err
		}
		instruction.Effects = effects
		body = body[:index]
	}
	if index := strings.Index(body, " -> "); index >= 0 {
		for _, target := range strings.Split(body[index+4:], ", ") {
			if !strings.HasPrefix(target, "b") {
				return core.Instruction{}, fmt.Errorf("bad target %q", target)
			}
			id, err := strconv.Atoi(strings.TrimPrefix(target, "b"))
			if err != nil {
				return core.Instruction{}, fmt.Errorf("bad target %q", target)
			}
			instruction.Targets = append(instruction.Targets, core.BlockID(id))
		}
		body = body[:index]
	}
	if index := strings.Index(body, " = "); index >= 0 {
		results, err := parseResults(body[:index])
		if err != nil {
			return core.Instruction{}, err
		}
		instruction.Results = results
		body = body[index+3:]
	}
	kind, operands, _ := strings.Cut(body, " ")
	instruction.Kind = core.Opcode(kind)
	if operands == "" {
		return instruction, nil
	}
	for _, part := range strings.Split(operands, ", ") {
		operand, err := parseOperand(part)
		if err != nil {
			return core.Instruction{}, err
		}
		instruction.Operands = append(instruction.Operands, operand)
	}
	return instruction, nil
}

func parseResults(text string) ([]core.Value, error) {
	var results []core.Value
	for _, part := range strings.Split(text, ", ") {
		name, typ, ok := strings.Cut(part, ":")
		if !ok || !strings.HasPrefix(name, "v") {
			return nil, fmt.Errorf("bad result %q", part)
		}
		id, err := strconv.ParseUint(strings.TrimPrefix(name, "v"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad result %q", part)
		}
		results = append(results, core.Value{ID: core.ValueID(id), Type: parseValueType(typ)})
	}
	return results, nil
}

func parseOperand(text string) (core.Operand, error) {
	switch {
	case strings.HasPrefix(text, "v"):
		id, err := strconv.ParseUint(strings.TrimPrefix(text, "v"), 10, 64)
		if err != nil {
			return core.Operand{}, fmt.Errorf("bad value %q", text)
		}
		return core.Operand{Kind: core.OperandValue, Value: core.ValueID(id)}, nil
	case strings.HasPrefix(text, "%"):
		name := strings.TrimPrefix(text, "%")
		return core.Operand{
			Kind: core.OperandRegister,
			Type: core.ValueType{Kind: core.TypeInteger, Bits: registerTypeBits(name)},
			Name: name,
		}, nil
	case strings.HasPrefix(text, "@"):
		return core.Operand{Kind: core.OperandSymbol, Name: strings.TrimPrefix(text, "@")}, nil
	case strings.HasPrefix(text, "0x"):
		value, err := strconv.ParseUint(strings.TrimPrefix(text, "0x"), 16, 64)
		if err != nil {
			return core.Operand{}, fmt.Errorf("bad constant %q", text)
		}
		literal := make([]byte, 8)
		binary.LittleEndian.PutUint64(literal, value)
		return core.Operand{Kind: core.OperandConstant, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}, Literal: literal}, nil
	default:
		return core.Operand{}, fmt.Errorf("bad operand %q", text)
	}
}

func parseValueType(text string) core.ValueType {
	switch {
	case strings.HasPrefix(text, "i"):
		bits, _ := strconv.Atoi(strings.TrimPrefix(text, "i"))
		return core.ValueType{Kind: core.TypeInteger, Bits: uint16(bits)}
	case strings.HasPrefix(text, "ptr"):
		bits, _ := strconv.Atoi(strings.TrimPrefix(text, "ptr"))
		return core.ValueType{Kind: core.TypePointer, Bits: uint16(bits)}
	case strings.HasPrefix(text, "f"):
		bits, _ := strconv.Atoi(strings.TrimPrefix(text, "f"))
		return core.ValueType{Kind: core.TypeFloat, Bits: uint16(bits)}
	default:
		return core.ValueType{Kind: core.TypeOpaque}
	}
}

func parseEffects(text string) (core.Effect, error) {
	names := map[string]core.Effect{
		"read_memory":    core.EffectReadMemory,
		"write_memory":   core.EffectWriteMemory,
		"read_register":  core.EffectReadRegister,
		"write_register": core.EffectWriteRegister,
		"call":           core.EffectCall,
		"may_trap":       core.EffectMayTrap,
		"volatile":       core.EffectVolatile,
		"atomic":         core.EffectAtomic,
		"unknown":        core.EffectUnknown,
		"frame_restore":  core.EffectFrameRestore,
	}
	var result core.Effect
	for _, part := range strings.Split(text, ", ") {
		if flag, ok := names[part]; ok {
			result |= flag
			continue
		}
		if strings.HasPrefix(part, "0x") {
			value, err := strconv.ParseUint(part[2:], 16, 32)
			if err == nil {
				result |= core.Effect(value)
				continue
			}
		}
		return 0, fmt.Errorf("bad effect %q", part)
	}
	return result, nil
}

func registerTypeBits(name string) uint16 {
	switch name {
	case "ah", "bh", "ch", "dh", "al", "bl", "cl", "dl":
		return 8
	case "ax", "bx", "cx", "dx", "si", "di", "sp", "bp":
		return 16
	case "eax", "ebx", "ecx", "edx", "esi", "edi", "esp", "ebp":
		return 32
	default:
		return 64
	}
}
