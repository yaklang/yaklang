package aireactdeps

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/stdinsys"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/jsonpath"

	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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
	// 设置全局信号处理器
	SetupSignalHandler(ctx, config)

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

			// 审核输入现在由 promptui 直接处理，不需要这里的处理
			if globalState.IsWaitingForReview() {
				if config.DebugMode {
					log.Debugf("Skipping input during review (handled by promptui): '%s'", input)
				}
				continue
			}

			input = strings.TrimSpace(input)
			// 对于非审核输入，过滤空输入
			if input == "" {
				printPrompt()
				continue
			}

			if input == "exit" || input == "quit" {
				log.Info("User requested exit")
				os.Exit(0)
			}

			if input == "/debug" {
				toggleDebugMode(config)
				printPrompt()
				continue
			}

			if input == "/queue" {
				displayQueueInfo(reactInstance)
				printPrompt()
				continue
			}

			if strings.HasSuffix(input, "???") || input == "/status" {
				displayStatus()
				printPrompt()
				continue
			}

			if strings.HasPrefix(input, "/breakpoint") || strings.HasPrefix(input, "/bp") {
				config.BreakpointMode = true
				log.Info("Breakpoint mode enabled")
				printPrompt()
				continue
			}

			if strings.HasPrefix(input, "/timeline") {
				handleTimelineCommand(input, reactInstance)
				printPrompt()
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
			printPrompt()
		case <-ctx.Done():
			log.Info("Context cancelled, exiting interactive loop")
			return
		}
	}
}

// SetupSignalHandler 设置全局信号处理器
func SetupSignalHandler(ctx context.Context, config *CLIConfig) {
	go func() {
		ins := stdinsys.GetStdinSys()
		ins.GetDefaultStdinMirror()
		printPrompt()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !ins.HaveDefaultStdinMirror() {
					// 如果没有默认的stdin镜像，等待一段时间再继续
					time.Sleep(50 * time.Millisecond)
					continue
				}

				reader := ins.GetDefaultStdinMirror()
				line, err := utils.ReadLine(reader)
				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				handleFreeInput(string(line)+"\n", config)
			}
		}
	}()
}

// EventMonitor 事件监控器
type EventMonitor struct {
	lastEventTime time.Time
	mu            sync.RWMutex
	menuReader    *ClosableReader
	menuActive    bool
}

// NewEventMonitor 创建新的事件监控器
func NewEventMonitor() *EventMonitor {
	return &EventMonitor{
		lastEventTime: time.Now(),
		menuReader:    NewClosableReader(os.Stdin),
	}
}

// UpdateEventTime 更新最后事件时间
func (em *EventMonitor) UpdateEventTime() {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.lastEventTime = time.Now()
}

// ShouldShowMenu 检查是否应该显示菜单（3秒无事件）
func (em *EventMonitor) ShouldShowMenu() bool {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return time.Since(em.lastEventTime) > 3*time.Second
}

// CloseMenu 关闭主菜单IO
func (em *EventMonitor) CloseMenu() {
	em.mu.Lock()
	defer em.mu.Unlock()
	if em.menuReader != nil {
		em.menuReader.Close()
	}
	em.menuActive = false
	fmt.Println()
}

// ResetMenu 重置菜单IO
func (em *EventMonitor) ResetMenu() {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.menuReader = NewClosableReader(os.Stdin)
	em.menuActive = false
}

// IsMenuActive 检查菜单是否活跃
func (em *EventMonitor) IsMenuActive() bool {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.menuActive
}

// SetMenuActive 设置菜单活跃状态
func (em *EventMonitor) SetMenuActive(active bool) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.menuActive = active
}

// GetMenuReader 获取菜单Reader
func (em *EventMonitor) GetMenuReader() *ClosableReader {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.menuReader
}

// 全局事件监控器
var globalEventMonitor = NewEventMonitor()

// GetGlobalEventMonitor 获取全局事件监控器
func GetGlobalEventMonitor() *EventMonitor {
	return globalEventMonitor
}

// MainCommandOption 定义主命令选项
type MainCommandOption struct {
	Value       string
	Description string
	Icon        string
}

// handleFreeInput 处理自由输入
func handleFreeInput(input string, config *CLIConfig) {
	input = strings.TrimSpace(input)

	if input == "" {
		printPrompt()
		return
	}

	if config.DebugMode {
		log.Debugf("Free input received: '%s'", input)
	}

	// 处理特殊命令
	switch {
	case input == "exit" || input == "quit":
		fmt.Printf("用户请求退出\n")
		os.Exit(0)
	case input == "/debug":
		toggleDebugMode(config)
		return
	case input == "/queue":
		displayQueueInfo(nil) // TODO: 需要传入reactInstance
		return
	case input == "/status":
		displayStatus()
		return
	case strings.HasPrefix(input, "/timeline"):
		handleTimelineCommand(input, nil) // TODO: 需要传入reactInstance
		return
	case input == "/breakpoint" || input == "/bp":
		config.BreakpointMode = true
		log.Info("Breakpoint mode enabled")
		return
	case strings.HasSuffix(input, "???"):
		displayStatus()
		return
	}

	// 发送普通查询到全局通道
	select {
	case globalState.UserInput <- input:
		if config.DebugMode {
			log.Debugf("Input sent to channel: '%s'", input)
		}
		fmt.Printf("✓ 查询已发送: %s\n", input)
	default:
		log.Errorf("Failed to send input to channel: channel may be full")
	}
}

var banner = "Yak AI >> "

func printPrompt() {
	fmt.Print(banner)
}

var printPromptDebounce, _ = lo.NewDebounce(2*time.Second, printPrompt)

// handleClientEvent 在客户端模式下使用输入通道处理事件
func handleClientEvent(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent, interactiveMode bool) {
	config := &CLIConfig{} // 临时配置，应该从上下文获取

	printPromptDebounce()

	// 更新事件时间，用于菜单显示逻辑
	globalEventMonitor.UpdateEventTime()

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

			printPrompt()

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

		// 在交互模式下，处理用户交互
		if interactiveMode {
			handleTaskReviewRequireClient(event, inputChan)
		}
	case schema.EVENT_TYPE_REVIEW_RELEASE:
		// receive this message will release review/require status blocked
		fmt.Printf("[review-release]: %s\n", string(event.Content))

		// 清除菜单状态，允许主输入重新显示
		globalEventMonitor.CloseMenu()

		// 更新事件时间，重置3秒计时器
		globalEventMonitor.UpdateEventTime()
	case schema.EVENT_TYPE_STREAM:
		// 始终显示带有滚动效果的流事件
		fmt.Printf("[stream]: %s\n", string(event.StreamDelta))
	case schema.EVENT_TYPE_PRESSURE, schema.EVENT_TYPE_AID_CONFIG:
		log.Debugf("received event: %s", event.Type)
	case schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS:
		fmt.Println("[status]: AI first byte cost (ms): ", jsonpath.FindFirst(string(event.Content), "$.ms"))
	default:
		eventTypeStr := strings.ToLower(string(event.Type))

		// 特殊处理工具调用相关事件
		if eventTypeStr == "tool_call_status" || eventTypeStr == "tool_call_done" {
			fmt.Printf("[%s]: %s\n", eventTypeStr, string(event.Content))

			// 工具调用完成时，确保stdin状态正常
			if eventTypeStr == "tool_call_done" {
				// 重置事件时间，允许主输入在3秒后显示
				globalEventMonitor.UpdateEventTime()
				log.Debugf("Tool call completed, stdin control released")
			}
		} else {
			fmt.Printf("Unhandled [%s]: %s\n", eventTypeStr, string(event.Content))
		}
	}

	// 如果事件类型表明任务完成，强制触发提示
	if event.Type == schema.EVENT_TYPE_RESULT || strings.Contains(string(event.Content), "final message") {
		if config.DebugMode {
			log.Debugf("Force triggering completion prompt due to event type: %s", event.Type)
		}

		// 确保任务完成后stdin状态正常
		go func() {
			time.Sleep(500 * time.Millisecond)
			// 重置菜单状态
			globalEventMonitor.CloseMenu()
			globalEventMonitor.UpdateEventTime()
			os.Stdout.Sync()
			log.Debugf("Task completion cleanup completed")
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
