package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type csharpConstructorCall struct {
	call   *ssa.Call
	callee *ssa.Function
	caller *ssa.Function
}

func collectCSharpConstructorCalls(prog *ssaapi.Program, className string) []csharpConstructorCall {
	var calls []csharpConstructorCall
	prog.Program.EachFunction(func(caller *ssa.Function) {
		for _, blockID := range caller.Blocks {
			block, ok := caller.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := caller.GetInstructionById(instructionID)
				if !ok {
					continue
				}
				call, ok := ssa.ToCall(instruction)
				if !ok {
					continue
				}
				method, ok := call.GetValueById(call.Method)
				if !ok {
					continue
				}
				callee, ok := ssa.ToFunction(method)
				if !ok || callee.GetMethodName() != className {
					continue
				}
				callee.Build()
				calls = append(calls, csharpConstructorCall{call: call, callee: callee, caller: caller})
			}
		}
	})
	return calls
}

func requireCSharpConstructorCallArities(t *testing.T, calls []csharpConstructorCall, want ...int) {
	t.Helper()
	require.Len(t, calls, len(want))
	seen := make(map[int]int)
	for _, item := range calls {
		require.Equal(t, len(item.callee.Params), len(item.call.Args),
			"constructor %s received %d SSA args for %d parameters", item.callee.GetName(), len(item.call.Args), len(item.callee.Params))
		seen[len(item.call.Args)-1]++ // exclude the synthetic $this parameter
	}
	for _, arity := range want {
		require.Positive(t, seen[arity], "missing constructor call with source arity %d", arity)
		seen[arity]--
	}
}

func csharpFunctionCallNames(function *ssa.Function) []string {
	if function == nil {
		return nil
	}
	function.Build()
	var names []string
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, instructionID := range block.Insts {
			instruction, ok := function.GetInstructionById(instructionID)
			if !ok {
				continue
			}
			call, ok := ssa.ToCall(instruction)
			if !ok {
				continue
			}
			method, ok := call.GetValueById(call.Method)
			if ok && method != nil {
				names = append(names, method.GetVerboseName())
			}
		}
	}
	return names
}

func TestCSharp_OOP_ConstructorOverloadDispatchByArity(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Box {
    public object Value;
    public Box() { Value = parameterlessSource(); }
    public Box(object value) { Value = value; }
}
public class Program {
    public static void Main() {
        zeroSink(new Box().Value);
        oneSink(new Box(argumentSource()).Value);
    }
}
`)

	requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "Box"), 0, 1)
}

func TestCSharp_OOP_ConstructorOverloadDispatchByType(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class TypedBox {
    public TypedBox(int value) { numericSink(value); }
    public TypedBox(string value) { stringSink(value); }
}
public class Program {
    public static void Main() {
        string value = typedSource();
        new TypedBox(value);
    }
}
`)

	calls := collectCSharpConstructorCalls(prog, "TypedBox")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].callee.Params, 2)
	parameter, ok := calls[0].callee.GetValueById(calls[0].callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, parameter.GetType().GetTypeKind())

	selected, err := prog.SyntaxFlowWithError(`typedSource() as $source; stringSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, selected.GetValues("origin"))
}

func TestCSharp_OOP_ThisAndBaseConstructorChainsKeepReceiver(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public object Value;
    public BaseRecord(object value) { Value = value; }
}
public class ChildRecord : BaseRecord {
    public ChildRecord() : this(chainSource()) { }
    public ChildRecord(object value) : base(value) { }
}
public class Program {
    public static void Main() { sink(new ChildRecord().Value); }
}
`)

	requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "ChildRecord"), 0, 1)
	requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "BaseRecord"), 1)
	result, err := prog.SyntaxFlowWithError(`chainSource() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("origin"), "this/base chain must project writes onto the originally allocated child")
}

func TestCSharp_OOP_ImplicitBaseConstructorCall(t *testing.T) {
	t.Run("explicit child constructor", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public object Value;
    public BaseRecord() { Value = implicitBaseSource(); }
}
public class ChildRecord : BaseRecord { public ChildRecord() { } }
public class Program { public static void Main() { sink(new ChildRecord().Value); } }
`)

		requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "ChildRecord"), 0)
		requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "BaseRecord"), 0)
	})

	t.Run("generated child constructor selects parameterless base overload", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public object Value;
    public BaseRecord(object value) { Value = value; }
    public BaseRecord() { Value = implicitBaseSource(); }
}
public class ChildRecord : BaseRecord { }
public class Program { public static void Main() { sink(new ChildRecord().Value); } }
`)

		baseCalls := collectCSharpConstructorCalls(prog, "BaseRecord")
		requireCSharpConstructorCallArities(t, baseCalls, 0)
		resultType, ok := ssa.ToBluePrintType(baseCalls[0].call.GetType())
		require.True(t, ok, "object creation must keep the derived blueprint result type")
		require.Equal(t, "ChildRecord", resultType.Name)
		result, err := prog.SyntaxFlowWithError(`implicitBaseSource() as $source; sink(* #-> as $origin)`)
		require.NoError(t, err)
		require.NotEmpty(t, result.GetValues("origin"))
	})
}

func TestCSharp_OOP_ExplicitBaseOverloadUsesArgumentArity(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public object Value;
    public BaseRecord(object value) { Value = value; }
    public BaseRecord() { Value = defaultSource(); }
}
public class ChildRecord : BaseRecord {
    public ChildRecord(object value) : base(value) { }
}
public class Program { public static void Main() { sink(new ChildRecord(argumentSource()).Value); } }
`)

	requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "ChildRecord"), 1)
	requireCSharpConstructorCallArities(t, collectCSharpConstructorCalls(prog, "BaseRecord"), 1)
}

func TestCSharp_OOP_ConstructorOptionalArgumentSelection(t *testing.T) {
	t.Run("implicit base accepts optional parameter", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class OptionalBase {
    public OptionalBase(object value = null) { optionalSink(value); }
}
public class OptionalChild : OptionalBase { }
public class Program { public static void Main() { new OptionalChild(); } }
`)

		calls := collectCSharpConstructorCalls(prog, "OptionalBase")
		require.Len(t, calls, 1)
		require.Len(t, calls[0].call.Args, 1, "the source call omits the optional argument and only passes $this")
		require.Len(t, calls[0].callee.Params, 2, "the selected overload must retain $this plus its optional formal")
		formal, ok := calls[0].callee.GetValueById(calls[0].callee.Params[1])
		require.True(t, ok)
		parameter, ok := ssa.ToParameter(formal)
		require.True(t, ok)
		require.NotNil(t, parameter.GetDefault(), "the omitted formal must carry its declared default value")
	})

	t.Run("exact arity wins over optional candidate", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class TieBox {
    public TieBox(object value = null) { optionalSink(value); }
    public TieBox() { exactSink(); }
}
public class Program { public static void Main() { new TieBox(); } }
`)

		calls := collectCSharpConstructorCalls(prog, "TieBox")
		requireCSharpConstructorCallArities(t, calls, 0)
		require.Len(t, calls[0].callee.Params, 1, "the exact parameterless overload must beat the optional overload")
	})
}

func TestCSharp_OOP_ConstructorNamedArgumentsBindAfterSourceOrderEvaluation(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class NamedBox {
    public NamedBox(int first, string second) { selectedSink(first); selectedSink(second); }
    public NamedBox(string first, int second) { wrongSink(first); wrongSink(second); }
}
public class Program {
    public static object Run() {
        return new NamedBox(second: (string)sideB(), first: (int)sideA());
    }
    public static void Main() { Run(); }
}
`)

	calls := collectCSharpConstructorCalls(prog, "NamedBox")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].call.Args, 3)
	require.Len(t, calls[0].callee.Params, 3)
	firstFormal, ok := calls[0].callee.GetValueById(calls[0].callee.Params[1])
	require.True(t, ok)
	secondFormal, ok := calls[0].callee.GetValueById(calls[0].callee.Params[2])
	require.True(t, ok)
	require.Equal(t, "first", firstFormal.GetName())
	require.Equal(t, ssa.NumberTypeKind, firstFormal.GetType().GetTypeKind())
	require.Equal(t, "second", secondFormal.GetName())
	require.Equal(t, ssa.StringTypeKind, secondFormal.GetType().GetTypeKind())

	firstArg, ok := calls[0].call.GetValueById(calls[0].call.Args[1])
	require.True(t, ok)
	secondArg, ok := calls[0].call.GetValueById(calls[0].call.Args[2])
	require.True(t, ok)
	firstValue, err := prog.NewValue(firstArg)
	require.NoError(t, err)
	secondValue, err := prog.NewValue(secondArg)
	require.NoError(t, err)
	require.Contains(t, firstValue.GetTopDefs().String(), "sideA", "Call.Args must be reordered to the first formal")
	require.Contains(t, secondValue.GetTopDefs().String(), "sideB", "Call.Args must be reordered to the second formal")
	requireReachableCallOrder(t, prog, "Run", "sideB", "sideA", "return")

}

func TestCSharp_OOP_ConstructorParamsArePackedIntoOneFormal(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ParamsBox {
    public ParamsBox(params object[] values) {
        firstSink(values[0]);
        secondSink(values[1]);
    }
}
public class Program {
    public static void Main() { new ParamsBox(firstSource(), secondSource()); }
}
`)

	calls := collectCSharpConstructorCalls(prog, "ParamsBox")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].callee.Params, 2, "constructor has $this plus one params formal")
	require.Len(t, calls[0].call.Args, 2, "expanded params values must be packed into one SSA argument")
	packed, ok := calls[0].call.GetValueById(calls[0].call.Args[1])
	require.True(t, ok)
	require.Equal(t, ssa.SliceTypeKind, packed.GetType().GetTypeKind())
	first, ok := ssa.GetLatestMemberByKeyString(packed, "0")
	require.True(t, ok)
	second, ok := ssa.GetLatestMemberByKeyString(packed, "1")
	require.True(t, ok)
	firstValue, err := prog.NewValue(first)
	require.NoError(t, err)
	secondValue, err := prog.NewValue(second)
	require.NoError(t, err)
	require.Contains(t, firstValue.GetTopDefs().String(), "firstSource")
	require.Contains(t, secondValue.GetTopDefs().String(), "secondSource")

	flow, err := prog.SyntaxFlowWithError(`
firstSink(* #-> as $first);
secondSink(* #-> as $second)
`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "firstSource")
	require.Contains(t, flow.GetValues("second").String(), "secondSource")
}

func TestCSharp_OOP_OmittedConstructorArgumentUsesDeclaredDefault(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class OptionalBox {
    public object Value;
    public OptionalBox(object value = "fallback") { Value = value; }
}
public class Program {
    public static void Main() { defaultSink(new OptionalBox().Value); }
}
`)

	flow, err := prog.SyntaxFlowWithError(`defaultSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "fallback")
}

func TestCSharp_OOP_ConstructorNamedArgumentMaterializesEarlierDefault(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class OptionalHoleBox {
    public OptionalHoleBox(object first = "fallback", object second = null) {
        firstSink(first);
        secondSink(second);
    }
}
public class Program {
    public static void Main() { new OptionalHoleBox(second: source()); }
}
`)

	calls := collectCSharpConstructorCalls(prog, "OptionalHoleBox")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].call.Args, 3, "receiver, materialized first default, named second argument")
	first, ok := calls[0].call.GetValueById(calls[0].call.Args[1])
	require.True(t, ok)
	firstValue, err := prog.NewValue(first)
	require.NoError(t, err)
	require.Contains(t, firstValue.GetTopDefs().String(), "fallback")
	second, ok := calls[0].call.GetValueById(calls[0].call.Args[2])
	require.True(t, ok)
	secondValue, err := prog.NewValue(second)
	require.NoError(t, err)
	require.Contains(t, secondValue.GetTopDefs().String(), "source")

	flow, err := prog.SyntaxFlowWithError(`firstSink(* #-> as $first); secondSink(* #-> as $second)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "fallback")
	require.Contains(t, flow.GetValues("second").String(), "source")
}

func TestCSharp_OOP_ConstructorIntegerLiteralPrefersIntOverLong(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class NumericBox {
    public NumericBox(long value) { longCtorSink(value); }
    public NumericBox(int value) { intCtorSink(value); }
}
public class Program { public static void Main() { new NumericBox(1); } }
`)

	calls := collectCSharpConstructorCalls(prog, "NumericBox")
	require.Len(t, calls, 1)
	names := csharpFunctionCallNames(calls[0].callee)
	require.Contains(t, names, "intCtorSink")
	require.NotContains(t, names, "longCtorSink")
}

func TestCSharp_OOP_ConstructorDerivedArgumentPrefersBaseOverObject(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseValue { }
public class DerivedValue : BaseValue { }
public class InheritanceBox {
    public InheritanceBox(object value) { objectCtorSink(value); }
    public InheritanceBox(BaseValue value) { baseCtorSink(value); }
}
public class Program { public static void Main() { new InheritanceBox(new DerivedValue()); } }
`)

	calls := collectCSharpConstructorCalls(prog, "InheritanceBox")
	require.Len(t, calls, 1)
	names := csharpFunctionCallNames(calls[0].callee)
	require.Contains(t, names, "baseCtorSink")
	require.NotContains(t, names, "objectCtorSink")
}
