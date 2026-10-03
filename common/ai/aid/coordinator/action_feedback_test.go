package coordinator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorPlanReceiptDoesNotScaleWithPlan(t *testing.T) {
	for _, size := range []int{128, 128 * 1024} {
		p := &Plan{Document: strings.Repeat("document", size), Tasks: []Task{{ID: "a", Goal: strings.Repeat("brief", size)}}}
		data, err := json.Marshal(planReceipt(Snapshot{DraftVersion: 2, ApprovedVersion: 1, SubmittedVersion: 2, Draft: p, Approved: p}))
		require.NoError(t, err)
		require.Less(t, len(data), 200, "plan inspection must return a bounded receipt")
		require.Contains(t, string(data), `"draft_version":2`)
		require.Contains(t, string(data), `"approved_version":1`)
		require.Contains(t, string(data), `"submitted_version":2`)
		require.NotContains(t, string(data), "documentdocument")
		require.NotContains(t, string(data), "briefbrief")
	}
}

func TestCoordinatorTaskObservationPreservesReviewFactsWithoutBrief(t *testing.T) {
	a := Attempt{
		Task: Task{ID: "logical-a", Goal: strings.Repeat("static-brief", 10000), DependsOn: []string{"upstream"}},
		ID:   7, PlanVersion: 3, State: AwaitingReview, Seen: true,
		Result:       Result{Summary: strings.Repeat("actual-result", 1000), Artifacts: []string{"artifacts/report.md"}, EvidenceIDs: []string{"source.observed"}, Error: "verification detail"},
		ReviewReason: "prior review detail",
	}
	data, err := json.Marshal(taskObservations([]Attempt{a}))
	require.NoError(t, err)
	require.NotContains(t, string(data), "static-brief")
	require.NotContains(t, string(data), "upstream")
	var decoded any
	require.NoError(t, json.Unmarshal(data, &decoded))
	item := decoded.([]any)[0].(map[string]any)
	require.Equal(t, "logical-a", item["task_id"])
	require.EqualValues(t, 7, item["attempt_id"])
	require.EqualValues(t, 3, item["plan_version"])
	require.Equal(t, string(AwaitingReview), item["state"])
	require.Equal(t, true, item["seen"])
	require.Equal(t, a.ReviewReason, item["review_reason"])
	result, err := json.Marshal(item["result"])
	require.NoError(t, err)
	var roundtrip Result
	require.NoError(t, json.Unmarshal(result, &roundtrip))
	require.Equal(t, a.Result, roundtrip, "review facts must not be truncated")
}
