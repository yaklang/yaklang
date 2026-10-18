package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// A valid unknown method is unsupported; it cannot hide a simultaneous MUST
// violation. These are boundary observations; the reserved test method deliberately stays
// separate from the implemented EAP-TLS Type13 profile.
func TestRADIUSEAPErrorPrecedence(t *testing.T) {
	for _, c := range []struct {
		name string
		typ  byte
		mac  bool
		kind string
	}{
		{"unknown-method-missing-mac", 255, false, "MalformedMessage"},
		{"unknown-method-valid-mac", 255, true, "UnsupportedFeature"},
		{"identity-missing-mac", 1, false, "MalformedMessage"},
	} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.name, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				ep := []byte{2, 1, 0, 6, c.typ, 'x'}
				attrs := append(radiusAttr(4, []byte{192, 0, 2, 9}), radiusAttr(79, ep)...)
				wantedAttrs := []map[string]any{{"Type": byte(4), "Raw": []byte{192, 0, 2, 9}}, {"Type": byte(79), "Raw": ep}}
				diag := []string{}
				if c.mac {
					attrs = append(attrs, radiusAttr(80, make([]byte, 16))...)
					wantedAttrs = append(wantedAttrs, map[string]any{"Type": byte(80), "Raw": make([]byte, 16)})
				} else {
					diag = append(diag, "eap-message-authenticator-missing")
				}
				inner := map[string]any{"Code": byte(2), "Identifier": byte(1), "Length": uint16(6), "Type": c.typ, "Type Data": []byte{'x'}, "Raw": ep, "Fragments": 1, "Padding Bytes": 0}
				if c.typ == 1 {
					inner["Type Name"] = "Identity"
					inner["Identity"] = []byte{'x'}
				} else {
					diag = append(diag, "eap-method-unimplemented")
				}
				wire := radiusPkt(1, 7, attrs)
				want := map[string]any{"Packet Name": "Access-Request", "Code": 1, "Identifier": 7, "Packet Length": len(wire), "Padding Bytes": 0, "Authenticator": []byte("0123456789abcdef"), "Authentication": "not-verified", "Role": "request", "Attributes": wantedAttrs, "Attribute Count": len(wantedAttrs), "EAP": inner, "EAP Diagnostics": diag, "Observation Scope": "conversation", "NAS-IP-Address": "192.0.2.9"}
				if c.mac {
					want["Message-Authenticator"] = make([]byte, 16)
				}
				o := s.Feed(0, time.Unix(100, 0), wire)
				rocTypedError(t, c.kind, o.Err)
				require.Equal(t, len(wire), o.Consumed)
				require.Len(t, o.Events, 1)
				v := o.Events[0]
				require.Equal(t, "radius", v.Protocol)
				require.Equal(t, "radius-eap-observed", v.Profile)
				require.Equal(t, wire, v.Raw)
				require.Equal(t, len(wire), v.Length)
				require.Equal(t, want, v.Session)
				require.Zero(t, v.TransactionID)
				require.Zero(t, v.ResponseTo)
				status := "malformed"
				if c.kind == "UnsupportedFeature" {
					status = "context-required"
				}
				require.Equal(t, status, v.Status)
				require.Equal(t, status, v.Completeness)
				f, err := v.GetFields()
				require.Nil(t, f)
				rocTypedError(t, c.kind, err)
				tlsDTLSPoison(v, "http")
				f, err = v.GetFields()
				require.Nil(t, f)
				rocTypedError(t, c.kind, err)
				qwire := radiusPkt(1, 8, append(radiusAttr(1, []byte("neighbor")), radiusAttr(4, []byte{192, 0, 2, 9})...))
				q := s.Feed(0, time.Unix(101, 0), qwire)
				require.Nil(t, q.Err)
				require.Len(t, q.Events, 1)
				require.Equal(t, eapGuardCanonical(qwire, "neighbor", "", ""), func() map[string]any {
					m := cloneSession(q.Events[0].Session)
					delete(m, "Observation Scope")
					return m
				}())
				id := q.Events[0].ID
				require.NotZero(t, id)
				require.Equal(t, id, q.Events[0].TransactionID)
				require.Zero(t, q.Events[0].ResponseTo)
				r := s.Feed(1, time.Unix(102, 0), radiusPkt(2, 8, nil))
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				require.Equal(t, id, r.Events[0].TransactionID)
				require.Equal(t, id, r.Events[0].ResponseTo)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
