package browsertools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type handoffCaller struct {
	mu    sync.Mutex
	state string
}

func (f *handoffCaller) CallDevice(
	_ context.Context,
	_ string,
	method string,
	_ interface{},
) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if method == "browser.handoff.status" {
		f.state = "completed"
	}
	return json.RawMessage(`{
		"id":"handoff-1","reason":"qr_code","state":"` + f.state + `","requestedAt":1,
		"target":{"tabId":7,"frameId":0,"title":"Sign in","grantedUrl":"https://example.test/login","origin":"https://example.test"}
	}`), nil
}

func TestCallAgentCapabilityEmitsMetadataOnlyAndWaitsForResolution(t *testing.T) {
	caller := &handoffCaller{state: "waiting_for_user"}
	var events []browserHandoffEvent
	runtime := &aitool.ToolRuntimeConfig{
		RuntimeID: "call-1",
		EmitUIEvent: func(eventType schema.EventType, _ string, payload any) error {
			require.Equal(t, schema.EVENT_TYPE_BROWSER_HANDOFF, eventType)
			events = append(events, payload.(browserHandoffEvent))
			return nil
		},
	}

	result, err := callAgentCapability(
		context.Background(), caller, "device-a", Target{}, "browser.handoff.request",
		aitool.InvokeParams{}, ReadTimeout, false, runtime,
	)
	require.NoError(t, err)
	require.Equal(t, "completed", result.(map[string]interface{})["state"])
	require.Len(t, events, 2)
	require.Equal(t, "waiting_for_user", events[0].State)
	require.Equal(t, "completed", events[1].State)
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "data:image")
	require.NotContains(t, string(encoded), "base64")
}
