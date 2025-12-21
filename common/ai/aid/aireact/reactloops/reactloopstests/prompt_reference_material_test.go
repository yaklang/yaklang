package reactloopstests

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// TestReActLoop_PromptReferenceMaterial tests that the prompt is emitted as reference material
func TestReActLoop_PromptReferenceMaterial(t *testing.T) {
	callCount := 0
	var events []*schema.AiOutputEvent
	var eventsMu sync.Mutex

	testInput := "test input for reference material"

	// Create ReAct instance as invoker
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()

			// Return finish action with a thought field to trigger stream event
			rsp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "finish",
				"answer": "Task completed",
				"human_readable_thought": "This is a test thought"
			}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err, "Failed to create ReAct")
	require.NotNil(t, reactIns, "ReAct instance should not be nil")

	// Create loop using ReAct as invoker
	loop, err := reactloops.NewReActLoop("test-ref-material-loop", reactIns)
	require.NoError(t, err, "Failed to create loop")
	require.NotNil(t, loop, "Loop should not be nil")

	// Execute loop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("test-task", ctx, testInput)
	require.NoError(t, err, "Execute should not fail")

	// Wait for events to be processed
	time.Sleep(500 * time.Millisecond)

	require.Greater(t, callCount, 0, "AI should have been called at least once")

	// Find reference material event
	eventsMu.Lock()
	defer eventsMu.Unlock()

	var referenceMaterialEvent *schema.AiOutputEvent
	for _, e := range events {
		if e.Type == schema.EVENT_TYPE_REFERENCE_MATERIAL {
			referenceMaterialEvent = e
			break
		}
	}

	require.NotNil(t, referenceMaterialEvent, "Expected to find reference material event, but none found. Total events: %d", len(events))

	// Verify event structure
	require.NotEmpty(t, referenceMaterialEvent.Content, "Reference material event content should not be empty")

	// Extract and verify payload
	payload := jsonpath.FindFirst(referenceMaterialEvent.Content, "$.payload")
	require.NotNil(t, payload, "Reference material payload should exist")

	payloadStr := utils.InterfaceToString(payload)
	require.NotEmpty(t, payloadStr, "Reference material payload should not be empty")

	// Verify event_uuid exists (this links to the stream event)
	eventUUID := jsonpath.FindFirst(referenceMaterialEvent.Content, "$.event_uuid")
	require.NotNil(t, eventUUID, "Reference material event_uuid should exist")
	require.NotEmpty(t, utils.InterfaceToString(eventUUID), "Reference material event_uuid should not be empty")

	// Verify type field
	typeField := jsonpath.FindFirst(referenceMaterialEvent.Content, "$.type")
	require.NotNil(t, typeField, "Reference material type field should exist")
	require.Equal(t, "text", utils.InterfaceToString(typeField), "Reference material type should be 'text'")

	// Verify payload contains expected content (prompt should include user input)
	require.Contains(t, payloadStr, testInput, "Reference material payload should contain user input")

	t.Logf("Reference material verified successfully")
	t.Logf("Event UUID: %s", utils.InterfaceToString(eventUUID))
	t.Logf("Payload length: %d bytes", len(payloadStr))
}

// TestReActLoop_PromptReferenceMaterial_OnlyOnce tests that reference material is emitted only once per transaction
func TestReActLoop_PromptReferenceMaterial_OnlyOnce(t *testing.T) {
	var events []*schema.AiOutputEvent
	var eventsMu sync.Mutex

	// Create ReAct instance as invoker
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()

			// Return finish action with multiple thought fields
			rsp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "finish",
				"answer": "Task completed",
				"human_readable_thought": "First thought",
				"reasoning": "Some reasoning",
				"summary": "Summary content"
			}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err, "Failed to create ReAct")

	// Create loop using ReAct as invoker
	loop, err := reactloops.NewReActLoop("test-ref-once-loop", reactIns)
	require.NoError(t, err, "Failed to create loop")

	// Execute loop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("test-task", ctx, "test input")
	require.NoError(t, err, "Execute should not fail")

	// Wait for events to be processed
	time.Sleep(500 * time.Millisecond)

	// Count reference material events
	eventsMu.Lock()
	defer eventsMu.Unlock()

	var referenceMaterialEvents []*schema.AiOutputEvent
	for _, e := range events {
		if e.Type == schema.EVENT_TYPE_REFERENCE_MATERIAL {
			referenceMaterialEvents = append(referenceMaterialEvents, e)
		}
	}

	require.Len(t, referenceMaterialEvents, 1, "Expected exactly one reference material event per transaction")

	// Verify the single event has valid content
	event := referenceMaterialEvents[0]
	require.NotEmpty(t, event.Content, "Reference material content should not be empty")

	payload := jsonpath.FindFirst(event.Content, "$.payload")
	require.NotNil(t, payload, "Payload should exist")
	require.NotEmpty(t, utils.InterfaceToString(payload), "Payload should not be empty")

	t.Logf("Successfully verified reference material was emitted exactly once")
}

// TestReActLoop_PromptReferenceMaterial_EventUUIDValid tests that reference material event_uuid is valid
func TestReActLoop_PromptReferenceMaterial_EventUUIDValid(t *testing.T) {
	var events []*schema.AiOutputEvent
	var eventsMu sync.Mutex

	// Create ReAct instance as invoker
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()

			rsp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "finish",
				"answer": "Task completed",
				"human_readable_thought": "Test thought"
			}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err, "Failed to create ReAct")

	loop, err := reactloops.NewReActLoop("test-event-uuid-loop", reactIns)
	require.NoError(t, err, "Failed to create loop")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("test-task", ctx, "test input")
	require.NoError(t, err, "Execute should not fail")

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	// Find reference material event and its linked stream event
	var refMaterialEvent *schema.AiOutputEvent
	streamEventIDs := make(map[string]bool)

	for _, e := range events {
		if e.Type == schema.EVENT_TYPE_REFERENCE_MATERIAL {
			refMaterialEvent = e
		}
		// Collect stream start event IDs
		if e.Type == schema.EVENT_TYPE_STREAM_START {
			eventWriterID := jsonpath.FindFirst(e.Content, "$.event_writer_id")
			if eventWriterID != nil {
				streamEventIDs[utils.InterfaceToString(eventWriterID)] = true
			}
		}
	}

	require.NotNil(t, refMaterialEvent, "Reference material event should exist")

	// Verify event_uuid in reference material matches a stream event
	eventUUID := jsonpath.FindFirst(refMaterialEvent.Content, "$.event_uuid")
	require.NotNil(t, eventUUID, "event_uuid should exist in reference material")

	eventUUIDStr := utils.InterfaceToString(eventUUID)
	require.NotEmpty(t, eventUUIDStr, "event_uuid should not be empty")

	// The event_uuid should match one of the stream event IDs
	require.True(t, streamEventIDs[eventUUIDStr], "event_uuid should match a stream event ID. Got: %s, Available: %v", eventUUIDStr, streamEventIDs)

	t.Logf("Reference material event_uuid validated: %s", eventUUIDStr)
}
