package sfpattern

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestLanguageAllowsPath(t *testing.T) {
	require.True(t, languageAllowsPath("", "Main.java"), "no language means no ceiling")
	require.True(t, languageAllowsPath("general", "Main.java"))
	require.True(t, languageAllowsPath("unknown-language", "Main.java"))

	require.True(t, languageAllowsPath("python", "app.py"))
	require.True(t, languageAllowsPath("python", "pkg/settings.pyi"))
	require.False(t, languageAllowsPath("python", "Main.java"))

	require.True(t, languageAllowsPath("java", "web/WEB-INF/web.xml"))
	require.True(t, languageAllowsPath("java", "app.properties"))
	require.False(t, languageAllowsPath("java", "app.py"))

	// Aliases resolve to the same ceiling.
	require.True(t, languageAllowsPath("javascript", "index.mjs"))
	require.True(t, languageAllowsPath("ecmascript", "App.jsx"))
	require.True(t, languageAllowsPath("go", "main.go"))
	require.False(t, languageAllowsPath("golang", "main.c"))

	// Name-based languages have no extension to check.
	require.True(t, languageAllowsPath("dockerfile", "Dockerfile"))
	require.True(t, languageAllowsPath("dockerfile", "build/app.dockerfile"))
	require.False(t, languageAllowsPath("dockerfile", "app.yaml"))
	require.True(t, languageAllowsPath("kubernetes", "deploy.yml"))
	require.False(t, languageAllowsPath("kubernetes", "Deploy.kt"))
}

// A source rule that declares a language keeps its blanket filter inside that
// language: a python rule must not report a hit from a java file, while a
// general rule still sees both.
func TestBlanketFilterRespectsRuleLanguage(t *testing.T) {
	pythonFrame, err := sfvm.CompileRule("desc(mode: source, language: python)\n" +
		`${*}.pattern_regex(/needle/) as $hit
alert $hit`)
	require.NoError(t, err)
	generalFrame, err := sfvm.CompileRule("desc(mode: source, language: general)\n" +
		`${*}.pattern_regex(/needle/) as $hit
alert $hit`)
	require.NoError(t, err)

	files := map[string]string{
		"app.py":       "needle\n",
		"Main.java":    "needle\n",
		"package.json": "needle\n",
	}

	t.Run("language ceiling", func(t *testing.T) {
		plan, err := NewSourceScanPlan([]*sfvm.SFFrame{pythonFrame})
		require.NoError(t, err)
		defer plan.Close()
		runtime := plan.Start(context.Background(), files)
		defer runtime.Close()
		root := NewRoot(files)
		root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
		values, err := root.FileFilter("*", "regexp", nil, []string{"needle"})
		require.NoError(t, err)
		require.Equal(t, 1, sfvm.ValuesLen(values), "only the python file may hit")
	})

	t.Run("general keeps the whole pool", func(t *testing.T) {
		plan, err := NewSourceScanPlan([]*sfvm.SFFrame{generalFrame})
		require.NoError(t, err)
		defer plan.Close()
		runtime := plan.Start(context.Background(), files)
		defer runtime.Close()
		root := NewRoot(files)
		root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
		values, err := root.FileFilter("*", "regexp", nil, []string{"needle"})
		require.NoError(t, err)
		require.Equal(t, 2, sfvm.ValuesLen(values), "manifests stay out, java and python stay in")
	})

	t.Run("shared search keeps the wider ceiling", func(t *testing.T) {
		plan, err := NewSourceScanPlan([]*sfvm.SFFrame{pythonFrame, generalFrame})
		require.NoError(t, err)
		defer plan.Close()
		runtime := plan.Start(context.Background(), files)
		defer runtime.Close()
		root := NewRoot(files)
		root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
		values, err := root.FileFilter("*", "regexp", nil, []string{"needle"})
		require.NoError(t, err)
		require.Equal(t, 2, sfvm.ValuesLen(values), "a general rule sharing the search widens it back")
	})
}
