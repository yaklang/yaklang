package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Rule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var ruleNames []string
	saveSyntaxFlowRule := func(num int) {
		req := &ypb.SaveSyntaxFlowRuleRequest{}
		for i := 0; i < num; i++ {
			ruleName := fmt.Sprintf("test_rule_%s.sf", uuid.NewString())
			ruleNames = append(ruleNames, ruleName)

			req = &ypb.SaveSyntaxFlowRuleRequest{
				RuleName: ruleName,
				Content:  fmt.Sprintf("check $a%d;", rand.Int()),
				Language: "java",
				Tags:     nil,
			}
			_, err := client.SaveSyntaxFlowRule(context.Background(), req)
			require.NoError(t, err)
		}
	}

	originRsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
	})
	require.NoError(t, err)
	saveSyntaxFlowRule(100)
	newRsp1, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
	})
	require.NoError(t, err)
	gapCount := newRsp1.DbMessage.EffectRows - originRsp.DbMessage.EffectRows
	require.Equal(t, 100, int(gapCount))

	deleteCount := 0
	for _, ruleName := range ruleNames {
		msg, err := client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{RuleName: ruleName},
		})
		require.NoError(t, err)
		deleteCount += int(msg.EffectRows)
	}
	require.Equal(t, 100, deleteCount)
	newRsp2, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
	})
	require.NoError(t, err)
	require.Equal(t, originRsp.DbMessage.EffectRows, newRsp2.DbMessage.EffectRows)
}
