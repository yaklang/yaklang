package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestConfig_Smoking(t *testing.T) {
	originalTiered := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(nil)
	t.Cleanup(func() {
		consts.SetTieredAIConfig(originalTiered)
	})

	config := NewConfig(context.Background())
	require.NotNil(t, config)
	require.False(t, config.AICallbackAvailable())
}

func TestConfig_AIServiceName(t *testing.T) {
	token := uuid.NewString()
	token2 := uuid.NewString()
	serviceNameOk := false
	serviceModelOk := false
	config := NewTestConfig(context.Background(),
		WithAIChatInfo(token, token2),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.AIService == token {
				serviceNameOk = true
			}
			if e.AIModelName == token2 {
				serviceModelOk = true
			}
		}),
	)
	config.EmitInfo("abc")

	if serviceNameOk == false {
		t.Fatalf("AIServiceName not set correctly")
	}

	if serviceModelOk == false {
		t.Fatalf("AIModelName not set correctly")
	}
}

// TestConfig_WithID_SyncsEmitterId verifies that WithID also updates the Emitter's internal id
// This ensures that events emitted after WithID is applied use the correct CoordinatorId
func TestConfig_WithID_SyncsEmitterId(t *testing.T) {
	customId := uuid.NewString()
	var capturedCoordinatorId string

	config := NewTestConfig(context.Background(),
		WithID(customId),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			capturedCoordinatorId = e.CoordinatorId
		}),
	)

	// Verify config.Id is set correctly
	require.Equal(t, customId, config.Id, "Config.Id should be set to the custom ID")

	// Emit an event to verify the Emitter uses the correct ID
	config.EmitInfo("test event")

	// Verify the event's CoordinatorId matches the custom ID
	require.Equal(t, customId, capturedCoordinatorId,
		"Event CoordinatorId should match the custom ID set via WithID. "+
			"This test ensures WithID syncs the Emitter's internal id, preventing a third CoordinatorId leak.")
}

func TestGetLastIDFromConfigOptions(t *testing.T) {
	firstID := uuid.NewString()
	lastID := uuid.NewString()

	id, ok := GetLastIDFromConfigOptions(
		WithAIServiceName("test-service"),
		WithID(firstID),
		WithLanguage("zh"),
		WithID(lastID),
	)
	require.True(t, ok)
	require.Equal(t, lastID, id)
}

func TestGetLastIDFromConfigOptions_NoID(t *testing.T) {
	id, ok := GetLastIDFromConfigOptions(
		WithAIServiceName("test-service"),
		WithLanguage("zh"),
	)
	require.False(t, ok)
	require.Empty(t, id)
}

func TestConfig_DefaultToolComposeConcurrency(t *testing.T) {
	config := NewConfig(context.Background())
	require.Equal(t, 2, config.ToolComposeConcurrency)
}

func TestConfig_ToolComposeConcurrencyPropagation(t *testing.T) {
	parent := NewConfig(context.Background(), WithToolComposeConcurrency(5))
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.Equal(t, 5, child.ToolComposeConcurrency)
}

func TestConfig_IntervalReviewConfigPropagation(t *testing.T) {
	parent := NewConfig(
		context.Background(),
		WithDisableToolCallerIntervalReview(true),
		WithToolCallerIntervalReviewDuration(7*time.Second),
		WithToolCallIntervalReviewExtraPrompt("Cancel immediately if no heartbeat appears twice."),
	)
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)

	require.True(t, child.DisableIntervalReview)
	require.Equal(t, 7*time.Second, child.IntervalReviewDuration)
	require.Equal(t,
		"Cancel immediately if no heartbeat appears twice.",
		child.ToolCallIntervalReviewExtraPrompt,
	)
	require.Equal(
		t,
		"Cancel immediately if no heartbeat appears twice.",
		child.GetConfigString(ConfigKeyToolCallIntervalReviewExtraPrompt),
	)
}

func TestConfig_ToolManagerPropagation(t *testing.T) {
	parent := NewConfig(context.Background())
	require.NotNil(t, parent.GetAiToolManager())

	parent.GetAiToolManager().AddRecentlyUsedTool(buildinaitools.GetBasicBuildInTools()[0])
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)

	require.Same(t, parent.GetAiToolManager(), child.GetAiToolManager())
	require.True(t, child.GetAiToolManager().IsRecentlyUsedTool("now"))
}

func TestConfig_SessionPromptStatePropagation(t *testing.T) {
	parent := NewConfig(context.Background())
	parent.SetSessionTitle("shared-title")
	_, err := parent.AppendUserInputHistory("round-1", time.Now())
	require.NoError(t, err)

	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.Same(t, parent.GetSessionPromptState(), child.GetSessionPromptState())
	require.Equal(t, "shared-title", child.GetSessionTitle())
	require.Equal(t, "round-1", child.GetPrevSessionUserInput())

	_, err = child.AppendUserInputHistory("round-2", time.Now())
	require.NoError(t, err)

	history := parent.GetUserInputHistory()
	require.Len(t, history, 2)
	require.Equal(t, "round-2", parent.GetPrevSessionUserInput())
}
