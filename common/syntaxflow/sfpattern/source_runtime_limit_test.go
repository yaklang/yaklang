package sfpattern_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// A pattern that cannot be evaluated because PCRE2 exhausted its backtracking
// budget must not abort the shared pre-scan. Its hits for that file are
// unavailable, but every other pattern keeps the hits it found, and the run
// reports the degradation instead of failing the whole project scan.
func TestSourceScanRuntime_ExecutionLimitDegradesOnePattern(t *testing.T) {
	frame, err := sfvm.CompileRule("desc(mode: source, language: general)\n" +
		`${*.txt}.pattern_regex(/(?=(a+)+$)a/) as $limited
${*.txt}.pattern_regex(/needle/) as $ok
$limited + $ok as $hit
alert $hit`)
	require.NoError(t, err)

	plan, err := sfpattern.NewSourceScanPlan([]*sfvm.SFFrame{frame})
	require.NoError(t, err)
	defer plan.Close()

	files := map[string]string{"a.txt": "needle " + strings.Repeat("a", 64) + "!"}
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	require.NoError(t, runtime.Wait(context.Background()),
		"one pattern hitting the PCRE2 limit must not fail the shared pre-scan")

	stats := runtime.Stats()
	require.Equal(t, 1, stats.DegradedPatterns, "the limited pattern must be recorded, not silently dropped")
	require.Equal(t, 1, stats.Hits, "the unaffected pattern must keep its hit")

	root := sfpattern.NewRoot(files)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
	hits, err := root.FileFilter("*", "regexp", nil, []string{"needle"})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(hits))
}

// The shipped Spring datasource rule used an ambiguous per-line repetition.
// On a config that ends in `password: ${...}` the trailing negative lookahead
// always fails, so PCRE2 explored an exponential number of line splits and
// hit its 10M backtracking budget. The rule must stay quiet and, more
// importantly, must not turn a completed project scan into a failure.
func TestSourceScanRuntime_BuiltinSpringDatasourceRuleEnvPassword(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(sfanalysis.BuiltinRuleRoot(t),
		"general", "config", "config-spring-datasource-password.sf"))
	require.NoError(t, err)
	frame, err := sfvm.CompileRule(string(raw))
	require.NoError(t, err)

	plan, err := sfpattern.NewSourceScanPlan([]*sfvm.SFFrame{frame})
	require.NoError(t, err)
	defer plan.Close()

	envConfig := "spring:\n  application:\n    name: doris-demo\n" +
		strings.Repeat("    extra: value\n", 20) +
		"  datasource:\n" +
		"    url: jdbc:mysql://127.0.0.1:9030/demo\n" +
		"    username: ${DB_USER}\n" +
		"    password: ${DB_PASSWORD}\n"
	files := map[string]string{"application-druid.yml": envConfig}

	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	require.NoError(t, runtime.Wait(context.Background()),
		"an environment-variable password must not exceed the pre-scan match limit")
	require.Zero(t, runtime.Stats().DegradedPatterns)

	root := sfpattern.NewRoot(files)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
	result, err := frame.Feed(sfvm.ValuesOf(root))
	require.NoError(t, err)
	require.Empty(t, runtimeAlertSnapshot(result), "placeholders are not hardcoded credentials")

	plainConfig := "spring:\n  datasource:\n    username: root\n    password: RootPass123\n"
	plainFiles := map[string]string{"application.yml": plainConfig}
	plainRuntime := plan.Start(context.Background(), plainFiles)
	defer plainRuntime.Close()
	require.NoError(t, plainRuntime.Wait(context.Background()))

	plainRoot := sfpattern.NewRoot(plainFiles)
	plainRoot.SetFileFilterMatcher(plainRuntime.Matcher(context.Background()))
	plainResult, err := frame.Feed(sfvm.ValuesOf(plainRoot))
	require.NoError(t, err)
	require.NotEmpty(t, runtimeAlertSnapshot(plainResult), "a plaintext credential must still alert")
}
