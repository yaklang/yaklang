package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitCapabilityInventorySnapshot_InheritsTaskIndexFromEmitter(t *testing.T) {
	var captured *schema.AiOutputEvent
	base := aicommon.NewEmitter("coordinator", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = e
		return e, nil
	})
	taskEmitter := base.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && event.TaskIndex == "" {
			event.TaskIndex = "1-2"
		}
		return event
	})

	cfg := aicommon.NewConfig(context.Background(), aicommon.WithEmitter(taskEmitter))
	loop := NewMinimalReActLoop(cfg, nil)

	EmitCapabilityInventorySnapshot(cfg, loop)

	require.NotNil(t, captured)
	require.Equal(t, aicommon.CapabilityInventoryNodeID, captured.NodeId)
	require.Equal(t, "1-2", captured.TaskIndex)
}
