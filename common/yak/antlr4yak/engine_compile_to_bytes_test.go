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
