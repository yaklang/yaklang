package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func rtps20TestWire(kind, flags byte, body []byte) []byte {
	w := make([]byte, 24)
	copy(w, []byte("RTPS"))
	w[4] = 2
	w[20] = kind
	w[21] = flags
	rtpsOrder(flags).PutUint16(w[22:], uint16(len(body)))
	return append(w, body...)
}
func rtps20TestBody(order binary.ByteOrder) []byte {
	b := make([]byte, 20)
	copy(b, []byte{0, 0, 1, 7, 0, 0, 2, 2})
	order.PutUint32(b[12:], 7)
	copy(b[16:], []byte{9, 10, 11, 12})
	return b
}
func rtps20TestChanges(t *testing.T, f map[string]any) []map[string]any {
	t.Helper()
	return f["Changes"].([]map[string]any)
}
func TestRTPS20LengthsBudgetAndOwnership(t *testing.T) {
	for _, flags := range []byte{0, 1} {
		order := rtpsOrder(flags)
		body := rtps20TestBody(order)
		wire := rtps20TestWire(2, flags, body)
		f, err := decodeRTPSDatagram(wire, 1)
		require.NoError(t, err)
		d := rtps20TestChanges(t, f)[0]
		require.Equal(t, "090a0b0c", d["Key Hash Suffix"])
		require.Equal(t, false, d["Key Hash Prefix Known"])
		require.Equal(t, uint32(7), d["Writer Sequence Low"])
		require.Equal(t, uint32(0), d["Status Info"])
		require.Equal(t, false, d["Disposed"])
		require.Equal(t, false, d["Unregistered"])
		for n := 0; n < len(body); n++ {
			_, err = decodeRTPSDatagram(rtps20TestWire(2, flags, body[:n]), 32)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
		}
		for _, off := range []int{8, 12} {
			bad := bytes.Clone(body)
			order.PutUint32(bad[8:], 0)
			order.PutUint32(bad[12:], 0)
			if off == 8 {
				order.PutUint32(bad[8:], 0x80000000)
			}
			_, err = decodeRTPSDatagram(rtps20TestWire(2, flags, bad), 32)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
		}
		q := make([]byte, 20)
		order.PutUint16(q, 5)
		order.PutUint16(q[2:], 4)
		copy(q[4:], []byte{1, 2, 3, 4})
		copy(q[8:], q[:8])
		order.PutUint16(q[16:], 1)
		order.PutUint16(q[18:], 0xffff)
		qb := append(bytes.Clone(body), q...)
		qw := rtps20TestWire(2, flags|2, qb)
		f, err = decodeRTPSDatagram(qw, 3)
		require.NoError(t, err)
		params := rtps20TestChanges(t, f)[0]["Inline QoS"].([]map[string]any)
		require.Len(t, params, 2)
		require.Equal(t, uint16(5), params[0]["ID"])
		require.Equal(t, 4, params[0]["Length"])
		require.Equal(t, []byte{1, 2, 3, 4}, params[0]["Value"])
		require.Equal(t, params[0], params[1])
		clear(qw)
		params[0]["Value"].([]byte)[0] = 255
		require.Equal(t, []byte{1, 2, 3, 4}, params[1]["Value"])
		_, err = decodeRTPSDatagram(rtps20TestWire(2, flags|2, qb), 2)
		var pe *ProtocolError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrResourceExceeded, pe.Kind)
		for _, length := range []uint16{3, 0x8000, 24} {
			bad := bytes.Clone(qb)
			order.PutUint16(bad[22:], length)
			_, err = decodeRTPSDatagram(rtps20TestWire(2, flags|2, bad), 32)
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
		}
		_, err = decodeRTPSDatagram(rtps20TestWire(2, flags|2, qb[:len(qb)-4]), 32)
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrMalformedMessage, pe.Kind)
		_, err = decodeRTPSDatagram(rtps20TestWire(2, flags, append(bytes.Clone(body), 0)), 32)
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrMalformedMessage, pe.Kind)
		data := append(bytes.Clone(body), []byte{0, 1, 0, 0, 10, 20, 30, 40}...)
		dw := rtps20TestWire(2, flags|4, data)
		f, err = decodeRTPSDatagram(dw, 1)
		require.NoError(t, err)
		clear(dw)
		require.Equal(t, []byte{0, 1, 0, 0, 10, 20, 30, 40}, rtps20TestChanges(t, f)[0]["Serialized Payload"])
		for n := 0; n < 4; n++ {
			_, err = decodeRTPSDatagram(rtps20TestWire(2, flags|4, append(bytes.Clone(body), make([]byte, n)...)), 1)
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
		}
		// NOKEY_DATA has no mandatory key suffix; it cannot reuse keyed offsets.
		noKey := append(bytes.Clone(body[:16]), []byte{0, 1, 0, 0}...)
		f, err = decodeRTPSDatagram(rtps20TestWire(3, flags|4, noKey), 1)
		require.NoError(t, err)
		nd := rtps20TestChanges(t, f)[0]
		require.Equal(t, "NOKEY_DATA", nd["Submessage"])
		require.NotContains(t, nd, "Key Hash Suffix")
		require.Equal(t, []byte{0, 1, 0, 0}, nd["Serialized Payload"])
	}
}
func TestRTPS20SessionRefusalAndOwnedFields(t *testing.T) {
	wire := rtps20TestWire(2, 1, rtps20TestBody(binary.LittleEndian))
	want, err := decodeRTPSDatagram(wire, 32)
	require.NoError(t, err)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
	require.NoError(t, err)
	for i, w := range [][]byte{wire[:35], wire, bytes.Clone(wire)} {
		input := bytes.Clone(w)
		r := s.Feed(i%2, time.Unix(1700000000, int64(i)), input)
		clear(input)
		require.Len(t, r.Events, 1)
		e := r.Events[0]
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		if i == 0 {
			var pe *ProtocolError
			_, err = e.GetFields()
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
			continue
		}
		f, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want, f)
		rocEqualFields(t, want, e.Session)
		mutateRTPSQuery(f)
		mutateRTPSQuery(e.Session)
		clear(e.Raw)
		f, err = e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want, f)
		testNativeObservationSessionQueries(t, e, want)
	}
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
	b := DefaultParserBudget()
	b.MaxCollectionElements = 1
	s, err = NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
	require.NoError(t, err)
	q := []byte{5, 0, 4, 0, 1, 2, 3, 4, 1, 0, 0, 0}
	over := rtps20TestWire(2, 3, append(rtps20TestBody(binary.LittleEndian), q...))
	r := s.Feed(0, time.Unix(1700000000, 0), over)
	require.Len(t, r.Events, 1)
	_, err = r.Events[0].GetFields()
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
	r = s.Feed(1, time.Unix(1700000001, 0), wire)
	require.Len(t, r.Events, 1)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
	require.Zero(t, r.Events[0].ResponseTo)
	require.Zero(t, r.Events[0].TransactionID)
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("done"))
}

func TestRTPS20DepthAndProbeBudget(t *testing.T) {
	body := rtps20TestBody(binary.LittleEndian)
	wire := rtps20TestWire(2, 1, body)
	q := []byte{0x34, 0x12, 4, 0, 1, 2, 3, 4, 1, 0, 0, 0}
	qos := rtps20TestWire(2, 3, append(bytes.Clone(body), q...))
	for _, tc := range []struct {
		wire     []byte
		depth    int
		accepted bool
	}{{wire, 2, false}, {wire, 3, true}, {qos, 4, false}, {qos, 5, true}} {
		b := DefaultParserBudget()
		b.MaxRecursionDepth = tc.depth
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
		require.NoError(t, err)
		for i := 0; i < 3; i++ {
			p := s.Probe(tc.wire)
			if tc.accepted {
				require.Equal(t, ProbeAccept, p.Verdict)
			} else {
				require.Equal(t, ProbeReject, p.Verdict)
			}
			require.Zero(t, s.Stats().Messages)
			require.Zero(t, s.Stats().BufferedBytes)
		}
		input := bytes.Clone(tc.wire)
		r := s.Feed(0, time.Unix(1700000000, 0), input)
		clear(input)
		require.Len(t, r.Events, 1)
		fields, err := r.Events[0].GetFields()
		if tc.accepted {
			require.NoError(t, err)
			want, e := decodeRTPSDatagram(tc.wire, 1024)
			require.NoError(t, e)
			rocEqualFields(t, want, fields)
		} else {
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrResourceExceeded, pe.Kind)
			require.Nil(t, fields)
		}
		require.Zero(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		r = s.Feed(0, time.Unix(1700000001, 0), wire)
		require.Empty(t, r.Events)
		require.Zero(t, r.Consumed)
		var pe *ProtocolError
		require.ErrorAs(t, r.Err, &pe)
		require.Equal(t, ErrFatalSessionError, pe.Kind)
	}
}

func TestRTPS20SealedReplay(t *testing.T) {
	testRTPSSealedReplay(t, rtpsBatchControls(t, "rtps-20-data", 66))
}

func TestRTPS20SealedSessionBounds(t *testing.T) {
	for _, c := range rtpsBatchControls(t, "rtps-20-data", 66) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rtpsInput(t, c)
			b := DefaultParserBudget()
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
			require.NoError(t, err)
			for i, encoded := range want.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				if want.PureErrors[i] == "" {
					require.Equal(t, ProbeAccept, s.Probe(wire).Verdict)
				} else {
					require.Equal(t, ProbeReject, s.Probe(wire).Verdict)
				}
				input := bytes.Clone(wire)
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), input)
				clear(input)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if want.PureErrors[i] == "" {
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want.Events[i].Fields, fields)
					rocEqualFields(t, want.Events[i].Fields, e.Session)
					testNativeObservationSessionQueries(t, e, want.Events[i].Fields)
					mutateRTPSQuery(e.Session)
					fields, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want.Events[i].Fields, fields)
				} else {
					_, err = e.GetFields()
					assertOwnedProtocolRefusal(t, e, want.PureErrors[i], err)
				}
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			if strings.HasPrefix(c.ID, "budget-refusal-") {
				b.MaxCollectionElements = 3
				s, err = NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
				require.NoError(t, err)
				for i, encoded := range want.Wires {
					wire, err := hex.DecodeString(encoded)
					require.NoError(t, err)
					r := s.Feed(i%2, time.Unix(1700000000, int64(i)), wire)
					require.Len(t, r.Events, 1)
					fields, err := r.Events[0].GetFields()
					if i == 0 {
						require.Nil(t, fields)
						assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
					} else {
						require.NoError(t, err)
						rocEqualFields(t, want.Events[i].Fields, fields)
					}
					require.Zero(t, r.Events[0].ResponseTo)
					require.Zero(t, r.Events[0].TransactionID)
					require.Zero(t, s.Stats().BufferedBytes)
				}
				require.Empty(t, s.Close("budget-end"))
			}
		})
	}
}

func TestRTPS20ByteBudgetAndCarrier(t *testing.T) {
	plain := rtps20TestWire(2, 1, rtps20TestBody(binary.LittleEndian))
	large := rtps20TestWire(2, 5, append(rtps20TestBody(binary.LittleEndian), append([]byte{0, 1, 0, 0}, make([]byte, 64)...)...))
	b := DefaultParserBudget()
	b.MaxFrameBytes = 64
	b.MaxMessageBytes = 64
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
	require.NoError(t, err)
	require.Equal(t, ProbeReject, s.Probe(large).Verdict)
	r := s.Feed(0, time.Unix(1700000000, 0), large)
	require.Len(t, r.Events, 1)
	_, err = r.Events[0].GetFields()
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
	require.Zero(t, s.Stats().BufferedBytes)
	r = s.Feed(1, time.Unix(1700000001, 0), plain)
	require.Len(t, r.Events, 1)
	fields, err := r.Events[0].GetFields()
	require.NoError(t, err)
	want, err := decodeRTPSDatagram(plain, 1024)
	require.NoError(t, err)
	rocEqualFields(t, want, fields)
	require.Zero(t, r.Events[0].ResponseTo)
	require.Zero(t, r.Events[0].TransactionID)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
	tcp, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	require.NotEqual(t, ProbeAccept, tcp.Probe(plain).Verdict)
	for _, e := range tcp.Feed(0, time.Unix(1700000000, 0), plain).Events {
		require.NotEqual(t, "rtps", e.Protocol)
	}
	tcp.Close("done")
	require.Zero(t, tcp.Stats().BufferedBytes)
}
