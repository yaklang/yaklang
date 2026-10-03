package loop_plan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const contextTokenBudget = 15000

func getLoopTaskContext(loop *reactloops.ReActLoop) string {
	type contextEntry struct {
		Title string
		Key   string
		Value string
	}

	entries := []contextEntry{
		{Title: "侦查结果", Key: "recon", Value: loop.Get(PLAN_RECON_RESULTS_KEY)},
		{Title: "文件结果", Key: "file", Value: loop.Get(PLAN_FILE_RESULTS_KEY)},
		{Title: "已有计划", Key: "plan", Value: loop.Get(PLAN_DATA_KEY)},
		{Title: "补充知识", Key: "enhance", Value: loop.Get(PLAN_ENHANCE_KEY)},
		{Title: "互联网结果", Key: "web", Value: loop.Get(PLAN_WEB_RESULTS_KEY)},
	}

	var parts []string
	usedTokens := 0

	for _, entry := range entries {
		value := strings.TrimSpace(entry.Value)
		if value == "" {
			continue
		}

		sectionHeader := fmt.Sprintf("## %s\n", entry.Title)
		headerTokens := aicommon.MeasureTokens(sectionHeader)
		remaining := contextTokenBudget - usedTokens - headerTokens
		if remaining <= 100 {
			break
		}

		valueTokens := aicommon.MeasureTokens(value)
		if valueTokens <= remaining {
			part := sectionHeader + value
			parts = append(parts, part)
			usedTokens += headerTokens + valueTokens
		} else {
			artifactPath := saveContextToArtifact(loop, entry.Key, entry.Title, value)
			summary := aicommon.ShrinkTextBlockByTokens(value, remaining-100)
			var part string
			if artifactPath != "" {
				part = fmt.Sprintf("%s%s\n\n> 以上为摘要，完整内容(%d tokens)已保存至: %s，可通过 read_file 查看", sectionHeader, summary, valueTokens, artifactPath)
			} else {
				part = sectionHeader + summary
			}
			partTokens := aicommon.MeasureTokens(part)
			parts = append(parts, part)
			usedTokens += partTokens
		}
	}
	return strings.Join(parts, "\n\n")
}

func saveContextToArtifact(loop *reactloops.ReActLoop, key string, title string, content string) string {
	invoker := loop.GetInvoker()
	if invoker == nil {
		return ""
	}
	filename := fmt.Sprintf("plan_context_%s", key)
	fullContent := fmt.Sprintf("# %s\n\n%s", title, content)
	path := invoker.EmitFileArtifactWithExt(filename, ".md", fullContent)
	if path != "" {
		log.Infof("plan loop: context section %q saved to artifact: %s", title, path)
	}
	return path
}

func hasValidPlan(loop *reactloops.ReActLoop) bool {
	planData := strings.TrimSpace(loop.Get(PLAN_DATA_KEY))
	if planData == "" {
		return false
	}
	action, err := aicommon.ExtractAction(planData, "plan", "plan")
	if err != nil {
		return false
	}
	return action.GetString("main_task") != "" && action.GetString("main_task_goal") != "" && len(action.GetInvokeParamsArray("tasks")) > 0
}

func isMaxIterationReason(reason any) bool {
	if reason == nil {
		return false
	}
	if err, ok := reason.(error); ok {
		return strings.Contains(err.Error(), "max iterations")
	}
	return strings.Contains(utils.InterfaceToString(reason), "max iterations")
}

func buildPlanPostIterationHook() reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		lastAction := loop.GetLastAction()
		if lastAction != nil {
			trackPlanModeFromAction(loop, lastAction)
		}
		if !isDone {
			return
		}

		ensureDirectPlanOnFinalize(loop, task)

		if !hasValidPlan(loop) && !isSimplePlanMode(loop) {
			document := generateGuidanceDocument(loop, task)
			if document != "" {
				loop.Set(PLAN_DOCUMENT_KEY, document)
				log.Infof("plan loop: generated guidance document at finalization")
			}

			planData := generatePlanFromDocument(loop, task)
			if planData != "" {
				loop.Set(PLAN_DATA_KEY, planData)
				log.Infof("plan loop: generated plan from document at finalization")
			}
		}

		if isMaxIterationReason(reason) && hasValidPlan(loop) {
			operator.IgnoreError()
		}
	})
}
