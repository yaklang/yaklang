package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func carrierTestParser(t *testing.T) *binParser {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	t.Cleanup(func() { a.closeUDPSessions(); require.Zero(t, a.buffered.Load()) })
	return a
}
func carrierTestParent(n uint64, reverse bool) *ProtocolEvent {
	s, d := "192.0.2.1:50000", "192.0.2.2:3389"
	if reverse {
		s, d = d, s
	}
	return &ProtocolEvent{ID: n, Transport: "udp", Source: s, Destination: d, Timestamp: time.Unix(int64(n), 0), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: n}}}}
}
func carrierTestRecord(typ byte, b []byte) []byte {
	return append([]byte{typ, 3, 3, byte(len(b) >> 8), byte(len(b))}, b...)
}
func carrierTestHello() []byte {
	return carrierTestRecord(22, reliabilityHS(1, reliabilityHello(true, false, 29, nil)))
}
func carrierTestSYN(ack bool, isn, peer uint32) []byte {
	w := make([]byte, 1132)
	binary.BigEndian.PutUint32(w, peer)
	binary.BigEndian.PutUint16(w[4:], 64)
	flags := uint16(0x1001)
	if ack {
		flags |= 4
	}
	binary.BigEndian.PutUint16(w[6:], flags)
	binary.BigEndian.PutUint32(w[8:], isn)
	binary.BigEndian.PutUint16(w[12:], 1132)
	binary.BigEndian.PutUint16(w[14:], 1132)
	binary.BigEndian.PutUint16(w[16:], 1)
	binary.BigEndian.PutUint16(w[18:], 0x101)
	return w
}
func carrierTestRDP2(channel, dataSeq uint16, b []byte) []byte {
	w := make([]byte, 7)
	w[0] = 0xe0
	binary.LittleEndian.PutUint16(w[1:], 0xf004)
	binary.LittleEndian.PutUint16(w[3:], dataSeq)
	binary.LittleEndian.PutUint16(w[5:], channel)
	w = append(w, b...)
	if len(w) < 8 {
		w = append(w, 0)
	}
	w[0], w[7] = w[7], w[0]
	return w
}
func carrierTestRDPHandshake(t *testing.T, a *binParser) uint64 {
	es, ok := a.decodeRDPUDPDatagram(carrierTestParent(1, false), carrierTestSYN(false, 1000, 0xffffffff))
	require.True(t, ok)
	require.Len(t, es, 1)
	es, ok = a.decodeRDPUDPDatagram(carrierTestParent(2, true), carrierTestSYN(true, 2000, 1000))
	require.True(t, ok)
	require.Len(t, es, 1)
	require.Equal(t, "decoded", es[0].Status)
	return es[0].FlowID
}
func TestRDPUDPReceiverSplitReorderWrapRetransmission(t *testing.T) {
	w := carrierTestHello()
	for _, cut := range []int{1, 4, 5, 8, len(w) - 1} {
		t.Run(fmt.Sprintf("cut-%d", cut), func(t *testing.T) {
			a := carrierTestParser(t)
			flow := carrierTestRDPHandshake(t, a)
			es, ok := a.decodeRDPUDPDatagram(carrierTestParent(3, false), carrierTestRDP2(0, 88, w[cut:]))
			require.True(t, ok)
			require.Len(t, es, 1)
			es, _ = a.decodeRDPUDPDatagram(carrierTestParent(4, false), carrierTestRDP2(65535, 87, w[:cut]))
			require.Len(t, es, 2)
			tls := es[1]
			require.Equal(t, "tls", tls.Protocol)
			require.Equal(t, "decoded", tls.Status, tls.Error)
			require.Equal(t, w, tls.Raw)
			require.Equal(t, flow, tls.FlowID)
			require.Equal(t, []string{"RDPUDP2"}, tls.CarrierLayers)
			require.Equal(t, "udp", tls.Transport)
			require.Equal(t, "reassembled", tls.SourceBytes.Kind)
			require.Len(t, tls.SourceBytes.PacketRefs, 2)
			require.Equal(t, uint64(4), tls.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, uint64(3), tls.SourceBytes.PacketRefs[1].Number)
			es, _ = a.decodeRDPUDPDatagram(carrierTestParent(5, false), carrierTestRDP2(0, 99, w[cut:]))
			require.Len(t, es, 1)
			require.Equal(t, true, es[0].Session["Retransmission"])
			bad := bytes.Clone(w[cut:])
			bad[len(bad)-1] ^= 1
			es, _ = a.decodeRDPUDPDatagram(carrierTestParent(6, false), carrierTestRDP2(0, 100, bad))
			require.Equal(t, "malformed", es[0].Status)
			es, _ = a.decodeRDPUDPDatagram(carrierTestParent(7, false), carrierTestRDP2(1, 101, w))
			require.Len(t, es, 1)
			require.Equal(t, "context-required", es[0].Status)
		})
	}
}
func TestRDPUDPRequiresCapturedGenerationAndKeepsGaps(t *testing.T) {
	a := carrierTestParser(t)
	w := carrierTestHello()
	_, ok := a.decodeRDPUDPDatagram(carrierTestParent(1, false), carrierTestRDP2(100, 1, w))
	require.False(t, ok)
	flow := carrierTestRDPHandshake(t, a)
	es, _ := a.decodeRDPUDPDatagram(carrierTestParent(3, false), carrierTestRDP2(100, 1, w))
	require.Len(t, es, 2)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(4, false), carrierTestRDP2(102, 3, w))
	require.Len(t, es, 1)
	require.Contains(t, es[0].Session["Reassembly"], "gap")
	// A fresh SYN alone must not move encrypted data into a new generation.
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(5, false), carrierTestSYN(false, 3000, 0xffffffff))
	require.NotEqual(t, flow, es[0].FlowID)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(6, false), carrierTestRDP2(101, 2, w))
	require.Equal(t, "context-required", es[0].Status)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(7, true), carrierTestSYN(true, 4000, 3000))
	require.NotEqual(t, flow, es[len(es)-1].FlowID)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(8, false), carrierTestRDP2(100, 1, w))
	require.Len(t, es, 2)
	require.NotEqual(t, flow, es[1].FlowID)
}

func TestRDPUDPRecoversDataBeforePeerSYNAndIsolatesDomain(t *testing.T) {
	a := carrierTestParser(t)
	w := carrierTestHello()
	a.decodeRDPUDPDatagram(carrierTestParent(1, false), carrierTestSYN(false, 1000, 0xffffffff))
	es, _ := a.decodeRDPUDPDatagram(carrierTestParent(2, false), carrierTestRDP2(100, 1, w))
	require.Len(t, es, 1)
	require.Equal(t, "context-required", es[0].Status)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(3, true), carrierTestSYN(true, 2000, 1000))
	require.Len(t, es, 3)
	require.Equal(t, "tls", es[2].Protocol)
	require.Equal(t, w, es[2].Raw)
	require.Equal(t, uint64(2), es[2].SourceBytes.PacketRefs[0].Number)
	p := carrierTestParent(4, false)
	p.Domain.Interface = 2
	p.SourceBytes.PacketRefs[0].Domain = p.Domain
	_, ok := a.decodeRDPUDPDatagram(p, carrierTestRDP2(101, 2, w))
	require.False(t, ok)
}

func TestRDPUDPHelloAcrossTLSRecords(t *testing.T) {
	a := carrierTestParser(t)
	carrierTestRDPHandshake(t, a)
	hs := reliabilityHS(1, reliabilityHello(true, false, 29, nil))
	first := carrierTestRecord(22, hs[:2])
	second := carrierTestRecord(22, hs[2:])
	es, _ := a.decodeRDPUDPDatagram(carrierTestParent(3, false), carrierTestRDP2(100, 1, first))
	require.Len(t, es, 1)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(4, false), carrierTestRDP2(101, 2, second))
	require.Len(t, es, 3)
	require.Equal(t, first, es[1].Raw)
	require.Equal(t, second, es[2].Raw)
	require.Equal(t, "decoded", es[2].Status, es[2].Error)
}

func TestRDPUDPScratchBudgetUsesObservedBytes(t *testing.T) {
	a := carrierTestParser(t)
	a.config.MaxBufferedBytes = 32 << 10
	a.budget.MaxBufferedBytes = 32 << 10
	carrierTestRDPHandshake(t, a)
	es, _ := a.decodeRDPUDPDatagram(carrierTestParent(3, false), carrierTestRDP2(100, 1, carrierTestHello()))
	require.Len(t, es, 2)
	require.Equal(t, "decoded", es[1].Status, es[1].Error)
}

func TestRDPUDPLegacyFINClosesTLSGeneration(t *testing.T) {
	a := carrierTestParser(t)
	for i, ack := range []bool{false, true} {
		isn, peer := uint32(1000), uint32(0xffffffff)
		flags := uint16(1)
		if ack {
			isn, peer, flags = 2000, 1000, 5
		}
		w := carrierTestSYN(ack, isn, peer)
		binary.BigEndian.PutUint16(w[6:], flags)
		clear(w[16:])
		es, ok := a.decodeRDPUDPDatagram(carrierTestParent(uint64(i+1), ack), w)
		require.True(t, ok)
		require.Equal(t, "decoded", es[0].Status)
	}
	data := make([]byte, 16)
	binary.BigEndian.PutUint32(data, 0xffffffff)
	binary.BigEndian.PutUint16(data[4:], 64)
	binary.BigEndian.PutUint16(data[6:], 8)
	binary.BigEndian.PutUint32(data[8:], 1)
	binary.BigEndian.PutUint32(data[12:], 1001)
	data = append(data, carrierTestHello()...)
	es, _ := a.decodeRDPUDPDatagram(carrierTestParent(3, false), data)
	require.Len(t, es, 2)
	require.Equal(t, "decoded", es[1].Status)
	fin := make([]byte, 8)
	binary.BigEndian.PutUint32(fin, 1001)
	binary.BigEndian.PutUint16(fin[4:], 64)
	binary.BigEndian.PutUint16(fin[6:], 2)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(4, true), fin)
	require.Contains(t, es[0].Session["Connection State"], "FIN-observed")
	binary.BigEndian.PutUint32(data[12:], 1002)
	es, _ = a.decodeRDPUDPDatagram(carrierTestParent(5, false), data)
	require.Len(t, es, 1)
	require.Equal(t, "context-required", es[0].Status)
}

func TestEAPTLSIncompleteFlightAndBudgetRelease(t *testing.T) {
	a := carrierTestParser(t)
	state := []byte("state")
	w := carrierTestHello()
	a.decodeEAPTLSCarrier(carrierTestParent(1, false), carrierTestRadius(1, 1, 1, 1, 0, nil, []byte("user"), 0))
	a.decodeEAPTLSCarrier(carrierTestParent(2, true), carrierTestRadius(1, 11, 2, 25, 32, state, nil, 0))
	require.Empty(t, a.decodeEAPTLSCarrier(carrierTestParent(3, false), carrierTestRadius(2, 1, 2, 25, 192, state, w[:8], uint32(len(w)))))
	es := a.closeCarrierSessions()
	require.Len(t, es, 1)
	require.Equal(t, "incomplete", es[0].Status)
	require.Equal(t, w[:8], es[0].Raw)
	require.Equal(t, uint64(3), es[0].SourceBytes.PacketRefs[0].Number)
	require.Zero(t, a.buffered.Load())
	require.Empty(t, a.closeCarrierSessions())
}

func FuzzCarrierTLSFraming(f *testing.F) {
	f.Add(carrierTestHello(), uint16(7))
	f.Add([]byte{23, 3, 3, 255, 255}, uint16(2))
	f.Add([]byte{22, 3}, uint16(1))
	f.Fuzz(func(t *testing.T, w []byte, cut uint16) {
		if len(w) > 4096 {
			return
		}
		a := carrierTestParser(t)
		a.budget.MaxMessageBytes = 4096
		c := newCarrierTLS(a, 1, [2]string{"a", "b"}, CaptureDomain{}, "PEAP")
		n := min(int(cut), len(w))
		c.feed(0, w[:n], carrierTestParent(1, false))
		c.feed(0, w[n:], carrierTestParent(2, false))
		c.close("fuzz")
		require.Zero(t, a.buffered.Load())
	})
}

func TestCarrierTLSAuthenticatedRecordRecovery(t *testing.T) {
	a := carrierTestParser(t)
	c := newCarrierTLS(a, 1, [2]string{"a", "b"}, CaptureDomain{}, "RDPUDP2")
	s := newBinTLS()
	s.version, s.suite, s.client = 0x304, 0x1301, 0
	s.random = bytes.Repeat([]byte{4}, 32)
	s.app = [2]bool{true, true}
	for dir := 0; dir < 2; dir++ {
		require.NoError(t, s.install13(dir, bytes.Repeat([]byte{byte(dir + 1)}, 32)))
	}
	c.f.tls = s
	bad := recoveryTLSRecord(t, s, 0, 0, []byte("corrupt"))
	bad[len(bad)-1] ^= 1
	es := c.feed(0, bad, carrierTestParent(1, false))
	require.Len(t, es, 1)
	require.NotEmpty(t, es[0].Error)
	require.False(t, c.broken)
	es = c.feed(0, recoveryTLSRecord(t, s, 0, 5, []byte("after loss")), carrierTestParent(2, false))
	require.Len(t, es, 1)
	require.Equal(t, true, es[0].Session["Authentication Verified"])
	require.Equal(t, true, es[0].Session["Record Recovery Verified"])
	es = c.feed(1, recoveryTLSRecord(t, s, 1, 0, []byte("reverse")), carrierTestParent(3, true))
	require.Len(t, es, 1)
	require.Equal(t, true, es[0].Session["Authentication Verified"])
	require.Equal(t, []byte("reverse"), es[0].Session["Authenticated Plaintext"])
	require.NotContains(t, es[0].Session, "HTTP")
	c.close("test")
	require.Zero(t, a.buffered.Load())
}

func carrierTestRadius(rid, code, eapID, method, flags byte, state, body []byte, declared uint32) []byte {
	eap := []byte{2, eapID, 0, 0, method}
	if code == 11 {
		eap[0] = 1
	}
	if method == 13 || method == 21 || method == 25 {
		eap = append(eap, flags)
		if flags&128 != 0 {
			eap = binary.BigEndian.AppendUint32(eap, declared)
		}
	}
	eap = append(eap, body...)
	binary.BigEndian.PutUint16(eap[2:], uint16(len(eap)))
	w := make([]byte, 20)
	w[0], w[1] = code, rid
	for len(eap) > 0 {
		n := min(253, len(eap))
		w = append(w, 79, byte(n+2))
		w = append(w, eap[:n]...)
		eap = eap[n:]
	}
	if len(state) > 0 {
		w = append(w, 24, byte(len(state)+2))
		w = append(w, state...)
	}
	w = append(w, 80, 18)
	w = append(w, make([]byte, 16)...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return w
}
func TestEAPTLSObservedStateChainAndFragments(t *testing.T) {
	for _, method := range []byte{21, 25} {
		for _, reverseOrder := range []bool{false, true} {
			t.Run(fmt.Sprintf("method-%d/reordered-%t", method, reverseOrder), func(t *testing.T) {
				a := carrierTestParser(t)
				w := carrierTestHello()
				state := []byte("opaque-state")
				inputs := [][]byte{
					carrierTestRadius(1, 1, 1, 1, 0, nil, []byte("test"), 0),
					carrierTestRadius(1, 11, 2, method, 32, state, nil, 0),
					carrierTestRadius(2, 1, 2, method, 192, state, w[:9], uint32(len(w))),
					carrierTestRadius(2, 11, 3, method, 0, state, nil, 0),
					carrierTestRadius(3, 1, 3, method, 0, state, w[9:], 0),
				}
				order := []int{0, 1, 2, 3, 4}
				if reverseOrder {
					order = []int{4, 3, 2, 1, 0}
				}
				var all []*ProtocolEvent
				for _, i := range order {
					all = append(all, a.decodeEAPTLSCarrier(carrierTestParent(uint64(i+1), i%2 == 1), inputs[i])...)
				}
				require.Len(t, all, 1)
				e := all[0]
				require.Equal(t, w, e.Raw)
				require.Equal(t, "decoded", e.Status, e.Error)
				require.Len(t, e.SourceBytes.PacketRefs, 2)
				require.Equal(t, []uint64{3, 5}, []uint64{e.SourceBytes.PacketRefs[0].Number, e.SourceBytes.PacketRefs[1].Number})
				require.Equal(t, "RADIUS", e.CarrierLayers[0])
				require.Equal(t, "udp", e.Transport)
				for i := range inputs {
					require.Empty(t, a.decodeEAPTLSCarrier(carrierTestParent(uint64(20+i), i%2 == 1), inputs[i]))
				}
			})
		}
	}
}
func TestEAPTLSRejectsFalseBoundariesAndWrongContext(t *testing.T) {
	a := carrierTestParser(t)
	w := carrierTestHello()
	state := []byte("state")
	feed := func(n uint64, reverse bool, b []byte) []*ProtocolEvent {
		return a.decodeEAPTLSCarrier(carrierTestParent(n, reverse), b)
	}
	require.Empty(t, feed(1, false, carrierTestRadius(2, 1, 2, 25, 128, state, w, uint32(len(w))))) // no Start
	feed(2, false, carrierTestRadius(1, 1, 1, 1, 0, nil, []byte("user"), 0))
	feed(3, true, carrierTestRadius(1, 11, 2, 25, 32, state, nil, 0))
	wrong := carrierTestRadius(2, 1, 2, 25, 128, []byte("other"), w, uint32(len(w)))
	require.Empty(t, feed(4, false, wrong))
	// An invalid attribute tail cannot contribute its otherwise valid TLS prefix.
	bad := carrierTestRadius(2, 1, 2, 25, 128, state, w, uint32(len(w)))
	bad = append(bad, 99, 1)
	binary.BigEndian.PutUint16(bad[2:], uint16(len(bad)))
	require.Empty(t, feed(5, false, bad))
	for cut := 0; cut < len(bad); cut++ {
		_, err := parseEAPTLSCarrier(carrierTestParent(6, false), bad[:cut], 4096)
		require.Error(t, err)
	}
}
func TestCarrierTLSAllRecordCutsAndOwnedFields(t *testing.T) {
	w := carrierTestHello()
	for cut := 1; cut < len(w); cut++ {
		a := carrierTestParser(t)
		c := newCarrierTLS(a, 9, [2]string{"a", "b"}, CaptureDomain{}, "PEAP")
		require.Empty(t, c.feed(0, w[:cut], carrierTestParent(1, false)))
		es := c.feed(0, w[cut:], carrierTestParent(2, false))
		require.Len(t, es, 1)
		e := es[0]
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Equal(t, w, e.Raw)
		want, err := e.GetFields()
		require.NoError(t, err)
		e.Session = map[string]any{"Record Length": 999}
		e.Fields = nil
		got, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, want, got)
		c.close("test")
		require.Zero(t, a.buffered.Load())
	}
}
