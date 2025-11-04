package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
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
	// 场景1: 远程规则不存在 → 上传规则（首次发布）
	t.Run("upload - remote not exists (first publish)", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleId,
				Content:  "bbb",
				Version:  "20251015.0001",
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			onlineRules = append(onlineRules, rule)
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
			ret := make(map[string]*yaklib.OnlineSyntaxFlowRule)
			for _, r := range onlineRules {
				ret[r.RuleId] = &yaklib.OnlineSyntaxFlowRule{
					RuleName: r.RuleName,
					RuleId:   r.RuleId,
					Content:  r.Content,
					Version:  r.Version,
				}
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, uploadCount)
		require.Equal(t, 1, len(onlineRules))
		require.Equal(t, "bbb", onlineRules[0].Content)
	})

	// 场景2: 远程存在，本地 v3.0 > 远程 v2.0，NeedUpdate=true → 覆盖上传
	t.Run("upload - local version newer with NeedUpdate=true", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleId,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName:   ruleName,
				RuleId:     ruleId,
				Content:    "bbb-modified",
				Version:    "20251015.0003",
				NeedUpdate: true, // 本地有修改
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			for i, r := range onlineRules {
				if r.RuleName == rule.RuleName {
					onlineRules[i] = rule
					break
				}
			}
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
			ret := make(map[string]*yaklib.OnlineSyntaxFlowRule)
			for _, r := range onlineRules {
				ret[r.RuleId] = &yaklib.OnlineSyntaxFlowRule{
					RuleName: r.RuleName,
					RuleId:   r.RuleId,
					Content:  r.Content,
					Version:  r.Version,
				}
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, uploadCount)
		require.Equal(t, "bbb-modified", onlineRules[0].Content)
	})

	// 场景3: 远程存在，本地 v3.0 > 远程 v2.0，NeedUpdate=false → 逻辑错误
	t.Run("error - local version newer with NeedUpdate=false", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleId,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName:   ruleName,
				RuleId:     ruleId,
				Content:    "bbb",
				Version:    "20251015.0003",
				NeedUpdate: false, // 逻辑错误：版本更新但没有修改标记
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
			ret := make(map[string]*yaklib.OnlineSyntaxFlowRule)
			for _, r := range onlineRules {
				ret[r.RuleId] = &yaklib.OnlineSyntaxFlowRule{
					RuleName: r.RuleName,
					RuleId:   r.RuleId,
					Content:  r.Content,
					Version:  r.Version,
				}
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, uploadCount) // 不应该上传
	})

	// 场景4: 远程存在，本地 v1.0 < 远程 v2.0，NeedUpdate=true → 冲突-跳过
	t.Run("conflict - remote version newer with NeedUpdate=true", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()

		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleId,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName:   ruleName,
				RuleId:     ruleId,
				Content:    "bbb-modified",
				Version:    "20251015.0001",
				NeedUpdate: true, // 本地有修改，但远程版本更新
			},
		}

		var conflictInfo conflictInfo
		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
			if msgType == string(DATA) {
				err := json.Unmarshal([]byte(msg), &conflictInfo)
				if err != nil {
					t.Fatal(err)
				}
			}
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
			ret := make(map[string]*yaklib.OnlineSyntaxFlowRule)
			for _, r := range onlineRules {
				ret[r.RuleId] = &yaklib.OnlineSyntaxFlowRule{
					RuleName: r.RuleName,
					RuleId:   r.RuleId,
					Content:  r.Content,
					Version:  r.Version,
				}
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, uploadCount) // 冲突，跳过上传
		require.Equal(t, conflictInfo.Local, testRules[0].Content)
		require.Equal(t, conflictInfo.Remote, onlineRules[0].Content)
	})

	// 场景5: 远程存在，本地 v1.0 < 远程 v2.0，NeedUpdate=false → 跳过上传（提示需要更新）
	t.Run("skip - remote version newer with NeedUpdate=false", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleId,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName:   ruleName,
				RuleId:     ruleId,
				Content:    "bbb",
				Version:    "20251015.0001",
				NeedUpdate: false, // 本地无修改，远程版本更新
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
			ret := make(map[string]*yaklib.OnlineSyntaxFlowRule)
			for _, r := range onlineRules {
				ret[r.RuleId] = &yaklib.OnlineSyntaxFlowRule{
					RuleName: r.RuleName,
					RuleId:   r.RuleId,
					Content:  r.Content,
					Version:  r.Version,
				}
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, uploadCount) // 跳过上传
	})
}

func TestDownloadSyntaxFlowRule(t *testing.T) {
	buildRule := func(ruleName, ruleId, version string, needUpdate bool) {
		rule, err := sfdb.CheckSyntaxFlowRuleContent("aaa")
		rule.RuleName = ruleName
		rule.RuleId = ruleId
		rule.Version = version
		rule.NeedUpdate = needUpdate
		rule.Content = "local-content"
		require.NoError(t, err)
		err = sfdb.MigrateSyntaxFlow(rule.CalcHash(), rule)
		require.NoError(t, err)
	}

	// 场景1: 本地规则不存在 → 下载规则（首次下载）
	t.Run("download - local not exists (first download)", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		saveCount := 0
		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleId,
				Version:  "20251015.0001",
				Content:  "new-content",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			saveCount++
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, saveCount)
	})

	// 场景2: 本地存在，在线 v2.0 > 本地 v1.0，NeedUpdate=false → 更新规则
	t.Run("download - online version newer with NeedUpdate=false", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		buildRule(ruleName, ruleId, "20251015.0001", false)
		defer sfdb.DeleteRuleByRuleName(ruleName)

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		saveCount := 0
		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleId,
				Version:  "20251015.0002",
				Content:  "updated-content",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			saveCount++
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, saveCount)
	})

	// 场景3: 本地存在，本地 v3.0 > 在线 v2.0，NeedUpdate=false → 跳过更新
	t.Run("skip - local version newer with NeedUpdate=false", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		buildRule(ruleName, ruleId, "20251015.0003", false)
		defer sfdb.DeleteRuleByRuleName(ruleName)

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		saveCount := 0
		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleId,
				Version:  "20251015.0002",
				Content:  "old-content",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			saveCount++
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, saveCount) // 跳过下载
	})

	// 场景4: 本地存在，在线 v2.0 >= 本地 v1.0/v2.0，NeedUpdate=true → 冲突-跳过
	t.Run("conflict - online version newer with NeedUpdate=true", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		buildRule(ruleName, ruleId, "20251015.0001", true) // 本地有修改
		defer sfdb.DeleteRuleByRuleName(ruleName)

		var conflictInfo conflictInfo
		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
			if msgType == string(DATA) {
				err := json.Unmarshal([]byte(msg), &conflictInfo)
				if err != nil {
					t.Fatal(err)
				}
			}
		}).Build()
		defer guard1.UnPatch()

		saveCount := 0
		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleId,
				Version:  "20251015.0002",
				Content:  "online-content",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			saveCount++
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, saveCount) // 冲突，跳过下载
		require.Equal(t, conflictInfo.Local, "local-content")
		require.Equal(t, conflictInfo.Remote, "online-content")
	})

	// 场景5: 本地存在，本地 v3.0 > 在线 v2.0，NeedUpdate=true → 逻辑错误
	t.Run("error - local version newer with NeedUpdate=true", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleId := uuid.NewString()
		buildRule(ruleName, ruleId, "20251015.0003", true) // 逻辑错误：本地版本更新但有修改标记
		defer sfdb.DeleteRuleByRuleName(ruleName)

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			log.Infof("[%s] %s", msgType, msg)
		}).Build()
		defer guard1.UnPatch()

		saveCount := 0
		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleId,
				Version:  "20251015.0002",
				Content:  "old-content",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			saveCount++
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleIds: []string{ruleId},
			},
		}, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, saveCount) // 逻辑错误，跳过下载
	})
}

type TestProgressStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *TestProgressStream) Context() context.Context {
	return s.ctx
}

func (s *TestProgressStream) Send(*ypb.SyntaxFlowRuleOnlineProgress) error {
	return nil
}
