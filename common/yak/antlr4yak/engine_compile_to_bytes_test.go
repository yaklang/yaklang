package antlr4yak

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalcHashTracksIncludedSource(t *testing.T) {
	includePath := filepath.Join(t.TempDir(), "dependency.yak")
	require.NoError(t, os.WriteFile(includePath, []byte(`value = "first"`), 0o600))
	code := fmt.Sprintf("include %q\nprintln(value)", includePath)

	first := calcHash(code, nil)
	require.NoError(t, os.WriteFile(includePath, []byte(`value = "second"`), 0o600))
	second := calcHash(code, nil)

	require.NotEqual(t, first, second, "changing an included file must invalidate the parent yakc cache")
}

func TestCalcHashIgnoresIncludeTextOutsideStatements(t *testing.T) {
	code := `println("include") // include "missing.yak"`
	require.Empty(t, includeCacheMaterial(code, make(map[string]struct{})))
}

func TestYakcMemoryCacheIsBounded(t *testing.T) {
	yakcCache.Purge()
	t.Cleanup(yakcCache.Purge)

	keys := make([]string, 0, yakcMemoryCacheCapacity+32)
	for index := 0; index < yakcMemoryCacheCapacity+32; index++ {
		key := fmt.Sprintf("%s-%d", t.Name(), index)
		keys = append(keys, key)
		yakcCache.Set(key, []byte(key))
	}

	require.LessOrEqual(t, yakcCache.Count(), yakcMemoryCacheCapacity)
	_, oldestExists := yakcCache.Get(keys[0])
	require.False(t, oldestExists, "capacity must evict the oldest yakc entry")
	newest, newestExists := yakcCache.Get(keys[len(keys)-1])
	require.True(t, newestExists, "newest yakc entry was unexpectedly evicted")
	require.Equal(t, []byte(keys[len(keys)-1]), newest)
}

func BenchmarkEvalUniqueSourceWithCache(b *testing.B) {
	b.Setenv("YAKIT_HOME", b.TempDir())
	padding := strings.Repeat("x", YAKC_CACHE_MAX_LENGTH)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine := New()
		code := fmt.Sprintf("handler = () => { return %d } // %s", i, padding)
		if err := engine.SafeEval(ctx, code); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalUniqueSourceWithoutCache(b *testing.B) {
	b.Setenv("YAKIT_HOME", b.TempDir())
	padding := strings.Repeat("x", YAKC_CACHE_MAX_LENGTH)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine := New()
		code := fmt.Sprintf("handler = () => { return %d } // %s", i, padding)
		if err := engine.SafeEvalWithoutCache(ctx, code); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEvalWithoutCacheDoesNotPersistYakc(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	code := fmt.Sprintf("handler = () => { return 1 } // %s", strings.Repeat("x", YAKC_CACHE_MAX_LENGTH))
	hash := calcHash(code, nil)
	engine := New()
	require.NoError(t, engine.SafeEvalWithoutCache(context.Background(), code))

	_, ok := yakcCache.Get(hash)
	require.False(t, ok, "cache-disabled evaluation must not populate the memory cache")
	cachePath := filepath.Join(os.Getenv("YAKIT_HOME"), "temp", fmt.Sprintf(".%s.yakc", hash))
	_, err := os.Stat(cachePath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestEvalWithoutCacheIgnoresExistingYakc(t *testing.T) {
	padding := strings.Repeat("x", YAKC_CACHE_MAX_LENGTH)
	source := fmt.Sprintf(`value = "source" // %s-%s`, t.Name(), padding)
	wrongSource := fmt.Sprintf(`value = "cached" // %s`, padding)
	wrongYakc, err := New().Marshal(wrongSource, nil)
	require.NoError(t, err)
	hash := calcHash(source, nil)
	yakcCache.Set(hash, wrongYakc)
	t.Cleanup(func() { yakcCache.Remove(hash) })

	engine := New()
	require.NoError(t, engine.SafeEvalWithoutCache(context.Background(), source))
	require.Equal(t, "source", engine.Var("value"))

	stillCached, ok := HaveYakcCache(source)
	require.True(t, ok)
	require.Equal(t, wrongYakc, stillCached, "no-cache evaluation must neither read nor overwrite an existing artifact")
}
