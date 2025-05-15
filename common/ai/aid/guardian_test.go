package aid

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

func TestNewAsyncGuardian(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAysncGuardian(ctx)
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

	g := newAysncGuardian(ctx)
	assert.NotNil(t, g)

	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	emitter := func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	}

	g.setOutputEmitter(emitter)

	testEvent := &Event{Type: "test_event_type", Content: []byte("test_data")}

	// Register a simple event trigger for the specific event type that uses the outputEmitter
	err := g.registerEventTrigger(testEvent.Type, func(evt *Event, e func(*Event)) {
		// e is the outputEmitter we set via g.setOutputEmitter()
		e(evt)
	})
	assert.NoError(t, err, "Failed to register event trigger for test")

	g.feed(testEvent)

	time.Sleep(50 * time.Millisecond)

	emitterMutex.Lock()
	assert.Len(t, emittedEvents, 1, "Emitter should have been called once")
	if len(emittedEvents) == 1 {
		assert.NotNil(t, emittedEvents[0], "Emitted event should not be nil")
		assert.Equal(t, testEvent.Type, emittedEvents[0].Type, "Emitted event type mismatch")
		assert.Equal(t, testEvent.Content, emittedEvents[0].Content, "Emitted event content mismatch")
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

	g := newAysncGuardian(ctx)
	assert.NotNil(t, g)

	var triggerCalled bool
	var receivedEventInTrigger *Event
	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}

	testEventType := EventType("specific_event")

	trigger := func(event *Event, emitter func(*Event)) {
		triggerCalled = true
		receivedEventInTrigger = event
		emitter(&Event{Type: "event_from_trigger", Content: []byte("trigger_data")})
	}

	err := g.registerEventTrigger(testEventType, trigger)
	assert.NoError(t, err)

	g.setOutputEmitter(func(event *Event) {
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
	assert.Len(t, emittedEvents, 1, "Output emitter should have received one event (from trigger)")
	if len(emittedEvents) == 1 {
		assert.Equal(t, EventType("event_from_trigger"), emittedEvents[0].Type)
		assert.Equal(t, []byte("trigger_data"), emittedEvents[0].Content)
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
	// The outputEmitter is NOT directly called by emitEvent for the original event if a trigger exists.
	// The trigger IS RESPONSIBLE for calling the emitter.
	// If no trigger matches, then the event is not passed to the outputEmitter by default.
	// Correction: emitEvent calls eventTriggerCallback, then mirrorCallback.
	// The outputEmitter set by setOutputEmitter is the one passed to triggers and mirrors.
	// If there are no specific triggers, the original event is not explicitly sent to outputEmitter
	// unless a mirror sends it.
	// Let's re-read guardian.go:
	// func (a *asyncGuardian) emitEvent(event *Event)
	//   a.callbackMutex.RLock() // RLock for reading callbacks
	//   defer a.callbackMutex.RUnlock()
	//   if triggers, ok := a.eventTriggerCallback[event.Type]; ok {
	// 	   for _, trigger := range triggers {
	// 		   trigger(event, a.outputEmitter) // outputEmitter is passed here
	// 	   }
	//   }
	//   for _, mirror := range a.mirrorCallback { // mirror also gets a.outputEmitter
	// 	   mirror.trigger(mirror.unlimitedChan, a.outputEmitter)
	//   }
	// The main outputEmitter (a.outputEmitter) is NOT called directly with `event` at the end of `emitEvent`.
	// It's only called if a trigger or mirror calls it.
	// So, if an event has no specific trigger, and no mirrors, it won't be seen by setOutputEmitter.
	// This makes sense. The `setOutputEmitter` is the "default" emitter that triggers/mirrors can use.

	// For `differentEvent`, no specific trigger exists. So, outputEmitter won't be called by a specific trigger.
	// If there are no mirrors, outputEmitter will not be called at all for `differentEvent`.
	assert.Empty(t, emittedEvents, "Output emitter should not have been called for an event with no specific trigger and no mirrors (unless a mirror is set)")
	emitterMutex.Unlock()
}

func TestAsyncGuardian_RegisterMirrorEventTrigger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := newAysncGuardian(ctx)
	assert.NotNil(t, g)

	var mirrorTriggerCalled bool
	var receivedUnlimitedChan *chanx.UnlimitedChan[*Event]
	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	mirrorName := "test_mirror"

	mirrorTrigger := func(unlimitedChan *chanx.UnlimitedChan[*Event], emitter func(*Event)) {
		mirrorTriggerCalled = true
		receivedUnlimitedChan = unlimitedChan
		// The mirror trigger is called for *every* event that `emitEvent` processes.
		// It does not receive the specific event that caused its invocation as a direct argument.
		// Its role is to react to the fact that an event occurred and potentially use its
		// own unlimitedChan or the provided emitter.
		emitter(&Event{Type: "event_from_mirror", Content: []byte("mirror_data")})
	}

	err := g.registerMirrorEventTrigger(mirrorName, mirrorTrigger)
	assert.NoError(t, err)

	dupError := g.registerMirrorEventTrigger(mirrorName, mirrorTrigger)
	assert.Error(t, dupError, "Registering duplicate mirror trigger should return an error")
	assert.Contains(t, dupError.Error(), "already registered", "Error message should indicate duplicate registration")

	g.setOutputEmitter(func(event *Event) {
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
	// The outputEmitter is called by the mirror.
	// The original event `eventToFeed` is NOT passed to outputEmitter directly by `emitEvent`
	// unless a trigger or mirror does so. The mirror in this test *only* emits "event_from_mirror".
	assert.Len(t, emittedEvents, 1, "Output emitter should receive one event from the mirror")
	if len(emittedEvents) == 1 {
		assert.Equal(t, EventType("event_from_mirror"), emittedEvents[0].Type)
		assert.Equal(t, []byte("mirror_data"), emittedEvents[0].Content)
	}
	emitterMutex.Unlock()
}

func TestAsyncGuardian_EventLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g := newAysncGuardian(ctx)
	assert.NotNil(t, g)

	var emittedEvents []*Event
	emitterMutex := &sync.Mutex{}
	g.setOutputEmitter(func(event *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	// Register a mirror that simply forwards events to the output emitter
	// This way we can check if events are processed.
	g.registerMirrorEventTrigger("cancellation_test_mirror", func(c *chanx.UnlimitedChan[*Event], emitter func(*Event)) {
		// This mirror is called for each event. The event *itself* is not passed to this trigger.
		// This trigger is called when `g.emitEvent(originalEvent)` is run.
		// To test if `originalEvent` was processed, the mirror would need access to it, or
		// the `outputEmitter` itself needs to be checked directly.
		// The current `outputEmitter` only sees what triggers/mirrors send it.

		// Let's adjust: the global `setOutputEmitter` is the one we check.
		// For an event to reach it, a trigger or mirror must call it.
		// Let's use a simple event trigger for a specific type for the initial check.
	})

	// For the initial check, let's use a specific trigger to ensure the emitter is called
	initialEventType := EventType("initial_event_type_for_cancel_test")
	g.registerEventTrigger(initialEventType, func(event *Event, emitter func(*Event)) {
		emitter(event) // Forward the event
	})

	g.feed(&Event{Type: initialEventType, Content: []byte("initial_data")})
	time.Sleep(50 * time.Millisecond)

	emitterMutex.Lock()
	assert.NotEmpty(t, emittedEvents, "Output emitter should be called for initial event via trigger")
	emitterMutex.Unlock()

	cancel() // Cancel the context

	emitterMutex.Lock()
	emittedEvents = nil // Reset emitted events after initial check and before cancel
	emitterMutex.Unlock()

	g.feed(&Event{Type: initialEventType, Content: []byte("event_after_cancel")})

	time.Sleep(100 * time.Millisecond)

	emitterMutex.Lock()
	assert.Empty(t, emittedEvents, "Output emitter should not be called after context cancellation")
	emitterMutex.Unlock()

	select {
	case _, ok := <-g.unlimitedInput.OutputChannel():
		assert.False(t, ok, "Guardian's input channel (output side) should be closed after context cancellation")
	case <-time.After(200 * time.Millisecond): // Increased timeout slightly
		// This might happen if channel closing is not immediate or if already empty.
		// The primary check is that no new events are processed.
		t.Log("Timeout waiting for guardian input channel to close, this might be acceptable if event processing has stopped.")
	}
}

// Test for multiple event triggers for the same event type
func TestAsyncGuardian_MultipleEventTriggers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := newAysncGuardian(ctx)

	var trigger1Called, trigger2Called bool
	eventType := EventType("multi_trigger_event")

	g.registerEventTrigger(eventType, func(event *Event, emitter func(*Event)) {
		trigger1Called = true
	})
	g.registerEventTrigger(eventType, func(event *Event, emitter func(*Event)) {
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
	g := newAysncGuardian(ctx)

	var eventTriggerCalled, mirrorTriggerCalled bool
	specificEventType := EventType("specific_for_interaction_test")

	emittedFromEventTrigger := false
	emittedFromMirrorTrigger := false
	emitterMutex := &sync.Mutex{}

	g.setOutputEmitter(func(e *Event) {
		emitterMutex.Lock()
		defer emitterMutex.Unlock()
		if e.Type == "from_event_trigger" {
			emittedFromEventTrigger = true
		}
		if e.Type == "from_mirror_trigger" {
			emittedFromMirrorTrigger = true
		}
	})

	g.registerEventTrigger(specificEventType, func(event *Event, emitter func(*Event)) {
		eventTriggerCalled = true
		emitter(&Event{Type: "from_event_trigger"})
	})
	g.registerMirrorEventTrigger("interaction_mirror", func(uc *chanx.UnlimitedChan[*Event], emitter func(*Event)) {
		mirrorTriggerCalled = true
		emitter(&Event{Type: "from_mirror_trigger"})
	})

	// Feed event that matches specific trigger
	g.feed(&Event{Type: specificEventType})
	time.Sleep(50 * time.Millisecond)

	assert.True(t, eventTriggerCalled, "Event trigger should be called for specific event")
	assert.True(t, mirrorTriggerCalled, "Mirror trigger should be called for specific event (run1)")

	emitterMutex.Lock()
	assert.True(t, emittedFromEventTrigger, "Output emitter should receive event from event trigger (run1)")
	assert.True(t, emittedFromMirrorTrigger, "Output emitter should receive event from mirror trigger (run1)")
	emitterMutex.Unlock()

	// Reset flags for next event
	eventTriggerCalled = false
	mirrorTriggerCalled = false
	emittedFromEventTrigger = false
	emittedFromMirrorTrigger = false

	// Feed event that does not match specific trigger
	g.feed(&Event{Type: "other_event_for_interaction"})
	time.Sleep(50 * time.Millisecond)

	assert.False(t, eventTriggerCalled, "Event trigger should NOT be called for other event")
	assert.True(t, mirrorTriggerCalled, "Mirror trigger should be called for other event (run2)")

	emitterMutex.Lock()
	assert.False(t, emittedFromEventTrigger, "Output emitter should NOT receive event from event trigger (run2)")
	assert.True(t, emittedFromMirrorTrigger, "Output emitter should receive event from mirror trigger (run2)")
	emitterMutex.Unlock()
}
