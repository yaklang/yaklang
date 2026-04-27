package schema

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
)

func TestRuntimeScopedBroadcast_MultiSubscriberAndThrottle(t *testing.T) {
	restore := resetRuntimeScopedBroadcastForTest(0.02)
	defer restore()

	firstCh := make(chan *RuntimeScopedBroadcastEvent, 4)
	secondCh := make(chan *RuntimeScopedBroadcastEvent, 4)
	otherRuntimeCh := make(chan *RuntimeScopedBroadcastEvent, 1)

	unsubscribeFirst := SubscribeRuntimeScopedBroadcast("runtime-a", func(event *RuntimeScopedBroadcastEvent) {
		firstCh <- event
	})
	defer unsubscribeFirst()

	unsubscribeSecond := SubscribeRuntimeScopedBroadcast("runtime-a", func(event *RuntimeScopedBroadcastEvent) {
		secondCh <- event
	})
	defer unsubscribeSecond()

	unsubscribeOther := SubscribeRuntimeScopedBroadcast("runtime-b", func(event *RuntimeScopedBroadcastEvent) {
		otherRuntimeCh <- event
	})
	defer unsubscribeOther()

	PublishRuntimeScopedBroadcast(RuntimeScopedBroadcastTypeHTTPFlow, "runtime-a", "update", 1)
	PublishRuntimeScopedBroadcast(RuntimeScopedBroadcastTypeHTTPFlow, "runtime-a", "update", 1)
	PublishRuntimeScopedBroadcast(RuntimeScopedBroadcastTypeHTTPFlow, "runtime-a", "create", 2)

	firstEventA := waitRuntimeScopedBroadcastEvent(t, firstCh)
	firstEventB := waitRuntimeScopedBroadcastEvent(t, firstCh)
	secondEventA := waitRuntimeScopedBroadcastEvent(t, secondCh)
	secondEventB := waitRuntimeScopedBroadcastEvent(t, secondCh)

	require.Equal(t, RuntimeScopedBroadcastTypeHTTPFlow, firstEventA.Type)
	require.Equal(t, RuntimeScopedBroadcastTypeHTTPFlow, firstEventB.Type)
	require.Equal(t, "runtime-a", firstEventA.RuntimeID)
	require.Equal(t, "runtime-a", firstEventB.RuntimeID)
	require.Equal(t, firstEventA.Type, secondEventA.Type)
	require.Equal(t, firstEventB.Type, secondEventB.Type)

	firstGot := map[string]uint{
		firstEventA.Action: firstEventA.ID,
		firstEventB.Action: firstEventB.ID,
	}
	secondGot := map[string]uint{
		secondEventA.Action: secondEventA.ID,
		secondEventB.Action: secondEventB.ID,
	}
	require.Equal(t, map[string]uint{
		"update": 1,
		"create": 2,
	}, firstGot)
	require.Equal(t, firstGot, secondGot)

	select {
	case event := <-otherRuntimeCh:
		t.Fatalf("unexpected event for unrelated runtime: %+v", event)
	case <-time.After(80 * time.Millisecond):
	}

	select {
	case event := <-firstCh:
		t.Fatalf("duplicate throttled event delivered to first subscriber: %+v", event)
	case <-time.After(80 * time.Millisecond):
	}
	select {
	case event := <-secondCh:
		t.Fatalf("duplicate throttled event delivered to second subscriber: %+v", event)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestRuntimeScopedBroadcast_HTTPFlowHooks(t *testing.T) {
	restore := resetRuntimeScopedBroadcastForTest(0.02)
	defer restore()

	events := make(chan *RuntimeScopedBroadcastEvent, 4)
	unsubscribe := SubscribeRuntimeScopedBroadcast("runtime-httpflow", func(event *RuntimeScopedBroadcastEvent) {
		events <- event
	})
	defer unsubscribe()

	flow := &HTTPFlow{
		Model:     gorm.Model{ID: 7},
		RuntimeId: "runtime-httpflow",
	}
	require.NoError(t, flow.AfterCreate(nil))
	require.NoError(t, flow.AfterUpdate(nil))

	first := waitRuntimeScopedBroadcastEvent(t, events)
	second := waitRuntimeScopedBroadcastEvent(t, events)
	got := map[string]uint{
		first.Action:  first.ID,
		second.Action: second.ID,
	}
	require.Equal(t, map[string]uint{
		"create": 7,
		"update": 7,
	}, got)
}

func TestRuntimeScopedBroadcast_RiskHooksAndUnsubscribe(t *testing.T) {
	restore := resetRuntimeScopedBroadcastForTest(0.02)
	defer restore()

	events := make(chan *RuntimeScopedBroadcastEvent, 4)
	unsubscribe := SubscribeRuntimeScopedBroadcast("runtime-risk", func(event *RuntimeScopedBroadcastEvent) {
		events <- event
	})

	risk := &Risk{
		Model:     gorm.Model{ID: 11},
		RuntimeId: "runtime-risk",
	}
	require.NoError(t, risk.AfterCreate(nil))
	require.NoError(t, risk.AfterDelete(nil))

	first := waitRuntimeScopedBroadcastEvent(t, events)
	second := waitRuntimeScopedBroadcastEvent(t, events)
	require.Equal(t, RuntimeScopedBroadcastTypeRisk, first.Type)
	require.Equal(t, RuntimeScopedBroadcastTypeRisk, second.Type)
	got := map[string]uint{
		first.Action:  first.ID,
		second.Action: second.ID,
	}
	require.Equal(t, map[string]uint{
		"create": 11,
		"delete": 11,
	}, got)

	unsubscribe()
	PublishRuntimeScopedBroadcast(RuntimeScopedBroadcastTypeRisk, "runtime-risk", "update", 11)

	select {
	case got := <-events:
		t.Fatalf("unexpected event after unsubscribe: %+v", got)
	case <-time.After(80 * time.Millisecond):
	}
}

func resetRuntimeScopedBroadcastForTest(interval float64) func() {
	oldCenter := runtimeBroadcastData
	oldInterval := runtimeScopedBroadcastThrottleInterval

	runtimeScopedBroadcastThrottleInterval = interval
	runtimeBroadcastData = newRuntimeScopedBroadcastCenter()

	return func() {
		runtimeBroadcastData = oldCenter
		runtimeScopedBroadcastThrottleInterval = oldInterval
	}
}

func waitRuntimeScopedBroadcastEvent(t *testing.T, ch <-chan *RuntimeScopedBroadcastEvent) *RuntimeScopedBroadcastEvent {
	t.Helper()

	select {
	case event := <-ch:
		return event
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timeout waiting for runtime scoped broadcast event")
		return nil
	}
}
