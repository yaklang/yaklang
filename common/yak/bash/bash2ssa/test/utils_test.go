package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() { ssatest.SetLanguage(ssaconfig.BASH) }

// A real shell function has no argv[0] operand. This lets the CheckPrintln
// helpers observe arguments without a production-only command.
const sink = "println() { :; }; "

var bashOption = ssaapi.WithLanguage(ssaconfig.BASH)

// checkBashPrintln asserts the exact values passed to every println call.
func checkBashPrintln(t *testing.T, code string, want ...string) {
	t.Helper()
	ssatest.CheckPrintlnValue(sink+code, want, t, bashOption)
}

// checkBashPrintlnContain asserts that every want is a substring of some
// println argument.
func checkBashPrintlnContain(t *testing.T, code string, want ...string) {
	t.Helper()
	ssatest.CheckPrintlnValueContain(sink+code, want, t, bashOption)
}

// checkBashPrintlnRegexp asserts each pattern against some println argument.
func checkBashPrintlnRegexp(t *testing.T, code string, want ...string) {
	t.Helper()
	ssatest.CheckPrintlnValueRegexp(sink+code, want, t, bashOption)
}

// checkBash runs a structural assertion on the parsed program.
func checkBash(t *testing.T, code string, handler func(prog *ssaapi.Program)) {
	t.Helper()
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		handler(prog)
		return nil
	}, bashOption, ssaapi.WithMemory())
}

// requireNoCall walks the CFG from the entry block and asserts that a call is
// not reachable. SyntaxFlow still returns instructions in orphaned blocks, so
// a plain query cannot prove that code after a jump is dead.
func requireNoCall(t *testing.T, prog *ssaapi.Program, callee string) {
	t.Helper()
	fn := prog.Program.GetFunction(string(ssa.MainFunctionName), "")
	require.NotNil(t, fn)

	seen := make(map[int64]struct{})
	queue := []int64{fn.EnterBlock}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		block, ok := fn.GetBasicBlockByID(id)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok {
				continue
			}
			call, ok := ssa.ToCall(inst)
			if !ok {
				continue
			}
			method, _ := fn.GetValueById(call.Method)
			if method != nil {
				require.NotEqual(t, callee, method.GetName(), "%s must be unreachable", callee)
			}
		}
		queue = append(queue, block.Succs...)
	}
}
