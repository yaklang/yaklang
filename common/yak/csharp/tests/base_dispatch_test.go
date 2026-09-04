package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCSharp_OOP_BaseAccessUsesDirectParentMember(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DispatchBase {
    public object Field;
    public virtual object Method() { return baseMethodSource(); }
    public virtual object Property {
        get { return basePropertySource(); }
        set { baseSetterSink(value); }
    }
}

public class DispatchChild : DispatchBase {
    public override object Method() { return childMethodSource(); }
    public override object Property {
        get { return childPropertySource(); }
        set { childSetterSink(value); }
    }

    public object ReadBaseMethod() { return base.Method(); }
    public object ReadBaseMethodGroup() {
        Func<object> direct = base.Method;
        var alias = direct;
        return alias();
    }
    public object ReadVirtualMethodGroup() {
        Func<object> direct = this.Method;
        var alias = direct;
        return alias();
    }
    public object ReadBaseProperty() { return base.Property; }
    public object ReadBaseField() { return base.Field; }
    public void WriteBaseProperty(object value) { base.Property = value; }
    public void WriteBaseField(object value) { base.Field = value; }
}

public class Program {
    public static void Main() {
        var child = new DispatchChild();
        child.WriteBaseField(fieldSource());
        child.WriteBaseProperty(setterSource());
        methodSink(child.ReadBaseMethod());
        methodGroupSink(child.ReadBaseMethodGroup());
        virtualMethodGroupSink(child.ReadVirtualMethodGroup());
        propertySink(child.ReadBaseProperty());
        fieldSink(child.ReadBaseField());
        virtualSink(child.Method());
    }
}
`)

	assertFlow := func(source, sink string) {
		t.Helper()
		result, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
		require.NoError(t, err)
		require.Contains(t, result.GetValues("origin").String(), "Undefined-"+source,
			"%s must reach %s", source, sink)
	}
	assertNoFlow := func(source, sink string) {
		t.Helper()
		result, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
		require.NoError(t, err)
		require.NotContains(t, result.GetValues("origin").String(), "Undefined-"+source,
			"%s must not reach %s", source, sink)
	}

	assertFlow("baseMethodSource", "methodSink")
	assertNoFlow("childMethodSource", "methodSink")
	assertFlow("baseMethodSource", "methodGroupSink")
	assertNoFlow("childMethodSource", "methodGroupSink")
	assertFlow("childMethodSource", "virtualMethodGroupSink")
	assertFlow("basePropertySource", "propertySink")
	assertNoFlow("childPropertySource", "propertySink")
	assertFlow("fieldSource", "fieldSink")
	assertFlow("setterSource", "baseSetterSink")
	assertFlow("childMethodSource", "virtualSink")

	nonVirtualCallers := map[string]bool{
		"ReadBaseMethod":      false,
		"ReadBaseMethodGroup": false,
		"ReadBaseProperty":    false,
		"WriteBaseProperty":   false,
	}
	baseBlueprint := prog.Program.GetBluePrint("DispatchBase")
	childBlueprint := prog.Program.GetBluePrint("DispatchChild")
	require.NotNil(t, baseBlueprint)
	require.NotNil(t, childBlueprint)
	baseSetter := baseBlueprint.GetNormalMethod("set_Property")
	childMethod := childBlueprint.GetNormalMethod("Method")
	require.NotNil(t, baseSetter)
	require.NotNil(t, childMethod)
	baseSetterTarget := false
	ordinaryVirtualCall := false
	ordinaryVirtualGroupCall := false
	prog.Program.EachFunction(func(function *ssa.Function) {
		function.Build()
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
				if call.IsNonVirtual {
					if _, tracked := nonVirtualCallers[function.GetMethodName()]; tracked {
						nonVirtualCallers[function.GetMethodName()] = true
					}
				}
				calleeValue, exists := call.GetValueById(call.Method)
				if !exists || calleeValue == nil {
					continue
				}
				if reference := calleeValue.GetReference(); reference != nil {
					calleeValue = reference
				}
				if function.GetMethodName() == "WriteBaseProperty" && call.IsNonVirtual &&
					calleeValue.GetId() == baseSetter.GetId() {
					baseSetterTarget = true
				}
				if function.GetMethodName() == "Main" && !call.IsNonVirtual &&
					calleeValue.GetId() == childMethod.GetId() {
					ordinaryVirtualCall = true
				}
				if function.GetMethodName() == "ReadVirtualMethodGroup" && !call.IsNonVirtual &&
					calleeValue.GetId() == childMethod.GetId() {
					ordinaryVirtualGroupCall = true
				}
			}
		}
	})
	require.True(t, nonVirtualCallers["ReadBaseMethod"], "base.Method() must be marked non-virtual")
	require.True(t, nonVirtualCallers["ReadBaseMethodGroup"], "an aliased base.Method delegate call must remain non-virtual")
	require.True(t, nonVirtualCallers["ReadBaseProperty"], "base.Property getter must be marked non-virtual")
	require.True(t, nonVirtualCallers["WriteBaseProperty"], "base.Property setter must be marked non-virtual")
	require.True(t, baseSetterTarget, "base.Property assignment must select DispatchBase.set_Property")
	require.True(t, ordinaryVirtualCall, "ordinary child.Method() call must remain virtual")
	require.True(t, ordinaryVirtualGroupCall, "ordinary child.Method method-group call must remain virtual")
}
