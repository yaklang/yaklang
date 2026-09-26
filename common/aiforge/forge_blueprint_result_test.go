package aiforge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestRetryEmptyForgeResult(t *testing.T) {
	calls := 0
	result, err := retryEmptyForgeResult("original report instructions", func(prompt string) (string, error) {
		calls++
		if calls == 1 {
			if prompt != "original report instructions" {
				t.Fatalf("first prompt changed: %q", prompt)
			}
			return " \n", nil
		}
		if !strings.Contains(prompt, "最终输出通道") {
			t.Fatalf("retry did not request a final output: %q", prompt)
		}
		return "# Report", nil
	})
	if err != nil || result != "# Report" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestRetryEmptyForgeResultNeverPublishesReasoningOrLoops(t *testing.T) {
	calls := 0
	result, err := retryEmptyForgeResult("report", func(string) (string, error) {
		calls++
		return "", nil
	})
	if err == nil || result != "" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
	calls = 0
	want := errors.New("provider failed")
	result, err = retryEmptyForgeResult("report", func(string) (string, error) {
		calls++
		return "", want
	})
	if !errors.Is(err, want) || result != "" || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

// Exercise the legacy Blueprint result handler through the same request-to-chat
// adapter used by model callbacks, while keeping the provider fully in process.
func TestForgeResultDefaultPreservesModelBudgetAndStructuredAction(t *testing.T) {
	for _, maxTokens := range []int64{8192, 512} {
		t.Run(fmt.Sprintf("max_tokens_%d", maxTokens), func(t *testing.T) {
			const prompt = "Return only JSON with @action=client_result and a value field."
			const output = `{"@action":"client_result","value":"kept"}`
			builder := NewYakForgeBlueprintConfig("client-result", "Analyze supplied data", "").
				WithResultPrompt(prompt).
				WithActionName("client_result")
			blueprint, err := builder.Build()
			require.NoError(t, err)

			type observedRequest struct {
				prompt    string
				maxTokens int64
			}
			requests := make(chan observedRequest, 4)
			callback := aicommon.AIChatToAICallbackType(func(prompt string, options ...aispec.AIConfigOption) (string, error) {
				// The model's configured budget is the baseline. Request options
				// arrive through the real AIChatToAICallbackType adapter.
				config := &aispec.AIConfig{}
				aispec.WithMaxTokens(maxTokens)(config)
				for _, option := range options {
					option(config)
				}
				requests <- observedRequest{prompt: prompt, maxTokens: *config.MaxTokens}
				return output, nil
			})
			coordinator := newForgeResultTestCoordinator(t, blueprint, callback)
			coordinator.ResultHandler(coordinator)

			require.Len(t, requests, 1, "legacy result generation must make one request")
			request := <-requests
			require.Equal(t, maxTokens, request.maxTokens, "result generation must preserve the model budget")
			require.Contains(t, request.prompt, prompt)
			require.NotContains(t, request.prompt, "Markdown")
			require.Equal(t, output, builder.ForgeResult.Formated)
			require.NotNil(t, builder.ForgeResult.Action, "JSON action must remain parseable")
			require.Equal(t, "client_result", builder.ForgeResult.Action.Name())
			require.Equal(t, "kept", builder.ForgeResult.Action.GetString("value"))
		})
	}
}

func TestForgeResultEmptyOutputRetryIsOptIn(t *testing.T) {
	for _, test := range []struct {
		name      string
		retry     bool
		second    string
		wantCalls int
		wantError bool
	}{
		{name: "legacy", wantCalls: 1},
		{name: "retry_json", retry: true, second: `{"@action":"result","value":"kept"}`, wantCalls: 2},
		{name: "retry_exhausted", retry: true, wantCalls: 2, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const prompt = "Return only JSON with @action=result and a value field."
			var result string
			var resultErr error
			blueprint := NewForgeBlueprint("result-retry", WithResultPrompt(prompt),
				WithResultHandler(func(value string, err error) { result, resultErr = value, err }))
			if test.retry {
				WithResultPolicy(ForgeResultPolicy{RetryEmptyOutput: true})(blueprint)
			}
			var prompts []string
			coordinator := newForgeResultTestCoordinator(t, blueprint,
				func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					prompts = append(prompts, request.GetPrompt())
					response := config.NewAIResponse()
					response.EmitReasonStream(strings.NewReader("private reasoning, never a final result"))
					output := ""
					if len(prompts) > 1 {
						output = test.second
					}
					response.EmitOutputStream(strings.NewReader(output))
					response.Close()
					return response, nil
				})
			coordinator.ResultHandler(coordinator)

			require.Len(t, prompts, test.wantCalls)
			if test.wantError {
				require.ErrorContains(t, resultErr, "empty final output after retry")
			} else {
				require.NoError(t, resultErr)
			}
			require.Equal(t, test.second, result)
			for _, got := range prompts {
				require.Contains(t, got, prompt)
				require.NotContains(t, got, "Markdown", "retry must preserve structured output instructions")
			}
			if test.retry {
				require.Contains(t, prompts[1], "最终输出通道")
			}
		})
	}
}

func newForgeResultTestCoordinator(t *testing.T, blueprint *ForgeBlueprint, callback aicommon.AICallbackType) *aid.Coordinator {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	coordinator, err := blueprint.CreateCoordinator(ctx, map[string]any{},
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableMemoryTriage(true),
		aicommon.WithAllowRequireForUserInteract(false),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(callback))
	require.NoError(t, err)
	require.NotNil(t, coordinator.ResultHandler)
	return coordinator
}
