//go:build !irify_exclude

package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func createSfRuleWithTag(client ypb.YakClient, ruleName, tags string) error {
	rule := &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Tags:     []string{tags},
			Language: "java",
		},
	}
	_, err := client.CreateSyntaxFlowRule(context.Background(), rule)
	return err
}

func TestGRPCMUSTPASS_SyntaxFlow_Export_And_Import(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	wantRulesCount := 16
	ruleNames := make([]string, 0, wantRulesCount)
	// one exclusive group for all rules (one-group-per-rule model)
	groupName := fmt.Sprintf("group_%s", uuid.NewString())
	err = createGroups(client, []string{groupName})
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteRuleGroup(client, []string{groupName})
	})

	// create rules
	tag := uuid.NewString()
	for i := 0; i < wantRulesCount; i++ {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		err = createSfRuleWithTag(client, ruleName, tag)
		ruleNames = append(ruleNames, ruleName)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		deleteRuleByNames(client, ruleNames)
	})
	err = addGroups(client, ruleNames, []string{groupName})
	require.NoError(t, err)

	exportAndImportTest := func(t *testing.T, importRequest *ypb.ImportSyntaxFlowsRequest, exportRequest *ypb.ExportSyntaxFlowsRequest) {
		t.Helper()
		// export
		ctx := utils.TimeoutContextSeconds(10)
		exportStream, err := client.ExportSyntaxFlows(ctx, exportRequest)
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := exportStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("export stream error: %v", err)
				}
				break
			}
			progress = msg.Progress
		}
		require.Equal(t, 1.0, progress)
		// delete, for test import
		deleteRuleGroup(client, []string{groupName})
		deleteRuleByNames(client, ruleNames)

		// import
		importStream, err := client.ImportSyntaxFlows(ctx, importRequest)
		require.NoError(t, err)
		progress = 0.0
		for {
			msg, err := importStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("import stream error: %v", err)
				}
				break
			}
			progress = msg.Progress
		}
		require.Equal(t, 1.0, progress)

		// check rules
		rules, err := queryRulesByName(client, ruleNames)
		require.NoError(t, err)
		require.Len(t, rules, wantRulesCount)
		for _, rule := range rules {
			require.Equal(t, tag, rule.GetTag())
		}
		count, err := queryRuleGroupCount(client, groupName)
		require.NoError(t, err)
		require.Equal(t, wantRulesCount, count)
	}

	t.Run("no password", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "export.zip")
		exportAndImportTest(t, &ypb.ImportSyntaxFlowsRequest{
			InputPath: p,
		}, &ypb.ExportSyntaxFlowsRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				GroupNames: []string{groupName},
			},
			TargetPath: p,
		})
	})

	t.Run("password", func(t *testing.T) {
		password := uuid.NewString()
		p := filepath.Join(t.TempDir(), "export.zip.enc")
		exportAndImportTest(t, &ypb.ImportSyntaxFlowsRequest{
			InputPath: p,
			Password:  password,
		}, &ypb.ExportSyntaxFlowsRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				GroupNames: []string{groupName},
			},
			TargetPath: p,
			Password:   password,
		})
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Override_BuildIn_Rule(t *testing.T) {
	rule_id := uuid.NewString()
	rule_name := uuid.NewString()
	content := `a as $a`
	group := uuid.NewString()
	db := consts.GetGormProfileDatabase()
	client, err := NewLocalClient()
	require.NoError(t, err)
	// create rule
	rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	rule.RuleId = rule_id
	rule.RuleName = rule_name
	rule.IsBuildInRule = true
	createdRule, err := sfdb.CreateRule(rule)
	require.NoError(t, err)
	t.Cleanup(func() {
		sfdb.DeleteRuleByRuleName(rule.RuleName)
	})
	// add group
	_, err = sfdb.BatchAddGroupsForRulesByRuleId(db, []string{rule_id}, []string{group})
	require.NoError(t, err)
	// export
	exportPath := filepath.Join(t.TempDir(), "export.zip")
	ctx := utils.TimeoutContextSeconds(10)
	exportStream, err := client.ExportSyntaxFlows(ctx, &ypb.ExportSyntaxFlowsRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{rule_name},
		},
		TargetPath: exportPath,
	})
	require.NoError(t, err)
	progress := 0.0
	for {
		msg, err := exportStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("export stream error: %v", err)
			}
			break
		}
		progress = msg.Progress
	}
	require.Equal(t, 1.0, progress)

	// update rule
	newContent := `b as $b`
	createdRule.Content = newContent
	createdRule.RuleGroup = group // keep exclusive group; UpdateRule writes full struct
	err = sfdb.UpdateRule(db, createdRule)
	require.NoError(t, err)
	// delete rule group
	// 删除的组在导入时会被重新添加
	count, err := sfdb.BatchRemoveGroupsForRulesById(db, []string{rule_id}, []string{group})
	require.Equal(t, int64(1), count)
	require.NoError(t, err)

	// import and override rule
	importStream, err := client.ImportSyntaxFlows(ctx, &ypb.ImportSyntaxFlowsRequest{
		InputPath: exportPath,
	})
	require.NoError(t, err)
	progress = 0.0
	for {
		msg, err := importStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("import stream error: %v", err)
			}
			break
		}
		progress = msg.Progress
	}
	require.Equal(t, 1.0, progress)
	// check rule
	rules, err := sfdb.QueryRulesById(db, []string{rule_id})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, rules[0].Content, content)
	// check rule groups
	groups, err := sfdb.QueryGroupByRuleIds(db, []string{rule.RuleId})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, groups[0].GroupName, group)
}

func TestGRPCMUSTPASS_SyntaxFlow_Override_Rule_Group(t *testing.T) {
	rule_id := uuid.NewString()
	rule_name := uuid.NewString()
	content := `a as $a`
	group := uuid.NewString()
	db := consts.GetGormProfileDatabase()
	client, err := NewLocalClient()
	require.NoError(t, err)
	// create rule
	rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	rule.RuleId = rule_id
	rule.RuleName = rule_name
	rule.IsBuildInRule = true
	createdRule, err := sfdb.CreateRule(rule)
	require.NoError(t, err)
	t.Cleanup(func() {
		sfdb.DeleteRuleByRuleName(rule.RuleName)
	})
	// add group
	_, err = sfdb.BatchAddGroupsForRulesByRuleId(db, []string{rule_id}, []string{group})
	require.NoError(t, err)
	t.Cleanup(func() {
		sfdb.DeleteGroup(db, group)
	})
	// export
	exportPath := filepath.Join(t.TempDir(), "export.zip")
	ctx := utils.TimeoutContextSeconds(10)
	exportStream, err := client.ExportSyntaxFlows(ctx, &ypb.ExportSyntaxFlowsRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{rule_name},
		},
		TargetPath: exportPath,
	})
	require.NoError(t, err)
	progress := 0.0
	for {
		msg, err := exportStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("export stream error: %v", err)
			}
			break
		}
		progress = msg.Progress
	}
	require.Equal(t, 1.0, progress)

	// update rule
	newContent := `b as $b`
	createdRule.Content = newContent
	createdRule.RuleGroup = group
	err = sfdb.UpdateRule(db, createdRule)
	require.NoError(t, err)
	// update rule group: exclusive model → last group wins (count == 1)
	newGroup1 := uuid.NewString()
	newGroup2 := uuid.NewString()
	count, err := sfdb.BatchAddGroupsForRulesByRuleId(db, []string{rule_id}, []string{newGroup1, newGroup2})
	require.Equal(t, int64(1), count)
	require.NoError(t, err)
	t.Cleanup(func() {
		sfdb.DeleteGroup(db, newGroup1)
		sfdb.DeleteGroup(db, newGroup2)
	})

	// import and override rule
	importStream, err := client.ImportSyntaxFlows(ctx, &ypb.ImportSyntaxFlowsRequest{
		InputPath: exportPath,
	})
	require.NoError(t, err)
	progress = 0.0
	for {
		msg, err := importStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("import stream error: %v", err)
			}
			break
		}
		progress = msg.Progress
	}
	require.Equal(t, 1.0, progress)
	// check rule
	rules, err := sfdb.QueryRulesById(db, []string{rule_id})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, rules[0].Content, content)
	// import restores exported exclusive group
	groups, err := sfdb.QueryGroupByRuleIds(db, []string{rule.RuleId})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, group, groups[0].GroupName)
}
