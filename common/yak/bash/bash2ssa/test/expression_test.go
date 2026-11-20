package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestBashPrintlnExpression(t *testing.T) {
	checkBashPrintlnRegexp(t, `
x=4
y=$((x+2))
println "$y"
`, `^castType\(string, add\(castType\(number, "4"\), 2\)\)$`)

	checkBashPrintlnRegexp(t, `
((i=4))
old=$((i++))
println "$old"
println "$((i))"
`, `^castType\(string, 4\)$`, `^castType\(string, 5\)$`)
}

// Arithmetic operands are converted explicitly: numbers for the operation and
// a string for the expansion result. The conversion kind is part of the ABI
// shared with other frontends, so assert it instead of only the value.
func TestBashArithmeticCasts(t *testing.T) {
	checkBash(t, `
x=4
y=$((x+2))
println "$y"
`, func(prog *ssaapi.Program) {
		fn := prog.Program.GetFunction(string(ssa.MainFunctionName), "")
		require.NotNil(t, fn)
		seen := map[ssa.TypeKind]bool{}
		for _, id := range fn.Blocks {
			block, _ := fn.GetBasicBlockByID(id)
			for _, instID := range block.Insts {
				inst, ok := fn.GetInstructionById(instID)
				if !ok {
					continue
				}
				if cast, ok := ssa.ToTypeCast(inst); ok {
					seen[cast.GetType().GetTypeKind()] = true
				}
			}
		}
		require.True(t, seen[ssa.NumberTypeKind], "arithmetic operand needs a numeric TypeCast")
		require.True(t, seen[ssa.StringTypeKind], "arithmetic expansion needs a string TypeCast")
	})
}

func TestBashPrintlnCommandSubstitution(t *testing.T) {
	checkBashPrintln(t, `
out=$(echo hi)
println "$out"
`, `"hi"`)

	checkBashPrintln(t, `
out=$(f(){ echo inner; }; f)
println "$out"
`, `"inner"`)

	checkBashPrintlnContain(t, `
out=$(printf %s value)
println "$out"
`, "printf", `"value"`)
}

func TestBashPrintlnPipelineScope(t *testing.T) {
	// Bash runs every pipeline component in a subshell unless lastpipe is set.
	checkBashPrintln(t, `
x=before
println hi | { x=pipe; }
println "$x"
`, `"hi"`, `"before"`)
}
