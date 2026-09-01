package yak

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestHotReloadVMYakFileCorpus(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	files := []string{
		"yaktest/mustpass/files/buildin_len.yak",
		"yaktest/mustpass/files/container_range.yak",
		"yaktest/mustpass/files/eval.yak",
		"yaktest/mustpass/files/forloopvar_go122.yak",
		"yaktest/mustpass/files/grammar_test.yak",
		"yaktest/mustpass/files/map_slice_auto_convert.yak",
		"yaktest/mustpass/files/operator_channel_comparison.yak",
		"yaktest/mustpass/files/defer-recover.yak",
		"yaktest/mustpass/files/fuzz_param_array.yak",
		"yaktest/mustpass/files/sandbox.yak",
		"yaktest/mustpass/files/yaklang_programming_complex.yak",
		"testdata/vm_hot_reload/async_nested_eval.yak",
		"testdata/vm_hot_reload/async_static_closure.yak",
		"testdata/vm_hot_reload/async_variadic_ellipsis.yak",
		"testdata/vm_hot_reload/sandbox_nested_closure.yak",
		"testdata/vm_hot_reload/sandbox_external_function.yak",
		"testdata/vm_hot_reload/string_index_cache.yak",
		"testdata/vm_hot_reload/include_main.yak",
	}

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			for revision := 0; revision < 2; revision++ {
				func() {
					source := fmt.Sprintf("%s\n// hot-reload-corpus-revision-%d %s", raw, revision, strings.Repeat("x", 320))
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()

					engine, execErr := NewScriptEngine(1).ExecuteWithoutCacheWithContext(ctx, source, map[string]interface{}{})
					require.NoError(t, execErr, "revision %d", revision)
					require.NotNil(t, engine)

					waitDone := make(chan struct{})
					go func() {
						engine.GetVM().AsyncWait()
						close(waitDone)
					}()
					select {
					case <-waitDone:
					case <-ctx.Done():
						t.Fatalf("revision %d left Yak async calls running: %v", revision, ctx.Err())
					}

					require.Nil(t, engine.GetVM().CurrentFM(), "revision %d left a current frame", revision)
					_, cached := antlr4yak.HaveYakcCache(source)
					require.False(t, cached, "revision %d entered yakc cache", revision)
				}()
			}
		})
	}
}

func TestHotReloadVMIncludeDependencyRevision(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	dependencyPath := filepath.Join(t.TempDir(), "dependency.yak")
	includePath := filepath.ToSlash(dependencyPath)

	for revision := 0; revision < 4; revision++ {
		dependency := fmt.Sprintf(`
includeValue = func() {
    return "dependency-revision-%d"
}
`, revision)
		require.NoError(t, os.WriteFile(dependencyPath, []byte(dependency), 0o600))

		source := fmt.Sprintf(`
include "%s"
assert includeValue() == "dependency-revision-%d", "stale include dependency at revision %d"
// %s
`, includePath, revision, revision, strings.Repeat("include-hot-reload-padding-", 16))
		engine, err := NewScriptEngine(1).ExecuteWithoutCacheWithContext(context.Background(), source, map[string]interface{}{})
		require.NoError(t, err, "revision %d", revision)
		require.NotNil(t, engine)
		require.Nil(t, engine.GetVM().CurrentFM(), "revision %d left a current frame", revision)

		_, cached := antlr4yak.HaveYakcCache(source)
		require.False(t, cached, "revision %d entered yakc cache", revision)
	}

	artifacts, err := filepath.Glob(filepath.Join(os.Getenv("YAKIT_HOME"), "temp", ".*.yakc*"))
	require.NoError(t, err)
	require.Empty(t, artifacts, "include hot reload left yakc artifacts: %v", artifacts)
}
