package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExpectedTaskResult 定义预期任务结果
type ExpectedTaskResult struct {
	TaskId           string   // 任务ID（可选，如果为空则按顺序匹配）
	Programs         []string // 预期程序列表
	Status           string   // 预期状态
	RiskCount        *int64   // 预期风险总数（nil表示不检查）
	NewRiskCount     *int64   // 预期新增风险数（nil表示不检查）
	HighCount        *int64   // 预期高风险数（nil表示不检查）
	NewHighCount     *int64   // 预期新增高风险数（nil表示不检查）
	LowCount         *int64   // 预期低风险数（nil表示不检查）
	NewLowCount      *int64   // 预期新增低风险数（nil表示不检查）
	CriticalCount    *int64   // 预期严重风险数（nil表示不检查）
	NewCriticalCount *int64   // 预期新增严重风险数（nil表示不检查）
	WarningCount     *int64   // 预期警告数（nil表示不检查）
	NewWarningCount  *int64   // 预期新增警告数（nil表示不检查）
	InfoCount        *int64   // 预期信息数（nil表示不检查）
	NewInfoCount     *int64   // 预期新增信息数（nil表示不检查）
}

// TestCase 定义测试用例
type TestCase struct {
	Name          string                        // 测试用例名称
	FileSystems   []FileSystemConfig            // 文件系统配置列表（支持多个程序）
	Rules         []RuleConfig                  // 规则配置列表
	QueryFilter   *ypb.SyntaxFlowScanTaskFilter // 查询过滤器
	ShowDiffRisk  bool                          // 是否显示差异风险
	ExpectedTasks []ExpectedTaskResult          // 预期任务结果列表
	CleanupFuncs  []func()                      // 清理函数列表
	// 额外的验证函数，在基本验证之后执行
	ExtraValidations []func(t *testing.T, resp *ypb.QuerySyntaxFlowScanTaskResponse, programIDs []string) // 额外验证函数
}

// FileSystemConfig 文件系统配置
type FileSystemConfig struct {
	ProgramName     string             // 程序名称
	Language        ssaconfig.Language // 语言
	ProgramPath     string             // 程序路径
	BaseProgramName string             // 基础程序名称（用于增量编译）
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

// checkQuerySyntaxFlowScanTask 统一的检查函数
// 输入：1. 文件系统配置 2. syntaxflow规则 3. 预期结果
// 内部调用接口检测实际结果和预期结果是否匹配
func checkQuerySyntaxFlowScanTask(t *testing.T, client ypb.YakClient, testCase TestCase) {
	ctx := context.Background()

	// 1. 创建程序
	var programIDs []string
	var cleanupFuncs []func()

	for _, fsConfig := range testCase.FileSystems {
		progID := fsConfig.ProgramName
		if progID == "" {
			progID = uuid.NewString()
		}
		programIDs = append(programIDs, progID)

		// 创建虚拟文件系统
		vf := filesys.NewVirtualFs()
		for path, content := range fsConfig.Files {
			vf.AddFile(path, content)
		}

		// 解析项目
		opts := []ssaconfig.Option{
			ssaapi.WithFileSystem(vf),
			ssaapi.WithLanguage(fsConfig.Language),
			ssaapi.WithProgramName(progID),
		}

		if fsConfig.ProgramPath != "" {
			opts = append(opts, ssaapi.WithProgramPath(fsConfig.ProgramPath))
		}

		if fsConfig.BaseProgramName != "" {
			opts = append(opts, ssaapi.WithBaseProgramName(fsConfig.BaseProgramName))
		}

		programs, err := ssaapi.ParseProject(opts...)
		require.NoError(t, err)
		require.NotNil(t, programs)
		require.Greater(t, len(programs), 0)

		// 添加清理函数
		cleanupFuncs = append(cleanupFuncs, func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID)
		})
	}

	// 2. 创建规则并执行扫描
	var taskIDs []string
	var ruleNames []string
	// 记录任务ID到规则名的映射，用于日志
	taskIDToRuleName := make(map[string]string)

	for ruleIdx, ruleConfig := range testCase.Rules {
		ruleName := ruleConfig.RuleName
		if ruleName == "" {
			ruleName = uuid.NewString()
		}
		ruleNames = append(ruleNames, ruleName)

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

	// 如果没有指定 Programs，使用所有程序ID
	if len(queryRequest.Filter.Programs) == 0 {
		queryRequest.Filter.Programs = programIDs
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

	// 创建任务ID到任务的映射
	taskMap := make(map[string]*ypb.SyntaxFlowScanTask)
	for _, task := range resp.Data {
		taskMap[task.TaskId] = task
	}

	// 验证每个预期任务
	for i, expected := range testCase.ExpectedTasks {
		log.Infof("=== Validating Expected Task[%d] ===", i)
		if expected.TaskId != "" {
			log.Infof("Expected TaskId: %s", expected.TaskId)
		} else {
			log.Infof("Expected TaskId: (empty, will match by index)")
		}
		log.Infof("Expected Programs: %v", expected.Programs)
		log.Infof("Expected Status: %s", expected.Status)
		if expected.RiskCount != nil {
			log.Infof("Expected RiskCount: %d", *expected.RiskCount)
		}
		if expected.NewRiskCount != nil {
			log.Infof("Expected NewRiskCount: %d", *expected.NewRiskCount)
		}
		var actual *ypb.SyntaxFlowScanTask

		if expected.TaskId != "" {
			// 如果指定了TaskId，使用TaskId查找
			actual = taskMap[expected.TaskId]
			require.NotNil(t, actual, "Task with ID %s not found", expected.TaskId)
			log.Infof("Found task by TaskId: %s", expected.TaskId)
		} else {
			// 否则按顺序匹配
			if i < len(resp.Data) {
				actual = resp.Data[i]
			}
			require.NotNil(t, actual, "Task at index %d not found", i)
			log.Infof("Found task by index: %d, TaskId: %s", i, actual.TaskId)
		}

		// 打印实际任务信息
		log.Infof("Actual Task: TaskId=%s, Programs=%v, Status=%s, RiskCount=%d, NewRiskCount=%d, HighCount=%d, NewHighCount=%d, LowCount=%d, NewLowCount=%d",
			actual.TaskId, actual.Programs, actual.Status, actual.RiskCount, actual.NewRiskCount,
			actual.HighCount, actual.NewHighCount, actual.LowCount, actual.NewLowCount)

		// 验证程序列表
		if len(expected.Programs) > 0 {
			log.Infof("Validating Programs: expected=%v, actual=%v", expected.Programs, actual.Programs)
			require.Equal(t, expected.Programs, actual.Programs, "Programs mismatch for task %s", actual.TaskId)
		}

		// 验证状态
		if expected.Status != "" {
			log.Infof("Validating Status: expected=%s, actual=%s", expected.Status, actual.Status)
			require.Equal(t, expected.Status, actual.Status, "Status mismatch for task %s", actual.TaskId)
		}

		// 验证风险计数
		if expected.RiskCount != nil {
			log.Infof("Validating RiskCount: expected=%d, actual=%d", *expected.RiskCount, actual.RiskCount)
			require.Equal(t, *expected.RiskCount, actual.RiskCount, "RiskCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewRiskCount != nil {
			log.Infof("Validating NewRiskCount: expected=%d, actual=%d", *expected.NewRiskCount, actual.NewRiskCount)
			if *expected.NewRiskCount != actual.NewRiskCount {
				// 打印更详细的错误信息
				log.Errorf("NewRiskCount mismatch for task %s: expected=%d, actual=%d", actual.TaskId, *expected.NewRiskCount, actual.NewRiskCount)
				log.Errorf("Task details: Programs=%v, RiskCount=%d, HighCount=%d, LowCount=%d, UpdatedAt=%d",
					actual.Programs, actual.RiskCount, actual.HighCount, actual.LowCount, actual.UpdatedAt)
				// 打印所有任务以便对比
				log.Errorf("All tasks in response:")
				for idx, task := range resp.Data {
					log.Errorf("  Task[%d]: TaskId=%s, Programs=%v, RiskCount=%d, NewRiskCount=%d, UpdatedAt=%d",
						idx, task.TaskId, task.Programs, task.RiskCount, task.NewRiskCount, task.UpdatedAt)
				}
			}
			require.Equal(t, *expected.NewRiskCount, actual.NewRiskCount, "NewRiskCount mismatch for task %s: expected=%d, actual=%d", actual.TaskId, *expected.NewRiskCount, actual.NewRiskCount)
		}

		if expected.HighCount != nil {
			require.Equal(t, *expected.HighCount, actual.HighCount, "HighCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewHighCount != nil {
			require.Equal(t, *expected.NewHighCount, actual.NewHighCount, "NewHighCount mismatch for task %s", actual.TaskId)
		}

		if expected.LowCount != nil {
			require.Equal(t, *expected.LowCount, actual.LowCount, "LowCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewLowCount != nil {
			require.Equal(t, *expected.NewLowCount, actual.NewLowCount, "NewLowCount mismatch for task %s", actual.TaskId)
		}

		if expected.CriticalCount != nil {
			require.Equal(t, *expected.CriticalCount, actual.CriticalCount, "CriticalCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewCriticalCount != nil {
			require.Equal(t, *expected.NewCriticalCount, actual.NewCriticalCount, "NewCriticalCount mismatch for task %s", actual.TaskId)
		}

		if expected.WarningCount != nil {
			require.Equal(t, *expected.WarningCount, actual.WarningCount, "WarningCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewWarningCount != nil {
			require.Equal(t, *expected.NewWarningCount, actual.NewWarningCount, "NewWarningCount mismatch for task %s", actual.TaskId)
		}

		if expected.InfoCount != nil {
			require.Equal(t, *expected.InfoCount, actual.InfoCount, "InfoCount mismatch for task %s", actual.TaskId)
		}

		if expected.NewInfoCount != nil {
			log.Infof("Validating NewInfoCount: expected=%d, actual=%d", *expected.NewInfoCount, actual.NewInfoCount)
			require.Equal(t, *expected.NewInfoCount, actual.NewInfoCount, "NewInfoCount mismatch for task %s", actual.TaskId)
		}

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
	client, err := NewLocalClient(true)
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
		ExpectedTasks: []ExpectedTaskResult{
			{
				Programs: []string{progID},
				Status:   "done",
			},
		},
	}

	checkQuerySyntaxFlowScanTask(t, client, testCase)
}

// TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithDiff 测试差异风险计算
func TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithDiff(t *testing.T) {
	client, err := NewLocalClient(true)
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
		ExpectedTasks: []ExpectedTaskResult{
			{
				// 注意：查询结果按 UpdatedAt 降序排列（最新的在前）
				// 这是第二个扫描任务（ruleName2），是最新的任务
				// NewRiskCount=3 表示相对于之前任务（ruleName1）有3个新增风险
				Programs:     []string{progID},
				Status:       "done",
				NewRiskCount: intPtr(3), // 第二个扫描任务（ruleName2），相对于第一个任务有3个新增风险
			},
			{
				// 这是第一个扫描任务（ruleName1），是较早的任务
				// NewRiskCount=0 表示这是第一个扫描，没有之前的任务可以比较
				Programs:     []string{progID},
				Status:       "done",
				NewRiskCount: intPtr(0), // 第一个扫描任务（ruleName1），没有新增风险（第一个扫描）
			},
		},
	}

	checkQuerySyntaxFlowScanTask(t, client, testCase)
}

// TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithIncrementalCompile 测试增量编译场景
func TestGRPCMUSTPASS_QuerySyntaxFlowScanTask_WithIncrementalCompile(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	baseProgID := uuid.NewString()
	diffProgID := uuid.NewString()
	ruleName := uuid.NewString()

	testCase := TestCase{
		Name: "test incremental compile scenario",
		FileSystems: []FileSystemConfig{
			{
				ProgramName: baseProgID,
				Language:    ssaconfig.JAVA,
				ProgramPath: "example",
				Files: map[string]string{
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
				},
			},
			{
				ProgramName:     diffProgID,
				Language:        ssaconfig.JAVA,
				ProgramPath:     "example",
				BaseProgramName: baseProgID,
				Files: map[string]string{
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
				},
			},
		},
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
		ExpectedTasks: []ExpectedTaskResult{
			{
				Programs:     []string{diffProgID},
				Status:       "done",
				NewRiskCount: intPtr(7), // 这个值会在实际测试中根据结果调整
			},
			{
				Programs:     []string{baseProgID},
				Status:       "done",
				NewRiskCount: intPtr(0), // 第一个扫描，没有新增风险
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

				// 验证基础程序的扫描结果
				require.Equal(t, baseTask.Programs, []string{baseProgID})
				require.Equal(t, baseTask.Status, "done")
				require.Greater(t, baseTask.RiskCount, int64(0), "Base program should have at least one risk")
				// 基础程序是第一个扫描的，所以新增漏洞数应该为 0（因为没有之前的扫描进行比较）
				require.Equal(t, baseTask.NewRiskCount, int64(0), "Base program should have 0 new risks (first scan)")

				// 验证增量程序的扫描结果
				require.Equal(t, diffTask.Programs, []string{diffProgID})
				require.Equal(t, diffTask.Status, "done")
				require.Greater(t, diffTask.RiskCount, baseTask.RiskCount, "Diff program should have more risks than base program")
				// 增量程序应该检测到新增的漏洞（通过 RuntimeId 比较，不依赖 ProgramName）
				require.Greater(t, diffTask.NewRiskCount, int64(0), "Diff program should have new risks detected via RuntimeId comparison")
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

// intPtr 辅助函数，返回int64指针
func intPtr(i int64) *int64 {
	return &i
}
