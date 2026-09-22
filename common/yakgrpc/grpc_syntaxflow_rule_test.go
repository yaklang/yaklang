//go:build !irify_exclude

package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yaklib"
	"google.golang.org/grpc"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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

func updateRuleByNames(client ypb.YakClient, names []string, des string) error {
	for _, name := range names {
		req := &ypb.UpdateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:    name,
				Language:    "php",
				Description: des,
				Content:     `println as $output`,
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

		flag := uuid.NewString()
		err = updateRuleByNames(client, ruleNames, flag)
		require.NoError(t, err)
		afterUpdateCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterUpdateCount-afterCreateCount, 0)

		rsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				Keyword: flag,
			},
			Pagination: &ypb.Paging{Limit: -1},
		})
		require.NoError(t, err)
		require.Equal(t, 100, len(rsp.GetRule()))

		err = deleteRuleByNames(client, ruleNames)
		require.NoError(t, err)
		afterDeleteCount, err := queryRulesCount(client)
		require.NoError(t, err)
		require.Equal(t, afterDeleteCount-beforeCreateCount, 0)
	})

	t.Run("test update group", func(t *testing.T) {
		ruleName := uuid.NewString()
		group1 := uuid.NewString()
		group2 := uuid.NewString()
		_, err = client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:   ruleName,
				Language:   "java",
				Content:    `println as $output`,
				GroupNames: []string{group1},
			},
		})
		require.NoError(t, err)
		_, err = client.UpdateSyntaxFlowRule(context.Background(), &ypb.UpdateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:   ruleName,
				Language:   "java",
				GroupNames: []string{group2},
				Content:    `println as $output`,
			},
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			deleteRuleByNames(client, []string{ruleName})
			deleteRuleGroup(client, []string{group2, group1})
		})

		rsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(rsp.GetRule()))
		require.Contains(t, rsp.GetRule()[0].GetGroupName(), group2)
		require.NotContains(t, rsp.GetRule()[0].GetGroupName(), group1)
	})

	t.Run("test dirty update", func(t *testing.T) {

		ruleName := fmt.Sprintf("test_%s.sf", uuid.NewString())
		err = createSfRule(client, ruleName)
		require.NoError(t, err)

		flag := uuid.NewString()
		err = updateRuleByNames(client, []string{ruleName}, flag)
		require.NoError(t, err)

		rsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				Keyword: flag,
			},
			Pagination: &ypb.Paging{Limit: -1},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(rsp.GetRule()))

		updateRule, err := sfdb.QueryRuleByName(consts.GetGormProfileDatabase(), ruleName)
		require.Equal(t, updateRule.NeedUpdate, true)

		err = deleteRuleByNames(client, []string{ruleName})
		require.NoError(t, err)
	})

	t.Run("test query syntax rule by key word", func(t *testing.T) {
		ruleName := uuid.NewString()
		token := uuid.NewString()
		createReq := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: ruleName,
				Content: fmt.Sprintf(`desc(
  title: '%s',
  type: audit,
  level: warning,
)`, token),
				Language: "java",
			},
		}
		_, err := client.CreateSyntaxFlowRule(context.Background(), createReq)
		require.NoError(t, err)

		queryReq := &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				Keyword: token,
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
		require.Equal(t, len(rules), 39)
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
			deleteRuleByNames(client, []string{ruleName})
			deleteRuleGroup(client, []string{groupName})
		})

		queryRule, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Contains(t, queryRule[0].GetGroupName(), groupName)

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

	t.Run("test rule version in create rule", func(t *testing.T) {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		_, err := createSfRuleEx(client, ruleName)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
		})

		db := consts.GetGormProfileDatabase()
		_, rules, err := yakit.QuerySyntaxFlowRule(db, &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{
					ruleName,
				},
			},
		})
		require.NotEqual(t, rules[0].Version, "")
	})

	t.Run("test rule version in update rule", func(t *testing.T) {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		_, err := createSfRuleEx(client, ruleName)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
		})

		db := consts.GetGormProfileDatabase()
		_, rules, err := yakit.QuerySyntaxFlowRule(db, &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{
					ruleName,
				},
			},
		})
		require.NoError(t, err)

		version := rules[0].Version
		require.NotEqual(t, version, "")

		_, err = client.UpdateSyntaxFlowRuleEx(context.Background(), &ypb.UpdateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: ruleName,
				Language: "java",
				Content:  "aaa",
			},
		})
		require.NoError(t, err)

		_, rules, err = yakit.QuerySyntaxFlowRule(db, &ypb.QuerySyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{
					ruleName,
				},
			},
		})

		require.NoError(t, err)
		require.NotEqual(t, version, rules[0].Version)
	})
}

func TestGRPCMUSTPASS_DeleteSyntaxFlow_With_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
	groupName := uuid.NewString()
	req := &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName:   ruleName,
			GroupNames: []string{groupName},
			Language:   "java",
		},
	}

	t.Cleanup(func() {
		_, err := deleteRuleGroup(client, []string{groupName})
		require.NoError(t, err)
	})
	_, err = client.CreateSyntaxFlowRule(context.Background(), req)
	require.NoError(t, err)
	beforeDelete, err := queryRulesByName(client, []string{ruleName})
	require.NoError(t, err)
	require.Equal(t, 1, len(beforeDelete))

	deleteReq := &ypb.DeleteSyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			GroupNames: []string{groupName},
		},
	}
	_, err = client.DeleteSyntaxFlowRule(context.Background(), deleteReq)
	require.NoError(t, err)
	afterDelete, err := queryRulesByName(client, []string{ruleName})
	require.NoError(t, err)
	require.Equal(t, 0, len(afterDelete))
}

func TestGrpcMUSTPASS_UpdateSyntaxFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ruleName := uuid.NewString()
	Content := `desc(
	lang: java
)
	println as $output
`
	req := &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Language: "java",
			RuleName: ruleName,
			Content:  Content,
		},
	}
	_, err = client.CreateSyntaxFlowRule(context.Background(), req)
	defer func() {
		err := sfdb.DeleteRuleByRuleName(ruleName)
		require.NoError(t, err)
	}()
	require.NoError(t, err)
	updateRuleContent := `
desc(
	title_zh: "1",
	lang: java
)
println as $output;
alert $output
`
	Updaterule, _ := sfdb.CheckSyntaxFlowRuleContent(updateRuleContent)
	Updaterule.RuleName = ruleName
	_, err = client.UpdateSyntaxFlowRule(context.Background(), &ypb.UpdateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Language: "java",
			RuleName: ruleName,
			Content:  updateRuleContent,
		},
	})
	require.NoError(t, err)
	_, rules, err := yakit.QuerySyntaxFlowRule(consts.GetGormProfileDatabase(), &ypb.QuerySyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
	require.True(t, len(rules) == 1)

	for _, rule := range rules {
		require.Equal(t, rule.RuleName, Updaterule.RuleName)
		require.Equal(t, rule.Tag, Updaterule.Tag)
		require.Equal(t, rule.OpCodes, Updaterule.OpCodes)
		require.Equal(t, rule.Content, updateRuleContent)
		require.Equal(t, rule.TitleZh, Updaterule.TitleZh)
		require.Equal(t, rule.AlertDesc, Updaterule.AlertDesc)
		require.Equal(t, rule.Hash, Updaterule.CalcHash())
	}
}

func TestGRPCMUSTPASS_Delete_BuildIn_Rule(t *testing.T) {
	t.Skip("build in rule allow to be delted")
	client, err := NewLocalClient()
	require.NoError(t, err)

	ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
	rule := &schema.SyntaxFlowRule{
		RuleName:      ruleName,
		IsBuildInRule: true,
	}
	db := consts.GetGormProfileDatabase()
	db.Create(rule)
	t.Cleanup(func() {
		db.Where("rule_name = ?", ruleName).Delete(&schema.SyntaxFlowRule{})
	})
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: nil,
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(rsp.GetRule()))

	_, err = client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)

	//内置规则不能删
	rsp, err = client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: nil,
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(rsp.GetRule()))
}

func TestGRPCMUSTPASS_Query_Lib_Rule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
	rule := &schema.SyntaxFlowRule{
		RuleName:      ruleName,
		AllowIncluded: true,
	}
	db := consts.GetGormProfileDatabase()
	db = db.Create(rule)
	require.NoError(t, db.Error)
	t.Cleanup(func() {
		db.Where("rule_name = ?", ruleName).Delete(&schema.SyntaxFlowRule{})
	})
	rsp, err := client.QuerySyntaxFlowRule(context.Background(), &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: nil,
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames:         []string{ruleName},
			FilterLibRuleKind: yakit.FilterLibRuleTrue,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(rsp.GetRule()))

	_, err = client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
	})
	require.NoError(t, err)
}

func TestUploadSyntaxFlowRule(t *testing.T) {
	cases := []struct {
		name, local, remote string
		dirty               bool
		upload              int
		summary             string
	}{
		{"first publish", "20251015.0001", "", false, 1, "成功 1"},
		{"modified newer", "20251015.0003", "20251015.0002", true, 1, "成功 1"},
		{"remote newer", "20251015.0001", "20251015.0002", false, 0, "跳过 1"},
		{"conflict", "20251015.0001", "20251015.0002", true, 0, "冲突 1"},
		{"unmarked newer", "20251015.0003", "20251015.0002", false, 0, "失败 1"},
		{"modified same", "20251015.0002", "20251015.0002", true, 1, "成功 1"},
		{"unmarked same", "20251015.0002", "20251015.0002", false, 0, "失败 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newOnlineTestDB(t, &schema.SyntaxFlowRule{}, &schema.SyntaxFlowGroup{})
			rule := &schema.SyntaxFlowRule{RuleName: uuid.NewString(), RuleId: uuid.NewString(), Content: "local-content", Version: tc.local, NeedUpdate: tc.dirty}
			require.NoError(t, db.Create(rule).Error)
			require.NoError(t, db.Create(&schema.SyntaxFlowRule{RuleName: "distractor", RuleId: "other", Content: "other"}).Error)
			uploads := 0
			remote := &stubOnlineService{
				downloadRules: func(ctx context.Context, token string, req *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream {
					require.Equal(t, "valid-token", token)
					require.Equal(t, []string{rule.RuleId}, req.Filter.RuleIds)
					ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
					if tc.remote != "" {
						ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{RuleId: rule.RuleId, Content: "remote-content", Version: tc.remote}, Total: 1}
					}
					close(ch)
					return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch}
				},
				upload: func(ctx context.Context, token string, raw []byte, endpoint string) error {
					uploads++
					require.Equal(t, "valid-token", token)
					require.Equal(t, "api/flow/rule/upload", endpoint)
					var envelope yaklib.UploadOnlineRequest
					require.NoError(t, json.Unmarshal(raw, &envelope))
					var got schema.SyntaxFlowRule
					require.NoError(t, json.Unmarshal(envelope.Content, &got))
					require.Equal(t, rule.RuleId, got.RuleId)
					require.Equal(t, rule.Content, got.Content)
					require.Equal(t, tc.local, got.Version)
					return nil
				},
			}
			server := &Server{profileDatabase: db, onlineClient: remote}
			stream := &TestProgressStream{ctx: context.Background()}
			require.NoError(t, server.SyntaxFlowRuleToOnline(&ypb.SyntaxFlowRuleToOnlineRequest{Token: "valid-token", Filter: &ypb.SyntaxFlowRuleFilter{RuleIds: []string{rule.RuleId}}}, stream))
			require.Equal(t, tc.upload, uploads)
			require.Contains(t, stream.messages[len(stream.messages)-1].Message, tc.summary)
			for _, m := range stream.messages {
				require.GreaterOrEqual(t, m.Progress, 0.0)
				require.LessOrEqual(t, m.Progress, 1.0)
			}
			if tc.name == "conflict" {
				var conflict conflictInfo
				found := false
				for _, m := range stream.messages {
					if m.MessageType == string(DATA) {
						require.NoError(t, json.Unmarshal([]byte(m.Message), &conflict))
						found = true
					}
				}
				require.True(t, found)
				require.Equal(t, "local-content", conflict.Local)
				require.Equal(t, "remote-content", conflict.Remote)
			}
		})
	}
}

func TestDownloadSyntaxFlowRule(t *testing.T) {
	cases := []struct {
		name, local string
		dirty, save bool
		summary     string
	}{
		{"first download", "", false, true, "成功 1"},
		{"update clean", "20251015.0001", false, true, "成功 1"},
		{"local newer", "20251015.0003", false, false, "跳过 1"},
		{"conflict", "20251015.0001", true, false, "冲突 1"},
		{"force same", "20251015.0002", false, true, "成功 1"},
		{"dirty newer", "20251015.0003", true, false, "失败 1"},
		{"dirty same", "20251015.0002", true, false, "冲突 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newOnlineTestDB(t, &schema.SyntaxFlowRule{}, &schema.SyntaxFlowGroup{})
			name, id := uuid.NewString(), uuid.NewString()
			if tc.local != "" {
				require.NoError(t, db.Create(&schema.SyntaxFlowRule{RuleName: name, RuleId: id, Version: tc.local, NeedUpdate: tc.dirty, Content: "local-content"}).Error)
			}
			remote := &stubOnlineService{downloadRules: func(ctx context.Context, token string, req *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream {
				require.Equal(t, []string{id}, req.Filter.RuleIds)
				ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
				ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{RuleName: name, RuleId: id, Version: "20251015.0002", Content: "remote-content"}, Total: 1}
				close(ch)
				return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
			}}
			server := &Server{profileDatabase: db, onlineClient: remote}
			stream := &TestProgressStream{ctx: context.Background()}
			require.NoError(t, server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{Filter: &ypb.SyntaxFlowRuleFilter{RuleIds: []string{id}}}, stream))
			require.Contains(t, stream.messages[len(stream.messages)-1].Message, tc.summary)
			saved, err := sfdb.QueryRuleByRuleId(db, id)
			require.NoError(t, err)
			if tc.save {
				require.Equal(t, "remote-content", saved.Content)
				require.Equal(t, "20251015.0002", saved.Version)
				require.False(t, saved.NeedUpdate)
			} else {
				require.Equal(t, "local-content", saved.Content)
				require.Equal(t, tc.local, saved.Version)
				require.Equal(t, tc.dirty, saved.NeedUpdate)
			}
		})
	}
}

func TestSyntaxFlowOnlineFailures(t *testing.T) {
	db := newOnlineTestDB(t, &schema.SyntaxFlowRule{}, &schema.SyntaxFlowGroup{})
	require.NoError(t, db.Create(&schema.SyntaxFlowRule{RuleName: "failure", RuleId: "failure", Content: "content"}).Error)
	remote := &stubOnlineService{
		downloadRules: func(context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream {
			return nil
		},
		upload: func(context.Context, string, []byte, string) error { return errors.New("remote unavailable") },
	}
	server := &Server{profileDatabase: db, onlineClient: remote}
	stream := &TestProgressStream{ctx: context.Background()}
	require.NoError(t, server.SyntaxFlowRuleToOnline(&ypb.SyntaxFlowRuleToOnlineRequest{Token: "token"}, stream))
	require.Contains(t, stream.messages[len(stream.messages)-1].Message, "失败 1")
	require.Error(t, server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{}, stream))
	require.Error(t, server.SyntaxFlowRuleToOnline(&ypb.SyntaxFlowRuleToOnlineRequest{}, stream))
}

type TestProgressStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*ypb.SyntaxFlowRuleOnlineProgress
}

func (s *TestProgressStream) Context() context.Context { return s.ctx }
func (s *TestProgressStream) Send(m *ypb.SyntaxFlowRuleOnlineProgress) error {
	s.messages = append(s.messages, m)
	return nil
}
