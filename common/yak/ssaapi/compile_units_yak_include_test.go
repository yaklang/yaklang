package ssaapi

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// Control AST completion order while preserving the production
// prehandle -> Clearup -> deferred body lifecycle, including multiple batches.
func parseYakFilesInOrder(t *testing.T, files map[string]string, batches [][]string, options ...ssaconfig.Option) *Program {
	t.Helper()
	vf := filesys.NewVirtualFs()
	for path, code := range files {
		vf.AddFile(path, code)
	}
	options = append([]ssaconfig.Option{WithFileSystem(vf), WithLanguage(ssaconfig.Yak), WithMemory()}, options...)
	cfg, err := DefaultConfig(options...)
	require.NoError(t, err)
	prog, builder, err := cfg.init(vf, len(files))
	require.NoError(t, err)
	cfg.LanguageBuilder.InitHandler(builder)
	defer cfg.LanguageBuilder.Clearup()
	for _, order := range batches {
		prog.SetPreHandler(true)
		env := &prehandleBatchEnv{
			c: cfg, prog: prog, builder: builder, filesystem: vf,
			fileUnits: make(map[string]string), preHandlerFileSet: make(map[string]struct{}),
			handlerFileSet: make(map[string]struct{}), preHandlerProcess: func() {}, concurrency: 1,
		}
		for _, path := range order {
			env.fileUnits[path] = "dir:" + strings.Split(path, "/")[0]
			env.preHandlerFileSet[path] = struct{}{}
			env.handlerFileSet[path] = struct{}{}
			ast, err := cfg.LanguageBuilder.ParseAST(files[path], cfg.LanguageBuilder.GetAntlrCache())
			require.NoError(t, err)
			env.prehandleFile(&ssareducer.FileContent{Path: path, Content: []byte(files[path]), AST: ast})
		}
		require.NoError(t, env.astErr)
		cfg.LanguageBuilder.AfterPreHandlerProject(builder)
		cfg.LanguageBuilder.Clearup()
		prog.SetPreHandler(false)
		prog.RunDeferredBuilds()
	}
	prog.Finish()
	return NewProgram(prog, cfg)
}

func requireYakDumpValues(t *testing.T, prog *Program, expected ...string) {
	t.Helper()
	result, err := prog.SyntaxFlowWithError(`dump(* as $value)`)
	require.NoError(t, err)
	var values []string
	for _, value := range result.GetValues("value") {
		values = append(values, value.String())
	}
	sort.Strings(values)
	sort.Strings(expected)
	require.Equal(t, expected, values)
}

func TestYakIncludeIndependentOfFileOrder(t *testing.T) {
	files := map[string]string{
		"a/a.yak": `include "b/b.yak"; dump(b)`,
		"b/b.yak": `b = 3`,
		"c/c.yak": `include "./b/b.yak"; dump(b + 1)`,
	}
	for _, order := range [][]string{
		{"a/a.yak", "b/b.yak", "c/c.yak"},
		{"b/b.yak", "a/a.yak", "c/c.yak"},
		{"c/c.yak", "b/b.yak", "a/a.yak"},
	} {
		for _, split := range []bool{false, true} {
			name := strings.Join(order, ",")
			if split {
				name += "/separate-batches"
			}
			t.Run(name, func(t *testing.T) {
				batches := [][]string{order}
				if split {
					batches = [][]string{{order[0]}, {order[1]}, {order[2]}}
				}
				prog := parseYakFilesInOrder(t, files, batches)
				require.Len(t, prog.Ref("b"), 1, "one source file must have one definition")
				requireYakDumpValues(t, prog, "3", "4")
			})
		}
	}
}

func TestYakIncludeNestedAfterLeafRoot(t *testing.T) {
	files := map[string]string{
		"b/leaf.yak": `b = 3`,
		// This file is only discovered by include, not by root registration.
		"m/middle.yaklang": `include "../b/leaf.yak"; c = b + 1; dump(b)`,
		"a/main.yak":       `include "m/middle.yaklang"; dump(c)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"b/leaf.yak"}, {"a/main.yak"}})
	require.Len(t, prog.Ref("b"), 1)
	require.Contains(t, prog.Ref("b")[0].GetRange().GetEditor().GetUrl(), "b/leaf.yak")
	requireYakDumpValues(t, prog, "3", "4")
}

func TestYakIncludeEqualSourcesKeepDistinctFiles(t *testing.T) {
	files := map[string]string{
		"a/leaf.yak": `b = 3`, "z/leaf.yak": `b = 3`,
		"a/main.yak": `include "a/leaf.yak"; dump(b)`,
		"z/main.yak": `include "z/leaf.yak"; dump(b)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"a/main.yak", "z/main.yak", "a/leaf.yak", "z/leaf.yak"}})
	values := prog.Ref("b")
	require.Len(t, values, 2)
	require.NotEqual(t, values[0].GetId(), values[1].GetId())
	requireYakDumpValues(t, prog, "3", "3")
}

func TestYakIncludePreservesRootGlobalsAndFileSnapshot(t *testing.T) {
	files := map[string]string{
		"a/globals.yak":   `shared = 7; b = 3`,
		"a/overwrite.yak": `dump(shared); b = 9; dump(b)`,
		"c/main.yak":      `include "a/globals.yak"; dump(b)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"a/globals.yak", "a/overwrite.yak", "c/main.yak"}})
	requireYakDumpValues(t, prog, "7", "9", "3")
}

func TestYakIncludePreservesLocalPrecedence(t *testing.T) {
	for _, code := range []string{`b = 3`, `read(b)`} {
		files := map[string]string{
			"leaf.yak": code,
			"main.yak": `b = 9; before(b); include "leaf.yak"; after(b)`,
		}
		prog := parseYakFilesInOrder(t, files, [][]string{{"main.yak"}})
		result, err := prog.SyntaxFlowWithError(`before(* as $before); after(* as $after)`)
		require.NoError(t, err)
		before, after := result.GetValues("before"), result.GetValues("after")
		require.Len(t, before, 1)
		require.Len(t, after, 1)
		require.Equal(t, "9", after[0].String())
		require.Equal(t, before[0].GetId(), after[0].GetId())
	}
}

func TestYakIncludeMostRecentFallbackWins(t *testing.T) {
	files := map[string]string{
		"first.yak": `b = 3`, "second.yak": `b = 4`,
		"main.yak": `include "first.yak"; dump(b); include "second.yak"; dump(b)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"main.yak"}})
	requireYakDumpValues(t, prog, "3", "4")
}

func TestYakIncludeOwnDefinitionsPrecedeNestedIncludes(t *testing.T) {
	files := map[string]string{
		"leaf.yak":    `b = 3`,
		"library.yak": `b = 9; include "leaf.yak"; dump(b)`,
		"main.yak":    `include "library.yak"; dump(b)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"main.yak"}, {"library.yak"}})
	result, err := prog.SyntaxFlowWithError(`dump(* as $value) as $call`)
	require.NoError(t, err)
	require.Len(t, result.GetValues("call"), 2, "both the file and its caller must execute")
	values := result.GetValues("value")
	require.Len(t, values, 1, "both calls reference the same file definition")
	require.Equal(t, "9", values[0].String())
}

func TestYakIncludePreservesParametersAndExportedCalls(t *testing.T) {
	files := map[string]string{
		"leaf.yak": `b = 3; exported = () => { return 7 }`,
		"main.yak": `f = (b) => { before(b); include "leaf.yak"; after(b); dump(exported()) }; f(9)`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"main.yak"}})
	result, err := prog.SyntaxFlowWithError(`before(* as $before); after(* as $after); dump(* #-> * as $source)`)
	require.NoError(t, err)
	before, after := result.GetValues("before"), result.GetValues("after")
	require.Len(t, before, 1)
	require.Len(t, after, 1)
	require.Equal(t, before[0].GetId(), after[0].GetId())
	sources := result.GetValues("source")
	require.Len(t, sources, 1)
	require.Equal(t, "7", sources[0].String())
}

func TestYakIncludeDatabaseReloadAcrossBatches(t *testing.T) {
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "0")
	t.Setenv(compileUnitBatchMaxFilesEnv, "1")
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/main.yak", `include "b/leaf.yak"; dump(b)`)
	vf.AddFile("b/leaf.yak", `b = 3`)
	vf.AddFile("c/main.yak", `include "./b/leaf.yak"; dump(b + 1)`)
	name := uuid.NewString()
	t.Cleanup(func() {
		ProgramCache.Remove(name)
		ssadb.DeleteProgram(ssadb.GetDB(), name)
	})
	programs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.Yak), WithProgramName(name),
		ssaconfig.WithCompileConcurrency(4), ssaconfig.WithCompileIrCacheTTL(time.Millisecond),
		ssaconfig.WithCompileIrCacheMax(1))
	require.NoError(t, err)
	require.Len(t, programs, 1)
	ProgramCache.Remove(name)
	loaded, err := FromDatabase(name)
	require.NoError(t, err)
	require.NotSame(t, programs[0].Program, loaded.Program)
	require.Len(t, loaded.Ref("b"), 1)
	require.Contains(t, loaded.Ref("b")[0].GetRange().GetEditor().GetUrl(), "b/leaf.yak")
	requireYakDumpValues(t, loaded, "3", "4")
}

func TestYakIncludeRootKeepsDefineFunction(t *testing.T) {
	files := map[string]string{
		"a/main.yak":      `include "b/functions.yak"`,
		"b/functions.yak": `f = func(value) { return value }`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"a/main.yak", "b/functions.yak"}}, WithDefineFunc(map[string]any{"f": func(string) string { return "" }}))
	values := prog.Ref("f")
	require.Len(t, values, 1)
	function, ok := ssa.ToFunction(values[0].GetSSAInst())
	require.True(t, ok)
	require.Len(t, function.Params, 1)
	parameter, ok := function.GetValueById(function.Params[0])
	require.True(t, ok)
	require.Equal(t, ssa.StringTypeKind, parameter.GetType().GetTypeKind())
}

func TestYakIncludeInsideFunctionDoesNotExportLocals(t *testing.T) {
	files := map[string]string{
		"leaf.yak":    `b = 3`,
		"library.yak": `f = () => { include "leaf.yak"; inside(b) }`,
		"main.yak":    `include "library.yak"; outside(b)`,
	}
	// Neither include file is registered as a root, so function-local
	// variables cannot be supplied by an independent root compilation.
	prog := parseYakFilesInOrder(t, files, [][]string{{"main.yak"}})
	result, err := prog.SyntaxFlowWithError(`inside(* as $inside); outside(* as $outside)`)
	require.NoError(t, err)
	inside := result.GetValues("inside")
	outside := result.GetValues("outside")
	require.Len(t, inside, 1)
	require.Equal(t, "3", inside[0].String())
	require.Len(t, outside, 1)
	require.Contains(t, outside[0].String(), "Undefined-b")
}

func TestYakIncludeReturnDoesNotFinishCaller(t *testing.T) {
	files := map[string]string{
		"a/main.yak": `include "b/leaf.yak"; dump("after")`,
		"b/leaf.yak": `return`,
	}
	prog := parseYakFilesInOrder(t, files, [][]string{{"a/main.yak", "b/leaf.yak"}})
	requireYakDumpValues(t, prog, `"after"`)
}

func TestYakIncludeFailuresAreNotReusable(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		files  map[string]string
		cancel bool
	}{
		{"cycle", map[string]string{"main.yak": `include "middle.yak"`, "middle.yak": `include "main.yak"`}, false},
		{"syntax-error", map[string]string{"main.yak": `include "bad.yak"`, "bad.yak": `a = (`}, false},
		{"canceled", map[string]string{"main.yak": `b = 3`}, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			for name, code := range fixture.files {
				vf.AddFile(name, code)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.Yak), WithMemory(), WithContext(ctx))
			require.NoError(t, err)
			prog, builder, err := cfg.init(vf, len(fixture.files))
			require.NoError(t, err)
			cfg.LanguageBuilder.InitHandler(builder)
			defer cfg.LanguageBuilder.Clearup()
			editor := prog.CreateEditor([]byte(fixture.files["main.yak"]), "main.yak")
			ast, err := cfg.LanguageBuilder.ParseAST(editor.GetSourceCode(), cfg.LanguageBuilder.GetAntlrCache())
			require.NoError(t, err)
			if fixture.cancel {
				cancel()
			}
			for attempt := 0; attempt < 2; attempt++ {
				require.Error(t, prog.Build(ast, editor, builder), "failed/partial files cannot become successful cache hits")
			}
		})
	}
}
