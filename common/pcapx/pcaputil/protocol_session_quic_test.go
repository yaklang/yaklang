package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func quicTestDCID() []byte { return []byte{8, 3, 9, 4, 0xc8, 0xf0, 0x3e, 0x51} }
func quicTestSCID() []byte { return []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5} }

func TestQUICParseStreamAndAckFrames(t *testing.T) {
	// RFC 9000 section 19.8: Offset is present only when the OFF bit (0x04) is set.
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"implicit_offset", []byte{0x0b, 0, 3, 'G', 'E', 'T', 2, 0, 0, 0, 0}},
		{"explicit_offset", []byte{0x0f, 0, 0, 3, 'G', 'E', 'T', 2, 0, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames, err := quicParseFrames(tc.wire, 64)
			require.NoError(t, err)
			require.Len(t, frames, 2)
			require.Equal(t, "STREAM", frames[0]["Frame Type"])
			require.Equal(t, uint64(0), frames[0]["Stream ID"])
			require.Equal(t, uint64(0), frames[0]["Offset"])
			require.Equal(t, true, frames[0]["FIN"])
			require.Equal(t, uint64(3), frames[0]["Length"])
			require.Equal(t, []byte("GET"), frames[0]["Stream Data"])
			require.Equal(t, "ACK", frames[1]["Frame Type"])
		})
	}

	t.Run("unexpected_offset_byte", func(t *testing.T) {
		_, err := quicParseFrames([]byte{0x0b, 0, 0, 3, 'G', 'E', 'T', 2, 0, 0, 0, 0}, 64)
		require.Error(t, err)
	})
	t.Run("truncated_stream_data", func(t *testing.T) {
		_, err := quicParseFrames([]byte{0x0b, 0, 3, 'G', 'E'}, 64)
		require.Error(t, err)
	})
}

func TestProtocolSessionQUICTransportAndLifecycle(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	initPkt := quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))
	p := s.Probe(initPkt)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "quic", p.Protocol)
	require.Equal(t, "1", p.Version)

	ts := time.Unix(1, 0)
	r := s.Feed(0, ts, initPkt)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Initial", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint32(1), r.Events[0].Session["Version"])
	require.Equal(t, dcid, r.Events[0].Session["DCID"])
	require.Equal(t, "initial", r.Events[0].Session["Packet Number Space"])
	require.Equal(t, uint64(0), r.Events[0].Session["Packet Number"])
	frames, _ := r.Events[0].Session["Frames"].([]map[string]any)
	require.NotEmpty(t, frames)
	require.Equal(t, "CRYPTO", frames[0]["Frame Type"])
	require.Equal(t, []byte("CHLO"), frames[0]["Crypto Data"])

	hs := quicLongPacket(2, 1, dcid, scid, nil, 0, quicCryptoFrame(0, []byte("SHLO")))
	r = s.Feed(1, ts, hs)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Handshake", r.Events[0].Session["Packet Name"])
	require.Equal(t, "handshake", r.Events[0].Session["Packet Number Space"])
	require.Equal(t, scid, r.Events[0].Session["SCID"])

	app := quicLongPacket(1, 1, dcid, scid, nil, 0, append(quicStreamFrame(0, 0, true, []byte("GET")), quicAckFrame(0)...))
	r = s.Feed(0, ts, app)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "0-RTT", r.Events[0].Session["Packet Name"])
	require.Equal(t, "application", r.Events[0].Session["Packet Number Space"])
	frames, _ = r.Events[0].Session["Frames"].([]map[string]any)
	require.Equal(t, "STREAM", frames[0]["Frame Type"])
	require.Equal(t, uint64(0), frames[0]["Stream ID"])
	require.Equal(t, true, frames[0]["FIN"])
	require.Equal(t, []byte("GET"), frames[0]["Stream Data"])
	require.Equal(t, "ACK", frames[1]["Frame Type"])

	rst := quicLongPacket(1, 1, dcid, scid, nil, 1, append(quicResetStream(0, 1, 3), quicStopSending(4, 2)...))
	r = s.Feed(1, ts, rst)
	require.Nil(t, r.Err, "%v", r.Err)
	frames, _ = r.Events[0].Session["Frames"].([]map[string]any)
	require.Equal(t, "RESET_STREAM", frames[0]["Frame Type"])
	require.Equal(t, "STOP_SENDING", frames[1]["Frame Type"])

	done := quicLongPacket(1, 1, dcid, scid, nil, 2, []byte{0x1e})
	r = s.Feed(1, ts, done)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "established", r.Events[0].Session["Connection State"])

	fin := quicLongPacket(2, 1, dcid, scid, nil, 1, quicConnectionClose(0, "bye"))
	r = s.Feed(1, ts, fin)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Connection Closed"])
	require.Equal(t, "closed", r.Events[0].Session["Connection State"])
}

func TestProtocolSessionQUICPNWrapDuplicateGapAndEncrypted(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	dcid := quicTestDCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 255, quicCryptoFrame(0, []byte("A")))).Err)
	r := s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(1, []byte("B"))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint64(256), r.Events[0].Session["Packet Number"])
	require.Nil(t, r.Events[0].Session["Gap"])
	require.Nil(t, r.Events[0].Session["Duplicate"])

	r = s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(1, []byte("B"))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Duplicate"])
	require.Equal(t, true, r.Events[0].Session["Retransmission"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 1, quicCryptoFrame(0, []byte("A")))).Err)
	r = s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 3, quicCryptoFrame(1, []byte("C"))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Gap"])
	require.Equal(t, uint64(1), r.Events[0].Session["Missing Count"])

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	protected := quicLongPacket(0, 1, dcid, nil, nil, 0, bytesRepeat(0xaa, 20))
	r = s3.Feed(0, ts, protected)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "Initial", r.Events[0].Session["Packet Name"])
	require.Equal(t, dcid, r.Events[0].Session["DCID"])
}

func TestProtocolSessionQUICFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, ProbeReject, s.Probe([]byte{0xc0, 0x00, 0x00}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0xc0, 0x00, 0x00, 0x00, 0x01}).Verdict)
	require.NotEqual(t, "quic", s.Probe([]byte{0x80, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x11, 0x11, 0x11, 0x11}).Protocol)
	require.NotEqual(t, "quic", s.Probe([]byte{0x00, 0x00, 0x00, 0x00}).Protocol)
	require.NotEqual(t, "quic", s.Probe([]byte("INVITE sip:a SIP/2.0\r\n")).Protocol)
	require.NotEqual(t, "quic", s.Probe([]byte{0xc0, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0xaa, 0x00, 0x10, 0x00}).Protocol)
	require.NotEqual(t, "quic", s.Probe([]byte{0xc0, 0x00, 0x00, 0x00, 0x01, 21, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}).Protocol)

	ts := time.Unix(1, 0)
	cut := s.Feed(0, ts, quicLongPacket(0, 1, quicTestDCID(), nil, nil, 0, quicCryptoFrame(0, []byte("X")))[:8])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	vn := quicVersionNegotiation(quicTestDCID(), nil, 1)
	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r := s2.Feed(0, ts, vn)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Version Negotiation", r.Events[0].Session["Packet Name"])
	require.Equal(t, []uint32{1}, r.Events[0].Session["Supported Versions"])
}

func TestProtocolSessionQUICFragmentation(t *testing.T) {
	dcid, scid := quicTestDCID(), quicTestSCID()
	steps := []sessionStep{
		{0, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))},
		{1, quicLongPacket(2, 1, dcid, scid, nil, 0, quicCryptoFrame(0, []byte("SHLO")))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, []byte("GET")))},
		{1, quicLongPacket(1, 1, dcid, scid, nil, 1, quicAckFrame(0))},
		{1, quicLongPacket(2, 1, dcid, scid, nil, 1, quicConnectionClose(0, "done"))},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["Packet Number"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
