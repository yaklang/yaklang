package buildin_rule

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func isCSharpBuiltinRule(tag, language, name string) bool {
	blob := strings.ToLower(strings.Join([]string{tag, language, name}, "|"))
	return strings.Contains(blob, "csharp") || strings.Contains(blob, "c#") || strings.Contains(blob, "dotnet")
}

func TestVerifiedCSharpBuiltinRules(t *testing.T) {
	yakit.InitialDatabase()
	require.NoError(t, sfbuildin.SyncEmbedRule())
	db := consts.GetGormProfileDatabase().Where("is_build_in_rule = ?", true)

	var verified int
	for rule := range sfdb.YieldSyntaxFlowRules(db, context.Background()) {
		if !isCSharpBuiltinRule(rule.Tag, string(rule.Language), rule.RuleName) {
			continue
		}
		frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
		require.NoError(t, err, "compile %s", rule.RuleName)
		if len(frame.VerifyFsInfo) == 0 {
			t.Fatalf("csharp rule %s missing embedded file:// tests", rule.RuleName)
		}
		verified++
		t.Run(rule.RuleName, func(t *testing.T) {
			err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t, false)
			require.NoError(t, err, rule.RuleName)
		})
	}
	require.Greater(t, verified, 0, "no csharp builtin rules were verified")
}
