package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
	wantGroupsCount := 16
	ruleNames := make([]string, 0, wantRulesCount)
	groupNames := make([]string, 0, wantGroupsCount)
	// create groups
	for i := 0; i < wantGroupsCount; i++ {
		groupName := fmt.Sprintf("group_%s", uuid.NewString())
		groupNames = append(groupNames, groupName)
	}
	err = createGroups(client, groupNames)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteRuleGroup(client, groupNames)
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
	err = addGroups(client, ruleNames, groupNames)
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
		deleteRuleGroup(client, groupNames)
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
		// check rule groups
		for _, groupName := range groupNames {
			count, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, wantRulesCount, count)
		}
	}

	t.Run("no password", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "export.zip")
		exportAndImportTest(t, &ypb.ImportSyntaxFlowsRequest{
			InputPath: p,
		}, &ypb.ExportSyntaxFlowsRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				GroupNames: groupNames,
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
				GroupNames: groupNames,
			},
			TargetPath: p,
			Password:   password,
		})
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_ImportOverWrite(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()

	groupName := fmt.Sprintf("group_%s", uuid.NewString())
	err = createGroups(client, []string{groupName})
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteRuleGroup(client, []string{groupName})
	})

	ruleName := uuid.NewString()
	tag := uuid.NewString()
	content := "a as $a"
	_, err = client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Tags:     []string{tag},
			Language: "java",
			Content:  content,
		},
	})

	require.NoError(t, err)
	t.Cleanup(func() {
		deleteRuleByNames(client, []string{ruleName})
	})
	err = addGroups(client, []string{ruleName}, []string{groupName})
	require.NoError(t, err)

	//export
	p := filepath.Join(t.TempDir(), "export.zip")
	exportStream, err := client.ExportSyntaxFlows(ctx, &ypb.ExportSyntaxFlowsRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{ruleName},
		},
		TargetPath: p,
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

	//update rule
	fakeGroupName := fmt.Sprintf("group_%s", uuid.NewString())
	fakeTag := uuid.NewString()
	fakeContent := "b as $b;"
	_, err = client.UpdateSyntaxFlowRuleEx(ctx, &ypb.UpdateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName:   ruleName,
			Language:   "java",
			GroupNames: []string{fakeGroupName},
			Tags:       []string{fakeTag},
			Content:    fakeContent,
		},
	})
	require.NoError(t, err)

	// import overwrite
	importStream, err := client.ImportSyntaxFlows(ctx, &ypb.ImportSyntaxFlowsRequest{
		InputPath: p,
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
	// check rules
	rules, err := queryRulesByName(client, []string{ruleName})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	spew.Dump(rules[0])
	require.Contains(t, rules[0].GroupName, groupName)
	require.Contains(t, rules[0].Tag, tag)
	require.Contains(t, rules[0].Content, content)

	require.NotContains(t, rules[0].Tag, fakeTag)
	require.NotContains(t, rules[0].Content, fakeContent)
	require.NotContains(t, rules[0].GroupName, fakeGroupName)
}
