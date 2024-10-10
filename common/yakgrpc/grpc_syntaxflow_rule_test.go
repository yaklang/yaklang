package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Rule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	createRule := func(ruleName string) {
		rule := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: ruleName,
				Language: "java",
			},
		}
		_, err := client.CreateSyntaxFlowRule(context.Background(), rule)
		require.NoError(t, err)
	}

	queryRulesCount := func() int {
		req := &ypb.QuerySyntaxFlowRuleRequest{
			Pagination: &ypb.Paging{Limit: -1},
		}
		rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
		return len(rsp.GetRule())
	}

	deleteRuleByNames := func(names []string) {
		req := &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: names,
			},
		}
		_, err = client.DeleteSyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
	}

	updateRuleByNames := func(names []string) {
		for _, name := range names {
			req := &ypb.UpdateSyntaxFlowRuleRequest{
				SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
					RuleName: name,
					Language: "php",
				},
			}
			_, err = client.UpdateSyntaxFlowRule(context.Background(), req)
			require.NoError(t, err)
		}
	}

	t.Run("test create and delete syntax flow rule", func(t *testing.T) {
		var ruleNames []string

		beforeCreateCount := queryRulesCount()
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			createRule(ruleName)
			ruleNames = append(ruleNames, ruleName)
		}
		afterCreateCount := queryRulesCount()
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)

		deleteRuleByNames(ruleNames)
		afterDeleteCount := queryRulesCount()
		require.Equal(t, afterDeleteCount-beforeCreateCount, 0)
	})

	t.Run("test create and update  syntax flow rule", func(t *testing.T) {
		var ruleNames []string

		beforeCreateCount := queryRulesCount()
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			createRule(ruleName)
			ruleNames = append(ruleNames, ruleName)
		}
		afterCreateCount := queryRulesCount()
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)

		updateRuleByNames(ruleNames)
		afterUpdateCount := queryRulesCount()
		require.Equal(t, afterUpdateCount-afterCreateCount, 0)

		deleteRuleByNames(ruleNames)
		afterDeleteCount := queryRulesCount()
		require.Equal(t, afterDeleteCount-beforeCreateCount, 0)
	})

	t.Run("test query syntax rule by key word", func(t *testing.T) {
		ruleName := uuid.NewString()
		createReq := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: ruleName,
				Content: `desc(
  title: '这是一个测试文件',
  type: audit,
  level: warning,
)`,
				Language: "java",
			},
		}
		_, err := client.CreateSyntaxFlowRule(context.Background(), createReq)
		require.NoError(t, err)

		queryReq := &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				Keyword: "这是一个测试文件",
			},
		}

		rsp, err := client.QuerySyntaxFlowRule(context.Background(), queryReq)
		require.NoError(t, err)
		require.Equal(t, 1, len(rsp.GetRule()))
		require.Equal(t, ruleName, rsp.GetRule()[0].RuleName)

		deleteRuleByNames([]string{ruleName})
	})

}
