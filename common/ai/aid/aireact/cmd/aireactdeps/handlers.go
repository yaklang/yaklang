package aireactdeps

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/jsonpath"

	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps/promptui"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleInitialQuery 发送初始查询直接到 ReAct
func handleInitialQuery(reactInstance *aireact.ReAct, query string) {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   query,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		log.Errorf("Failed to send initial query: %v", err)
	} else {
		log.Infof("Initial query sent to ReAct: %s", query)
	}
}

// handleInteractiveLoop 处理持续的用户交互
func handleInteractiveLoop(reactInstance *aireact.ReAct, ctx context.Context, config *CLIConfig) {
	// 在后台启动全局输入读取器
	go globalInputReader(ctx, config)

	// 如果有初始查询正在运行，不要立即显示提示
	// 提示将在初始查询完成后显示
	firstInput := true

	for {
		if config.DebugMode {
			log.Debugf("Interactive loop: waiting for input...")
		}

		select {
		case input := <-globalState.UserInput:
			if config.DebugMode {
				log.Debugf("Interactive loop received input: '%s'", input)
			}

			// 首先检查是否正在等待断点输入
			if globalState.IsWaitingForBreakpoint() {
				if config.DebugMode {
					log.Debugf("Processing breakpoint input: '%s'", input)
				}
				// 信号断点输入已接收 - 断点函数将处理它
				globalState.SetBreakpointWaiting(false)
				continue
			}

			// 检查是否正在等待审核输入（在过滤空输入之前）
			if globalState.IsWaitingForReview() {
				if config.DebugMode {
					log.Debugf("Processing review input: '%s'", input)
				}
				// 在等待审核时始终立即处理审核输入
				// 允许审核的空输入（选择默认继续）
				processReviewInput(input, reactInstance)
				fmt.Print("> ")
				continue
			}

			// 对于非审核输入，过滤空输入
			if input == "" {
				fmt.Print("> ")
				continue
			}

			if input == "exit" || input == "quit" {
				log.Info("User requested exit")
				os.Exit(0)
			}

			if input == "/debug" {
				toggleDebugMode(config)
				fmt.Print("> ")
				continue
			}

			if input == "/queue" {
				displayQueueInfo(reactInstance)
				fmt.Print("> ")
				continue
			}

			if strings.HasSuffix(input, "???") || input == "/status" {
				displayStatus()
				fmt.Print("> ")
				continue
			}

			if strings.HasPrefix(input, "/breakpoint") || strings.HasPrefix(input, "/bp") {
				config.BreakpointMode = true
				log.Info("Breakpoint mode enabled")
				fmt.Print("> ")
				continue
			}

			if strings.HasPrefix(input, "/timeline") {
				handleTimelineCommand(input, reactInstance)
				fmt.Print("> ")
				continue
			}

			// 如果这是第一个常规输入或在任务完成后需要，显示交互提示
			if firstInput {
				showWelcomeMessage()
				firstInput = false
			}

			// 直接向 ReAct 发送查询
			if config.DebugMode {
				log.Debugf("Sending regular input to ReAct: '%s'", input)
			}

			event := &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   input,
			}

			err := reactInstance.SendInputEvent(event)
			if err != nil {
				fmt.Printf("Failed to send query: %v\n", err)
			} else {
				fmt.Printf("Query sent to ReAct: %s\n", input)
			}
			fmt.Print("> ")

		case <-ctx.Done():
			log.Info("Context cancelled, exiting interactive loop")
			return
		}
	}
}

// globalInputReader 从 stdin 读取并发送到全局通道
func globalInputReader(ctx context.Context, config *CLIConfig) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		input := strings.TrimSpace(scanner.Text())
		if config.DebugMode {
			log.Debugf("Input reader got: '%s'", input)
		}

		if config.DebugMode {
			log.Infof("start to put input into globalUserInput")
		}

		select {
		case globalState.UserInput <- input:
			// 成功发送
			if config.DebugMode {
				log.Debugf("Input sent to channel: '%s'", input)
			}
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
	}
}

// handleClientEvent 在客户端模式下使用输入通道处理事件
func handleClientEvent(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent, interactiveMode bool) {
	config := &CLIConfig{} // 临时配置，应该从上下文获取

	if config.DebugMode {
		content := string(event.Content)
		preview := content
		if len(content) > 100 {
			preview = content[:100] + "..."
		}
		log.Debugf("Handling client event: type=%s, content_preview=%s", event.Type, preview)
	}

	// 使用简化显示处理输出事件
	switch event.Type {
	case schema.EVENT_TYPE_CONSUMPTION:
	// keep quiet
	case schema.EVENT_TYPE_THOUGHT:
		fmt.Printf("[think]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_ACTION:
		fmt.Printf("[action]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_OBSERVATION:
		fmt.Printf("[observe]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_RESULT:
		if config.DebugMode {
			log.Debugf("Processing EVENT_TYPE_RESULT case")
		}
		result := extractResultContent(string(event.Content))
		fmt.Printf("[result]: %s\n", result)
		fmt.Printf("[ai]: final message for current loop\n")

		// 当 ReAct 循环完成时重置审核状态
		globalState.SetReviewState(false, nil, "")

		// 任务完成后显示下一次交互的提示
		if config.DebugMode {
			log.Debugf("Task completed, showing prompt after delay...")
		}

		go func() {
			// 添加更长的延迟以确保所有输出都被刷新
			time.Sleep(500 * time.Millisecond)

			if config.DebugMode {
				log.Debugf("Displaying task completion prompt now")
			}

			fmt.Print("> ")

			// 多次强制刷新输出
			os.Stdout.Sync()
			os.Stderr.Sync()

			if config.DebugMode {
				log.Debugf("Task completion prompt displayed and flushed")
			}
		}()

	case schema.EVENT_TYPE_STRUCTURED:
		handleStructuredEvent(string(event.Content), config.DebugMode)
	case schema.EVENT_TYPE_ITERATION:
		if config.DebugMode {
			fmt.Printf("DEBUG: [iteration]: %s\n", string(event.Content))
		}
	case schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE:
		// 处理工具审核事件
		fmt.Printf("[tool_review]: %s\n", string(event.Content))

		// 在交互模式下，处理用户交互
		if interactiveMode {
			handleReviewRequireClient(event, inputChan)
		} else {

		}
	case schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE:
		fmt.Printf("[require-user-interative] received, start to trigger user option")
		fmt.Println(string(event.Content))

		// 在交互模式下，处理用户交互
		if interactiveMode {
			handleUserInteractiveClient(event, inputChan)
		}
	case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE:
		fmt.Printf("[plan_review]: %s\n", string(event.Content))

		// 在交互模式下，处理用户交互
		if interactiveMode {
			handlePlanReviewRequireClient(event, inputChan)
		}
	case schema.EVENT_TYPE_TASK_REVIEW_REQUIRE:
		fmt.Printf("[task_review]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_REVIEW_RELEASE:
		// receive this message will release review/require status blocked
		fmt.Printf("[review-release]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_STREAM:
		// 始终显示带有滚动效果的流事件
		fmt.Printf("[stream]: %s\n", string(event.StreamDelta))
	case schema.EVENT_TYPE_PRESSURE, schema.EVENT_TYPE_AID_CONFIG:
		log.Debugf("received event: %s", event.Type)
	case schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS:
		fmt.Println("[status]: AI first byte cost (ms): ", jsonpath.FindFirst(string(event.Content), "$.ms"))
	default:
		fmt.Printf("Unhandled [%s]: %s\n", strings.ToLower(string(event.Type)), string(event.Content))
	}

	// 如果事件类型表明任务完成，强制触发提示
	if event.Type == schema.EVENT_TYPE_RESULT || strings.Contains(string(event.Content), "final message") {
		if config.DebugMode {
			log.Debugf("Force triggering completion prompt due to event type: %s", event.Type)
		}
		go func() {
			time.Sleep(1 * time.Second) // 更长的延迟
			os.Stdout.Sync()
		}()
	}
}

// extractResultContent 从JSON结果中提取实际结果并格式化以获得更好的可读性
func extractResultContent(content string) string {
	// 尝试从JSON中提取"result"字段
	if strings.Contains(content, `"result"`) {
		start := strings.Index(content, `"result":"`)
		if start != -1 {
			start += 10 // `"result":"` 的长度
			end := strings.Index(content[start:], `"`)
			if end != -1 {
				result := content[start : start+end]
				// 取消转义JSON字符串
				result = strings.ReplaceAll(result, `\"`, `"`)
				result = strings.ReplaceAll(result, `\\`, `\`)
				result = strings.ReplaceAll(result, `\n`, "\n")
				result = strings.ReplaceAll(result, `\t`, "\t")
				return result
			}
		}
	}

	// 如果它已经是人类可读的文本，按原样返回
	return content
}

// displayQueueInfo 显示 ReAct 队列信息
func displayQueueInfo(reactInstance *aireact.ReAct) {
	// 使用标准的 AIInputEvent 发送同步请求
	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aireact.SYNC_TYPE_QUEUE_INFO,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		fmt.Printf("Failed to get queue info: %v\n", err)
		return
	}
	fmt.Println("Queue info request sent - check output events for details")
}

// handleTimelineCommand 处理时间线命令
func handleTimelineCommand(input string, reactInstance *aireact.ReAct) {
	// 解析可选的限制参数
	parts := strings.Fields(input)
	limit := 20 // 默认限制
	if len(parts) > 1 {
		if parsedLimit, err := strconv.Atoi(parts[1]); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	displayTimelineInfo(reactInstance, limit)
}

// displayTimelineInfo 显示 ReAct 时间线信息
func displayTimelineInfo(reactInstance *aireact.ReAct, limit int) {
	// 使用标准的 AIInputEvent 发送同步请求
	var syncJsonInput string
	if limit > 0 {
		params := map[string]interface{}{
			"limit": limit,
		}
		if paramsJson, err := json.Marshal(params); err == nil {
			syncJsonInput = string(paramsJson)
		}
	}

	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aireact.SYNC_TYPE_TIMELINE,
		SyncJsonInput: syncJsonInput,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		fmt.Printf("Failed to get timeline info: %v\n", err)
		return
	}
	fmt.Printf("Timeline info request sent (limit: %d) - check output events for details\n", limit)
}

// toggleDebugMode 切换调试模式
func toggleDebugMode(config *CLIConfig) {
	config.DebugMode = !config.DebugMode
	if config.DebugMode {
		fmt.Println("[debug]: enabled")
		log.SetLevel(log.DebugLevel)
	} else {
		fmt.Println("[debug]: disabled")
		log.SetLevel(log.InfoLevel)
	}
}

// displayStatus 显示系统状态
func displayStatus() {
	waiting, options, _ := globalState.GetReviewState()
	fmt.Printf("\n=== SYSTEM STATUS ===\n")
	fmt.Printf("Debug mode: %v\n", true) // 需要从配置获取
	fmt.Printf("Waiting for review: %v\n", waiting)
	fmt.Printf("Review options count: %d\n", len(options))
	fmt.Printf("====================\n")

	// 强制显示提示
	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("🎯 Manual prompt trigger! Ready for next question.\n")
	showWelcomeMessage()
}

// showWelcomeMessage 显示欢迎消息
func showWelcomeMessage() {
	fmt.Printf("ReAct CLI ready. Enter your question (type 'exit' to quit, '/debug' to toggle debug mode, '/queue' to view queue, '/timeline [limit]' to view timeline):\n")
}

// handleStructuredEvent 处理结构化事件
func handleStructuredEvent(content string, debugMode bool) {
	if strings.Contains(content, "queue_name") {
		fmt.Printf("\n=== REACT QUEUE INFO ===\n")
		fmt.Printf("%s\n", content)
		fmt.Printf("========================\n\n")
	} else if strings.Contains(content, "total_entries") {
		displayFormattedTimeline(content)
	} else if debugMode {
		fmt.Printf("[structured]: %s\n", content)
	}
}

// displayFormattedTimeline 显示格式化的时间线信息
func displayFormattedTimeline(jsonContent string) {
	// 解析JSON内容
	var timelineData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &timelineData); err != nil {
		log.Errorf("Failed to parse timeline JSON: %v", err)
		fmt.Printf("\n=== REACT TIMELINE ===\n")
		fmt.Printf("%s\n", jsonContent)
		fmt.Printf("======================\n\n")
		return
	}

	// 提取基本信息
	totalEntries, _ := timelineData["total_entries"].(float64)
	limit, _ := timelineData["limit"].(float64)
	entriesData, _ := timelineData["entries"].([]interface{})

	// 显示标题和统计信息
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                                🕐 REACT TIMELINE                             ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Total Entries: %-3.0f │ Showing: %-3.0f │ Timeline Activity Overview         ║\n", totalEntries, limit)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")

	if len(entriesData) == 0 {
		fmt.Printf("┌─ No timeline entries available\n")
		fmt.Printf("└─ Timeline is empty\n\n")
		return
	}

	// 显示时间线条目
	for i, entryData := range entriesData {
		entryMap, ok := entryData.(map[string]interface{})
		if !ok {
			continue
		}

		// 解析时间戳
		timestampStr, _ := entryMap["timestamp"].(string)
		entryType, _ := entryMap["type"].(string)
		content, _ := entryMap["content"].(string)

		// 解析时间
		var timeStr string
		if timestamp, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
			timeStr = timestamp.Format("15:04:05.000")
		} else {
			timeStr = "unknown"
		}

		// 根据类型选择图标和颜色前缀
		var icon, typeDisplay string
		switch entryType {
		case "tool_result":
			icon = "🔧"
			typeDisplay = "TOOL"
		case "user_interaction":
			icon = "👤"
			typeDisplay = "USER"
		case "text":
			icon = "📝"
			typeDisplay = "TEXT"
		default:
			icon = "❓"
			typeDisplay = strings.ToUpper(entryType)
		}

		// 显示连接线
		isLast := i == len(entriesData)-1
		connector := "├─"
		if isLast {
			connector = "└─"
		}

		// 显示主要条目信息
		fmt.Printf("%s[%s] %s %s\n", connector, timeStr, icon, typeDisplay)

		// 处理内容显示
		if content != "" {
			contentLines := utils.ParseStringToRawLines(content)
			for j, line := range contentLines {
				// 限制每行长度避免过宽显示
				if len(line) > 100 {
					line = line[:97] + "..."
				}

				linePrefix := "│    "
				if isLast {
					linePrefix = "     "
				}

				// 对于第一行，显示内容标题
				if j == 0 && len(contentLines) > 1 {
					fmt.Printf("%s┌─ Content:\n", linePrefix)
					fmt.Printf("%s│  %s\n", linePrefix, line)
				} else if j == 0 {
					fmt.Printf("%s━━ %s\n", linePrefix, line)
				} else if j == len(contentLines)-1 && len(contentLines) > 1 {
					fmt.Printf("%s└─ %s\n", linePrefix, line)
				} else {
					fmt.Printf("%s│  %s\n", linePrefix, line)
				}

				// 限制显示行数避免过长输出
				if j >= 8 && len(contentLines) > 10 {
					remaining := len(contentLines) - j - 1
					fmt.Printf("%s└─ ... (%d more lines)\n", linePrefix, remaining)
					break
				}
			}
		}

		// 添加条目间的分隔
		if !isLast {
			fmt.Printf("│\n")
		}
	}

	fmt.Printf("\n")
}

// PlanSelector 定义计划审核选择器项
type PlanSelector struct {
	ID               string `json:"id"`
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`
	ParamSchema      string `json:"param_schema"`
}

// handlePlanReviewRequireClient 使用 promptui 处理 PLAN_REVIEW_REQUIRE 事件
func handlePlanReviewRequireClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
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
	}

	fmt.Printf("\n[PLAN REVIEW REQUIRED]\n")
	fmt.Printf("请审核AI制定的执行计划，选择您的操作：\n\n")

	selectedIndex, _, err := prompt.Run()
	if err != nil {
		log.Errorf("Prompt failed: %v", err)
		return
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

	// 构建响应 - 使用选项的索引
	response := map[string]interface{}{
		"choice":          selectedOption.Index,
		"selected_option": selectedOption.PromptTitle,
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
