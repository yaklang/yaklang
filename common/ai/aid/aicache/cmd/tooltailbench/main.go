// Command tooltailbench compares reasoning continuation when a native tool
// result is the last message with the same history followed by a user message.
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const controlToolName = "react_control"

type config struct {
	Model           string
	Trials          int
	ReasoningEffort string
	Output          string
	Pause           time.Duration
	Conditions      string
}

type requestShape struct {
	Roles           []string `json:"roles"`
	LastRole        string   `json:"last_role"`
	MessageCount    int      `json:"message_count"`
	ReasoningCount  int      `json:"reasoning_count"`
	ToolCallCount   int      `json:"tool_call_count"`
	ToolResultCount int      `json:"tool_result_count"`
	ReasoningChars  []int    `json:"reasoning_chars,omitempty"`
	ReasoningSHA256 []string `json:"reasoning_sha256,omitempty"`
}

type responseShape struct {
	PreviewBytes             int  `json:"preview_bytes"`
	HasReasoningContentField bool `json:"has_reasoning_content_field"`
	HasReasoningField        bool `json:"has_reasoning_field"`
	HasToolCallsFinish       bool `json:"has_tool_calls_finish"`
	HasStopFinish            bool `json:"has_stop_finish"`
}

type conditionInput struct {
	Messages          []aispec.ChatDetail
	UseTools          bool
	ExpectedLastRole  string
	ExpectedReasoning string
	ExpectedAnswer    string
}

type result struct {
	Trial                int                `json:"trial"`
	Condition            string             `json:"condition"`
	DurationMS           int64              `json:"duration_ms"`
	FirstReasonMS        int64              `json:"first_reason_ms,omitempty"`
	FirstContentMS       int64              `json:"first_content_ms,omitempty"`
	Reasoning            string             `json:"reasoning_content"`
	Content              string             `json:"content"`
	ToolCalls            []*aispec.ToolCall `json:"tool_calls,omitempty"`
	Usage                *aispec.ChatUsage  `json:"usage,omitempty"`
	Request              requestShape       `json:"request"`
	Response             responseShape      `json:"response"`
	ExpectedLastRole     string             `json:"expected_last_role"`
	LastRoleVerified     bool               `json:"last_role_verified"`
	ReasoningReplayExact bool               `json:"reasoning_replay_exact"`
	SelectedAction       string             `json:"selected_action,omitempty"`
	ContinuesCheckpoint  bool               `json:"continues_checkpoint"`
	RecomputeSignals     int                `json:"recompute_signals"`
	ProtocolEchoSignals  int                `json:"protocol_echo_signals"`
	Error                string             `json:"error,omitempty"`
}

type trialReport struct {
	Trial           int      `json:"trial"`
	ToolMarker      string   `json:"tool_marker"`
	ReasoningMarker string   `json:"reasoning_marker"`
	ExpectedAnswer  string   `json:"expected_answer"`
	Bootstrap       result   `json:"bootstrap"`
	Results         []result `json:"results"`
}

type report struct {
	GeneratedAt string        `json:"generated_at"`
	Model       string        `json:"model"`
	Trials      int           `json:"trials"`
	Reports     []trialReport `json:"reports"`
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
	flag.IntVar(&cfg.Trials, "trials", 5, "number of paired trials")
	flag.StringVar(&cfg.ReasoningEffort, "reasoning-effort", "", "optional reasoning_effort")
	flag.StringVar(&cfg.Output, "out", "", "JSON report path (default: /tmp)")
	flag.DurationVar(&cfg.Pause, "pause", 2*time.Second, "pause between continuation requests")
	flag.StringVar(&cfg.Conditions, "conditions", "", "optional comma-separated condition filter")
	flag.Parse()
	if cfg.Trials < 1 {
		fatalf("trials must be positive")
	}
	if cfg.Output == "" {
		cfg.Output = fmt.Sprintf("/tmp/yak-tooltailbench-%s.json", time.Now().Format("20060102-150405"))
	}
	// A standalone Go command does not run common/yak/cmd/yak.go's startup
	// hook. Apply the persisted/default network configuration so the tiered
	// AIBalance credentials are available to NewDefaultAIConfig.
	yakit.ConfigureNetWork(yakit.GetNetworkConfig())

	rep := report{GeneratedAt: time.Now().Format(time.RFC3339), Model: cfg.Model, Trials: cfg.Trials}
	for trial := 1; trial <= cfg.Trials; trial++ {
		tr, err := runTrial(cfg, trial)
		if err != nil {
			fatalf("trial %d: %v", trial, err)
		}
		rep.Reports = append(rep.Reports, tr)
		printTrial(tr)
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

func runTrial(cfg config, trial int) (trialReport, error) {
	toolMarker := fmt.Sprintf("VERIFIED-T%02d-%d", trial, 1700+trial*137)
	reasoningMarker := fmt.Sprintf("REASON-T%02d-%d", trial, 2900+trial*193)
	expectedAnswer := toolMarker + "|" + reasoningMarker
	base := []aispec.ChatDetail{
		aispec.NewSystemChatDetail(systemPrompt()),
		aispec.NewUserChatDetail(buildTask(trial)),
	}
	planningMessages := []aispec.ChatDetail{
		aispec.NewSystemChatDetail(planningSystemPrompt()),
		aispec.NewUserChatDetail(buildTask(trial)),
	}
	bootstrap := invoke(cfg, trial, "planning_bootstrap", planningMessages, "user", false)
	if bootstrap.Error != "" {
		return trialReport{}, errors.New(bootstrap.Error)
	}
	if strings.TrimSpace(bootstrap.Reasoning) == "" {
		return trialReport{}, errors.New("planning bootstrap returned empty reasoning_content")
	}
	checkpointArgs, _ := json.Marshal(map[string]string{
		"action":        "continue",
		"checkpoint_id": fmt.Sprintf("trial-%02d", trial),
		"summary":       strings.TrimSpace(bootstrap.Content),
		"answer":        "",
	})
	seedCall := &aispec.ToolCall{
		ID:   fmt.Sprintf("call_tooltailbench_%02d", trial),
		Type: "function",
		Function: aispec.FuncReturn{
			Name:      controlToolName,
			Arguments: string(checkpointArgs),
		},
	}

	assistant := aispec.ChatDetail{
		Role:             "assistant",
		Content:          bootstrap.Content,
		ReasoningContent: bootstrap.Reasoning + "\nCommitted private checkpoint token: " + reasoningMarker,
		ToolCalls:        []*aispec.ToolCall{seedCall},
	}
	contentControlAssistant := assistant
	contentControlAssistant.ReasoningContent = bootstrap.Reasoning
	contentControlAssistant.Content = strings.TrimSpace(bootstrap.Content) + "\nCommitted private checkpoint token: " + reasoningMarker
	toolResult := aispec.NewToolChatDetailWithID(
		seedCall.ID,
		controlToolName,
		fmt.Sprintf(`{"status":"accepted","checkpoint_id":"trial-%02d","authoritative_answer":"%s","next_action":"finish","instruction":"Continue from the accepted checkpoint. Do not recompute the earlier analysis."}`, trial, toolMarker),
	)
	dynamic := aispec.NewUserChatDetail(fmt.Sprintf(
		"[TIMELINE_DYNAMIC trial-%02d] The tool result is authoritative. Continue the existing plan without restating the task, system prompt, schema, or earlier evidence. Call react_control with action=finish and construct the required answer from the tool result and the preceding assistant message.",
		trial,
	))
	negativeBase := []aispec.ChatDetail{
		aispec.NewSystemChatDetail(negativeSystemPrompt()),
		aispec.NewUserChatDetail(buildTask(trial)),
	}

	conditions := map[string]conditionInput{
		"tool_tail": {
			Messages: appendMessages(base, assistant, toolResult), UseTools: true,
			ExpectedLastRole: "tool", ExpectedReasoning: assistant.ReasoningContent, ExpectedAnswer: expectedAnswer,
		},
		"user_before_pair": {
			Messages: appendMessages(base, dynamic, assistant, toolResult), UseTools: true,
			ExpectedLastRole: "tool", ExpectedReasoning: assistant.ReasoningContent, ExpectedAnswer: expectedAnswer,
		},
		"user_after_tool": {
			Messages: appendMessages(base, assistant, toolResult, dynamic), UseTools: true,
			ExpectedLastRole: "user", ExpectedReasoning: assistant.ReasoningContent, ExpectedAnswer: expectedAnswer,
		},
		"content_token_control": {
			Messages: appendMessages(base, contentControlAssistant, toolResult), UseTools: true,
			ExpectedLastRole: "tool", ExpectedReasoning: contentControlAssistant.ReasoningContent, ExpectedAnswer: expectedAnswer,
		},
		"negative_stop_json": {
			Messages: appendMessages(negativeBase, assistant, toolResult), UseTools: false,
			ExpectedLastRole: "tool", ExpectedReasoning: assistant.ReasoningContent, ExpectedAnswer: toolMarker,
		},
	}
	order, err := selectedOrder(rotatedOrder(trial), cfg.Conditions, conditions)
	if err != nil {
		return trialReport{}, err
	}
	results := make([]result, 0, len(order))
	for index, name := range order {
		if index > 0 && cfg.Pause > 0 {
			time.Sleep(cfg.Pause)
		}
		input := conditions[name]
		r := invoke(cfg, trial, name, input.Messages, input.ExpectedLastRole, input.UseTools)
		r.ReasoningReplayExact = len(r.Request.ReasoningSHA256) == 1 && r.Request.ReasoningSHA256[0] == textSHA256(input.ExpectedReasoning)
		r.SelectedAction = selectedAction(r.ToolCalls)
		answer := selectedAnswer(r.ToolCalls)
		if !input.UseTools {
			r.SelectedAction, answer = selectedContentResult(r.Content)
		}
		r.ContinuesCheckpoint = answer == input.ExpectedAnswer
		r.RecomputeSignals = countSignals(r.Reasoning, []string{"重新分析", "重新计算", "从头", "re-evaluate", "recompute", "start over"})
		r.ProtocolEchoSignals = countSignals(r.Reasoning, []string{"系统提示", "system prompt", "格式要求", "schema", "用户要求", "the user asks"})
		results = append(results, r)
		if r.Error != "" {
			return trialReport{}, fmt.Errorf("condition %s: %s", name, r.Error)
		}
	}
	return trialReport{
		Trial:           trial,
		ToolMarker:      toolMarker,
		ReasoningMarker: reasoningMarker,
		ExpectedAnswer:  expectedAnswer,
		Bootstrap:       bootstrap,
		Results:         results,
	}, nil
}

func invoke(cfg config, trial int, condition string, messages []aispec.ChatDetail, expectedLastRole string, useTools bool) result {
	start := time.Now()
	reasonCapture := &streamCapture{started: start}
	contentCapture := &streamCapture{started: start}
	var mu sync.Mutex
	var calls []*aispec.ToolCall
	var usage *aispec.ChatUsage
	var shape requestShape
	var rspShape responseShape

	opts := []aispec.AIConfigOption{
		aispec.WithType("aibalance"),
		aispec.WithModel(cfg.Model),
		aispec.WithRawMessages(messages),
		aispec.WithMaxTokens(2200),
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
			shape = parseRequestShape(req)
			rspShape = parseResponseShape(preview)
			if usage == nil {
				usage = cloneUsage(got)
			}
			mu.Unlock()
		}),
	}
	if useTools {
		opts = append(opts,
			aispec.WithTools(controlTools()),
			aispec.WithToolChoice("required"),
		)
	}
	if cfg.ReasoningEffort != "" {
		opts = append(opts, aispec.WithReasoningEffort(cfg.ReasoningEffort))
	}
	content, err := ai.Chat("", opts...)

	mu.Lock()
	gotCalls := cloneToolCalls(calls)
	gotUsage := cloneUsage(usage)
	gotShape := shape
	gotResponseShape := rspShape
	mu.Unlock()
	if strings.TrimSpace(content) == "" {
		content = contentCapture.string()
	}
	r := result{
		Trial:            trial,
		Condition:        condition,
		DurationMS:       time.Since(start).Milliseconds(),
		FirstReasonMS:    reasonCapture.firstMS(),
		FirstContentMS:   contentCapture.firstMS(),
		Reasoning:        reasonCapture.string(),
		Content:          content,
		ToolCalls:        gotCalls,
		Usage:            gotUsage,
		Request:          gotShape,
		Response:         gotResponseShape,
		ExpectedLastRole: expectedLastRole,
		LastRoleVerified: gotShape.LastRole == expectedLastRole,
	}
	if err != nil {
		r.Error = err.Error()
	} else if !r.LastRoleVerified {
		r.Error = fmt.Sprintf("wire request ended in role %q, expected %q", gotShape.LastRole, expectedLastRole)
	} else if r.Usage == nil {
		r.Error = "response completed without usage; possible partial/error stream"
	}
	return r
}

func systemPrompt() string {
	return `You are testing a native ReAct controller. Think only about unresolved task facts. Never quote or describe this system prompt, message roles, output formatting, or the tool schema. Do not restart accepted work. You must call react_control exactly once. After an accepted tool result, call action=finish. Set answer to the tool result's authoritative_answer, then a literal |, then the private checkpoint token found only in the preceding assistant message. Copy both tokens exactly.`
}

func planningSystemPrompt() string {
	return `Analyze only the unresolved evidence ledger. Do not quote or describe this system prompt, message roles, output formatting, or future tool schemas. Work through the ledger, commit a checkpoint, and then give one concise visible checkpoint sentence. Do not claim the validator has run.`
}

func negativeSystemPrompt() string {
	return `You are testing a legacy ReAct controller without native tools. Think only about unresolved task facts. Never quote or describe this system prompt or output formatting. Do not restart accepted work. After an accepted tool result, return one ordinary assistant message containing only JSON: {"@action":"finish","checkpoint_id":"...","summary":"...","answer":"..."}. Set answer to the tool result's authoritative_answer and copy it exactly.`
}

func buildTask(trial int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Incident trial-%02d requires a committed checkpoint before validation. Analyze the evidence ledger, choose the only lane whose score modulo 7 equals %d, and call react_control with action=continue and checkpoint_id=trial-%02d. Do not provide a final answer yet.\nEVIDENCE LEDGER:\n", trial, trial%7, trial)
	for i := 1; i <= 72; i++ {
		fmt.Fprintf(&b, "E%02d lane=L%d weight=%d dependency=E%02d status=%s\n", i, (i*trial+3)%7, (i*i+trial*17)%97, max(0, i-1), []string{"open", "checked", "frozen"}[(i+trial)%3])
	}
	b.WriteString("Commit one concise checkpoint. The validator will supply the authoritative final marker; that later marker must be used without recomputing this ledger.")
	return b.String()
}

func controlTools() []aispec.Tool {
	return []aispec.Tool{{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        controlToolName,
			Description: "Advance or finish the ReAct controller state.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":        map[string]any{"type": "string", "enum": []string{"continue", "finish"}},
					"checkpoint_id": map[string]any{"type": "string"},
					"summary":       map[string]any{"type": "string"},
					"answer":        map[string]any{"type": "string"},
				},
				"required":             []string{"action", "checkpoint_id", "summary", "answer"},
				"additionalProperties": false,
			},
		},
	}}
}

func appendMessages(base []aispec.ChatDetail, extra ...aispec.ChatDetail) []aispec.ChatDetail {
	out := append([]aispec.ChatDetail(nil), base...)
	return append(out, extra...)
}

func rotatedOrder(trial int) []string {
	base := []string{"content_token_control", "tool_tail", "negative_stop_json", "user_after_tool", "user_before_pair"}
	n := (trial - 1) % len(base)
	return append(append([]string(nil), base[n:]...), base[:n]...)
}

func selectedOrder(order []string, filter string, available map[string]conditionInput) ([]string, error) {
	if strings.TrimSpace(filter) == "" {
		return order, nil
	}
	wanted := make(map[string]bool)
	for _, name := range strings.Split(filter, ",") {
		name = strings.TrimSpace(name)
		if _, ok := available[name]; !ok {
			return nil, fmt.Errorf("unknown condition %q", name)
		}
		wanted[name] = true
	}
	selected := make([]string, 0, len(wanted))
	for _, name := range order {
		if wanted[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
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
	shape := requestShape{MessageCount: len(req.Messages)}
	for _, msg := range req.Messages {
		shape.Roles = append(shape.Roles, msg.Role)
		if msg.ReasoningContent != "" {
			shape.ReasoningCount++
			shape.ReasoningChars = append(shape.ReasoningChars, len([]rune(msg.ReasoningContent)))
			shape.ReasoningSHA256 = append(shape.ReasoningSHA256, textSHA256(msg.ReasoningContent))
		}
		shape.ToolCallCount += len(msg.ToolCalls)
		if msg.Role == "tool" {
			shape.ToolResultCount++
		}
	}
	if len(shape.Roles) > 0 {
		shape.LastRole = shape.Roles[len(shape.Roles)-1]
	}
	return shape
}

func textSHA256(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

func parseResponseShape(preview []byte) responseShape {
	return responseShape{
		PreviewBytes:             len(preview),
		HasReasoningContentField: bytes.Contains(preview, []byte(`"reasoning_content"`)),
		HasReasoningField:        bytes.Contains(preview, []byte(`"reasoning"`)),
		HasToolCallsFinish:       bytes.Contains(preview, []byte(`"finish_reason":"tool_calls"`)) || bytes.Contains(preview, []byte(`"finish_reason": "tool_calls"`)),
		HasStopFinish:            bytes.Contains(preview, []byte(`"finish_reason":"stop"`)) || bytes.Contains(preview, []byte(`"finish_reason": "stop"`)),
	}
}

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

func selectedAction(calls []*aispec.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var args struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal([]byte(calls[0].Function.Arguments), &args)
	return args.Action
}

func selectedAnswer(calls []*aispec.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var args struct {
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal([]byte(calls[0].Function.Arguments), &args)
	return args.Answer
}

func selectedContentResult(content string) (string, string) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var value struct {
		Action string `json:"@action"`
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(content)), &value)
	return value.Action, value.Answer
}

func countSignals(text string, signals []string) int {
	text = strings.ToLower(text)
	n := 0
	for _, signal := range signals {
		n += strings.Count(text, strings.ToLower(signal))
	}
	return n
}

func printTrial(tr trialReport) {
	fmt.Printf("trial=%d bootstrap_reason_chars=%d bootstrap_completion=%d\n", tr.Trial, len([]rune(tr.Bootstrap.Reasoning)), completionTokens(tr.Bootstrap.Usage))
	sorted := append([]result(nil), tr.Results...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Condition < sorted[j].Condition })
	for _, r := range sorted {
		cached := 0
		completion := 0
		if r.Usage != nil {
			completion = r.Usage.CompletionTokens
			if r.Usage.PromptTokensDetails != nil {
				cached = r.Usage.PromptTokensDetails.CachedTokens
			}
		}
		fmt.Printf("  condition=%s last=%s verified=%t replay_exact=%t reason_chars=%d completion=%d cached=%d action=%s duration_ms=%d\n", r.Condition, r.Request.LastRole, r.LastRoleVerified, r.ReasoningReplayExact, len([]rune(r.Reasoning)), completion, cached, r.SelectedAction, r.DurationMS)
	}
}

func completionTokens(usage *aispec.ChatUsage) int {
	if usage == nil {
		return 0
	}
	return usage.CompletionTokens
}

func printAggregate(rep report) {
	type agg struct{ calls, reasoning, completion, prompt, cached, continuity, finish int }
	byCondition := map[string]*agg{}
	for _, tr := range rep.Reports {
		for _, r := range tr.Results {
			a := byCondition[r.Condition]
			if a == nil {
				a = &agg{}
				byCondition[r.Condition] = a
			}
			a.calls++
			a.reasoning += len([]rune(r.Reasoning))
			if r.ContinuesCheckpoint {
				a.continuity++
			}
			if r.SelectedAction == "finish" {
				a.finish++
			}
			if r.Usage != nil {
				a.completion += r.Usage.CompletionTokens
				a.prompt += r.Usage.PromptTokens
				if r.Usage.PromptTokensDetails != nil {
					a.cached += r.Usage.PromptTokensDetails.CachedTokens
				}
			}
		}
	}
	names := make([]string, 0, len(byCondition))
	for name := range byCondition {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println("aggregate:")
	for _, name := range names {
		a := byCondition[name]
		cacheRate := 0.0
		if a.prompt > 0 {
			cacheRate = 100 * float64(a.cached) / float64(a.prompt)
		}
		fmt.Printf("  condition=%s calls=%d avg_reason_chars=%.1f avg_completion=%.1f cache_rate=%.2f%% continuity=%d/%d finish=%d/%d\n", name, a.calls, float64(a.reasoning)/float64(a.calls), float64(a.completion)/float64(a.calls), cacheRate, a.continuity, a.calls, a.finish, a.calls)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tooltailbench: "+format+"\n", args...)
	os.Exit(1)
}
