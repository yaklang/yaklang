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

// getString 安全地从映射中提取字符串值
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// handleReviewRequireClient 使用 promptui 处理 TOOL_USE_REVIEW_REQUIRE 事件
func handleReviewRequireClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
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
		log.Errorf("Failed to parse review event: %v", err)
		return
	}

	// 从事件中提取信息
	eventID := event.GetInteractiveId()
	if eventID == "" {
		log.Errorf("No interactive ID found in review event")
		return
	}

	toolName, _ := reviewData["tool"].(string)
	toolDesc, _ := reviewData["tool_description"].(string)
	selectors, _ := reviewData["selectors"].([]interface{})

	// 显示工具信息
	fmt.Printf("\n[TOOL REVIEW REQUIRED]\n")
	fmt.Printf("工具: %s\n", toolName)
	if toolDesc != "" {
		fmt.Printf("描述: %s\n", toolDesc)
	}

	// 显示参数信息（调试模式）
	if params, ok := reviewData["params"]; ok {
		fmt.Printf("参数: %v\n", params)
	}

	// 显示选择器（如果可用）
	var options []ReviewOption
	if len(selectors) > 0 {
		for _, sel := range selectors {
			if selMap, ok := sel.(map[string]interface{}); ok {
				option := ReviewOption{
					Value:  getString(selMap, "value"),
					Prompt: getString(selMap, "prompt"),
				}
				if option.Prompt == "" {
					option.Prompt = getString(selMap, "prompt_english")
				}
				options = append(options, option)
			}
		}
	}

	// 如果没有提供选项，使用默认选项，默认为 continue
	if len(options) == 0 {
		options = []ReviewOption{
			{Value: "continue", Prompt: "同意工具使用"},
			{Value: "wrong_tool", Prompt: "工具选择不当"},
			{Value: "wrong_params", Prompt: "参数不合理"},
			{Value: "direct_answer", Prompt: "要求AI直接回答"},
		}
	}

	var idx int
	for idxNow, i := range options {
		if i.Value == "continue" {
			idx = idxNow
			break
		}
	}

	// 将 continue 选项移到第一位
	if idx > 0 {
		continueSelector := options[idx]
		// 移除原位置的 continue 选项
		options = append(options[:idx], options[idx+1:]...)
		// 将 continue 选项插入到第一位
		options = append([]ReviewOption{continueSelector}, options...)
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
{{ "描述:" | faint }}	{{ .Prompt }}`,
	}

	searcher := func(input string, index int) bool {
		option := options[index]
		name := strings.Replace(strings.ToLower(option.Prompt), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)
		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     "请选择对该工具使用的操作",
		Items:     options,
		Templates: templates,
		Size:      4,
		Searcher:  searcher,
		Stdin:     io.NopCloser(stdin),
	}

	fmt.Printf("\n请审核AI要使用的工具，选择您的操作：\n\n")

	var selectedIndex int
	var err error
	for {
		selectedIndex, _, err = prompt.Run()
		if err != nil {
			log.Errorf("Tool Use prompt failed: %v", err)
			// 发生错误时默认选择 continue
			sendToolReviewResponse(inputChan, eventID, "continue")
			continue
		}
		break
	}

	selectedOption := options[selectedIndex]
	fmt.Printf("\n您选择了: %s - %s\n", selectedOption.Value, selectedOption.Prompt)

	// 如果选择了需要额外输入的选项，询问用户
	var extraPrompt string
	if selectedOption.Value != "continue" {
		extraPromptUI := promptui.Prompt{
			Label: "请提供额外的指导意见 (可选，直接回车跳过)",
		}
		extraPrompt, _ = extraPromptUI.Run()
	}

	// 发送响应
	sendToolReviewResponse(inputChan, eventID, selectedOption.Value, extraPrompt)
}

// 辅助函数

// sendToolReviewResponse 发送工具审核响应
func sendToolReviewResponse(inputChan chan<- *ypb.AIInputEvent, eventID, suggestion string, extraPrompt ...string) {
	// 构建响应
	response := map[string]interface{}{
		"suggestion": suggestion,
	}

	if len(extraPrompt) > 0 && strings.TrimSpace(extraPrompt[0]) != "" {
		response["extra_prompt"] = strings.TrimSpace(extraPrompt[0])
	}

	responseJSON, _ := json.Marshal(response)

	// 创建并发送输入事件
	inputEvent := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        eventID,
		InteractiveJSONInput: string(responseJSON),
	}

	// 发送输入事件
	select {
	case inputChan <- inputEvent:
		fmt.Printf("✓ 已发送您的选择: %s\n\n", suggestion)
	default:
		log.Errorf("Failed to send tool review input event")
	}
}

// parseSelectionIndex 将用户输入解析为选择索引（基于1）并返回基于0的索引，如果无效则返回-1
func parseSelectionIndex(input string, maxOptions int) int {
	if len(input) == 1 && input[0] >= '1' && input[0] <= '9' {
		idx := int(input[0] - '1')
		if idx < maxOptions {
			return idx
		}
	}
	return -1
}
