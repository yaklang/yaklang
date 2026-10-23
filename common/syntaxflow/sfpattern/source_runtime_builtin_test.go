package sfpattern_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func runtimeAlertSnapshot(result *sfvm.SFFrameResult) []string {
	var out []string
	result.AlertSymbolTable.ForEach(func(name string, values sfvm.Values) bool {
		_ = values.Recursive(func(value sfvm.ValueOperator) error {
			hit := value.(*sfvm.SimpleValue)
			out = append(out, fmt.Sprintf("%s:%s:%d:%d:%s", name, hit.Path(), hit.Start(), hit.End(), hit.String()))
			return nil
		})
		return true
	})
	sort.Strings(out)
	return out
}

// Compile the full selected source set into ONE database, then compare every
// embedded positive and negative fixture with ordinary SFVM execution. This
// exercises cross-rule candidate filtering, real globs, PCRE2 and postprocessing.
func TestSourceScanRuntime_BuiltinEquivalence(t *testing.T) {
	type rule struct {
		name  string
		frame *sfvm.SFFrame
	}
	var rules []rule
	var frames []*sfvm.SFFrame
	sourceMode := regexp.MustCompile(`mode:\s*"?source\b`)
	err := filepath.WalkDir(sfanalysis.BuiltinRuleRoot(t), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".sf") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !sourceMode.Match(raw) {
			return nil
		}
		frame, err := sfvm.CompileRule(string(raw))
		if err != nil {
			return fmt.Errorf("compile %s: %w", path, err)
		}
		if !sfvm.FrameIsSourceMode(frame) {
			return nil
		}
		rules = append(rules, rule{filepath.Base(path), frame})
		frames = append(frames, frame)
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, rules)
	plan, err := sfpattern.NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	t.Logf("aggregated source rules: %d", len(rules))
	for _, rule := range rules {
		t.Run(rule.name, func(t *testing.T) {
			positive, err := rule.frame.ExtractVerifyFilesystemAndLanguage()
			require.NoError(t, err)
			negative, err := rule.frame.ExtractNegativeFilesystemAndLanguage()
			require.NoError(t, err)
			require.NotEmpty(t, positive)
			require.NotEmpty(t, negative)
			for i, fixture := range append(positive, negative...) {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					files, err := sfpattern.LoadFilesFromFS(fixture.GetVirtualFs())
					require.NoError(t, err)
					baseline, err := sfpattern.ExecFrame(rule.frame, files)
					require.NoError(t, err)
					runtime := plan.Start(context.Background(), files)
					defer runtime.Close()
					root := sfpattern.NewRoot(files)
					root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
					actual, err := rule.frame.Feed(sfvm.ValuesOf(root))
					require.NoError(t, err)
					require.NoError(t, runtime.Wait(context.Background()))
					require.Equal(t, runtimeAlertSnapshot(baseline), runtimeAlertSnapshot(actual), "alert paths, byte spans and text must be identical")
				})
			}
		})
	}
}
