package yakscripttools_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

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

		namePath := strings.Trim(strings.TrimPrefix(dirname, "yakscriptforai"), "/")
		prepared := yakscripttools.PrepareJavaAuditToolContent(namePath, string(content))
		res := yak.StaticAnalyze(prepared)
		errRes := lo.Filter(res, func(item *result.StaticAnalyzeResult, _ int) bool {
			return item.Severity == result.Error
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
