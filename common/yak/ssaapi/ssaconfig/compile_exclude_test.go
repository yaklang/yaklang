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
	require.Contains(t, patterns, ".github/**")
	require.Contains(t, patterns, ".mvn/**")
	require.Contains(t, patterns, "docs/**")
	require.Contains(t, patterns, "eclipse/**")
	require.Contains(t, patterns, "target/**")
	require.Contains(t, patterns, "**/test")
	require.Contains(t, patterns, "**/test/**")
	require.Contains(t, patterns, "**/testdata")
	require.Contains(t, patterns, "**/testdata/**")
	require.Contains(t, patterns, "**/vendor/**")

	// Aggregated language-specific defaults: directory names.
	require.Contains(t, patterns, "__pycache__/**")
	require.Contains(t, patterns, "**/CMakeFiles/**")
	require.Contains(t, patterns, "obj/**")
	require.Contains(t, patterns, "**/tests/**")
	require.Contains(t, patterns, "**/fixtures/**")
	require.Contains(t, patterns, "yakit-projects/**")

	// Aggregated language-specific defaults: file globs.
	require.Contains(t, patterns, "**/*_test.go")
	require.Contains(t, patterns, "**/*.min.js")
	require.Contains(t, patterns, "**/gradle/wrapper/**")
	require.Contains(t, patterns, "**/composer.lock")
	require.Contains(t, patterns, "**/*.nupkg")

	// Blanket archive exclusions must never appear in defaults: bundled
	// runtime/dependency archives can be legitimate scan targets. Only
	// test-related archives are excluded, by name or by test directory.
	require.NotContains(t, patterns, "**/*.jar")
	require.NotContains(t, patterns, "**/*.war")
	require.NotContains(t, patterns, "**/*.ear")
	require.NotContains(t, patterns, "**/*.class")
}

func TestBuildCompileExcludeFunc(t *testing.T) {
	t.Run("user pattern", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor"}, "", true)
		require.True(t, exclude("vendor"))
	})

	t.Run("user testdata", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"**/testdata/"}, "", true)
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata"))
		require.True(t, exclude("src/cmd/compile/internal/syntax/testdata/issue47704.go"))
	})

	t.Run("default test inputs", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude("src/test/service_test.go"))
		require.True(t, exclude("src/testdata/issue47704.go"))
		// "testing" directories are now excluded whole like other test dirs;
		// a similarly-named production directory must stay untouched.
		require.True(t, exclude("src/testing/service.go"))
		require.False(t, exclude("src/contest/service.go"))
	})

	t.Run("aggregated test directories and fixtures", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude("spring-orm/src/test/resources/order.jar"))
		require.True(t, exclude("src/tests/service_test.go"))
		require.True(t, exclude("src/__tests__/app.test.js"))
		require.True(t, exclude("src/test-fixtures/data.json"))
		require.True(t, exclude("src/it/java/Integration.java"))
	})

	t.Run("test-named archives excluded, runtime archives kept", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude("libs/mocktest.jar"))
		require.True(t, exclude("libs/MockTest.jar"))
		require.False(t, exclude("libs/spring-core.jar"))
	})

	t.Run("language-specific defaults", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		// Java build machinery
		require.True(t, exclude("gradle/wrapper/gradle-wrapper.jar"))
		// Go
		require.True(t, exclude("pkg/service/service_test.go"))
		require.True(t, exclude("go.sum"))
		// JS/TS
		require.True(t, exclude("app/dist/bundle.min.js"))
		require.True(t, exclude("src/app.spec.ts"))
		// Python
		require.True(t, exclude(".venv/lib/site.py"))
		require.True(t, exclude("src/test_service.py"))
		// PHP
		require.True(t, exclude("app/Tests/UnitTest.php"))
		// C#
		require.True(t, exclude("src/bin/Debug/app.dll"))
		// C/C++
		require.True(t, exclude("build/CMakeFiles/rule.o"))
		// Yak
		require.True(t, exclude("yakit-projects/default/1.yak"))
		// OS metadata
		require.True(t, exclude(".DS_Store"))
	})

	t.Run("default vendor", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude("src/vendor/lib.go"))
	})

	t.Run("default root dot git", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude(".git"))
		require.True(t, exclude(".git/objects/pack/pack.idx"))
		require.True(t, exclude("src/.git/config"))
		require.True(t, exclude(`src\.git\config`))
	})

	t.Run("default generated directories", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc(nil, "", true)
		require.True(t, exclude("node_modules/pkg/index.js"))
		require.True(t, exclude("src/target/classes/App.java"))
		require.True(t, exclude("build/generated/App.go"))
		require.True(t, exclude("src/.gradle/caches/modules.lock"))
	})

	t.Run("folder trailing slash", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor/"}, "", true)
		require.True(t, exclude("vendor/a.php"))
	})

	t.Run("defaults disabled keeps only user patterns", func(t *testing.T) {
		exclude := BuildCompileExcludeFunc([]string{"vendor/"}, "", false)
		// user pattern still applies (trailing slash expands to dir + dir/**)
		require.True(t, exclude("vendor/lib.go"))
		// built-in defaults no longer apply
		require.False(t, exclude("test.jar"))
		require.False(t, exclude("composer.lock"))
		require.False(t, exclude("node_modules/pkg/index.js"))
		require.False(t, exclude("src/testdata/issue47704.go"))
	})
}

func TestShouldSkipCompileDirName(t *testing.T) {
	require.True(t, ShouldSkipCompileDirName("testdata"))
	require.True(t, ShouldSkipCompileDirName("test"))
	require.True(t, ShouldSkipCompileDirName(".git"))
	require.True(t, ShouldSkipCompileDirName("node_modules"))
	require.True(t, ShouldSkipCompileDirName("target"))
	require.True(t, ShouldSkipCompileDirName("tests"))
	require.True(t, ShouldSkipCompileDirName("__pycache__"))
	require.True(t, ShouldSkipCompileDirName("testing"))
	require.False(t, ShouldSkipCompileDirName("contest"))
}
