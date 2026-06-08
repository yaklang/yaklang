package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCompileExcludePatterns(t *testing.T) {
	patterns := DefaultCompileExcludePatterns()
	require.Contains(t, patterns, ".git")
	require.Contains(t, patterns, ".git/**")
	require.Contains(t, patterns, "**/.git")
	require.Contains(t, patterns, "**/.git/**")
	require.Contains(t, patterns, "node_modules/**")
	require.Contains(t, patterns, "target/**")
	require.NotContains(t, patterns, "**/testdata")
	require.NotContains(t, patterns, "**/testdata/**")
	require.Contains(t, patterns, "**/vendor/**")
}

func TestBuildCompileExcludeFunc(t *testing.T) {
	t.Run("user pattern", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor"}, "")
		require.True(t, exclude("vendor"))
	})

	t.Run("user testdata", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"**/testdata/"}, "")
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata"))
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata/issue47704.go"))
	})

	t.Run("default keeps test inputs", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.False(t, exclude("src/test/service_test.go"))
		require.False(t, exclude("src/testdata/issue47704.go"))
	})

	t.Run("default vendor", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.True(t, exclude("src/vendor/lib.go"))
	})

	t.Run("default root dot git", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.True(t, exclude(".git"))
		require.True(t, exclude(".git/objects/pack/pack.idx"))
		require.True(t, exclude("src/.git/config"))
		require.True(t, exclude(`src\.git\config`))
	})

	t.Run("default generated directories", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "")
		require.True(t, exclude("node_modules/pkg/index.js"))
		require.True(t, exclude("src/target/classes/App.java"))
		require.True(t, exclude("build/generated/App.go"))
		require.True(t, exclude("src/.gradle/caches/modules.lock"))
	})

	t.Run("folder trailing slash", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor/"}, "")
		require.True(t, exclude("vendor/a.php"))
	})
}

func TestShouldSkipCompileDirName(t *testing.T) {
	require.False(t, ShouldSkipCompileDirName("testdata"))
	require.False(t, ShouldSkipCompileDirName("test"))
	require.True(t, ShouldSkipCompileDirName(".git"))
	require.True(t, ShouldSkipCompileDirName("node_modules"))
	require.True(t, ShouldSkipCompileDirName("target"))
	require.False(t, ShouldSkipCompileDirName("testing"))
}
