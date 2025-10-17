package aireactdeps

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/stdinsys"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps/promptui"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// PlanSelector 定义计划审核选择器项
type PlanSelector struct {
	ID               string `json:"id"`
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`
	ParamSchema      string `json:"param_schema"`
}

// TaskSelector 定义任务审核选择器项
type TaskSelector struct {
	ID               string `json:"id"`
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`
	ParamSchema      string `json:"param_schema"`
}

// handlePlanReviewRequireClient 使用 promptui 处理 PLAN_REVIEW_REQUIRE 事件
func handlePlanReviewRequireClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
	ins := stdinsys.GetStdinSys()
	ins.PreventDefaultStdinMirror()
	defer ins.GetDefaultStdinMirror()
	id, stdin := ins.CreateTemporaryStdinMirror()
	defer func() {
		ins.RemoveStdinMirror(id)
	}()

	// 解析审核事件内容
	var reviewData map[string]interface{}
	if err := json.Unmarshal(event.Content, &reviewData); err != nil {
		log.Errorf("Failed to parse plan review event: %v", err)
		return
	}

	// 从事件中提取信息
	eventID := event.GetInteractiveId()
	if eventID == "" {
		log.Errorf("No interactive ID found in plan review event")
		return
	}

	// 提取 selectors
	selectors, _ := reviewData["selectors"].([]interface{})
	var planSelectors []PlanSelector

	if len(selectors) > 0 {
		for _, sel := range selectors {
			if selMap, ok := sel.(map[string]interface{}); ok {
				selector := PlanSelector{
					ID:               getString(selMap, "id"),
					Value:            getString(selMap, "value"),
					Prompt:           getString(selMap, "prompt"),
					PromptEnglish:    getString(selMap, "prompt_english"),
					AllowExtraPrompt: getBool(selMap, "allow_extra_prompt"),
					ParamSchema:      getString(selMap, "param_schema"),
				}
				planSelectors = append(planSelectors, selector)
			}
		}
	}

	// 如果没有提供选项，使用默认选项
	if len(planSelectors) == 0 {
		planSelectors = []PlanSelector{
			{Value: "continue", Prompt: "计划合理，继续执行", PromptEnglish: "The plan is reasonable, continue execution"},
			{Value: "unclear", Prompt: "目标不明确", PromptEnglish: "The plan is too vague and fuzzy"},
			{Value: "incomplete", Prompt: "有遗漏", PromptEnglish: "The plan is not complete enough"},
		}
	}

	var idx int
	for idxNow, i := range planSelectors {
		if i.Value == "continue" {
			idx = idxNow
			break
		}
	}

	// 将 continue 选项移到第一位
	if idx > 0 {
		continueSelector := planSelectors[idx]
		// 移除原位置的 continue 选项
		planSelectors = append(planSelectors[:idx], planSelectors[idx+1:]...)
		// 将 continue 选项插入到第一位
		planSelectors = append([]PlanSelector{continueSelector}, planSelectors...)
	}

	// 创建 promptui 选择器
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "▶ {{ .Prompt | cyan }}",
		Inactive: "  {{ .Prompt }}",
		Selected: "✓ {{ .Prompt | green }}",
		Details: `
--------- 选项详情 ----------
{{ "操作:" | faint }}	{{ .Value }}
{{ "描述:" | faint }}	{{ .Prompt }}
{{ if .PromptEnglish }}{{ "English:" | faint }}	{{ .PromptEnglish }}{{ end }}`,
	}

	searcher := func(input string, index int) bool {
		selector := planSelectors[index]
		name := strings.Replace(strings.ToLower(selector.Prompt), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)
		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     "请选择对该计划的操作",
		Items:     planSelectors,
		Templates: templates,
		Size:      4,
		Searcher:  searcher,
		Stdin:     io.NopCloser(stdin),
	}

	fmt.Printf("\n[PLAN REVIEW REQUIRED]\n")
	fmt.Printf("请审核AI制定的执行计划，选择您的操作：\n\n")

	var selectedIndex int
	var err error
	for {
		selectedIndex, _, err = prompt.Run()
		if err != nil {
			log.Warnf("Plan review prompt skipped: %v, with option continue", err)
			// 如果是 Ctrl+C 中断，提示用户再次按 Ctrl+C 退出
			if err.Error() == "^C" {
				fmt.Println("按 Ctrl+C 再次退出程序")
			}
			// 发生错误时默认选择第一个选项（通常是 continue）
			selectedIndex = 0
			continue
		}
		break
	}

	selectedSelector := planSelectors[selectedIndex]
	fmt.Printf("\n您选择了: %s - %s\n", selectedSelector.Value, selectedSelector.Prompt)

	// 如果需要额外输入，询问用户
	var extraPrompt string
	if selectedSelector.AllowExtraPrompt && selectedSelector.Value != "continue" {
		extraPromptUI := promptui.Prompt{
			Label: "请提供额外的指导意见 (可选，直接回车跳过)",
		}
		extraPrompt, _ = extraPromptUI.Run()
	}

	// 构建响应
	response := map[string]interface{}{
		"suggestion": selectedSelector.Value,
	}

	if strings.TrimSpace(extraPrompt) != "" {
		response["extra_prompt"] = strings.TrimSpace(extraPrompt)
	}

	responseJSON, _ := json.Marshal(response)

	// 创建并发送输入事件到 ReAct
	inputEvent := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        eventID,
		InteractiveJSONInput: string(responseJSON),
	}

	// 发送输入事件
	select {
	case inputChan <- inputEvent:
		fmt.Printf("✓ 已发送您的选择: %s\n\n", selectedSelector.Value)
	default:
		log.Errorf("Failed to send plan review input event")
	}
}
