package loop_plan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	PlanFactsFieldName = "facts"
	PlanFactsAITagName = "FACTS"
	PlanFactsAINodeID  = "plan-facts"

	planFactsGeneralSection = "## 通用事实"
)

type factsSection struct {
	Title string
	Lines []string
	seen  map[string]struct{}
}

func normalizeFactsDocument(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func parseFactsSections(content string) []*factsSection {
	content = normalizeFactsDocument(content)
	if content == "" {
		return nil
	}

	sections := make([]*factsSection, 0)
	sectionMap := make(map[string]*factsSection)
	currentTitle := ""

	getSection := func(title string) *factsSection {
		if title == "" {
			title = planFactsGeneralSection
		}
		if sec, ok := sectionMap[title]; ok {
			return sec
		}
		sec := &factsSection{
			Title: title,
			Lines: make([]string, 0),
			seen:  make(map[string]struct{}),
		}
		sectionMap[title] = sec
		sections = append(sections, sec)
		return sec
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			currentTitle = line
			getSection(currentTitle)
			continue
		}
		sec := getSection(currentTitle)
		if _, exists := sec.seen[line]; exists {
			continue
		}
		sec.seen[line] = struct{}{}
		sec.Lines = append(sec.Lines, line)
	}

	filtered := make([]*factsSection, 0, len(sections))
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		filtered = append(filtered, sec)
	}
	return filtered
}

func mergeFactsDocuments(existing, incoming string) string {
	sections := parseFactsSections(existing)
	sectionMap := make(map[string]*factsSection, len(sections))
	for _, sec := range sections {
		sectionMap[sec.Title] = sec
	}

	for _, inc := range parseFactsSections(incoming) {
		target, ok := sectionMap[inc.Title]
		if !ok {
			target = &factsSection{
				Title: inc.Title,
				Lines: make([]string, 0, len(inc.Lines)),
				seen:  make(map[string]struct{}, len(inc.Lines)),
			}
			sections = append(sections, target)
			sectionMap[inc.Title] = target
		}
		for _, line := range inc.Lines {
			if _, exists := target.seen[line]; exists {
				continue
			}
			target.seen[line] = struct{}{}
			target.Lines = append(target.Lines, line)
		}
	}

	var blocks []string
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		blocks = append(blocks, sec.Title+"\n\n"+strings.Join(sec.Lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func appendPlanFacts(loop *reactloops.ReActLoop, incoming string) (string, bool) {
	incoming = normalizeFactsDocument(incoming)
	if incoming == "" {
		return loop.Get(PLAN_FACTS_KEY), false
	}

	existing := loop.Get(PLAN_FACTS_KEY)
	merged := mergeFactsDocuments(existing, incoming)
	if merged == normalizeFactsDocument(existing) {
		return merged, false
	}
	loop.Set(PLAN_FACTS_KEY, merged)
	return merged, true
}

func emitFactsMarkdown(loop *reactloops.ReActLoop, facts string) {
	facts = normalizeFactsDocument(facts)
	if facts == "" {
		return
	}

	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(PlanFactsAINodeID, strings.NewReader(facts), taskIndex, func() {}); err != nil {
			log.Warnf("plan loop: emit facts markdown failed: %v", err)
		}
	}
}

func getLoopTaskContext(loop *reactloops.ReActLoop) string {
	var parts []string
	for _, kv := range []struct {
		Title string
		Value string
	}{
		{Title: "已有事实", Value: loop.Get(PLAN_FACTS_KEY)},
		{Title: "已有计划", Value: loop.Get(PLAN_DATA_KEY)},
		{Title: "补充知识", Value: loop.Get(PLAN_ENHANCE_KEY)},
		{Title: "文件结果", Value: loop.Get(PLAN_FILE_RESULTS_KEY)},
		{Title: "互联网结果", Value: loop.Get(PLAN_WEB_RESULTS_KEY)},
		{Title: "侦查结果", Value: loop.Get(PLAN_RECON_RESULTS_KEY)},
	} {
		value := strings.TrimSpace(kv.Value)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s\n%s", kv.Title, utils.ShrinkString(value, 12000)))
	}
	return strings.Join(parts, "\n\n")
}

func autoGenerateFacts(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, mode string, lastAction *reactloops.ActionRecord) string {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	lastActionText := ""
	if lastAction != nil {
		lastActionText = fmt.Sprintf("ActionType: %s\nActionName: %s\nActionParams: %s", lastAction.ActionType, lastAction.ActionName, utils.InterfaceToString(lastAction.ActionParams))
	}

	instruction := "基于最新执行结果，只输出本轮新增且可验证的事实，不能重复已有事实。"
	if mode == "bootstrap" {
		instruction = "当前还没有任何已记录 facts。请基于用户原始诉求、已有 plan 和已收集结果，整理出一份初始事实文档。只能写明确存在于上下文里的事实，禁止编造。"
	}

	prompt := fmt.Sprintf(`你正在为任务规划循环生成 FACTS 文档。

要求：
- 输出 Markdown
- 使用 ## 标题组织内容
- 每条事实单独一行，优先使用 bullet point
- 只允许具体、可验证的信息
- 禁止使用“等”“其他”“若干”“一些”“相关”“类似”这类模糊词
- 如果没有新增事实，返回空字符串

本次模式：%s
%s

## 用户输入
%s

## 最近 action
%s

## 当前上下文
%s
`, mode, instruction, userInput, lastActionText, getLoopTaskContext(loop))

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"plan_facts_hook",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam(PlanFactsFieldName, aitool.WithParam_Description("增量 facts markdown；没有新增事实时返回空字符串")),
		},
	)
	if err != nil {
		log.Warnf("plan loop: auto generate facts failed: %v", err)
		return ""
	}
	if action == nil {
		return ""
	}
	return normalizeFactsDocument(action.GetString(PlanFactsFieldName))
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

func generateFallbackPlan(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) string {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	prompt := fmt.Sprintf(`你必须补全一个缺失的任务计划。

输出要求：
- 生成一个完整 plan
- main_task / main_task_goal 不能为空
- tasks 至少包含 1 个子任务
- 每个子任务都要有 subtask_name / subtask_goal / depends_on
- 只能基于已知上下文，不要编造不存在的环境信息

## 用户输入
%s

## 当前上下文
%s
`, userInput, getLoopTaskContext(loop))

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"plan_auto_fallback",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("main_task", aitool.WithParam_Required(true)),
			aitool.WithStringParam("main_task_identifier"),
			aitool.WithStringParam("main_task_goal", aitool.WithParam_Required(true)),
			aitool.WithStructArrayParam(
				"tasks",
				nil,
				nil,
				aitool.WithStringParam("subtask_name", aitool.WithParam_Required(true)),
				aitool.WithStringParam("subtask_identifier"),
				aitool.WithStringParam("subtask_goal", aitool.WithParam_Required(true)),
				aitool.WithStringArrayParam("depends_on"),
			),
		},
	)
	if err != nil {
		log.Warnf("plan loop: fallback plan generation failed: %v", err)
		return ""
	}
	if action == nil {
		return ""
	}

	tasks := action.GetInvokeParamsArray("tasks")
	if action.GetString("main_task") == "" || action.GetString("main_task_goal") == "" || len(tasks) == 0 {
		return ""
	}

	taskPayload := make([]map[string]any, 0, len(tasks))
	for _, subtask := range tasks {
		item := map[string]any{
			"subtask_name": subtask.GetString("subtask_name"),
			"subtask_goal": subtask.GetString("subtask_goal"),
			"depends_on":   subtask.GetStringSlice("depends_on"),
		}
		if identifier := subtask.GetString("subtask_identifier"); identifier != "" {
			item["subtask_identifier"] = identifier
		}
		taskPayload = append(taskPayload, item)
	}

	payload := map[string]any{
		"@action":        "plan",
		"main_task":      action.GetString("main_task"),
		"main_task_goal": action.GetString("main_task_goal"),
		"tasks":          taskPayload,
	}
	if identifier := action.GetString("main_task_identifier"); identifier != "" {
		payload["main_task_identifier"] = identifier
	}
	return string(utils.Jsonify(payload))
}

func shouldAutoFactsForAction(actionType string) bool {
	switch actionType {
	case "", "output_facts":
		return false
	default:
		return true
	}
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

func buildPlanPostIterationHook(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	_ = r
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		lastAction := loop.GetLastAction()
		if lastAction != nil && shouldAutoFactsForAction(lastAction.ActionType) {
			incoming := ""
			if lastAction.ActionParams != nil {
				incoming = normalizeFactsDocument(utils.InterfaceToString(lastAction.ActionParams[PlanFactsFieldName]))
			}
			if incoming == "" {
				incoming = autoGenerateFacts(loop, task, "incremental", lastAction)
			}
			if merged, changed := appendPlanFacts(loop, incoming); changed {
				emitFactsMarkdown(loop, merged)
				log.Infof("plan loop: post-action facts hook merged facts after action=%s iteration=%d", lastAction.ActionType, iteration)
			}
		}

		if !isDone {
			return
		}

		if strings.TrimSpace(loop.Get(PLAN_FACTS_KEY)) == "" {
			bootstrapFacts := autoGenerateFacts(loop, task, "bootstrap", lastAction)
			if merged, changed := appendPlanFacts(loop, bootstrapFacts); changed {
				emitFactsMarkdown(loop, merged)
				log.Infof("plan loop: generated bootstrap facts at finalization")
			}
		}

		if !hasValidPlan(loop) {
			fallbackPlan := generateFallbackPlan(loop, task)
			if fallbackPlan != "" {
				loop.Set(PLAN_DATA_KEY, fallbackPlan)
				log.Infof("plan loop: generated fallback plan at finalization")
			}
		}

		if isMaxIterationReason(reason) && hasValidPlan(loop) {
			operator.IgnoreError()
		}
	})
}
