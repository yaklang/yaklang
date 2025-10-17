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

// UserInteractiveOption 定义用户交互选项
type UserInteractiveOption struct {
	Index             int    `json:"index"`
	PromptTitle       string `json:"prompt_title"`
	OptionName        string `json:"option_name"`
	OptionDescription string `json:"option_description"`
	Prompt            string `json:"prompt"`
}

// handleUserInteractiveClient 使用 promptui 处理 EVENT_TYPE_REQUIRE_USER_INTERACTIVE 事件
func handleUserInteractiveClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
	ins := stdinsys.GetStdinSys()
	ins.PreventDefaultStdinMirror()
	defer ins.GetDefaultStdinMirror()
	id, stdin := ins.CreateTemporaryStdinMirror()
	defer func() {
		ins.RemoveStdinMirror(id)
	}()

	// 解析交互事件内容
	var interactiveData map[string]interface{}
	if err := json.Unmarshal(event.Content, &interactiveData); err != nil {
		log.Errorf("Failed to parse interactive event: %v", err)
		return
	}

	// 从事件中提取信息
	eventID := event.GetInteractiveId()
	if eventID == "" {
		log.Errorf("No interactive ID found in interactive event")
		return
	}

	question := getString(interactiveData, "prompt")
	if question == "" {
		question = getString(interactiveData, "question")
	}

	// 提取选项
	options, _ := interactiveData["options"].([]interface{})
	var userOptions []UserInteractiveOption

	if len(options) > 0 {
		for i, opt := range options {
			if optMap, ok := opt.(map[string]interface{}); ok {
				option := UserInteractiveOption{
					Index:             i,
					PromptTitle:       getString(optMap, "prompt_title"),
					OptionName:        getString(optMap, "option_name"),
					OptionDescription: getString(optMap, "option_description"),
					Prompt:            getString(optMap, "prompt"),
				}

				// 如果有 option_name，优先使用它作为显示文本
				if option.OptionName != "" && option.PromptTitle == "" {
					option.PromptTitle = option.OptionName
				}

				userOptions = append(userOptions, option)
			}
		}
	}

	// 如果没有提供选项，创建默认选项
	if len(userOptions) == 0 {
		userOptions = []UserInteractiveOption{
			{Index: 0, PromptTitle: "继续", OptionDescription: "继续执行"},
			{Index: 1, PromptTitle: "取消", OptionDescription: "取消操作"},
		}
	}

	// 查找 "继续" 选项并将其移到第一位
	var continueIndex = -1
	for i, option := range userOptions {
		if strings.Contains(strings.ToLower(option.PromptTitle), "继续") ||
			strings.Contains(strings.ToLower(option.PromptTitle), "continue") {
			continueIndex = i
			break
		}
	}

	// 如果找到了 "继续" 选项且不在第一位，将其移到第一位
	if continueIndex > 0 {
		continueOption := userOptions[continueIndex]
		// 移除原位置的 "继续" 选项
		userOptions = append(userOptions[:continueIndex], userOptions[continueIndex+1:]...)
		// 将 "继续" 选项插入到第一位
		userOptions = append([]UserInteractiveOption{continueOption}, userOptions...)

		// 重新设置索引
		for i := range userOptions {
			userOptions[i].Index = i
		}
	}

	// 创建 promptui 选择器
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "▶ {{ .PromptTitle | cyan }}",
		Inactive: "  {{ .PromptTitle }}",
		Selected: "✓ {{ .PromptTitle | green }}",
		Details: `
--------- 选项详情 ----------
{{ "选项:" | faint }}	{{ .PromptTitle }}
{{ if .OptionDescription }}{{ "描述:" | faint }}	{{ .OptionDescription }}{{ end }}`,
	}

	searcher := func(input string, index int) bool {
		option := userOptions[index]
		name := strings.Replace(strings.ToLower(option.PromptTitle), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)
		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     question,
		Items:     userOptions,
		Templates: templates,
		Size:      4,
		Searcher:  searcher,
		Stdin:     io.NopCloser(stdin),
	}

	fmt.Printf("\n[用户交互请求]\n")
	fmt.Printf("问题: %s\n\n", question)
	fmt.Printf("请选择您的回答：\n\n")

	selectedIndex, _, err := prompt.Run()
	if err != nil {
		log.Errorf("Prompt failed: %v", err)
		return
	}

	selectedOption := userOptions[selectedIndex]
	fmt.Printf("\n您选择了: %s", selectedOption.PromptTitle)
	if selectedOption.OptionDescription != "" {
		fmt.Printf(" - %s", selectedOption.OptionDescription)
	}
	fmt.Printf("\n")
	// 构建响应 - 根据用户选择的选项构建响应
	response := map[string]interface{}{
		"suggestion": selectedOption.PromptTitle,
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
		fmt.Printf("✓ 已发送您的选择: %s\n\n", selectedOption.PromptTitle)
	default:
		log.Errorf("Failed to send user interactive input event")
	}
}

// getBool 安全地从映射中提取布尔值
func getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}
