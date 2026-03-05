package aihttp

import (
	"encoding/json"
	"io"
	"net/http"
)

func (gw *AIAgentHTTPGateway) handleGetSetting(w http.ResponseWriter, r *http.Request) {
	setting, err := gw.getSettingByKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, setting)
}

func (gw *AIAgentHTTPGateway) handleUpdateSetting(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	current, err := gw.getSettingByKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
		return
	}
	req, err := mergeSettingPayloadPatch(current, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	resp, err := gw.saveSettingByKey(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save setting failed: "+err.Error())
		return
	}
	gw.applySettingToRuntime(resp)
	writeJSON(w, http.StatusOK, resp)
}

func mergeSettingPayloadPatch(base aiAgentChatSettingPayload, patchRaw []byte) (aiAgentChatSettingPayload, error) {
	var patchMap map[string]any
	if err := json.Unmarshal(patchRaw, &patchMap); err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	normalizeSettingPatchKeys(patchMap)

	baseRaw, err := json.Marshal(base)
	if err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	merged := make(map[string]any)
	if err := json.Unmarshal(baseRaw, &merged); err != nil {
		return aiAgentChatSettingPayload{}, err
	}

	for key, value := range patchMap {
		merged[key] = value
	}

	mergedRaw, err := json.Marshal(merged)
	if err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	var req aiAgentChatSettingPayload
	if err := json.Unmarshal(mergedRaw, &req); err != nil {
		return aiAgentChatSettingPayload{}, err
	}
	return req, nil
}

func normalizeSettingPatchKeys(values map[string]any) {
	if values == nil {
		return
	}
	aliases := map[string]string{
		"use_default_ai_config":              "UseDefaultAIConfig",
		"ai_service":                         "AIService",
		"ai_model_name":                      "AIModelName",
		"forge_name":                         "ForgeName",
		"review_policy":                      "ReviewPolicy",
		"max_iteration":                      "ReActMaxIteration",
		"react_max_iteration":                "ReActMaxIteration",
		"disable_tool_use":                   "DisableToolUse",
		"enable_system_file_system_operator": "EnableSystemFileSystemOperator",
		"disallow_require_for_user_prompt":   "DisallowRequireForUserPrompt",
		"ai_review_risk_control_score":       "AIReviewRiskControlScore",
		"ai_call_auto_retry":                 "AICallAutoRetry",
		"ai_transaction_retry":               "AITransactionRetry",
		"enable_ai_search_tool":              "EnableAISearchTool",
		"enable_ai_search_internet":          "EnableAISearchInternet",
		"enable_qwen_no_think_mode":          "EnableQwenNoThinkMode",
		"allow_plan_user_interact":           "AllowPlanUserInteract",
		"plan_user_interact_max_count":       "PlanUserInteractMaxCount",
		"timeline_item_limit":                "TimelineItemLimit",
		"timeline_content_size_limit":        "TimelineContentSizeLimit",
		"user_interact_limit":                "UserInteractLimit",
		"timeline_session_id":                "TimelineSessionID",
		"selected_provider_id":               "SelectedProviderID",
		"selected_model_name":                "SelectedModelName",
		"selected_model_tier":                "SelectedModelTier",
	}
	for oldKey, newKey := range aliases {
		value, ok := values[oldKey]
		if !ok {
			continue
		}
		if _, exists := values[newKey]; !exists {
			values[newKey] = value
		}
	}
}
