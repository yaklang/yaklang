package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func resolveCSharpTestCallFunction(call *ssa.Call) *ssa.Function {
	if call == nil {
		return nil
	}
	value, ok := call.GetValueById(call.Method)
	if !ok {
		return nil
	}
	seen := make(map[int64]struct{})
	for value != nil {
		if _, exists := seen[value.GetId()]; exists {
			return nil
		}
		seen[value.GetId()] = struct{}{}
		if function, ok := ssa.ToFunction(value); ok {
			function.Build()
			return function
		}
		value = value.GetReference()
	}
	return nil
}

func collectCSharpMethodCalls(prog *ssaapi.Program, callerName, methodName string) []*ssa.Call {
	var calls []*ssa.Call
	prog.Program.EachFunction(func(caller *ssa.Function) {
		if caller.GetMethodName() != callerName {
			return
		}
		caller.Build()
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
				callee := resolveCSharpTestCallFunction(call)
				if callee != nil && callee.GetMethodName() == methodName {
					calls = append(calls, call)
				}
			}
		}
	})
	return calls
}

func requireCSharpArgumentTopDef(t *testing.T, prog *ssaapi.Program, call *ssa.Call, index int, want string) {
	t.Helper()
	require.Less(t, index, len(call.Args))
	argument, ok := call.GetValueById(call.Args[index])
	require.True(t, ok)
	value, err := prog.NewValue(argument)
	require.NoError(t, err)
	require.Contains(t, value.GetTopDefs().String(), want)
}

func TestCSharp_Expression_SourceMethodNamedArguments(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MethodArguments {
    public static void M(object first, object second) {
        firstSink(first);
        secondSink(second);
    }
    public static object Run() {
        M(second: sideB(), first: sideA());
        return done();
    }
}
public class Program { public static void Main() { MethodArguments.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2)
	requireCSharpArgumentTopDef(t, prog, calls[0], 0, "sideA")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "sideB")
	requireReachableCallOrder(t, prog, "Run", "sideB", "sideA")

	flow, err := prog.SyntaxFlowWithError(`firstSink(* #-> as $first); secondSink(* #-> as $second)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "sideA")
	require.Contains(t, flow.GetValues("second").String(), "sideB")
}

func TestCSharp_Expression_SourceMethodNamedArgumentOptionalHole(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MethodArguments {
    public static void M(object first = "fallback", object second = null) {
        firstSink(first);
        secondSink(second);
    }
    public static void Run() { M(second: source()); }
}
public class Program { public static void Main() { MethodArguments.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2)
	requireCSharpArgumentTopDef(t, prog, calls[0], 0, "fallback")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "source")
	flow, err := prog.SyntaxFlowWithError(`firstSink(* #-> as $first); secondSink(* #-> as $second)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "fallback")
	require.Contains(t, flow.GetValues("second").String(), "source")
}

func TestCSharp_Expression_SourceMethodParamsArePacked(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MethodArguments {
    public static void P(params object[] values) {
        firstSink(values[0]);
        secondSink(values[1]);
    }
    public static void Run() { P(firstSource(), secondSource()); }
}
public class Program { public static void Main() { MethodArguments.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "P")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 1)
	packed, ok := calls[0].GetValueById(calls[0].Args[0])
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

	flow, err := prog.SyntaxFlowWithError(`firstSink(* #-> as $first); secondSink(* #-> as $second)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "firstSource")
	require.Contains(t, flow.GetValues("second").String(), "secondSource")
}

func TestCSharp_Expression_NamedRefArgumentsKeepFormalIndexes(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MethodArguments {
	public static void M(ref object first, ref object second) {
		first = firstUpdated();
		second = secondUpdated();
	}
    public static void Run() {
        object first = firstInitial();
        object second = secondInitial();
        M(second: ref second, first: ref first);
        afterFirst(first);
        afterSecond(second);
    }
}
public class Program { public static void Main() { MethodArguments.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	requireCSharpArgumentTopDef(t, prog, calls[0], 0, "firstInitial")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "secondInitial")
	flow, err := prog.SyntaxFlowWithError(`afterFirst(* #-> as $first); afterSecond(* #-> as $second)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("first").String(), "firstUpdated")
	require.NotContains(t, flow.GetValues("first").String(), "secondUpdated")
	require.Contains(t, flow.GetValues("second").String(), "secondUpdated")
	require.NotContains(t, flow.GetValues("second").String(), "firstUpdated")
}

func TestCSharp_Expression_StaticOutParameterFlowsBackToCaller(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class RefOutFlow {
    public static void Fill(out object value) { value = outSource(); }
    public static void Run() {
        object value;
        Fill(out value);
        outSink(value);
    }
}
public class Program { public static void Main() { RefOutFlow.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "Fill")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 1)
	flow, err := prog.SyntaxFlowWithError(`outSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "outSource")
}

func TestCSharp_Expression_InstanceRefParameterUsesReceiverOffset(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class RefOutFlow {
    public void Fill(ref object value) { value = refSource(); }
    public static void Run() {
        object value = cleanInitial();
        var receiver = new RefOutFlow();
        receiver.Fill(ref value);
        refSink(value);
    }
}
public class Program { public static void Main() { RefOutFlow.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "Fill")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2, "instance this occupies index zero and ref value index one")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "cleanInitial")
	flow, err := prog.SyntaxFlowWithError(`refSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "refSource")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanInitial")
}

func TestCSharp_Expression_LocalFunctionOutParameterFlowsBackToCaller(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class LocalRefOutFlow {
	public static object Source() { return localOutSource(); }
    public static void Run() {
		void Fill(out object value) { value = LocalRefOutFlow.Source(); }
        object value;
        Fill(out value);
        localOutSink(value);
    }
}
public class Program { public static void Main() { LocalRefOutFlow.Run(); } }
`)

	flow, err := prog.SyntaxFlowWithError(`localOutSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "localOutSource")
}

func TestCSharp_Expression_ForwardLocalFunctionOutParameterFlowsBackToCaller(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ForwardLocalRefOutFlow {
    public static object Source() { return forwardLocalOutSource(); }
    public static void Run() {
        object value;
        Fill(out value);
        forwardLocalOutSink(value);
        void Fill(out object target) { target = ForwardLocalRefOutFlow.Source(); }
    }
}
public class Program { public static void Main() { ForwardLocalRefOutFlow.Run(); } }
`)

	flow, err := prog.SyntaxFlowWithError(`forwardLocalOutSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "forwardLocalOutSource")
}

func TestCSharp_Expression_InstanceOverloadSelectsCalleeAndNamedBindingTogether(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MethodOverloads {
    public object M(object a, string b) { wrongOverloadSink(a); return cleanResult(); }
    public object M(string b, int a) { selectedOverloadSink(a); return a; }
    public static object Run() {
        var receiver = new MethodOverloads();
        return finalSink(receiver.M(b: (string)cleanString(), a: (int)source()));
    }
}
public class Program { public static void Main() { MethodOverloads.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 3, "instance receiver plus two selected formals")
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 3)
	bFormal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	aFormal, ok := callee.GetValueById(callee.Params[2])
	require.True(t, ok)
	require.Equal(t, "b", bFormal.GetName())
	require.Equal(t, "a", aFormal.GetName())
	require.Contains(t, csharpFunctionCallNames(callee), "selectedOverloadSink")
	require.NotContains(t, csharpFunctionCallNames(callee), "wrongOverloadSink")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "cleanString")
	requireCSharpArgumentTopDef(t, prog, calls[0], 2, "source")

	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanResult",
		"the Blueprint's first overload must not remain a pointer child of the selected overload")
}

func TestCSharp_Expression_AmbiguousUnknownOverloadKeepsAllActualFlows(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class AmbiguousOverloads {
    public object M(object a, string b) { return cleanResult(); }
    public object M(string b, int a) { return a; }
    public static object Run() {
        var receiver = new AmbiguousOverloads();
        return finalSink(receiver.M(b: cleanString(), a: source()));
    }
}
public class Program { public static void Main() { AmbiguousOverloads.Run(); } }
`)

	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source",
		"when Any-typed actuals cannot select one overload, the conservative call must retain every actual")
}

func TestCSharp_Expression_StaticOverloadSelectsCalleeAndNamedBindingTogether(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class StaticOverloads {
    public static object M(object a, string b) { return cleanResult(); }
    public static object M(string b, int a) { return a; }
    public static object Run() {
        return finalSink(M(b: (string)cleanString(), a: (int)source()));
    }
}
public class Program { public static void Main() { StaticOverloads.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2)
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 2)
	first, ok := callee.GetValueById(callee.Params[0])
	require.True(t, ok)
	second, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, "b", first.GetName())
	require.Equal(t, "a", second.GetName())
	requireCSharpArgumentTopDef(t, prog, calls[0], 0, "cleanString")
	requireCSharpArgumentTopDef(t, prog, calls[0], 1, "source")

	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanResult")
}

func TestCSharp_Expression_PositionalOverloadPrefersSpecificType(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class PositionalOverloads {
    public object Pick(object value) { return cleanResult(); }
    public object Pick(string value) { return value; }
    public static object Run() {
        var receiver = new PositionalOverloads();
        string value = (string)source();
        return finalSink(receiver.Pick(value));
    }
}
public class Program { public static void Main() { PositionalOverloads.Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "Pick")
	require.Len(t, calls, 1)
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 2)
	formal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, formal.GetType().GetTypeKind())
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanResult")
}

func TestCSharp_Expression_BaseOverloadUsesDirectParentCandidates(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseOverloads {
    public object M(object a, string b) { return cleanResult(); }
    public object M(string b, int a) { return a; }
}
public class ChildOverloads : BaseOverloads {
    public object Run() {
        return finalSink(base.M(b: (string)cleanString(), a: (int)source()));
    }
}
public class Program { public static void Main() { new ChildOverloads().Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.True(t, calls[0].IsNonVirtual)
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 3)
	first, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	second, ok := callee.GetValueById(callee.Params[2])
	require.True(t, ok)
	require.Equal(t, "b", first.GetName())
	require.Equal(t, "a", second.GetName())
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanResult")
}

func TestCSharp_Expression_BareInstanceOverloadInjectsThisOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BareInstanceOverloads {
    public object M(object value) { return cleanResult(); }
    public object M(string value) { return value; }
    public object Run() {
        string value = (string)source();
        return finalSink(M(value));
    }
}
public class Program { public static void Main() { new BareInstanceOverloads().Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2, "bare instance call must inject exactly one this argument")
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	formal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, formal.GetType().GetTypeKind())
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanResult")
}

func TestCSharp_Expression_BareCallCombinesStaticAndInstanceOverloads(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class MixedBareOverloads {
    public static object M(int value) { return "clean"; }
    public object M(string value) { return value; }
    public object Run() {
        string value = (string)source();
        return finalSink(M(value));
    }
}
public class Program { public static void Main() { new MixedBareOverloads().Run(); } }
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Args, 2, "the selected instance overload must receive this plus value")
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 2)
	formal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, formal.GetType().GetTypeKind())
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "source")
	require.NotContains(t, flow.GetValues("origin").String(), "clean")
}

func TestCSharp_Expression_LaterOverloadKeepsVirtualOverrideDispatch(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class VirtualBaseOverloads {
    public virtual object M(int value) { return cleanBaseInt(); }
    public virtual object M(string value) { return cleanBaseString(); }
}
public class VirtualDerivedOverloads : VirtualBaseOverloads {
    public override object M(string value) { return overrideSource(); }
}
public class VirtualDispatchHarness {
    public static object Run(VirtualBaseOverloads receiver) {
        return finalSink(receiver.M("x"));
    }
}
public class Program {
    public static void Main() { VirtualDispatchHarness.Run(new VirtualDerivedOverloads()); }
}
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	require.False(t, calls[0].IsNonVirtual, "ordinary virtual calls must not be marked as base/nonvirtual calls")
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 2)
	formal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, formal.GetType().GetTypeKind(), "compile-time overload resolution selects Base.M(string)")
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "overrideSource",
		"the selected later overload must retain matching derived override dispatch")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanBaseInt",
		"the unrelated first overload must remain outside the virtual dispatch graph")
}

func TestCSharp_Expression_InterfaceLaterOverloadDispatchesToImplementation(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public interface IInterfaceOverloads {
    object M(int value);
    object M(string value);
}
public class InterfaceImplementation : IInterfaceOverloads {
    public object M(int value) { return cleanImplementation(); }
    public object M(string value) { return implementationSource(); }
}
public class InterfaceDispatchHarness {
    public static object Run(IInterfaceOverloads receiver) {
        return finalSink(receiver.M("x"));
    }
}
public class Program {
    public static void Main() { InterfaceDispatchHarness.Run(new InterfaceImplementation()); }
}
`)

	calls := collectCSharpMethodCalls(prog, "Run", "M")
	require.Len(t, calls, 1)
	callee := resolveCSharpTestCallFunction(calls[0])
	require.NotNil(t, callee)
	require.Len(t, callee.Params, 2)
	formal, ok := callee.GetValueById(callee.Params[1])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, formal.GetType().GetTypeKind())
	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "implementationSource")
	require.NotContains(t, flow.GetValues("origin").String(), "cleanImplementation")
}

func TestCSharp_Expression_UnrelatedDerivedOverloadDoesNotBecomeOverride(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class UnrelatedBaseOverloads {
    public virtual object M(int value) { return selectedBaseSource(); }
}
public class UnrelatedDerivedOverloads : UnrelatedBaseOverloads {
    public object M(string value) { return unrelatedDerivedSource(); }
}
public class UnrelatedDispatchHarness {
    public static object Run(UnrelatedBaseOverloads receiver) {
        return finalSink(receiver.M(1));
    }
}
public class Program {
    public static void Main() { UnrelatedDispatchHarness.Run(new UnrelatedDerivedOverloads()); }
}
`)

	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "selectedBaseSource")
	require.NotContains(t, flow.GetValues("origin").String(), "unrelatedDerivedSource",
		"relation-only C# inheritance must not create a name-only method pointer")
}

func TestCSharp_Expression_InterfaceDispatchUsesInheritedImplementation(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public interface IInheritedImplementation {
    object M(string value);
}
public class InheritedImplementationBase {
    public object M(string value) { return inheritedImplementationSource(); }
}
public class InheritedImplementationChild : InheritedImplementationBase, IInheritedImplementation { }
public class InheritedInterfaceDispatch {
    public static void Read(IInheritedImplementation value) { inheritedInterfaceSink(value.M("x")); }
    public static void Main() { Read(new InheritedImplementationChild()); }
}
`)

	flow, err := prog.SyntaxFlowWithError(`inheritedInterfaceSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "inheritedImplementationSource")
}

func TestCSharp_Expression_PartialMethodDefinitionAndImplementationAreOneCallable(t *testing.T) {
	declaration := `
partial class PartialMethodBox {
    partial void M(object value);
    public void Run() { M(partialMethodSource()); }
}`
	implementation := `
partial class PartialMethodBox {
    partial void M(object value) { partialMethodSink(value); }
}`
	for _, test := range []struct {
		name string
		code string
	}{
		{name: "definition before implementation", code: declaration + implementation},
		{name: "implementation before definition", code: implementation + declaration},
	} {
		t.Run(test.name, func(t *testing.T) {
			prog := parseCSharpSemantics(t, test.code+`
public class Program { public static void Main() { new PartialMethodBox().Run(); } }
`)
			flow, err := prog.SyntaxFlowWithError(`partialMethodSink(* #-> as $origin)`)
			require.NoError(t, err)
			require.Contains(t, flow.GetValues("origin").String(), "partialMethodSource")
		})
	}
}

func TestCSharp_Expression_DeclaredBaseTypeControlsOverloadSelection(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DeclaredTypeBase {
    public virtual object M(int value) { return declaredBaseSource(); }
}
public class DeclaredTypeDerived : DeclaredTypeBase {
    public object M(string value) { return wrongDerivedSource(); }
}
public class Program {
    public static void Main() {
        DeclaredTypeBase receiver = new DeclaredTypeDerived();
        declaredTypeSink(receiver.M(1));
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`declaredTypeSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "declaredBaseSource")
	require.NotContains(t, flow.GetValues("origin").String(), "wrongDerivedSource",
		"overload lookup must use the declared Base type, not the initializer's Derived type")
}
