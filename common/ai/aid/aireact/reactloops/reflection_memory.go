package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

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
		tail := reflections[len(reflections)-3:]
		hasSpin := false
		for _, item := range tail {
			if item != nil && item.IsSpinning {
				hasSpin = true
				break
			}
		}
		if hasSpin {
			reflections = tail
		} else {
			var recentSpin *ActionReflection
			for i := len(reflections) - 4; i >= 0; i-- {
				if reflections[i] != nil && reflections[i].IsSpinning {
					recentSpin = reflections[i]
					break
				}
			}
			if recentSpin != nil {
				reflections = []*ActionReflection{recentSpin, tail[1], tail[2]}
			} else {
				reflections = tail
			}
		}
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
