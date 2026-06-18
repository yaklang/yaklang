package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitSessionSnapshot_EmitsLegacyCapabilityInventoryWithTaskIndex(t *testing.T) {
	var captured *schema.AiOutputEvent
	base := aicommon.NewEmitter("coordinator", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if e != nil && e.NodeId == aicommon.CapabilityInventoryNodeID {
			captured = e
		}
		return e, nil
	})
	taskEmitter := base.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && event.TaskIndex == "" {
			event.TaskIndex = "1-2"
		}
		return event
	})

	cfg := aicommon.NewConfig(context.Background(),
		aicommon.WithEmitter(taskEmitter),
		aicommon.WithDisableAutoSkills(true),
	)
	loop := NewMinimalReActLoop(cfg, nil)

	EmitSessionSnapshot(cfg, loop, nil)

	require.NotNil(t, captured)
	require.Equal(t, aicommon.CapabilityInventoryNodeID, captured.NodeId)
	require.Equal(t, "1-2", captured.TaskIndex)
}
