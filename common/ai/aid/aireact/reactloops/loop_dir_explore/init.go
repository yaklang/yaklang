package loop_dir_explore

import (
	"math"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_report_generating"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_DIR_EXPLORE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return BuildDirExploreLoop(r, opts...)
		},
		reactloops.WithLoopDescription("Directory exploration mode: AI autonomously explores a target directory/project, identifies all entry points, tech stack, and module structure, then generates a structured exploration report."),
		reactloops.WithLoopDescriptionZh("目录探索模式：AI 自主探索目标目录/项目，识别所有入口点（含多 main 函数）、技术栈和模块结构，生成结构化探索报告。"),
		reactloops.WithVerboseName("Directory Explorer"),
		reactloops.WithVerboseNameZh("目录探索"),
		reactloops.WithLoopUsagePrompt(`当用户需要 AI 探索某个代码目录或项目，了解其结构、技术栈、所有入口点和核心模块功能时使用此流程。适用于快速了解陌生项目或在代码审计前的预探索阶段。`),
		reactloops.WithLoopOutputExample(`
* 当需要探索目录或了解项目结构时：
  {"@action": "dir_explore", "human_readable_thought": "需要探索目标目录了解项目结构和入口点"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_DIR_EXPLORE, err)
	}
}

// generateExploreReport 调用 report_generating 子 loop 将探索文件汇总成报告。
func generateExploreReport(
	r aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	writePrompt string,
	reportPath string,
	noteFiles []string,
) error {
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			switch action.ActionType {
			case "read_reference_file", "grep_reference", "change_view_offset",
				"write_section", "modify_section", "insert_section", "delete_section", "finish":
				return true
			}
			return false
		}),
		// This child assembles existing notes; it does not need exploratory
		// perception or a second verification watchdog on the shared runtime.
		reactloops.WithDisableLoopPerception(true),
		reactloops.WithDisablePeriodicVerification(true),
		loop_report_generating.WithInternalReportOutput(),
		reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, task aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			innerLoop.Set("report_filename", reportPath)
			innerLoop.Set("full_report_code", "")
			innerLoop.Set("user_requirements", writePrompt)
			innerLoop.Set("available_files", buildAvailableFilesHint(noteFiles))
			innerLoop.Set("available_knowledge_bases", "")
			innerLoop.Set("collected_references", "")
			innerLoop.Set("is_modify_mode", "false")
			innerOp.Continue()
		}),
	)
	if err != nil {
		return err
	}

	subTask := aicommon.NewSubTaskBase(parentLoop.GetCurrentTask(), "dir-explore-report", writePrompt, true)
	if err := reportLoop.ExecuteWithExistedTask(subTask); err != nil {
		return err
	}
	// Execute can also return nil after a soft iteration limit. Only an accepted
	// finish and a saved, nonempty report count as successful delivery.
	if reportLoop.Get("report_finished") != "true" || subTask.IsUserCancelled() {
		return utils.Error("报告生成未完成")
	}
	if err := parentLoop.GetCurrentTask().GetContext().Err(); err != nil {
		return err
	}
	content, err := os.ReadFile(reportPath)
	if err != nil {
		return utils.Errorf("无法读取生成的报告: %v", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return utils.Error("生成的报告为空")
	}
	if strings.TrimSpace(string(content)) != strings.TrimSpace(reportLoop.Get("full_report_code")) {
		return utils.Error("报告内容尚未完整保存")
	}
	return nil
}
