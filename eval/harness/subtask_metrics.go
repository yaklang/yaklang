package harness

import (
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SubtaskMetrics records time/token/event statistics for a detected AI subtask.
type SubtaskMetrics struct {
	Name            string    `json:"name"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	EventCount      int       `json:"event_count"`
	ThoughtCount    int       `json:"thought_count"`
	ToolCallCount   int       `json:"tool_call_count"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	TotalTokens     int       `json:"total_tokens"`
}

// ComputeSubtaskMetrics groups events by detected subtask/phase and computes
// per-subtask duration and token consumption. It is best-effort: events that
// cannot be attributed to a named subtask are grouped under "__other__".
func ComputeSubtaskMetrics(events []*ypb.AIOutputEvent) []SubtaskMetrics {
	groups := make(map[string]*SubtaskMetrics)
	var current string

	for _, e := range events {
		if e == nil {
			continue
		}
		// Stream/tool events during a focus-mode scan carry the scan phase
		// as their NodeId (e.g. "code_audit_scan_sql_injection").
		if strings.HasPrefix(e.NodeId, "code_audit_scan_") {
			current = e.NodeId
		}
		if e.NodeId == "dir_explore" {
			current = "dir_explore"
		}

		key := current
		if key == "" {
			key = "__other__"
		}

		sm, ok := groups[key]
		if !ok {
			sm = &SubtaskMetrics{Name: key}
			groups[key] = sm
		}

		if e.Timestamp > 0 {
			ts := time.Unix(e.Timestamp, 0)
			if sm.StartTime.IsZero() || ts.Before(sm.StartTime) {
				sm.StartTime = ts
			}
			if ts.After(sm.EndTime) {
				sm.EndTime = ts
			}
		}

		sm.EventCount++
		if IsThoughtEvent(e) {
			sm.ThoughtCount++
		}
		if IsToolCallEvent(e) {
			sm.ToolCallCount++
		}

		usage := EstimateEventTokens(e.Content, e.StreamDelta, e.Type, e.NodeId)
		sm.InputTokens += usage.InputTokens
		sm.OutputTokens += usage.OutputTokens
		sm.TotalTokens += usage.TotalTokens
	}

	result := make([]SubtaskMetrics, 0, len(groups))
	for _, sm := range groups {
		if !sm.StartTime.IsZero() && !sm.EndTime.IsZero() {
			sm.DurationSeconds = sm.EndTime.Sub(sm.StartTime).Seconds()
		}
		result = append(result, *sm)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.Before(result[j].StartTime)
	})
	return result
}
