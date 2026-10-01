package yakgrpc

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type languageSuggestionMethodBuilder struct {
	name       string
	returnType ssa.TypeKind
}

func (b *languageSuggestionMethodBuilder) Build(typ ssa.Type, name string) *ssa.Function {
	if (name != "OwnedMethod" && name != b.name+"Only") || typ.GetTypeKind() != ssa.StringTypeKind {
		return nil
	}
	var result ssa.Type
	if b.returnType == ssa.StringTypeKind {
		result = ssa.CreateStringType()
	} else {
		result = ssa.CreateBooleanType()
	}
	return ssa.NewFunctionWithType(b.name+"."+name, ssa.NewFunctionType(
		b.name+"."+name, []ssa.Type{typ, ssa.CreateNumberType()}, result, false,
	))
}

func (b *languageSuggestionMethodBuilder) GetMethodNames(typ ssa.Type) []string {
	if typ.GetTypeKind() == ssa.StringTypeKind {
		return []string{"OwnedMethod", b.name + "Only"}
	}
	return nil
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionMethodBuilderProgramIsolation(t *testing.T) {
	const code = "completionAuditGate()\nreceiver = \"value\"\nresult = receiver.OwnedMethod(1)"
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseParses := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseParses()
	type parsed struct {
		builder *languageSuggestionMethodBuilder
		program *ssaapi.Program
		err     error
	}
	results := make(chan parsed, 2)
	builders := []*languageSuggestionMethodBuilder{
		{name: "stringOwner", returnType: ssa.StringTypeKind},
		{name: "booleanOwner", returnType: ssa.BooleanTypeKind},
	}
	for _, builder := range builders {
		go func(builder *languageSuggestionMethodBuilder) {
			program, err := ssaapi.Parse(code,
				ssaapi.WithEnableCache(false),
				ssaapi.WithExternMethod(builder),
				ssaapi.WithExternValue(map[string]any{"completionAuditGate": func() {}}),
				ssaapi.WithExternBuildValueHandler("completionAuditGate", func(_ *ssa.FunctionBuilder, name string, _ any) ssa.Value {
					// Both Config.init calls have completed before either parser
					// can resolve a method. A process-global builder necessarily
					// makes at least one of these programs use the wrong owner.
					entered <- struct{}{}
					<-release
					return ssa.NewFunctionWithType(name, ssa.NewFunctionType(name, nil, ssa.CreateNullType(), false))
				}),
			)
			results <- parsed{builder: builder, program: program, err: err}
		}(builder)
	}
	for i := 0; i < len(builders); i++ {
		select {
		case <-entered:
		case early := <-results:
			require.NoError(t, early.err)
			t.Fatalf("%s completed before reaching the parse barrier", early.builder.name)
		}
	}
	releaseParses()
	for i := 0; i < len(builders); i++ {
		result := <-results
		require.NoError(t, result.err)
		require.NotNil(t, result.program)
		values := result.program.Ref("result")
		require.Len(t, values, 1)
		require.Equal(t, result.builder.returnType, values[0].GetTypeKind(), "program %s used another program's method return type", result.builder.name)
		receivers := result.program.Ref("receiver")
		require.Len(t, receivers, 1)
		method := ssa.GetMethod(ssaapi.GetBareType(receivers[0].GetType()), "OwnedMethod", true)
		require.NotNil(t, method)
		require.Equal(t, fmt.Sprintf("%s.OwnedMethod", result.builder.name), method.GetName())
		functionType, ok := ssa.ToFunctionType(method.GetType())
		require.True(t, ok)
		require.Equal(t, result.builder.returnType, functionType.ReturnType.GetTypeKind())
		require.Len(t, functionType.Parameter, 2)
		require.Equal(t, ssa.StringTypeKind, functionType.Parameter[0].GetTypeKind())
		require.Equal(t, ssa.NumberTypeKind, functionType.Parameter[1].GetTypeKind())
		require.True(t, functionType.IsMethod)
		keys := result.program.Program.GetAllKey(ssaapi.GetBareType(receivers[0].GetType()))
		require.Contains(t, keys, "OwnedMethod")
		require.Contains(t, keys, result.builder.name+"Only", "method suggestions must use the same program configuration")
		for _, other := range builders {
			if other != result.builder {
				require.NotContains(t, keys, other.name+"Only", "method suggestions must not include another program's builder")
			}
		}
	}
}
