package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestCSharp_TypeFamilies(t *testing.T) {
	tests := []struct {
		name string
		decl string
		kind ssa.TypeKind
	}{
		{name: "number", decl: "int target = source();", kind: ssa.NumberTypeKind},
		{name: "string", decl: "string target = source();", kind: ssa.StringTypeKind},
		{name: "boolean", decl: "bool target = source();", kind: ssa.BooleanTypeKind},
		{name: "byte", decl: "byte target = source();", kind: ssa.ByteTypeKind},
		{name: "array", decl: "int[] target = source();", kind: ssa.SliceTypeKind},
		{name: "generic list", decl: "List<int> target = source();", kind: ssa.SliceTypeKind},
		{name: "generic map", decl: "Dictionary<string, int> target = source();", kind: ssa.MapTypeKind},
		{name: "nullable", decl: "int? target = source();", kind: ssa.NumberTypeKind},
		{name: "task unwrap", decl: "Task<string> target = source();", kind: ssa.StringTypeKind},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog := parseCSharpSemantics(t, `
using System.Collections.Generic;
using System.Threading.Tasks;

public class TypeSamples {
    public static void Main(string[] args) {
        `+test.decl+`
        sink(target);
    }
}
`)
			values := prog.Ref("target")
			require.NotEmpty(t, values)
			matched := false
			for _, value := range values {
				if value.GetTypeKind() == test.kind {
					matched = true
					break
				}
			}
			require.Truef(t, matched, "target does not contain expected type kind %v: %v", test.kind, values)
		})
	}
}

func TestCSharp_CustomTypePreservedAcrossNamespaceUsing(t *testing.T) {
	prog := parseCSharpSemantics(t, `
namespace Demo.Data {
    public class Model { }
}
namespace Demo.App {
    using Demo.Data;

    public class Program {
        public static void Main(string[] args) {
            Model target = source();
            sink(target);
        }
    }
}
`)
	values := prog.Ref("target")
	require.NotEmpty(t, values)
	matched := false
	for _, value := range values {
		if value.GetTypeKind() == ssa.ClassBluePrintTypeKind {
			matched = true
			break
		}
	}
	require.True(t, matched, "declared custom type must remain a class blueprint")
}

func TestCSharp_GenericUsingAliasPreservesTypeArguments(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using Names = System.Collections.Generic.List<string>;

public class AliasSamples {
    public static void Run(Names target) {
        sink(target);
    }
}
`)

	values := prog.Ref("target")
	require.NotEmpty(t, values)
	for _, value := range values {
		if !value.IsParameter() {
			continue
		}
		require.Equal(t, ssa.SliceTypeKind, value.GetTypeKind())
		require.Contains(t, value.GetType().String(), "string",
			"the alias target's generic argument must not degrade to any")
		return
	}
	require.Fail(t, "target parameter was not emitted")
}

func TestCSharp_NestedGenericQualifierDoesNotLeakArguments(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Outer<T> {
    public class List { }
}

public class NestedSamples {
    public static void Run(Outer<int>.List target) {
        sink(target);
    }
}
`)

	for _, value := range prog.Ref("target") {
		if !value.IsParameter() {
			continue
		}
		bp, ok := ssa.ToBluePrintType(ssaapi.GetBareType(value.GetType()))
		require.True(t, ok, "nested source type must remain a blueprint: %s", value.GetType())
		require.Equal(t, "Outer$List", bp.Name)
		return
	}
	require.Fail(t, "target parameter was not emitted")
}

func requireCSharpBlueprintParameter(t *testing.T, prog *ssaapi.Program, name, fullName string) *ssa.Blueprint {
	t.Helper()
	for _, value := range prog.Ref(name) {
		if !value.IsParameter() {
			continue
		}
		bp, ok := ssa.ToBluePrintType(ssaapi.GetBareType(value.GetType()))
		require.True(t, ok, "%s must remain a source blueprint: %s", name, value.GetType())
		require.Contains(t, bp.GetFullTypeNames(), fullName)
		return bp
	}
	require.Failf(t, "parameter was not emitted", "missing parameter %s", name)
	return nil
}

func TestCSharp_SourceTypesWinOverBCLContainerShorthand(t *testing.T) {
	t.Run("fully qualified BCL names", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class Consumer {
    public static void Run(
        System.Collections.Generic.List<string> bclList,
        System.Threading.Tasks.Task<string> bclTask,
        System.Collections.Generic.Dictionary<string, int> bclDictionary) { }
}
`)

		want := map[string]ssa.TypeKind{
			"bclList":       ssa.SliceTypeKind,
			"bclTask":       ssa.StringTypeKind,
			"bclDictionary": ssa.MapTypeKind,
		}
		for name, kind := range want {
			matched := false
			for _, value := range prog.Ref(name) {
				if value.IsParameter() && value.GetTypeKind() == kind {
					matched = true
					break
				}
			}
			require.Truef(t, matched, "%s must lower to BCL type kind %v", name, kind)
		}
	})

	t.Run("qualified source names", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
namespace Vendor {
    public class List<T> { }
    public class Task<T> { }
    public class Dictionary<TKey, TValue> { }
}

public class Consumer {
    public static void Run(
        Vendor.List<string> vendorList,
        Vendor.Task<string> vendorTask,
        Vendor.Dictionary<string, int> vendorDictionary) { }
}
`)

		requireCSharpBlueprintParameter(t, prog, "vendorList", "Vendor.List")
		requireCSharpBlueprintParameter(t, prog, "vendorTask", "Vendor.Task")
		requireCSharpBlueprintParameter(t, prog, "vendorDictionary", "Vendor.Dictionary")
	})

	t.Run("using namespace source name", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
namespace Custom {
    public class List<T> { }
}
namespace App {
    using Custom;

    public class Consumer {
        public static void Run(List<string> customList) { }
    }
}
`)

		requireCSharpBlueprintParameter(t, prog, "customList", "Custom.List")
	})

	t.Run("qualified namespace alias", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
namespace Custom {
    public class Payload { }
}
namespace App {
    using C = Custom;

    public class Consumer {
        public static void Run(C::Payload payload) { }
    }
}
`)

		requireCSharpBlueprintParameter(t, prog, "payload", "Custom.Payload")
	})

	t.Run("unknown qualified names keep distinct identities", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class Consumer {
    public static void Run(A.Widget leftWidget, B.Widget rightWidget) { }
}
`)

		left := requireCSharpBlueprintParameter(t, prog, "leftWidget", "A.Widget")
		right := requireCSharpBlueprintParameter(t, prog, "rightWidget", "B.Widget")
		require.NotSame(t, left, right, "qualified unknown types must not share one blueprint")
		require.NotContains(t, left.GetFullTypeNames(), "B.Widget")
		require.NotContains(t, right.GetFullTypeNames(), "A.Widget")
	})
}

func csharpSliceDepth(typ ssa.Type) int {
	depth := 0
	for typ != nil && typ.GetTypeKind() == ssa.SliceTypeKind {
		object, ok := typ.(*ssa.ObjectType)
		if !ok || object.FieldType == nil {
			break
		}
		depth++
		typ = object.FieldType
	}
	return depth
}

func requireCSharpRefSliceDepth(t *testing.T, prog *ssaapi.Program, name string, want int) {
	t.Helper()
	for _, value := range prog.Ref(name) {
		if depth := csharpSliceDepth(ssaapi.GetBareType(value.GetType())); depth == want {
			return
		}
	}
	require.Failf(t, "array rank was not preserved", "%s want depth=%d refs=%s", name, want, prog.Ref(name))
}

func TestCSharp_ArrayRankPreservedForFieldsParametersAndLocals(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class ArrayShapes {
    public int[,] matrixField;
    public int[,,][] mixedField;
    public int[][] jaggedField;

    public static void Run(int[,] matrixParam, int[,,][] mixedParam, int[][] jaggedParam) {
        int[,] matrixLocal = sourceMatrix();
        int[,,][] mixedLocal = sourceMixed();
        int[][] jaggedLocal = sourceJagged();
        sink(matrixLocal, mixedLocal, jaggedLocal);
    }
}
`)

	blueprint := prog.Program.GetBluePrint("ArrayShapes")
	require.NotNil(t, blueprint)
	for name, depth := range map[string]int{
		"matrixField": 2,
		"mixedField":  4,
		"jaggedField": 2,
	} {
		member := blueprint.GetNormalMember(name)
		require.NotNil(t, member, "missing field %s", name)
		require.Equal(t, depth, csharpSliceDepth(member.GetType()), "field %s", name)
	}

	for name, depth := range map[string]int{
		"matrixParam": 2,
		"mixedParam":  4,
		"jaggedParam": 2,
		"matrixLocal": 2,
		"mixedLocal":  4,
		"jaggedLocal": 2,
	} {
		requireCSharpRefSliceDepth(t, prog, name, depth)
	}
}
