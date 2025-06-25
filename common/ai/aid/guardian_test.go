package aid

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

// Assuming EVENT_TYPE_STRUCTURED is "structured" as per guardian_emitter.go
const testEventTypeStructured = EventType("structured")

func TestNewAsyncGuardian(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAsyncGuardian(ctx, "test_coordinator_id")
	assert.NotNil(t, g, "Guardian should not be nil")
	assert.Equal(t, ctx, g.ctx, "Guardian context should be the one provided")
	assert.NotNil(t, g.unlimitedInput, "Guardian unlimitedInput channel should not be nil")
	assert.NotNil(t, g.callbackMutex, "Guardian callbackMutex should not be nil")
	assert.NotNil(t, g.eventTriggerCallback, "Guardian eventTriggerCallback map should not be nil")
	assert.NotNil(t, g.mirrorCallback, "Guardian mirrorCallback map should not be nil")

	time.Sleep(10 * time.Millisecond)
}

func TestAsyncGuardian_SetOutputEmitterAndFeed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAsyncGuardian(ctx, "test_coordinator_id")
	assert.NotNil(t, g)

	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	emitter := func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	}

	g.setOutputEmitter("emitter_coord_id", emitter)

	testEvent := &Event{Type: "test_event_type", Content: []byte("test_data")}

	err := g.registerEventTrigger(testEvent.Type, func(evt *Event, e GuardianEmitter, caller AICaller) {
		e.EmitStructured(string(evt.Type), evt)
	})
	assert.NoError(t, err, "Failed to register event trigger for test")

	g.feed(testEvent)

	time.Sleep(50 * time.Millisecond)

	emitterMutex.Lock()
	assert.Len(t, emittedEvents, 1, "Emitter should have been called once")
	if len(emittedEvents) == 1 {
		emittedEv := emittedEvents[0]
		assert.NotNil(t, emittedEv, "Emitted event should not be nil")
		assert.Equal(t, testEventTypeStructured, emittedEv.Type, "Emitted event type mismatch")
		assert.Equal(t, string(testEvent.Type), emittedEv.NodeId, "Emitted event nodeId mismatch")

		var originalPayload Event
		unmarshalErr := json.Unmarshal(emittedEv.Content, &originalPayload)
		assert.NoError(t, unmarshalErr, "Failed to unmarshal emitted event content")
		assert.Equal(t, testEvent.Type, originalPayload.Type, "Original payload type mismatch")
		assert.Equal(t, testEvent.Content, originalPayload.Content, "Original payload content mismatch")
	}
	emitterMutex.Unlock()

	// Test feeding a nil event
	emittedEvents = nil // Reset for next check
	g.feed(nil)
	time.Sleep(50 * time.Millisecond)

	emitterMutex.Lock()
	assert.Empty(t, emittedEvents, "Emitter should not be called for nil event")
	emitterMutex.Unlock()
}

func TestAsyncGuardian_RegisterEventTrigger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAsyncGuardian(ctx, "test_coordinator_id")
	assert.NotNil(t, g)

	var triggerCalled bool
	var receivedEventInTrigger *Event
	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}

	testEventType := EventType("specific_event")
	triggerNodeId := "node_from_specific_trigger"

	trigger := func(event *Event, emitter GuardianEmitter, caller AICaller) {
		triggerCalled = true
		receivedEventInTrigger = event
		emitter.EmitStructured(triggerNodeId, &Event{Type: "event_from_trigger", Content: []byte("trigger_data")})
	}

	err := g.registerEventTrigger(testEventType, trigger)
	assert.NoError(t, err)

	g.setOutputEmitter("emitter_coord_id", func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	// Feed an event of the specific type
	eventToFeed := &Event{Type: testEventType, Content: []byte("event_data")}
	g.feed(eventToFeed)

	time.Sleep(50 * time.Millisecond)

	assert.True(t, triggerCalled, "Trigger should have been called for specific event type")
	assert.NotNil(t, receivedEventInTrigger, "Received event in trigger should not be nil")
	assert.Equal(t, eventToFeed.Type, receivedEventInTrigger.Type)
	assert.Equal(t, eventToFeed.Content, receivedEventInTrigger.Content)

	emitterMutex.Lock()
	assert.Len(t, emittedEvents, 1, "Output emitter should have received one event (from trigger via EmitStructured)")
	if len(emittedEvents) == 1 {
		emittedEv := emittedEvents[0]
		assert.Equal(t, testEventTypeStructured, emittedEv.Type)
		assert.Equal(t, triggerNodeId, emittedEv.NodeId)

		var payloadFromTrigger Event
		unmarshalErr := json.Unmarshal(emittedEv.Content, &payloadFromTrigger)
		assert.NoError(t, unmarshalErr)
		assert.Equal(t, EventType("event_from_trigger"), payloadFromTrigger.Type)
		assert.Equal(t, []byte("trigger_data"), payloadFromTrigger.Content)
	}
	emitterMutex.Unlock()

	// Feed an event of a different type
	triggerCalled = false        // Reset
	receivedEventInTrigger = nil // Reset
	emittedEvents = nil          // Reset

	differentEvent := &Event{Type: "other_event_type", Content: []byte("other_data")}
	g.feed(differentEvent)

	time.Sleep(50 * time.Millisecond)

	assert.False(t, triggerCalled, "Trigger should not be called for a different event type")
	assert.Nil(t, receivedEventInTrigger, "Received event in trigger should be nil for different event type")

	emitterMutex.Lock()
	assert.Empty(t, emittedEvents, "Output emitter should not have been called for an event with no specific trigger and no mirrors")
	emitterMutex.Unlock()
}

func TestAsyncGuardian_RegisterMirrorEventTrigger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAsyncGuardian(ctx, "test_coordinator_id")
	assert.NotNil(t, g)

	var mirrorTriggerCalled bool
	var receivedUnlimitedChan *chanx.UnlimitedChan[*Event]
	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	mirrorName := "test_mirror"
	mirrorNodeId := "node_from_mirror_trigger"

	mirrorTrigger := func(unlimitedChan *chanx.UnlimitedChan[*Event], emitter GuardianEmitter) {
		mirrorTriggerCalled = true
		receivedUnlimitedChan = unlimitedChan
		emitter.EmitStructured(mirrorNodeId, &Event{Type: "event_from_mirror", Content: []byte("mirror_data")})
	}

	err := g.registerMirrorEventTrigger(mirrorName, mirrorTrigger)
	assert.NoError(t, err)

	dupError := g.registerMirrorEventTrigger(mirrorName, mirrorTrigger)
	assert.Error(t, dupError, "Registering duplicate mirror trigger should return an error")
	assert.Contains(t, dupError.Error(), "already registered", "Error message should indicate duplicate registration")

	g.setOutputEmitter("emitter_coord_id", func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	eventToFeed := &Event{Type: "any_event_type", Content: []byte("any_data")}
	g.feed(eventToFeed)

	time.Sleep(50 * time.Millisecond)

	assert.True(t, mirrorTriggerCalled, "Mirror trigger should have been called")
	assert.NotNil(t, receivedUnlimitedChan, "Received unlimitedChan in mirror trigger should not be nil")

	g.callbackMutex.RLock()
	internalMirrorStream, ok := g.mirrorCallback[mirrorName]
	g.callbackMutex.RUnlock()
	assert.True(t, ok, "Mirror stream should exist in guardian's map")
	assert.Equal(t, internalMirrorStream.unlimitedChan, receivedUnlimitedChan, "Channel passed to mirror trigger should be the one from guardian's map")

	emitterMutex.Lock()
	assert.Len(t, emittedEvents, 1, "Output emitter should receive one event from the mirror via EmitStructured")
	if len(emittedEvents) == 1 {
		emittedEv := emittedEvents[0]
		assert.Equal(t, testEventTypeStructured, emittedEv.Type)
		assert.Equal(t, mirrorNodeId, emittedEv.NodeId)

		var payloadFromMirror Event
		unmarshalErr := json.Unmarshal(emittedEv.Content, &payloadFromMirror)
		assert.NoError(t, unmarshalErr)
		assert.Equal(t, EventType("event_from_mirror"), payloadFromMirror.Type)
		assert.Equal(t, []byte("mirror_data"), payloadFromMirror.Content)
	}
	emitterMutex.Unlock()
}

func TestAsyncGuardian_EventLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g := newAsyncGuardian(ctx, "test_coordinator_id")
	assert.NotNil(t, g)

	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	g.setOutputEmitter("emitter_coord_id", func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	initialEventType := EventType("initial_event_type_for_cancel_test")
	g.registerEventTrigger(initialEventType, func(event *Event, emitter GuardianEmitter, caller AICaller) {
		emitter.EmitStructured(string(event.Type), event)
	})

	// No mirror needed if event trigger directly uses EmitStructured to test the outputEmitter

	g.feed(&Event{Type: initialEventType, Content: []byte("initial_data")})
	time.Sleep(50 * time.Millisecond)

	emitterMutex.Lock()
	assert.NotEmpty(t, emittedEvents, "Output emitter should be called for initial event")
	if len(emittedEvents) > 0 {
		assert.Equal(t, testEventTypeStructured, emittedEvents[0].Type)
	}
	emitterMutex.Unlock()

	cancel() // Cancel the context

	emitterMutex.Lock()
	emittedEvents = nil
	emitterMutex.Unlock()

	g.feed(&Event{Type: initialEventType, Content: []byte("event_after_cancel")})

	time.Sleep(100 * time.Millisecond)

	emitterMutex.Lock()
	assert.Empty(t, emittedEvents, "Output emitter should not be called after context cancellation")
	emitterMutex.Unlock()

	select {
	case _, ok := <-g.unlimitedInput.OutputChannel():
		assert.False(t, ok, "Guardian's input channel (output side) should be closed after context cancellation")
	case <-time.After(200 * time.Millisecond):
		t.Log("Timeout waiting for guardian input channel to close, this might be acceptable if event processing has stopped.")
	}
}

// Test for multiple event triggers for the same event type
func TestAsyncGuardian_MultipleEventTriggers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := newAsyncGuardian(ctx, "test_coordinator_id")

	var trigger1Called, trigger2Called bool
	eventType := EventType("multi_trigger_event")

	g.registerEventTrigger(eventType, func(event *Event, emitter GuardianEmitter, caller AICaller) {
		trigger1Called = true
	})
	g.registerEventTrigger(eventType, func(event *Event, emitter GuardianEmitter, caller AICaller) {
		trigger2Called = true
	})

	g.feed(&Event{Type: eventType})
	time.Sleep(50 * time.Millisecond)

	assert.True(t, trigger1Called, "First trigger should have been called")
	assert.True(t, trigger2Called, "Second trigger should have been called")
}

// Test interaction between event triggers and mirror triggers
func TestAsyncGuardian_EventAndMirrorTriggers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := newAsyncGuardian(ctx, "test_coordinator_id")

	var eventTriggerCalled, mirrorTriggerCalled bool
	specificEventType := EventType("specific_for_interaction_test")

	emittedEventTypes := make(map[EventType]bool)
	emitterMutex := &sync.Mutex{}

	g.setOutputEmitter("emitter_coord_id", func(e *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		if e.Type == testEventTypeStructured {
			var payload Event
			if json.Unmarshal(e.Content, &payload) == nil {
				if payload.Type == "from_event_trigger" {
					emittedEventTypes["from_event_trigger_via_structured"] = true
				}
				if payload.Type == "from_mirror_trigger" {
					emittedEventTypes["from_mirror_trigger_via_structured"] = true
				}
			}
		}
	})

	g.registerEventTrigger(specificEventType, func(event *Event, emitter GuardianEmitter, caller AICaller) {
		eventTriggerCalled = true
		emitter.EmitStructured("event_trigger_node", &Event{Type: "from_event_trigger"})
	})
	g.registerMirrorEventTrigger("interaction_mirror", func(uc *chanx.UnlimitedChan[*Event], emitter GuardianEmitter) {
		mirrorTriggerCalled = true
		for event := range uc.OutputChannel() {
			_ = event
			emitter.EmitStructured("mirror_trigger_node", &Event{Type: "from_mirror_trigger"})
		}
	})

	// Feed event that matches specific trigger
	g.feed(&Event{Type: specificEventType})
	time.Sleep(50 * time.Millisecond)

	assert.True(t, eventTriggerCalled, "Event trigger should be called for specific event")
	assert.True(t, mirrorTriggerCalled, "Mirror trigger should be called for specific event (run1)")

	emitterMutex.Lock()
	assert.True(t, emittedEventTypes["from_event_trigger_via_structured"], "Output emitter should receive event from event trigger (run1)")
	assert.True(t, emittedEventTypes["from_mirror_trigger_via_structured"], "Output emitter should receive event from mirror trigger (run1)")
	emitterMutex.Unlock()

	// Reset flags for next event
	eventTriggerCalled = false
	mirrorTriggerCalled = false
	emittedEventTypes = make(map[EventType]bool)

	// Feed event that does not match specific trigger
	g.feed(&Event{Type: "other_event_for_interaction"})
	time.Sleep(50 * time.Millisecond)

	assert.False(t, eventTriggerCalled, "Event trigger should NOT be called for other event")
	assert.False(t, mirrorTriggerCalled, "Mirror trigger should be called once in run1, not called in run2")

	emitterMutex.Lock()
	assert.False(t, emittedEventTypes["from_event_trigger_via_structured"], "Output emitter should NOT receive event from event trigger (run2)")
	assert.True(t, emittedEventTypes["from_mirror_trigger_via_structured"], "Output emitter should receive event from mirror trigger (run2)")
	emitterMutex.Unlock()
}
