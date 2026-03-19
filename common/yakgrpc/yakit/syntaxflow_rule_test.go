package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestParseSyntaxFlowInput_LanguageFallback(t *testing.T) {
	t.Run("infer language from rule name", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "java-demo.sf",
			Content:  "",
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.JAVA, rule.Language)
	})

	t.Run("fallback to general", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "demo.sf",
			Content:  "",
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.General, rule.Language)
		require.Equal(t, "demo.sf", rule.RuleName)
	})

	t.Run("keep invalid content for later evaluation", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "java-invalid.sf",
			Content:  `invalid syntax here $$$`,
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.JAVA, rule.Language)
		require.Equal(t, `invalid syntax here $$$`, rule.Content)
		require.Equal(t, "java-invalid.sf", rule.RuleName)
	})
}
