package pcaputil

import (
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func nasPresenceIndependentTyped(t *testing.T, s map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(s)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	for _, k := range []string{"Code", "Identifier", "Packet Length", "Padding Bytes", "Attribute Count"} {
		if v, ok := m[k].(float64); ok {
			m[k] = int(v)
		}
	}
	for _, k := range []string{"Authenticator", "Message-Authenticator"} {
		if v, ok := m[k].(string); ok {
			w, e := hex.DecodeString(v)
			require.NoError(t, e)
			m[k] = w
		}
	}
	as := make([]map[string]any, 0)
	for _, v := range m["Attributes"].([]any) {
		a := v.(map[string]any)
		b, e := hex.DecodeString(a["Raw"].(string))
		require.NoError(t, e)
		as = append(as, map[string]any{"Type": byte(a["Type"].(float64)), "Raw": b})
	}
	m["Attributes"] = as
	if a, ok := m["EAP Diagnostics"].([]any); ok {
		v := make([]string, len(a))
		for i, x := range a {
			v[i] = x.(string)
		}
		m["EAP Diagnostics"] = v
	}
	if e, ok := m["EAP"].(map[string]any); ok {
		for _, k := range []string{"Code", "Identifier", "Type"} {
			if v, ok := e[k].(float64); ok {
				e[k] = byte(v)
			}
		}
		for _, k := range []string{"Fragments", "Padding Bytes", "Method Count"} {
			if v, ok := e[k].(float64); ok {
				e[k] = int(v)
			}
		}
		if v, ok := e["Length"].(float64); ok {
			e["Length"] = uint16(v)
		}
		for _, k := range []string{"Vendor-Id", "Vendor-Type"} {
			if v, ok := e[k].(float64); ok {
				e[k] = uint32(v)
			}
		}
		for _, k := range []string{"Raw", "Type Data", "Desired Types", "Vendor Data", "Identity", "Prompt"} {
			if v, ok := e[k].(string); ok {
				b, err := hex.DecodeString(v)
				require.NoError(t, err)
				e[k] = b
			}
		}
		if a, ok := e["Desired Expanded Types"].([]any); ok {
			rows := make([]map[string]any, len(a))
			for i, v := range a {
				x := v.(map[string]any)
				rows[i] = map[string]any{"Vendor-Id": uint32(x["Vendor-Id"].(float64)), "Vendor-Type": uint32(x["Vendor-Type"].(float64))}
			}
			e["Desired Expanded Types"] = rows
		}
	}
	return m
}
func TestRADIUSAccessNASPresenceIndependent(t *testing.T) {
	var root struct {
		Cases []struct {
			Steps []struct {
				Wire      string         `json:"input_hex"`
				Fields    map[string]any `json:"fields_hexbytes"`
				Direction int
				At        int64 `json:"timestamp_unix"`
			}
		}
	}
	b := finsReadBatch(t, "radius-nak", "RADIUS_NAK_TEST_ROOT", "nas-presence.json")
	require.NoError(t, json.Unmarshal(b, &root))
	steps := root.Cases[0].Steps
	t.Run("missing-NAS-cannot-own-late-reply", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, e)
		defer s.Close("cleanup")
		var events []*ProtocolEvent
		var firstErr *ProtocolError
		for i, x := range steps {
			w, e := hex.DecodeString(x.Wire)
			require.NoError(t, e)
			o := s.Feed(x.Direction, time.Unix(x.At, 0), w)
			require.Len(t, o.Events, 1)
			events = append(events, o.Events[0])
			if i == 0 {
				firstErr = o.Err
			}
			t.Logf("step=%d actual_error=%v Session=%v Tx=%d ResponseTo=%d", i, o.Err, o.Events[0].Session, o.Events[0].TransactionID, o.Events[0].ResponseTo)
		}
		require.NotNil(t, firstErr, "RFC2865 4.1 +RFC3579 3 require NASidentity; oldimplementation acceptsandowns invalidrequest")
		require.Equal(t, ErrMalformedMessage, firstErr.Kind)
		bad := events[0]
		require.Nil(t, bad.Session)
		require.Equal(t, "radius-access-observed", bad.Profile)
		require.Equal(t, "malformed", bad.Completeness)
		require.Zero(t, bad.TransactionID)
		require.Zero(t, bad.ResponseTo)
		f, err := bad.GetFields()
		require.Nil(t, f)
		require.Error(t, err)
		require.Equal(t, ErrMalformedMessage, err.(*ProtocolError).Kind)
		require.Zero(t, events[2].TransactionID)
		require.Zero(t, events[2].ResponseTo)
		require.Equal(t, nasPresenceIndependentTyped(t, steps[2].Fields), events[2].Session)
		owner := events[1].ID
		require.Equal(t, owner, events[1].TransactionID)
		require.Equal(t, owner, events[3].TransactionID)
		require.Equal(t, owner, events[3].ResponseTo)
		require.Equal(t, nasPresenceIndependentTyped(t, steps[1].Fields), events[1].Session)
		require.Equal(t, nasPresenceIndependentTyped(t, steps[3].Fields), events[3].Session)
		for i, x := range steps {
			w, _ := hex.DecodeString(x.Wire)
			require.Equal(t, w, events[i].Raw)
			require.Equal(t, x.Direction, events[i].Direction)
			require.Equal(t, time.Unix(x.At, 0), events[i].Timestamp)
		}
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("NAS32-valid-neighbor", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, e)
		defer s.Close("cleanup")
		var owner uint64
		for _, i := range []int{1, 3} {
			x := steps[i]
			w, e := hex.DecodeString(x.Wire)
			require.NoError(t, e)
			o := s.Feed(x.Direction, time.Unix(x.At, 0), w)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			ev := o.Events[0]
			if i == 1 {
				owner = ev.ID
				require.Zero(t, ev.ResponseTo)
			} else {
				require.Equal(t, owner, ev.ResponseTo)
			}
			require.Equal(t, owner, ev.TransactionID)
			require.Equal(t, nasPresenceIndependentTyped(t, x.Fields), ev.Session)
			f, e := ev.GetFields()
			require.NoError(t, e)
			require.Equal(t, nasPresenceIndependentTyped(t, x.Fields), f)
			require.Equal(t, w, ev.Raw)
			require.Equal(t, x.Direction, ev.Direction)
			require.Equal(t, time.Unix(x.At, 0), ev.Timestamp)
		}
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
