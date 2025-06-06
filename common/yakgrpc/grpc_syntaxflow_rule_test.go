package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"google.golang.org/grpc"
	"testing"

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
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
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

func TestSyntaxFlowRuleToOnline(t *testing.T) {
	guard := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
		assert.True(t, progress >= 0 && progress <= 1)
	}).Build()
	defer guard.UnPatch()

	testRules := []*schema.SyntaxFlowRule{
		{RuleName: "test-rule-1", IsBuildInRule: false},
		{RuleName: "test-rule-2", IsBuildInRule: false},
	}

	mockey.Mock(yakit.AllSyntaxFlowRule).To(func(*gorm.DB, *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
		return testRules, nil
	}).Build()

	mockey.Mock(uploadRule).To(func(context.Context, *yaklib.OnlineClient, string, *schema.SyntaxFlowRule) error {
		return nil
	}).Build()

	server := &Server{}
	req := &ypb.SyntaxFlowRuleToOnlineRequest{Token: "valid-token"}
	stream := &TestProgressStream{ctx: context.Background()}

	err := server.SyntaxFlowRuleToOnline(req, stream)
	assert.NoError(t, err)
}

func TestDownloadSyntaxFlowRule(t *testing.T) {
	guard := mockey.Mock(sendProgress).To(func(stream ProgressStream, progress float64, msg, msgType string) {
		assert.True(t, progress >= 0 && progress <= 1)
	}).Build()
	defer guard.UnPatch()

	mockey.Mock((*yaklib.OnlineClient).DownloadOnlineSyntaxFlowRule).To(func(
		*yaklib.OnlineClient, context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest,
	) *yaklib.OnlineDownloadFlowRuleStream {
		ch := make(chan *yaklib.OnlineSyntaxFlowRuleItem, 2)
		ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{RuleName: "rule1"}}
		ch <- &yaklib.OnlineSyntaxFlowRuleItem{Rule: &yaklib.OnlineSyntaxFlowRule{RuleName: "rule2"}}
		close(ch)
		return &yaklib.OnlineDownloadFlowRuleStream{Chan: ch, Total: 2}
	}).Build()

	mockey.Mock((*yaklib.OnlineClient).SaveSyntaxFlowRule).To(func(*yaklib.OnlineClient, *gorm.DB, ...*yaklib.OnlineSyntaxFlowRule) error {
		return nil
	}).Build()

	server := &Server{}
	stream := &TestProgressStream{ctx: context.Background()}

	err := server.DownloadSyntaxFlowRule(&ypb.DownloadSyntaxFlowRuleRequest{}, stream)
	assert.NoError(t, err)
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
