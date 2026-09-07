package antlr4yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestAnonymousFunctionAssignmentKeepsTemplateAndAliasBindingsIndependent(t *testing.T) {
	const source = `
direct = func() { return 1 }
directAlias = direct

holders = [func() { return 2 }]
first = holders[0]
second = holders[0]
`

	engine := New()
	codes, err := engine.Compile(source)
	require.NoError(t, err)
	templates, _, _, _ := snapshotCompiledCodeTemplates(t, codes)

	require.NoError(t, engine.vm.ExecYakCode(context.Background(), source, codes, yakvm.None))

	direct := engine.Var("direct").(*yakvm.Function)
	directAlias := engine.Var("directAlias").(*yakvm.Function)
	require.Same(t, direct, directAlias, "aliasing a bound function must preserve its original binding")
	require.Equal(t, "direct", direct.GetBindName())

	holder := engine.Var("holders").([]interface{})[0].(*yakvm.Function)
	first := engine.Var("first").(*yakvm.Function)
	second := engine.Var("second").(*yakvm.Function)
	require.Empty(t, holder.GetBindName(), "publishing an anonymous function in a container must not bind the shared value")
	require.NotSame(t, holder, first)
	require.NotSame(t, holder, second)
	require.NotSame(t, first, second)
	require.Equal(t, "first", first.GetBindName())
	require.Equal(t, "second", second.GetBindName())

	assertCompiledCodeTemplatesUnchanged(t, templates)
}

func BenchmarkAnonymousFunctionDirectAssignment(b *testing.B) {
	const source = `handler = func() { return 1 }`
	engine := New()
	codes, err := engine.Compile(source)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.vm.ExecYakCode(ctx, source, codes, yakvm.None); err != nil {
			b.Fatal(err)
		}
	}
}
