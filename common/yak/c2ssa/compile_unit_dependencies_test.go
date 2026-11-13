package c2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func dependencyTestUnits() []*ssa.CompileUnit {
	return []*ssa.CompileUnit{
		{Key: "dir:a", Path: "a", Files: []string{"a/main.c"}},
		{Key: "dir:b", Path: "b", Files: []string{"b/util.h"}},
	}
}

// Planning only needs the textual #include lines. If it went through the C
// preprocessing hook it would expand the whole include closure for every TU,
// which is the dominant cost on macro-heavy trees (nuttx).
func TestCompileUnitDependenciesReadsRawSource(t *testing.T) {
	inner := filesys.NewVirtualFs()
	require.NoError(t, inner.WriteFile("a/main.c", []byte("#include \"b/util.h\"\nint f(void);\n"), 0o644))
	require.NoError(t, inner.WriteFile("b/util.h", []byte("int g(void);\n"), 0o644))

	hookCalls := 0
	fs := filesys.NewHookFS(inner)
	fs.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".c"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			hookCalls++
			return []byte("/* preprocessed away */\n"), nil
		},
	})

	edges := new(SSABuilder).CompileUnitDependencies(fs, dependencyTestUnits())

	require.Len(t, edges, 1)
	require.Equal(t, "dir:a", edges[0].From)
	require.Equal(t, "dir:b", edges[0].To)
	require.Zero(t, hookCalls, "dependency planning must not run the C preprocessor")
}

// A macro operand cannot be resolved from raw text, so THAT file must still go
// through preprocessing while literal includes keep using the raw path.
func TestCompileUnitDependenciesExpandsMacroInclude(t *testing.T) {
	inner := filesys.NewVirtualFs()
	require.NoError(t, inner.WriteFile("a/main.c", []byte("#define HDR \"b/util.h\"\n#include HDR\n"), 0o644))
	require.NoError(t, inner.WriteFile("b/util.h", []byte("int g(void);\n"), 0o644))

	hookCalls := 0
	fs := filesys.NewHookFS(inner)
	fs.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".c"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			hookCalls++
			// Stand-in for PreprocessTU: only the expanded form carries a literal path.
			return []byte("#include \"b/util.h\"\n"), nil
		},
	})

	edges := new(SSABuilder).CompileUnitDependencies(fs, dependencyTestUnits())

	require.Equal(t, 1, hookCalls, "macro include must fall back to the preprocessed source")
	require.Len(t, edges, 1)
	require.Equal(t, "dir:a", edges[0].From)
	require.Equal(t, "dir:b", edges[0].To)
}

func TestHasMacroInclude(t *testing.T) {
	require.False(t, hasMacroInclude("#include \"a.h\"\n#include <b.h>\n"))
	require.False(t, hasMacroInclude("#define X 1\nint x;\n"))
	require.True(t, hasMacroInclude("#include HDR\n"))
	require.True(t, hasMacroInclude("#define H \"x.h\"\n#include H\n"))
	require.False(t, hasMacroInclude("#include /* comment */\n"))
}
