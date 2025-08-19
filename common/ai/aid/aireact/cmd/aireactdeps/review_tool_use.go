package aireactdeps

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
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

// handleReviewRequireClient 使用输入通道处理 TOOL_USE_REVIEW_REQUIRE 事件
func handleReviewRequireClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
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
	fmt.Printf("Tool: %s\n", toolName)
	if toolDesc != "" {
		fmt.Printf("Description: %s\n", toolDesc)
	}

	config := &CLIConfig{DebugMode: true} // 临时配置
	if config.DebugMode {
		if params, ok := reviewData["params"]; ok {
			fmt.Printf("Parameters: %v\n", params)
		}
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

	// 如果没有提供选项，使用默认选项
	if len(options) == 0 {
		options = []ReviewOption{
			{Value: "continue", Prompt: "同意工具使用"},
			{Value: "wrong_tool", Prompt: "工具选择不当"},
			{Value: "wrong_params", Prompt: "参数不合理"},
			{Value: "direct_answer", Prompt: "要求AI直接回答"},
		}
	}

	// 显示选项
	fmt.Printf("\nPlease choose an action:\n")
	for i, option := range options {
		if option.Value == "continue" {
			fmt.Printf("  %d. %s - %s (default, press Enter)\n", i+1, option.Value, option.Prompt)
		} else {
			fmt.Printf("  %d. %s - %s\n", i+1, option.Value, option.Prompt)
		}
	}

	// 检查继续选项是否存在以显示提示消息
	hasContinue := false
	for _, option := range options {
		if option.Value == "continue" {
			hasContinue = true
			break
		}
	}

	if hasContinue {
		fmt.Printf("Your choice (1-%d, Enter for continue): ", len(options))
	} else {
		fmt.Printf("Your choice (1-%d): ", len(options))
	}

	// 设置审核状态并等待全局输入
	globalState.SetReviewState(true, options, eventID)
	//// 添加超时机制以在没有收到输入时自动继续
	//go func(eventID string) {
	//	time.Sleep(60 * time.Second) // 60秒超时
	//	waiting, _, currentEventID := globalState.GetReviewState()
	//	if waiting && currentEventID == eventID {
	//		log.Warnf("Review timeout reached, auto-selecting continue")
	//		globalState.SetReviewState(false, nil, "")
	//
	//		// 直接发送继续响应
	//		inputEvent := &ypb.AIInputEvent{
	//			IsInteractiveMessage: true,
	//			InteractiveId:        eventID,
	//			InteractiveJSONInput: `{"suggestion": "continue"}`,
	//		}
	//
	//		// 尝试通过 inputChan 发送
	//		select {
	//		case inputChan <- inputEvent:
	//			fmt.Printf("\n[TIMEOUT]: Auto-selected continue after 60 seconds\n> ")
	//		default:
	//			log.Errorf("Failed to send timeout input event")
	//		}
	//	}
	//}(eventID)

	// processReviewInput 函数将在输入到达时处理实际输入
}

// processReviewInput 处理审核选择的用户输入
func processReviewInput(input string, reactInstance *aireact.ReAct) {
	waiting, options, eventID := globalState.GetReviewState()
	if !waiting {
		return
	}

	var selectedValue string

	// 处理空输入（只是按回车）
	if strings.TrimSpace(input) == "" {
		// 首先寻找 "continue" 选项
		for _, option := range options {
			if option.Value == "continue" {
				selectedValue = "continue"
				fmt.Printf("[REVIEW]: Empty input detected, selecting default: %s\n", selectedValue)
				break
			}
		}
		// 如果没有找到 "continue" 选项，使用第一个选项
		if selectedValue == "" {
			selectedValue = options[0].Value
			fmt.Printf("[REVIEW]: Empty input detected, selecting first option: %s\n", selectedValue)
		}
	} else {
		// 首先尝试解析为数字
		if idx := parseSelectionIndex(input, len(options)); idx >= 0 {
			selectedValue = options[idx].Value
		} else {
			// 尝试按值匹配
			for _, option := range options {
				if strings.EqualFold(input, option.Value) {
					selectedValue = option.Value
					break
				}
			}
		}

		// 如果可用，默认为继续，否则为第一个选项
		if selectedValue == "" {
			// 首先寻找 "continue" 选项
			for _, option := range options {
				if option.Value == "continue" {
					selectedValue = "continue"
					fmt.Printf("[REVIEW]: Invalid input '%s', defaulting to %s\n", input, selectedValue)
					break
				}
			}
			// 如果没有找到 "continue" 选项，使用第一个选项
			if selectedValue == "" {
				selectedValue = options[0].Value
				fmt.Printf("[REVIEW]: Invalid input '%s', defaulting to %s\n", input, selectedValue)
			}
		} else {
			fmt.Printf("[REVIEW]: Selected action: %s\n", selectedValue)
		}
	}

	// 创建并发送输入事件到 ReAct
	inputEvent := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        eventID,
		InteractiveJSONInput: fmt.Sprintf(`{"suggestion": "%s"}`, selectedValue),
	}

	// 通过 ReAct 发送输入事件
	err := reactInstance.SendInputEvent(inputEvent)
	if err != nil {
		log.Errorf("Failed to send input event: %v", err)
	}

	fmt.Print("Continuing with ReAct processing...\n\n")

	globalState.SetReviewState(false, nil, "")
}

// 辅助函数

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
