// Package ssa2source renders YakSSA as source-like text.
//
// The output is inspection-oriented pseudocode, not compilable source. It keeps
// SSA values, block labels, implicit calls and effects visible so that an
// analyst can compare the reconstructed program with the original machine code.
package ssa2source

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Renderer adapts Render to the core.SourceRenderer contract.
type Renderer struct{}

// RenderSource renders a YakSSA program.
func (Renderer) RenderSource(ctx context.Context, program *ssa.Program) (string, error) {
	return Render(ctx, program)
}

// Render converts a YakSSA program into source-like text.
func Render(ctx context.Context, program *ssa.Program) (string, error) {
	if program == nil {
		return "", fmt.Errorf("ssa2source: nil program")
	}
	var output strings.Builder
	var renderErr error
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if err := ctx.Err(); err != nil {
			renderErr = err
			return false
		}
		if function == nil || strings.HasPrefix(function.GetName(), "reify.") {
			return true
		}
		var text strings.Builder
		renderFunction(&text, function)
		output.WriteString(text.String())
		output.WriteByte('\n')
		return true
	})
	if renderErr != nil {
		return "", renderErr
	}
	return output.String(), nil
}

func renderFunction(text *strings.Builder, function *ssa.Function) {
	fmt.Fprintf(text, "function %s() {\n", function.GetName())
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		renderBlock(text, function, block)
	}
	text.WriteString("}\n")
}

func renderBlock(text *strings.Builder, function *ssa.Function, block *ssa.BasicBlock) {
	fmt.Fprintf(text, "%s:\n", block.GetName())
	for _, phiID := range block.Phis {
		instruction, ok := function.GetInstructionById(phiID)
		if !ok || instruction == nil {
			continue
		}
		phi, ok := ssa.ToPhi(instruction)
		if !ok || phi == nil {
			continue
		}
		fmt.Fprintf(text, "  %s = phi(%s);\n", assignName(phi), joinValues(phi.GetValues()))
	}
	for _, instructionID := range block.Insts {
		instruction, ok := function.GetInstructionById(instructionID)
		if !ok || instruction == nil {
			continue
		}
		renderInstruction(text, function, instruction)
	}
}

func renderInstruction(text *strings.Builder, function *ssa.Function, instruction ssa.Instruction) {
	switch value := instruction.(type) {
	case *ssa.ConstInst:
		// Constants are rendered at their use sites.
	case *ssa.BinOp:
		fmt.Fprintf(text, "  %s = %s %s %s;\n", assignName(value), valueNameByID(value, value.X), binarySymbol(value.Op), valueNameByID(value, value.Y))
	case *ssa.UnOp:
		fmt.Fprintf(text, "  %s = %s%s;\n", assignName(value), unarySymbol(value.Op), valueNameByID(value, value.X))
	case *ssa.Call:
		fmt.Fprintf(text, "  %s = %s;\n", assignName(value), callExpression(value))
	case *ssa.TypeCast:
		fmt.Fprintf(text, "  %s = cast(%s, %s);\n", assignName(value), value.GetType(), valueNameByID(value, value.Value))
	case *ssa.SideEffect:
		fmt.Fprintf(text, "  %s = side-effect(%s);\n", assignName(value), value.String())
	case *ssa.Phi:
		fmt.Fprintf(text, "  %s = phi(%s);\n", assignName(value), joinValues(value.GetValues()))
	case *ssa.Undefined:
		fmt.Fprintf(text, "  %s = undefined;\n", assignName(value))
	case *ssa.Parameter:
		fmt.Fprintf(text, "  %s = parameter;\n", assignName(value))
	case *ssa.Return:
		if values := joinValues(value.GetValues()); values != "" {
			fmt.Fprintf(text, "  return %s;\n", values)
		} else {
			text.WriteString("  return;\n")
		}
	case *ssa.Jump:
		fmt.Fprintf(text, "  goto %s;\n", blockName(function, value.To))
	case *ssa.If:
		fmt.Fprintf(text, "  if (%s) goto %s; else goto %s;\n", valueNameByID(value, value.Cond), blockName(function, value.True), blockName(function, value.False))
	default:
		fmt.Fprintf(text, "  %s = %s;\n", assignNameIfValue(instruction), instruction.String())
	}
}

func assignName(value ssa.Value) string {
	if value == nil {
		return "unknown"
	}
	if name := value.GetVerboseName(); name != "" {
		return name
	}
	if name := firstVariableName(value); name != "" {
		return name
	}
	if name := value.GetName(); name != "" {
		return name
	}
	return fmt.Sprintf("v%d", value.GetId())
}

// firstVariableName uses the register or source variable a value was assigned
// to. Versioned values share one display name, which is the intended
// variable-form output.
func firstVariableName(value ssa.Value) string {
	names := make([]string, 0, 4)
	for name := range value.GetAllVariables() {
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

func assignNameIfValue(instruction ssa.Instruction) string {
	if value, ok := ssa.ToValue(instruction); ok && value != nil {
		return assignName(value)
	}
	return "value"
}

func valueNameByID(instruction ssa.Instruction, id int64) string {
	if instruction == nil || id == 0 {
		return "nil"
	}
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return fmt.Sprintf("v%d", id)
	}
	return valueName(value)
}

func valueName(value ssa.Value) string {
	if value == nil {
		return "nil"
	}
	if constant, ok := ssa.ToConstInst(value); ok && constant != nil {
		return constText(constant)
	}
	if block, ok := ssa.ToBasicBlock(value); ok && block != nil {
		return block.GetName()
	}
	if function, ok := ssa.ToFunction(value); ok && function != nil {
		return function.GetName()
	}
	return assignName(value)
}

func constText(constant *ssa.ConstInst) string {
	if constant == nil || constant.Const == nil {
		return "nil"
	}
	raw := constant.GetRawValue()
	switch value := raw.(type) {
	case string:
		return strconv.Quote(value)
	case []byte:
		return fmt.Sprintf("%#v", value)
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%v", value)
	}
}

func callExpression(call *ssa.Call) string {
	if call == nil {
		return "call()"
	}
	target := valueNameByID(call, call.Method)
	args := call.GetValuesByIDs(call.Args)
	return fmt.Sprintf("%s(%s)", target, joinValues(args))
}

func joinValues(values ssa.Values) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, valueName(value))
	}
	return strings.Join(parts, ", ")
}

func blockName(function *ssa.Function, id int64) string {
	if function == nil || id == 0 {
		return "unknown"
	}
	block, ok := function.GetBasicBlockByID(id)
	if !ok || block == nil {
		return fmt.Sprintf("b%d", id)
	}
	return block.GetName()
}

func binarySymbol(op ssa.BinaryOpcode) string {
	switch op {
	case ssa.OpAdd:
		return "+"
	case ssa.OpSub:
		return "-"
	case ssa.OpMul:
		return "*"
	case ssa.OpDiv:
		return "/"
	case ssa.OpMod:
		return "%"
	case ssa.OpAnd:
		return "&"
	case ssa.OpOr:
		return "|"
	case ssa.OpXor:
		return "^"
	case ssa.OpShl:
		return "<<"
	case ssa.OpShr:
		return ">>"
	case ssa.OpEq:
		return "=="
	case ssa.OpNotEq:
		return "!="
	case ssa.OpLt:
		return "<"
	case ssa.OpLtEq:
		return "<="
	case ssa.OpGt:
		return ">"
	case ssa.OpGtEq:
		return ">="
	default:
		return string(op)
	}
}

func unarySymbol(op ssa.UnaryOpcode) string {
	switch op {
	case ssa.OpNeg:
		return "-"
	case ssa.OpNot:
		return "!"
	case ssa.OpBitwiseNot:
		return "~"
	default:
		return string(op) + " "
	}
}
