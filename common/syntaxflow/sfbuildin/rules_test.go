package sfbuildin

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"strings"
	"testing"
)

func Test_BuildIn_Rule_Ast_And_Rule_Id(t *testing.T) {
	fsInstance := filesys.NewEmbedFS(ruleFS)
	ruleIdMap := make(map[string]bool)
	filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		t.Run(name, func(t *testing.T) {
			raw, err := fsInstance.ReadFile(s)
			require.NoError(t, err)

			content := string(raw)
			rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
			require.NoError(t, err)
			// check empty rule id
			if rule.GetInfo().Title != "" && rule.RuleId == "" {
				t.Fatalf("rule id should not be empty,rule name:%s", rule.RuleName)
			}
			// check deduplication of rule id
			if rule.RuleId == "" {
				return
			}
			ruleId := rule.RuleId
			if !ruleIdMap[ruleId] {
				ruleIdMap[ruleId] = true
			} else {
				t.Fatalf("The rule ID should not be duplicated:%s", ruleId)
			}

			// check id length
			require.Equal(t, 8, len(ruleId))
			// check language
			switch strings.Split(name, "-")[0] {
			case "golang":
				require.Equal(t, "1", string(ruleId[0]))
			case "java":
				require.Equal(t, "2", string(ruleId[0]))
			case "php":
				require.Equal(t, "3", string(ruleId[0]))
			}
		})
		return nil
	}))
}
