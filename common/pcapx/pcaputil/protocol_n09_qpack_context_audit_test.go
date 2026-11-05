package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// These fixtures declare native key/ALPN state explicitly. Every assertion
// below dispatches a real protected UDP datagram and completes a.emit; these
// are not claims that an independent peer produced the supplied handshake.
type n09QPACKNative struct {
	a      *binParser
	q      *binQUIC
	f      *binFlow
	domain CaptureDomain
}

func n09QPACKOwner(t *testing.T, q *binQUIC) *n09QPACKNative {
	t.Helper()
	a := earlyRecoveryParser(t)
	q.alpn, q.byteLimit, q.zeroCID = "h3", 65536, [2]bool{true, true}
	domain := CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91/pppoe:test-owner"}
	return &n09QPACKNative{a, q, earlyRecoveryOwner(a, q, domain), domain}
}

func (n *n09QPACKNative) dispatch(t *testing.T, dir int, frame uint64, wire []byte) *ProtocolEvent {
	t.Helper()
	e := earlyRecoveryBase(n.f, n.domain, frame)
	if dir == 1 {
		e.Source, e.Destination = e.Destination, e.Source
	}
	events, handled := n.a.decodeQUICDatagram(e, wire)
	require.True(t, handled)
	require.Len(t, events, 1)
	n.a.emit(events[0]) // public ExpertCode belongs to the actual emit stage
	return events[0]
}

func (n *n09QPACKNative) send(t *testing.T, dir int, frame, pn uint64, kind string, key *quicTrafficKeys, sid, off uint64, data []byte) *ProtocolEvent {
	t.Helper()
	return n.dispatch(t, dir, frame, quicConformancePacket(t, key, kind, pn, 0, quicStreamFrame(sid, off, false, data)))
}

func n09QPACKFixture(dir int) (capacity uint64, encoder, field []byte) {
	if dir == 0 {
		return 220, rfcHex("3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468"), rfcHex("03811011d1d7")
	}
	// Two literal insertions, :status=200 and x-test=yes, under capacity 128.
	return 128, append([]byte{0x3f, 0x61, 0x47}, []byte(":status\x03200\x46x-test\x03yes")...), rfcHex("03811011")
}

func n09QPACKRequireContext(t *testing.T, e *ProtocolEvent, text string) {
	t.Helper()
	require.Equal(t, "context-required", e.Status)
	require.Equal(t, string(ErrContextRequired), e.ExpertCode)
	require.Equal(t, "http3", e.Protocol)
	require.Equal(t, true, e.Session["Authentication Verified"])
	require.Contains(t, fmt.Sprint(e.Session), text)
	require.NotContains(t, e.Session, "Headers")
}

func TestN09QPACKNativeFieldWaitsForObservedSettings(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				t.Run(fmt.Sprintf("v%x/%s/dir%d", version, algorithm, dir), func(t *testing.T) {
					q, key := quicConformanceContext(t, version, algorithm)
					n := n09QPACKOwner(t, q)
					capacity, encoder, field := n09QPACKFixture(dir)
					framed := h3Frame(1, field)
					wire := quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(0, 0, false, framed))
					original := n.dispatch(t, dir, 11, wire)
					n09QPACKRequireContext(t, original, "peer SETTINGS not observed")
					require.True(t, q.streams[0].h3.blocked[dir])
					require.Zero(t, q.streams[0].h3.fieldFailure[dir])
					require.Zero(t, q.qpack.table[dir].insertCount)
					require.NotContains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(1))
					originalPDU := original.ID
					original.Raw[0] ^= 1
					original.SourceBytes.PacketRefs[0].Number = 999
					settings := n.send(t, 1-dir, 12, 2, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity, 7, 4))
					require.Equal(t, "decoded", settings.Status)
					rows, ok := settings.Session["QPACK Resumed Field Contexts"].([]map[string]any)
					require.True(t, ok)
					require.Len(t, rows, 1)
					require.Equal(t, originalPDU, rows[0]["Reprocessed From PDU"])
					require.Equal(t, "context-required", rows[0]["Status"])
					require.Equal(t, framed, rows[0]["Field Section Frame Bytes"])
					source := rows[0]["Byte Source"].(map[string]any)
					require.Equal(t, []uint64{originalPDU}, source["Parent PDUs"])
					require.Equal(t, []map[string]any{{"Packet": uint64(11), "Section": uint32(2), "Interface": 7, "Encapsulation": n.domain.Encapsulation}}, source["Packet References"])
					require.Equal(t, originalPDU, q.streams[0].h3.blockedPDU[dir])
					insert := n.send(t, dir, 13, 3, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
					require.Equal(t, "decoded", insert.Status)
					require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
					require.False(t, q.streams[0].h3.blocked[dir])
					require.Equal(t, 1, q.streams[0].h3.headers[dir])
					resumed, ok := insert.Session["QPACK Resumed Streams"].([]map[string]any)
					require.True(t, ok)
					require.Len(t, resumed, 1)
					require.Equal(t, originalPDU, resumed[0]["Blocked PDU ID"])
					frames := resumed[0]["HTTP3 Frames"].([]map[string]any)
					require.Len(t, frames, 1)
					require.Equal(t, []uint64{originalPDU}, frames[0]["Byte Source"].(map[string]any)["Parent PDUs"])
					again := n.dispatch(t, dir, 14, wire)
					require.NotContains(t, again.Session, "QPACK Resumed Streams")
					require.Equal(t, 1, q.streams[0].h3.headers[dir])
					require.Equal(t, "context-required", original.Status, "published original observation is immutable")
				})
			}
		}
	}
}

func TestN09QPACKNativeCompleteFieldPrefixStructureBeforeContext(t *testing.T) {
	for _, payload := range [][]byte{{3}, {3, 0xff}, {0}, {0xff}, {0xff, 0x80}} {
		t.Run(fmt.Sprintf("%x", payload), func(t *testing.T) {
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			n := n09QPACKOwner(t, q)
			e := n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, h3Frame(1, payload))
			require.Equal(t, "malformed", e.Status)
			require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
			require.False(t, q.streams[0].h3.blocked[0])
			require.NotContains(t, e.Session, "QPACK Field Context")
		})
	}
}

func TestN09QPACKNativeKnownZeroIsDifferentFromUnobserved(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprint(dynamic), func(t *testing.T) {
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			n := n09QPACKOwner(t, q)
			setting := n.send(t, 1, 10, 1, "1-RTT", key, 3, 0, h3ControlSETTINGS())
			require.Equal(t, "decoded", setting.Status)
			payload := h3RequestHeaders()
			if dynamic {
				_, _, field := n09QPACKFixture(0)
				payload = h3Frame(1, field)
			}
			e := n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, payload)
			if dynamic {
				require.Equal(t, "malformed", e.Status)
				require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
				require.False(t, q.streams[0].h3.blocked[0])
			} else {
				require.Equal(t, "decoded", e.Status)
				require.Equal(t, 1, q.streams[0].h3.headers[0])
			}
		})
	}
}

func TestN09QPACKNativeEarlyDynamicContextCannotUseCurrentSettings(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, firstKind := range []string{"field", "encoder"} {
				t.Run(fmt.Sprintf("v%x/%x/%s", version, suite, firstKind), func(t *testing.T) {
					q, early, app, finish := earlyRecoveryFixture(t, version, suite, true, false)
					finish(true)
					n := n09QPACKOwner(t, q)
					capacity, encoder, field := n09QPACKFixture(0)
					sid, payload := uint64(0), h3Frame(1, field)
					if firstKind == "encoder" {
						sid, payload = 6, h3EncoderStream(encoder)
					}
					first := n.send(t, 0, 11, 0, "0-RTT", early, sid, 0, payload)
					n09QPACKRequireContext(t, first, "remembered SETTINGS")
					require.NotContains(t, q.spaces[0][quicSpaceApplication].rejected, uint64(0))
					require.Zero(t, q.qpack.table[0].insertCount)
					serverApp := q.keys.app[1][0]
					require.NotNil(t, serverApp)
					settings := n.send(t, 1, 12, 1, "1-RTT", serverApp, 3, 0, h3ControlSETTINGS(1, capacity, 7, 4))
					require.Equal(t, "decoded", settings.Status)
					require.NotContains(t, settings.Session, "QPACK Resumed Field Contexts")
					require.NotContains(t, settings.Session, "QPACK Resumed Encoders")
					require.Zero(t, q.qpack.table[0].insertCount)
					if firstKind == "field" {
						insert := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, h3EncoderStream(encoder))
						require.Equal(t, "decoded", insert.Status)
						require.NotContains(t, insert.Session, "QPACK Resumed Streams")
						require.True(t, q.streams[0].h3.blocked[0])
						require.Zero(t, q.streams[0].h3.headers[0])
						later := n.send(t, 0, 14, 3, "1-RTT", app, 4, 0, h3Frame(1, field))
						require.Equal(t, "decoded", later.Status)
						require.Equal(t, 1, q.streams[4].h3.headers[0])
					} else {
						later := n.send(t, 0, 13, 2, "1-RTT", app, 0, 0, h3RequestHeaders())
						require.Equal(t, "decoded", later.Status)
						require.Zero(t, q.qpack.table[0].insertCount)
						require.Equal(t, 1, q.streams[0].h3.headers[0])
					}
				})
			}
		}
	}
}

func TestN09QPACKNativeEarlyStaticAndLaterPrefixOnSameStream(t *testing.T) {
	for _, earlyFrameHeaderOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(earlyFrameHeaderOnly), func(t *testing.T) {
			q, early, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
			finish(true)
			n := n09QPACKOwner(t, q)
			capacity, encoder, field := n09QPACKFixture(0)
			prefix := h3RequestHeaders()
			if earlyFrameHeaderOnly {
				prefix = h3Frame(1, field)[:2] // only frame type/length; RIC first byte is later 1-RTT
			}
			first := n.send(t, 0, 11, 0, "0-RTT", early, 0, 0, prefix)
			require.Equal(t, "decoded", first.Status)
			require.NotContains(t, first.Session, "QPACK Field Context")
			setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, capacity, 7, 4))
			require.Equal(t, "decoded", setting.Status)
			if earlyFrameHeaderOnly {
				insert := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, h3EncoderStream(encoder))
				require.Equal(t, "decoded", insert.Status)
				last := n.send(t, 0, 14, 3, "1-RTT", app, 0, uint64(len(prefix)), field)
				require.Equal(t, "decoded", last.Status)
				require.Equal(t, 1, q.streams[0].h3.headers[0])
			} else {
				// Capacity + two regular fields; the later block is trailers, so it
				// deliberately contains no pseudo-headers from the earlier request.
				trailEncoder := append([]byte{0x3f, 0x61, 0x46}, []byte("x-test\x03yes\x46y-test\x03yes")...)
				insert := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, h3EncoderStream(trailEncoder))
				require.Equal(t, "decoded", insert.Status)
				later := n.send(t, 0, 14, 3, "1-RTT", app, 0, uint64(len(prefix)), h3Frame(1, rfcHex("03811011")))
				require.Equal(t, "decoded", later.Status)
				require.Equal(t, 2, q.streams[0].h3.headers[0])
			}
		})
	}
}

func TestN09QPACKNativeFieldContextDoesNotCrossCaptureOwner(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	n := n09QPACKOwner(t, q)
	_, _, field := n09QPACKFixture(0)
	first := n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, h3Frame(1, field))
	n09QPACKRequireContext(t, first, "peer SETTINGS not observed")
	for _, foreign := range []CaptureDomain{{Section: 3, Interface: 7, Encapsulation: n.domain.Encapsulation}, {Section: 2, Interface: 8, Encapsulation: n.domain.Encapsulation}, {Section: 2, Interface: 7, Encapsulation: "/vlan:92/pppoe:test-owner"}, {Section: 2, Interface: 7, Encapsulation: "/vlan:91/pppoe:other-owner"}} {
		other, _ := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
		other.alpn, other.byteLimit = "h3", 65536
		f := earlyRecoveryOwner(n.a, other, foreign)
		base := earlyRecoveryBase(f, foreign, 12)
		base.Source, base.Destination = base.Destination, base.Source
		wire := quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(1, 220, 7, 4)))
		events, handled := n.a.decodeQUICDatagram(base, wire)
		require.True(t, handled)
		require.Len(t, events, 1)
		n.a.emit(events[0])
		require.Equal(t, "decoded", events[0].Status)
		require.NotContains(t, events[0].Session, "QPACK Resumed Field Contexts")
		require.False(t, q.qpack.maxKnown[0])
		require.True(t, q.streams[0].h3.blocked[0])
		require.Equal(t, first.ID, q.streams[0].h3.blockedPDU[0])
	}
	require.True(t, bytes.Equal(h3Frame(1, field), q.streams[0].h3.buf[0]))
}

func TestN09QPACKNativeUnknownFieldDoesNotDiscardIndependentStream(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	n := n09QPACKOwner(t, q)
	_, _, field := n09QPACKFixture(0)
	plain := quicStreamFrame(0, 0, false, h3Frame(1, field))
	plain = append(plain, quicStreamFrame(4, 0, false, h3RequestHeaders())...)
	e := n.dispatch(t, 0, 11, quicConformancePacket(t, key, "1-RTT", 1, 0, plain))
	require.Equal(t, "context-required", e.Status)
	require.Equal(t, string(ErrContextRequired), e.ExpertCode)
	require.True(t, q.streams[0].h3.blocked[0])
	require.Equal(t, 1, q.streams[4].h3.headers[0])
	require.NotContains(t, q.spaces[0][quicSpaceApplication].rejected, uint64(1))
	peer := n.send(t, 1, 12, 1, "1-RTT", key, 4, 0, h3ResponseHeaders())
	require.Equal(t, "decoded", peer.Status)
	require.Equal(t, 1, q.streams[4].h3.headers[1])
}

func TestN09QPACKNativeLateZeroOrResetHasOriginalFieldDisposition(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprint(reset), func(t *testing.T) {
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			n := n09QPACKOwner(t, q)
			_, _, field := n09QPACKFixture(0)
			framed := h3Frame(1, field)
			first := n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, framed)
			n09QPACKRequireContext(t, first, "peer SETTINGS not observed")
			if reset {
				plain := append([]byte{4, 0, 0}, quicPutVarint(uint64(len(framed)))...)
				stop := n.dispatch(t, 0, 12, quicConformancePacket(t, key, "1-RTT", 2, 0, plain))
				require.Equal(t, "decoded", stop.Status)
				require.True(t, q.streams[0].resetDirection[0])
			}
			setting := n.send(t, 1, 13, 1, "1-RTT", key, 3, 0, h3ControlSETTINGS())
			require.Equal(t, "decoded", setting.Status, "current SETTINGS itself is valid")
			if reset {
				require.NotContains(t, setting.Session, "QPACK Resumed Field Contexts")
			} else {
				rows, ok := setting.Session["QPACK Resumed Field Contexts"].([]map[string]any)
				require.True(t, ok)
				require.Len(t, rows, 1)
				require.Equal(t, "malformed", rows[0]["Status"])
				require.Equal(t, string(ErrMalformedMessage), rows[0]["Expert Code"])
				require.Equal(t, first.ID, rows[0]["Reprocessed From PDU"])
				require.Equal(t, []uint64{first.ID}, rows[0]["Byte Source"].(map[string]any)["Parent PDUs"])
			}
			require.Zero(t, q.streams[0].h3.headers[0])
		})
	}
}
