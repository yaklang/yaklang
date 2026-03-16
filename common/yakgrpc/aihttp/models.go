package aihttp

import "time"

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusCancelled RunStatus = "cancelled"
	RunStatusFailed    RunStatus = "failed"
)

type AIParams struct {
	ForgeName                      string   `json:"forge_name,omitempty"`
	ReviewPolicy                   string   `json:"review_policy,omitempty"`
	AIService                      string   `json:"ai_service,omitempty"`
	AIModelName                    string   `json:"ai_model_name,omitempty"`
	MaxIteration                   int32    `json:"max_iteration,omitempty"` // legacy alias of ReActMaxIteration
	ReActMaxIteration              int64    `json:"react_max_iteration,omitempty"`
	DisableToolUse                 bool     `json:"disable_tool_use,omitempty"`
	UseDefaultAI                   bool     `json:"use_default_ai,omitempty"`
	AttachedFiles                  []string `json:"attached_files,omitempty"`
	EnableSystemFileSystemOperator bool     `json:"enable_system_file_system_operator,omitempty"`
	DisallowRequireForUserPrompt   bool     `json:"disallow_require_for_user_prompt,omitempty"`
	AIReviewRiskControlScore       float64  `json:"ai_review_risk_control_score,omitempty"`
	AICallAutoRetry                int64    `json:"ai_call_auto_retry,omitempty"`
	AITransactionRetry             int64    `json:"ai_transaction_retry,omitempty"`
	EnableAISearchTool             bool     `json:"enable_ai_search_tool,omitempty"`
	EnableAISearchInternet         bool     `json:"enable_ai_search_internet,omitempty"`
	EnableQwenNoThinkMode          bool     `json:"enable_qwen_no_think_mode,omitempty"`
	AllowPlanUserInteract          bool     `json:"allow_plan_user_interact,omitempty"`
	PlanUserInteractMaxCount       int64    `json:"plan_user_interact_max_count,omitempty"`
	TimelineItemLimit              int64    `json:"timeline_item_limit,omitempty"`
	TimelineContentSizeLimit       int64    `json:"timeline_content_size_limit,omitempty"` // KB in HTTP setting
	UserInteractLimit              int64    `json:"user_interact_limit,omitempty"`
	TimelineSessionID              string   `json:"timeline_session_id,omitempty"`
	DisableToolIntervalReview      bool     `json:"disable_tool_interval_review,omitempty"`
}

type CreateSessionRequest struct {
	RunID  string   `json:"run_id,omitempty"`
	Params AIParams `json:"params,omitempty"`
}

type CreateSessionResponse struct {
	RunID  string    `json:"run_id"`
	Status RunStatus `json:"status"`
}

type SessionItem struct {
	RunID     string    `json:"run_id"`
	Title     string    `json:"title,omitempty"`
	Status    RunStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	IsAlive   bool      `json:"is_alive"`
}

type SessionListResponse struct {
	Sessions []SessionItem `json:"sessions"`
}

type UpdateSessionTitleRequest struct {
	Title string `json:"title"`
}

type SettingAIProvider struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`
	Domain     string `json:"domain,omitempty"`
	Disabled   bool   `json:"disabled,omitempty"`
	HasAPIKey  bool   `json:"has_api_key,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type SettingAIModel struct {
	ID           string            `json:"id"`
	Tier         string            `json:"tier"` // intelligent/lightweight/vision
	ModelName    string            `json:"model_name,omitempty"`
	ProviderID   int64             `json:"provider_id,omitempty"`
	ProviderType string            `json:"provider_type,omitempty"`
	ExtraParams  map[string]string `json:"extra_params,omitempty"`
}

type RuntimeModelOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	AIService   string `json:"ai_service,omitempty"`
	AIModelName string `json:"ai_model_name,omitempty"`
	ProviderID  int64  `json:"provider_id,omitempty"`
	Tier        string `json:"tier,omitempty"`
}

type RuntimeOptionResponse struct {
	Models         []RuntimeModelOption `json:"models"`
	ReviewPolicies []string             `json:"review_policies"`
	FocusModes     []string             `json:"focus_modes"`
	Providers      []SettingAIProvider  `json:"providers,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}
