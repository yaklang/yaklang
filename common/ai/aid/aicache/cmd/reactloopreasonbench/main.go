// Command reactloopreasonbench compares the current Yak-style ordinary JSON
// ReAct control loop with a native tool-call control loop over the same
// multi-round task. A sample is valid only when it completes every continue
// round and terminates with one controller-confirmed finish.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	controlToolName    = "react_control"
	negativeArm        = "negative_yak_stop_json"
	positiveArm        = "positive_native_toolcall"
	positiveTrimmedArm = "positive_native_toolcall_trimmed_history"
)

type config struct {
	Model      string
	Trials     int
	Rounds     int
	Pause      time.Duration
	Output     string
	MaxTokens  int
	PromptPath string
	OnlyArm    string
}

type requestShape struct {
	Roles                     []string `json:"roles"`
	LastRole                  string   `json:"last_role"`
	MessageCount              int      `json:"message_count"`
	ReasoningCount            int      `json:"reasoning_count"`
	ToolCallCount             int      `json:"tool_call_count"`
	ToolResultCount           int      `json:"tool_result_count"`
	ReasoningChars            []int    `json:"reasoning_chars,omitempty"`
	ReasoningSHA256           []string `json:"reasoning_sha256,omitempty"`
	ToolCallIDs               []string `json:"tool_call_ids,omitempty"`
	ToolResultIDs             []string `json:"tool_result_ids,omitempty"`
	ToolPairsValid            bool     `json:"tool_pairs_valid"`
	LatestAcceptedCheckpoints []string `json:"latest_accepted_checkpoints,omitempty"`
}

type responseShape struct {
	PreviewBytes       int    `json:"preview_bytes"`
	FinishReason       string `json:"finish_reason"`
	FinishReasonSource string `json:"finish_reason_source"`
	HasReasoningField  bool   `json:"has_reasoning_content_field"`
	HasToolCallsFinish bool   `json:"has_tool_calls_finish"`
	HasStopFinish      bool   `json:"has_stop_finish"`
}

type controlDecision struct {
	Action       string `json:"action"`
	CheckpointID string `json:"checkpoint_id"`
	EvidenceKey  string `json:"evidence_key"`
	Summary      string `json:"summary"`
	FinalAnswer  string `json:"final_answer"`
}

type roundResult struct {
	Trial                int                `json:"trial"`
	Arm                  string             `json:"arm"`
	Round                int                `json:"round"`
	ExpectedAction       string             `json:"expected_action"`
	ActualAction         string             `json:"actual_action"`
	ExpectedCheckpointID string             `json:"expected_checkpoint_id"`
	ExpectedEvidenceKey  string             `json:"expected_evidence_key"`
	DecisionValid        bool               `json:"decision_valid"`
	ControllerStatus     string             `json:"controller_status"`
	DurationMS           int64              `json:"duration_ms"`
	FirstReasonMS        int64              `json:"first_reason_ms,omitempty"`
	FirstContentMS       int64              `json:"first_content_ms,omitempty"`
	ReasoningContent     string             `json:"reasoning_content"`
	ReasoningChars       int                `json:"reasoning_chars"`
	Content              string             `json:"content"`
	ToolCalls            []*aispec.ToolCall `json:"tool_calls,omitempty"`
	Usage                *aispec.ChatUsage  `json:"usage,omitempty"`
	Request              requestShape       `json:"request"`
	Response             responseShape      `json:"response"`
	ExpectedLastRole     string             `json:"expected_last_role"`
	LastRoleVerified     bool               `json:"last_role_verified"`
	ReasoningReplayExact bool               `json:"reasoning_replay_exact"`
	ProtocolEchoSignals  int                `json:"protocol_echo_signals"`
	RecomputationSignals int                `json:"recomputation_signals"`
	HistoryPolicy        string             `json:"history_policy"`
	EvictedToolPairs     int                `json:"evicted_tool_pairs_before_request"`
	Error                string             `json:"error,omitempty"`
}

type chainResult struct {
	Trial          int           `json:"trial"`
	Arm            string        `json:"arm"`
	RoundsExpected int           `json:"rounds_expected"`
	RoundsExecuted int           `json:"rounds_executed"`
	Finished       bool          `json:"finished"`
	FinishRound    int           `json:"finish_round"`
	Failure        string        `json:"failure,omitempty"`
	Rounds         []roundResult `json:"rounds"`
}

type armAggregate struct {
	Arm                    string  `json:"arm"`
	Chains                 int     `json:"chains"`
	FinishedChains         int     `json:"finished_chains"`
	Calls                  int     `json:"calls"`
	ValidCalls             int     `json:"valid_calls"`
	ReasoningChars         int     `json:"reasoning_chars"`
	CompletionTokens       int     `json:"completion_tokens"`
	PromptTokens           int     `json:"prompt_tokens"`
	CachedTokens           int     `json:"cached_tokens"`
	AverageReasoningChars  float64 `json:"average_reasoning_chars"`
	AverageCompletion      float64 `json:"average_completion_tokens"`
	AverageReasoningPerRun float64 `json:"average_reasoning_chars_per_chain"`
	CacheRate              float64 `json:"cache_rate_percent"`
}

type report struct {
	GeneratedAt            string         `json:"generated_at"`
	Model                  string         `json:"model"`
	Trials                 int            `json:"trials"`
	RoundsPerTrial         int            `json:"rounds_per_trial"`
	NegativeProtocol       string         `json:"negative_protocol"`
	PositiveProtocol       string         `json:"positive_protocol"`
	TrimmedProtocol        string         `json:"trimmed_protocol"`
	Chains                 []chainResult  `json:"chains"`
	Aggregate              []armAggregate `json:"aggregate"`
	ReasoningReductionPct  *float64       `json:"reasoning_reduction_percent,omitempty"`
	CompletionReductionPct *float64       `json:"completion_reduction_percent,omitempty"`
}

type streamCapture struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	first   time.Time
	started time.Time
}

func (c *streamCapture) consume(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.mu.Lock()
			if c.first.IsZero() {
				c.first = time.Now()
			}
			_, _ = c.buf.Write(buf[:n])
			c.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (c *streamCapture) string() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *streamCapture) firstMS() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.first.IsZero() {
		return 0
	}
	return c.first.Sub(c.started).Milliseconds()
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Model, "model", "memfit-standard-thinking-free", "AIBalance model")
	flag.IntVar(&cfg.Trials, "trials", 5, "number of paired chains")
	flag.IntVar(&cfg.Rounds, "rounds", 6, "control rounds per chain")
	flag.DurationVar(&cfg.Pause, "pause", 2*time.Second, "pause between model calls")
	flag.StringVar(&cfg.Output, "out", "", "JSON report path")
	flag.IntVar(&cfg.MaxTokens, "max-tokens", 1800, "maximum completion tokens per call")
	flag.StringVar(&cfg.PromptPath, "yak-prompt", "common/ai/aid/aireact/reactloops/loop_default/prompts/instruction.txt", "Yak default-loop instruction path")
	flag.StringVar(&cfg.OnlyArm, "only-arm", "", "run only one arm: negative_yak_stop_json, positive_native_toolcall, or positive_native_toolcall_trimmed_history")
	flag.Parse()
	if cfg.Trials < 1 || cfg.Rounds < 2 || cfg.MaxTokens < 200 {
		fatalf("require trials >= 1, rounds >= 2, max-tokens >= 200")
	}
	if cfg.Output == "" {
		cfg.Output = fmt.Sprintf("/tmp/yak-reactloop-reasonbench-%s.json", time.Now().Format("20060102-150405"))
	}
	if cfg.OnlyArm != "" && cfg.OnlyArm != negativeArm && cfg.OnlyArm != positiveArm && cfg.OnlyArm != positiveTrimmedArm {
		fatalf("unknown -only-arm %q", cfg.OnlyArm)
	}
	prompt, err := os.ReadFile(cfg.PromptPath)
	if err != nil {
		fatalf("read Yak prompt: %v", err)
	}
	yakit.ConfigureNetWork(yakit.GetNetworkConfig())

	rep := report{
		GeneratedAt:      time.Now().Format(time.RFC3339),
		Model:            cfg.Model,
		Trials:           cfg.Trials,
		RoundsPerTrial:   cfg.Rounds,
		NegativeProtocol: "ordinary assistant JSON @action; finish_reason=stop; next round ends in user Timeline observation",
		PositiveProtocol: "native react_control tool call; finish_reason=tool_calls; matching tool result; continue until controller confirms finished",
		TrimmedProtocol:  "same native protocol; before round 3+ atomically evict the oldest completed react_control continue pair; retain only the latest pair and cumulative accepted-checkpoint snapshot",
	}
	for trial := 1; trial <= cfg.Trials; trial++ {
		order := []string{negativeArm, positiveArm, positiveTrimmedArm}
		if cfg.OnlyArm != "" {
			order = []string{cfg.OnlyArm}
		}
		if trial%2 == 0 {
			for left, right := 0, len(order)-1; left < right; left, right = left+1, right-1 {
				order[left], order[right] = order[right], order[left]
			}
		}
		for _, arm := range order {
			chain := runChain(cfg, string(prompt), trial, arm)
			rep.Chains = append(rep.Chains, chain)
			printChain(chain)
			if chain.Failure != "" {
				fatalf("trial %d arm %s: %s", trial, arm, chain.Failure)
			}
			if cfg.Pause > 0 {
				time.Sleep(cfg.Pause)
			}
		}
	}
	rep.Aggregate = aggregate(rep.Chains)
	if hasReductionBaseline(rep.Aggregate) {
		reasoningReduction := reduction(rep.Aggregate, func(a armAggregate) float64 { return float64(a.ReasoningChars) })
		completionReduction := reduction(rep.Aggregate, func(a armAggregate) float64 { return float64(a.CompletionTokens) })
		rep.ReasoningReductionPct = &reasoningReduction
		rep.CompletionReductionPct = &completionReduction
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	if err := os.WriteFile(cfg.Output, data, 0o600); err != nil {
		fatalf("write report: %v", err)
	}
	printAggregate(rep)
	fmt.Printf("report=%s\n", cfg.Output)
}

func runChain(cfg config, yakPrompt string, trial int, arm string) chainResult {
	chain := chainResult{Trial: trial, Arm: arm, RoundsExpected: cfg.Rounds}
	useTools := arm == positiveArm || arm == positiveTrimmedArm
	trimHistory := arm == positiveTrimmedArm
	system := yakPrompt + "\n\n" + experimentControllerPrompt(useTools)
	messages := []aispec.ChatDetail{
		aispec.NewSystemChatDetail(system),
		aispec.NewUserChatDetail(initialTask(trial, cfg.Rounds)),
	}
	for round := 1; round <= cfg.Rounds; round++ {
		if round == 1 || !useTools {
			messages = append(messages, aispec.NewUserChatDetail(roundTimeline(trial, round, cfg.Rounds)))
		}
		expectedAction := "continue"
		if round == cfg.Rounds {
			expectedAction = "finish"
		}
		expectedLastRole := "user"
		if useTools && round > 1 {
			expectedLastRole = "tool"
		}
		evictedPairs := 0
		if trimHistory && round >= 3 {
			messages, evictedPairs = evictOldestCompletedToolPair(messages)
		}
		expectedReasoning := reasoningItems(messages)
		r := invoke(cfg, trial, arm, round, messages, expectedLastRole, useTools)
		r.EvictedToolPairs = evictedPairs
		if trimHistory {
			r.HistoryPolicy = "retain_latest_completed_tool_pair"
		} else {
			r.HistoryPolicy = "retain_full_history"
		}
		r.ExpectedAction = expectedAction
		r.ExpectedCheckpointID = checkpointID(trial, round)
		r.ExpectedEvidenceKey = evidenceKey(trial, round)
		decision, err := parseDecision(r, useTools)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.ActualAction = decision.Action
			r.DecisionValid = decision.Action == r.ExpectedAction &&
				decision.CheckpointID == r.ExpectedCheckpointID &&
				decision.EvidenceKey == r.ExpectedEvidenceKey &&
				strings.TrimSpace(decision.Summary) != ""
			if expectedAction == "finish" {
				r.DecisionValid = r.DecisionValid && strings.Contains(decision.FinalAnswer, finalMarker(trial))
				if useTools {
					for checkpointRound := 1; checkpointRound <= cfg.Rounds; checkpointRound++ {
						r.DecisionValid = r.DecisionValid && strings.Contains(decision.FinalAnswer, checkpointID(trial, checkpointRound))
					}
				}
			}
		}
		r.ReasoningReplayExact = requestReplays(r.Request, expectedReasoning)
		if round == 1 && r.Request.ReasoningCount == 0 {
			r.ReasoningReplayExact = true
		}
		if !r.LastRoleVerified {
			r.Error = fmt.Sprintf("request ended in %q, expected %q", r.Request.LastRole, expectedLastRole)
		} else if useTools && !r.Request.ToolPairsValid {
			r.Error = "wire request contains an orphaned or reordered tool result"
		} else if useTools && round > 1 && !acceptedCheckpointSnapshotExact(r.Request.LatestAcceptedCheckpoints, trial, round-1) {
			r.Error = "latest control result did not carry the complete accepted-checkpoint snapshot"
		} else if trimHistory && round >= 3 && evictedPairs != 1 {
			r.Error = fmt.Sprintf("trimmed history evicted %d tool pairs, expected 1", evictedPairs)
		} else if !r.ReasoningReplayExact {
			r.Error = "wire request did not replay every retained reasoning item exactly"
		} else if !r.DecisionValid {
			r.Error = fmt.Sprintf("invalid decision action=%q checkpoint=%q evidence=%q", decision.Action, decision.CheckpointID, decision.EvidenceKey)
		} else if useTools && r.Response.FinishReason != "tool_calls" {
			r.Error = fmt.Sprintf("positive response finish_reason=%q", r.Response.FinishReason)
		} else if !useTools && r.Response.FinishReason != "stop" {
			r.Error = fmt.Sprintf("negative response finish_reason=%q", r.Response.FinishReason)
		}

		assistant := aispec.ChatDetail{Role: "assistant", Content: r.Content, ReasoningContent: r.ReasoningContent, ToolCalls: cloneToolCalls(r.ToolCalls)}
		messages = append(messages, assistant)
		if useTools {
			call := firstToolCall(r.ToolCalls)
			status := "accepted_continue"
			if expectedAction == "finish" {
				status = "finished"
			}
			r.ControllerStatus = status
			messages = append(messages, aispec.NewToolChatDetailWithID(call.ID, controlToolName, controllerResult(trial, round, cfg.Rounds, status)))
		} else if expectedAction == "finish" {
			r.ControllerStatus = "finished"
		} else {
			r.ControllerStatus = "accepted_continue"
		}
		chain.Rounds = append(chain.Rounds, r)
		chain.RoundsExecuted++
		if r.Error != "" {
			chain.Failure = fmt.Sprintf("round %d: %s", round, r.Error)
			return chain
		}
		if expectedAction == "finish" {
			chain.Finished = true
			chain.FinishRound = round
			return chain
		}
		if cfg.Pause > 0 {
			time.Sleep(cfg.Pause)
		}
	}
	chain.Failure = "round limit exhausted without finish"
	return chain
}

// evictOldestCompletedToolPair removes one assistant tool-call message and its
// immediately matching tool result. It never removes only one half of the
// protocol transaction. The first two messages (system and initial task) are
// intentionally retained.
func evictOldestCompletedToolPair(messages []aispec.ChatDetail) ([]aispec.ChatDetail, int) {
	for i := 0; i+1 < len(messages); i++ {
		assistant := messages[i]
		toolResult := messages[i+1]
		if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || toolResult.Role != "tool" {
			continue
		}
		call := assistant.ToolCalls[0]
		if call == nil || call.ID == "" || call.Function.Name != controlToolName || toolResult.Name != controlToolName || toolResult.ToolCallID != call.ID {
			continue
		}
		var decision controlDecision
		if json.Unmarshal([]byte(call.Function.Arguments), &decision) != nil || decision.Action != "continue" {
			continue
		}
		trimmed := make([]aispec.ChatDetail, 0, len(messages)-2)
		trimmed = append(trimmed, messages[:i]...)
		trimmed = append(trimmed, messages[i+2:]...)
		return trimmed, 1
	}
	return messages, 0
}

func reasoningItems(messages []aispec.ChatDetail) []string {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.ReasoningContent != "" {
			items = append(items, message.ReasoningContent)
		}
	}
	return items
}

func invoke(cfg config, trial int, arm string, round int, messages []aispec.ChatDetail, expectedLastRole string, useTools bool) roundResult {
	start := time.Now()
	reasonCapture := &streamCapture{started: start}
	contentCapture := &streamCapture{started: start}
	var mu sync.Mutex
	var calls []*aispec.ToolCall
	var usage *aispec.ChatUsage
	var reqShape requestShape
	var rspShape responseShape
	opts := []aispec.AIConfigOption{
		aispec.WithType("aibalance"),
		aispec.WithModel(cfg.Model),
		aispec.WithRawMessages(messages),
		aispec.WithMaxTokens(int64(cfg.MaxTokens)),
		aispec.WithTemperature(0),
		aispec.WithTimeout(300),
		aispec.WithReasonStreamHandler(reasonCapture.consume),
		aispec.WithStreamHandler(contentCapture.consume),
		aispec.WithToolCallCallback(func(got []*aispec.ToolCall) {
			mu.Lock()
			calls = cloneToolCalls(got)
			mu.Unlock()
		}),
		aispec.WithUsageCallback(func(got *aispec.ChatUsage) {
			mu.Lock()
			usage = cloneUsage(got)
			mu.Unlock()
		}),
		aispec.WithRawHTTPRequestResponseCallback(func(req, _ []byte, preview []byte, got *aispec.ChatUsage) {
			mu.Lock()
			reqShape = parseRequestShape(req)
			rspShape = parseResponseShape(preview)
			if usage == nil {
				usage = cloneUsage(got)
			}
			mu.Unlock()
		}),
	}
	if useTools {
		opts = append(opts, aispec.WithTools(controlTools()), aispec.WithToolChoice("required"))
	}
	content, err := ai.Chat("", opts...)
	mu.Lock()
	gotCalls := cloneToolCalls(calls)
	gotUsage := cloneUsage(usage)
	gotRequest := reqShape
	gotResponse := rspShape
	mu.Unlock()
	if strings.TrimSpace(content) == "" {
		content = contentCapture.string()
	}
	if gotResponse.FinishReason != "" {
		gotResponse.FinishReasonSource = "raw_sse_preview"
	} else if err == nil && gotUsage != nil && len(gotCalls) > 0 {
		// RawHTTPRequestResponseCallback intentionally keeps only the first 4 KiB.
		// Long streams may therefore omit the terminal SSE frame. A completed
		// native call with parsed tool_calls can only terminate as tool_calls in
		// this experiment because max_tokens is well above the tiny arguments.
		gotResponse.FinishReason = "tool_calls"
		gotResponse.FinishReasonSource = "sdk_completed_tool_call_inference"
	} else if err == nil && gotUsage != nil && len(gotCalls) == 0 {
		// The same preview limit hides the terminal frame after long reasoning.
		// A normally completed, usage-bearing, non-tool response whose JSON was
		// fully parsed is recorded as stop, with the inference source explicit.
		gotResponse.FinishReason = "stop"
		gotResponse.FinishReasonSource = "sdk_completed_non_tool_inference"
	}
	reasoning := reasonCapture.string()
	r := roundResult{
		Trial:                trial,
		Arm:                  arm,
		Round:                round,
		DurationMS:           time.Since(start).Milliseconds(),
		FirstReasonMS:        reasonCapture.firstMS(),
		FirstContentMS:       contentCapture.firstMS(),
		ReasoningContent:     reasoning,
		ReasoningChars:       len([]rune(reasoning)),
		Content:              content,
		ToolCalls:            gotCalls,
		Usage:                gotUsage,
		Request:              gotRequest,
		Response:             gotResponse,
		ExpectedLastRole:     expectedLastRole,
		LastRoleVerified:     gotRequest.LastRole == expectedLastRole,
		ProtocolEchoSignals:  countSignals(reasoning, []string{"system prompt", "系统提示", "schema", "格式要求", "output format"}),
		RecomputationSignals: countSignals(reasoning, []string{"重新", "从头", "recompute", "start over", "re-evaluate"}),
	}
	if err != nil {
		r.Error = err.Error()
	} else if gotUsage == nil {
		r.Error = "response completed without usage; possible partial/error stream"
	}
	return r
}

func experimentControllerPrompt(native bool) string {
	shared := `
================================================================================
# REACTLOOP REASONING REDUCTION EXPERIMENT OVERRIDE
This controlled experiment overrides only the final control-output mechanism.
The Timeline contains CONTROL_ROUND=i/N and one EVIDENCE_KEY. Process only the
current round. For rounds 1..N-1 choose continue. For round N choose finish.
Copy CHECKPOINT_ID and EVIDENCE_KEY exactly. Give a concise non-empty summary.
On finish, final_answer must contain the supplied FINAL_MARKER. Never finish
early, never continue after round N, and never emit more than one decision.
================================================================================
`
	if native {
		return shared + `Use the native react_control function exactly once. Do not emit legacy @action JSON in assistant content. The function call is the complete decision. The latest tool result contains accepted_checkpoints as the controller's cumulative state snapshot. On finish, final_answer must contain every accepted checkpoint plus the current CHECKPOINT_ID, in addition to FINAL_MARKER.`
	}
	return shared + `Do not call any native function. Return exactly one ordinary assistant JSON object and no surrounding prose: {"@action":"continue|finish","checkpoint_id":"...","evidence_key":"...","summary":"...","final_answer":"..."}. This is the current Yak-style stop-JSON control arm.`
}

func initialTask(trial, rounds int) string {
	return fmt.Sprintf(`CURRENT-TASK trial-%02d: inspect an incident ledger through exactly %d controller rounds. Each Timeline round exposes one independent evidence window. Commit every checkpoint in order. Do not skip rounds. The controller, not the model, decides whether a decision was accepted.`, trial, rounds)
}

func roundTimeline(trial, round, rounds int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# TIMELINE OPEN\nCONTROL_ROUND=%d/%d\nCHECKPOINT_ID=%s\nEVIDENCE_KEY=%s\n", round, rounds, checkpointID(trial, round), evidenceKey(trial, round))
	if round == rounds {
		fmt.Fprintf(&b, "FINAL_MARKER=%s\n", finalMarker(trial))
	}
	b.WriteString("Observation window (evaluate only this window):\n")
	for i := 1; i <= 36; i++ {
		fmt.Fprintf(&b, "OBS-%02d lane=L%d weight=%d state=%s predecessor=OBS-%02d\n", i, (i*trial+round*3)%7, (i*i+trial*17+round*13)%101, []string{"open", "checked", "frozen"}[(i+trial+round)%3], max(0, i-1))
	}
	if round < rounds {
		b.WriteString("Required controller decision: continue. Summarize the strongest signal from this window; do not finish.\n")
	} else {
		b.WriteString("All scheduled windows are now present. Required controller decision: finish. Include FINAL_MARKER in final_answer.\n")
	}
	return b.String()
}

func controllerResult(trial, round, rounds int, status string) string {
	acceptedCheckpoints := make([]string, 0, round)
	for acceptedRound := 1; acceptedRound <= round; acceptedRound++ {
		acceptedCheckpoints = append(acceptedCheckpoints, checkpointID(trial, acceptedRound))
	}
	value := map[string]any{
		"status":               status,
		"checkpoint_id":        checkpointID(trial, round),
		"accepted":             true,
		"accepted_checkpoints": acceptedCheckpoints,
	}
	if round < rounds {
		value["next_round"] = round + 1
		value["timeline_open"] = roundTimeline(trial, round+1, rounds)
	} else {
		value["loop_state"] = "terminated_safely"
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func acceptedCheckpointSnapshotExact(got []string, trial, acceptedRounds int) bool {
	if len(got) != acceptedRounds {
		return false
	}
	for i := 1; i <= acceptedRounds; i++ {
		if got[i-1] != checkpointID(trial, i) {
			return false
		}
	}
	return true
}

func checkpointID(trial, round int) string { return fmt.Sprintf("T%02d-R%02d", trial, round) }
func evidenceKey(trial, round int) string {
	return fmt.Sprintf("EVID-%02d-%02d-%04d", trial, round, 3100+trial*97+round*131)
}
func finalMarker(trial int) string { return fmt.Sprintf("FINISHED-T%02d-%04d", trial, 7300+trial*173) }

func controlTools() []aispec.Tool {
	return []aispec.Tool{{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        controlToolName,
			Description: "Commit one ReActLoop control transition for the current Timeline round.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":        map[string]any{"type": "string", "enum": []string{"continue", "finish"}},
					"checkpoint_id": map[string]any{"type": "string"},
					"evidence_key":  map[string]any{"type": "string"},
					"summary":       map[string]any{"type": "string"},
					"final_answer":  map[string]any{"type": "string"},
				},
				"required":             []string{"action", "checkpoint_id", "evidence_key", "summary", "final_answer"},
				"additionalProperties": false,
			},
		},
	}}
}

func parseDecision(r roundResult, native bool) (controlDecision, error) {
	var decision controlDecision
	if native {
		call := firstToolCall(r.ToolCalls)
		if call == nil || call.Function.Name != controlToolName {
			return decision, errors.New("missing react_control tool call")
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &decision); err != nil {
			return decision, fmt.Errorf("decode tool arguments: %w", err)
		}
		return decision, nil
	}
	content := strings.TrimSpace(r.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var legacy struct {
		Action       string `json:"@action"`
		CheckpointID string `json:"checkpoint_id"`
		EvidenceKey  string `json:"evidence_key"`
		Summary      string `json:"summary"`
		FinalAnswer  string `json:"final_answer"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &legacy); err != nil {
		return decision, fmt.Errorf("decode legacy JSON: %w", err)
	}
	return controlDecision(legacy), nil
}

func firstToolCall(calls []*aispec.ToolCall) *aispec.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	return calls[0]
}

func requestReplays(shape requestShape, prior []string) bool {
	if shape.ReasoningCount != len(prior) || len(shape.ReasoningSHA256) != len(prior) {
		return false
	}
	for i, reasoning := range prior {
		if shape.ReasoningSHA256[i] != textSHA256(reasoning) {
			return false
		}
	}
	return true
}

func parseRequestShape(packet []byte) requestShape {
	body := packet
	if idx := bytes.Index(packet, []byte("\r\n\r\n")); idx >= 0 {
		body = packet[idx+4:]
	}
	var req struct {
		Messages []aispec.ChatDetail `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return requestShape{}
	}
	shape := requestShape{MessageCount: len(req.Messages), ToolPairsValid: true}
	seenToolCalls := make(map[string]struct{})
	for _, msg := range req.Messages {
		shape.Roles = append(shape.Roles, msg.Role)
		if msg.ReasoningContent != "" {
			shape.ReasoningCount++
			shape.ReasoningChars = append(shape.ReasoningChars, len([]rune(msg.ReasoningContent)))
			shape.ReasoningSHA256 = append(shape.ReasoningSHA256, textSHA256(msg.ReasoningContent))
		}
		shape.ToolCallCount += len(msg.ToolCalls)
		for _, call := range msg.ToolCalls {
			if call == nil || call.ID == "" {
				shape.ToolPairsValid = false
				continue
			}
			shape.ToolCallIDs = append(shape.ToolCallIDs, call.ID)
			seenToolCalls[call.ID] = struct{}{}
		}
		if msg.Role == "tool" {
			shape.ToolResultCount++
			shape.ToolResultIDs = append(shape.ToolResultIDs, msg.ToolCallID)
			if _, ok := seenToolCalls[msg.ToolCallID]; !ok || msg.ToolCallID == "" {
				shape.ToolPairsValid = false
			}
			if content, ok := msg.Content.(string); ok {
				var result struct {
					AcceptedCheckpoints []string `json:"accepted_checkpoints"`
				}
				if json.Unmarshal([]byte(content), &result) == nil && len(result.AcceptedCheckpoints) > 0 {
					shape.LatestAcceptedCheckpoints = append([]string(nil), result.AcceptedCheckpoints...)
				}
			}
		}
	}
	if len(shape.Roles) > 0 {
		shape.LastRole = shape.Roles[len(shape.Roles)-1]
	}
	return shape
}

func parseResponseShape(preview []byte) responseShape {
	finishReason := ""
	if match := regexp.MustCompile(`"finish_reason"\s*:\s*"([^"]+)"`).FindSubmatch(preview); len(match) == 2 {
		finishReason = string(match[1])
	}
	shape := responseShape{
		PreviewBytes:       len(preview),
		FinishReason:       finishReason,
		HasReasoningField:  bytes.Contains(preview, []byte(`"reasoning_content"`)),
		HasToolCallsFinish: finishReason == "tool_calls",
		HasStopFinish:      finishReason == "stop",
	}
	return shape
}

func textSHA256(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text))) }

func cloneToolCalls(in []*aispec.ToolCall) []*aispec.ToolCall {
	out := make([]*aispec.ToolCall, 0, len(in))
	for _, call := range in {
		out = append(out, call.Clone())
	}
	return out
}

func cloneUsage(in *aispec.ChatUsage) *aispec.ChatUsage {
	if in == nil {
		return nil
	}
	out := *in
	if in.PromptTokensDetails != nil {
		details := *in.PromptTokensDetails
		out.PromptTokensDetails = &details
	}
	return &out
}

func countSignals(text string, signals []string) int {
	text = strings.ToLower(text)
	total := 0
	for _, signal := range signals {
		total += strings.Count(text, strings.ToLower(signal))
	}
	return total
}

func aggregate(chains []chainResult) []armAggregate {
	byArm := map[string]*armAggregate{}
	for _, chain := range chains {
		a := byArm[chain.Arm]
		if a == nil {
			a = &armAggregate{Arm: chain.Arm}
			byArm[chain.Arm] = a
		}
		a.Chains++
		if chain.Finished {
			a.FinishedChains++
		}
		for _, round := range chain.Rounds {
			a.Calls++
			a.ReasoningChars += round.ReasoningChars
			if round.DecisionValid && round.Error == "" {
				a.ValidCalls++
			}
			if round.Usage != nil {
				a.CompletionTokens += round.Usage.CompletionTokens
				a.PromptTokens += round.Usage.PromptTokens
				if round.Usage.PromptTokensDetails != nil {
					a.CachedTokens += round.Usage.PromptTokensDetails.CachedTokens
				}
			}
		}
	}
	result := make([]armAggregate, 0, len(byArm))
	for _, a := range byArm {
		if a.Calls > 0 {
			a.AverageReasoningChars = float64(a.ReasoningChars) / float64(a.Calls)
			a.AverageCompletion = float64(a.CompletionTokens) / float64(a.Calls)
		}
		if a.Chains > 0 {
			a.AverageReasoningPerRun = float64(a.ReasoningChars) / float64(a.Chains)
		}
		if a.PromptTokens > 0 {
			a.CacheRate = 100 * float64(a.CachedTokens) / float64(a.PromptTokens)
		}
		result = append(result, *a)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Arm < result[j].Arm })
	return result
}

func reduction(aggregates []armAggregate, value func(armAggregate) float64) float64 {
	var negative, positive float64
	for _, a := range aggregates {
		if a.Arm == negativeArm {
			negative = value(a)
		} else if a.Arm == positiveArm {
			positive = value(a)
		}
	}
	if negative == 0 {
		return 0
	}
	return 100 * (negative - positive) / negative
}

func hasReductionBaseline(aggregates []armAggregate) bool {
	hasNegative := false
	hasPositive := false
	for _, aggregate := range aggregates {
		switch aggregate.Arm {
		case negativeArm:
			hasNegative = true
		case positiveArm:
			hasPositive = true
		}
	}
	return hasNegative && hasPositive
}

func printChain(chain chainResult) {
	fmt.Printf("trial=%d arm=%s finished=%t rounds=%d/%d\n", chain.Trial, chain.Arm, chain.Finished, chain.RoundsExecuted, chain.RoundsExpected)
	for _, round := range chain.Rounds {
		completion := 0
		if round.Usage != nil {
			completion = round.Usage.CompletionTokens
		}
		fmt.Printf("  round=%d action=%s finish_reason=%s reason_chars=%d completion=%d replay=%t valid=%t duration_ms=%d\n", round.Round, round.ActualAction, round.Response.FinishReason, round.ReasoningChars, completion, round.ReasoningReplayExact, round.DecisionValid, round.DurationMS)
	}
}

func printAggregate(rep report) {
	fmt.Println("aggregate:")
	for _, a := range rep.Aggregate {
		fmt.Printf("  arm=%s chains=%d/%d calls=%d valid=%d avg_reason_chars=%.1f avg_completion=%.1f cache_rate=%.2f%%\n", a.Arm, a.FinishedChains, a.Chains, a.Calls, a.ValidCalls, a.AverageReasoningChars, a.AverageCompletion, a.CacheRate)
	}
	if rep.ReasoningReductionPct != nil && rep.CompletionReductionPct != nil {
		fmt.Printf("  reasoning_reduction=%.2f%% completion_reduction=%.2f%%\n", *rep.ReasoningReductionPct, *rep.CompletionReductionPct)
	} else {
		fmt.Println("  reduction=n/a (negative and positive comparison arms were not both selected)")
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "reactloopreasonbench: "+format+"\n", args...)
	os.Exit(1)
}
