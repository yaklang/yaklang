package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"testing"
)

func TestMUSTPASS_SyntaxFlowRuleUpdate(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	yakit.DelKey(consts.GetGormProfileDatabase(), consts.EmbedSfBuildInRuleKey)
	update, err := client.CheckSyntaxFlowRuleUpdate(context.Background(), &ypb.CheckSyntaxFlowRuleUpdateRequest{})
	spew.Dump(update)
	require.NoError(t, err)
	require.True(t, update.GetNeedUpdate())
	stream, err := client.ApplySyntaxFlowRuleUpdate(context.Background(), &ypb.ApplySyntaxFlowRuleUpdateRequest{})
	require.NoError(t, err)
	var finalProcess float64
	finish := false
	for {
		rsp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		spew.Dump(rsp)
		finalProcess = rsp.GetPercent()
		if strings.Contains(rsp.GetMessage(), "更新SyntaxFlow内置规则成功！") {
			finish = true
		}
	}
	require.Equal(t, float64(1), finalProcess)
	require.True(t, finish)

	update, err = client.CheckSyntaxFlowRuleUpdate(context.Background(), &ypb.CheckSyntaxFlowRuleUpdateRequest{})
	require.NoError(t, err)
	require.False(t, update.GetNeedUpdate())
}

func TestMUSTPASS_SyntaxFlowRuleUpdateAlertDesc(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ruleName := uuid.NewString()
	defer func() {
		client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			}})
	}()
	content := `desc(
	level: "low"
)
a() as $sink
alert $low for{
	title: "存在xxx漏洞"
}
alert $high for{
	level: "high",
	title: "存在xxx漏洞2"
}
`
	res, err := client.CreateSyntaxFlowRuleEx(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Content:  content,
			Language: "php",
		},
	})
	require.NoError(t, err)
	hg, exist := res.Rule.AlertMsg["high"]
	require.True(t, exist)
	hg.Severity = string(schema.SFR_SEVERITY_HIGH)
	_, rules, err := yakit.QuerySyntaxFlowRule(consts.GetGormProfileDatabase(), &ypb.QuerySyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(rules))
	rule := rules[0]
	high, isHigh := rule.AlertDesc["high"]
	require.True(t, isHigh)
	require.True(t, high.Severity == schema.SFR_SEVERITY_HIGH)
	require.NoError(t, err)
	_, err = client.UpdateSyntaxFlowRule(context.Background(), &ypb.UpdateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Content:  content,
			AlertMsg: map[string]*ypb.AlertMessage{
				"high": {
					Severity: string(schema.SFR_SEVERITY_CRITICAL),
				},
			},
			Language: "php",
		},
	})
	require.NoError(t, err)
	_, rules, err = yakit.QuerySyntaxFlowRule(consts.GetGormProfileDatabase(), &ypb.QuerySyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
	require.True(t, len(rules) == 1)
	for _, flowRule := range rules {
		high, isexist := flowRule.AlertDesc["high"]
		require.True(t, isexist)
		require.True(t, high.Severity == schema.SFR_SEVERITY_CRITICAL)
	}
}
