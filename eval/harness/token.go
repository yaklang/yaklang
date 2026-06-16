package harness

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"unicode"
	"unicode/utf8"
)

// EstimateTokens provides a rough token count estimate for mixed-language text.
// It is intended for cost/usage reporting when the upstream LLM does not expose
// precise usage metadata through the event stream.
//
// Heuristic:
//   - CJK characters: ~1.5 chars per token
//   - Other runes (Latin, digits, symbols, whitespace): ~4 chars per token
func EstimateTokens(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	total := 0.0
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			// invalid UTF-8 byte, count as 1/4 token
			total += 0.25
			data = data[1:]
			continue
		}
		if isCJK(r) {
			total += 0.67
		} else {
			total += 0.25
		}
		data = data[size:]
	}
	if total < 1 {
		return 1
	}
	return int(total)
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		(unicode.Is(unicode.Hangul, r) && r >= 0xAC00)
}

// TokenUsage tracks estimated input/output token consumption for a task.
type TokenUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

// Total returns the sum of input and output tokens.
func (u TokenUsage) Total() int {
	return u.InputTokens + u.OutputTokens
}

// EstimateEventTokens classifies an AI event as input or output and estimates
// its token contribution. The classification is heuristic:
//   - Output: thought, stream-thought, result, call_tool, plan
//   - Input: observation, tool_stdout, tool_stderr, memory_retrieval
//   - Other: counted as output (model-generated messages)
func EstimateEventTokens(content, streamDelta []byte, eventType, nodeID string) TokenUsage {
	total := EstimateTokens(content) + EstimateTokens(streamDelta)
	if total == 0 {
		return TokenUsage{}
	}
	switch eventType {
	case "observation", "tool_stdout", "tool_stderr", "memory_retrieval", "memory_search":
		return TokenUsage{InputTokens: total, TotalTokens: total}
	case "thought", "call_tool", "stream":
		return TokenUsage{OutputTokens: total, TotalTokens: total}
	case "structured":
		// Result/final answer is model output; other structured events (tool results) are input.
		if nodeID == "result" || nodeID == "final" {
			return TokenUsage{OutputTokens: total, TotalTokens: total}
		}
		return TokenUsage{InputTokens: total, TotalTokens: total}
	default:
		return TokenUsage{OutputTokens: total, TotalTokens: total}
	}
}

// ParseConsumptionEvent extracts token usage from aibalance "consumption" events.
// These events contain authoritative (upstream-reported) input/output consumption.
// Returns (0,0,false) if the event is not a consumption event or cannot be parsed.
func ParseConsumptionEvent(e *ypb.AIOutputEvent) (inputTokens, outputTokens int, ok bool) {
	if e == nil || e.Type != "consumption" || len(e.Content) == 0 {
		return 0, 0, false
	}
	inputTokens = int(gjson.GetBytes(e.Content, "input_consumption").Int())
	outputTokens = int(gjson.GetBytes(e.Content, "output_consumption").Int())
	if inputTokens == 0 && outputTokens == 0 {
		return 0, 0, false
	}
	return inputTokens, outputTokens, true
}

// MarshalJSON implements custom JSON serialization for TokenUsage so that
// zero values render clearly instead of being omitted.
func (u TokenUsage) MarshalJSON() ([]byte, error) {
	type alias TokenUsage
	return json.Marshal(&struct {
		alias
	}{
		alias: alias(u),
	})
}
