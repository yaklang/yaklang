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

type CreateSessionRequest struct {
	RunID string `json:"run_id,omitempty"`
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
