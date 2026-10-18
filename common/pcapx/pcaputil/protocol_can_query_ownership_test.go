package pcaputil

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
)

// Reuse immutable accepted controller and DSDL inputs. Queries must retain the
// complete independently authored original Session after caller mutation.
func testSocketCANPrivateSessionQueries(t *testing.T) {
	type input struct {
		name      string
		raw       []byte
		selection string
		fields    map[string]any
	}
	inputs := []input{}
	for _, c := range canControls(t) {
		raw, a := canInput(t, c)
		if a.Fields != nil {
			inputs = append(inputs, input{c.ID, raw, "", a.Fields})
		}
		if a.SelectedFields != nil {
			inputs = append(inputs, input{c.ID + "-selected", raw, "j1939", a.SelectedFields})
		}
	}
	raw, err := trafficfixture.ReadFile("dronecan-node-status/manifest.json")
	require.NoError(t, err)
	var m struct{ Cases []rocControl }
	require.NoError(t, json.Unmarshal(raw, &m))
	for _, c := range m.Cases {
		b, err := trafficfixture.ReadFile("dronecan-node-status/" + c.Answer)
		require.NoError(t, err)
		var a struct{ Fields map[string]any }
		require.NoError(t, json.Unmarshal(b, &a))
		if a.Fields == nil {
			continue
		}
		capture, err := trafficfixture.ReadFile("dronecan-node-status/" + c.Capture)
		require.NoError(t, err)
		inputs = append(inputs, input{c.ID + "-dronecan", capture, "dronecan", a.Fields})
	}
	require.NotEmpty(t, inputs)
	for _, c := range inputs {
		t.Run(c.name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				var events []*ProtocolEvent
				opts := []CaptureOption{WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
				if c.selection != "" {
					opts = append(opts, WithCANDecodeAs(0, c.selection))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(c.raw), opts...))
				require.Len(t, events, 1)
				e := events[0]
				require.Empty(t, e.Error)
				rocEqualFields(t, c.fields, e.Session)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.FlowID)
				before, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, c.fields, before["session"].(map[string]any))
				before["session"].(map[string]any)["Identifier"] = "forged"
				e.Protocol, e.Transport = "websocket", "tcp"
				e.Session = map[string]any{"forged": true}
				e.Fields = map[string]any{"forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Raw[0] ^= 255
				after, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, c.fields, after["session"].(map[string]any))
				fields, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.fields, fields)
			}
		})
	}
}

func testSocketCANRefusalQueries(t *testing.T) {
	for _, c := range canControls(t) {
		raw, a := canInput(t, c)
		if a.Fields != nil || a.ReaderError != "" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 1)
			e := events[0]
			require.NotNil(t, e.sessionError)
			expected := *e.sessionError
			e.Protocol, e.Transport, e.Status, e.Error = "websocket", "tcp", "decoded", ""
			e.Fields = map[string]any{"forged": true}
			e.Structured = map[string]any{"fields": e.Fields}
			e.Session = map[string]any{"forged": true}
			fields, err := e.GetFields()
			require.Nil(t, fields)
			require.Equal(t, &expected, err)
			decoded, err := e.Decode()
			require.Nil(t, decoded)
			require.Equal(t, &expected, err)
			// Returned error pointers must be copies of the retained refusal.
			err.(*ProtocolError).Message = "forged error"
			_, fresh := e.Decode()
			require.Equal(t, &expected, fresh)
		})
	}
}
