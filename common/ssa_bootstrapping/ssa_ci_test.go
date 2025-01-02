package ssa_bootstrapping

import (
	"embed"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"strings"
	"testing"
)

//go:embed ci_rule/***
var ruleFS embed.FS

func TestSSACI(t *testing.T) {

	fsInstance := filesys.NewEmbedFS(ruleFS)
	sfRules := []*schema.SyntaxFlowRule{}
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}

		raw, err := fsInstance.ReadFile(s)
		require.NoError(t, err)

		content := string(raw)

		sfRule, err := sfdb.CheckSyntaxFlowRuleContent(content)
		require.NoError(t, err) // import builtin rule

		sfRules = append(sfRules, sfRule)
		return nil
	}))
	require.NoError(t, err)

	gitFs, err := yakgit.FromCommitRange("./", "a4cf4db245366393e99d9496498c4c64802c5cc3", "c2dcc262ad9abb4a47746e1ddf53d3f5131d30cf")
	require.NoError(t, err)

	progs, err := ssaapi.ParseProjectWithFS(gitFs, ssaapi.WithLanguage(ssaapi.GO))
	require.NoError(t, err)
	require.Len(t, progs, 1)
	prog := progs[0]

	res, err := prog.SyntaxFlowRule(&schema.SyntaxFlowRule{})
	require.NoError(t, err)

	_ = res
}
