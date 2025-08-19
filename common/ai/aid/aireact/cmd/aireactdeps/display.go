package aireactdeps

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"time"
)

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
