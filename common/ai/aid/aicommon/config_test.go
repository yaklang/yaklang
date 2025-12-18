package aicommon

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestConfig_Smoking(t *testing.T) {
	config := NewConfig(context.Background())
	require.NotNil(t, config)
	require.NotNil(t, config.OriginalAICallback)
}

func TestConfig_AIServiceName(t *testing.T) {
	token := uuid.NewString()
	serviceNameOk := false
	config := NewTestConfig(context.Background(),
		WithAIServiceName(token),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.AIService == token {
				serviceNameOk = true
			}
		}),
	)
	config.EmitInfo("abc")

	if serviceNameOk == false {
		t.Fatalf("AIServiceName not set correctly")
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
