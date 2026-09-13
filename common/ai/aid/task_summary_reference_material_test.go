package aid

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestTaskSummary_DoesNotEmitPromptReferenceMaterial(t *testing.T) {
	for _, tc := range []struct{ name, response, want string }{
		{"streamed", `{"@action":"summary","task_short_summary":"brief result","task_long_summary":"detailed result"}`, "detailed result"},
		{"fallback", `{"@action":"summary","task_short_summary":"fallback result"}`, "fallback result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var mu sync.Mutex
			var events []*schema.AiOutputEvent
			promptToken := "summary-input-must-not-be-reference"
			coordinator, err := NewCoordinatorContext(ctx, promptToken,
				aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
					mu.Lock()
					defer mu.Unlock()
					events = append(events, event)
				}),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					require.Contains(t, request.GetPrompt(), promptToken)
					response := config.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(tc.response))
					response.Close()
					return response, nil
				}),
			)
			require.NoError(t, err)
			coordinator.Workdir = t.TempDir()
			task := coordinator.generateAITaskWithName("Summary", promptToken)
			task.Index = "1"
			task.SetUserInput(promptToken)
			coordinator.rootTask = task
			coordinator.ContextProvider.RootTask = task
			coordinator.ContextProvider.StoreCurrentTask(task)
			require.NoError(t, task.generateTaskSummary("", ""))
			coordinator.WaitForStream()
			require.Equal(t, tc.want, task.LongSummary)

			mu.Lock()
			defer mu.Unlock()
			var output strings.Builder
			for _, event := range events {
				require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, event.Type,
					"task summary must not attach its full prompt")
				if event.Type == schema.EVENT_TYPE_STREAM && event.NodeId == "summary-long" {
					output.Write(event.StreamDelta)
				}
			}
			require.Contains(t, output.String(), tc.want, "summary output must still reach the user")
		})
	}
}
