package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestIfMerge(t *testing.T) {
	code := `
		if(condA){println(a);}else if(condB){println(a);}else{a=1; println(a);} println(a);
`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)
	log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
	ssatest.CheckPrintlnValue(code, []string{"1", "Undefined-a", "Undefined-a", "phi(a)[Undefined-a,Undefined-a,1]"}, t)
}

func TestIfMergeLocal(t *testing.T) {
	code := `
		if(condA){println(a);}else if(condB){println(a);}else{let a=1; println(a);} println(a);
`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)
	log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
	ssatest.CheckPrintlnValue(code, []string{"1", "Undefined-a", "Undefined-a", "Undefined-a"}, t)
}

func TestSyntaxBlockMergeLocal(t *testing.T) {
	code := `
		{let a = 1;} println(a);
`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)
	log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
	ssatest.CheckPrintlnValue(code, []string{"Undefined-a"}, t)
}

func Disabled_TestFunctionSideEffect(t *testing.T) {
	code := `
function demo() {
  a = 1; // 没有用 var/let/const
}
function demoA(){
	b = 1;
}
demo();
println(a); // 1
println(b); // Undefined-b
		`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("js"),
	)
	require.NoError(t, err)
	log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
	ssatest.CheckPrintlnValue(code, []string{"1", "Undefined-b"}, t)
}
