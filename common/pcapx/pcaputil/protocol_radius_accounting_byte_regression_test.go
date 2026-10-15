package pcaputil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func accountingByteHex(t *testing.T, s string) []byte {
	t.Helper()
	w, e := hex.DecodeString(s)
	require.NoError(t, e)
	return w
}
func accountingByteWant(t *testing.T, in map[string]any, legacy bool) map[string]any {
	t.Helper()
	o := cloneSession(in)
	if legacy {
		o["Code"] = byte(o["Code"].(float64))
		o["Identifier"] = byte(o["Identifier"].(float64))
		o["Length"] = uint16(o["Length"].(float64))
	} else {
		for _, k := range []string{"Code", "Identifier", "Packet Length", "Padding Bytes", "Attribute Count"} {
			if v, ok := o[k].(float64); ok {
				o[k] = int(v)
			}
		}
		for _, k := range []string{"Acct-Status-Type", "Acct-Delay-Time", "Acct-Input-Octets", "Acct-Output-Octets", "Acct-Authentic", "Acct-Session-Time", "Acct-Input-Packets", "Acct-Output-Packets", "Acct-Terminate-Cause", "Acct-Input-Gigawords", "Acct-Output-Gigawords"} {
			if v, ok := o[k].(float64); ok {
				o[k] = uint32(v)
			}
		}
		for _, k := range []string{"Acct-Input-Octets64", "Acct-Output-Octets64"} {
			if v, ok := o[k].(string); ok {
				n, e := strconv.ParseUint(v, 10, 64)
				require.NoError(t, e)
				o[k] = n
			}
		}
	}
	for _, k := range []string{"Authenticator", "Acct-Session-Id"} {
		if v, ok := o[k].(string); ok {
			o[k] = accountingByteHex(t, v)
		}
	}
	if a, ok := o["Attributes"].([]any); ok {
		if legacy {
			rows := make([]any, 0, len(a))
			for _, v := range a {
				m := cloneSession(v.(map[string]any))
				m["Type"] = byte(m["Type"].(float64))
				m["Length"] = byte(m["Length"].(float64))
				for _, k := range []string{"User-Name", "User-Password", "Message-Authenticator", "Attribute Data"} {
					if s, ok := m[k].(string); ok {
						m[k] = string(accountingByteHex(t, s))
					}
				}
				if s, ok := m["NAS-IP-Address"].(string); ok {
					m["NAS-IP-Address"] = accountingByteHex(t, s)
				}
				if n, ok := m["NAS-Port"].(float64); ok {
					m["NAS-Port"] = uint32(n)
				}
				rows = append(rows, m)
			}
			o["Attributes"] = rows
		} else {
			rows := make([]map[string]any, 0, len(a))
			for _, v := range a {
				m := v.(map[string]any)
				rows = append(rows, map[string]any{"Type": byte(m["Type"].(float64)), "Raw": accountingByteHex(t, m["Raw"].(string))})
			}
			o["Attributes"] = rows
		}
	}
	return o
}
func TestRADIUSAccountingByteIndependentMissingAPI(t *testing.T) {
	t.Run("code4-first-code5-reply", func(t *testing.T) {
		raw := finsReadBatch(t, "radius-accounting-byte", "RADIUS_ACCOUNTING_BYTE_TEST_ROOT", "controls.json")
		var data struct {
			Selected struct {
				Steps []struct {
					Input     string `json:"input_hex"`
					Direction int
					At        int64          `json:"timestamp_unix"`
					Session   map[string]any `json:"expected_session_hexbytes"`
					Legacy    map[string]any `json:"legacy_fields_hexstrings"`
				} `json:"steps"`
			} `json:"selected_case"`
		}
		require.NoError(t, json.Unmarshal(raw, &data))
		s, e := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, e)
		defer s.Close("cleanup")
		var owner uint64
		for i, z := range data.Selected.Steps {
			w := accountingByteHex(t, z.Input)
			p := s.Probe(w)
			t.Logf("step%d probe=%+v input%d", i, p, len(w))
			out := s.Feed(z.Direction, time.Unix(z.At, 0), w)
			require.Nil(t, out.Err, "required Accounting bytecarrier behavior missing")
			require.Len(t, out.Events, 1)
			v := out.Events[0]
			require.Equal(t, "radius", v.Protocol)
			require.Equal(t, "radius-accounting-observed", v.Profile)
			require.Equal(t, w, v.Raw)
			require.Equal(t, len(w), v.Length)
			require.Equal(t, z.Direction, v.Direction)
			require.True(t, time.Unix(z.At, 0).Equal(v.Timestamp))
			require.NotZero(t, v.ID)
			if i == 0 {
				owner = v.ID
				require.Zero(t, v.ResponseTo)
			} else {
				require.Equal(t, owner, v.ResponseTo)
			}
			require.Equal(t, owner, v.TransactionID)
			want := accountingByteWant(t, z.Session, false)
			require.Equal(t, want, v.Session)
			require.Equal(t, "not-verified", v.Session["Authentication"])
			f, e := v.GetFields()
			require.NoError(t, e)
			require.Equal(t, accountingByteWant(t, z.Legacy, true), f)
			mutateRTPSQuery(f)
			clear(w)
			f, e = v.GetFields()
			require.NoError(t, e)
			require.Equal(t, accountingByteWant(t, z.Legacy, true), f)
		}
		require.Empty(t, s.Close("first"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("access-adjacent", func(t *testing.T) {
		s, e := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, e)
		defer s.Close("cleanup")
		q := radiusPkt(1, 7, append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...))
		r := radiusPkt(2, 7, nil)
		out := s.Feed(0, time.Unix(100, 0), q)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		owner := out.Events[0].ID
		require.Equal(t, owner, out.Events[0].TransactionID)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Equal(t, q, out.Events[0].Raw)
		require.Equal(t, "alice", out.Events[0].Session["User-Name"])
		out = s.Feed(1, time.Unix(101, 0), r)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		require.Equal(t, owner, out.Events[0].TransactionID)
		require.Equal(t, owner, out.Events[0].ResponseTo)
		require.Equal(t, r, out.Events[0].Raw)
		require.Equal(t, "Access-Request", out.Events[0].Session["In Reply To"])
		f, e := out.Events[0].GetFields()
		require.NoError(t, e)
		require.Equal(t, map[string]any{"Code": byte(2), "Identifier": byte(7), "Length": uint16(20), "Authenticator": []byte("0123456789abcdef")}, f)
		require.Empty(t, s.Close("first"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
func TestRADIUSAccountingByteIndependentAdmissionGuards(t *testing.T) {
	raw := finsReadBatch(t, "radius-accounting-byte", "RADIUS_ACCOUNTING_BYTE_TEST_ROOT", "controls.json")
	var data struct {
		Rows     []struct{ Name, Input, Protocol, Carrier string } `json:"unused"`
		Controls []struct {
			Name     string
			Input    string  `json:"input_hex"`
			Protocol *string `json:"expected_protocol"`
			Carrier  string
		} `json:"boundary_controls"`
	}
	require.NoError(t, json.Unmarshal(raw, &data))
	for _, c := range data.Controls {
		t.Run(c.Name, func(t *testing.T) {
			w := accountingByteHex(t, c.Input)
			var s ProtocolSession
			var e error
			switch c.Carrier {
			case "udp":
				s, e = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
			case "tcp":
				s, e = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(41000, 1813))
			default:
				s, e = NewProtocolSession(DefaultParserBudget())
			}
			require.NoError(t, e)
			defer s.Close("cleanup")
			p := s.Probe(w)
			o := s.Feed(0, time.Unix(100, 0), w)
			report := map[string]any{"probe": p, "feed_state": o.State, "error": o.Err}
			ev := make([]string, 0)
			for _, v := range o.Events {
				ev = append(ev, fmt.Sprintf("%s/%s/%s", v.Protocol, v.Status, v.Profile))
			}
			report["events"] = ev
			bb, e := json.Marshal(report)
			require.NoError(t, e)
			t.Log(string(bb))
			if c.Protocol == nil {
				require.NotEqual(t, "radius", p.Protocol)
				for _, v := range o.Events {
					require.NotEqual(t, "radius", v.Protocol)
				}
			} else {
				require.Equal(t, ProbeAccept, p.Verdict)
				require.Equal(t, *c.Protocol, p.Protocol)
				require.NotEmpty(t, o.Events)
				require.Equal(t, *c.Protocol, o.Events[0].Protocol)
			}
			s.Close("first")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
