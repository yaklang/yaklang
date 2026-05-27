package loop_ssa_risk_overview

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	recentActionsKey            = "ssa_overview_recent_action_records"
	lastActionTypeKey           = "ssa_overview_last_action_type"
	directlyAnsweredKey         = "ssa_overview_directly_answered"
	finalAnswerDeliveredKey     = "ssa_overview_final_answer_delivered"
	recentActionsLimit          = 12
	promptActionLimit           = 8
)

type actionRecord struct {
	ActionName    string    `json:"action_name"`
	ParamSummary  string    `json:"param_summary,omitempty"`
	ResultSummary string    `json:"result_summary,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	RecordedAt    time.Time `json:"recorded_at"`
}

func getRecentActions(loop *reactloops.ReActLoop) []actionRecord {
	if loop == nil {
		return nil
	}
	raw := loop.GetVariable(recentActionsKey)
	switch values := raw.(type) {
	case []actionRecord:
		out := make([]actionRecord, len(values))
		copy(out, values)
		return out
	case []*actionRecord:
		out := make([]actionRecord, 0, len(values))
		for _, item := range values {
			if item != nil {
				out = append(out, *item)
			}
		}
		return out
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

func buildActionRecordSummary(record actionRecord) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("[%s]", record.ActionName))
	if record.ParamSummary != "" {
		out.WriteString(" ")
		out.WriteString(record.ParamSummary)
	}
	if record.ResultSummary != "" {
		out.WriteString(" -> ")
		out.WriteString(record.ResultSummary)
	}
	return strings.TrimSpace(out.String())
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

func recordAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary string) actionRecord {
	return appendActionRecord(loop, actionRecord{
		ActionName:    actionName,
		ParamSummary:  strings.TrimSpace(paramSummary),
		ResultSummary: strings.TrimSpace(resultSummary),
	})
}

func recordMetaAction(loop *reactloops.ReActLoop, actionName, paramSummary, resultSummary string) actionRecord {
	return appendActionRecord(loop, actionRecord{
		ActionName:    actionName,
		ParamSummary:  strings.TrimSpace(paramSummary),
		ResultSummary: strings.TrimSpace(resultSummary),
	})
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
	v := loop.GetVariable(directlyAnsweredKey)
	switch x := v.(type) {
	case bool:
		return x
	default:
		return strings.TrimSpace(loop.Get(directlyAnsweredKey)) == "true"
	}
}

func markFinalAnswerDelivered(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(finalAnswerDeliveredKey, true)
	markLastAction(loop, "finalize_answer")
	loop.Set(directlyAnsweredKey, false)
}

// resetOverviewLoopTaskState clears per-task loop vars when a new AIStatefulTask starts on a reused loop instance.
func resetOverviewLoopTaskState(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	setRecentActions(loop, nil)
	loop.Set("ssa_overview_last_query_filter_key", "")
	loop.Set(directlyAnsweredKey, false)
	loop.Set(finalAnswerDeliveredKey, false)
	loop.Set(lastActionTypeKey, "")
}

func hasFinalAnswerDelivered(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	v := loop.GetVariable(finalAnswerDeliveredKey)
	switch x := v.(type) {
	case bool:
		return x
	default:
		return strings.TrimSpace(loop.Get(finalAnswerDeliveredKey)) == "true"
	}
}
