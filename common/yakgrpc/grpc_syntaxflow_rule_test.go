package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func deleteRuleByNames(client ypb.YakClient, names []string) error {
	req := &ypb.DeleteSyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: names,
		},
	}
	_, err := client.DeleteSyntaxFlowRule(context.Background(), req)
	return err
}

func queryRulesCount(client ypb.YakClient) (int, error) {
	req := &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
	}
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
	if err != nil {
		return 0, err
	}
	return len(rsp.GetRule()), nil
}

func updateRuleByNames(client ypb.YakClient, names []string) error {
	for _, name := range names {
		req := &ypb.UpdateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: name,
				Language: "php",
			},
		}
		_, err := client.UpdateSyntaxFlowRule(context.Background(), req)
		if err != nil {
			return err
		}
	}
	return nil
}

func queryRulesId(client ypb.YakClient, ruleName []string) ([]int64, error) {
	req := &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: ruleName,
		},
	}
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, rule := range rsp.GetRule() {
		if rule.GetId() == 0 {
			return nil, errors.New("rule id must not be 0")
		}
		ids = append(ids, rule.GetId())
	}
	return ids, nil
}

func queryRulesById(client ypb.YakClient, afterID, beforeId int64) ([]*ypb.SyntaxFlowRule, error) {
	req := &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
		Filter: &ypb.SyntaxFlowRuleFilter{
			AfterId:  afterID,
			BeforeId: beforeId,
		},
	}
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return rsp.GetRule(), nil
}

func queryRulesByName(client ypb.YakClient, ruleNames []string) ([]*ypb.SyntaxFlowRule, error) {
	req := &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Limit: -1},
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: ruleNames,
		},
	}
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return rsp.GetRule(), nil
}

func createSfRuleEx(client ypb.YakClient, ruleName string) (*ypb.SyntaxFlowRule, error) {
	rule := &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Language: "java",
		},
	}
	rsp, err := client.CreateSyntaxFlowRuleEx(context.Background(), rule)
	return rsp.Rule, err
}

func TestGRPCMUSTPASS_SyntaxFlow_Rule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("test create and delete syntax flow rule", func(t *testing.T) {
		var ruleNames []string

		beforeCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			err := createSfRule(client, ruleName)
			require.NoError(t, err)
			ruleNames = append(ruleNames, ruleName)
		}
		afterCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)

		err = deleteRuleByNames(client, ruleNames)
		require.NoError(t, err)
		afterDeleteCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterDeleteCount-beforeCreateCount, 0)
	})

	t.Run("test create and update syntax flow rule", func(t *testing.T) {
		var ruleNames []string

		beforeCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			err := createSfRule(client, ruleName)
			require.NoError(t, err)
			ruleNames = append(ruleNames, ruleName)
		}
		afterCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)

		err = updateRuleByNames(client, ruleNames)
		require.NoError(t, err)
		afterUpdateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterUpdateCount-afterCreateCount, 0)

		err = deleteRuleByNames(client, ruleNames)
		require.NoError(t, err)
		afterDeleteCount, err := queryRulesCount(client)
		require.NoError(t, err)
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

		err = deleteRuleByNames(client, []string{ruleName})
		require.NoError(t, err)
	})

	t.Run("test query infinite list", func(t *testing.T) {
		var ruleNames []string
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			err = createSfRule(client, ruleName)
			require.NoError(t, err)
			ruleNames = append(ruleNames, ruleName)
		}
		ids, err := queryRulesId(client, ruleNames)
		require.NoError(t, err)
		require.Equal(t, len(ids), 100)
		rules, err := queryRulesById(client, ids[20], ids[60])
		require.Equal(t, len(rules), 40)
		err = deleteRuleByNames(client, ruleNames)
		require.NoError(t, err)
	})

	t.Run("test createSyntaxFlowEx", func(t *testing.T) {
		ids := make(map[int]struct{})
		var ruleNames []string

		beforeCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			rsp, err := createSfRuleEx(client, ruleName)
			log.Infof("rule created: %v", rsp)
			require.NotNil(t, rsp)
			require.NoError(t, err)
			require.Equal(t, rsp.RuleName, ruleName)
			ruleNames = append(ruleNames, ruleName)

			if _, ok := ids[int(rsp.Id)]; ok {
				t.Fatalf("id %d already exists", rsp.Id)
			} else {
				ids[int(rsp.Id)] = struct{}{}
			}
		}
		t.Cleanup(func() {
			err = deleteRuleByNames(client, ruleNames)
			require.NoError(t, err)
		})
		afterCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)
	})

	t.Run("test updateSyntaxFlowEx ", func(t *testing.T) {
		var ids []int64
		var ruleNames []string

		beforeCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
			rsp, err := createSfRuleEx(client, ruleName)
			require.NotNil(t, rsp)
			require.NoError(t, err)
			require.Equal(t, rsp.RuleName, ruleName)
			ruleNames = append(ruleNames, ruleName)
			ids = append(ids, (rsp.Id))
		}
		afterCreateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterCreateCount-beforeCreateCount, 100)

		updateToPHPByRuleName := func(name string) (*ypb.SyntaxFlowRule, error) {
			req := &ypb.UpdateSyntaxFlowRuleRequest{
				SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
					RuleName: name,
					Language: "php",
					Content:  "desc(\n  title: 'AAA',\n  type: audit,\n  level: warning,\n)",
				},
			}
			rsp, err := client.UpdateSyntaxFlowRuleEx(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, rsp)
			return rsp.Rule, err
		}

		for idx, name := range ruleNames {
			rsp, err := updateToPHPByRuleName(name)
			require.NotNil(t, rsp)
			require.NoError(t, err)
			require.Equal(t, rsp.RuleName, name)
			require.Equal(t, rsp.Language, "php")
			require.Contains(t, rsp.Content, "desc(")
			require.Equal(t, rsp.Id, ids[idx])
		}
		t.Cleanup(func() {
			err = deleteRuleByNames(client, ruleNames)
			require.NoError(t, err)
		})
		afterUpdateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterUpdateCount-afterCreateCount, 0)
	})

	t.Run("test create rule with group", func(t *testing.T) {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		groupName := fmt.Sprintf("group_%s", uuid.NewString())

		req := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:   ruleName,
				Language:   "java",
				GroupNames: []string{groupName},
			},
		}

		_, err = client.CreateSyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
		})

		queryRule, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, groupName, queryRule[0].GetGroupName()[0])

		count, err := queryRuleGroupCount(client, groupName)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})

	t.Run("test create rule with description", func(t *testing.T) {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		des := uuid.NewString()
		req := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:    ruleName,
				Language:    "java",
				Description: des,
			},
		}

		_, err = client.CreateSyntaxFlowRule(context.Background(), req)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
		})

		queryRule, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, des, queryRule[0].Description)
	})
}
