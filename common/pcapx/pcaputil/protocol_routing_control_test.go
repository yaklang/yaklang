package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
	"time"
)

func routingBFD(my, peer uint32, flags byte) []byte {
	w := make([]byte, 24)
	w[0], w[1], w[2], w[3] = 0x20, flags, 3, 24
	binary.BigEndian.PutUint32(w[4:], my)
	binary.BigEndian.PutUint32(w[8:], peer)
	binary.BigEndian.PutUint32(w[12:], 100000)
	binary.BigEndian.PutUint32(w[16:], 200000)
	return w
}
func routingHSRP2(identifier byte) []byte {
	w := make([]byte, 42)
	copy(w, []byte{1, 40, 2, 0, 6, 4, 0, 1})
	w[13] = identifier
	binary.BigEndian.PutUint32(w[14:], 100)
	binary.BigEndian.PutUint32(w[18:], 3000)
	binary.BigEndian.PutUint32(w[22:], 10000)
	copy(w[26:], []byte{192, 0, 2, 1})
	return w
}
func routingGLBP(group byte, owner byte) []byte {
	w := []byte{1, 0, 0, group, 0, 0, 0, 1, 2, 3, 4, owner, 2, 20, 1, 32, 0, 100, 50, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0xb4, 0, 1, 1}
	return w
}
func routingParser(t *testing.T) *binParser {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	t.Cleanup(c.binParser.closeUDPSessions)
	return c.binParser
}
func routingEvent(at int64, reverse bool, domain int) *ProtocolEvent {
	e := &ProtocolEvent{Transport: "udp", Source: "192.0.2.1:51000", Destination: "192.0.2.2:3784", Timestamp: time.Unix(at, 0), Domain: CaptureDomain{Interface: domain}}
	if reverse {
		e.Source, e.Destination = "192.0.2.2:52000", "192.0.2.1:3784"
	}
	return e
}
func TestRoutingBFDDiscriminatorGenerationsAndPollEvidence(t *testing.T) {
	a := routingParser(t)
	decode := func(at int64, reverse bool, my, peer uint32, flags byte, domain int) *ProtocolEvent {
		e := routingEvent(at, reverse, domain)
		require.True(t, a.decodeControlDatagram(e, routingBFD(my, peer, flags), 51000, 3784, ""))
		require.Equal(t, "decoded", e.Status, e.Error)
		return e
	}
	one := decode(1, false, 11, 0, 0x40, 0)
	two := decode(2, true, 22, 11, 0x80, 0)
	require.Equal(t, one.FlowID, two.FlowID)
	poll := decode(3, false, 11, 22, 0xe0, 0)
	require.Equal(t, one.FlowID, poll.FlowID)
	final := decode(4, true, 22, 11, 0xd0, 0)
	require.Equal(t, poll.ID, final.ResponseTo)
	// A missing Final makes later Polls ambiguous, even on the same pair.
	decode(5, false, 11, 22, 0xe0, 0)
	decode(6, false, 11, 22, 0xe0, 0)
	final = decode(7, true, 22, 11, 0xd0, 0)
	require.Zero(t, final.ResponseTo)
	// Historical arrival must not consume pending evidence from the future.
	poll = decode(10, false, 11, 22, 0xe0, 0)
	require.Zero(t, decode(9, true, 22, 11, 0xd0, 0).ResponseTo)
	require.Equal(t, poll.ID, decode(11, true, 22, 11, 0xd0, 0).ResponseTo)
	// A different capture domain and different discriminator pair cannot bind.
	require.NotEqual(t, one.FlowID, decode(12, true, 22, 11, 0xc0, 1).FlowID)
	require.NotEqual(t, one.FlowID, decode(13, true, 23, 11, 0xc0, 0).FlowID)
	// Observed Down bootstrap after both sides were seen starts a generation.
	reused := decode(14, false, 11, 0, 0x40, 0)
	require.NotEqual(t, one.FlowID, reused.FlowID)
	require.Zero(t, decode(15, true, 22, 11, 0xd0, 0).ResponseTo)
	// Timeout prevents old pending Poll context leaking into a later session.
	poll = decode(16, false, 11, 22, 0xe0, 0)
	final = decode(140, true, 22, 11, 0xd0, 0)
	require.Zero(t, final.ResponseTo)
	require.NotEqual(t, poll.FlowID, final.FlowID)
	// Service-port hints never admit malformed control messages.
	for _, bad := range [][]byte{routingBFD(0, 22, 0xc0), routingBFD(11, 0, 0xc0), routingBFD(11, 22, 0xf0), []byte{0x20, 0xc0}} {
		e := routingEvent(150, false, 0)
		require.False(t, a.decodeControlDatagram(e, bad, 51000, 3784, ""))
		require.Zero(t, e.FlowID)
	}
}
func TestRoutingSenderContextIsolationAndNoFieldInheritance(t *testing.T) {
	a := routingParser(t)
	short := []byte{2, 4, 0, 1, 0, 2}
	decode := func(w []byte, at int64, domain int, source string) (*ProtocolEvent, bool) {
		e := routingEvent(at, false, domain)
		e.Source = source
		return e, a.decodeControlDatagram(e, w, 51000, 1985, "")
	}
	e, ok := decode(short, 1, 0, "192.0.2.1:1985")
	require.False(t, ok)
	require.Zero(t, e.FlowID)
	full, ok := decode(routingHSRP2(1), 2, 0, "192.0.2.1:1985")
	require.True(t, ok)
	e, ok = decode(short, 3, 0, full.Source)
	require.True(t, ok)
	require.Equal(t, full.FlowID, e.FlowID)
	require.NotContains(t, e.Session, "Group")
	require.NotContains(t, e.Session, "Virtual IP")
	_, ok = decode(short, 4, 1, full.Source)
	require.False(t, ok)
	_, ok = decode(short, 5, 0, "192.0.2.2:1985")
	require.False(t, ok)
	newSender, ok := decode(routingHSRP2(2), 6, 0, full.Source)
	require.True(t, ok)
	require.NotEqual(t, full.FlowID, newSender.FlowID)
	_, ok = decode(short, 130, 0, full.Source)
	require.False(t, ok)
	// A future timestamp in an unrelated capture interface cannot expire it.
	_, ok = decode(routingHSRP2(2), 7, 0, full.Source)
	require.True(t, ok)
	_, ok = decode(routingHSRP2(3), 10000, 5, full.Source)
	require.True(t, ok)
	_, ok = decode(short, 8, 0, full.Source)
	require.True(t, ok)
	// GLBP groups and changed owner identity remain separate observations.
	gd := func(w []byte) *ProtocolEvent {
		e := routingEvent(10, false, 0)
		require.True(t, a.decodeControlDatagram(e, w, 51000, 3222, ""))
		return e
	}
	g1, g2 := gd(routingGLBP(1, 1)), gd(routingGLBP(2, 1))
	require.NotEqual(t, g1.FlowID, g2.FlowID)
	changed := gd(routingGLBP(1, 2))
	require.NotEqual(t, g1.FlowID, changed.FlowID)
	unknown := append(bytes.Clone(routingGLBP(1, 1)[:12]), 99, 2)
	e = routingEvent(11, false, 0)
	require.False(t, a.decodeControlDatagram(e, unknown, 51000, 3222, ""))
	unknown[11] = 2
	e = routingEvent(12, false, 0)
	require.True(t, a.decodeControlDatagram(e, unknown, 51000, 3222, ""))
	require.Equal(t, changed.FlowID, e.FlowID)
}
func TestRoutingCodecAuthenticationBoundariesAndWireOwnership(t *testing.T) {
	w := routingBFD(1, 2, 0xc4)
	w[3] = 48
	w = append(w, 2, 24, 7, 0, 0, 0, 0, 5)
	w = append(w, make([]byte, 16)...)
	m, err := stream_parser.DecodeRoutingControl(w, "bfd", 16)
	require.NoError(t, err)
	require.False(t, m["Authentication Verified"].(bool))
	require.Equal(t, uint64(5), m["Authentication"].(map[string]any)["Sequence"])
	for cut := 24; cut < len(w); cut++ {
		_, err = stream_parser.DecodeRoutingControl(w[:cut], "bfd", 16)
		require.Error(t, err)
	}
	m["Authentication"].(map[string]any)["Digest"].([]byte)[0] = 99
	require.Zero(t, w[32])
	w[25] = 23
	_, err = stream_parser.DecodeRoutingControl(w, "bfd", 16)
	require.Error(t, err)
	for _, tc := range []struct {
		proto string
		wire  []byte
	}{{"hsrp", routingHSRP2(1)}, {"glbp", routingGLBP(1, 1)}, {"bfd", routingBFD(1, 2, 0xc0)}} {
		for i := 0; i < len(tc.wire); i++ {
			_, err = stream_parser.DecodeRoutingControl(tc.wire[:i], tc.proto, 16)
			require.Error(t, err, "%s cut %d", tc.proto, i)
		}
		_, err = stream_parser.DecodeRoutingControl(tc.wire, tc.proto, 0)
		require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	}
	a := routingParser(t)
	e := routingEvent(1, false, 0)
	require.True(t, a.decodeControlDatagram(e, routingHSRP2(1), 51000, 1985, ""))
	fields, err := e.GetFields()
	require.NoError(t, err)
	fields["Group"] = uint64(999)
	require.Equal(t, uint64(1), e.Session["Group"])
	// Advertisement opcode 3 has a 14-byte TLV, including its four-byte header.
	adv := []byte{0, 3, 0, 1, 0, 14, 2, 0, 0, 0, 0, 1, 111, 0, 0, 0}
	m, err = stream_parser.DecodeRoutingControl(adv, "hsrp", 16)
	require.NoError(t, err)
	require.Equal(t, uint64(1862270976), m["Advertisement Reserved Tail"])
}
func FuzzRoutingControlDatagrams(f *testing.F) {
	for _, w := range [][]byte{routingBFD(1, 2, 0xe0), routingHSRP2(1), routingGLBP(1, 1), {2, 4, 0, 1, 0, 2}} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65508 {
			return
		}
		for _, proto := range []string{"hsrp", "glbp", "bfd"} {
			m, err := stream_parser.DecodeRoutingControl(w, proto, 32)
			if err == nil {
				require.NotEmpty(t, m["Packet Name"])
				require.False(t, m["Authentication Verified"].(bool))
			}
		}
	})
}

func TestRoutingObservationCapacityAndRelease(t *testing.T) {
	a := routingParser(t)
	a.budget.MaxCollectionElements = 1
	one := routingEvent(1, false, 0)
	require.True(t, a.decodeControlDatagram(one, routingBFD(1, 2, 0xc0), 51000, 3784, ""))
	require.NotZero(t, one.FlowID)
	other := routingEvent(2, false, 0)
	require.True(t, a.decodeControlDatagram(other, routingBFD(3, 4, 0xc0), 51000, 3784, ""))
	require.Zero(t, other.FlowID)
	require.Contains(t, other.Session["Association"], "capacity")
	require.Len(t, a.udpSessions.entries, 1)
	require.NotZero(t, a.buffered.Load())
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
	// Field queries remain the accepted observation after public edits.
	one.Session = map[string]any{"My Discriminator": uint64(999)}
	decoded, err := one.Decode()
	require.NoError(t, err)
	require.NotNil(t, decoded)
	require.Equal(t, []any{uint64(1)}, one.DisplayFields()["bfd.my_discriminator"])
}

func FuzzRoutingAssociationIsolation(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			return
		}
		a := routingParser(t)
		a.budget.MaxCollectionElements = 16
		for i, b := range data {
			e := routingEvent(int64(i+1), b&1 != 0, int(b>>6))
			my, peer := uint32(b&7)+1, uint32(b>>3&7)+20
			if b&1 != 0 {
				my, peer = peer, my
			}
			require.True(t, a.decodeControlDatagram(e, routingBFD(my, peer, 0xc0), 51000, 3784, ""))
			bad := routingEvent(int64(i+1), false, 0)
			require.False(t, a.decodeControlDatagram(bad, []byte{b, 0}, 51000, 3784, ""))
			require.Zero(t, bad.FlowID)
		}
		if a.udpSessions != nil {
			require.LessOrEqual(t, len(a.udpSessions.entries), 16)
		}
		a.closeUDPSessions()
		require.Zero(t, a.buffered.Load())
	})
}
