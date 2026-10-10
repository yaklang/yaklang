package loop_dir_explore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func withExploreOutput() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		reactloops.WithLoopEmitterProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
			if e == nil {
				return nil
			}
			if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME && e.GetContentJSONPath("$.path") != loop.Get("result_report_path") {
				return nil
			}
			// Once the target is confirmed, model/tool bookkeeping must not
			// overwrite the user-facing exploration or report-generation phase.
			if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "status" && loop.Get("explore_phase") != "" {
				var status aicommon.StatusPayload
				_ = json.Unmarshal(e.Content, &status)
				if !strings.HasPrefix(status.Code, "dir_explore.") {
					return nil
				}
			}
			return e
		})(loop)
	}
}

func emitExplorePhase(loop *reactloops.ReActLoop, phase, zh, en string, state aicommon.StatusState) {
	loop.Set("explore_phase", phase)
	reactloops.EmitStatusI18n(loop, zh, en,
		aicommon.WithStatusCode("dir_explore."+phase), aicommon.WithStatusState(state))
	if phase != "completed" { // The report card and its reference already deliver completion.
		reactloops.EmitActionLog(loop, "dir-explore-progress", zh)
	}
}

func emitExploreStart(loop *reactloops.ReActLoop, target string) {
	emitExplorePhase(loop, "exploring",
		fmt.Sprintf("开始探索 %s：梳理目录结构、技术栈、入口点和核心模块。", target),
		fmt.Sprintf("Exploring %s: directory structure, tech stack, entry points and core modules.", target),
		aicommon.StatusStateRunning)
}

func emitExploreNoteProgress(loop *reactloops.ReActLoop, path string) {
	labels := map[string]string{
		"dir_structure.md": "目录结构已整理", "entry_points.md": "项目入口已整理",
		"tech_stack.md": "技术栈与依赖已整理", "modules_overview.md": "核心模块职责已整理",
	}
	if label := labels[filepath.Base(path)]; label != "" {
		emitExplorePhase(loop, "exploring", label+"，正在继续探索项目。",
			"Continuing project exploration.", aicommon.StatusStateRunning)
	}
}

func buildExploreSummary(state *ExploreState) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s 项目概览\n", state.ProjectName)
	if overview := strings.TrimSpace(state.ProjectOverview); overview != "" {
		fmt.Fprintf(&sb, "\n%s\n", overview)
	}
	fmt.Fprintf(&sb, "\n- **技术栈**：%s\n- **主要入口**：%s\n", state.TechStack, state.EntryPoints)
	if modules := strings.TrimSpace(state.ModulesSummary); modules != "" {
		fmt.Fprintf(&sb, "- **核心模块**：%s\n", modules)
	}
	if guide := strings.TrimSpace(state.ReadingGuide); guide != "" {
		fmt.Fprintf(&sb, "\n**建议从这里开始阅读**：%s\n", guide)
	}
	if state.ReportFilePath != "" {
		sb.WriteString("\n完整目录结构、入口点明细和关键配置见报告文件。")
	}
	return sb.String()
}

func failExploreReport(loop *reactloops.ReActLoop, state *ExploreState, err error, op *reactloops.LoopActionHandlerOperator) {
	message := "项目探索信息已整理，但完整报告未生成：" + err.Error()
	task := loop.GetCurrentTask()
	cancelled := task.IsUserCancelled() || task.GetContext().Err() != nil
	if cancelled {
		message = "探索已停止，完整报告尚未生成。"
	}
	summary := buildExploreSummary(state) + "\n\n" + message
	loop.Set("result_summary", summary)
	loop.GetCurrentTask().SetResult(summary)
	if cancelled {
		// The caller owns cancellation UI. Its closed context can no longer
		// reliably accept streams; do not turn cancellation into a failure card.
		loop.Set("explore_phase", "cancelled")
		op.Fail(err)
		return
	}
	emitExplorePhase(loop, "failed", message, "The full exploration report could not be completed.", aicommon.StatusStateError)
	_, _ = loop.GetEmitter().EmitResult("result", summary, false)
	op.Fail(err)
}
