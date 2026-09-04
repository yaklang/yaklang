package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCSharp_OOP_DerivedConstructorBodyFlow(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public BaseRecord() { }
}

public class DerivedRecord : BaseRecord {
    public string Value;

    public DerivedRecord() : base() {
        Value = source();
    }
}

public class Program {
    public static void Main(string[] args) {
        var item = new DerivedRecord();
        sink(item.Value);
    }
}
`)
	derived := prog.Program.GetBluePrint("DerivedRecord")
	require.NotNil(t, derived)
	constructor, ok := ssa.ToFunction(derived.Constructor)
	require.True(t, ok, "derived blueprint must retain its own concrete constructor")
	require.Equal(t, "DerivedRecord", constructor.GetMethodName())

	result, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("source"))
	require.NotEmpty(t, result.GetValues("origin"), "derived constructor body must reach the constructed instance")
}

func TestCSharp_OOP_BaseConstructorMutatesDerivedInstance(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class BaseRecord {
    public string Value;

    public BaseRecord() {
        Value = baseSource();
    }
}

public class DerivedRecord : BaseRecord {
    public DerivedRecord() : base() { }
}

public class Program {
    public static void Main(string[] args) {
        sink(new DerivedRecord().Value);
    }
}
`)

	result, err := prog.SyntaxFlowWithError(`baseSource() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("source"))
	require.NotEmpty(t, result.GetValues("origin"), "base constructor writes must reach the derived instance")
}

func TestCSharp_OOP_UnresolvedINameBaseIsInterface(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class DisposableResource : IDisposable {
    public void Dispose() { }
}
`)
	resource := prog.Program.GetBluePrint("DisposableResource")
	require.NotNil(t, resource)
	require.Empty(t, resource.GetParentBlueprint(), "an unresolved IName entry must not become a base class")
	require.Len(t, resource.GetInterfaceBlueprint(), 1)
	require.Equal(t, "IDisposable", resource.GetInterfaceBlueprint()[0].Name)
	require.True(t, resource.GetInterfaceBlueprint()[0].IsInterface())
}

func TestCSharp_OOP_KnownBlueprintBaseIsInterface(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public interface ResourceContract {
    void Release();
}

public class ManagedResource : ResourceContract {
    public void Release() { }
}
`)
	resource := prog.Program.GetBluePrint("ManagedResource")
	require.NotNil(t, resource)
	require.Empty(t, resource.GetParentBlueprint(), "a declared interface must not become a base class")
	require.Len(t, resource.GetInterfaceBlueprint(), 1)
	require.Equal(t, "ResourceContract", resource.GetInterfaceBlueprint()[0].Name)
	require.True(t, resource.GetInterfaceBlueprint()[0].IsInterface())
}
