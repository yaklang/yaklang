package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
)

func TestCSharpEdgeSnippets_Frontend(t *testing.T) {
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "namespace using class",
			src:  `using System; namespace N { public class C { } }`,
		},
		{
			name: "struct interface enum delegate",
			src: `
using System;
public struct S { public int X; }
public interface I { void M(); }
public enum E { A, B }
public delegate void D(int x);
`,
		},
		{
			name: "nested types",
			src:  `public class Outer { public class Inner { } public struct Box { } public enum K { A } }`,
		},
		{
			name: "generics and constraints",
			src:  `public class Box<T> where T : class, new() { public T Value; }`,
		},
		{
			name: "property indexer event",
			src: `
using System;
public class W {
    public int X { get; set; }
    public int this[int i] { get { return i; } }
    public event EventHandler Changed;
}
`,
		},
		{
			name: "ref out params in",
			src:  `public class C { public void M(ref int a, out int b, in int c, params int[] rest) { b = a + c; } }`,
		},
		{
			name: "attributes",
			src:  `[System.Obsolete] public class C { [System.Obsolete] public void M() {} }`,
		},
		{
			name: "async await",
			src: `
using System.Threading.Tasks;
public class C {
    public async Task<int> M() { await Task.Yield(); return 1; }
}
`,
		},
		{
			name: "yield return",
			src: `
using System.Collections.Generic;
public class C {
    public IEnumerable<int> M() { yield return 1; yield break; }
}
`,
		},
		{
			name: "interpolated string",
			src:  `public class C { public string M(string n) { return $"hi {n}"; } }`,
		},
		{
			name: "linq query",
			src: `
using System.Linq;
public class C {
    public object M(int[] xs) { return from x in xs where x > 0 select x; }
}
`,
		},
		{
			name: "preprocessor elif false branch",
			src: `
#define A
class C {
#if B
    int skippedB = 1;
#elif A
    int keptA = 2;
#else
    int skippedElse = 3;
#endif
}
`,
		},
		{
			name: "constructor and method",
			src:  `public class C { public C(int x) {} public int M() { return 1; } }`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			ast, err := csharp2ssa.Frontend(tc.src, cache)
			require.NoError(t, err, "parse AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration, "parse took too long for %s", tc.name)
			_, ok := ast.(csharpparser.ICompilation_unitContext)
			require.True(t, ok)
		})
	}
}
