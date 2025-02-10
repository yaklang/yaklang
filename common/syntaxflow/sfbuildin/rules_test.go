package sfbuildin

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"strings"
	"testing"
)

func Test_BuildIn_Rule_Ast(t *testing.T) {
	fsInstance := filesys.NewEmbedFS(ruleFS)
	filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		t.Run(name, func(t *testing.T) {
			raw, err := fsInstance.ReadFile(s)
			require.NoError(t, err)

			content := string(raw)
			_, err = sfdb.CheckSyntaxFlowRuleContent(content)
			require.NoError(t, err)
		})
		return nil
	}))
}
