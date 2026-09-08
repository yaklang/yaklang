package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCSharp_Expression_DynamicMethodTieEntersEveryBestBody(t *testing.T) {
	requireCSharpFlowContains(t, `
public class DynamicMethods {
    public static void M(object value) { cleanMethod(value); }
    public static void M(string value) { dynamicMethodSink(value); }
    public static dynamic Source() { return externalSource(); }
    public static void Main() { M(Source()); }
}
`, `dynamicMethodSink(* #-> as $origin)`, "externalSource")
}

func TestCSharp_Expression_DynamicMethodTieMergesReturns(t *testing.T) {
	requireCSharpFlowContains(t, `
public class DynamicMethods {
    public static object M(object value) { return objectReturnSource(); }
    public static object M(string value) { return stringReturnSource(); }
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(M(Source())); }
}
`, `sink(* #-> as $origin)`, "objectReturnSource", "stringReturnSource")
}

func TestCSharp_Expression_DynamicMethodTieMergesReturnedObjectState(t *testing.T) {
	t.Run("ordinary call", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Box {
    public object Value;
    public Box(object value) { Value = value; }
}
public class Holder {
    public Box M(object marker) { return new Box(returnObjectSource()); }
    public Box M(string marker) { return new Box(returnStringSource()); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        var holder = new Holder();
        returnSink(holder.M(Marker()).Value);
    }
}
`, `returnSink(* #-> as $origin)`, "returnObjectSource", "returnStringSource")
	})

	t.Run("base call", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Box {
    public object Value;
    public Box(object value) { Value = value; }
}
public class BaseHolder {
    public Box M(object marker) { return new Box(baseReturnObjectSource()); }
    public Box M(string marker) { return new Box(baseReturnStringSource()); }
}
public class ChildHolder : BaseHolder {
    public object Run(dynamic marker) { return baseReturnSink(base.M(marker).Value); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() { new ChildHolder().Run(Marker()); }
}
`, `baseReturnSink(* #-> as $origin)`, "baseReturnObjectSource", "baseReturnStringSource")
	})
}

func TestCSharp_Expression_DynamicMethodTieIsolatesReceiverState(t *testing.T) {
	requireCSharpFlowContains(t, `
public class DynamicReceiver {
    public object Value = receiverInitialSource();
    public void M(object value) { Value = objectReceiverSource(); }
    public void M(string value) { observe(value); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() {
        var receiver = new DynamicReceiver();
        receiver.M(Source());
        receiverSink(receiver.Value);
    }
}
`, `receiverSink(* #-> as $origin)`, "receiverInitialSource", "objectReceiverSource")
}

func TestCSharp_Expression_MethodProjectsNestedReceiverState(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = methodBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public void M() { Cell.Value = methodUniqueWriter(); }
}
public class Program {
    public static void Main() {
        var receiver = new C();
        receiver.M();
        methodUniqueSink(receiver.Cell.Value);
    }
}
`, `methodUniqueSink(* #-> as $origin)`, "methodUniqueWriter")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		code := `
public class Cell { public object Value = methodTieBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public void M(object marker) { Cell.Value = methodTieWriter(); }
    public void M(string marker) { methodTieReadSink(Cell.Value); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        var receiver = new C();
        receiver.M(Marker());
        methodTieFinalSink(receiver.Cell.Value);
    }
}
`
		requireCSharpFlowContains(t, code, `methodTieReadSink(* #-> as $origin)`, "methodTieBaseline")
		requireCSharpFlowContains(t, code, `methodTieFinalSink(* #-> as $origin)`, "methodTieBaseline", "methodTieWriter")
	})
}

func TestCSharp_Expression_DynamicMethodTieMergesStaticState(t *testing.T) {
	t.Run("single candidate same class bare static", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicStatic {
    public static object State = sameClassMethodStaticBaseline();
    public static void M() { State = sameClassMethodStaticWriter(); }
    public static void Main() {
        M();
        sameClassMethodStaticSink(State);
    }
}
`, `sameClassMethodStaticSink(* #-> as $origin)`, "sameClassMethodStaticWriter")
	})

	t.Run("both candidates write", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicStatic {
    public static object State = staticInitialSource();
    public static void M(object value) { State = objectStaticSource(); }
    public static void M(string value) { State = stringStaticSource(); }
    public static dynamic Source() { return externalSource(); }
    public static void Main() {
        M(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "objectStaticSource", "stringStaticSource")
	})

	t.Run("untouched candidate keeps baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicStatic {
    public static object State = staticInitialSource();
    public static void M(object value) { State = objectStaticSource(); }
    public static void M(string value) { observe(value); }
    public static dynamic Source() { return externalSource(); }
    public static void Main() {
        M(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "staticInitialSource", "objectStaticSource")
	})

	t.Run("parameter dependent writes bind through each call", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicStatic {
    public static object State = staticInitialSource();
    public static void M(object value) { State = value; }
    public static void M(string value) { State = value; }
    public static dynamic Source() { return externalSource(); }
    public static void Main() {
        M(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "externalSource")
	})
}

func TestCSharp_Expression_DynamicMethodTieMergesByReferenceOutputs(t *testing.T) {
	t.Run("ref", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicMethods {
    public static void M(ref object value) { value = objectRefSource(); }
    public static void M(ref string value) { value = stringRefSource(); }
    public static void Main() {
        dynamic value = initialSource();
        M(ref value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "objectRefSource", "stringRefSource")
	})

	t.Run("out", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicMethods {
    public static void M(out object value) { value = objectOutSource(); }
    public static void M(out string value) { value = stringOutSource(); }
    public static void Main() {
        dynamic value;
        M(out value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "objectOutSource", "stringOutSource")
	})

	t.Run("untouched ref candidate keeps baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class DynamicMethods {
    public static void M(ref object value) { value = objectRefSource(); }
    public static void M(ref string value) { observe(value); }
    public static void Main() {
        dynamic value = initialSource();
        M(ref value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "initialSource", "objectRefSource")
	})
}

func TestCSharp_Expression_DynamicMethodTieKeepsNamedBindings(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DynamicMethods {
    public static void M(object first, string second) {
        firstSink(first);
        secondSink(second);
    }
    public static void M(string second, object first) {
        alternateFirstSink(first);
        alternateSecondSink(second);
    }
    public static void Run() {
        M(second: sideB(), first: sideA());
    }
}
public class Program { public static void Main() { DynamicMethods.Run(); } }
`)

	assertOrigin := func(sink, source string) {
		t.Helper()
		flow, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
		require.NoError(t, err)
		require.Contains(t, flow.GetValues("origin").String(), source)
	}
	assertOrigin("firstSink", "sideA")
	assertOrigin("secondSink", "sideB")
	assertOrigin("alternateFirstSink", "sideA")
	assertOrigin("alternateSecondSink", "sideB")
	requireReachableCallOrder(t, prog, "Run", "sideB", "sideA")
}

func TestCSharp_Expression_NullMethodTieKeepsMoreSpecificBodySound(t *testing.T) {
	requireCSharpFlowContains(t, `
public class BaseValue { }
public class DerivedValue : BaseValue { }
public class DynamicMethods {
    public static void M(BaseValue value) { baseNullSink(value); }
    public static void M(DerivedValue value) { derivedNullSink(value); }
    public static void Main() { M(null); }
}
`, `derivedNullSink(* #-> as $origin)`, "nil")
}

func TestCSharp_Expression_DynamicMethodTieEvaluatesReceiverAndActualOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DynamicReceiver {
    public object M(object value) { return objectResult(); }
    public object M(string value) { return stringResult(); }
}
public class Harness {
    public static DynamicReceiver GetReceiver() { return new DynamicReceiver(); }
    public static dynamic GetActual() { return actualSource(); }
    public static object Run() { return GetReceiver().M(GetActual()); }
}
public class Program { public static void Main() { sink(Harness.Run()); } }
`)

	var names []string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() == "Run" {
			names = append(names, csharpFunctionCallNames(function)...)
		}
	})
	countMatching := func(fragment string) int {
		count := 0
		for _, name := range names {
			if strings.Contains(name, fragment) {
				count++
			}
		}
		return count
	}
	require.Equal(t, 1, countMatching("GetReceiver"), "receiver expression must be evaluated before and outside dispatch branches")
	require.Equal(t, 1, countMatching("GetActual"), "actual expression must be evaluated before and outside dispatch branches")
}

func TestCSharp_Expression_DynamicBaseMethodTieStaysNonVirtual(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DynamicBase {
    public object M(object value) { return baseObjectResult(); }
    public object M(string value) { return baseStringResult(); }
}
public class DynamicChild : DynamicBase {
    public object Run(dynamic value) { return sink(base.M(value)); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { new DynamicChild().Run(Source()); }
}
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 2)
	for _, call := range calls {
		require.True(t, call.IsNonVirtual)
	}
	flow, err := prog.SyntaxFlowWithError(`sink(* #-> as $origin)`)
	require.NoError(t, err)
	origins := flow.GetValues("origin").String()
	require.Contains(t, origins, "baseObjectResult")
	require.Contains(t, origins, "baseStringResult")
}

func TestCSharp_Expression_MethodProjectsNestedStaticState(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = methodNestedStaticBaseline(); }
public class StaticStore { public static Cell Holder = new Cell(); }
public class C { public void M() { StaticStore.Holder.Value = methodNestedStaticWriter(); } }
public class Program {
    public static void Main() {
        new C().M();
        methodNestedStaticSink(StaticStore.Holder.Value);
    }
}
`, `methodNestedStaticSink(* #-> as $origin)`, "methodNestedStaticWriter")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = methodNestedStaticTieBaseline(); }
public class StaticStore { public static Cell Holder = new Cell(); }
public class C {
    public void M(object marker) { StaticStore.Holder.Value = methodNestedStaticTieWriter(); }
    public void M(string marker) { observe(marker); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        new C().M(Marker());
        methodNestedStaticTieSink(StaticStore.Holder.Value);
    }
}
`, `methodNestedStaticTieSink(* #-> as $origin)`, "methodNestedStaticTieBaseline", "methodNestedStaticTieWriter")
	})
}
