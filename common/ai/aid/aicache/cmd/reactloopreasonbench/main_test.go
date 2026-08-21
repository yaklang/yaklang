package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestRoundTimelineHasDeterministicTerminalContract(t *testing.T) {
	continueRound := roundTimeline(2, 1, 3)
	require.Contains(t, continueRound, "CONTROL_ROUND=1/3")
	require.Contains(t, continueRound, "CHECKPOINT_ID=T02-R01")
	require.Contains(t, continueRound, "Required controller decision: continue")
	require.NotContains(t, continueRound, "FINAL_MARKER=")

	finishRound := roundTimeline(2, 3, 3)
	require.Contains(t, finishRound, "CONTROL_ROUND=3/3")
	require.Contains(t, finishRound, "Required controller decision: finish")
	require.Contains(t, finishRound, "FINAL_MARKER="+finalMarker(2))
}

func TestControllerResultCarriesCumulativeCheckpointSnapshot(t *testing.T) {
	result := controllerResult(2, 3, 6, "accepted_continue")
	var decoded struct {
		AcceptedCheckpoints []string `json:"accepted_checkpoints"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &decoded))
	require.Equal(t, []string{"T02-R01", "T02-R02", "T02-R03"}, decoded.AcceptedCheckpoints)
	require.True(t, acceptedCheckpointSnapshotExact(decoded.AcceptedCheckpoints, 2, 3))
	require.False(t, acceptedCheckpointSnapshotExact(decoded.AcceptedCheckpoints[1:], 2, 2))
}

func TestParseNativeAndLegacyDecision(t *testing.T) {
	decision := controlDecision{
		Action:       "finish",
		CheckpointID: "T01-R06",
		EvidenceKey:  "EVID-01-06-0001",
		Summary:      "done",
		FinalAnswer:  finalMarker(1),
	}
	data, err := json.Marshal(decision)
	require.NoError(t, err)

	native, err := parseDecision(roundResult{ToolCalls: []*aispec.ToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: aispec.FuncReturn{
			Name:      controlToolName,
			Arguments: string(data),
		},
	}}}, true)
	require.NoError(t, err)
	require.Equal(t, decision, native)

	legacyJSON := `{"@action":"finish","checkpoint_id":"T01-R06","evidence_key":"EVID-01-06-0001","summary":"done","final_answer":"` + finalMarker(1) + `"}`
	legacy, err := parseDecision(roundResult{Content: legacyJSON}, false)
	require.NoError(t, err)
	require.Equal(t, decision, legacy)
}

func TestRequestReplayRequiresEveryPriorReasoningItemInOrder(t *testing.T) {
	prior := []string{"round one reasoning", "第二轮 reasoning"}
	shape := requestShape{
		ReasoningCount:  2,
		ReasoningSHA256: []string{textSHA256(prior[0]), textSHA256(prior[1])},
	}
	require.True(t, requestReplays(shape, prior))
	shape.ReasoningSHA256[0], shape.ReasoningSHA256[1] = shape.ReasoningSHA256[1], shape.ReasoningSHA256[0]
	require.False(t, requestReplays(shape, prior))
}

func TestEvictOldestCompletedToolPairIsAtomic(t *testing.T) {
	continueArgs := `{"action":"continue"}`
	firstCall := &aispec.ToolCall{ID: "call-1", Type: "function", Function: aispec.FuncReturn{Name: controlToolName, Arguments: continueArgs}}
	secondCall := &aispec.ToolCall{ID: "call-2", Type: "function", Function: aispec.FuncReturn{Name: controlToolName, Arguments: continueArgs}}
	messages := []aispec.ChatDetail{
		aispec.NewSystemChatDetail("system"),
		aispec.NewUserChatDetail("task"),
		{Role: "assistant", ToolCalls: []*aispec.ToolCall{firstCall}},
		aispec.NewToolChatDetailWithID("call-1", controlToolName, "round 2"),
		{Role: "assistant", ToolCalls: []*aispec.ToolCall{secondCall}},
		aispec.NewToolChatDetailWithID("call-2", controlToolName, "round 3"),
	}

	trimmed, evicted := evictOldestCompletedToolPair(messages)
	require.Equal(t, 1, evicted)
	require.Len(t, trimmed, 4)
	require.Equal(t, []string{"system", "user", "assistant", "tool"}, []string{trimmed[0].Role, trimmed[1].Role, trimmed[2].Role, trimmed[3].Role})
	require.Equal(t, "call-2", trimmed[2].ToolCalls[0].ID)
	require.Equal(t, "call-2", trimmed[3].ToolCallID)

	shape := requestShape{ToolPairsValid: true}
	data, err := json.Marshal(struct {
		Messages []aispec.ChatDetail `json:"messages"`
	}{Messages: trimmed})
	require.NoError(t, err)
	shape = parseRequestShape(data)
	require.True(t, shape.ToolPairsValid)
	require.Equal(t, []string{"call-2"}, shape.ToolCallIDs)
	require.Equal(t, []string{"call-2"}, shape.ToolResultIDs)
}

func TestEvictionNeverRemovesBusinessToolOrFinishControl(t *testing.T) {
	business := &aispec.ToolCall{ID: "business-1", Type: "function", Function: aispec.FuncReturn{Name: "scan_port", Arguments: `{}`}}
	finish := &aispec.ToolCall{ID: "finish-1", Type: "function", Function: aispec.FuncReturn{Name: controlToolName, Arguments: `{"action":"finish"}`}}
	for _, pair := range []struct {
		call *aispec.ToolCall
		tool aispec.ChatDetail
	}{
		{business, aispec.NewToolChatDetailWithID("business-1", "scan_port", "done")},
		{finish, aispec.NewToolChatDetailWithID("finish-1", controlToolName, "finished")},
	} {
		messages := []aispec.ChatDetail{{Role: "assistant", ToolCalls: []*aispec.ToolCall{pair.call}}, pair.tool}
		trimmed, evicted := evictOldestCompletedToolPair(messages)
		require.Zero(t, evicted)
		require.Equal(t, messages, trimmed)
	}
}

func TestOrphanedToolResultIsRejectedByWireValidator(t *testing.T) {
	messages := []aispec.ChatDetail{
		aispec.NewSystemChatDetail("system"),
		aispec.NewUserChatDetail("task"),
		aispec.NewToolChatDetailWithID("call-removed", controlToolName, "round 2"),
	}
	data, err := json.Marshal(struct {
		Messages []aispec.ChatDetail `json:"messages"`
	}{Messages: messages})
	require.NoError(t, err)
	shape := parseRequestShape(data)
	require.False(t, shape.ToolPairsValid)
	require.Empty(t, shape.ToolCallIDs)
	require.Equal(t, []string{"call-removed"}, shape.ToolResultIDs)
}

func TestAggregateAndReduction(t *testing.T) {
	chains := []chainResult{
		{Arm: negativeArm, Finished: true, Rounds: []roundResult{
			{ReasoningChars: 1000, DecisionValid: true, Usage: &aispec.ChatUsage{CompletionTokens: 200}},
			{ReasoningChars: 1000, DecisionValid: true, Usage: &aispec.ChatUsage{CompletionTokens: 200}},
		}},
		{Arm: positiveArm, Finished: true, Rounds: []roundResult{
			{ReasoningChars: 100, DecisionValid: true, Usage: &aispec.ChatUsage{CompletionTokens: 100}},
			{ReasoningChars: 100, DecisionValid: true, Usage: &aispec.ChatUsage{CompletionTokens: 100}},
		}},
	}
	aggs := aggregate(chains)
	require.Len(t, aggs, 2)
	require.True(t, hasReductionBaseline(aggs))
	require.InDelta(t, 90, reduction(aggs, func(a armAggregate) float64 { return float64(a.ReasoningChars) }), 0.001)
	require.InDelta(t, 50, reduction(aggs, func(a armAggregate) float64 { return float64(a.CompletionTokens) }), 0.001)
	require.False(t, hasReductionBaseline(aggs[1:]))
}
