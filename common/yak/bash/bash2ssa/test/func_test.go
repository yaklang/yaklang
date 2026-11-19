package test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestBashPrintlnFunction(t *testing.T) {
	ssatest.CheckPrintlnValueRegexp(sink+`f(){ println "$1"; println "${10}"; println "$#"; println "$*"; return 7; }; f a b c d e f g h i j; println "$?"`, []string{`ParameterMember-.*0`, `ParameterMember-.*9`, `ParameterMember-.*#`, `ParameterMember-.*\*`, `7`}, t)
}

func functionCalls(t *testing.T, caller, target *ssa.Function) []*ssa.Call {
	t.Helper()
	var calls []*ssa.Call
	for _, id := range caller.Blocks {
		b, ok := caller.GetBasicBlockByID(id)
		require.True(t, ok)
		for _, id := range b.Insts {
			v, ok := caller.GetInstructionById(id)
			require.True(t, ok)
			if c, ok := ssa.ToCall(v); ok && c.Method == target.GetId() {
				calls = append(calls, c)
			}
		}
	}
	return calls
}

func TestBashFunctionCallLayout(t *testing.T) {
	checkBash(t, `f(){ observe "$1" "$2" "${10}" "$#" "$*"; }; f; f "a b" ""; f a b c d e f g h i j k; set -- 'x y' z; f "$@"; recur(){ recur "$1"; }; recur value`, func(p *ssaapi.Program) {
		main := p.Program.GetFunction(string(ssa.MainFunctionName), "")
		f, _ := ssa.ToFunction(p.Program.GetExportValue("f"))
		require.NotNil(t, f)
		require.Len(t, f.Params, 1)
		require.True(t, f.Type.IsVariadic)
		calls := functionCalls(t, main, f)
		require.Len(t, calls, 4)
		for i, want := range [][]string{{}, {"a b", ""}, {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}, {"x y", "z"}} {
			call := calls[i]
			require.Len(t, call.Args, len(want))
			for j, id := range call.Args {
				v, _ := call.GetValueById(id)
				require.Equal(t, want[j], v.String())
			}
			require.Len(t, call.ArgMember, 5)
			for j, which := range []int{0, 1, 9} {
				expected := ""
				if which < len(want) {
					expected = want[which]
				}
				v, _ := call.GetValueById(call.ArgMember[j])
				require.Equal(t, expected, v.String())
			}
			v, _ := call.GetValueById(call.ArgMember[3])
			c, ok := ssa.ToConstInst(v)
			require.True(t, ok)
			require.EqualValues(t, len(want), c.Number())
		}
		recur, _ := ssa.ToFunction(p.Program.GetExportValue("recur"))
		require.Len(t, functionCalls(t, recur, recur), 1)
	})
}
