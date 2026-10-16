package aispec

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrPromptOverflow means that the provider rejected the request's context
// budget. Replaying the same prompt cannot repair this error.
var ErrPromptOverflow = errors.New("ai prompt overflow")

type PromptOverflowConfidence uint8

const (
	PromptOverflowLow PromptOverflowConfidence = iota
	PromptOverflowMedium
	PromptOverflowHigh
)

type PromptOverflowSource string

const (
	PromptOverflowSourceHTTP      PromptOverflowSource = "http"
	PromptOverflowSourceJSON      PromptOverflowSource = "json"
	PromptOverflowSourceSSE       PromptOverflowSource = "sse"
	PromptOverflowSourceCallback  PromptOverflowSource = "callback"
	PromptOverflowSourceAuxiliary PromptOverflowSource = "auxiliary"
)

// PromptOverflowAmbiguity is a provider error that is neither a confirmed
// overflow nor a known unrelated failure. It contains only redacted error
// fields and never the original prompt.
type PromptOverflowAmbiguity struct {
	HTTPStatus int
	Code       string
	Type       string
	Message    string
	Raw        string
}

// PromptOverflowEvidence contains provider facts, not local token estimates.
// Unknown identities and token counts are left empty/zero.
type PromptOverflowEvidence struct {
	HTTPStatus       int
	Provider         string
	Model            string
	Code             string
	Type             string
	Param            string
	Message          string
	UpstreamMessage  string
	MaxContextTokens int64
	InputTokens      int64 // Prompt/input tokens, excluding the output budget.
	RequestTokens    int64 // Total requested tokens, if explicitly reported.
	MatchedPattern   string
	Confidence       PromptOverflowConfidence
	Source           PromptOverflowSource
	Raw              string // Sanitized error-field excerpt, at most 512 bytes.
}

// PromptOverflowError preserves both the classification and the original error.
// Evidence is a value so callers can enrich a copy without mutating a shared
// callback error.
type PromptOverflowError struct {
	PromptOverflowEvidence
	Cause error
}

func (e *PromptOverflowError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "ai prompt overflow: " + e.Message
}

func (e *PromptOverflowError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *PromptOverflowError) Is(target error) bool { return e != nil && target == ErrPromptOverflow }

type providerErrorFields struct {
	Code     string
	Type     string
	Param    string
	Message  string
	Upstream string
}

func providerErrorString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// extractProviderError inspects only protocol error envelopes, never recursive
// $..error matches in tool arguments or generated model content. allowFlat is
// reserved for rejected HTTP requests (some gateways return just code/message).
func extractProviderError(body []byte, allowFlat bool) (providerErrorFields, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || obj == nil {
		return providerErrorFields{}, false
	}
	eventType := providerErrorString(obj["type"])
	if eventType == "response.failed" {
		var response map[string]json.RawMessage
		if json.Unmarshal(obj["response"], &response) == nil && response != nil {
			obj = response
		}
	} else if eventType != "error" && strings.HasPrefix(eventType, "response.") {
		return providerErrorFields{}, false
	}

	rawError, hasError := obj["error"]
	hasError = hasError && len(rawError) > 0 && strings.TrimSpace(string(rawError)) != "null"
	if hasError {
		var fields map[string]json.RawMessage
		if json.Unmarshal(rawError, &fields) == nil && fields != nil {
			obj = fields
		} else if message := providerErrorString(rawError); message != "" {
			return providerErrorFields{Message: message}, true
		} else {
			return providerErrorFields{}, false
		}
	} else if eventType != "error" && eventType != "response.failed" &&
		providerErrorString(obj["status"]) != "failed" {
		if !allowFlat || obj["choices"] != nil || obj["output"] != nil || obj["code"] == nil && obj["message"] == nil {
			return providerErrorFields{}, false
		}
	}

	fields := providerErrorFields{
		Code:     providerErrorString(obj["code"]),
		Type:     providerErrorString(obj["type"]),
		Param:    providerErrorString(obj["param"]),
		Message:  providerErrorString(obj["message"]),
		Upstream: providerErrorString(obj["upstream_last_error"]),
	}
	// Gateways may retain a structured upstream error instead of its text.
	if fields.Upstream == "" && len(obj["upstream_last_error"]) > 0 {
		fields.Upstream = string(obj["upstream_last_error"])
	}
	return fields, hasError || eventType == "error" || eventType == "response.failed" ||
		providerErrorString(obj["status"]) == "failed" || fields.Message != ""
}

var promptOverflowPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"prompt_too_long", regexp.MustCompile(`(?i)\b(?:prompt|input(?: text)?)\s+(?:is\s+)?too long\b`)},
	{"exceeds_context", regexp.MustCompile(`(?i)\b(?:input|prompt|messages?|request(?:ed)?)(?:\s+[\w'()-]+){0,10}\s+(?:exceeds?|exceeded)\s+(?:the\s+)?(?:model'?s?\s+)?(?:maximum\s+)?context(?:\s+(?:window|length|limit))?\b`)},
	{"context_exceeded", regexp.MustCompile(`(?i)\bcontext (?:length|window|limit)\s+(?:has been\s+|is\s+)?exceeded\b`)},
	{"chinese_prompt_too_long", regexp.MustCompile(`(?i)(?:prompt|输入|提示词)\s*(?:超长|过长)`)},
	{"chinese_context_exceeded", regexp.MustCompile(`(?:输入|提示词|上下文)(?:长度|窗口|token数|tokens)?\s*(?:超过|超出|超限)(?:.{0,20})(?:上下文|窗口|token|长度限制|最大长度)|上下文(?:长度|窗口)(?:已)?超限`)},
}

var (
	promptOverflowMaxTokens = regexp.MustCompile(`(?i)maximum context (?:length|window)(?:\s+(?:is|of))?\s*[:=]?\s*([0-9][0-9,]*)\s*tokens?`)
	promptOverflowRequested = regexp.MustCompile(`(?i)(?:you requested|requested a total of)\s+([0-9][0-9,]*)\s*tokens?`)
	promptOverflowInput     = regexp.MustCompile(`(?i)(?:messages resulted in|prompt is too long:|input(?: length)?(?: is|:))\s*([0-9][0-9,]*)\s*tokens?`)
	promptOverflowMessages  = regexp.MustCompile(`(?i)([0-9][0-9,]*)\s+(?:tokens?\s+)?in the messages`)
	promptOverflowCompare   = regexp.MustCompile(`(?i)([0-9][0-9,]*)\s*tokens?\s*>\s*([0-9][0-9,]*)\s*maximum`)
	promptOverflowNegative  = regexp.MustCompile(`(?i)rate[_ -]?limit|insufficient[_ ]quota|quota[_ ](?:exceeded|exhausted)|invalid[_ ]api[_ -]?key|unauthorized|permission[_ ](?:denied|error)|authentication[_ ](?:failed|error)|content[_ -]?filter|content[_ ]policy|safety[_ ](?:violation|error)|余额不足|额度(?:不足|耗尽)|频率限制|审核(?:拒绝|不通过)`)
	promptOverflowBadParam  = regexp.MustCompile(`(?i)\bmax_(?:completion_)?tokens\b.{0,40}(?:is too large|is invalid|invalid value|must be|must not|out of range)|(?:invalid|unsupported)\s+(?:value for\s+)?['"]?max_(?:completion_)?tokens\b`)
	promptOverflowSecrets   = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]+|\bbearer\s+[^\s"\\]+|(?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*["']?[^\s"',;}{\\]+`)
)

func promptOverflowTokenCount(s string) int64 {
	n, err := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func promptOverflowExtractTokens(ev *PromptOverflowEvidence, message string) {
	extract := func(re *regexp.Regexp) int64 {
		if match := re.FindStringSubmatch(message); len(match) > 1 {
			return promptOverflowTokenCount(match[1])
		}
		return 0
	}
	ev.MaxContextTokens = extract(promptOverflowMaxTokens)
	ev.RequestTokens = extract(promptOverflowRequested)
	ev.InputTokens = extract(promptOverflowInput)
	if ev.InputTokens == 0 {
		ev.InputTokens = extract(promptOverflowMessages)
	}
	if match := promptOverflowCompare.FindStringSubmatch(message); len(match) > 2 {
		ev.InputTokens = promptOverflowTokenCount(match[1])
		ev.MaxContextTokens = promptOverflowTokenCount(match[2])
	}
}

func promptOverflowExcerpt(fields providerErrorFields, upstreamMessage string) string {
	// Deliberately omit arbitrary response fields (API keys, echoed prompts,
	// metadata, etc.). Message stays unmodified in Evidence.Message.
	raw, _ := json.Marshal(struct {
		Code            string `json:"code,omitempty"`
		Type            string `json:"type,omitempty"`
		Message         string `json:"message,omitempty"`
		UpstreamMessage string `json:"upstream_message,omitempty"`
	}{fields.Code, fields.Type, promptOverflowSecrets.ReplaceAllString(fields.Message, "[REDACTED]"),
		promptOverflowSecrets.ReplaceAllString(upstreamMessage, "[REDACTED]")})
	s := string(raw)
	if len(s) > 512 {
		s = s[:512]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// DetectPromptOverflow is conservative: a rejected HTTP request, a protocol
// error envelope, or an actual callback error is required. errText must be an
// error, never generated output. Status alone, local size estimates, vague
// "context length" text, and unverified numeric provider codes are insufficient.
// Only confirmed (High) evidence is returned; nil means "not confirmed".
func DetectPromptOverflow(status int, body []byte, errText string) *PromptOverflowEvidence {
	return detectPromptOverflow(status, body, errText, 0)
}

func detectPromptOverflow(status int, body []byte, errText string, depth int) *PromptOverflowEvidence {
	rejected := status == 400 || status == 413 || status == 422
	if !rejected && status != 0 && (status < 200 || status >= 300) {
		return nil
	}
	fields, envelope := extractProviderError(body, rejected)
	source := PromptOverflowSourceJSON
	if !envelope {
		// This also supports custom callbacks that retain an SSE error preview
		// instead of going through the aispec stream decoder.
		if depth < 3 {
			for _, line := range strings.Split(string(body), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "data:") {
					payload := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
					if _, ok := extractProviderError(payload, false); ok {
						ev := detectPromptOverflow(status, payload, "", depth+1)
						if ev != nil {
							ev.Source = PromptOverflowSourceSSE
						}
						return ev
					}
				}
			}
		}
		// A valid non-error JSON payload must not be searched for quoted error
		// examples. Truncated/non-JSON HTTP error bodies can still carry facts.
		if rejected && len(body) > 0 && !json.Valid(body) {
			fields.Message = string(body)
		} else if errText != "" {
			fields.Message = errText
			source = PromptOverflowSourceCallback
		} else {
			return nil
		}
	}
	if rejected {
		source = PromptOverflowSourceHTTP
	}
	if promptOverflowNegative.MatchString(fields.Code + " " + fields.Type + " " + fields.Message) {
		return nil
	}

	ev := &PromptOverflowEvidence{
		HTTPStatus: status,
		Code:       fields.Code,
		Type:       fields.Type,
		Param:      fields.Param,
		Message:    fields.Message,
		Confidence: PromptOverflowHigh,
		Source:     source,
	}
	for _, code := range []string{fields.Code, fields.Type} {
		switch strings.ToLower(code) {
		case "context_length_exceeded", "context_window_exceeded", "prompt_too_long", "input_too_long":
			ev.MatchedPattern = "code:" + strings.ToLower(code)
		}
	}
	if ev.MatchedPattern == "" {
		if promptOverflowBadParam.MatchString(fields.Message) {
			return nil
		}
		switch strings.ToLower(fields.Param) {
		case "temperature", "top_p", "tools", "tool_choice", "response_format":
			return nil
		}
		for _, pattern := range promptOverflowPatterns {
			if pattern.re.MatchString(fields.Message) {
				ev.MatchedPattern = pattern.name
				break
			}
		}
	}
	promptOverflowExtractTokens(ev, fields.Message)
	if ev.MatchedPattern == "" && ev.MaxContextTokens > 0 &&
		(ev.InputTokens > ev.MaxContextTokens || ev.RequestTokens > ev.MaxContextTokens) {
		ev.MatchedPattern = "reported_tokens_exceed_context"
	}
	if ev.MatchedPattern == "" && fields.Upstream != "" && depth < 3 {
		// Inspect only this explicitly identified upstream error, not the whole
		// gateway response. Keep the outer provider message and code intact.
		upstream := detectPromptOverflow(400, []byte(fields.Upstream), "", depth+1)
		if upstream != nil {
			ev.MatchedPattern = "upstream:" + upstream.MatchedPattern
			ev.UpstreamMessage = upstream.Message
			ev.MaxContextTokens = upstream.MaxContextTokens
			ev.InputTokens = upstream.InputTokens
			ev.RequestTokens = upstream.RequestTokens
		}
	}
	if ev.MatchedPattern == "" {
		return nil
	}
	ev.Raw = promptOverflowExcerpt(fields, ev.UpstreamMessage)
	return ev
}

// DetectPromptOverflowAmbiguity reports a provider error that deterministic
// rules cannot classify. Confirmed overflow, negative matches, transport
// failures, and ordinary model output return nil. errText is only used to
// avoid treating an already confirmed overflow as ambiguous.
func DetectPromptOverflowAmbiguity(status int, body []byte, errText string) *PromptOverflowAmbiguity {
	if DetectPromptOverflow(status, body, errText) != nil {
		return nil
	}
	return promptOverflowAmbiguity(status, body, 0)
}

func promptOverflowAmbiguity(status int, body []byte, depth int) *PromptOverflowAmbiguity {
	rejected := status == 400 || status == 413 || status == 422
	if !rejected && status != 0 && (status < 200 || status >= 300) {
		return nil
	}
	fields, envelope := extractProviderError(body, rejected)
	if !envelope && depth < 3 {
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if sample := promptOverflowAmbiguity(status, payload, depth+1); sample != nil {
				return sample
			}
		}
	}
	if !envelope {
		if !rejected || len(body) == 0 || json.Valid(body) {
			return nil
		}
		fields.Message = string(body)
	}
	combined := fields.Code + " " + fields.Type + " " + fields.Message
	if promptOverflowNegative.MatchString(combined) || promptOverflowBadParam.MatchString(fields.Message) {
		return nil
	}
	switch strings.ToLower(fields.Param) {
	case "temperature", "top_p", "tools", "tool_choice", "response_format":
		return nil
	}
	fields.Message = promptOverflowSecrets.ReplaceAllString(strings.TrimSpace(fields.Message), "[REDACTED]")
	fields.Message = clipOverflowRunes(fields.Message, 800)
	if fields.Message == "" && strings.TrimSpace(fields.Code) == "" {
		return nil
	}
	return &PromptOverflowAmbiguity{
		HTTPStatus: status,
		Code:       fields.Code,
		Type:       fields.Type,
		Message:    fields.Message,
		Raw:        promptOverflowExcerpt(fields, ""),
	}
}

func clipOverflowRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	end := 0
	for runes := 0; end < len(s) && runes < maxRunes; runes++ {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}
	return s[:end]
}

// promptOverflowAfterStream classifies a completed response body once.
// Streaming parsers must not call it for individual events.
func promptOverflowAfterStream(status int, body []byte) error {
	evidence := DetectPromptOverflow(status, body, "")
	if evidence == nil {
		return nil
	}
	message := evidence.Message
	if message == "" {
		message = evidence.Code
	}
	if message == "" {
		message = "prompt overflow"
	}
	return &PromptOverflowError{
		PromptOverflowEvidence: *evidence,
		Cause:                  fmt.Errorf("AI provider returned %s error: %s", evidence.Source, message),
	}
}
