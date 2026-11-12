package c2ssa

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// hookOnlyFS hides HookFS.ReadFileRaw so filesys.ReadFileRaw falls back to
// ReadFile, which runs the read hooks. It reproduces the pre-optimization code
// path: every planning read goes through the C preprocessor.
type hookOnlyFS struct {
	fi.FileSystem
}

func TestCompileUnitDependenciesMatchPreprocessedForLiteralIncludes(t *testing.T) {
	inner := filesys.NewVirtualFs()
	files := map[string]string{
		"a/main.c":   "#include \"../b/util.h\"\n#include <c/cfg.h>\nint f(void);\n",
		"b/util.h":   "#include \"b.h\"\nint g(void);\n",
		"b/b.h":      "#define B 1\nint b(void);\n",
		"c/cfg.h":    "#include \"../c/extra.h\"\n#define CFG 1\n",
		"c/extra.h":  "int extra(void);\n",
		"d/orphan.c": "int orphan(void);\n",
	}
	for name, body := range files {
		require.NoError(t, inner.WriteFile(name, []byte(body), 0o644))
	}

	units := []*ssa.CompileUnit{
		{Key: "dir:a", Path: "a", Files: []string{"a/main.c"}},
		{Key: "dir:b", Path: "b", Files: []string{"b/util.h", "b/b.h"}},
		{Key: "dir:c", Path: "c", Files: []string{"c/cfg.h", "c/extra.h"}},
		{Key: "dir:d", Path: "d", Files: []string{"d/orphan.c"}},
	}

	// Optimized path: raw reads, no hooks.
	optimized := new(SSABuilder).CompileUnitDependencies(inner, units)

	// Reference path: force every read through the hooked ReadFile.
	hooked := filesys.NewHookFS(inner)
	hookCalls := 0
	hooked.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".c", ".h"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			hookCalls++
			// A real preprocessor rewrites macro bodies but leaves literal
			// #include lines intact, which is all planning consumes.
			return data, nil
		},
	})
	reference := new(SSABuilder).CompileUnitDependencies(hookOnlyFS{FileSystem: hooked}, units)

	require.Positive(t, hookCalls, "reference run must actually go through the hook")
	require.Equal(t, sortEdges(reference), sortEdges(optimized),
		"raw-source planning must yield the same include edges")
}

func sortEdges(edges []ssa.UnitRef) []ssa.UnitRef {
	out := append([]ssa.UnitRef(nil), edges...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Raw < out[j].Raw
	})
	return out
}
