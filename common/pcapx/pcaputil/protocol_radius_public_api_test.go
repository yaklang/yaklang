package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func agentRadiusRequest(id byte) []byte {
	return radiusPkt(1, id, append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...))
}
func TestRADIUSContextPublicAPI(t *testing.T) {
	t.Run("udp-adjacent-context", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		q := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(7))
		require.Nil(t, q.Err)
		require.Len(t, q.Events, 1)
		require.Equal(t, "Access-Request", q.Events[0].Session["Packet Name"])
		r := s.Feed(1, time.Unix(101, 0), radiusPkt(2, 7, nil))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "Access-Request", r.Events[0].Session["In Reply To"])
		require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
		require.Equal(t, q.Events[0].ID, r.Events[0].TransactionID)
	})
	t.Run("udp-rfc-padding", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		r := s.Feed(0, time.Unix(100, 0), append(agentRadiusRequest(7), 0xaa, 0xbb, 0xcc))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "radius", r.Events[0].Protocol)
		require.Equal(t, "alice", r.Events[0].Session["User-Name"])
	})
	t.Run("legacy-public-cache-protocol-input-ownership", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer s.Close("cleanup")
		wire := agentRadiusRequest(7)
		saved := bytes.Clone(wire)
		out := s.Feed(0, time.Unix(100, 0), wire)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		e := out.Events[0]
		f, err := e.GetFields()
		require.NoError(t, err)
		require.NotEmpty(t, f)
		snapshot := cloneSession(f)
		for k := range f {
			delete(f, k)
		}
		e.Fields = map[string]any{"forged": true}
		e.Structured = map[string]any{"forged": true}
		e.Session = map[string]any{"forged": true}
		e.Protocol = "http"
		clear(wire)
		clear(e.Raw)
		got, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, snapshot, got)
		require.NotEmpty(t, saved)
	})
	t.Run("legacy-challenge-new-id-adjacent", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer s.Close("cleanup")
		feed := func(d int, w []byte) *ProtocolEvent {
			o := s.Feed(d, time.Unix(100, 0), w)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			return o.Events[0]
		}
		feed(0, agentRadiusRequest(7))
		c := feed(1, radiusPkt(11, 7, append(radiusAttr(18, []byte("OTP?")), radiusAttr(24, []byte{0, 255, 65})...)))
		require.Equal(t, "Access-Request", c.Session["In Reply To"])
		q := agentRadiusRequest(8)
		q[4] = 0x99
		q = append(q, radiusAttr(24, []byte{0, 255, 65})...)
		q[3] = byte(len(q))
		feed(0, q)
		a := feed(1, radiusPkt(2, 8, nil))
		require.Equal(t, "Access-Request", a.Session["In Reply To"])
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("udp-identifier-zero-reject", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		q := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(0))
		require.Nil(t, q.Err)
		require.Len(t, q.Events, 1)
		r := s.Feed(1, time.Unix(101, 0), radiusPkt(3, 0, nil))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "radius", r.Events[0].Protocol)
		require.Equal(t, "Access-Reject", r.Events[0].Session["Packet Name"])
	})
	t.Run("udp-nonstandard-valid-adjacent", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 41001))
		require.NoError(t, err)
		defer s.Close("cleanup")
		r := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(7))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "radius", r.Events[0].Protocol)
		require.Equal(t, "alice", r.Events[0].Session["User-Name"])
	})
	t.Run("udp-known-port-malformed-adjacent", func(t *testing.T) {
		for _, deferred := range []bool{false, true} {
			radiusAssertStrongSTUNNeighbor(t, deferred)
		}
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		r := s.Feed(0, time.Unix(100, 0), radiusPkt(1, 7, []byte{1, 1}))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrMalformedMessage, r.Err.Kind)
		require.Len(t, r.Events, 1)
		require.Equal(t, "radius", r.Events[0].Protocol)
	})
	t.Run("udp-nonstandard-malformed-no-admission", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 41001))
		require.NoError(t, err)
		defer s.Close("cleanup")
		r := s.Feed(0, time.Unix(100, 0), radiusPkt(1, 7, []byte{1, 1}))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrUnsupportedFeature, r.Err.Kind)
		require.Len(t, r.Events, 1)
		require.NotEqual(t, "radius", r.Events[0].Protocol)
	})

}

func radiusAssertStrongSTUNNeighbor(t *testing.T, deferred bool) {
	t.Helper()
	// RFC8489 section6: M=Binding1, C=success2 -> encoded type0x0101;
	// length0, cookie0x2112a442, 96-bit transaction ID ending07.
	// The zero-attribute structural rule never processes an Attributes subnode.
	// This is a parsed orphan success-class observation, not evidence that a
	// server accepted a request, authentication, integrity or mapped address.
	wantWire := []byte{1, 1, 0, 0, 0x21, 0x12, 0xa4, 0x42, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}
	transaction := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}
	wantFields := map[string]any{
		"Message Type": uint16(0x0101), "Length": uint16(0),
		"Magic Cookie": uint32(0x2112a442), "Transaction ID": transaction,
	}
	wantSession := map[string]any{
		"Packet Name": "Binding", "Class": uint16(2),
		"Transaction ID": "000000000000000000000007", "TURN": false,
		"Integrity": "absent", "Attributes": []any{}, "Matched": false,
		"Pending Transactions": int(0), "Observation Scope": "conversation",
	}
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(1812, 1813))
	require.NoError(t, err)
	defer s.Close("cleanup")
	s.(*captureSession).f.a.config.Deferred = deferred
	input := bytes.Clone(wantWire)
	out := s.Feed(1, time.Unix(100, 0), input)
	clear(input)
	require.Nil(t, out.Err)
	require.False(t, out.NeedMore)
	require.Equal(t, len(wantWire), out.Consumed)
	require.Equal(t, "stun", out.State)
	require.Len(t, out.Events, 1)
	e := out.Events[0]
	require.Equal(t, "stun", e.Protocol)
	require.Equal(t, "stun-rfc8489-udp", e.Profile)
	require.Equal(t, "udp", e.Transport)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	require.Equal(t, status, e.Status)
	require.Equal(t, "message", e.Completeness)
	require.Empty(t, e.Error)
	require.Empty(t, e.ExpertCode)
	require.NotZero(t, e.ID)
	require.NotZero(t, e.FlowID)
	require.Equal(t, 1, e.Direction) // Public native Feed direction, not first-flow role.
	require.Equal(t, "192.0.2.2:1813", e.Source)
	require.Equal(t, "192.0.2.1:1812", e.Destination)
	require.True(t, time.Unix(100, 0).Equal(e.Timestamp))
	require.Equal(t, CaptureDomain{}, e.Domain)
	require.Equal(t, wantWire, e.Raw)
	require.Equal(t, len(wantWire), e.Length)
	require.Zero(t, e.Offset)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, "captured", e.SourceBytes.Kind)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wantWire)), e.SourceBytes.SHA256)
	require.Empty(t, e.SourceBytes.PacketRefs)
	require.Equal(t, wantSession, e.Session)
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, wantFields, f) // Exact typed legacy wire rule tree; no Attributes key.
	mutateRTPSQuery(f)
	tree, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, wantFields, protocolFields(tree))
	require.Equal(t, wantSession, tree["session"])
	mutateRTPSQuery(tree)
	tlsDTLSPoison(e, "radius")
	f, err = e.GetFields()
	require.NoError(t, err)
	require.Equal(t, wantFields, f)
	tree, err = e.Decode()
	require.NoError(t, err)
	require.Equal(t, wantSession, tree["session"])
	require.Empty(t, s.Close("complete"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.EqualValues(t, 20, s.Stats().InputBytes)
	require.EqualValues(t, 20, s.Stats().MessageBytes)
	require.EqualValues(t, 1, s.Stats().Messages)
	require.Zero(t, s.Stats().LimitedBytes)
}
