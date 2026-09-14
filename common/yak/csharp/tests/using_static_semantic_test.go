package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_UsingStaticDeclaredTypeAndFrameworkFallback(t *testing.T) {
	prog := parseCSharpSemantics(t, `
namespace Demo.Utility {
    public class Helpers {
        public static int Answer = 42;
        public static int Forward(int value) { return value; }
    }
}

namespace Demo.App {
    using static Demo.Utility.Helpers;
    using static System.Console;

    public class Program {
        public static void Main(string[] args) {
            var value = Forward(source());
            sink(value);
            println(Answer);
            WriteLine(value);
        }
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin); WriteLine(* as $written); println(* as $printed)`)
	require.NoError(t, err)
	require.NotEmpty(t, flow.GetValues("source"))
	require.NotEmpty(t, flow.GetValues("origin"), "declared static import must preserve call data flow")
	written := flow.GetValues("written")
	require.Len(t, written, 1, "framework static call must not gain an implicit receiver: %s", written.String())
	require.True(t, strings.Contains(written.String(), "Forward") || strings.Contains(written.String(), "source"),
		"framework static call must receive the source-derived value: %s", written.String())

	printed := flow.GetValues("printed")
	require.NotEmpty(t, printed)
	require.Contains(t, printed.String(), "42")
}
