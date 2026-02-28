package java2ssa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestResolveJavaMethodOverloadByType(t *testing.T) {
	y := newJavaBuilderForUnitTest()
	prog := ssa.NewProgram(context.Background(), "java-overload-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	class := ssa.NewBlueprint("Demo")

	intMethod := prog.NewFunction("pick_int")
	stringMethod := prog.NewFunction("pick_string")

	y.registerJavaMethodOverload(class, "pick", intMethod, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)
	y.registerJavaMethodOverload(class, "pick", stringMethod, []javaParamSignature{
		{key: "string", typ: ssa.CreateStringType()},
	}, false)

	fb := newJavaFunctionBuilderForUnitTest()
	argInt := fb.EmitConstInst(1)
	argString := fb.EmitConstInst("a")
	require.Equal(t, ssa.NumberTypeKind, argInt.GetType().GetTypeKind())
	require.Equal(t, ssa.StringTypeKind, argString.GetType().GetTypeKind())

	intScore, _, ok := y.scoreJavaCallableCandidate(y.methodOverloads[class]["pick"][0], []ssa.Value{argInt})
	require.True(t, ok)
	stringScore, _, ok := y.scoreJavaCallableCandidate(y.methodOverloads[class]["pick"][1], []ssa.Value{argInt})
	require.False(t, ok)
	require.Greater(t, intScore, stringScore)

	require.Equal(t, intMethod, y.resolveJavaMethodOverload(class, "pick", []ssa.Value{argInt}))
	require.Equal(t, stringMethod, y.resolveJavaMethodOverload(class, "pick", []ssa.Value{argString}))
}

func TestResolveJavaConstructorOverloadByType(t *testing.T) {
	y := newJavaBuilderForUnitTest()
	prog := ssa.NewProgram(context.Background(), "java-ctor-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	class := ssa.NewBlueprint("DemoCtor")

	intCtor := prog.NewFunction("ctor_int")
	stringCtor := prog.NewFunction("ctor_string")

	y.registerJavaConstructorOverload(class, intCtor, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)
	y.registerJavaConstructorOverload(class, stringCtor, []javaParamSignature{
		{key: "string", typ: ssa.CreateStringType()},
	}, false)

	fb := newJavaFunctionBuilderForUnitTest()
	argInt := fb.EmitConstInst(2)
	argString := fb.EmitConstInst("x")
	require.Equal(t, ssa.NumberTypeKind, argInt.GetType().GetTypeKind())
	require.Equal(t, ssa.StringTypeKind, argString.GetType().GetTypeKind())

	intScore, _, ok := y.scoreJavaCallableCandidate(y.constructorOverloads[class][0], []ssa.Value{argInt})
	require.True(t, ok)
	stringScore, _, ok := y.scoreJavaCallableCandidate(y.constructorOverloads[class][1], []ssa.Value{argInt})
	require.False(t, ok)
	require.Greater(t, intScore, stringScore)

	require.Equal(t, intCtor, y.resolveJavaConstructorOverload(class, []ssa.Value{argInt}))
	require.Equal(t, stringCtor, y.resolveJavaConstructorOverload(class, []ssa.Value{argString}))
}

func TestJavaStableCallableNameDeterministic(t *testing.T) {
	class := ssa.NewBlueprint("StableClass")
	params := []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
		{key: "string", typ: ssa.CreateStringType()},
	}

	y1 := newJavaBuilderForUnitTest()
	y2 := newJavaBuilderForUnitTest()

	name1 := y1.javaStableCallableName("method", class, "mix", params, false, nil)
	name2 := y2.javaStableCallableName("method", class, "mix", params, false, nil)
	require.Equal(t, name1, name2)

	name3 := y1.javaStableCallableName("method", class, "mix", params, false, nil)
	require.NotEqual(t, name1, name3)
}

func TestResolveJavaMethodOverloadPreferFixedOverVariadic(t *testing.T) {
	y := newJavaBuilderForUnitTest()
	prog := ssa.NewProgram(context.Background(), "java-overload-variadic-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	class := ssa.NewBlueprint("DemoVariadic")

	fixed := prog.NewFunction("pick_fixed")
	variadic := prog.NewFunction("pick_variadic")

	y.registerJavaMethodOverload(class, "pick", fixed, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)
	y.registerJavaMethodOverload(class, "pick", variadic, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, true)

	fb := newJavaFunctionBuilderForUnitTest()
	arg := fb.EmitConstInst(7)

	require.Equal(t, fixed, y.resolveJavaMethodOverload(class, "pick", []ssa.Value{arg}))
}

func TestResolveJavaMethodOverloadFromParentBlueprint(t *testing.T) {
	y := newJavaBuilderForUnitTest()
	prog := ssa.NewProgram(context.Background(), "java-overload-parent-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	parent := ssa.NewBlueprint("Parent")
	child := ssa.NewBlueprint("Child")
	child.AddParentBlueprint(parent)

	parentMethod := prog.NewFunction("pick_parent")
	childMethod := prog.NewFunction("pick_child")
	y.registerJavaMethodOverload(parent, "pick", parentMethod, []javaParamSignature{
		{key: "string", typ: ssa.CreateStringType()},
	}, false)
	y.registerJavaMethodOverload(child, "pick", childMethod, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)

	fb := newJavaFunctionBuilderForUnitTest()
	argInt := fb.EmitConstInst(1)
	argString := fb.EmitConstInst("x")

	require.Equal(t, childMethod, y.resolveJavaMethodOverload(child, "pick", []ssa.Value{argInt}))
	require.Equal(t, parentMethod, y.resolveJavaMethodOverload(child, "pick", []ssa.Value{argString}))
}

func TestResolveJavaMethodOverloadTieBreakByDeclarationOrder(t *testing.T) {
	y := newJavaBuilderForUnitTest()
	prog := ssa.NewProgram(context.Background(), "java-overload-order-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	class := ssa.NewBlueprint("DemoOrder")

	first := prog.NewFunction("pick_first")
	second := prog.NewFunction("pick_second")

	y.registerJavaMethodOverload(class, "pick", first, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)
	y.registerJavaMethodOverload(class, "pick", second, []javaParamSignature{
		{key: "number", typ: ssa.CreateNumberType()},
	}, false)

	fb := newJavaFunctionBuilderForUnitTest()
	arg := fb.EmitConstInst(3)
	require.Equal(t, first, y.resolveJavaMethodOverload(class, "pick", []ssa.Value{arg}))
}

func newJavaBuilderForUnitTest() *singleFileBuilder {
	return &singleFileBuilder{
		methodOverloads:      make(map[*ssa.Blueprint]map[string][]*javaCallableCandidate),
		constructorOverloads: make(map[*ssa.Blueprint][]*javaCallableCandidate),
		stableNameCollision:  make(map[string]int),
	}
}

func newJavaFunctionBuilderForUnitTest() *ssa.FunctionBuilder {
	prog := ssa.NewProgram(context.Background(), "java-arg-builder-test", ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	editor := memedit.NewMemEditorWithFileUrl("class A {}", "/tmp/Test.java")
	prog.PushEditor(editor)
	builder := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	builder.SetEditor(editor)
	return builder
}
