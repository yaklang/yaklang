package loop_http_flow_analyze

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	recentActionsKey        = "recent_action_records"
	lastActionTypeKey       = "last_action_type"
	directlyAnsweredKey     = "directly_answered"
	finalAnswerDeliveredKey = "final_answer_delivered"
	recentActionsLimit      = 12
	promptActionLimit       = 8
)

type actionRecord struct {
	ActionName     string    `json:"action_name"`
	ParamSummary   string    `json:"param_summary,omitempty"`
	ResultSummary  string    `json:"result_summary,omitempty"`
	MatcherDetails string    `json:"matcher_details,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}

func getRecentActions(loop *reactloops.ReActLoop) []actionRecord {
	if loop == nil {
		return nil
	}
	raw := loop.GetVariable(recentActionsKey)
	if raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []actionRecord:
		result := make([]actionRecord, len(values))
		copy(result, values)
		return result
	case []*actionRecord:
		result := make([]actionRecord, 0, len(values))
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

func setRecentActions(loop *reactloops.ReActLoop, records []actionRecord) {
	if loop == nil {
		return
	}
	cloned := make([]actionRecord, len(records))
	copy(cloned, records)
	loop.Set(recentActionsKey, cloned)
}

func appendActionRecord(loop *reactloops.ReActLoop, record actionRecord) actionRecord {
	if loop == nil {
		return record
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now()
	}
	if strings.TrimSpace(record.Summary) == "" {
		record.Summary = buildActionRecordSummary(record)
	}
	records := getRecentActions(loop)
	records = append(records, record)
	if len(records) > recentActionsLimit {
		records = records[len(records)-recentActionsLimit:]
	}
	setRecentActions(loop, records)
	markLastAction(loop, record.ActionName)
	return record
}

func recordAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary, matcherDetails string) actionRecord {
	record := actionRecord{
		ActionName:     actionName,
		ParamSummary:   strings.TrimSpace(paramSummary),
		ResultSummary:  strings.TrimSpace(resultSummary),
		MatcherDetails: strings.TrimSpace(matcherDetails),
	}
	return appendActionRecord(loop, record)
}

func recordMetaAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary string) actionRecord {
	record := actionRecord{
		ActionName:    actionName,
		ParamSummary:  strings.TrimSpace(paramSummary),
		ResultSummary: strings.TrimSpace(resultSummary),
	}
	return appendActionRecord(loop, record)
}

func buildActionRecordSummary(record actionRecord) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("[%s]", record.ActionName))
	if record.ParamSummary != "" {
		out.WriteString(" ")
		out.WriteString(record.ParamSummary)
	}
	if record.MatcherDetails != "" {
		out.WriteString(" | matcher: ")
		out.WriteString(record.MatcherDetails)
	}
	if record.ResultSummary != "" {
		out.WriteString(" -> ")
		out.WriteString(record.ResultSummary)
	}
	return strings.TrimSpace(out.String())
}

func buildRecentActionsPrompt(loop *reactloops.ReActLoop) string {
	records := getRecentActions(loop)
	if len(records) == 0 {
		return ""
	}
	if len(records) > promptActionLimit {
		records = records[len(records)-promptActionLimit:]
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

func markLastAction(loop *reactloops.ReActLoop, actionName string) {
	if loop == nil {
		return
	}
	loop.Set(lastActionTypeKey, strings.TrimSpace(actionName))
	if actionName != "directly_answer" {
		loop.Set(directlyAnsweredKey, false)
	}
}

func getLastAction(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	return strings.TrimSpace(loop.Get(lastActionTypeKey))
}

func markDirectlyAnswered(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(directlyAnsweredKey, true)
	markLastAction(loop, "directly_answer")
}

func hasDirectlyAnswered(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return utils.InterfaceToBoolean(loop.GetVariable(directlyAnsweredKey))
}

func markFinalAnswerDelivered(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(finalAnswerDeliveredKey, true)
	markLastAction(loop, "finalize_answer")
	loop.Set(directlyAnsweredKey, false)
}

func hasFinalAnswerDelivered(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return utils.InterfaceToBoolean(loop.GetVariable(finalAnswerDeliveredKey))
}
