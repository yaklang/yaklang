package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_ModernSyntax_TopLevelAndGlobalUsing(t *testing.T) {
	prog := parseCSharpSemantics(t, `
global using static System.Console;

var value = source();
WriteLine(value);
sink(value);
`)

	flow, err := prog.SyntaxFlowWithError(`source() as $source; WriteLine(* #-> as $written); sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, flow.GetValues("source"))
	require.NotEmpty(t, flow.GetValues("written"), "global using static must be visible to top-level statements")
	require.NotEmpty(t, flow.GetValues("origin"), "top-level statement data flow must reach sink")
}

func TestCSharp_ModernSyntax_FileScopedNamespaceAndInit(t *testing.T) {
	prog := parseCSharpSemantics(t, `
namespace Demo.Models;

public class Payload {
    public string Value { get; init; }
}

public class Program {
    public static void Main(string[] args) {
        var payload = new Payload { Value = source() };
        sink(payload.Value);
    }
}
`)

	require.NotEmpty(t, prog.Ref("Payload"), "file-scoped namespace must register contained types")
	flow, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, flow.GetValues("source"))
	require.NotEmpty(t, flow.GetValues("origin"), "init-only property assignment must preserve data flow")
}
