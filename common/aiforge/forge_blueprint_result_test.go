package aiforge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

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

func newForgeResultTestCoordinator(t *testing.T, blueprint *ForgeBlueprint, callback aicommon.AICallbackType) *coordinator_legacy.Coordinator {
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

func TestForgeResultPreservesPartialReadFailure(t *testing.T) {
	failure := errors.New("result stream interrupted")
	result, err := readForgeResult(io.MultiReader(strings.NewReader("partial report"), iotest.ErrReader(failure)))
	require.Equal(t, "partial report", result)
	require.ErrorIs(t, err, failure)
	result, err = readForgeResult(strings.NewReader("complete report"))
	require.NoError(t, err)
	require.Equal(t, "complete report", result)
}

func TestForgeResultGeneratorDeliversOnce(t *testing.T) {
	for _, failure := range []error{nil, errors.New("generation failed")} {
		var callbacks, generations int
		blueprint := NewForgeBlueprint("custom-generator", WithResultPrompt("original format"), WithResultHandler(func(value string, err error) {
			callbacks++
			require.Equal(t, "partial or complete", value)
			require.ErrorIs(t, err, failure)
		}))
		blueprint.ResultGenerator = func(_ *coordinator_legacy.Coordinator, prompt string) (string, error) {
			generations++
			require.Contains(t, prompt, "original format")
			return "partial or complete", failure
		}
		coordinator := newForgeResultTestCoordinator(t, blueprint, func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			t.Fatal("custom generator must replace the default request")
			return nil, nil
		})
		coordinator.ResultHandler(coordinator)
		require.Equal(t, 1, generations)
		require.Equal(t, 1, callbacks)
	}
}
