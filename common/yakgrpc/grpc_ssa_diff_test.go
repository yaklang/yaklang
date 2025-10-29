package yakgrpc

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_SSAReusltDiff(t *testing.T) {
	// 已弃用：现在不使用ProgramName进行diff对比
	t.Skip()
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
		ssaapi.WithLanguage(ssaconfig.PHP),
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
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormDefaultSSADataBase(), []string{baseProg, newProg}, schema.Program)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormDefaultSSADataBase(), []string{baseProg, newProg}, schema.Program)
			yakit.DeleteSSADiffResultByRule(consts.GetGormDefaultSSADataBase(), []string{rulename})
		}()
		virtualFs := filesys.NewVirtualFs()
		virtualFs.AddFile("tt.php", code)
		program, err2 := ssaapi.ParseProjectWithFS(fs,
			ssaapi.WithLanguage(ssaconfig.PHP),
			ssaapi.WithProgramName(newProg),
		)
		require.NoError(t, err2)
		result, err2 := program.SyntaxFlowRuleName(rulename, ssaapi.QueryWithSave(schema.SFResultKindScan))
		require.NoError(t, err2)
		result.Show()

		diff, err := client.SSARiskDiff(context.Background(), &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{ProgramName: baseProg},
			Compare:  &ypb.SSARiskDiffItem{ProgramName: newProg},
		})
		require.NoError(t, err)
		flag := false
		for {
			recv, err := diff.Recv()
			spew.Dump(recv)
			if err != nil {
				break
			}
			if recv.Status == string(yakit.Equal) {
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
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormDefaultSSADataBase(), []string{baseProg, newProg}, schema.Program)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormDefaultSSADataBase(), []string{baseProg, newProg}, schema.Program)
		}()
		virtualfs := filesys.NewVirtualFs()
		virtualfs.AddFile("aa.php", `<?php
include($_GET[1]);
`)
		program, err := ssaapi.ParseProjectWithFS(virtualfs, ssaapi.WithLanguage(ssaconfig.PHP),
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

func TestGRPCMUSTPASS_SyntaxFlow_SSAReusltCompareWithTaskId(t *testing.T) {
	createTask := func(t *testing.T, program string) string {
		taskID := uuid.NewString()
		// task := &schema.SyntaxFlowScanTask{
		// 	TaskId:    taskID,
		// 	Programs:  program,
		// 	RiskCount: 10,
		// }
		// err := schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), task)
		// require.NoError(t, err)
		return taskID
	}

	baseProg := uuid.NewString()
	rulename := uuid.NewString()
	rulename2 := uuid.NewString()
	taskID1 := createTask(t, baseProg)
	taskID2 := createTask(t, baseProg)
	client, err := NewLocalClient()
	require.NoError(t, err)

	fs := filesys.NewVirtualFs()
	fs.AddFile("test.go", `package main

import (
    "fmt"
    "io/ioutil"
    "net/http"
    "path/filepath"
    "strings"
)

const allowedBasePath = "/allowed/path/"

func handler(w http.ResponseWriter, r *http.Request) {
    userInput := r.URL.Query().Get("file")

    requestedPath := filepath.Join(allowedBasePath, userInput)
    cleanedPath := filepath.Clean(requestedPath)

    if !strings.HasPrefix(cleanedPath, allowedBasePath) {
        http.Error(w, "Invalid file path", http.StatusBadRequest)
        return
    }

    content, err := ioutil.ReadFile(cleanedPath)
    if err != nil {
        http.Error(w, "File not found", http.StatusNotFound)
        return
    }

    w.Write(content)
}

func main() {
    http.HandleFunc("/", handler)
    fmt.Println("Server is running on :8080")
    http.ListenAndServe(":8080", nil)
}
`)
	program, err := ssaapi.ParseProjectWithFS(fs,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramName(baseProg),
	)
	require.NoError(t, err)
	client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content: `
ioutil?{<fullTypeName>?{have: 'io/ioutil'}} as $entry
$entry.ReadAll(* #-> as $sink) 
$entry.ReadFile(* #-> as $sink)

$sink?{have: 'Parameter'} as $high;
alert $high for{
level: high
}`,
			RuleName: rulename,
			Language: "golang",
		},
	})
	defer func() {
		client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{rulename},
			},
		})
		yakit.DeleteSSAProgram(consts.GetGormDefaultSSADataBase(), &ypb.SSAProgramFilter{
			ProgramNames: []string{baseProg},
		})
	}()

	t.Run("taskid compare", func(t *testing.T) {
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
ioutil?{<fullTypeName>?{have: 'io/ioutil'}} as $entry
$entry.ReadAll(* #-> as $sink) 
$entry.ReadFile(* #-> as $sink)

$sink #-> as $high;
alert $high for{
level: high
}`,
				RuleName: rulename2,
				Language: "golang",
			},
		})
		defer func() {
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{rulename2},
				},
			})
		}()

		result, err := program.SyntaxFlowRuleName(rulename, ssaapi.QueryWithSave(schema.SFResultKindDebug))
		result.Save(schema.SFResultKindDebug, taskID1)
		result.Show()
		require.NoError(t, err)

		result2, err := program.SyntaxFlowRuleName(rulename2, ssaapi.QueryWithSave(schema.SFResultKindDebug))
		require.NoError(t, err)
		result2.Show()
		result2.Save(schema.SFResultKindDebug, taskID2)

		diff, err := client.SSARiskDiff(context.Background(), &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{RiskRuntimeId: taskID1},
			Compare:  &ypb.SSARiskDiffItem{RiskRuntimeId: taskID2},
		})
		require.NoError(t, err)
		flag := false
		for {
			recv, err := diff.Recv()
			if err != nil {
				break
			}
			if recv.Status == string(yakit.Del) {
				flag = true
				break
			}
		}
		require.True(t, flag)
	})

	t.Run("taskid compare with db", func(t *testing.T) {
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
ioutil?{<fullTypeName>?{have: 'io/ioutil'}} as $entry
$entry.ReadAll(* #-> as $sink) 
$entry.ReadFile(* #-> as $sink)

$sink #-> as $high;
alert $high for{
level: high
}`,
				RuleName: rulename2,
				Language: "golang",
			},
		})
		defer func() {
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{rulename2},
				},
			})
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormDefaultSSADataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormDefaultSSADataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
		}()

		// 不需要Save Risk也可以从数据库中读取Diff
		diff, err := client.SSARiskDiff(context.Background(), &ypb.SSARiskDiffRequest{
			BaseLine: &ypb.SSARiskDiffItem{RiskRuntimeId: taskID1},
			Compare:  &ypb.SSARiskDiffItem{RiskRuntimeId: taskID2},
		})
		require.NoError(t, err)
		flag := false
		for {
			recv, err := diff.Recv()
			if err != nil {
				break
			}
			if recv.Status == string(yakit.Del) {
				flag = true
				break
			}
		}
		require.True(t, flag)
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_SSAReusltCompareInQuerySSARisk(t *testing.T) {
	client, err := NewLocalClient(true) // use yakit handler local database, this test-case should use local grpc
	require.NoError(t, err)

	taskID1 := uuid.NewString() // 旧的扫描结果
	taskID2 := uuid.NewString() // 新的扫描结果
	baseProg := uuid.NewString()

	yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		Title:       "AA",
		FromRule:    "AA",
		RuntimeId:   taskID1,
		ProgramName: baseProg,
	})
	yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		Title:       "BB",
		FromRule:    "BB",
		RuntimeId:   taskID2,
		ProgramName: baseProg,
	})
	yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		Title:       "CC",
		FromRule:    "CC",
		RuntimeId:   taskID2,
		ProgramName: baseProg,
	})

	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			Title:     "AA",
			RuntimeID: []string{taskID1},
		})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			Title:     "BB",
			RuntimeID: []string{taskID2},
		})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			Title:     "CC",
			RuntimeID: []string{taskID2},
		})
	}()

	t.Run("taskid compare in QuerySSARisk", func(t *testing.T) {
		defer func() {
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormDefaultSSADataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormDefaultSSADataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
		}()
		response, err := client.QuerySSARisks(context.Background(), &ypb.QuerySSARisksRequest{
			Filter: &ypb.SSARisksFilter{
				RuntimeID: []string{taskID2},
				SSARiskDiffRequest: &ypb.SSARiskDiffRequest{
					Compare: &ypb.SSARiskDiffItem{RiskRuntimeId: taskID1},
				},
			},
			Pagination: &ypb.Paging{
				Page:    1,
				Order:   "desc",
				OrderBy: "id",
			},
		})
		require.NoError(t, err)

		// 新的扫描结果与旧的扫描结果相比，新增了两条RuntimeID为taskID2的risk
		data := response.GetData()
		require.Len(t, data, 2)
		require.Equal(t, data[0].GetRuntimeID(), taskID2)
		require.Equal(t, data[1].GetRuntimeID(), taskID2)
	})
}
