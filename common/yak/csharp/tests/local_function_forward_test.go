package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func csharpFunctionsNamed(prog *ssaapi.Program, name string) []*ssa.Function {
	var functions []*ssa.Function
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function != nil && function.GetName() == name {
			functions = append(functions, function)
		}
	})
	return functions
}

func TestCSharp_LocalFunctionForwardCallUsesSingleShell(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Program {
    public static void Main() {
        forwardSink(Forward("forward-token"));
        object Forward(object value) { return value; }
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`"forward-token" as $source; forwardSink(* #-> as $origin)`)
	functions := csharpFunctionsNamed(prog, "Program.Main.Forward")
	require.Len(t, functions, 1, "predeclaration and body emission must share one function shell")
	require.True(t, functions[0].IsFinished(), "the forward-declared shell must have its body built")
	require.Len(t, functions[0].Params, 1, "signature parameters must not be created twice")
}

func TestCSharp_LocalFunctionAfterReturnIsBuilt(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Program {
    public static object Run() {
        return AfterReturn("after-return-token");
        object AfterReturn(object value) => value;
    }

    public static void Main() {
        afterReturnSink(Run());
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`"after-return-token" as $source; afterReturnSink(* #-> as $origin)`)
	functions := csharpFunctionsNamed(prog, "Program.Run.AfterReturn")
	require.Len(t, functions, 1)
	require.True(t, functions[0].IsFinished(), "a declaration after an abrupt statement must still be populated")
}

func TestCSharp_LocalFunctionRecursionAndMutualRecursion(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Program {
    public static void Main() {
        recursiveSink(Recur(0));
        mutualSink(First("mutual-token", false));

        object Recur(int depth) {
            return depth == 0 ? "recursive-token" : Recur(depth - 1);
        }
        object First(object value, bool again) {
            return again ? Second(value, false) : value;
        }
        object Second(object value, bool again) {
            return again ? First(value, false) : value;
        }
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`"recursive-token" as $source; recursiveSink(* #-> as $origin)`)
	requireCSharpSourceReachesSink(t, prog,
		`"mutual-token" as $source; mutualSink(* #-> as $origin)`)
	for _, name := range []string{"Program.Main.Recur", "Program.Main.First", "Program.Main.Second"} {
		functions := csharpFunctionsNamed(prog, name)
		require.Len(t, functions, 1, "%s must have exactly one shell", name)
		require.True(t, functions[0].IsFinished(), "%s must be built", name)
	}
}

func TestCSharp_LocalFunctionForwardCapture(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Program {
    public static void Main() {
        object captured = "capture-token";
        captureSink(ReadCaptured());
        object ReadCaptured() => captured;
    }
}`)

	requireCSharpSourceReachesSink(t, prog,
		`"capture-token" as $source; captureSink(* #-> as $origin)`)
}

func TestCSharp_LocalFunctionShellDoesNotShiftLambdaSerial(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Program {
    public static void Main() {
        System.Func<object> first = () => "first-lambda";
        object Local() => "local";
        System.Func<object> second = () => "second-lambda";
        consume(first(), Local(), second());
    }
}`)

	var anonymousNames []string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function != nil && strings.HasPrefix(function.GetName(), "Program.Main$") {
			anonymousNames = append(anonymousNames, function.GetName())
		}
	})
	require.ElementsMatch(t, []string{"Program.Main$1", "Program.Main$2"}, anonymousNames)
	require.Len(t, csharpFunctionsNamed(prog, "Program.Main.Local"), 1)
}
