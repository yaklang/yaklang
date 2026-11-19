package test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestBashPrintlnExpression(t *testing.T) {
	ssatest.CheckPrintlnValueContain(sink+`s=$(greet hello); println "$s"`, []string{`greet`, `"hello"`}, t)
	checkBash(t, sink+`x=4; y=$((x+2)); println "$y"`, func(p *ssaapi.Program) {
		fn := p.Program.GetFunction(string(ssa.MainFunctionName), "")
		seen := map[ssa.TypeKind]bool{}
		for _, id := range fn.Blocks {
			b, _ := fn.GetBasicBlockByID(id)
			for _, id := range b.Insts {
				if i, ok := fn.GetInstructionById(id); ok {
					if cast, ok := ssa.ToTypeCast(i); ok {
						seen[cast.GetType().GetTypeKind()] = true
					}
				}
			}
		}
		require.True(t, seen[ssa.NumberTypeKind], "arithmetic operand needs a numeric TypeCast")
		require.True(t, seen[ssa.StringTypeKind], "arithmetic expansion needs a string TypeCast")
	})
	ssatest.CheckPrintlnValueRegexp(sink+`((i=4)); old=$((i++)); println "$old"; println "$((i))"`, []string{`4`, `5`}, t)
}

func TestBashPrintlnCommandSubstitution(t *testing.T) {
	ssatest.CheckPrintlnValue(sink+`out=$(echo hi); println "$out"`, []string{`"hi"`}, t)
	ssatest.CheckPrintlnValue(sink+`out=$(f(){ echo inner; }; f); println "$out"`, []string{`"inner"`}, t)
	ssatest.CheckPrintlnValueContain(sink+`out=$(printf %s value); println "$out"`, []string{`printf`, `"value"`}, t)
}

func TestBashPrintlnPipelineScope(t *testing.T) {
	// Bash runs every pipeline component in a subshell unless lastpipe is set.
	ssatest.CheckPrintlnValue(sink+`x=before; println hi | { x=pipe; }; println "$x"`, []string{`"hi"`, `"before"`}, t)
}
