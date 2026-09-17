package sfbuildin

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestInitEmbedFS(t *testing.T) {
	hash, err := ruleFSWithHash.GetHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

// TestExtractRuleTitleMetadata_MatchesCompile guards the duplicate-title scan:
// it must return exactly the title/title_zh a full SyntaxFlow compile produces,
// otherwise the fast metadata path would silently change which duplicates are
// reported. The scan deliberately avoids the heredoc bodies, so any rule where
// it drifts from the compiler would be a real regression here.
func TestExtractRuleTitleMetadata_MatchesCompile(t *testing.T) {
	var checked int
	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(path)
		require.NoError(t, err)
		content := string(raw)

		compiled, err := sfdb.CheckSyntaxFlowRuleContent(content)
		if err != nil {
			// Rules that do not compile are skipped by the duplicate check too.
			return nil
		}
		meta := extractRuleTitleMetadata(content)
		require.Equal(t, compiled.Title, meta.Title, "title mismatch in %s", path)
		require.Equal(t, compiled.TitleZh, meta.TitleZh, "title_zh mismatch in %s", path)
		checked++
		return nil
	}))
	require.NoError(t, err)
	require.Greater(t, checked, 0, "expected at least one builtin rule to be checked")
}
