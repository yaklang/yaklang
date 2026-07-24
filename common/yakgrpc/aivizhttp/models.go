package aivizhttp

import "time"

// SessionSummary session 列表项
type SessionSummary struct {
	SessionID  string    `json:"session_id"`
	Title      string    `json:"title,omitempty"`
	Source     string    `json:"source,omitempty"`
	IsAlive    bool      `json:"is_alive"`
	EventCount int64     `json:"event_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

// SessionListResponse session 列表响应
type SessionListResponse struct {
	Sessions []SessionSummary `json:"sessions"`
	Total    int              `json:"total"`
}

// EventItem 事件项 (精简版, 供前端列表展示)
type EventItem struct {
	ID              uint   `json:"id"`
	Type            string `json:"type"`
	NodeId          string `json:"node_id,omitempty"`
	Timestamp       int64  `json:"timestamp"`
	TaskIndex       string `json:"task_index,omitempty"`
	TaskId          string `json:"task_id,omitempty"`
	TaskUUID        string `json:"task_uuid,omitempty"`
	TaskName        string `json:"task_name,omitempty"`
	CoordinatorId   string `json:"coordinator_id,omitempty"`
	IsSubAgent      bool   `json:"is_sub_agent,omitempty"`
	ParentTaskID    string `json:"parent_task_id,omitempty"`
	CallToolID      string `json:"call_tool_id,omitempty"`
	IsStream        bool   `json:"is_stream"`
	IsReason        bool   `json:"is_reason"`
	IsJson          bool   `json:"is_json"`
	Content         string `json:"content,omitempty"`
	StreamDelta     string `json:"stream_delta,omitempty"`
	ContentType     string `json:"content_type,omitempty"`
	AIService       string `json:"ai_service,omitempty"`
	AIModelName     string `json:"ai_model_name,omitempty"`
	EventUUID       string `json:"event_uuid,omitempty"`
	RecoveryIndexID string `json:"recovery_index_id,omitempty"`
}

// EventListResponse 事件列表响应
type EventListResponse struct {
	Events  []EventItem `json:"events"`
	Total   int64       `json:"total"`
	Page    int64       `json:"page"`
	Limit   int64       `json:"limit"`
	HasMore bool        `json:"has_more"`
}

// ToolCallSummary 工具调用汇总 (start→done/error 配对)
type ToolCallSummary struct {
	CallToolID string `json:"call_tool_id"`
	ToolName   string `json:"tool_name"`
	Status     string `json:"status"` // running / done / error / cancelled
	Reason     string `json:"reason,omitempty"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Params     string `json:"params,omitempty"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

// ToolCallListResponse 工具调用列表响应
type ToolCallListResponse struct {
	ToolCalls []ToolCallSummary `json:"tool_calls"`
	Total     int               `json:"total"`
}

// StatsResponse 统计信息
type StatsResponse struct {
	SessionID       string       `json:"session_id"`
	TotalEvents     int64        `json:"total_events"`
	ToolCallCount   int64        `json:"tool_call_count"`
	StreamCount     int64        `json:"stream_count"`
	InputTokens     int64        `json:"input_tokens"`
	OutputTokens    int64        `json:"output_tokens"`
	TotalTokens     int64        `json:"total_tokens"`
	AICallCount     int64        `json:"ai_call_count"`
	FirstByteCostMs int64        `json:"first_byte_cost_ms_avg"`
	TotalCostMs     int64        `json:"total_cost_ms_avg"`
	ContextPressure float64      `json:"context_pressure_max"`
	ModelBreakdown  []ModelUsage `json:"model_breakdown,omitempty"`
}

// ModelUsage 按模型统计用量
type ModelUsage struct {
	Model        string `json:"model"`
	CallCount    int64  `json:"call_count"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// TimelineTask 时间线中的任务分组
type TimelineTask struct {
	TaskIndex  string      `json:"task_index"`
	TaskId     string      `json:"task_id,omitempty"`
	Events     []EventItem `json:"events"`
	EventCount int         `json:"event_count"`
}

// TimelineResponse 时间线响应
type TimelineResponse struct {
	Tasks []TimelineTask `json:"tasks"`
	Total int            `json:"total"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status       string `json:"status"`
	DBAvailable  bool   `json:"db_available"`
	SessionCount int64  `json:"session_count"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// TrajectoryNode 是执行轨迹树中的一个节点（session / loop / phase / subagent / iteration）。
type TrajectoryNode struct {
	// NodeID 是节点在树中的稳定标识，通常是 task_id 或 session_id。
	NodeID string `json:"node_id"`
	// Kind 是节点类型：session, loop, phase, subagent, iteration。
	Kind string `json:"kind"`
	// Label 是人类可读的名称。
	Label string `json:"label"`
	// LoopName 是关联的 ReAct loop 名称（如 dir_explore）。
	LoopName string `json:"loop_name,omitempty"`
	// EnterLine / ExitLine 是节点在投影时间线中的行号范围。
	EnterLine int64 `json:"enter_line"`
	ExitLine  int64 `json:"exit_line,omitempty"`
	// Summary 是节点的一句话摘要。
	Summary string `json:"summary,omitempty"`
	// Children 是子节点（嵌套的 phase / subagent / iteration）。
	Children []*TrajectoryNode `json:"children,omitempty"`
	// BlockLineNos 是 Context tab 中属于该节点的块行号，用于点击跳转。
	BlockLineNos []int64 `json:"block_line_nos,omitempty"`
}

// TrajectoryResponse 是 GET /sessions/:id/trajectory 的响应 DTO。
type TrajectoryResponse struct {
	SessionID string          `json:"session_id"`
	Root      *TrajectoryNode `json:"root"`
}
