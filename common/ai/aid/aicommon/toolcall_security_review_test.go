package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSecurityRuleReviewDecision(t *testing.T) {
	cases := []struct {
		name   string
		params aitool.InvokeParams
		want   bool
	}{
		{name: "continue-cn", params: aitool.InvokeParams{"suggestion": "继续执行:"}, want: true},
		{name: "cancel-cn", params: aitool.InvokeParams{"suggestion": "取消:"}, want: false},
		{name: "continue-en", params: aitool.InvokeParams{"suggestion": "continue"}, want: true},
		{name: "cancel-en", params: aitool.InvokeParams{"suggestion": "cancel"}, want: false},
		{name: "continue-value", params: aitool.InvokeParams{"value": "continue"}, want: true},
		{name: "cancel-value", params: aitool.InvokeParams{"value": "cancel"}, want: false},
		{name: "continue-index", params: aitool.InvokeParams{"index": 1}, want: true},
		{name: "cancel-index", params: aitool.InvokeParams{"index": 2}, want: false},
		{name: "ambiguous", params: aitool.InvokeParams{"suggestion": "想想"}, want: false},
		{name: "command-text-is-not-approval", params: aitool.InvokeParams{"command": "echo continue"}, want: false},
		{name: "negated-cancel-is-not-approval", params: aitool.InvokeParams{"suggestion": "不要取消"}, want: false},
		{name: "empty", params: nil, want: false},
	}
	for _, c := range cases {
		got, reason := securityRuleReviewDecision(c.params)
		require.Equal(t, c.want, got, "%s: %s", c.name, reason)
	}

	prompt := securityRuleReviewPrompt(schema.SFR_ACTION_ASK, "bash", aitool.InvokeParams{
		"command": "curl http://169.254.169.254/latest/meta-data/",
	})
	require.Contains(t, prompt, "规则权限策略命中")
	require.Contains(t, prompt, "判定：询问")
	require.Contains(t, prompt, "169.254.169.254")
	require.NotContains(t, prompt, "级别：")
	require.Len(t, securityRuleReviewOptions(), 2)
}
