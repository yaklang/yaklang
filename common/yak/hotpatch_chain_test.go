package yak

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestChainedHotPatchProductionPathDoesNotPersistYakc(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	padding := strings.Repeat("hot-patch-revision-padding-", 20)

	for revision := 0; revision < 16; revision++ {
		globalCode := fmt.Sprintf(`
globalOnly = func(params) { return ["global-%d-" + params] }
shared = func(params) { return ["global-shared-%d-" + params] }
streamGlobal = func(params, yield) {
    yield("global-a-%d-" + params)
    yield("global-b-%d-" + params)
}
// %s
`, revision, revision, revision, revision, padding)
		moduleCode := fmt.Sprintf(`
shared = func(params) { return ["module-%d-" + params] }
streamModule = func(params, yield) {
    yield("module-a-%d-" + params)
    yield("module-b-%d-" + params)
}
// %s
`, revision, revision, revision, padding)

		opts := Fuzz_WithAllHotPatchChained(context.Background(), HotPatchChain{
			GlobalCode: globalCode,
			ModuleCode: moduleCode,
		})
		fuzzOpts := append(opts, mutate.Fuzz_WithEnableDangerousTag())

		result, err := mutate.FuzzTagExec("{{yak(shared|x)}}", fuzzOpts...)
		require.NoError(t, err)
		require.Equal(t, []string{fmt.Sprintf("module-%d-x", revision)}, result)

		result, err = mutate.FuzzTagExec("{{yak(globalOnly|x)}}", fuzzOpts...)
		require.NoError(t, err)
		require.Equal(t, []string{fmt.Sprintf("global-%d-x", revision)}, result)

		result, err = mutate.FuzzTagExec("{{yak(streamModule|x)}}", fuzzOpts...)
		require.NoError(t, err)
		require.Equal(t, []string{
			fmt.Sprintf("module-a-%d-x", revision),
			fmt.Sprintf("module-b-%d-x", revision),
		}, result)

		result, err = mutate.FuzzTagExec("{{yak:dyn(shared|x)}}", fuzzOpts...)
		require.NoError(t, err)
		require.Equal(t, []string{fmt.Sprintf("module-%d-x", revision)}, result)

		_, cached := antlr4yak.HaveYakcCache(globalCode)
		require.False(t, cached, "global hot-patch revision %d entered yakc cache", revision)
		_, cached = antlr4yak.HaveYakcCache(moduleCode)
		require.False(t, cached, "module hot-patch revision %d entered yakc cache", revision)
	}

	artifacts, err := filepath.Glob(filepath.Join(os.Getenv("YAKIT_HOME"), "temp", ".*.yakc*"))
	require.NoError(t, err)
	require.Empty(t, artifacts, "production chained hot reload left yakc artifacts: %v", artifacts)
}

func TestLegacyHotPatchAPIsDoNotPersistYakc(t *testing.T) {
	tests := []struct {
		name      string
		buildOpts func(context.Context, string) []mutate.FuzzConfigOpt
		tags      []string
	}{
		{
			name: "regular",
			buildOpts: func(ctx context.Context, code string) []mutate.FuzzConfigOpt {
				return []mutate.FuzzConfigOpt{Fuzz_WithHotPatch(ctx, code)}
			},
			tags: []string{"yak"},
		},
		{
			name: "dynamic",
			buildOpts: func(ctx context.Context, code string) []mutate.FuzzConfigOpt {
				return []mutate.FuzzConfigOpt{Fuzz_WithDynHotPatch(ctx, code)}
			},
			tags: []string{"yak:dyn"},
		},
		{
			name:      "all",
			buildOpts: Fuzz_WithAllHotPatch,
			tags:      []string{"yak", "yak:dyn"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("YAKIT_HOME", t.TempDir())
			code := fmt.Sprintf(`
legacy = func(params) { return ["%s-value-" + params] }
legacyYield = func(params, yield) {
    yield("%s-yield-a-" + params)
    yield("%s-yield-b-" + params)
}
// %s %s
`, test.name, test.name, test.name, os.Getenv("YAKIT_HOME"), strings.Repeat("legacy-hot-reload-padding-", 20))

			opts := append(test.buildOpts(context.Background(), code), mutate.Fuzz_WithEnableDangerousTag())
			for _, tag := range test.tags {
				result, err := mutate.FuzzTagExec(fmt.Sprintf("{{%s(legacy|x)}}", tag), opts...)
				require.NoError(t, err)
				require.Equal(t, []string{fmt.Sprintf("%s-value-x", test.name)}, result)

				result, err = mutate.FuzzTagExec(fmt.Sprintf("{{%s(legacyYield|x)}}", tag), opts...)
				require.NoError(t, err)
				expected := []string{
					fmt.Sprintf("%s-yield-a-x", test.name),
					fmt.Sprintf("%s-yield-b-x", test.name),
				}
				if tag == "yak:dyn" {
					// Dynamic tags consume one yielded item per expansion step.
					expected = expected[:1]
				}
				require.Equal(t, expected, result)
			}

			_, cached := antlr4yak.HaveYakcCache(code)
			require.False(t, cached, "legacy %s hot-patch API entered yakc cache", test.name)
			artifacts, err := filepath.Glob(filepath.Join(os.Getenv("YAKIT_HOME"), "temp", ".*.yakc*"))
			require.NoError(t, err)
			require.Empty(t, artifacts)
		})
	}
}

func TestOrdinaryScriptExecutionStillCachesYakc(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	// Include the isolated cache directory so repeated test-process runs do not
	// hit the package-global memory cache populated by an earlier -count round.
	code := fmt.Sprintf(`value = "ordinary" // %s %s`, os.Getenv("YAKIT_HOME"), strings.Repeat("ordinary-cache-padding-", 20))

	engine := NewScriptEngine(1)
	_, err := engine.ExecuteExWithContext(context.Background(), code, map[string]interface{}{})
	require.NoError(t, err)
	_, cached := antlr4yak.HaveYakcCache(code)
	require.True(t, cached, "cache disabling must stay scoped to hot reload entrypoints")

	yakcFiles, err := filepath.Glob(filepath.Join(os.Getenv("YAKIT_HOME"), "temp", ".*.yakc"))
	require.NoError(t, err)
	require.NotEmpty(t, yakcFiles)
	sipFiles, err := filepath.Glob(filepath.Join(os.Getenv("YAKIT_HOME"), "temp", ".*.yakc.sip"))
	require.NoError(t, err)
	require.NotEmpty(t, sipFiles)
}

func BenchmarkScriptEngineStableSourceWithCache(b *testing.B) {
	benchmarkScriptEngineStableSource(b, true)
}

func BenchmarkScriptEngineStableSourceWithoutCache(b *testing.B) {
	benchmarkScriptEngineStableSource(b, false)
}

func benchmarkScriptEngineStableSource(b *testing.B, cache bool) {
	b.Setenv("YAKIT_HOME", b.TempDir())
	code := `handler = func(value) { return value + "-stable" } // ` + strings.Repeat("stable-global-padding-", 24)
	ctx := context.Background()
	if cache {
		_, err := NewScriptEngine(1).ExecuteExWithContext(ctx, code, map[string]interface{}{})
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := NewScriptEngine(1)
		var err error
		if cache {
			_, err = engine.ExecuteExWithContext(ctx, code, map[string]interface{}{})
		} else {
			_, err = engine.ExecuteWithoutCacheWithContext(ctx, code, map[string]interface{}{})
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}
