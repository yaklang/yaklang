package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// searchRelevantMemories 根据反思级别搜索相关记忆
func (r *ReActLoop) searchRelevantMemories(reflection *ActionReflection, level ReflectionLevel) string {
	// 如果没有 memoryTriage，返回空
	if r.memoryTriage == nil {
		log.Debug("memory triage not available, skip memory search")
		return ""
	}

	// 根据反思级别决定搜索深度
	var searchSizeLimit int
	switch level {
	case ReflectionLevel_Minimal:
		return "" // 最小级别不搜索记忆
	case ReflectionLevel_Standard:
		searchSizeLimit = 2 * 1024 // 2KB
	case ReflectionLevel_Deep:
		searchSizeLimit = 5 * 1024 // 5KB
	case ReflectionLevel_Critical:
		searchSizeLimit = 10 * 1024 // 10KB - 关键反思需要更多上下文
	default:
		return ""
	}

	// 构建搜索查询
	query := fmt.Sprintf("action '%s' execution analysis failure success pattern",
		reflection.ActionType)

	if !reflection.Success && reflection.ErrorMessage != "" {
		query += " " + reflection.ErrorMessage
	}

	log.Infof("searching memories for reflection with query[%s], size_limit[%d]",
		query, searchSizeLimit)

	// 搜索记忆
	searchResult, err := r.memoryTriage.SearchMemory(query, searchSizeLimit)
	if err != nil {
		log.Warnf("failed to search memories: %v", err)
		return ""
	}

	if searchResult == nil || len(searchResult.Memories) == 0 {
		log.Debug("no relevant memories found")
		return ""
	}

	// 格式化记忆内容
	var buf strings.Builder
	for i, memory := range searchResult.Memories {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}
		buf.WriteString(fmt.Sprintf("### Memory %d\n\n", i+1))
		buf.WriteString(memory.Content)
		buf.WriteString("\n")
	}

	log.Infof("found %d relevant memories for reflection", len(searchResult.Memories))
	return buf.String()
}

// cacheReflection 缓存反思结果供 prompt 使用（保留最近 3 条）
func (r *ReActLoop) cacheReflection(reflection *ActionReflection) {
	var reflections []*ActionReflection
	historyRaw := r.GetVariable("self_reflections")
	if !utils.IsNil(historyRaw) {
		if history, ok := historyRaw.([]*ActionReflection); ok {
			reflections = history
		}
	}

	// 只保留最近 3 条用于 prompt 上下文
	reflections = append(reflections, reflection)
	if len(reflections) > 3 {
		reflections = reflections[len(reflections)-3:]
	}

	r.Set("self_reflections", reflections)
	log.Debugf("cached reflection for action[%s], cache size: %d", reflection.ActionType, len(reflections))
}

// addReflectionToTimeline 将反思添加到 Timeline（使用强语气）
// Timeline 的 diff 会自动触发记忆系统生成记忆，无需手动保存
func (r *ReActLoop) addReflectionToTimeline(reflection *ActionReflection) {
	invoker := r.GetInvoker()
	if invoker == nil {
		log.Warn("invoker not available, skip adding reflection to timeline")
		return
	}

	// 构建强语气的 Timeline 消息
	var timelineMsg strings.Builder

	if reflection.Success {
		timelineMsg.WriteString(fmt.Sprintf("✓ [REFLECTION] Action '%s' EXECUTED SUCCESSFULLY",
			reflection.ActionType))
	} else {
		timelineMsg.WriteString(fmt.Sprintf("✗ [CRITICAL REFLECTION] Action '%s' FAILED",
			reflection.ActionType))
		if reflection.ErrorMessage != "" {
			timelineMsg.WriteString(fmt.Sprintf(" - %s", reflection.ErrorMessage))
		}
	}

	timelineMsg.WriteString(fmt.Sprintf(" (iteration %d, %v, level: %s)\n\n",
		reflection.IterationNum, reflection.ExecutionTime, reflection.ReflectionLevel))

	// 添加建议
	if len(reflection.Suggestions) > 0 {
		timelineMsg.WriteString("MANDATORY RECOMMENDATIONS FOR FUTURE ACTIONS:\n")
		for i, suggestion := range reflection.Suggestions {
			timelineMsg.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		timelineMsg.WriteString("\n")
	}

	// 根据反思级别使用不同的事件类型
	eventType := "reflection"
	if !reflection.Success {
		eventType = "critical-reflection"
	}

	// 添加到 Timeline
	invoker.AddToTimeline(eventType, timelineMsg.String())

	log.Infof("reflection added to timeline for action[%s], event_type[%s]",
		reflection.ActionType, eventType)
}
