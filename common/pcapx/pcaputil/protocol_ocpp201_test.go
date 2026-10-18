package pcaputil

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The 2.0.1 action/schema cannot be inferred from the 1.6 carrier or fields.
func TestProtocolSessionOCPP201BootHeartbeat(t *testing.T) {
	s := ocppTestSession(t, "ocpp1.6, ocpp2.0.1", "ocpp2.0.1")
	boot := `[2,"boot","BootNotification",{"chargingStation":{"model":"Sim","vendorName":"MVP","modem":{"iccid":"test","customData":{"vendorId":"example","extra":{"literal":1}}}},"reason":"PowerUp","customData":{"vendorId":"example","flag":true}}]`
	r := s.Feed(0, time.Time{}, ocppTestFrame(0, boot))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	app, ok := r.Events[0].Session["OCPP"].(map[string]any)
	require.True(t, ok, "negotiated OCPP2.0.1 requires its independent action/schema observation")
	require.Equal(t, "2.0.1", app["Version"])
	require.Equal(t, "observed", app["Context Status"])
	require.Equal(t, "boot", app["Unique ID"])
	var want []any
	dec := json.NewDecoder(strings.NewReader(boot))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&want))
	require.Equal(t, want[3], app["Payload"])
	id := r.Events[0].ID
	response := `[3,"boot",{"currentTime":"2026-10-12T00:00:00Z","interval":300,"status":"Accepted","statusInfo":{"reasonCode":"ok","additionalInfo":"literal","customData":{"vendorId":"example"}}}]`
	r = s.Feed(1, time.Time{}, ocppTestFrame(1, response))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, id, r.Events[0].ResponseTo)
	require.Equal(t, id, r.Events[0].TransactionID)
	require.Equal(t, "matched", r.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
	r = s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"hb","Heartbeat",{"customData":{"vendorId":"example","extension":[1,true,"raw"]}}]`))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	id = r.Events[0].ID
	r = s.Feed(1, time.Time{}, ocppTestFrame(1, `[3,"hb",{"currentTime":"2026-10-12T00:00:01Z","customData":{"vendorId":"example"}}]`))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, id, r.Events[0].ResponseTo)
	require.Equal(t, id, r.Events[0].TransactionID)
	require.Equal(t, "matched", r.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}

func TestProtocolSessionOCPP201ResourceRelease(t *testing.T) { testOCPPResourceRelease(t, "2.0.1") }

func TestProtocolSessionOCPP201IndependentSchema(t *testing.T) {
	raw, err := trafficfixture.ReadFile("testdata/ocpp201-rpc/schema-inputs.json")
	require.NoError(t, err)
	var cases []struct {
		Name, Schema string
		Payload      any
		Valid        bool
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	require.Len(t, cases, 47)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			s := ocppTestSession(t, "ocpp2.0.1, ocpp1.6", "ocpp2.0.1")
			action := "Heartbeat"
			if strings.HasPrefix(c.Schema, "BootNotification") {
				action = "BootNotification"
			}
			response := strings.HasSuffix(c.Schema, "Response")
			dir := 0
			var wire []byte
			var request uint64
			if response {
				payload := any(map[string]any{})
				if action == "BootNotification" {
					payload = map[string]any{"chargingStation": map[string]any{"model": "Sim", "vendorName": "MVP"}, "reason": "PowerUp"}
				}
				req, err := json.Marshal([]any{2, "test", action, payload})
				require.NoError(t, err)
				result := s.Feed(0, time.Time{}, ocppTestFrame(0, string(req)))
				require.Nil(t, result.Err)
				require.Len(t, result.Events, 1)
				request = result.Events[0].ID
				dir = 1
				wire, err = json.Marshal([]any{3, "test", c.Payload})
			} else {
				wire, err = json.Marshal([]any{2, "test", action, c.Payload})
			}
			require.NoError(t, err)
			r := s.Feed(dir, time.Time{}, ocppTestFrame(dir, string(wire)))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			event := r.Events[0]
			app := event.Session["OCPP"].(map[string]any)
			require.Equal(t, "2.0.1", app["Version"])
			require.Equal(t, action, app["Action"])
			require.Equal(t, "test", app["Unique ID"])
			if c.Valid {
				status := "observed"
				if response {
					status = "matched"
					require.Equal(t, request, event.ResponseTo)
					require.Equal(t, request, event.TransactionID)
				} else {
					require.Zero(t, event.ResponseTo)
					require.Equal(t, event.ID, event.TransactionID)
				}
				require.Equal(t, map[string]any{"Version": "2.0.1", "Action": action, "Unique ID": "test", "Message Type": map[bool]string{false: "2", true: "3"}[response], "Context Status": status, "Payload": c.Payload}, app)
			} else {
				require.Equal(t, "malformed", app["Context Status"])
				require.NotEmpty(t, app["Reason"])
				require.Zero(t, event.ResponseTo)
				require.Zero(t, event.TransactionID)
				if _, ok := c.Payload.(map[string]any); ok {
					require.Equal(t, c.Payload, app["Payload"])
				}
			}
			fields, err := event.GetFields()
			require.NoError(t, err)
			require.Equal(t, app, fields["OCPP"])
			owned := cloneSession(event.Session)
			event.Session = map[string]any{"forged": true}
			event.Raw[0] ^= 255
			event.Fields = map[string]any{"forged": true}
			fields["OCPP"] = map[string]any{"forged": true}
			decoded, err := event.Decode()
			require.NoError(t, err)
			require.Equal(t, owned, decoded["session"])
			fields, err = event.GetFields()
			require.NoError(t, err)
			require.Equal(t, owned["OCPP"], fields["OCPP"])
			// A malformed response retires the exact pending request. A late valid
			// reply cannot turn this refusal into a successful association.
			if response && !c.Valid {
				payload := map[string]any{"currentTime": "2026-10-12T00:00:00Z"}
				if action == "BootNotification" {
					payload["interval"] = 300
					payload["status"] = "Accepted"
				}
				late, err := json.Marshal([]any{3, "test", payload})
				require.NoError(t, err)
				after := s.Feed(1, time.Time{}, ocppTestFrame(1, string(late)))
				require.Nil(t, after.Err)
				require.Len(t, after.Events, 1)
				require.Equal(t, "context-required", after.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
				require.Zero(t, after.Events[0].ResponseTo)
				require.Zero(t, after.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Nil(t, s.Close("again"))
		})
	}
}
