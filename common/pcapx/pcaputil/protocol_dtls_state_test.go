package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dtlsTestRecord(typ byte, epoch uint16, seq uint64, body []byte) []byte {
	w := []byte{typ, 0xfe, 0xfd, byte(epoch >> 8), byte(epoch)}
	for n := 5; n >= 0; n-- {
		w = append(w, byte(seq>>uint(n*8)))
	}
	w = binary.BigEndian.AppendUint16(w, uint16(len(body)))
	return append(w, body...)
}
func dtlsTestFragment(typ byte, seq uint16, total, offset int, b []byte) []byte {
	w := []byte{typ, byte(total >> 16), byte(total >> 8), byte(total), byte(seq >> 8), byte(seq), byte(offset >> 16), byte(offset >> 8), byte(offset), byte(len(b) >> 16), byte(len(b) >> 8), byte(len(b))}
	return append(w, b...)
}
func dtlsTestHello() []byte {
	b := append([]byte{0xfe, 0xfd}, bytes.Repeat([]byte{7}, 32)...)
	return append(b, 0, 0, 0, 2, 0xc0, 0x2f, 1, 0)
}
func dtlsTestParser(t testing.TB, budget int) (*CaptureConfig, *binParser) {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{MaxBufferedBytes: budget, MaxMessageBytes: budget, OnEvent: func(*ProtocolEvent) {}})(c))
	require.NoError(t, c.prepareBinParser())
	return c, c.binParser
}
func dtlsTestFeed(t testing.TB, a *binParser, dir int, domain CaptureDomain, ts time.Time, w []byte) *ProtocolEvent {
	t.Helper()
	endpoints := [2]string{"192.0.2.1:40000", "192.0.2.2:4444"}
	e := &ProtocolEvent{Source: endpoints[dir], Destination: endpoints[1-dir], Domain: domain, Transport: "udp", Timestamp: ts, Length: len(w), SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: uint64(ts.Unix()), Domain: domain}}}}
	require.True(t, a.decodeDTLSDatagram(e, w, false))
	return e
}
func TestDTLSFragmentsReplayAndIsolation(t *testing.T) {
	c, a := dtlsTestParser(t, 0)
	defer c.finishBinParser()
	hello := dtlsTestHello()
	ts := time.Unix(10, 0)
	tail := dtlsTestRecord(22, 0, 1, dtlsTestFragment(1, 0, len(hello), 20, hello[20:]))
	head := dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, len(hello), 0, hello[:25]))
	e := dtlsTestFeed(t, a, 0, CaptureDomain{}, ts, tail)
	require.Empty(t, e.Error)
	require.Equal(t, false, dtlsTestMessages(e.Session["Records"].([]map[string]any)[0])[0]["Complete"])
	e = dtlsTestFeed(t, a, 1, CaptureDomain{}, ts, head)
	require.Empty(t, e.Error)
	require.Equal(t, false, dtlsTestMessages(e.Session["Records"].([]map[string]any)[0])[0]["Complete"])
	e = dtlsTestFeed(t, a, 0, CaptureDomain{Interface: 1}, ts, head)
	require.Empty(t, e.Error)
	require.Equal(t, false, dtlsTestMessages(e.Session["Records"].([]map[string]any)[0])[0]["Complete"])
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, ts.Add(time.Second), head)
	require.Empty(t, e.Error)
	m := dtlsTestMessages(e.Session["Records"].([]map[string]any)[0])[0]
	require.Equal(t, true, m["Complete"])
	require.Len(t, m["Packet Refs"], 2)
	require.Equal(t, uint16(0xfefd), m["Hello Version"])
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, ts.Add(2*time.Second), head)
	require.Equal(t, true, e.Session["Records"].([]map[string]any)[0]["Retransmission"])
	full := dtlsTestRecord(22, 0, 3, dtlsTestFragment(1, 0, len(hello), 0, hello))
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, ts.Add(3*time.Second), full)
	require.Empty(t, e.Error)
	require.Equal(t, true, dtlsTestMessages(e.Session["Records"].([]map[string]any)[0])[0]["Retransmission"])
	conflict := bytes.Clone(head)
	conflict[len(conflict)-1] ^= 1
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, ts.Add(4*time.Second), conflict)
	require.Equal(t, "malformed", e.Status)
	require.Contains(t, e.Error, "conflicting captured bytes")
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)
}
func TestDTLSMalformedBoundsAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"record-truncated", []byte{22, 0xfe, 0xfd}}, {"fragment-range", dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, 5, 4, []byte{1, 2}))},
		{"ccs", dtlsTestRecord(20, 0, 0, []byte{2})}, {"alert", dtlsTestRecord(21, 0, 0, []byte{2})}, {"clear-application", dtlsTestRecord(23, 0, 0, []byte{1})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, a := dtlsTestParser(t, 0)
			e := dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(1, 0), tc.wire)
			require.NotEmpty(t, e.Error)
			require.NotEqual(t, "decoded", e.Status)
			require.NoError(t, c.finishBinParser())
			require.Zero(t, a.stats().BufferedBytes)
		})
	}
	c, a := dtlsTestParser(t, 2048)
	large := dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, 16384, 0, []byte{1}))
	e := dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(1, 0), large)
	require.Equal(t, "limited", e.Status)
	require.LessOrEqual(t, a.stats().BufferedBytes, int64(2048))
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)
	c, a = dtlsTestParser(t, 0)
	partial := dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, 200, 0, []byte{1}))
	dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(1, 0), partial)
	old := a.stats().BufferedBytes
	require.Positive(t, old)
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(100, 0), dtlsTestRecord(21, 0, 0, []byte{1, 0}))
	require.Empty(t, e.Error)
	require.Empty(t, a.udpSessions.lru.Front().Value.(*binUDPEntry).flow.dtls.fragments)
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)
}
func TestDTLSAdmissionRequiresWireAndPreservesOpaque(t *testing.T) {
	c, a := dtlsTestParser(t, 0)
	defer c.finishBinParser()
	for _, wire := range [][]byte{[]byte("ordinary text"), []byte{22, 3, 3, 0, 0}, []byte{22, 0xfe, 0xfa, 0, 0}, []byte{0x2c, 1, 2, 3}} {
		e := &ProtocolEvent{Source: "192.0.2.1:443", Destination: "192.0.2.2:4444"}
		require.False(t, a.decodeDTLSDatagram(e, wire, false))
		require.Empty(t, e.Protocol)
	}
	e := dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(1, 0), dtlsTestRecord(23, 1, 1, []byte{1, 2, 3}))
	require.Empty(t, e.Error)
	r := e.Session["Records"].([]map[string]any)[0]
	require.Equal(t, "encrypted", r["Semantic Status"])
	require.Equal(t, false, r["Content Decoded"])
	require.Equal(t, false, e.Session["Authentication Verified"])
	unsupported := dtlsTestRecord(22, 0, 0, []byte{1})
	unsupported[2] = 0xfc
	e = dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(2, 0), unsupported)
	require.Equal(t, ErrUnsupportedVersion, e.sessionError.Kind)
	for _, modern := range [][]byte{{0x2c, 1, 2, 3}, {25, 0xfe, 0xfd}, {22, 0xfe, 0xfc}} {
		e = dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(3, 0), modern)
		require.Equal(t, "context-required", e.Status)
		require.Contains(t, []ProtocolErrorKind{ErrUnsupportedFeature, ErrUnsupportedVersion}, e.sessionError.Kind)
	}
}

func TestDTLSReplayCacheBudgetAndRelease(t *testing.T) {
	c, a := dtlsTestParser(t, 0)
	for i := 0; i < dtlsReplayWindow*3; i++ {
		body := dtlsTestFragment(14, uint16(i), 0, 0, nil)
		e := dtlsTestFeed(t, a, 1, CaptureDomain{}, time.Unix(int64(i), 0), dtlsTestRecord(22, 0, uint64(i), body))
		require.Empty(t, e.Error)
		state := a.udpSessions.lru.Front().Value.(*binUDPEntry).flow.dtls
		require.LessOrEqual(t, len(state.records), dtlsReplayWindow)
		require.LessOrEqual(t, len(state.completed), dtlsReplayWindow)
		require.LessOrEqual(t, cap(state.recordOrder), dtlsReplayWindow)
		require.LessOrEqual(t, cap(state.completedOrder), dtlsReplayWindow)
		require.GreaterOrEqual(t, a.stats().BufferedBytes, state.storage())
	}
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)

	c, a = dtlsTestParser(t, 4096)
	defer c.finishBinParser()
	limited := false
	for i := 0; i < dtlsReplayWindow; i++ {
		e := dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(int64(i), 0), dtlsTestRecord(23, 1, uint64(i), []byte{1}))
		if e.sessionError != nil && e.sessionError.Kind == ErrResourceExceeded {
			require.Equal(t, "context-required", e.Status)
			require.Empty(t, a.udpSessions.entries)
			require.Zero(t, a.stats().BufferedBytes)
			limited = true
			break
		}
		require.Empty(t, e.Error)
		require.LessOrEqual(t, a.stats().BufferedBytes, int64(4096))
	}
	require.True(t, limited, "cache capacity must participate in the retained-state budget")
	e := dtlsTestFeed(t, a, 0, CaptureDomain{}, time.Unix(1000, 0), dtlsTestRecord(23, 1, 0, []byte{1}))
	require.Empty(t, e.Error, "a new context can start after the failed context was released")
}

func FuzzDTLSStateSequence(f *testing.F) {
	hello := dtlsTestHello()
	f.Add(dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, len(hello), 0, hello)))
	f.Add(dtlsTestRecord(23, 1, 0, []byte{1, 2, 3}))
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 4096 {
			t.Skip()
		}
		c, a := dtlsTestParser(t, 16<<10)
		for i := 0; i < 3; i++ {
			e := &ProtocolEvent{Source: "192.0.2.1:4444", Destination: "192.0.2.2:40000", Transport: "udp", Timestamp: time.Unix(int64(i), 0)}
			next := bytes.Clone(wire)
			if len(next) >= 13 {
				next[10] ^= byte(i)
			}
			if i == 1 {
				e.Source, e.Destination = e.Destination, e.Source
			}
			a.decodeDTLSDatagram(e, next, true)
			require.LessOrEqual(t, a.stats().BufferedBytes, int64(16<<10))
		}
		require.NoError(t, c.finishBinParser())
		require.Zero(t, a.stats().BufferedBytes)
		require.Zero(t, a.stats().CallbackPanics)
	})
}
