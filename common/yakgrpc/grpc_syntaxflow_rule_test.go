package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func createSfRule(client ypb.YakClient, ruleName string) error {
	rule := &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Language: "java",
		},
	}
	_, err := client.CreateSyntaxFlowRule(context.Background(), rule)
	return err
}

func TestGRPCMUSTPASS_SyntaxFlow_Rule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

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

	queryRulesId := func(ruleName []string) []int64 {
		req := &ypb.QuerySyntaxFlowRuleRequest{
			Pagination: &ypb.Paging{Limit: -1},
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: ruleName,
			},
		}
		rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
		var ids []int64
		for _, rule := range rsp.GetRule() {
			require.NotEqual(t, rule.GetId(), int64(0))
			ids = append(ids, rule.GetId())
		}
		return ids
	}

	queryRulesById := func(fromId, utilId int64) []*ypb.SyntaxFlowRule {
		req := &ypb.QuerySyntaxFlowRuleRequest{
			Pagination: &ypb.Paging{Limit: -1},
			Filter: &ypb.SyntaxFlowRuleFilter{
				FromId:  fromId,
				UntilId: utilId,
			},
		}
		rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
		return rsp.GetRule()
	}

	t.Run("test create and delete syntax flow rule", func(t *testing.T) {
		var ruleNames []string

		beforeCreateCount := queryRulesCount()
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			err := createSfRule(client, ruleName)
			require.NoError(t, err)
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
			err := createSfRule(client, ruleName)
			require.NoError(t, err)
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

	t.Run("test query infinite list", func(t *testing.T) {
		var ruleNames []string
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			err = createSfRule(client, ruleName)
			require.NoError(t, err)
			ruleNames = append(ruleNames, ruleName)
		}
		ids := queryRulesId(ruleNames)
		require.Equal(t, len(ids), 100)
		rules := queryRulesById(ids[20], ids[60])
		require.Equal(t, len(rules), 40)
		deleteRuleByNames(ruleNames)
	})
}
