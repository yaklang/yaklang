package aispec

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// Synthetic provider-format fixtures: these test the classifier contract, not
// live provider compatibility. Actual captured fixtures can extend this table.
func TestDetectPromptOverflow(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		errText   string
		code      string
		maxTokens int64
		input     int64
		requested int64
	}{
		{"openai/code", 400, `{"error":{"code":"context_length_exceeded","type":"invalid_request_error","param":"messages","message":"This model's maximum context length is 128000 tokens. However, your messages resulted in 145000 tokens."}}`, "", "context_length_exceeded", 128000, 145000, 0},
		{"openai/code_only", 400, `{"error":{"code":"context_length_exceeded"}}`, "", "context_length_exceeded", 0, 0, 0},
		{"openai/responses", 400, `{"error":{"type":"invalid_request_error","message":"Your input exceeds the context window of this model."}}`, "", "", 0, 0, 0},
		{"deepseek", 400, `{"error":{"code":"invalid_request_error","message":"This model's maximum context length is 65536 tokens. However, you requested 87731 tokens (85507 in the messages, 2224 in the completion). Please reduce the length of the messages or completion."}}`, "", "invalid_request_error", 65536, 85507, 87731},
		{"anthropic", 400, `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 214315 tokens > 204798 maximum"}}`, "", "", 204798, 214315, 0},
		{"anthropic/budget", 400, `{"error":{"message":"input length and max_tokens exceed context limit: 200000"}}`, "", "", 0, 0, 0},
		{"glm/chinese", 400, `{"error":{"code":"1261","message":"Prompt 超长"}}`, "", "1261", 0, 0, 0},
		{"glm/english", 400, `{"error":{"code":1261,"message":"Prompt is too long"}}`, "", "1261", 0, 0, 0},
		{"chinese/input", 400, `{"error":{"message":"输入超长"}}`, "", "", 0, 0, 0},
		{"chinese/context", 400, `{"error":{"message":"输入长度超出模型上下文窗口限制"}}`, "", "", 0, 0, 0},
		{"gateway/plain_413", 413, "prompt is too long", "", "", 0, 0, 0},
		{"gateway/flat_422", 422, `{"code":"validation_error","message":"The input length exceeds the context length"}`, "", "validation_error", 0, 0, 0},
		{"gateway/upstream_text", 400, `{"error":{"message":"All upstream requests failed","upstream_last_error":"prompt is too long: 12345 tokens > 8192 maximum"}}`, "", "", 8192, 12345, 0},
		{"gateway/upstream_json", 400, `{"error":{"message":"Upstream failed","upstream_last_error":{"error":{"code":"context_length_exceeded","message":"too many tokens"}}}}`, "", "", 0, 0, 0},
		{"success_status/error_envelope", 200, `{"error":{"code":"context_length_exceeded","message":"Context rejected"}}`, "", "context_length_exceeded", 0, 0, 0},
		{"success_status/responses_failed", 200, `{"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"Context rejected"}}}`, "", "context_length_exceeded", 0, 0, 0},
		{"success_status/flat_error_event", 200, `{"type":"error","code":"context_length_exceeded","message":"Context rejected"}`, "", "context_length_exceeded", 0, 0, 0},
		{"success_status/failed_json", 200, `{"status":"failed","error":{"message":"prompt is too long"}}`, "", "", 0, 0, 0},
		{"callback/error_only", 0, "", "request post failed: prompt is too long", "", 0, 0, 0},
		{"callback/error_with_status", 200, "", "input length and max_tokens exceed context limit", "", 0, 0, 0},
		{"thousands_separators", 400, `{"error":{"message":"prompt is too long: 214,315 tokens > 204,798 maximum"}}`, "", "", 204798, 214315, 0},
		{"truncated_http_error", 400, `{"error":{"message":"prompt is too long ` + strings.Repeat("x", 4096), "", "", 0, 0, 0},
		{"sse_preview", 200, "data: " + `{"error":{"code":"context_length_exceeded"}}` + "\n\n", "", "context_length_exceeded", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := DetectPromptOverflow(tt.status, []byte(tt.body), tt.errText)
			require.NotNil(t, ev)
			require.Equal(t, tt.status, ev.HTTPStatus)
			require.Equal(t, PromptOverflowHigh, ev.Confidence)
			require.Equal(t, tt.code, ev.Code)
			require.Equal(t, tt.maxTokens, ev.MaxContextTokens)
			require.Equal(t, tt.input, ev.InputTokens)
			require.Equal(t, tt.requested, ev.RequestTokens)
			require.NotEmpty(t, ev.MatchedPattern)
			require.LessOrEqual(t, len(ev.Raw), 512)
			require.True(t, utf8.ValidString(ev.Raw))
		})
	}
}

func TestDetectPromptOverflowRejectsUnrelatedErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		errText string
	}{
		{"429", 429, `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`, ""},
		{"rate_limit", 400, `{"error":{"code":"rate_limit_exceeded","message":"Rate limit: prompt is too long"}}`, ""},
		{"quota", 400, `{"error":{"code":"insufficient_quota","message":"prompt is too long"}}`, ""},
		{"balance", 400, `{"error":{"message":"余额不足，输入超长"}}`, ""},
		{"authentication", 401, `{"error":{"message":"prompt is too long"}}`, ""},
		{"permission", 403, `{"error":{"code":"context_length_exceeded"}}`, ""},
		{"server_error", 500, `{"error":{"message":"prompt is too long"}}`, ""},
		{"auth_message", 400, `{"error":{"message":"invalid api key; prompt is too long"}}`, ""},
		{"auth_code", 200, `{"error":{"code":"invalid_api_key","message":"prompt is too long"}}`, ""},
		{"permission_type", 200, `{"error":{"type":"permission_error","message":"prompt is too long"}}`, ""},
		{"policy_code", 200, `{"error":{"code":"content_policy_violation","message":"prompt is too long"}}`, ""},
		{"max_tokens", 400, `{"error":{"param":"max_tokens","message":"max_tokens is too large. This model's maximum context length is 128000 tokens."}}`, ""},
		{"invalid_temperature", 400, `{"error":{"param":"temperature","message":"Invalid temperature; prompt is too long"}}`, ""},
		{"invalid_top_p", 400, `{"error":{"param":"top_p","message":"Context length must be positive"}}`, ""},
		{"invalid_role", 400, `{"error":{"param":"messages[0].role","message":"messages role invalid"}}`, ""},
		{"invalid_tool", 400, `{"error":{"param":"tools","message":"tool arguments invalid: prompt is too long"}}`, ""},
		{"content_filter", 400, `{"error":{"code":"content_filter","message":"prompt is too long"}}`, ""},
		{"content_policy", 400, `{"error":{"message":"content policy violation; prompt is too long"}}`, ""},
		{"glm_other", 400, `{"error":{"code":"1210","message":"参数错误"}}`, ""},
		{"unverified_numeric_code", 400, `{"error":{"code":"1261"}}`, ""},
		{"context_mention", 400, `{"error":{"message":"The context length setting is invalid"}}`, ""},
		{"reduce_mention", 400, `{"error":{"message":"请减少请求频率"}}`, ""},
		{"status_only", 413, "Request Entity Too Large", ""},
		{"bodyless", 400, "", ""},
		{"model_text", 200, `{"choices":[{"message":{"content":"prompt is too long: 214315 tokens > 204798 maximum"}}]}`, ""},
		{"model_json", 200, `{"choices":[{"message":{"content":{"error":{"code":"context_length_exceeded"}}}}]}`, ""},
		{"model_plain", 200, "prompt is too long", ""},
		{"flat_success", 200, `{"message":"prompt is too long"}`, ""},
		{"output_length", 200, `{"choices":[{"finish_reason":"length","message":{"content":""}}]}`, ""},
		{"responses_incomplete", 200, `{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`, ""},
		{"responses_content", 200, `{"type":"response.output_text.delta","delta":"prompt is too long"}`, ""},
		{"http_echo", 400, `{"messages":[{"content":"prompt is too long"}]}`, ""},
		{"unrelated_callback", 0, "", "context deadline exceeded"},
		{"gateway_negative_wins", 400, `{"error":{"code":"insufficient_quota","upstream_last_error":"prompt is too long"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Nil(t, DetectPromptOverflow(tt.status, []byte(tt.body), tt.errText))
		})
	}
}

func TestPromptOverflowErrorPreservesChain(t *testing.T) {
	original := errors.New("provider rejected prompt")
	overflow := &PromptOverflowError{
		PromptOverflowEvidence: PromptOverflowEvidence{Message: "prompt is too long"},
		Cause:                  original,
	}
	wrapped := utils.Wrapf(fmt.Errorf("callback: %w", overflow), "transaction failed")
	require.ErrorIs(t, wrapped, ErrPromptOverflow)
	require.ErrorIs(t, wrapped, original)
	var got *PromptOverflowError
	require.ErrorAs(t, wrapped, &got)
	require.Same(t, overflow, got)
	require.Contains(t, wrapped.Error(), original.Error())
}

func TestPromptOverflowEvidenceSanitizedAndBounded(t *testing.T) {
	body := `{"error":{"message":"prompt is too long; bearer sensitive-token sk-test-secret api_key=private-key ` +
		strings.Repeat("中文", 512) + `","secret":"must-not-be-copied"}}`
	ev := DetectPromptOverflow(400, []byte(body), "")
	require.NotNil(t, ev)
	require.LessOrEqual(t, len(ev.Raw), 512)
	require.True(t, utf8.ValidString(ev.Raw))
	for _, secret := range []string{"sensitive-token", "sk-test-secret", "private-key", "must-not-be-copied"} {
		require.NotContains(t, ev.Raw, secret)
	}
	require.Contains(t, ev.Message, "sensitive-token", "retain the original provider message")
}

func TestPromptOverflowInvalidTokenNumberIsUnknown(t *testing.T) {
	ev := DetectPromptOverflow(400, []byte(`{"error":{"message":"prompt is too long: 999999999999999999999999999999 tokens > 204798 maximum"}}`), "")
	require.NotNil(t, ev)
	require.Zero(t, ev.InputTokens)
	require.EqualValues(t, 204798, ev.MaxContextTokens)
}

func TestPromptOverflowGatewayNestingIsBounded(t *testing.T) {
	body := `{"error":{"message":"prompt is too long"}}`
	for i := 0; i < 10; i++ {
		body = fmt.Sprintf(`{"error":{"upstream_last_error":%s}}`, body)
	}
	require.Nil(t, DetectPromptOverflow(400, []byte(body), ""))
}

func TestDetectPromptOverflowAmbiguity(t *testing.T) {
	ambiguous := []struct {
		name, body string
		status     int
	}{
		{"unknown_rejection", `{"error":{"code":"upstream_error","message":"the upstream worker rejected the request payload"}}`, 400},
		{"sse_unknown_error", "data: " + `{"error":{"message":"worker refused the submitted payload"}}` + "\n\n", 200},
		{"truncated_text", "upstream rejected payload before json completed", 422},
	}
	for _, tc := range ambiguous {
		t.Run(tc.name, func(t *testing.T) {
			sample := DetectPromptOverflowAmbiguity(tc.status, []byte(tc.body), "")
			require.NotNil(t, sample)
			require.Equal(t, tc.status, sample.HTTPStatus)
			require.NotEmpty(t, sample.Message)
			require.NotContains(t, sample.Message, "sk-")
		})
	}

	rejected := []struct {
		name, body string
		status     int
	}{
		{"confirmed_overflow", `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`, 400},
		{"rate_limit", `{"error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}`, 429},
		{"authentication", `{"error":{"message":"invalid api key"}}`, 401},
		{"bad_parameter", `{"error":{"message":"max_tokens is too large","param":"max_tokens"}}`, 400},
		{"server_error", `{"error":{"message":"upstream failed"}}`, 500},
		{"quoted_output", `{"choices":[{"message":{"content":"prompt is too long"}}]}`, 200},
		{"empty", ``, 400},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			require.Nil(t, DetectPromptOverflowAmbiguity(tc.status, []byte(tc.body), ""))
		})
	}
}

func TestPromptOverflowAmbiguityRedactsAndClipsMessage(t *testing.T) {
	body := `{"error":{"message":"upstream rejected api_key=sk-testsecret ` + strings.Repeat("甲", 900) + `"}}`
	sample := DetectPromptOverflowAmbiguity(400, []byte(body), "")
	require.NotNil(t, sample)
	require.NotContains(t, sample.Message, "sk-testsecret")
	require.LessOrEqual(t, utf8.RuneCountInString(sample.Message), 800)
}
