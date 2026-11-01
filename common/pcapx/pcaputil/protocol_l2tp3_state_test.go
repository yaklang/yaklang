package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func l2tp3TestAVP(typ uint16, value []byte) []byte {
	w := make([]byte, 6+len(value))
	binary.BigEndian.PutUint16(w, uint16(len(w))|0x8000)
	binary.BigEndian.PutUint16(w[4:], typ)
	copy(w[6:], value)
	return w
}
func l2tp3TestNumber(typ uint16, value uint32, size int) []byte {
	w := make([]byte, 4)
	binary.BigEndian.PutUint32(w, value)
	return l2tp3TestAVP(typ, w[4-size:])
}
func l2tp3TestControl(udp bool, cid uint32, ns, nr, mt uint16, avps ...[]byte) []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w, 0xc803)
	binary.BigEndian.PutUint32(w[4:], cid)
	binary.BigEndian.PutUint16(w[8:], ns)
	binary.BigEndian.PutUint16(w[10:], nr)
	if mt != 0 {
		w = append(w, l2tp3TestNumber(0, uint32(mt), 2)...)
	}
	for _, a := range avps {
		w = append(w, a...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	if !udp {
		w = append([]byte{0, 0, 0, 0}, w...)
	}
	return w
}
func l2tp3TestSetup(udp bool, cid0, cid1, sid0, sid1 uint32, cookie0, cookie1 []byte, sequencing uint16) [][]byte {
	caps := []byte{0, 5}
	setup := func(cid uint32) [][]byte {
		return [][]byte{l2tp3TestAVP(7, []byte("router")), l2tp3TestAVP(60, []byte{192, 0, 2, 1}), l2tp3TestNumber(61, cid, 4), l2tp3TestAVP(62, caps)}
	}
	rq := [][]byte{l2tp3TestNumber(63, sid0, 4), l2tp3TestNumber(64, 0, 4), l2tp3TestNumber(15, 1, 4), l2tp3TestNumber(68, 5, 2), l2tp3TestAVP(66, []byte("eth0")), l2tp3TestNumber(71, 2, 2), l2tp3TestAVP(65, cookie0), l2tp3TestNumber(69, 1, 2), l2tp3TestNumber(70, uint32(sequencing), 2)}
	rp := [][]byte{l2tp3TestNumber(63, sid1, 4), l2tp3TestNumber(64, sid0, 4), l2tp3TestNumber(71, 3, 2), l2tp3TestAVP(65, cookie1), l2tp3TestNumber(69, 1, 2), l2tp3TestNumber(70, uint32(sequencing), 2)}
	cn := [][]byte{l2tp3TestNumber(63, sid0, 4), l2tp3TestNumber(64, sid1, 4), l2tp3TestNumber(71, 1, 2)}
	return [][]byte{l2tp3TestControl(udp, 0, 0, 0, 1, setup(cid0)...), l2tp3TestControl(udp, cid0, 0, 1, 2, setup(cid1)...), l2tp3TestControl(udp, cid1, 1, 1, 3), l2tp3TestControl(udp, cid1, 2, 1, 10, rq...), l2tp3TestControl(udp, cid0, 1, 3, 11, rp...), l2tp3TestControl(udp, cid1, 3, 2, 12, cn...)}
}

type l2tp3TestPeer struct {
	a      *binParser
	udp    bool
	domain CaptureDomain
	n      uint64
}

func l2tp3TestParser(t *testing.T, udp bool) *l2tp3TestPeer {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	v := &l2tp3TestPeer{a: c.binParser, udp: udp, domain: CaptureDomain{Interface: 3}}
	t.Cleanup(func() { v.a.closeUDPSessions(); require.Zero(t, v.a.buffered.Load()) })
	return v
}
func (v *l2tp3TestPeer) feed(w []byte, dir int, initial bool) (*ProtocolEvent, []byte, string) {
	src, dst := "192.0.2.1", "192.0.2.2"
	if v.udp {
		src, dst = "192.0.2.1:40000", "192.0.2.2:50000"
		if initial {
			dst = "192.0.2.2:1701"
		}
	}
	if dir == 1 {
		src, dst = dst, src
	}
	v.n++
	e := &ProtocolEvent{Domain: v.domain, Transport: "network", Source: src, Destination: dst, Timestamp: time.Unix(int64(v.n), 0), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: v.n, Domain: v.domain}}}}
	if v.udp {
		e.Transport = "udp"
	}
	inner, suffix, _ := v.a.decodeL2TPv3Event(e, w, v.udp, false, nil)
	return e, inner, suffix
}
func (v *l2tp3TestPeer) setup(ws [][]byte) {
	for i, w := range ws {
		v.feed(w, []int{0, 1, 0, 0, 1, 0}[i], i == 0)
	}
}
func l2tp3TestData(udp bool, sid uint32, cookie []byte, seq uint32) []byte {
	w := make([]byte, 4)
	binary.BigEndian.PutUint32(w, sid)
	if udp {
		w = append([]byte{0, 3, 0, 0}, w...)
	}
	w = append(w, cookie...)
	x := make([]byte, 4)
	binary.BigEndian.PutUint32(x, 0x40000000|seq)
	w = append(w, x...)
	return append(w, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 8, 0, 0x45, 0, 0, 20, 1, 2, 3, 4}...)
}

func TestL2TPv3ObservedReceiverDirectionAndDynamicUDPPort(t *testing.T) {
	for _, udp := range []bool{false, true} {
		t.Run(map[bool]string{false: "IP", true: "UDP"}[udp], func(t *testing.T) {
			v := l2tp3TestParser(t, udp)
			c0, c1 := []byte{0, 0, 0, 0}, []byte{1, 2, 3, 4, 5, 6, 7, 8}
			v.setup(l2tp3TestSetup(udp, 100, 200, 300, 400, c0, c1, 2))
			e, inner, suffix := v.feed(l2tp3TestData(udp, 400, c1, 7), 0, false)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.True(t, e.Session["Session Setup Complete"].(bool))
			require.True(t, e.Session["Cookie Matches Proposal"].(bool))
			require.Equal(t, uint64(1), e.Session["Receiver Proposal Side"])
			require.Len(t, e.Session["Context Proof"], 6)
			require.Equal(t, byte(0x45), inner[14])
			require.Contains(t, suffix, ":s300~400:")
			e.Raw[0] ^= 1
			e.Session["Ethernet Bytes"].([]byte)[14] = 0
			require.Equal(t, byte(0x45), inner[14], "public fields must not mutate admitted inner bytes")
			e, inner, _ = v.feed(l2tp3TestData(udp, 300, c0, 9), 1, false)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.NotEmpty(t, inner)
			e, inner, _ = v.feed(l2tp3TestData(udp, 400, c0, 10), 0, false)
			require.Equal(t, "context-required", e.Status)
			require.Empty(t, inner)
		})
	}
}

func TestL2TPv3MissingReorderedControlRecoveryAndACK(t *testing.T) {
	for _, udp := range []bool{false, true} {
		t.Run(map[bool]string{false: "IP", true: "UDP"}[udp], func(t *testing.T) {
			v := l2tp3TestParser(t, udp)
			ws := l2tp3TestSetup(udp, 100, 200, 300, 400, nil, nil, 2)
			v.feed(ws[1], 1, false) // reply before request, including a changed responder port
			v.feed(ws[0], 0, true)
			v.feed(ws[3], 0, false) // session request before missing SCCCN
			v.feed(ws[4], 1, false)
			v.feed(ws[5], 0, false)
			e, inner, _ := v.feed(l2tp3TestData(udp, 400, nil, 1), 0, false)
			require.Empty(t, inner)
			require.False(t, e.Session["Payload Decoded"].(bool))
			v.feed(l2tp3TestControl(udp, 200, 1, 2, 20), 0, false)
			v.feed(l2tp3TestControl(udp, 200, 1, 2, 0), 0, false)
			v.feed(ws[2], 0, false) // gap repaired; drain both directions' dependent controls
			e, inner, _ = v.feed(l2tp3TestData(udp, 400, nil, 2), 0, false)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.NotEmpty(t, inner)
			charge := v.a.buffered.Load()
			retrans := bytes.Clone(ws[2])
			at := 0
			if !udp {
				at = 4
			}
			binary.BigEndian.PutUint16(retrans[at+10:], 20)
			for i := 0; i < 100; i++ {
				v.feed(retrans, 0, false)
			}
			require.Equal(t, charge, v.a.buffered.Load(), "changing Nr retransmissions must not grow state or reset the session")
			e, inner, _ = v.feed(l2tp3TestData(udp, 400, nil, 3), 0, false)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.NotEmpty(t, inner)
		})
	}
}

func TestL2TPv3SequenceWrapLossReorderingAndConflict(t *testing.T) {
	v := l2tp3TestParser(t, false)
	v.setup(l2tp3TestSetup(false, 100, 200, 300, 400, nil, nil, 2))
	for _, tt := range []struct {
		seq  uint32
		want string
		gap  uint64
	}{{0xfffffe, "first observed sequence", 0}, {0, "forward", 1}, {0xffffff, "reordered whole frame", 0}, {0, "identical repeated wire frame", 0}, {1, "forward", 0}} {
		e, inner, _ := v.feed(l2tp3TestData(false, 400, nil, tt.seq), 0, false)
		require.Equal(t, "decoded", e.Status, e.Error)
		require.NotEmpty(t, inner)
		require.Equal(t, tt.want, e.Session["Sequence Observation"])
		if tt.gap > 0 {
			require.Equal(t, tt.gap, e.Session["Unobserved Sequence Count"])
		}
	}
	w := l2tp3TestData(false, 400, nil, 1)
	w[len(w)-1] ^= 1
	e, inner, _ := v.feed(w, 0, false)
	require.Equal(t, "context-required", e.Status)
	require.Empty(t, inner)
	e, inner, _ = v.feed(l2tp3TestData(false, 400, nil, 2), 0, false)
	require.Empty(t, inner)
	require.Contains(t, e.Error, "quarantined")
}

func TestL2TPv3ControlConflictCannotInheritSession(t *testing.T) {
	v := l2tp3TestParser(t, false)
	ws := l2tp3TestSetup(false, 100, 200, 300, 400, nil, nil, 0)
	v.setup(ws)
	w := bytes.Clone(ws[0])
	w[len(w)-1] ^= 1
	e, _, _ := v.feed(w, 0, true)
	require.Contains(t, e.Session["Association Diagnostic"], "reused control receiver ID")
	e, inner, _ := v.feed(l2tp3TestData(false, 400, nil, 1), 0, false)
	require.Empty(t, inner)
	require.Contains(t, e.Error, "quarantined")
}

func TestL2TPv3SessionReuseWithDistinctAndAmbiguousCookies(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-cookie", true: "new-cookie"}[different], func(t *testing.T) {
			v := l2tp3TestParser(t, false)
			cookie := []byte{1, 2, 3, 4}
			ws := l2tp3TestSetup(false, 100, 200, 300, 400, cookie, cookie, 0)
			v.setup(ws)
			_, _, oldDomain := v.feed(l2tp3TestData(false, 400, cookie, 1), 0, false)
			v.feed(l2tp3TestControl(false, 200, 4, 2, 14, l2tp3TestNumber(1, 1, 2), l2tp3TestNumber(63, 300, 4), l2tp3TestNumber(64, 400, 4)), 0, false)
			if different {
				cookie = []byte{5, 6, 7, 8}
			}
			next := l2tp3TestSetup(false, 100, 200, 300, 400, cookie, cookie, 0)
			for j, ns := range []uint16{5, 2, 6} {
				w := next[3+j]
				binary.BigEndian.PutUint16(w[12:], ns)
				v.feed(w, []int{0, 1, 0}[j], false)
			}
			e, inner, newDomain := v.feed(l2tp3TestData(false, 400, cookie, 1), 0, false)
			if different {
				require.Equal(t, "decoded", e.Status, e.Error)
				require.NotEmpty(t, inner)
				require.NotEqual(t, oldDomain, newDomain)
				e, inner, _ = v.feed(l2tp3TestData(false, 400, []byte{1, 2, 3, 4}, 2), 0, false)
				require.Empty(t, inner)
				require.Contains(t, e.Error, "retired")
			} else {
				require.Empty(t, inner)
				require.Contains(t, e.Error, "indistinguishable")
			}
		})
	}
}

func TestL2TPv3DomainAndPeerIsolation(t *testing.T) {
	v := l2tp3TestParser(t, true)
	v.setup(l2tp3TestSetup(true, 100, 200, 300, 400, nil, nil, 0))
	v.domain.Interface++
	e, inner, _ := v.feed(l2tp3TestData(true, 400, nil, 1), 0, false)
	require.Empty(t, inner)
	require.False(t, e.Session["Payload Decoded"].(bool))
	v.domain.Interface--
	v.a.l2tp3Mu.Lock()
	require.Empty(t, v.a.l2tp3UDPAdmission(v.domain, l2tp3Endpoint{"192.0.2.3", 40000}, l2tp3Endpoint{"192.0.2.2", 50000}, l2tp3TestData(true, 400, nil, 1)))
	v.a.l2tp3Mu.Unlock()
}

func TestL2TPv3ObservationBudgetCannotForgetAndReinherit(t *testing.T) {
	v := l2tp3TestParser(t, false)
	ws := l2tp3TestSetup(false, 100, 200, 300, 400, nil, nil, 0)
	v.setup(ws)
	v.a.config.MaxBufferedBytes = int(v.a.buffered.Load() + 1)
	v.feed(l2tp3TestControl(false, 200, 4, 2, 6), 0, false)
	require.True(t, v.a.l2tp3.disabled)
	e, inner, _ := v.feed(l2tp3TestData(false, 400, nil, 1), 0, false)
	require.Empty(t, inner)
	require.Contains(t, e.Error, "history exhausted")
	require.LessOrEqual(t, v.a.buffered.Load(), int64(v.a.config.MaxBufferedBytes))
}

func FuzzL2TPv3ObservedControlRecovery(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5}, false)
	f.Add([]byte{1, 0, 3, 4, 5, 2}, true)
	f.Add([]byte{0, 1, 3, 4, 5}, false)
	f.Fuzz(func(t *testing.T, order []byte, udp bool) {
		if len(order) > 64 {
			return
		}
		v := l2tp3TestParser(t, udp)
		ws := l2tp3TestSetup(udp, 100, 200, 300, 400, []byte{0, 0, 0, 0}, []byte{1, 2, 3, 4}, 2)
		for _, x := range order {
			i := int(x) % len(ws)
			w := bytes.Clone(ws[i])
			if x&0x80 != 0 {
				w = w[:len(w)/2]
			}
			v.feed(w, []int{0, 1, 0, 0, 1, 0}[i], i == 0)
		}
		e, inner, _ := v.feed(l2tp3TestData(udp, 400, []byte{1, 2, 3, 4}, 1), 0, false)
		if len(inner) > 0 {
			require.Equal(t, true, e.Session["Control Setup Complete"])
			require.Equal(t, true, e.Session["Session Setup Complete"])
			require.Len(t, e.Session["Context Proof"], 6)
		}
		require.LessOrEqual(t, v.a.buffered.Load(), int64(v.a.config.MaxBufferedBytes))
	})
}

func l2tp3TestEthernet(t *testing.T, w []byte, ipProtocol layers.IPProtocol, src, dst string, sport, dport uint16, syn bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Id: 22, Protocol: ipProtocol, SrcIP: net.ParseIP(src), DstIP: net.ParseIP(dst)}
	ls := []gopacket.SerializableLayer{eth, ip}
	if ipProtocol == layers.IPProtocolUDP {
		u := &layers.UDP{SrcPort: layers.UDPPort(sport), DstPort: layers.UDPPort(dport)}
		require.NoError(t, u.SetNetworkLayerForChecksum(ip))
		ls = append(ls, u)
	} else if ipProtocol == layers.IPProtocolTCP {
		u := &layers.TCP{SrcPort: layers.TCPPort(sport), DstPort: layers.TCPPort(dport), Seq: 100, SYN: syn, ACK: !syn, Window: 65535}
		if !syn {
			u.Seq = 101
		}
		require.NoError(t, u.SetNetworkLayerForChecksum(ip))
		ls = append(ls, u)
	}
	ls = append(ls, gopacket.Payload(w))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ls...))
	return bytes.Clone(b.Bytes())
}
func l2tp3TestCarrier(t *testing.T, w []byte, udp bool, dir int, initial bool) []byte {
	src, dst := "192.0.2.1", "192.0.2.2"
	sport, dport := uint16(40000), uint16(50000)
	if initial {
		dport = 1701
	}
	if dir == 1 {
		src, dst = dst, src
		sport, dport = dport, sport
	}
	proto := layers.IPProtocol(115)
	if udp {
		proto = layers.IPProtocolUDP
	}
	return l2tp3TestEthernet(t, w, proto, src, dst, sport, dport, false)
}

func TestL2TPv3OfflineFastPathInnerTransportAndFragmentEvidence(t *testing.T) {
	for _, udp := range []bool{false, true} {
		for _, profile := range []string{"dns", "dns-fragmented", "mqtt"} {
			t.Run(map[bool]string{false: "IP", true: "UDP"}[udp]+"/"+profile, func(t *testing.T) {
				var frames [][]byte
				for i, w := range l2tp3TestSetup(udp, 100, 200, 300, 400, nil, nil, 0) {
					frames = append(frames, l2tp3TestCarrier(t, w, udp, []int{0, 1, 0, 0, 1, 0}[i], i == 0))
				}
				payload := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
				inners := [][]byte{l2tp3TestEthernet(t, payload, layers.IPProtocolUDP, "198.51.100.1", "198.51.100.2", 41000, 53, false)}
				wantProtocol := "dns"
				if profile == "dns-fragmented" {
					inners = icmpFragmentFrames(t, inners[0], 16, false)
					inners[0], inners[1] = inners[1], inners[0]
				}
				if profile == "mqtt" {
					wantProtocol = "mqtt"
					payload = binMQTTConnect
					inners = [][]byte{l2tp3TestEthernet(t, nil, layers.IPProtocolTCP, "198.51.100.1", "198.51.100.2", 41000, 1883, true), l2tp3TestEthernet(t, payload, layers.IPProtocolTCP, "198.51.100.1", "198.51.100.2", 41000, 1883, false)}
				}
				for _, inner := range inners {
					w := l2tp3TestData(udp, 400, nil, 0)
					at := 8
					if udp {
						at = 12
					}
					w = append(w[:at], inner...)
					frames = append(frames, l2tp3TestCarrier(t, w, udp, 0, false))
				}
				capture := icmpFragmentCapture(t, frames)
				for _, observe := range []bool{false, true} {
					seen := 0
					var opts []CaptureOption
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
					}
					es, stats, err := binReplay(t, capture, 1, opts...)
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					require.Zero(t, stats.CallbackPanics)
					if observe {
						require.Equal(t, len(frames), seen, "logical decapsulation must not invent physical frames")
					}
					var child *ProtocolEvent
					parents := map[uint64]*ProtocolEvent{}
					for _, e := range es {
						if e.Protocol == wantProtocol {
							require.Nil(t, child)
							child = e
						}
						if e.Protocol == "l2tp" && e.Session["Control"] == false {
							parents[e.ID] = e
						}
					}
					require.NotNil(t, child)
					require.Equal(t, payload, child.Raw)
					require.Equal(t, "198.51.100.1:41000", child.Source)
					require.Contains(t, child.CarrierLayers, "L2TPv3")
					require.Contains(t, child.Domain.Encapsulation, "/l2tp3:")
					require.NotZero(t, child.SourceBytes.ParentPDU)
					require.NotEmpty(t, child.SourceBytes.ParentPDUs)
					for _, id := range child.SourceBytes.ParentPDUs {
						require.NotNil(t, parents[id])
						require.True(t, parents[id].Session["Payload Decoded"].(bool))
					}
					wantRefs := 1
					if profile == "dns-fragmented" {
						wantRefs = 2
						require.Len(t, child.SourceBytes.ParentPDUs, 2)
					}
					require.Len(t, child.SourceBytes.PacketRefs, wantRefs)
					for _, ref := range child.SourceBytes.PacketRefs {
						require.Equal(t, CaptureDomain{}, ref.Domain, "physical parent domain must not be rewritten to a logical tunnel identity")
						require.GreaterOrEqual(t, ref.Number, uint64(7))
					}
				}
			})
		}
	}
}

func TestL2TPv3NativeInnerNetworkPreservesFullHeaderPath(t *testing.T) {
	v := l2tp3TestParser(t, true)
	v.setup(l2tp3TestSetup(true, 100, 200, 300, 400, nil, nil, 0))
	inner := l2tp3TestEthernet(t, []byte{8, 0, 0, 0, 0, 1, 0, 1}, layers.IPProtocolICMPv4, "198.51.100.1", "198.51.100.2", 0, 0, false)
	w := append(l2tp3TestData(true, 400, nil, 0)[:12], inner...)
	raw := l2tp3TestCarrier(t, w, true, 0, false)
	p := gopacket.NewPacket(raw, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	p.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(20, 0), Length: len(raw), CaptureLength: len(raw)}, captureEvidence{Ref: PacketReference{Number: 20, Domain: v.domain}})
	var es []*ProtocolEvent
	v.a.config.OnEvent = func(e *ProtocolEvent) { es = append(es, e) }
	_, handled := v.a.networkPacket(p)
	require.True(t, handled)
	require.Len(t, es, 2)
	e := es[1]
	require.Equal(t, "icmp", e.Protocol)
	require.Equal(t, "198.51.100.1", e.Source)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "L2TPv3", "Ethernet", "IPv4", "ICMPv4"}, e.Session["Observed Header Layers"])
	require.Equal(t, v.domain, e.SourceBytes.PacketRefs[0].Domain)
	require.Contains(t, e.Domain.Encapsulation, "/l2tp3:")
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, e.Session["Observed Header Layers"], fields["Observed Header Layers"])
}

func TestL2TPv3InnerIPBoundsCannotInventNetworkMessage(t *testing.T) {
	for _, mode := range []string{"zero-length", "missing-tail", "wrong-version"} {
		t.Run(mode, func(t *testing.T) {
			v := l2tp3TestParser(t, true)
			v.setup(l2tp3TestSetup(true, 100, 200, 300, 400, nil, nil, 0))
			inner := l2tp3TestEthernet(t, []byte{8, 0, 0, 0, 0, 1, 0, 1}, layers.IPProtocolICMPv4, "198.51.100.1", "198.51.100.2", 0, 0, false)
			switch mode {
			case "zero-length":
				inner[16], inner[17] = 0, 0
			case "missing-tail":
				binary.BigEndian.PutUint16(inner[16:18], uint16(len(inner)))
			case "wrong-version":
				inner[14] = 0x65
			}
			w := append(l2tp3TestData(true, 400, nil, 0)[:12], inner...)
			raw := l2tp3TestCarrier(t, w, true, 0, false)
			p := gopacket.NewPacket(raw, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			p.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(20, 0), Length: len(raw), CaptureLength: len(raw)}, captureEvidence{Ref: PacketReference{Number: 20, Domain: v.domain}})
			var es []*ProtocolEvent
			v.a.config.OnEvent = func(e *ProtocolEvent) { es = append(es, e) }
			_, handled := v.a.networkPacket(p)
			require.True(t, handled)
			for _, e := range es {
				require.NotEqual(t, "icmp", e.Protocol, "a complete Ethernet envelope does not prove complete valid inner IP")
			}
		})
	}
}
