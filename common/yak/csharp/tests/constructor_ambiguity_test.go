package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func requireCSharpFlowContains(t *testing.T, code, query string, names ...string) {
	t.Helper()
	prog := parseCSharpSemantics(t, code)
	flow, err := prog.SyntaxFlowWithError(query)
	require.NoError(t, err)
	origins := flow.GetValues("origin").String()
	for _, name := range names {
		require.Contains(t, origins, name)
	}
}

func TestCSharp_OOP_DynamicConstructorTieEntersEveryBestBody(t *testing.T) {
	requireCSharpFlowContains(t, `
public class C {
    public C(object value) { cleanCtor(value); }
    public C(string value) { dynamicCtorSink(value); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { new C(Source()); }
}
`, `dynamicCtorSink(* #-> as $origin)`, "externalSource")
}

func TestCSharp_OOP_DynamicConstructorTieMergesReceiverFields(t *testing.T) {
	requireCSharpFlowContains(t, `
public class C {
    public object Value;
    public C(object value) { Value = objectCtorSource(); }
    public C(string value) { Value = stringCtorSource(); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new C(Source()).Value); }
}
`, `sink(* #-> as $origin)`, "objectCtorSource", "stringCtorSource")
}

func TestCSharp_OOP_DynamicConstructorTieRestoresReceiverBeforeEachCandidate(t *testing.T) {
	requireCSharpFlowContains(t, `
public class C {
    public object Value = initializerSource();
    public C(object value) { Value = objectCtorSource(); }
    public C(string value) { observe(value); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new C(Source()).Value); }
}
`, `sink(* #-> as $origin)`, "initializerSource", "objectCtorSource")
}

func TestCSharp_OOP_DynamicConstructorTieMergesNestedReceiverState(t *testing.T) {
	t.Run("single candidate control", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Holder {
    public object Value = nestedInitialSource();
}
public class C {
    public Holder State = new Holder();
    public C(object value) { State.Value = nestedControlSource(); }
}
public class Program {
    public static void Main() { sink(new C(null).State.Value); }
}
`, `sink(* #-> as $origin)`, "nestedControlSource")
	})

	t.Run("writer and untouched candidate", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Holder {
    public object Value = nestedInitialSource();
}
public class C {
    public Holder State = new Holder();
    public C(object value) { State.Value = nestedCtorSource(); }
    public C(string value) { observe(value); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new C(Source()).State.Value); }
}
`, `sink(* #-> as $origin)`, "nestedInitialSource", "nestedCtorSource")
	})
}

func TestCSharp_OOP_DynamicConstructorTieMergesStaticState(t *testing.T) {
	t.Run("single candidate same class bare static", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public static object State = sameClassStaticBaseline();
    public C() { State = sameClassStaticWriter(); }
    public static void Run() {
        new C();
        sameClassStaticSink(State);
    }
}
public class Program { public static void Main() { C.Run(); } }
`, `sameClassStaticSink(* #-> as $origin)`, "sameClassStaticWriter")
	})

	t.Run("single candidate external static control", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class StaticStore {
    public static object State = staticInitialSource();
}
public class C {
    public C(object value) { StaticStore.State = singleStaticWriter(); }
}
public class Program {
    public static void Main() {
        new C(new object());
        staticSink(StaticStore.State);
    }
}
`, `staticSink(* #-> as $origin)`, "singleStaticWriter")
	})

	t.Run("external static container", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class StaticStore {
    public static object State = staticInitialSource();
}
public class C {
    public C(object value) { StaticStore.State = objectStaticSource(); }
    public C(string value) { StaticStore.State = stringStaticSource(); }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() {
        new C(Source());
        staticSink(StaticStore.State);
    }
}
`, `staticSink(* #-> as $origin)`, "objectStaticSource", "stringStaticSource")
	})

	t.Run("both candidates write", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public static object State = staticInitialSource();
    public C(object value) { State = objectStaticSource(); }
    public C(string value) { State = stringStaticSource(); }
    public static dynamic Source() { return externalSource(); }
    public static void Run() {
        new C(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "objectStaticSource", "stringStaticSource")
	})

	t.Run("untouched candidate keeps baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public static object State = staticInitialSource();
    public C(object value) { State = objectStaticSource(); }
    public C(string value) { observe(value); }
    public static dynamic Source() { return externalSource(); }
    public static void Run() {
        new C(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "staticInitialSource", "objectStaticSource")
	})

	t.Run("parameter dependent writes bind through each call", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public static object State = staticInitialSource();
    public C(object value) { State = value; }
    public C(string value) { State = value; }
    public static dynamic Source() { return externalSource(); }
    public static void Run() {
        new C(Source());
        staticSink(State);
    }
}
`, `staticSink(* #-> as $origin)`, "externalSource")
	})
}

func TestCSharp_OOP_DynamicConstructorTieMergesInitializerReceiverState(t *testing.T) {
	t.Run("base", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class BaseRecord {
    public object Value;
    public BaseRecord(object value) { Value = baseObjectSource(); }
    public BaseRecord(string value) { Value = baseStringSource(); }
}
public class ChildRecord : BaseRecord {
    public ChildRecord(dynamic value) : base(value) { }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new ChildRecord(Source()).Value); }
}
`, `sink(* #-> as $origin)`, "baseObjectSource", "baseStringSource")
	})

	t.Run("this", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class ThisRecord {
    public object Value;
    public ThisRecord(object value) { Value = thisObjectSource(); }
    public ThisRecord(string value) { Value = thisStringSource(); }
    public ThisRecord(dynamic value, bool dispatch) : this(value) { }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new ThisRecord(Source(), true).Value); }
}
`, `sink(* #-> as $origin)`, "thisObjectSource", "thisStringSource")
	})

	t.Run("nested base", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Holder {
    public object Value = nestedBaseInitialSource();
}
public class BaseRecord {
    public Holder State = new Holder();
    public BaseRecord(object value) { State.Value = nestedBaseObjectSource(); }
    public BaseRecord(string value) { State.Value = nestedBaseStringSource(); }
}
public class ChildRecord : BaseRecord {
    public ChildRecord(dynamic value) : base(value) { }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new ChildRecord(Source()).State.Value); }
}
`, `sink(* #-> as $origin)`, "nestedBaseObjectSource", "nestedBaseStringSource")
	})

	t.Run("nested this", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Holder {
    public object Value = nestedThisInitialSource();
}
public class ThisRecord {
    public Holder State = new Holder();
    public ThisRecord(object value) { State.Value = nestedThisObjectSource(); }
    public ThisRecord(string value) { State.Value = nestedThisStringSource(); }
    public ThisRecord(dynamic value, bool dispatch) : this(value) { }
}
public class Program {
    public static dynamic Source() { return externalSource(); }
    public static void Main() { sink(new ThisRecord(Source(), true).State.Value); }
}
`, `sink(* #-> as $origin)`, "nestedThisObjectSource", "nestedThisStringSource")
	})
}

func TestCSharp_OOP_DynamicConstructorTieMergesByReferenceOutputs(t *testing.T) {
	t.Run("ref", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public C(ref object value) { value = objectRefSource(); }
    public C(ref string value) { value = stringRefSource(); }
}
public class Program {
    public static void Main() {
        dynamic value = initialSource();
        new C(ref value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "objectRefSource", "stringRefSource")
	})

	t.Run("out", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public C(out object value) { value = objectOutSource(); }
    public C(out string value) { value = stringOutSource(); }
}
public class Program {
    public static void Main() {
        dynamic value;
        new C(out value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "objectOutSource", "stringOutSource")
	})

	t.Run("untouched ref candidate keeps baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public C(ref object value) { value = objectRefSource(); }
    public C(ref string value) { observe(value); }
}
public class Program {
    public static void Main() {
        dynamic value = initialSource();
        new C(ref value);
        sink(value);
    }
}
`, `sink(* #-> as $origin)`, "initialSource", "objectRefSource")
	})

	t.Run("shared ref slot keeps writer and baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public C(object marker, ref object value) { value = refWriterSource(); }
    public C(string marker, ref object value) { refReadOnlySink(value); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        object value = refBaseline();
        new C(Marker(), ref value);
        refFinalSink(value);
    }
}
`, `refFinalSink(* #-> as $origin)`, "refBaseline", "refWriterSource")
	})
}

func TestCSharp_OOP_DynamicConstructorTieEvaluatesInputsOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class C {
    public object State = receiverInitializerSource();
    public C(object value) { observeObject(value); }
    public C(string value) { observeString(value); }
}
public class Program {
    public static dynamic Actual() { return actualSource(); }
    public static void Main() { sink(new C(Actual()).State); }
}
`)

	var names []string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() == "Main" {
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
	require.Equal(t, 1, countMatching("Actual"), "constructor actual must be evaluated outside candidate branches")
	calls := collectCSharpConstructorCalls(prog, "C")
	require.Len(t, calls, 2)
	require.GreaterOrEqual(t, len(calls[0].call.Args), 2)
	receiverID := calls[0].call.Args[0]
	actualID := calls[0].call.Args[1]
	for _, candidate := range calls[1:] {
		require.GreaterOrEqual(t, len(candidate.call.Args), 2)
		require.Equal(t, receiverID, candidate.call.Args[0], "all candidate calls must reuse one initialized receiver")
		require.Equal(t, actualID, candidate.call.Args[1], "all candidate calls must reuse one evaluated actual")
	}
}

func TestCSharp_OOP_ConstructorProjectsIndirectHelperWrites(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class StaticStore { public static object Value = indirectStaticBaseline(); }
public class C {
    public static void Initialize() { StaticStore.Value = indirectStaticWriter(); }
    public C() { Initialize(); }
}
public class Program {
    public static void Main() {
        new C();
        indirectStaticSink(StaticStore.Value);
    }
}
`, `indirectStaticSink(* #-> as $origin)`, "indirectStaticWriter")
	})

	t.Run("nested receiver", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = indirectNestedBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public void Initialize() { Cell.Value = indirectNestedWriter(); }
    public C() { Initialize(); }
}
public class Program {
    public static void Main() {
        var receiver = new C();
        indirectNestedSink(receiver.Cell.Value);
    }
}
`, `indirectNestedSink(* #-> as $origin)`, "indirectNestedWriter")
	})

	t.Run("dynamic tie keeps static baseline", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class StaticStore { public static object Value = indirectTieStaticBaseline(); }
public class C {
    public static void Initialize() { StaticStore.Value = indirectTieStaticWriter(); }
    public C(object marker) { Initialize(); }
    public C(string marker) { observe(marker); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        new C(Marker());
        indirectTieStaticSink(StaticStore.Value);
    }
}
`, `indirectTieStaticSink(* #-> as $origin)`, "indirectTieStaticBaseline", "indirectTieStaticWriter")
	})
}

func TestCSharp_OOP_ConstructorNestedRootReplacement(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = replaceBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public C() {
        Cell = new Cell();
        Cell.Value = replaceWriter();
    }
}
public class Program {
    public static void Main() {
        var receiver = new C();
        replaceSink(receiver.Cell.Value);
    }
}
`, `replaceSink(* #-> as $origin)`, "replaceWriter")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = replaceTieBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public C(object marker) {
        Cell = new Cell();
        Cell.Value = replaceTieWriter();
    }
    public C(string marker) { observe(marker); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        var receiver = new C(Marker());
        replaceTieSink(receiver.Cell.Value);
    }
}
`, `replaceTieSink(* #-> as $origin)`, "replaceTieBaseline", "replaceTieWriter")
	})
}

func TestCSharp_OOP_ConstructorActualizesIndexedReceiverPath(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public object[] Values = new object[1];
    public C(int index) { Values[index] = indexedUniqueWriter(); }
}
public class Program {
    public static void Main() {
        var receiver = new C(0);
        indexedUniqueSink(receiver.Values[0]);
    }
}
`, `indexedUniqueSink(* #-> as $origin)`, "indexedUniqueWriter")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class C {
    public object[] Values = new object[1];
    public C(object marker, int index) { Values[index] = indexedTieWriter(); }
    public C(string marker, int index) { observe(marker); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        var receiver = new C(Marker(), 0);
        indexedTieSink(receiver.Values[0]);
    }
}
`, `indexedTieSink(* #-> as $origin)`, "indexedTieWriter")
	})
}

func TestCSharp_OOP_ConstructorProjectsFormalMemberWrites(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Input { public object Payload = formalMemberSource(); }
public class StaticStore { public static object Value = formalStateBaseline(); }
public class C { public C(Input input) { StaticStore.Value = input.Payload; } }
public class Program {
    public static void Main() {
        new C(new Input());
        formalSink(StaticStore.Value);
    }
}
`, `formalSink(* #-> as $origin)`, "formalMemberSource")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Input { public object Payload = formalTieMemberSource(); }
public class StaticStore { public static object Value = formalTieStateBaseline(); }
public class C {
    public C(object marker, Input input) { StaticStore.Value = input.Payload; }
    public C(string marker, Input input) { StaticStore.Value = input.Payload; }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        new C(Marker(), new Input());
        formalTieSink(StaticStore.Value);
    }
}
`, `formalTieSink(* #-> as $origin)`, "formalTieMemberSource")
	})
}

func TestCSharp_OOP_ConstructorExplicitWritesKeepLastAssignment(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		code := `
public class StaticStore { public static object Value = seqBaseline(); }
public class C {
    public C() {
        StaticStore.Value = seqOverwrittenSource();
        StaticStore.Value = seqFinalClean();
    }
}
public class Program {
    public static void Main() {
        new C();
        seqSink(StaticStore.Value);
    }
}
`
		requireCSharpFlowContains(t, code, `seqSink(* #-> as $origin)`, "seqFinalClean")
		prog := parseCSharpSemantics(t, code)
		flow, err := prog.SyntaxFlowWithError(`seqSink(* #-> as $origin)`)
		require.NoError(t, err)
		require.NotContains(t, flow.GetValues("origin").String(), "seqOverwrittenSource")
	})

	t.Run("nested receiver", func(t *testing.T) {
		code := `
public class Cell { public object Value = seqNestedBaseline(); }
public class C {
    public Cell Cell = new Cell();
    public C() {
        Cell.Value = seqNestedOverwrittenSource();
        Cell.Value = seqNestedFinalClean();
    }
}
public class Program {
    public static void Main() {
        var receiver = new C();
        seqNestedSink(receiver.Cell.Value);
    }
}
`
		requireCSharpFlowContains(t, code, `seqNestedSink(* #-> as $origin)`, "seqNestedFinalClean")
		prog := parseCSharpSemantics(t, code)
		flow, err := prog.SyntaxFlowWithError(`seqNestedSink(* #-> as $origin)`)
		require.NoError(t, err)
		require.NotContains(t, flow.GetValues("origin").String(), "seqNestedOverwrittenSource")
	})
}

func TestCSharp_OOP_NullConstructorTieKeepsMoreSpecificBodySound(t *testing.T) {
	requireCSharpFlowContains(t, `
public class BaseValue { }
public class DerivedValue : BaseValue { }
public class C {
    public C(BaseValue value) { baseNullSink(value); }
    public C(DerivedValue value) { derivedNullSink(value); }
}
public class Program {
    public static void Main() { new C(null); }
}
`, `derivedNullSink(* #-> as $origin)`, "nil")
}

func TestCSharp_OOP_ConstructorProjectsNestedStaticState(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = nestedStaticBaseline(); }
public class StaticStore { public static Cell Holder = new Cell(); }
public class C { public C() { StaticStore.Holder.Value = nestedStaticCtorWriter(); } }
public class Program {
    public static void Main() {
        new C();
        nestedStaticCtorSink(StaticStore.Holder.Value);
    }
}
`, `nestedStaticCtorSink(* #-> as $origin)`, "nestedStaticCtorWriter")
	})

	t.Run("dynamic tie", func(t *testing.T) {
		requireCSharpFlowContains(t, `
public class Cell { public object Value = nestedStaticTieBaseline(); }
public class StaticStore { public static Cell Holder = new Cell(); }
public class C {
    public C(object marker) { StaticStore.Holder.Value = nestedStaticTieWriter(); }
    public C(string marker) { observe(marker); }
}
public class Program {
    public static dynamic Marker() { return dynamicMarker(); }
    public static void Main() {
        new C(Marker());
        nestedStaticTieSink(StaticStore.Holder.Value);
    }
}
`, `nestedStaticTieSink(* #-> as $origin)`, "nestedStaticTieBaseline", "nestedStaticTieWriter")
	})
}
