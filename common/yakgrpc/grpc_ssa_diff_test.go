package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_SSAReusltDiff(t *testing.T) {
	code := `<?php
$a = $_GET[1];
eval($a);
`
	baseProg := uuid.NewString()
	newProg := uuid.NewString()
	rulename := uuid.NewString()
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建规则
	client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content: `eval() as $sink 
alert $sink for{
level: high
}`,
			RuleName: rulename,
			Language: "php",
		},
	})
	defer func() {
		yakit.DeleteSSAProgram(consts.GetGormDefaultSSADataBase(), &ypb.SSAProgramFilter{
			ProgramNames: []string{baseProg},
		})
	}()

	fs := filesys.NewVirtualFs()
	fs.AddFile("test.php", code)
	program, err2 := ssaapi.ParseProjectWithFS(fs,
		ssaapi.WithLanguage(ssaapi.PHP),
		ssaapi.WithProgramName(baseProg),
	)
	require.NoError(t, err2)
	result, err2 := program.SyntaxFlowRuleName(rulename, ssaapi.QueryWithSave(schema.SFResultKindScan))
	require.NoError(t, err2)
	result.Show()

	t.Run("base compare need equal", func(t *testing.T) {
		defer func() {
			yakit.DeleteSSAProgram(consts.GetGormDefaultSSADataBase(), &ypb.SSAProgramFilter{
				ProgramNames: []string{newProg},
			})
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{rulename},
				},
			})
		}()
		virtualFs := filesys.NewVirtualFs()
		virtualFs.AddFile("tt.php", code)
		program, err2 := ssaapi.ParseProjectWithFS(fs,
			ssaapi.WithLanguage(ssaapi.PHP),
			ssaapi.WithProgramName(newProg),
		)
		require.NoError(t, err2)
		result, err2 := program.SyntaxFlowRuleName(rulename, ssaapi.QueryWithSave(schema.SFResultKindScan))
		require.NoError(t, err2)
		result.Show()

		diff, err := client.SSARiskDiff(context.Background(), &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{ProgramName: baseProg},
			Compare:  &ypb.SSARiskDiffItem{ProgramName: newProg},
			Type:     "risk",
		})
		require.NoError(t, err)
		flag := false
		for {
			recv, err := diff.Recv()
			spew.Dump(recv)
			if err != nil {
				break
			}
			if recv.Status == "equal" {
				flag = true
				break
			}
		}
		require.True(t, flag)
	})
	t.Run("base compare for singal rule", func(t *testing.T) {
		defer func() {
			yakit.DeleteSSAProgram(consts.GetGormDefaultSSADataBase(), &ypb.SSAProgramFilter{
				ProgramNames: []string{newProg},
			})
		}()
		virtualfs := filesys.NewVirtualFs()
		virtualfs.AddFile("aa.php", `<?php
include($_GET[1]);
`)
		program, err := ssaapi.ParseProjectWithFS(virtualfs, ssaapi.WithLanguage(ssaapi.PHP),
			ssaapi.WithProgramName(newProg))
		require.NoError(t, err)
		require.NoError(t, err2)
		result, err2 := program.SyntaxFlowRuleName(`检测PHP代码执行漏洞`, ssaapi.QueryWithSave(schema.SFResultKindScan))
		program.SyntaxFlowRuleName("审计PHP文件包含漏洞", ssaapi.QueryWithSave(schema.SFResultKindScan))
		require.NoError(t, err2)
		result.Show()
		diff, err := client.SSARiskDiff(context.Background(), &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{
				ProgramName: baseProg,
				RuleName:    "检测PHP代码执行漏洞",
			},
			Compare: &ypb.SSARiskDiffItem{
				ProgramName: newProg,
				RuleName:    "检测PHP代码执行漏洞",
			},
			Type: "risk",
		})
		require.NoError(t, err)
		for {
			recv, err := diff.Recv()
			if err != nil {
				break
			}
			spew.Dump(recv)
			require.True(t, recv.RuleName == "检测PHP代码执行漏洞")
		}
	})
}
