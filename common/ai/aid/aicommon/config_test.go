package aicommon

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestConfig_Smoking(t *testing.T) {
	config := NewConfig(context.Background())
	require.NotNil(t, config)
	require.NotNil(t, config.OriginalAICallback)
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

func TestCallAITransaction_PromptFallbackCompressionLevelResetsPerAttempt(t *testing.T) {
	var compressionLevels []int
	var callCount int

	config := NewTestConfig(
		context.Background(),
		WithAiCallTokenLimit(64),
		WithAITransactionAutoRetry(2),
	)

	postHandlerCalls := 0
	err := CallAITransaction(
		config,
		strings.Repeat("long prompt ", 200),
		func(request *AIRequest) (*AIResponse, error) {
			callCount++
			_, err := config.prepareRequestPrompt(request)
			if err != nil {
				return nil, err
			}
			rsp := NewUnboundAIResponse()
			rsp.Close()
			return rsp, nil
		},
		func(rsp *AIResponse) error {
			postHandlerCalls++
			if postHandlerCalls == 1 {
				return errors.New("retry once")
			}
			return nil
		},
		WithAIRequest_PromptFallback(func(expectedContextSize int, currentContextSize int, compressionLevel int) (string, error) {
			compressionLevels = append(compressionLevels, compressionLevel)
			return "short prompt", nil
		}),
	)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
	require.Equal(t, []int{0, 0}, compressionLevels)
}
