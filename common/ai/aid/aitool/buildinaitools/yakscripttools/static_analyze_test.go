package yakscripttools_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func embedToolNamePath(dirname, toolname string) string {
	namePath, ok := strings.CutPrefix(dirname, "yakscriptforai")
	if !ok {
		namePath = dirname
	}
	namePath = strings.Trim(namePath, `/\`)
	if namePath == "" {
		return toolname
	}
	return path.Join(namePath, toolname)
}

// libBundleSSAFalsePositive filters SSA closure-capture noise on concatenated lib+entry scripts.
// Runtime execution is validated in java_audit_test.go; bundled lib uses nested callbacks SSA mishandles.
func libBundleSSAFalsePositive(item *result.StaticAnalyzeResult) bool {
	if item == nil {
		return false
	}
	msg := item.String()
	return strings.Contains(msg, "closure function expects to capture variable")
}

func TestSSAParse(t *testing.T) {
	filesystem := yakscripttools.GetEmbedFS()

	results := make(map[string][]*result.StaticAnalyzeResult)
	filesys.Recursive(".", filesys.WithFileSystem(filesystem), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		filename := info.Name()
		_, filename = filesystem.PathSplit(filename)
		dirname, _ := filesystem.PathSplit(s)
		if filesystem.Ext(filename) != ".yak" {
			return nil
		}
		toolname := strings.TrimSuffix(filename, ".yak")
		if yakscripttools.ShouldSkipYakScriptEmbedPath(dirname, toolname) {
			return nil
		}

		content, err := filesystem.ReadFile(s)
		require.NoError(t, err)

		namePath := embedToolNamePath(dirname, toolname)
		code := string(content)
		libBundled := false
		if yakscripttools.NeedsLibBundlePrepForPath(namePath, code) {
			code = yakscripttools.PrepareToolContent(namePath, code)
			libBundled = true
		}

		res := yak.StaticAnalyze(code)
		errRes := lo.Filter(res, func(item *result.StaticAnalyzeResult, _ int) bool {
			if item.Severity != result.Error {
				return false
			}
			if libBundled && libBundleSSAFalsePositive(item) {
				return false
			}
			return true
		})
		if len(errRes) > 0 {
			results[filename] = errRes
		}
		return nil
	}))

	for filename, errs := range results {
		for _, err := range errs {
			t.Logf("Error in %s:\n \t%s\n\n", filename, err.String())
		}
	}

	require.Len(t, results, 0)
}
