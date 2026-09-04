package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireDeclaredStaticOrigin(t *testing.T, code, query, expected string, rejected ...string) {
	t.Helper()
	prog := parseCSharpSemantics(t, code)
	flow, err := prog.SyntaxFlowWithError(query)
	require.NoError(t, err)
	origins := flow.GetValues("origin").String()
	require.Contains(t, origins, expected)
	for _, name := range rejected {
		require.NotContains(t, origins, name)
	}
}

func TestCSharp_DeclaredStaticType_CastPreservesConstructorMemberState(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DeclaredMemberBase {
    public object Value = declaredMemberInitializerClean();
}
public class DeclaredMemberDerived : DeclaredMemberBase {
    public DeclaredMemberDerived() { Value = declaredMemberConstructorSource(); }
}
public class Program {
    public static void Main() {
        DeclaredMemberBase value = new DeclaredMemberDerived();
        declaredMemberSink(value.Value);

        DeclaredMemberBase reassigned = null;
        reassigned = new DeclaredMemberDerived();
        reassignedMemberSink(reassigned.Value);
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`declaredMemberSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "declaredMemberConstructorSource")
	require.NotContains(t, flow.GetValues("origin").String(), "declaredMemberInitializerClean",
		"reading through the static-type cast must retain the derived constructor's member state")

	reassignedFlow, err := prog.SyntaxFlowWithError(`reassignedMemberSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, reassignedFlow.GetValues("origin").String(), "declaredMemberConstructorSource",
		"a cast introduced by a later assignment must keep its constructor operand")
	require.NotContains(t, reassignedFlow.GetValues("origin").String(), "declaredMemberInitializerClean")
}

func TestCSharp_DeclaredStaticType_ReassignmentControlsOverloadSelection(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ReassignedReceiverBase {
    public virtual object Select(object value) { return reassignedBaseSource(); }
}
public class ReassignedReceiverDerived : ReassignedReceiverBase {
    public object Select(string value) { return unrelatedDerivedClean(); }
}
public class Program {
    public static void Main() {
        ReassignedReceiverBase receiver = null;
        receiver = new ReassignedReceiverDerived();
        reassignedReceiverSink(receiver.Select("input"));
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`reassignedReceiverSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "reassignedBaseSource")
	require.NotContains(t, flow.GetValues("origin").String(), "unrelatedDerivedClean",
		"a later assignment must not replace the receiver's declared static type")
}

func TestCSharp_DeclaredStaticType_ReassignmentKeepsVirtualDispatch(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ReassignedVirtualBase {
    public virtual object Select(object value) { return virtualBaseClean(); }
}
public class ReassignedVirtualDerived : ReassignedVirtualBase {
    public override object Select(object value) { return virtualDerivedSource(); }
}
public class Program {
    public static void Main() {
        ReassignedVirtualBase receiver = null;
        receiver = new ReassignedVirtualDerived();
        reassignedVirtualSink(receiver.Select("input"));
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`reassignedVirtualSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "virtualDerivedSource",
		"static overload lookup must still allow runtime dispatch to a same-signature override")
}

func TestCSharp_DeclaredStaticType_MethodParameterReassignment(t *testing.T) {
	requireDeclaredStaticOrigin(t, `
public class ParameterBase {
    public virtual object Pick(object value) { return parameterBaseSource(); }
}
public class ParameterDerived : ParameterBase {
    public object Pick(string value) { return parameterDerivedWrong(); }
}
public class Program {
    public static void Run(ParameterBase receiver) {
        receiver = new ParameterDerived();
        parameterSink(receiver.Pick("input"));
    }
    public static void Main() { Run(null); }
}
`, `parameterSink(* #-> as $origin)`, "parameterBaseSource", "parameterDerivedWrong")
}

func TestCSharp_DeclaredStaticType_ClosureParameterReassignment(t *testing.T) {
	t.Run("explicit lambda", func(t *testing.T) {
		requireDeclaredStaticOrigin(t, `
public class LambdaBase {
    public virtual object Pick(object value) { return lambdaBaseSource(); }
}
public class LambdaDerived : LambdaBase {
    public object Pick(string value) { return lambdaDerivedWrong(); }
}
public class Program {
    public static void Main() {
        System.Func<LambdaBase, object> choose = (LambdaBase receiver) => {
            receiver = new LambdaDerived();
            return receiver.Pick("input");
        };
        lambdaSink(choose(null));
    }
}
`, `lambdaSink(* #-> as $origin)`, "lambdaBaseSource", "lambdaDerivedWrong")
	})

	t.Run("local function", func(t *testing.T) {
		requireDeclaredStaticOrigin(t, `
public class LocalFunctionBase {
    public virtual object Pick(object value) { return localFunctionBaseSource(); }
}
public class LocalFunctionDerived : LocalFunctionBase {
    public object Pick(string value) { return localFunctionDerivedWrong(); }
}
public class Program {
    public static void Main() {
        object Choose(LocalFunctionBase receiver) {
            receiver = new LocalFunctionDerived();
            return receiver.Pick("input");
        }
        localFunctionSink(Choose(null));
    }
}
`, `localFunctionSink(* #-> as $origin)`, "localFunctionBaseSource", "localFunctionDerivedWrong")
	})
}

func TestCSharp_DeclaredStaticType_CatchVariableReassignment(t *testing.T) {
	requireDeclaredStaticOrigin(t, `
public class CatchBase {
    public virtual object Pick(object value) { return catchBaseSource(); }
}
public class CatchDerived : CatchBase {
    public object Pick(string value) { return catchDerivedWrong(); }
}
public class Program {
    public static void Main() {
        try { throw new CatchBase(); }
        catch (CatchBase error) {
            error = new CatchDerived();
            catchSink(error.Pick("input"));
        }
    }
}
`, `catchSink(* #-> as $origin)`, "catchBaseSource", "catchDerivedWrong")
}

func TestCSharp_DeclaredStaticType_KnownRefOutWriteback(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class WritebackBase {
    public virtual object Pick(object value) { return writebackBaseSource(); }
}
public class WritebackDerived : WritebackBase {
    public object Pick(string value) { return writebackDerivedWrong(); }
}
public class Program {
    public static void FillOut(out WritebackBase receiver) { receiver = new WritebackDerived(); }
    public static void FillRef(ref WritebackBase receiver) { receiver = new WritebackDerived(); }
    public static void Main() {
        WritebackBase outValue = null;
        FillOut(out outValue);
        declaredOutSink(outValue.Pick("input"));

        WritebackBase refValue = null;
        FillRef(ref refValue);
        declaredRefSink(refValue.Pick("input"));
    }
}
`)
	for _, sink := range []string{"declaredOutSink", "declaredRefSink"} {
		flow, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
		require.NoError(t, err)
		origins := flow.GetValues("origin").String()
		require.Contains(t, origins, "writebackBaseSource")
		require.NotContains(t, origins, "writebackDerivedWrong")
	}
}

func TestCSharp_DeclaredStaticType_FieldReassignment(t *testing.T) {
	requireDeclaredStaticOrigin(t, `
public class FieldBase {
    public virtual object Pick(object value) { return fieldBaseSource(); }
}
public class FieldDerived : FieldBase {
    public object Pick(string value) { return fieldDerivedWrong(); }
}
public class Holder {
    public FieldBase Receiver;
    public void Run() {
        Receiver = new FieldDerived();
        fieldSink(Receiver.Pick("input"));
    }
}
public class Program { public static void Main() { new Holder().Run(); } }
`, `fieldSink(* #-> as $origin)`, "fieldBaseSource", "fieldDerivedWrong")
}

func TestCSharp_DeclaredStaticType_HiddenFieldOwnerGating(t *testing.T) {
	requireDeclaredStaticOrigin(t, `
public class HiddenFieldBase {
    public object Value = hiddenFieldBaseSource();
}
public class HiddenFieldDerived : HiddenFieldBase {
    public new object Value = hiddenFieldDerivedInitializerWrong();
    public HiddenFieldDerived() { Value = hiddenFieldDerivedConstructorWrong(); }
}
public class Program {
    public static void Main() {
        HiddenFieldBase value = new HiddenFieldDerived();
        hiddenFieldSink(value.Value);
    }
}
`, `hiddenFieldSink(* #-> as $origin)`, "hiddenFieldBaseSource",
		"hiddenFieldDerivedInitializerWrong", "hiddenFieldDerivedConstructorWrong")
}
