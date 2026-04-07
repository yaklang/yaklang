package loop_http_fuzztest

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	loopHTTPFuzzRecentActionsKey        = "recent_action_records"
	loopHTTPFuzzTestedPayloadsKey       = "tested_payloads_by_action"
	loopHTTPFuzzLastActionTypeKey       = "last_action_type"
	loopHTTPFuzzDirectlyAnsweredKey     = "directly_answered"
	loopHTTPFuzzFinalAnswerDeliveredKey = "final_answer_delivered"
	loopHTTPFuzzRecentActionsLimit      = 12
	loopHTTPFuzzPromptActionLimit       = 6
	loopHTTPFuzzPromptPayloadLimit      = 6
)

type loopHTTPFuzzActionRecord struct {
	ActionName             string    `json:"action_name"`
	ParamSummary           string    `json:"param_summary,omitempty"`
	Payloads               []string  `json:"payloads,omitempty"`
	NewPayloads            []string  `json:"new_payloads,omitempty"`
	DuplicatePayloads      []string  `json:"duplicate_payloads,omitempty"`
	ResultSummary          string    `json:"result_summary,omitempty"`
	VerificationSummary    string    `json:"verification_summary,omitempty"`
	RepresentativeHTTPFlow string    `json:"representative_httpflow,omitempty"`
	Summary                string    `json:"summary,omitempty"`
	RecordedAt             time.Time `json:"recorded_at"`
}

func clearLoopHTTPFuzzActionTracking(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(loopHTTPFuzzRecentActionsKey, []loopHTTPFuzzActionRecord{})
	loop.Set(loopHTTPFuzzTestedPayloadsKey, map[string][]string{})
	loop.Set(loopHTTPFuzzLastActionTypeKey, "")
	loop.Set(loopHTTPFuzzDirectlyAnsweredKey, false)
	loop.Set(loopHTTPFuzzFinalAnswerDeliveredKey, false)
}

func getLoopHTTPFuzzRecentActions(loop *reactloops.ReActLoop) []loopHTTPFuzzActionRecord {
	if loop == nil {
		return nil
	}
	raw := loop.GetVariable(loopHTTPFuzzRecentActionsKey)
	if raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []loopHTTPFuzzActionRecord:
		result := make([]loopHTTPFuzzActionRecord, len(values))
		copy(result, values)
		return result
	case []*loopHTTPFuzzActionRecord:
		result := make([]loopHTTPFuzzActionRecord, 0, len(values))
		for _, item := range values {
			if item != nil {
				result = append(result, *item)
			}
		}
		return result
	default:
		return nil
	}
}

func setLoopHTTPFuzzRecentActions(loop *reactloops.ReActLoop, records []loopHTTPFuzzActionRecord) {
	if loop == nil {
		return
	}
	cloned := make([]loopHTTPFuzzActionRecord, len(records))
	copy(cloned, records)
	loop.Set(loopHTTPFuzzRecentActionsKey, cloned)
}

func appendLoopHTTPFuzzActionRecord(loop *reactloops.ReActLoop, record loopHTTPFuzzActionRecord) loopHTTPFuzzActionRecord {
	if loop == nil {
		return record
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now()
	}
	if strings.TrimSpace(record.Summary) == "" {
		record.Summary = buildLoopHTTPFuzzActionRecordSummary(record)
	}
	records := getLoopHTTPFuzzRecentActions(loop)
	records = append(records, record)
	if len(records) > loopHTTPFuzzRecentActionsLimit {
		records = records[len(records)-loopHTTPFuzzRecentActionsLimit:]
	}
	setLoopHTTPFuzzRecentActions(loop, records)
	markLoopHTTPFuzzLastAction(loop, record.ActionName)
	return record
}

func getLoopHTTPFuzzTestedPayloads(loop *reactloops.ReActLoop) map[string][]string {
	if loop == nil {
		return map[string][]string{}
	}
	raw := loop.GetVariable(loopHTTPFuzzTestedPayloadsKey)
	if raw == nil {
		return map[string][]string{}
	}
	switch values := raw.(type) {
	case map[string][]string:
		return cloneStringSliceMap(values)
	case map[string]any:
		result := make(map[string][]string, len(values))
		for key, item := range values {
			result[key] = utils.InterfaceToStringSlice(item)
		}
		return result
	default:
		return map[string][]string{}
	}
}

func setLoopHTTPFuzzTestedPayloads(loop *reactloops.ReActLoop, payloads map[string][]string) {
	if loop == nil {
		return
	}
	loop.Set(loopHTTPFuzzTestedPayloadsKey, cloneStringSliceMap(payloads))
}

func mergeLoopHTTPFuzzActionPayloadHistory(loop *reactloops.ReActLoop, actionName string, payloads []string) ([]string, []string, []string) {
	tracked := getLoopHTTPFuzzTestedPayloads(loop)
	existing := append([]string{}, tracked[actionName]...)
	seen := make(map[string]struct{}, len(existing))
	for _, payload := range existing {
		normalized := normalizeLoopHTTPFuzzPayload(payload)
		if normalized != "" {
			seen[normalized] = struct{}{}
		}
	}

	newPayloads := make([]string, 0)
	duplicatePayloads := make([]string, 0)
	for _, payload := range payloads {
		trimmed := strings.TrimSpace(payload)
		if trimmed == "" {
			continue
		}
		normalized := normalizeLoopHTTPFuzzPayload(trimmed)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			duplicatePayloads = appendUniqueString(duplicatePayloads, trimmed)
			continue
		}
		seen[normalized] = struct{}{}
		existing = append(existing, trimmed)
		newPayloads = append(newPayloads, trimmed)
	}
	tracked[actionName] = existing
	setLoopHTTPFuzzTestedPayloads(loop, tracked)
	return newPayloads, duplicatePayloads, existing
}

func normalizeLoopHTTPFuzzPayload(payload string) string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ""
	}
	return strings.Join(strings.Fields(payload), " ")
}

func recordLoopHTTPFuzzAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary, verificationSummary, representativeHTTPFlow string, payloads []string) loopHTTPFuzzActionRecord {
	newPayloads, duplicatePayloads, _ := mergeLoopHTTPFuzzActionPayloadHistory(loop, actionName, payloads)
	record := loopHTTPFuzzActionRecord{
		ActionName:             actionName,
		ParamSummary:           strings.TrimSpace(paramSummary),
		Payloads:               dedupeStringSlice(payloads),
		NewPayloads:            dedupeStringSlice(newPayloads),
		DuplicatePayloads:      dedupeStringSlice(duplicatePayloads),
		ResultSummary:          strings.TrimSpace(resultSummary),
		VerificationSummary:    strings.TrimSpace(verificationSummary),
		RepresentativeHTTPFlow: strings.TrimSpace(representativeHTTPFlow),
	}
	return appendLoopHTTPFuzzActionRecord(loop, record)
}

func recordLoopHTTPFuzzMetaAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary string) loopHTTPFuzzActionRecord {
	record := loopHTTPFuzzActionRecord{
		ActionName:    actionName,
		ParamSummary:  strings.TrimSpace(paramSummary),
		ResultSummary: strings.TrimSpace(resultSummary),
	}
	return appendLoopHTTPFuzzActionRecord(loop, record)
}

func buildLoopHTTPFuzzActionRecordSummary(record loopHTTPFuzzActionRecord) string {
	var out strings.Builder
	payloadCount := len(record.Payloads)
	if payloadCount > 0 {
		out.WriteString(fmt.Sprintf("针对 %s 生成 %d 个 payload。", record.ActionName, payloadCount))
		if len(record.NewPayloads) > 0 {
			out.WriteString(fmt.Sprintf(" 新增 %d 个。", len(record.NewPayloads)))
		}
		if len(record.DuplicatePayloads) > 0 {
			out.WriteString(fmt.Sprintf(" %d 个已经测试过。", len(record.DuplicatePayloads)))
		}
	} else {
		out.WriteString(fmt.Sprintf("执行了 %s。", record.ActionName))
	}
	if record.ParamSummary != "" {
		out.WriteString("\n参数：")
		out.WriteString(record.ParamSummary)
	}
	if len(record.Payloads) > 0 {
		out.WriteString("\nPayload：")
		out.WriteString(shrinkLoopHTTPFuzzList(record.Payloads, 6, 240))
	}
	if record.ResultSummary != "" {
		out.WriteString("\n响应：")
		out.WriteString(record.ResultSummary)
	}
	if record.VerificationSummary != "" {
		out.WriteString("\n验证：")
		out.WriteString(record.VerificationSummary)
	}
	if record.RepresentativeHTTPFlow != "" {
		out.WriteString("\n代表性 HTTPFlow：")
		out.WriteString(record.RepresentativeHTTPFlow)
	}
	return strings.TrimSpace(out.String())
}

func buildLoopHTTPFuzzActionFeedback(record loopHTTPFuzzActionRecord) string {
	if strings.TrimSpace(record.Summary) == "" {
		record.Summary = buildLoopHTTPFuzzActionRecordSummary(record)
	}
	return "=== Action Summary ===\n" + record.Summary
}

func buildLoopHTTPFuzzRecentActionsPrompt(loop *reactloops.ReActLoop) string {
	records := getLoopHTTPFuzzRecentActions(loop)
	if len(records) == 0 {
		return ""
	}
	if len(records) > loopHTTPFuzzPromptActionLimit {
		records = records[len(records)-loopHTTPFuzzPromptActionLimit:]
	}
	var out strings.Builder
	for idx, record := range records {
		out.WriteString(fmt.Sprintf("%d. %s", idx+1, strings.TrimSpace(record.Summary)))
		if idx < len(records)-1 {
			out.WriteString("\n\n")
		}
	}
	return out.String()
}

func buildLoopHTTPFuzzTestedPayloadPrompt(loop *reactloops.ReActLoop) string {
	tracked := getLoopHTTPFuzzTestedPayloads(loop)
	if len(tracked) == 0 {
		return ""
	}
	actions := make([]string, 0, len(tracked))
	for actionName, payloads := range tracked {
		if len(payloads) > 0 {
			actions = append(actions, actionName)
		}
	}
	if len(actions) == 0 {
		return ""
	}
	sort.Strings(actions)
	var out strings.Builder
	for index, actionName := range actions {
		payloads := tracked[actionName]
		out.WriteString(fmt.Sprintf("- %s: %s", actionName, shrinkLoopHTTPFuzzList(payloads, loopHTTPFuzzPromptPayloadLimit, 260)))
		if len(payloads) > loopHTTPFuzzPromptPayloadLimit {
			out.WriteString(fmt.Sprintf(" (共 %d 个)", len(payloads)))
		}
		if index < len(actions)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

func markLoopHTTPFuzzLastAction(loop *reactloops.ReActLoop, actionName string) {
	if loop == nil {
		return
	}
	loop.Set(loopHTTPFuzzLastActionTypeKey, strings.TrimSpace(actionName))
	if actionName != "directly_answer" {
		loop.Set(loopHTTPFuzzDirectlyAnsweredKey, false)
	}
}

func getLoopHTTPFuzzLastAction(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	return strings.TrimSpace(loop.Get(loopHTTPFuzzLastActionTypeKey))
}

func markLoopHTTPFuzzDirectlyAnswered(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(loopHTTPFuzzDirectlyAnsweredKey, true)
	markLoopHTTPFuzzLastAction(loop, "directly_answer")
}

func hasLoopHTTPFuzzDirectlyAnswered(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return utils.InterfaceToBoolean(loop.GetVariable(loopHTTPFuzzDirectlyAnsweredKey))
}

func markLoopHTTPFuzzFinalAnswerDelivered(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(loopHTTPFuzzFinalAnswerDeliveredKey, true)
	markLoopHTTPFuzzLastAction(loop, "finalize_answer")
	loop.Set(loopHTTPFuzzDirectlyAnsweredKey, false)
}

func hasLoopHTTPFuzzFinalAnswerDelivered(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return utils.InterfaceToBoolean(loop.GetVariable(loopHTTPFuzzFinalAnswerDeliveredKey))
}

func cloneStringSliceMap(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return map[string][]string{}
	}
	result := make(map[string][]string, len(input))
	for key, values := range input {
		copied := make([]string, len(values))
		copy(copied, values)
		result[key] = copied
	}
	return result
}

func dedupeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeLoopHTTPFuzzPayload(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if normalizeLoopHTTPFuzzPayload(existing) == normalizeLoopHTTPFuzzPayload(value) {
			return values
		}
	}
	return append(values, value)
}

func shrinkLoopHTTPFuzzList(values []string, maxItems int, maxChars int) string {
	values = dedupeStringSlice(values)
	if len(values) == 0 {
		return "(none)"
	}
	shown := values
	if len(shown) > maxItems {
		shown = shown[:maxItems]
	}
	text := strings.Join(shown, "; ")
	if len(values) > len(shown) {
		text += fmt.Sprintf(" ... 另有 %d 个", len(values)-len(shown))
	}
	return utils.ShrinkTextBlock(text, maxChars)
}
