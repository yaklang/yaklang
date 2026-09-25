package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestQualifiedExternalInnerConstructorKeepsArgument(t *testing.T) {
	prog, err := ssaapi.Parse(`class Example {
void print(String input) { println(ansi().new Text(input)); }
}`, ssaapi.WithLanguage("java"))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`*.Text(* as $args) as $constructors`)
	require.NoError(t, err)
	require.Len(t, result.GetValues("constructors"), 1)
	require.NotEmpty(t, result.GetValues("args"))
	require.Contains(t, result.GetValues("args").String(), "Parameter-input")
	result, err = prog.SyntaxFlowWithError(`ansi() as $calls`)
	require.NoError(t, err)
	require.Len(t, result.GetValues("calls"), 1, "qualifier must be evaluated once")
}
