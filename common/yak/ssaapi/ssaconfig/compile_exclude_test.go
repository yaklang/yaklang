package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCompileExcludePatterns(t *testing.T) {
	patterns := DefaultCompileExcludePatterns()
	require.Contains(t, patterns, "**/testdata")
	require.Contains(t, patterns, "**/testdata/**")
	require.Contains(t, patterns, "**/vendor/**")
}

func TestBuildCompileExcludeFunc(t *testing.T) {
	t.Run("user pattern", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor"}, "")
		require.True(t, exclude("vendor"))
	})

	t.Run("default testdata", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata"))
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata/issue47704.go"))
	})

	t.Run("default vendor", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.True(t, exclude("src/vendor/lib.go"))
	})

	t.Run("folder trailing slash", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor/"}, "")
		require.True(t, exclude("vendor/a.php"))
	})
}

func TestShouldSkipCompileDirName(t *testing.T) {
	require.True(t, ShouldSkipCompileDirName("testdata"))
	require.True(t, ShouldSkipCompileDirName("test"))
	require.True(t, ShouldSkipCompileDirName(".git"))
	require.False(t, ShouldSkipCompileDirName("testing"))
}
