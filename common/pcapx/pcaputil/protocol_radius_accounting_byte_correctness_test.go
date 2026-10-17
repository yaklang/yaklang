package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func abAttrs(parts ...[]byte) []byte {
	var w []byte
	for _, p := range parts {
		w = append(w, p...)
	}
	return w
}
func abAccess(id byte) []byte {
	return radiusPkt(1, id, abAttrs(radiusAttr(1, []byte("neighbor")), radiusAttr(4, []byte{192, 0, 2, 9}), radiusAttr(24, []byte("observed-state"))))
}
func TestAccountingByteIndependentNormRefusalKeepsNeighbor(t *testing.T) {
	s, e := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, e)
	defer s.Close("cleanup")
	neighbor := abAccess(8)
	o := s.Feed(0, time.Unix(100, 0), neighbor)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	owner := o.Events[0].ID
	require.NotZero(t, owner)
	require.Equal(t, owner, o.Events[0].TransactionID)
	require.Zero(t, o.Events[0].ResponseTo)
	attrs := abAttrs(radiusAttr(4, []byte{192, 0, 2, 9}), radiusAttr(40, []byte{0, 0, 0, 2}), radiusAttr(44, []byte("session")), radiusAttr(41, []byte{0, 0, 1}))
	bad := radiusPkt(4, 7, attrs)
	o = s.Feed(0, time.Unix(101, 0), bad)
	require.NotNil(t, o.Err)
	require.Equal(t, ErrMalformedMessage, o.Err.Kind)
	require.Len(t, o.Events, 1)
	v := o.Events[0]
	require.Equal(t, "radius", v.Protocol)
	require.Equal(t, "radius-accounting-observed", v.Profile)
	require.Equal(t, "malformed", v.Status)
	require.Equal(t, "malformed", v.Completeness)
	require.Equal(t, bad, v.Raw)
	require.Nil(t, v.Session)
	require.Zero(t, v.TransactionID)
	require.Zero(t, v.ResponseTo)
	f, e := v.GetFields()
	require.Nil(t, f)
	rocTypedError(t, "MalformedMessage", e)
	answer := radiusPkt(2, 8, nil)
	o = s.Feed(1, time.Unix(102, 0), answer)
	require.Nil(t, o.Err, "completeCode4 semanticrefusal mustnotinvalidate unrelated liveAccessowner")
	require.Len(t, o.Events, 1)
	v = o.Events[0]
	require.Equal(t, owner, v.TransactionID)
	require.Equal(t, owner, v.ResponseTo)
	require.Equal(t, "Access-Request", v.Session["In Reply To"])
	require.Equal(t, answer, v.Raw)
	late := radiusPkt(5, 7, nil)
	o = s.Feed(1, time.Unix(103, 0), late)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	v = o.Events[0]
	require.Equal(t, "ambiguous-identifier", v.Session["Association"])
	require.Zero(t, v.TransactionID)
	require.Zero(t, v.ResponseTo)
	require.Equal(t, late, v.Raw)
	require.Empty(t, s.Close("first"))
	require.Empty(t, s.Close("second"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestRADIUSNativeIndependentNASPortBoundary(t *testing.T) {
	for _, c := range []struct {
		name      string
		v         []byte
		duplicate bool
		invalid   bool
	}{{"width3", []byte{1, 2, 3}, false, true}, {"duplicate", []byte{1, 2, 3, 4}, true, true}, {"width4-adjacent", []byte{1, 2, 3, 4}, false, false}} {
		t.Run(c.name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
			require.NoError(t, e)
			defer s.Close("cleanup")
			attrs := abAttrs(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9}), radiusAttr(24, []byte("observed-state")), radiusAttr(5, c.v))
			if c.duplicate {
				attrs = append(attrs, radiusAttr(5, []byte{5, 6, 7, 8})...)
			}
			w := radiusPkt(1, 7, attrs)
			o := s.Feed(0, time.Unix(100, 0), w)
			if c.invalid {
				require.NotNil(t, o.Err, "RFC2865 fixedwidth/singleton boundary is currentlyignored")
				require.Equal(t, ErrMalformedMessage, o.Err.Kind)
				require.Len(t, o.Events, 1)
				v := o.Events[0]
				require.Equal(t, "malformed", v.Status)
				require.Equal(t, "malformed", v.Completeness)
				require.Equal(t, w, v.Raw)
				require.Nil(t, v.Session)
				require.Zero(t, v.TransactionID)
				require.Zero(t, v.ResponseTo)
			} else {
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				v := o.Events[0]
				want := map[string]any{"Packet Name": "Access-Request", "Code": 1, "Identifier": 7, "Authenticator": []byte("0123456789abcdef"), "Packet Length": len(w), "Padding Bytes": 0, "Authentication": "not-verified", "Role": "request", "User-Name": "alice", "NAS-IP-Address": "192.0.2.9", "Attribute Count": 4, "Attributes": []map[string]any{{"Type": byte(1), "Raw": []byte("alice")}, {"Type": byte(4), "Raw": []byte{192, 0, 2, 9}}, {"Type": byte(24), "Raw": []byte("observed-state")}, {"Type": byte(5), "Raw": []byte{1, 2, 3, 4}}}, "Observation Scope": "conversation"}
				require.Equal(t, want, v.Session)
				f, e := v.GetFields()
				require.NoError(t, e)
				require.Equal(t, want, f)
				owner := v.ID
				require.NotZero(t, owner)
				require.Equal(t, owner, v.TransactionID)
				require.Zero(t, v.ResponseTo)
				o = s.Feed(1, time.Unix(101, 0), radiusPkt(2, 7, nil))
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				require.Equal(t, owner, o.Events[0].TransactionID)
				require.Equal(t, owner, o.Events[0].ResponseTo)
				require.Equal(t, "Access-Request", o.Events[0].Session["In Reply To"])
			}
			require.Empty(t, s.Close("first"))
			require.Empty(t, s.Close("second"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
