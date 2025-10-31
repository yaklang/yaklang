package yakgrpc

import (
	"context"
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
		require.NoError(t, err)
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

func TestGRPCMUSTPASS_SyntaxFlow_Rule_ByTemplate(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("create rule by template - basic", func(t *testing.T) {
		ruleName := fmt.Sprintf("test_template_%s", uuid.NewString())

		req := &ypb.CreateSyntaxFlowRuleAutoRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleAutoInput{
				RuleName:        ruleName,
				Language:        "golang",
				RuleSubjects:    []string{"any() as $entry"},
				RuleSafeTests:   []string{"package main\n\nfunc safe() {}"},
				RuleUnSafeTests: []string{"package main\n\nfunc unsafe() {}"},
				RuleLevels:      []string{"high"},
				GroupNames:      []string{"test-group"},
				Description:     "Auto generated test rule",
			},
		}

		rsp, err := client.CreateSyntaxFlowRuleAuto(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, rsp)
		require.NotNil(t, rsp.Rule)
		require.Equal(t, ruleName, rsp.Rule.RuleName)
		require.Equal(t, "golang", rsp.Rule.Language)
		require.Contains(t, rsp.Rule.Content, "any() as $entry")
		require.Contains(t, rsp.Rule.Content, "type: audit")
		require.Contains(t, rsp.Rule.Content, "level: high")
		require.Contains(t, rsp.Rule.Content, "func safe()")
		require.Contains(t, rsp.Rule.Content, "func unsafe()")

		t.Cleanup(func() {
			deleteRuleByNames(client, []string{ruleName})
			deleteRuleGroup(client, []string{"test-group"})
		})

		queryRsp, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryRsp))
		require.Equal(t, ruleName, queryRsp[0].RuleName)
		require.Equal(t, "golang", queryRsp[0].Language)
		require.NotEqual(t, "", queryRsp[0].Id)
		require.Equal(t, "Auto generated test rule", queryRsp[0].Description)
	})

	t.Run("create rule by template - multiple subjects", func(t *testing.T) {
		ruleName := fmt.Sprintf("test_multi_subject_%s", uuid.NewString())

		req := &ypb.CreateSyntaxFlowRuleAutoRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleAutoInput{
				RuleName: ruleName,
				Language: "java",
				RuleSubjects: []string{
					"any() as $entry",
					"println(* #-> as $sink)",
				},
				RuleSafeTests: []string{
					"class Safe { void test() {} }",
					"class Safe2 { void test() {} }",
				},
				RuleUnSafeTests: []string{
					"class Unsafe { void test() {} }",
					"class Unsafe2 { void test() {} }",
				},
				RuleLevels: []string{"critical", "high"},
			},
		}

		rsp, err := client.CreateSyntaxFlowRuleAuto(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, rsp.Rule)

		require.Contains(t, rsp.Rule.Content, "any() as $entry")
		require.Contains(t, rsp.Rule.Content, "println(* #-> as $sink)")

		require.Contains(t, rsp.Rule.Content, "level: critical")

		require.Contains(t, rsp.Rule.Content, "class Safe")
		require.Contains(t, rsp.Rule.Content, "class Unsafe")
		require.Contains(t, rsp.Rule.Content, "class Safe2")
		require.Contains(t, rsp.Rule.Content, "class Unsafe2")

		t.Cleanup(func() {
			deleteRuleByNames(client, []string{ruleName})
		})

		queryRsp, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryRsp))
		require.Equal(t, ruleName, queryRsp[0].RuleName)
		require.Equal(t, "java", queryRsp[0].Language)
		require.NotEqual(t, "", queryRsp[0].Id)
	})

	t.Run("create rule by template - default values", func(t *testing.T) {
		ruleName := fmt.Sprintf("test_default_%s", uuid.NewString())

		req := &ypb.CreateSyntaxFlowRuleAutoRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleAutoInput{
				RuleName: ruleName,
				Language: "php",
			},
		}

		rsp, err := client.CreateSyntaxFlowRuleAuto(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, rsp.Rule)

		require.Contains(t, rsp.Rule.Content, "level: info")
		require.Contains(t, rsp.Rule.Content, "any() as $entry")
		require.Contains(t, rsp.Rule.Content, "risk: \"\"")
		require.Contains(t, rsp.Rule.Content, "rule_id:")

		t.Cleanup(func() {
			deleteRuleByNames(client, []string{ruleName})
		})

		queryRsp, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryRsp))
		require.Equal(t, ruleName, queryRsp[0].RuleName)
		require.Equal(t, "php", queryRsp[0].Language)
		require.NotEqual(t, "", queryRsp[0].Id)
	})

	t.Run("query and update rule created by template", func(t *testing.T) {
		ruleName := fmt.Sprintf("test_query_update_%s", uuid.NewString())

		createReq := &ypb.CreateSyntaxFlowRuleAutoRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleAutoInput{
				RuleName:     ruleName,
				Language:     "golang",
				RuleSubjects: []string{"any() as $test"},
				RuleLevels:   []string{"middle"},
			},
		}

		createRsp, err := client.CreateSyntaxFlowRuleAuto(context.Background(), createReq)
		require.NoError(t, err)
		originalRuleID := createRsp.GetRule().Id

		t.Cleanup(func() {
			deleteRuleByNames(client, []string{ruleName})
		})

		queryRsp, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryRsp))
		require.Equal(t, "golang", queryRsp[0].Language)
		require.Contains(t, queryRsp[0].Content, "any() as $test")

		updateReq := &ypb.UpdateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:    ruleName,
				Language:    "golang",
				Content:     "desc(title: \"updated\")\nany() as $updated",
				Description: "Updated description",
			},
		}

		_, err = client.UpdateSyntaxFlowRule(context.Background(), updateReq)
		require.NoError(t, err)

		updatedRsp, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, 1, len(updatedRsp))
		require.Contains(t, updatedRsp[0].Content, "any() as $updated")
		require.Equal(t, "Updated description", updatedRsp[0].Description)

		require.Equal(t, originalRuleID, updatedRsp[0].Id)
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
	t.Run("normal - new rules upload", func(t *testing.T) {
		ruleName1 := uuid.NewString()
		ruleName2 := uuid.NewString()
		ruleHash1 := uuid.NewString()
		ruleHash2 := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName1,
				RuleId:   ruleHash1,
				Content:  "aaa",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName2,
				RuleId:   ruleHash2,
				Content:  "bbb",
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++
			for i, r := range onlineRules {
				if r.RuleName == rule.RuleName {
					onlineRules[i] = rule
				} else {
					onlineRules = append(onlineRules, rule)
				}
			}
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]string, error) {
			ret := make(map[string]string)
			for _, r := range onlineRules {
				ret[r.RuleName] = r.Version
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName2},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, uploadCount)

		require.Equal(t, 2, len(onlineRules))
		byName := make(map[string]string)
		for _, r := range onlineRules {
			byName[r.RuleName] = r.Content
		}
		require.Equal(t, "aaa", byName[ruleName1])
		require.Equal(t, "bbb", byName[ruleName2])
	})

	t.Run("skip upload - online version is newer", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleHash := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleHash,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RuleId:   ruleHash,
				Content:  "bbb",
				Version:  "20251015.0001",
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++

			for i, r := range onlineRules {
				if r.RuleName == rule.RuleName {
					onlineRules[i] = rule
				} else {
					onlineRules = append(onlineRules, rule)
				}
			}
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]string, error) {
			ret := make(map[string]string)
			for _, r := range onlineRules {
				ret[r.RuleName] = r.Version
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 0, uploadCount)

		require.Equal(t, 1, len(onlineRules))
		byName := make(map[string]string)
		for _, r := range onlineRules {
			byName[r.RuleName] = r.Content
		}
		require.Equal(t, "aaa", byName[ruleName])
	})

	t.Run("upload - local version is newer", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleHash := uuid.NewString()
		onlineRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RiskType: ruleHash,
				Content:  "aaa",
				Version:  "20251015.0002",
			},
		}

		testRules := []*schema.SyntaxFlowRule{
			{
				RuleName: ruleName,
				RiskType: ruleHash,
				Content:  "bbb",
				Version:  "20251015.0003",
			},
		}

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard1.UnPatch()

		uploadCount := 0
		guard2 := mockey.Mock(uploadRule).To(func(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
			uploadCount++

			for i, r := range onlineRules {
				if r.RuleName == rule.RuleName {
					onlineRules[i] = rule
				} else {
					onlineRules = append(onlineRules, rule)
				}
			}
			return nil
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
			return testRules, nil
		}).Build()
		defer guard3.UnPatch()

		guard4 := mockey.Mock(fetchRemoteRuleVersionMap).To(func(context.Context, *yaklib.OnlineClient, string, []string) (map[string]string, error) {
			ret := make(map[string]string)
			for _, r := range onlineRules {
				ret[r.RuleName] = r.Version
			}
			return ret, nil
		}).Build()
		defer guard4.UnPatch()

		server := &Server{}
		req := &ypb.SyntaxFlowRuleToOnlineRequest{
			Token: "valid-token",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
		}
		stream := &TestProgressStream{ctx: context.Background()}

		err := server.SyntaxFlowRuleToOnline(req, stream)
		assert.NoError(t, err)
		require.Equal(t, 1, uploadCount)

		require.Equal(t, 1, len(onlineRules))
		byName := make(map[string]string)
		for _, r := range onlineRules {
			byName[r.RuleName] = r.Content
		}
		require.Equal(t, "bbb", byName[ruleName])
	})
}

func TestDownloadSyntaxFlowRule(t *testing.T) {
	buildRule := func(ruleName, ruleId, version string) {
		rule, err := sfdb.CheckSyntaxFlowRuleContent("aaa")
		rule.RuleName = ruleName
		rule.RuleId = ruleId
		rule.Version = version
		require.NoError(t, err)
		err = sfdb.MigrateSyntaxFlow(rule.CalcHash(), rule)
		require.NoError(t, err)
	}

	t.Run("normal - new rules download", func(t *testing.T) {
		ruleName1 := uuid.NewString()
		ruleName2 := uuid.NewString()
		ruleHash1 := uuid.NewString()
		ruleHash2 := uuid.NewString()

		guard1 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard1.UnPatch()

		guard2 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 2)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName1,
				RuleId:   ruleHash1,
			}, Total: 2}
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName2,
				RuleId:   ruleHash2,
			}, Total: 2}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 2}
		}).Build()
		defer guard2.UnPatch()

		guard3 := mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
			return nil
		}).Build()
		defer guard3.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{}, stream)
		assert.NoError(t, err)
	})

	t.Run("skip download - local version is newer", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleHash := uuid.NewString()
		buildRule(ruleName, ruleHash, "20251015.0002")
		defer sfdb.DeleteRuleByRuleName(ruleName)

		guard4 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard4.UnPatch()

		guard5 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleHash,
				Version:  "20251015.0001",
				Content:  "bbb",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard5.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{}, stream)
		assert.NoError(t, err)

		localRule, err := sfdb.QueryRuleByName(consts.GetGormProfileDatabase(), ruleName)
		assert.NoError(t, err)
		require.Equal(t, localRule.Content, "aaa")
	})

	t.Run("download - online version is newer", func(t *testing.T) {
		ruleName := uuid.NewString()
		ruleHash := uuid.NewString()
		buildRule(ruleName, ruleHash, "20251015.0002")
		defer sfdb.DeleteRuleByRuleName(ruleName)

		guard7 := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
			assert.True(t, progress >= 0 && progress <= 1)
			log.Info(msg)
		}).Build()
		defer guard7.UnPatch()

		guard8 := mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
			*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
		) *yaklib.OnlineDownloadFlowRuleStream {
			ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 1)
			ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{
				RuleName: ruleName,
				RuleId:   ruleHash,
				Version:  "20251015.0003",
				Content:  "ccc",
			}, Total: 1}
			close(ch)
			return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 1}
		}).Build()
		defer guard8.UnPatch()

		server := &Server{}
		stream := &TestProgressStream{ctx: context.Background()}
		err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{}, stream)
		assert.NoError(t, err)

		localRule, err := sfdb.QueryRuleByName(consts.GetGormProfileDatabase(), ruleName)
		assert.NoError(t, err)
		require.Equal(t, localRule.Content, "ccc")
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
