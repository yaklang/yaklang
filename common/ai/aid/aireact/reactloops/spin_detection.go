package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// getTimelineContentForSpinDetection 获取用于 SPIN 检测的 Timeline 内容
// 被 reflection_prompt.go 的 buildReflectionPrompt 调用，为反思 prompt 提供
// 最近 2048 token 的 Timeline 上下文。
func (r *ReActLoop) getTimelineContentForSpinDetection() string {
	config := r.GetConfig()
	if config == nil {
		return ""
	}

	// 尝试通过类型断言获取 Timeline
	var timeline *aicommon.Timeline
	if cfg, ok := config.(interface{ GetTimeline() *aicommon.Timeline }); ok {
		timeline = cfg.GetTimeline()
	} else {
		return ""
	}

	if timeline == nil {
		return ""
	}

	return timeline.DumpRecentForPrompt(2048)
}

// IsInSameActionTypeSpin 检测是否连续 N 次执行了"相同 ActionType + 相同 ToolName"
// 的同质动作. 这是一个低成本的纯本地检测方法, 不调用 AI.
//
// 双维度判定:
//   - ActionType 必须一致 (例如都是 require_tool / directly_call_tool / 自定义 action 名)
//   - ToolName 必须一致 (从 action 参数里抽出的实际工具名; 若该 action 不是 tool 调用类,
//     ToolName 通常是空串, 此时退化为只比 ActionType — 仍保持兼容)
//
// 设计意图: 旧版只比 ActionType 会把"同样 require_tool 但调用了不同工具"误判成 SPIN,
// 大量误触发. 现在要求 tool 名也一致, 与"用户感知的同质操作"对齐. 同时把默认阈值
// 从 3 提到 8, 降低 SPIN 触发频率, 避免干扰正常的多步执行流.
//
// 关键词: IsInSameActionTypeSpin, SPIN 双维度判定, ActionType+ToolName,
//
//	降误触, 默认阈值 8
func (r *ReActLoop) IsInSameActionTypeSpin() bool {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	threshold := r.sameActionTypeSpinThreshold
	if threshold <= 0 {
		threshold = 8 // 默认阈值
	}

	historyLen := len(r.actionHistory)
	if historyLen < threshold {
		return false
	}

	lastActionType := r.actionHistory[historyLen-1].ActionType
	lastToolName := r.actionHistory[historyLen-1].ToolName
	// A normal soft TODO checkpoint records two consecutive finish actions:
	// the initial request and the confirmation after the checkpoint. They are
	// termination control signals, not repeated work, so they must never be
	// classified as an execution spin.
	if lastActionType == loopAction_Finish.ActionType {
		return false
	}
	for i := historyLen - threshold; i < historyLen; i++ {
		if r.actionHistory[i].ActionType != lastActionType {
			return false
		}
		if r.actionHistory[i].ToolName != lastToolName {
			return false
		}
	}

	log.Infof("detected same action+tool spin: %d consecutive actions of type %q tool %q",
		threshold, lastActionType, lastToolName)
	return true
}
