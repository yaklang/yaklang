package pcaputil

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func rtmpTestInitial() []byte {
	w := make([]byte, 1537)
	w[0] = 3
	binary.BigEndian.PutUint32(w[1:], 100)
	for i := 9; i < len(w); i++ {
		w[i] = byte(i)
	}
	return w
}
func rtmpTestReady(t *testing.T) ProtocolSession {
	s := observedTestSession(t, 1935)
	w := rtmpTestInitial()
	require.Len(t, s.Feed(0, time.Unix(1, 0), w).Events, 1)
	require.Len(t, s.Feed(1, time.Unix(2, 0), w).Events, 1)
	for d := 0; d < 2; d++ {
		r := s.Feed(d, time.Unix(3, 0), w[1:])
		require.Len(t, r.Events, 1)
		require.Empty(t, r.Events[0].Error)
	}
	return s
}
func rtmpTestHeader(csid byte, ts uint32, length int, typ byte, stream uint32) []byte {
	w := []byte{csid, byte(ts >> 16), byte(ts >> 8), byte(ts), byte(length >> 16), byte(length >> 8), byte(length), typ}
	w = binary.LittleEndian.AppendUint32(w, stream)
	return w
}
func rtmpTestMessage(csid byte, typ byte, payload []byte, chunk int) []byte {
	w := rtmpTestHeader(csid, 0, len(payload), typ, 0)
	for off := 0; off < len(payload); {
		n := min(chunk, len(payload)-off)
		w = append(w, payload[off:off+n]...)
		off += n
		if off < len(payload) {
			w = append(w, 0xc0|csid)
		}
	}
	return w
}
func rtmpTestCommand(name string, n float64) []byte {
	w := []byte{2, byte(len(name) >> 8), byte(len(name))}
	w = append(w, name...)
	w = append(w, 0)
	w = binary.BigEndian.AppendUint64(w, math.Float64bits(n))
	return append(w, 5)
}
func rtmpTestLogical(events []*ProtocolEvent) []*ProtocolEvent {
	var out []*ProtocolEvent
	for _, e := range events {
		if e.Profile == "rtmp-message" {
			out = append(out, e)
		}
	}
	return out
}
func TestRTMPAllInitialCutsAndEcho(t *testing.T) {
	w := rtmpTestInitial()
	for cut := 1; cut < len(w); cut++ {
		s := observedTestSession(t, 1935)
		require.Empty(t, s.Feed(0, time.Unix(1, 0), w[:cut]).Events)
		r := s.Feed(0, time.Unix(2, 0), w[cut:])
		require.Len(t, r.Events, 1)
		require.Empty(t, r.Events[0].Error)
		s.Close("cut")
	}
}
func TestRTMPDirectionalChunkSizeInterleaveAndTransactions(t *testing.T) {
	s := rtmpTestReady(t)
	set := rtmpTestMessage(2, 1, []byte{0, 0, 0, 4}, 128)
	r := s.Feed(0, time.Unix(4, 0), set)
	require.Len(t, rtmpTestLogical(r.Events), 1)
	a := rtmpTestCommand("connect", 1)
	b := rtmpTestCommand("createStream", 2)
	aw := rtmpTestMessage(3, 20, a, 4)
	bw := rtmpTestMessage(4, 20, b, 4)
	// CSIDs can interleave; one stream must never consume the other's header.
	w := append(append(append([]byte{}, aw[:16]...), bw...), aw[16:]...)
	var logical []*ProtocolEvent
	for _, v := range w {
		r = s.Feed(0, time.Unix(5, 0), []byte{v})
		logical = append(logical, rtmpTestLogical(r.Events)...)
	}
	require.Len(t, logical, 2)
	require.Equal(t, "createStream", logical[0].Session["Command"])
	require.Equal(t, b, logical[0].Raw)
	require.Equal(t, a, logical[1].Raw)
	require.Greater(t, len(logical[1].SourceBytes.ParentPDUs), 1)
	// Peer chunk size remains the default 128. A response uses the original
	// request's PDU, while a reused transaction number starts a new cycle.
	r = s.Feed(1, time.Unix(6, 0), rtmpTestMessage(3, 20, rtmpTestCommand("_result", 1), 128))
	ans := rtmpTestLogical(r.Events)
	require.Len(t, ans, 1)
	require.Empty(t, ans[0].Error)
	require.Equal(t, logical[1].ID, ans[0].ResponseTo)
	r = s.Feed(0, time.Unix(7, 0), aw)
	next := rtmpTestLogical(r.Events)
	require.Len(t, next, 1)
	require.NotEqual(t, logical[1].ID, next[0].ID)
	r = s.Feed(1, time.Unix(8, 0), rtmpTestMessage(3, 20, rtmpTestCommand("_result", 1), 128))
	require.Equal(t, next[0].ID, rtmpTestLogical(r.Events)[0].ResponseTo)
}
func TestRTMPAbortMalformedControlAndIncompleteClose(t *testing.T) {
	s := rtmpTestReady(t)
	p := bytes.Repeat([]byte{7}, 200)
	partial := rtmpTestMessage(5, 8, p, 128)[:140]
	require.Empty(t, rtmpTestLogical(s.Feed(0, time.Unix(4, 0), partial).Events))
	r := s.Feed(0, time.Unix(5, 0), rtmpTestMessage(2, 2, []byte{0, 0, 0, 5}, 128))
	v := rtmpTestLogical(r.Events)
	require.Len(t, v, 2)
	require.Equal(t, "incomplete", v[0].Status)
	require.Equal(t, p[:128], v[0].Raw)
	// Bad control payload cannot change the directional chunk size. Its known
	// complete boundary permits the next independently framed message.
	w := append(rtmpTestMessage(2, 1, []byte{0, 0, 0, 0}, 128), rtmpTestMessage(3, 20, rtmpTestCommand("connect", 5), 128)...)
	v = rtmpTestLogical(s.Feed(0, time.Unix(6, 0), w).Events)
	require.Len(t, v, 2)
	require.Equal(t, "malformed", v[0].Status)
	require.Empty(t, v[1].Error)
	s.Feed(0, time.Unix(7, 0), partial)
	v = rtmpTestLogical(s.Close("capture end"))
	require.Len(t, v, 1)
	require.Equal(t, "incomplete", v[0].Status)
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestRTMPCompressedHeadersExtendedTimeAndMissingContext(t *testing.T) {
	prev := stream_parser.RTMPChunkContext{}
	w := rtmpTestHeader(3, 0xffffff, 8, 8, 12)
	w = binary.BigEndian.AppendUint32(w, 0xfffffff0)
	w = append(w, 1, 2, 3, 4)
	h, err := stream_parser.DecodeRTMPChunkHeader(w, prev, 4, 1024)
	require.NoError(t, err)
	require.Equal(t, len(w), h.WireLength)
	prev = h.Context
	prev.Received = 4
	c := []byte{0xc3, 0xff, 0xff, 0xff, 0xf0, 5, 6, 7, 8}
	h, err = stream_parser.DecodeRTMPChunkHeader(c, prev, 4, 1024)
	require.NoError(t, err)
	require.Equal(t, uint32(0xfffffff0), h.Context.Timestamp)
	require.True(t, h.Continuation)
	prev = h.Context
	prev.Received = 8
	h, err = stream_parser.DecodeRTMPChunkHeader(c, prev, 4, 1024)
	require.NoError(t, err)
	require.Equal(t, uint32(0xffffffe0), h.Context.Timestamp)
	require.False(t, h.Continuation)
	_, err = stream_parser.DecodeRTMPChunkHeader([]byte{0x83, 0, 0, 1}, stream_parser.RTMPChunkContext{}, 128, 1024)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireContext)
	prev.Extended = false
	prev.Timestamp = 100
	prev.Delta = 4
	h, err = stream_parser.DecodeRTMPChunkHeader([]byte{0x43, 0, 0, 5, 0, 0, 4, 8, 1, 2, 3, 4}, prev, 128, 1024)
	require.NoError(t, err)
	require.Equal(t, uint32(105), h.Context.Timestamp)
	prev = h.Context
	prev.Received = 4
	h, err = stream_parser.DecodeRTMPChunkHeader([]byte{0x83, 0, 0, 6, 1, 2, 3, 4}, prev, 128, 1024)
	require.NoError(t, err)
	require.Equal(t, uint32(111), h.Context.Timestamp)
}
func TestRTMPAMF0BoundsAndOwnedRefusal(t *testing.T) {
	w := rtmpTestCommand("connect", 1)
	for cut := 0; cut < len(w); cut++ {
		_, err := stream_parser.DecodeRTMPMessage(w[:cut], 20, 100)
		require.Error(t, err)
	}
	_, err := stream_parser.DecodeAMF0([]byte{7, 0, 0}, 100)
	require.Error(t, err)
	_, err = stream_parser.DecodeAMF0([]byte{10, 0xff, 0xff, 0xff, 0xff}, 100)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	s := rtmpTestReady(t)
	w = append(rtmpTestMessage(3, 20, []byte{2, 0, 5, 'b'}, 128), rtmpTestMessage(3, 20, rtmpTestCommand("connect", 1), 128)...)
	v := rtmpTestLogical(s.Feed(0, time.Unix(4, 0), w).Events)
	require.Len(t, v, 2)
	require.Equal(t, "malformed", v[0].Status)
	require.Empty(t, v[1].Error)
	v[0].Status = "decoded"
	v[0].Session = map[string]any{"Command": "invented"}
	_, err = v[0].GetFields()
	require.Error(t, err)
}
func FuzzObservedBGP_AJP_RTMP(f *testing.F) {
	for _, w := range [][]byte{bgpTestOpen(true), bgpTestWire(4, nil), ajpTestWire(true, []byte{10}), rtmpTestInitial(), rtmpTestMessage(3, 20, rtmpTestCommand("connect", 1), 128)} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 8192 {
			return
		}
		_, _ = stream_parser.DecodeBGPMessage(w, stream_parser.BGPWireContext{}, 64)
		_, _ = stream_parser.DecodeAJPMessage(w, false, 64)
		_, _ = stream_parser.DecodeAJPMessage(w, true, 64)
		_, _ = stream_parser.RTMPInitialHandshake(w)
		_, _ = stream_parser.DecodeRTMPInitialChunk(w, 8192)
		_, _ = stream_parser.DecodeAMF0(w, 64)
		// Exercise the same malformed bytes after a proven connection identity,
		// not just through independent stateless codec calls.
		if len(w) == 0 {
			return
		}
		budget := DefaultParserBudget()
		budget.MaxFrameBytes, budget.MaxMessageBytes, budget.MaxBufferedBytes, budget.MaxCollectionElements = 8192, 8192, 65536, 64
		port := []uint16{179, 8009, 1935}[int(w[0])%3]
		s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"), WithSessionPorts(40000, port), WithSessionClientDirection(0))
		require.NoError(t, err)
		switch port {
		case 179:
			s.Feed(0, time.Unix(1, 0), bgpTestOpen(true))
		case 8009:
			s.Feed(0, time.Unix(1, 0), ajpTestWire(true, []byte{10}))
		case 1935:
			initial := rtmpTestInitial()
			s.Feed(0, time.Unix(1, 0), initial)
			s.Feed(1, time.Unix(2, 0), initial)
			s.Feed(0, time.Unix(3, 0), initial[1:])
			s.Feed(1, time.Unix(4, 0), initial[1:])
		}
		step := 1 + int(w[0])%31
		for off := 0; off < len(w); off += step {
			r := s.Feed((off/step)%2, time.Unix(int64(5+off), 0), w[off:min(off+step, len(w))])
			for _, e := range r.Events {
				if e.Status == "decoded" {
					_, err = e.GetFields()
					require.NoError(t, err)
				}
			}
		}
		s.Close("fuzz end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
