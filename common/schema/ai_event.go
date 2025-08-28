package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type EventType string

const (
	EVENT_TYPE_STREAM     EventType = "stream"
	EVENT_TYPE_STRUCTURED EventType = "structured"

	// Token 开销情况
	EVENT_TYPE_CONSUMPTION EventType = "consumption" // token consumption include `{"input_"}`

	// 探活
	EVENT_TYPE_PONG EventType = "pong" // ping response ping-pong is a check for alive item

	// 压力值
	EVENT_TYPE_PRESSURE EventType = "pressure" // pressure for ai context percent

	EVENT_TYPE_AI_FIRST_BYTE_COST_MS EventType = "ai_first_byte_cost_ms" // first byte cost
	EVENT_TYPE_AI_TOTAL_COST_MS      EventType = "ai_total_cost_ms"      // first byte cost

	// AI 请求用户交互
	EVENT_TYPE_REQUIRE_USER_INTERACTIVE = "require_user_interactive"

	// risk control prompt is the prompt for risk control
	// contains score, reason, and other information to help uesr interactivation
	EVENT_TYPE_RISK_CONTROL_PROMPT = "risk_control_prompt"

	EVENT_TOOL_CALL_START       = "tool_call_start"       // tool call start event, used to emit the tool call start information
	EVENT_TOOL_CALL_STATUS      = "tool_call_status"      // tool call status event, used to emit the tool call status information
	EVENT_TOOL_CALL_USER_CANCEL = "tool_call_user_cancel" // tool call user cancel event, used to emit the tool call user cancel information
	EVENT_TOOL_CALL_DONE        = "tool_call_done"        // tool call end event, used to emit the tool call end information
	EVENT_TOOL_CALL_ERROR       = "tool_call_error"       // tool call error event, used to emit the tool call error information
	EVENT_TOOL_CALL_SUMMARY     = "tool_call_summary"     // tool call summary event, used to emit the tool call summary information

	EVENT_TYPE_START_PLAN_AND_EXECUTION EventType = "start_plan_and_execution"
	EVENT_TYPE_END_PLAN_AND_EXECUTION   EventType = "end_plan_and_execution"
	EVENT_TYPE_PLAN                     EventType = "plan"
	EVENT_TYPE_SELECT                   EventType = "select"
	EVENT_TYPE_PERMISSION_REQUIRE       EventType = "permission_require"
	EVENT_TYPE_TASK_REVIEW_REQUIRE      EventType = "task_review_require"
	EVENT_TYPE_PLAN_REVIEW_REQUIRE      EventType = "plan_review_require"
	EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE  EventType = "tool_use_review_require"

	EVENT_PLAN_TASK_ANALYSIS EventType = "plan_task_analysis" // plan task analysis event, used to emit the plan task analysis information

	EVENT_TYPE_TOOL_CALL_WATCHER EventType = "tool_call_watcher" // tool call watcher event, used to emit the tool call watcher information. user can cancel this tool call

	EVENT_TYPE_REVIEW_RELEASE EventType = "review_release"

	EVENT_TYPE_INPUT EventType = "input"

	EVENT_TYPE_AID_CONFIG = "aid_config" // aid config event, used to emit the current config information

	EVENT_TYPE_YAKIT_EXEC_RESULT = "yak_exec_result" // yakit exec result event, used to emit the yakit exec result information

	// AI 推理过程通用事件类型
	EVENT_TYPE_THOUGHT     EventType = "thought"     // AI 思考过程（适用于 ReAct、CoT 等推理模式）
	EVENT_TYPE_ACTION      EventType = "action"      // AI 行动执行（工具调用、函数执行等）
	EVENT_TYPE_OBSERVATION EventType = "observation" // AI 观察结果（工具执行结果、环境反馈等）
	EVENT_TYPE_ITERATION   EventType = "iteration"   // AI 迭代信息（循环推理、多轮对话等）
	EVENT_TYPE_RESULT      EventType = "result"      // AI 最终结果（推理结论、任务完成等）
)

type AiOutputEvent struct {
	gorm.Model
	CoordinatorId string
	Type          EventType

	NodeId      string
	IsSystem    bool
	IsStream    bool
	IsReason    bool
	IsSync      bool
	StreamDelta []byte
	IsJson      bool
	Content     []byte

	SyncID    string
	EventUUID string

	Timestamp int64

	// task index
	TaskIndex string
	// disable markdown render
	DisableMarkdown bool

	Processes []*AiProcess `gorm:"many2many:ai_processes_and_events;"`
}

func (e *AiOutputEvent) ShouldSave() bool {
	return !e.IsSync && e.Type != EVENT_TYPE_CONSUMPTION
}

func (e *AiOutputEvent) IsInteractive() bool {
	if e.IsJson {
		var i map[string]any
		if err := json.Unmarshal(e.Content, &i); err == nil {
			// 检查事件类型是否为需要交互的类型
			switch e.Type {
			case EVENT_TYPE_PLAN_REVIEW_REQUIRE,
				EVENT_TYPE_TASK_REVIEW_REQUIRE,
				EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
				EVENT_TYPE_PERMISSION_REQUIRE,
				EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
				EVENT_TYPE_TOOL_CALL_WATCHER,
				EVENT_TYPE_REVIEW_RELEASE:
				return true
			}
		}
	}
	return false
}

func (e *AiOutputEvent) GetInteractiveId() string {
	if e.IsJson {
		var i map[string]any
		if err := json.Unmarshal(e.Content, &i); err == nil {
			// 检查事件类型是否为需要交互的类型
			switch e.Type {
			case EVENT_TYPE_PLAN_REVIEW_REQUIRE,
				EVENT_TYPE_TASK_REVIEW_REQUIRE,
				EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
				EVENT_TYPE_PERMISSION_REQUIRE,
				EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
				EVENT_TYPE_TOOL_CALL_WATCHER,
				EVENT_TYPE_REVIEW_RELEASE:
				if id, ok := i["id"].(string); ok {
					return id
				}
			}
		}
	}
	return ""
}

func (e *AiOutputEvent) String() string {
	var parts []string

	if e.CoordinatorId != "" {
		parts = append(parts, fmt.Sprintf("id: %s", utils.ShrinkString(e.CoordinatorId, 4096)))
	}
	if e.Type != "" {
		// 为不同事件类型添加特殊展示
		typeStr := string(e.Type)
		switch e.Type {
		case EVENT_TYPE_THOUGHT:
			typeStr = "[thought]"
		case EVENT_TYPE_ACTION:
			typeStr = "[action]"
		case EVENT_TYPE_OBSERVATION:
			typeStr = "[observation]"
		case EVENT_TYPE_ITERATION:
			typeStr = "[iteration]"
		case EVENT_TYPE_RESULT:
			typeStr = "[result]"
		case EVENT_TYPE_STREAM:
			typeStr = "[stream]"
		case EVENT_TYPE_STRUCTURED:
			typeStr = "[structured]"
		}
		parts = append(parts, fmt.Sprintf("[type:%s]", typeStr))
	}
	if e.NodeId != "" {
		parts = append(parts, fmt.Sprintf("[node:%v]", e.NodeId))
	}
	if e.TaskIndex != "" {
		parts = append(parts, fmt.Sprintf("[task:%s]", e.TaskIndex))
	}
	if e.IsSystem {
		parts = append(parts, "system:true")
	}
	if e.IsStream {
		parts = append(parts, "stream:true")
	}
	if e.IsReason {
		parts = append(parts, "reason:true")
	}
	if len(e.StreamDelta) > 0 {
		parts = append(parts, fmt.Sprintf("delta:%v", string(e.StreamDelta)))
	}
	if e.IsJson {
		parts = append(parts, "json:true")
	}
	if len(e.Content) > 0 {
		contentStr := utils.ShrinkString(string(e.Content), 4096)
		// 对特定事件类型的内容进行解析和美化
		if e.IsJson && (e.Type == EVENT_TYPE_THOUGHT || e.Type == EVENT_TYPE_ACTION ||
			e.Type == EVENT_TYPE_OBSERVATION || e.Type == EVENT_TYPE_ITERATION || e.Type == EVENT_TYPE_RESULT) {
			var data map[string]interface{}
			if err := json.Unmarshal(e.Content, &data); err == nil {
				switch e.Type {
				case EVENT_TYPE_THOUGHT:
					if thought, ok := data["thought"].(string); ok {
						contentStr = fmt.Sprintf("thought: %s", utils.ShrinkString(thought, 4096))
					}
				case EVENT_TYPE_ACTION:
					if action, ok := data["action"].(string); ok {
						actionType := data["action_type"]
						contentStr = fmt.Sprintf("action[%v]: %s", actionType, utils.ShrinkString(action, 4096))
					}
				case EVENT_TYPE_OBSERVATION:
					if obs, ok := data["observation"].(string); ok {
						source := data["source"]
						contentStr = fmt.Sprintf("observe[%v]: %s", source, utils.ShrinkString(obs, 4096))
					}
				case EVENT_TYPE_ITERATION:
					if current, ok := data["current"].(float64); ok {
						if max, ok := data["max"].(float64); ok {
							contentStr = fmt.Sprintf("iter: %v/%v", int(current), int(max))
						}
					}
				case EVENT_TYPE_RESULT:
					if success, ok := data["success"].(bool); ok {
						status := "[failed]"
						if success {
							status = "[success]"
						}
						contentStr = fmt.Sprintf("result: %s", status)
					}
				}
			}
		} else {
			contentStr = fmt.Sprintf("data:%s", contentStr)
		}
		parts = append(parts, contentStr)
	}

	return fmt.Sprintf("event: %s", strings.Join(parts, ", "))
}

func (e *AiOutputEvent) ToGRPC() *ypb.AIOutputEvent {
	return &ypb.AIOutputEvent{
		CoordinatorId:   e.CoordinatorId,
		Type:            string(e.Type),
		NodeId:          utils.EscapeInvalidUTF8Byte([]byte(e.NodeId)),
		IsSystem:        e.IsSystem,
		IsStream:        e.IsStream,
		IsReason:        e.IsReason,
		IsSync:          e.IsSync,
		StreamDelta:     e.StreamDelta,
		IsJson:          e.IsJson,
		Content:         e.Content,
		Timestamp:       e.Timestamp,
		TaskIndex:       e.TaskIndex,
		DisableMarkdown: e.DisableMarkdown,
		SyncID:          e.SyncID,
		EventUUID:       e.EventUUID,
	}
}
