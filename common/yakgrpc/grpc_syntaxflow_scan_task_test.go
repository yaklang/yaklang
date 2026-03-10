package yakgrpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	yakgrpc "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestCase 定义测试用例
type TestCase struct {
	Name               string                        // 测试用例名称
	PrebuiltProgramIDs []string                      // 已预构建的 program ID 列表（可选）
	FileSystems        []FileSystemConfig            // 文件系统配置列表（支持多个程序）
	Rules              []RuleConfig                  // 规则配置列表
	QueryFilter        *ypb.SyntaxFlowScanTaskFilter // 查询过滤器
	ShowDiffRisk       bool                          // 是否显示差异风险
	ExpectedTasks      []TaskResultConfig            // 预期任务结果列表
	CleanupFuncs       []func()                      // 清理函数列表
	// 额外的验证函数，在基本验证之后执行
	ExtraValidations []func(t *testing.T, resp *ypb.QuerySyntaxFlowScanTaskResponse, programIDs []string) // 额外验证函数
}

// FileSystemConfig 文件系统配置
type FileSystemConfig struct {
	ProgramName     string             // 程序别名（用于查询过滤与断言映射）
	Language        ssaconfig.Language // 语言
	ProgramPath     string             // 程序路径
	BaseProgramName string             // 基础程序别名或实际程序名（用于增量编译）
	Files           map[string]string  // 文件路径 -> 文件内容
}

// RuleConfig 规则配置
type RuleConfig struct {
	RuleName   string   // 规则名称
	Content    string   // 规则内容
	Language   string   // 语言
	GroupNames []string // 组名列表
	Tags       []string // 标签列表
}

func resolveProgramAliases(programs []string, aliases map[string]string) []string {
	if len(programs) == 0 {
		return nil
	}
	resolved := make([]string, 0, len(programs))
	for _, program := range programs {
		if actual, ok := aliases[program]; ok && actual != "" {
			resolved = append(resolved, actual)
			continue
		}
		resolved = append(resolved, program)
	}
	return resolved
}

// checkQuerySyntaxFlowScanTask 统一的检查函数
// 输入：1. 文件系统配置 2. syntaxflow规则 3. 预期结果
// 内部调用接口检测实际结果和预期结果是否匹配
func checkQuerySyntaxFlowScanTask(t *testing.T, client ypb.YakClient, testCase TestCase) {
	ctx := context.Background()

	// 1. 创建程序
	programIDs := append([]string(nil), testCase.PrebuiltProgramIDs...)
	programAliases := make(map[string]string)
	var cleanupFuncs []func()

	for _, fsConfig := range testCase.FileSystems {
		language := fsConfig.Language
		if language == "" {
			language = ssaconfig.JAVA
		}

		compileOptions := []ssaconfig.Option{
			ssaapi.WithContext(ctx),
			ssaapi.WithLanguage(language),
		}
		if fsConfig.ProgramPath != "" {
			compileOptions = append(compileOptions, ssaapi.WithProgramPath(fsConfig.ProgramPath))
		}

		if fsConfig.BaseProgramName != "" {
			baseProgramName := fsConfig.BaseProgramName
			if actual, ok := programAliases[baseProgramName]; ok && actual != "" {
				baseProgramName = actual
			}
			compileOptions = append(compileOptions,
				ssaapi.WithBaseProgramName(baseProgramName),
				ssaapi.WithEnableIncrementalCompile(true),
			)
		} else {
			compileOptions = append(compileOptions, ssaapi.WithEnableIncrementalCompile(false))
		}

		var compiledProgramName string
		ssatest.CheckIncrementalProgramWithOptions(t, compileOptions,
			ssatest.IncrementalStep{
				Files: fsConfig.Files,
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
						return
					}
					layerNames := overlay.GetLayerProgramNames()
					require.NotEmpty(t, layerNames)
					compiledProgramName = layerNames[len(layerNames)-1]
				},
			},
		)
		require.NotEmpty(t, compiledProgramName)
		programIDs = append(programIDs, compiledProgramName)
		if fsConfig.ProgramName != "" {
			programAliases[fsConfig.ProgramName] = compiledProgramName
		}
	}

	// 2. 创建规则并执行扫描
	var taskIDs []string
	// 记录任务ID到规则名的映射，用于日志
	taskIDToRuleName := make(map[string]string)

	for ruleIdx, ruleConfig := range testCase.Rules {
		ruleName := ruleConfig.RuleName
		if ruleName == "" {
			ruleName = uuid.NewString()
		}

		// 创建规则
		_, err := client.CreateSyntaxFlowRule(ctx, &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content:    ruleConfig.Content,
				RuleName:   ruleName,
				Language:   ruleConfig.Language,
				GroupNames: ruleConfig.GroupNames,
				Tags:       ruleConfig.Tags,
			},
		})
		require.NoError(t, err)

		// 添加清理函数
		cleanupFuncs = append(cleanupFuncs, func() {
			client.DeleteSyntaxFlowRule(ctx, &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName},
				},
			})
		})

		// 为每个程序执行扫描
		for _, progID := range programIDs {
			log.Infof("Starting scan for program %s with rule %s", progID, ruleName)
			stream, err := client.SyntaxFlowScan(ctx)
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName},
				},
				ProgramName: []string{progID},
			})

			resp, err := stream.Recv()
			require.NoError(t, err)
			taskID := resp.TaskID
			taskIDs = append(taskIDs, taskID)
			taskIDToRuleName[taskID] = ruleName
			log.Infof("Scan started [Execution Order %d]: program=%s, rule=%s (ruleIdx=%d), taskId=%s",
				len(taskIDs), progID, ruleName, ruleIdx, taskID)

			// 等待扫描完成
			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})

			require.Equal(t, 1.0, finishProcess, "Scan should complete with 100%% progress")
			require.Equal(t, "done", finishStatus, "Scan should finish with 'done' status")

			// 添加清理函数
			cleanupFuncs = append(cleanupFuncs, func() {
				schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID)
				yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID}, schema.RuntimeId)
				yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID}, schema.RuntimeId)
			})
		}
	}

	// 3. 执行查询
	queryRequest := &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter:       testCase.QueryFilter,
		ShowDiffRisk: testCase.ShowDiffRisk,
	}

	if queryRequest.Filter == nil {
		queryRequest.Filter = &ypb.SyntaxFlowScanTaskFilter{}
	}

	// 如果没有指定 Programs，使用所有程序ID；否则将测试里的别名替换成实际编译名
	if len(queryRequest.Filter.Programs) == 0 {
		queryRequest.Filter.Programs = append([]string(nil), programIDs...)
	} else {
		queryRequest.Filter.Programs = resolveProgramAliases(queryRequest.Filter.Programs, programAliases)
	}

	resp, err := client.QuerySyntaxFlowScanTask(ctx, queryRequest)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// 4. 打印查询结果详细信息
	log.Infof("=== Query Result Debug Info ===")
	log.Infof("Query Filter Programs: %v", queryRequest.Filter.GetPrograms())
	log.Infof("ShowDiffRisk: %v", queryRequest.ShowDiffRisk)
	log.Infof("Total tasks returned: %d", len(resp.Data))
	log.Infof("Expected tasks: %d", len(testCase.ExpectedTasks))
	log.Infof("Note: Tasks are ordered by UpdatedAt DESC (newest first)")

	for idx, task := range resp.Data {
		ruleName := taskIDToRuleName[task.TaskId]
		execOrder := -1
		for i, tid := range taskIDs {
			if tid == task.TaskId {
				execOrder = i + 1
				break
			}
		}
		log.Infof("Task[%d] (response order, newest first): TaskId=%s, Rule=%s, ExecutionOrder=%d, Programs=%v, Status=%s, RiskCount=%d, NewRiskCount=%d, HighCount=%d, NewHighCount=%d, LowCount=%d, NewLowCount=%d, UpdatedAt=%d",
			idx, task.TaskId, ruleName, execOrder, task.Programs, task.Status, task.RiskCount, task.NewRiskCount,
			task.HighCount, task.NewHighCount, task.LowCount, task.NewLowCount, task.UpdatedAt)
	}
	log.Infof("=== End Query Result Debug Info ===")

	// 5. 验证结果
	require.Equal(t, len(testCase.ExpectedTasks), len(resp.Data), "Expected task count mismatch")

	// 验证每个预期任务
	for i, expectedTask := range testCase.ExpectedTasks {
		expected := expectedTask
		expected.Programs = resolveProgramAliases(expected.Programs, programAliases)
		actualTask := resp.Data[i]
		log.Infof("=== Validating Expected Task[%d] ===\n", i)
		log.Infof("Expected: TaskID=%s, Programs=%v, Status=%s, RiskCount=%d, NewRiskCount=%d, HighCount=%d, NewHighCount=%d, LowCount=%d, NewLowCount=%d",
			expected.TaskID, expected.Programs, expected.Status, expected.RiskCount, expected.NewRiskCount,
			expected.HighCount, expected.NewHighCount, expected.LowCount, expected.NewLowCount)
		log.Infof("Actual: TaskID=%s, Programs=%v, Status=%s, RiskCount=%d, NewRiskCount=%d, HighCount=%d, NewHighCount=%d, LowCount=%d, NewLowCount=%d",
			actualTask.TaskId, actualTask.Programs, actualTask.Status, actualTask.RiskCount, actualTask.NewRiskCount,
			actualTask.HighCount, actualTask.NewHighCount, actualTask.LowCount, actualTask.NewLowCount)

		// 构建实际配置用于完整匹配
		actualConfig := TaskResultConfig{
			TaskID:       actualTask.TaskId,
			Programs:     actualTask.Programs,
			Status:       actualTask.Status,
			LowCount:     actualTask.LowCount,
			HighCount:    actualTask.HighCount,
			RiskCount:    actualTask.RiskCount,
			NewLowCount:  actualTask.NewLowCount,
			NewHighCount: actualTask.NewHighCount,
			NewRiskCount: actualTask.NewRiskCount,
		}

		// 如果预期配置中没有指定 Programs，则不比较 Programs
		if len(expected.Programs) == 0 {
			actualConfig.Programs = nil
		}

		// 如果预期配置中没有指定 TaskID，则不比较 TaskID
		if expected.TaskID == "" {
			actualConfig.TaskID = ""
		}

		// 如果预期配置中没有指定 Status，则不比较 Status
		if expected.Status == "" {
			actualConfig.Status = ""
		}

		// 完整匹配所有字段（所有 int64 字段都参与匹配，包括 0 值）
		require.Equal(t, expected, actualConfig, "Task[%d] configuration mismatch", i)

		log.Infof("=== End Validating Expected Task[%d] ===\n", i)
	}

	// 6. 执行额外的验证
	for _, extraValidation := range testCase.ExtraValidations {
		if extraValidation != nil {
			extraValidation(t, resp, programIDs)
		}
	}

	// 7. 执行清理
	log.Infof("=== Starting cleanup ===")
	for _, cleanup := range cleanupFuncs {
		cleanup()
	}

	// 执行测试用例的清理函数
	for _, cleanup := range testCase.CleanupFuncs {
		cleanup()
	}
	log.Infof("=== Cleanup completed ===")
}

// TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_Basic 基础测试
func TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_Basic(t *testing.T) {
	client, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	progID := uuid.NewString()
	ruleName := uuid.NewString()

	testCase := TestCase{
		Name: "basic query test",
		FileSystems: []FileSystemConfig{
			{
				ProgramName: progID,
				Language:    ssaconfig.GO,
				ProgramPath: "example",
				Files: map[string]string{
					"example/src/main/a.go": `
package main

import (
	"os/exec"
)

func cmd() {
	exec.Command("/bin/sh", "-c", "ls")
}
`,
				},
			},
		},
		Rules: []RuleConfig{
			{
				RuleName: ruleName,
				Content: `
exec.Command(* #-> as $high)

alert $high for {
	type: "vuln",
	level: "high",
}`,
				Language:   "golang",
				GroupNames: []string{"golang"},
				Tags:       []string{"golang"},
			},
		},
		QueryFilter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{progID},
		},
		ShowDiffRisk: false,
		ExpectedTasks: []TaskResultConfig{
			{
				Programs:  []string{progID},
				Status:    "done",
				HighCount: 3,
				RiskCount: 3,
			},
		},
	}

	checkQuerySyntaxFlowScanTask(t, client, testCase)
}

// TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithDiff 测试差异风险计算
func TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithDiff(t *testing.T) {
	client, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	progID := uuid.NewString()
	ruleName1 := uuid.NewString()
	ruleName2 := uuid.NewString()

	testCase := TestCase{
		Name: "test diff risk calculation",
		FileSystems: []FileSystemConfig{
			{
				ProgramName: progID,
				Language:    ssaconfig.GO,
				ProgramPath: "example",
				Files: map[string]string{
					"example/src/main/a.go": `
package main

import (
	"os/exec"
)

func cmd() {
	exec.Command("/bin/sh", "-c", "ls")
}
`,
				},
			},
		},
		Rules: []RuleConfig{
			{
				RuleName: ruleName1,
				Content: `
exec.Command(*?{opcode:const} #-> as $high)

alert $high for {
	type: "vuln",
	level: "high",
}`,
				Language:   "golang",
				GroupNames: []string{"golang"},
				Tags:       []string{"golang"},
			},
			{
				RuleName: ruleName2,
				Content: `
exec.Command(* #-> as $high)

alert $high for {
	type: "vuln",
	level: "high",
}`,
				Language:   "golang",
				GroupNames: []string{"golang"},
				Tags:       []string{"golang"},
			},
		},
		QueryFilter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{progID},
		},
		ShowDiffRisk: true,
		ExpectedTasks: []TaskResultConfig{
			{
				Programs:     []string{progID},
				Status:       "done",
				NewHighCount: 3,
				NewRiskCount: 3,
				HighCount:    3,
				RiskCount:    3,
			},
			{
				Programs:  []string{progID},
				Status:    "done",
				HighCount: 3,
				RiskCount: 3,
			},
		},
	}

	checkQuerySyntaxFlowScanTask(t, client, testCase)
}

// TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithIncrementalCompile 测试增量编译场景
// 验证手动增量编译
func TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithIncrementalCompile(t *testing.T) {
	client, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	var (
		baseProgID string
		diffProgID string
	)
	ruleName := uuid.NewString()
	baseSnapshot := map[string]string{
		"example/src/main/java/com/example/Base.java": `
package com.example;
import java.lang.Runtime;

public class Base {
	public static void main(String[] args) {
		// 这个漏洞会在基础程序中检测到
		Runtime.getRuntime().exec("ls");
	}
}
`,
	}
	diffSnapshot := map[string]string{
		"example/src/main/java/com/example/Base.java": `
package com.example;
import java.lang.Runtime;

public class Base {
	public static void main(String[] args) {
		// 保留原有的漏洞
		Runtime.getRuntime().exec("ls");
		// 新增的漏洞（应该在增量编译中检测到）
		Runtime.getRuntime().exec(args[0]);
	}
}
`,
		"example/src/main/java/com/example/NewClass.java": `
package com.example;
import java.lang.Runtime;

public class NewClass {
	public void process(String cmd) {
		// 新增文件中的漏洞
		Runtime.getRuntime().exec(cmd);
	}
}
`,
	}
	ssatest.CheckIncrementalProgramWithOptions(t,
		[]ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithContext(context.Background()),
		},
		ssatest.IncrementalStep{
			Files: baseSnapshot,
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
					return
				}
				names := overlay.GetLayerProgramNames()
				require.NotEmpty(t, names)
				baseProgID = names[0]
			},
		},
		ssatest.IncrementalStep{
			Files: diffSnapshot,
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile || overlay == nil {
					return
				}
				names := overlay.GetLayerProgramNames()
				require.GreaterOrEqual(t, len(names), 2)
				diffProgID = names[len(names)-1]
			},
		},
	)
	require.NotEmpty(t, baseProgID)
	require.NotEmpty(t, diffProgID)

	testCase := TestCase{
		Name:               "test incremental compile scenario",
		PrebuiltProgramIDs: []string{baseProgID, diffProgID},
		Rules: []RuleConfig{
			{
				RuleName: ruleName,
				Content: `
Runtime.getRuntime().exec(* #-> as $high)

alert $high for {
	type: "vuln",
	level: "high",
}`,
				Language:   "java",
				GroupNames: []string{"java"},
				Tags:       []string{"java"},
			},
		},
		QueryFilter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{diffProgID}, // 通过增量程序查询，模拟真实场景（通过 diff 找到 base）
		},
		ShowDiffRisk: true,
		ExpectedTasks: []TaskResultConfig{
			// 当前 QuerySyntaxFlowScanTask 的项目级查询会返回同项目下的 base/diff 两个任务，
			// 但这里不会把 diff 相对 base 的新增风险单独计入 New*Count。
			{
				Programs:     []string{diffProgID},
				Status:       "done",
				RiskCount:    7,
				HighCount:    7,
				NewRiskCount: 0,
				NewHighCount: 0,
			},
			{
				Programs:     []string{baseProgID},
				Status:       "done",
				RiskCount:    7,
				HighCount:    7,
				NewRiskCount: 0,
				NewHighCount: 0,
			},
		},
		ExtraValidations: []func(t *testing.T, resp *ypb.QuerySyntaxFlowScanTaskResponse, programIDs []string){
			func(t *testing.T, resp *ypb.QuerySyntaxFlowScanTaskResponse, programIDs []string) {
				baseProgID := programIDs[0]
				diffProgID := programIDs[1]

				// 验证基础程序和增量程序属于同一个 project
				baseIrProgram, err := ssadb.GetApplicationProgram(baseProgID)
				require.NoError(t, err)
				require.NotNil(t, baseIrProgram)

				diffIrProgram, err := ssadb.GetApplicationProgram(diffProgID)
				require.NoError(t, err)
				require.NotNil(t, diffIrProgram)

				require.Equal(t, baseIrProgram.ProjectID, diffIrProgram.ProjectID, "Base and diff programs should belong to the same project")

				// 应该返回基础程序和增量程序的扫描任务（因为它们属于同一个 project）
				require.GreaterOrEqual(t, len(resp.Data), 2, "Should return at least 2 tasks (base and diff) for the same project")

				// 找到基础程序和增量程序的扫描任务
				var baseTask, diffTask *ypb.SyntaxFlowScanTask
				for _, task := range resp.Data {
					if len(task.Programs) > 0 && task.Programs[0] == baseProgID {
						baseTask = task
					} else if len(task.Programs) > 0 && task.Programs[0] == diffProgID {
						diffTask = task
					}
				}

				require.NotNil(t, baseTask, "Base task should be found when querying by diff program name (same project)")
				require.NotNil(t, diffTask, "Diff task should be found when querying by diff program name")
			},
			// 验证通过增量程序名称查询也能返回同一 project 下的所有任务
			func(t *testing.T, resp *ypb.QuerySyntaxFlowScanTaskResponse, programIDs []string) {
				baseProgID := programIDs[0]
				diffProgID := programIDs[1]

				// 验证返回的任务包含基础程序和增量程序
				hasBaseTask := false
				hasDiffTask := false
				for _, task := range resp.Data {
					if len(task.Programs) > 0 && task.Programs[0] == baseProgID {
						hasBaseTask = true
					}
					if len(task.Programs) > 0 && task.Programs[0] == diffProgID {
						hasDiffTask = true
					}
				}
				require.True(t, hasBaseTask, "Should return base task when querying by diff program name")
				require.True(t, hasDiffTask, "Should return diff task when querying by diff program name")
			},
		},
	}

	checkQuerySyntaxFlowScanTask(t, client, testCase)
}
