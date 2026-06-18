package aihttp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	aiAgentChatSettingKey = "ai-agent-chat-setting"
)

type aiAgentChatSettingPayload struct {
	EnableSystemFileSystemOperator bool    `json:"EnableSystemFileSystemOperator"`
	UseDefaultAIConfig             bool    `json:"UseDefaultAIConfig"`
	ForgeName                      string  `json:"ForgeName"`
	DisallowRequireForUserPrompt   bool    `json:"DisallowRequireForUserPrompt"`
	ReviewPolicy                   string  `json:"ReviewPolicy"`
	AIReviewRiskControlScore       float64 `json:"AIReviewRiskControlScore"`
	DisableToolUse                 bool    `json:"DisableToolUse"`
	AICallAutoRetry                int64   `json:"AICallAutoRetry"`
	AITransactionRetry             int64   `json:"AITransactionRetry"`
	EnableAISearchTool             bool    `json:"EnableAISearchTool"`
	EnableAISearchInternet         bool    `json:"EnableAISearchInternet"`
	EnableQwenNoThinkMode          bool    `json:"EnableQwenNoThinkMode"`
	AllowPlanUserInteract          bool    `json:"AllowPlanUserInteract"`
	PlanUserInteractMaxCount       int64   `json:"PlanUserInteractMaxCount"`
	AIService                      string  `json:"AIService"`
	AIModelName                    string  `json:"AIModelName"`
	ReActMaxIteration              int64   `json:"ReActMaxIteration"`
	TimelineItemLimit              int64   `json:"TimelineItemLimit"`
	TimelineContentSizeLimit       int64   `json:"TimelineContentSizeLimit"` // KB
	UserInteractLimit              int64   `json:"UserInteractLimit"`
	TimelineSessionID              string  `json:"TimelineSessionID"`

	SelectedProviderID int64  `json:"SelectedProviderID"`
	SelectedModelName  string `json:"SelectedModelName"`
	SelectedModelTier  string `json:"SelectedModelTier"`
}

type legacyAIHTTPSettingPayload struct {
	EnableSystemFileSystemOperator bool    `json:"enable_system_file_system_operator"`
	UseDefaultAIConfig             bool    `json:"use_default_ai_config"`
	ForgeName                      string  `json:"forge_name"`
	DisallowRequireForUserPrompt   bool    `json:"disallow_require_for_user_prompt"`
	ReviewPolicy                   string  `json:"review_policy"`
	AIReviewRiskControlScore       float64 `json:"ai_review_risk_control_score"`
	DisableToolUse                 bool    `json:"disable_tool_use"`
	AICallAutoRetry                int64   `json:"ai_call_auto_retry"`
	AITransactionRetry             int64   `json:"ai_transaction_retry"`
	EnableAISearchTool             bool    `json:"enable_ai_search_tool"`
	EnableAISearchInternet         bool    `json:"enable_ai_search_internet"`
	EnableQwenNoThinkMode          bool    `json:"enable_qwen_no_think_mode"`
	AllowPlanUserInteract          bool    `json:"allow_plan_user_interact"`
	PlanUserInteractMaxCount       int64   `json:"plan_user_interact_max_count"`
	AIService                      string  `json:"ai_service"`
	AIModelName                    string  `json:"ai_model_name"`
	MaxIteration                   int64   `json:"max_iteration"`
	ReActMaxIteration              int64   `json:"react_max_iteration"`
	TimelineItemLimit              int64   `json:"timeline_item_limit"`
	TimelineContentSizeLimit       int64   `json:"timeline_content_size_limit"`
	UserInteractLimit              int64   `json:"user_interact_limit"`
	TimelineSessionID              string  `json:"timeline_session_id"`

	SelectedProviderID int64  `json:"selected_provider_id"`
	SelectedModelName  string `json:"selected_model_name"`
	SelectedModelTier  string `json:"selected_model_tier"`
}

func defaultAIAgentChatSettingPayload() aiAgentChatSettingPayload {
	return aiAgentChatSettingPayload{
		EnableSystemFileSystemOperator: true,
		UseDefaultAIConfig:             true,
		ForgeName:                      "",
		DisallowRequireForUserPrompt:   true,
		ReviewPolicy:                   "manual",
		AIReviewRiskControlScore:       0.5,
		DisableToolUse:                 false,
		AICallAutoRetry:                3,
		AITransactionRetry:             5,
		EnableAISearchTool:             true,
		EnableAISearchInternet:         false,
		EnableQwenNoThinkMode:          false,
		AllowPlanUserInteract:          true,
		PlanUserInteractMaxCount:       3,
		AIService:                      "",
		AIModelName:                    "",
		ReActMaxIteration:              100,
		TimelineItemLimit:              100,
		TimelineContentSizeLimit:       20,
		UserInteractLimit:              0,
		TimelineSessionID:              "",
	}
}

func normalizeAIAgentSettingAliases(s *aiAgentChatSettingPayload) {
	if s == nil {
		return
	}
	s.AIService = strings.TrimSpace(s.AIService)
	s.AIModelName = strings.TrimSpace(s.AIModelName)
	s.ReviewPolicy = strings.TrimSpace(s.ReviewPolicy)
	s.SelectedModelName = strings.TrimSpace(s.SelectedModelName)
	s.SelectedModelTier = strings.TrimSpace(s.SelectedModelTier)

	if s.SelectedModelName == "" && s.AIModelName != "" {
		s.SelectedModelName = s.AIModelName
	}
	if s.AIModelName == "" && s.SelectedModelName != "" {
		s.AIModelName = s.SelectedModelName
	}
}

func applyAIAgentSettingDefaults(s aiAgentChatSettingPayload) aiAgentChatSettingPayload {
	defaults := defaultAIAgentChatSettingPayload()

	if strings.TrimSpace(s.ReviewPolicy) == "" {
		s.ReviewPolicy = defaults.ReviewPolicy
	}
	if s.ReActMaxIteration == 0 {
		s.ReActMaxIteration = defaults.ReActMaxIteration
	}
	if s.AIReviewRiskControlScore == 0 {
		s.AIReviewRiskControlScore = defaults.AIReviewRiskControlScore
	}
	if s.AICallAutoRetry == 0 {
		s.AICallAutoRetry = defaults.AICallAutoRetry
	}
	if s.AITransactionRetry == 0 {
		s.AITransactionRetry = defaults.AITransactionRetry
	}
	if s.PlanUserInteractMaxCount == 0 {
		s.PlanUserInteractMaxCount = defaults.PlanUserInteractMaxCount
	}
	if s.TimelineItemLimit == 0 {
		s.TimelineItemLimit = defaults.TimelineItemLimit
	}
	if s.TimelineContentSizeLimit == 0 {
		s.TimelineContentSizeLimit = defaults.TimelineContentSizeLimit
	}

	normalizeAIAgentSettingAliases(&s)
	return s
}

func convertLegacySettingPayload(legacy legacyAIHTTPSettingPayload) aiAgentChatSettingPayload {
	reactMaxIteration := legacy.ReActMaxIteration
	if reactMaxIteration == 0 && legacy.MaxIteration > 0 {
		reactMaxIteration = legacy.MaxIteration
	}
	return aiAgentChatSettingPayload{
		EnableSystemFileSystemOperator: legacy.EnableSystemFileSystemOperator,
		UseDefaultAIConfig:             legacy.UseDefaultAIConfig,
		ForgeName:                      legacy.ForgeName,
		DisallowRequireForUserPrompt:   legacy.DisallowRequireForUserPrompt,
		ReviewPolicy:                   legacy.ReviewPolicy,
		AIReviewRiskControlScore:       legacy.AIReviewRiskControlScore,
		DisableToolUse:                 legacy.DisableToolUse,
		AICallAutoRetry:                legacy.AICallAutoRetry,
		AITransactionRetry:             legacy.AITransactionRetry,
		EnableAISearchTool:             legacy.EnableAISearchTool,
		EnableAISearchInternet:         legacy.EnableAISearchInternet,
		EnableQwenNoThinkMode:          legacy.EnableQwenNoThinkMode,
		AllowPlanUserInteract:          legacy.AllowPlanUserInteract,
		PlanUserInteractMaxCount:       legacy.PlanUserInteractMaxCount,
		AIService:                      legacy.AIService,
		AIModelName:                    legacy.AIModelName,
		ReActMaxIteration:              reactMaxIteration,
		TimelineItemLimit:              legacy.TimelineItemLimit,
		TimelineContentSizeLimit:       legacy.TimelineContentSizeLimit,
		UserInteractLimit:              legacy.UserInteractLimit,
		TimelineSessionID:              legacy.TimelineSessionID,
		SelectedProviderID:             legacy.SelectedProviderID,
		SelectedModelName:              legacy.SelectedModelName,
		SelectedModelTier:              legacy.SelectedModelTier,
	}
}

func decodeSettingPayloadRaw(raw string) (aiAgentChatSettingPayload, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return aiAgentChatSettingPayload{}, false
	}

	var payload aiAgentChatSettingPayload
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if strings.Contains(raw, "\"use_default_ai_config\"") ||
			strings.Contains(raw, "\"review_policy\"") ||
			strings.Contains(raw, "\"react_max_iteration\"") {
			var legacy legacyAIHTTPSettingPayload
			if errLegacy := json.Unmarshal([]byte(raw), &legacy); errLegacy == nil {
				payload = convertLegacySettingPayload(legacy)
			}
		}
		payload = applyAIAgentSettingDefaults(payload)
		return payload, true
	}

	var legacy legacyAIHTTPSettingPayload
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil {
		payload = applyAIAgentSettingDefaults(convertLegacySettingPayload(legacy))
		return payload, true
	}
	return aiAgentChatSettingPayload{}, false
}

func (gw *AIAgentHTTPGateway) loadSettingPayloadFromDB(db *gorm.DB) (aiAgentChatSettingPayload, bool) {
	payload := defaultAIAgentChatSettingPayload()
	if db == nil {
		return payload, false
	}

	raw := strings.TrimSpace(yakit.GetKey(db, aiAgentChatSettingKey))
	if decoded, ok := decodeSettingPayloadRaw(raw); ok {
		return decoded, true
	}

	return payload, false
}

func (gw *AIAgentHTTPGateway) getSettingByKey() (aiAgentChatSettingPayload, error) {
	return gw.GetSettingFromDB()
}

func (gw *AIAgentHTTPGateway) GetSettingFromDB() (aiAgentChatSettingPayload, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return aiAgentChatSettingPayload{}, fmt.Errorf("profile database is unavailable")
	}
	payload, _ := gw.loadSettingPayloadFromDB(db)
	payload = applyAIAgentSettingDefaults(payload)
	return payload, nil
}

func (gw *AIAgentHTTPGateway) saveSettingByKey(s aiAgentChatSettingPayload) (aiAgentChatSettingPayload, error) {
	return gw.SaveSettingToDB(s)
}

func (gw *AIAgentHTTPGateway) SaveSettingToDB(s aiAgentChatSettingPayload) (aiAgentChatSettingPayload, error) {
	result := applyAIAgentSettingDefaults(s)

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return aiAgentChatSettingPayload{}, fmt.Errorf("profile database is unavailable")
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	if err := yakit.SetKey(db, aiAgentChatSettingKey, string(raw)); err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	return result, nil
}
