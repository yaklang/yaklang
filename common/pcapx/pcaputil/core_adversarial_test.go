package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Answers are constructed independently from the RFC-defined wire and reject
// policy. Each configuration must satisfy the answer, not just agree with peers.
func TestCoreAdversarialSealedReplay(t *testing.T) {
	raw, err := trafficfixture.ReadFile("core-adversarial/manifest.json")
	require.NoError(t, err)
	var manifest struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "pr5013-core-adversarial/v1", manifest.Schema)
	require.NotEmpty(t, manifest.Cases)
	for _, c := range manifest.Cases {
		t.Run(c.ID, func(t *testing.T) {
			wire, err := trafficfixture.ReadFile("core-adversarial/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
			answer, err := trafficfixture.ReadFile("core-adversarial/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var want struct {
				Messages      []struct{ Protocol, Raw, Status, Summary string }
				Codes         []string
				ReadError     bool   `json:"read_error"`
				Invalid       uint64 `json:"invalid_segments"`
				DecodeErrors  uint64 `json:"decode_errors"`
				Empty         int    `json:"unrecognized_empty"`
				ErrorContains string `json:"error_contains"`
				Refs          [][]uint64
			}
			require.NoError(t, json.Unmarshal(answer, &want))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", workers, deferred, observer), func(t *testing.T) {
							var mu sync.Mutex
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var observed atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { observed.Add(1) }))
							}
							input := bytes.Clone(wire)
							err := ReplayPcap(bytes.NewReader(input), opts...)
							require.Equal(t, want.ReadError, err != nil, "reader error: %v", err)
							if want.ReadError {
								require.ErrorContains(t, err, want.ErrorContains)
							}
							require.Equal(t, want.Invalid, assembly.InvalidSegments)
							require.Equal(t, want.DecodeErrors, assembly.DecodeErrors)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, assembly.UnreassembledBytes)
							require.Zero(t, assembly.UnreassembledSegments)
							require.Zero(t, stats.CallbackPanics)
							if observer {
								require.EqualValues(t, c.Packets, observed.Load())
							}
							clear(input) // Events and reassembled bytes must own their storage.
							var codes []string
							var messages []*ProtocolEvent
							empty := 0
							for _, e := range events {
								if e.Protocol == "ip" {
									codes = append(codes, e.ExpertCode)
									continue
								}
								if strings.Contains(c.ID, "udp-empty") && e.Status == "unrecognized" && len(e.Raw) == 0 {
									empty++
									continue
								}
								messages = append(messages, e)
							}
							require.Equal(t, want.Empty, empty)
							require.Equal(t, len(want.Codes), len(codes), "codes: %v", codes)
							for i, code := range want.Codes {
								require.Equal(t, code, codes[i])
							}
							require.Len(t, messages, len(want.Messages), "all messages, including any erroneous admission")
							for i, e := range messages {
								require.Equal(t, want.Messages[i].Protocol, e.Protocol)
								require.Equal(t, want.Messages[i].Raw, hex.EncodeToString(e.Raw))
								if want.Messages[i].Status != "" {
									require.Equal(t, want.Messages[i].Status, e.Status)
									require.Equal(t, want.Messages[i].Summary, e.Summary)
									require.Nil(t, e.Structured)
								} else {
									require.Contains(t, []string{"decoded", "deferred"}, e.Status)
									_, err := e.GetFields()
									require.NoError(t, err)
								}
								require.Empty(t, e.Error)
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								var refs []uint64
								for _, ref := range e.SourceBytes.PacketRefs {
									require.Equal(t, e.Domain, ref.Domain)
									refs = append(refs, ref.Number)
								}
								if want.Refs != nil {
									require.Equal(t, want.Refs[i], refs)
								}
								if i > 0 && e.Protocol == "mqtt" {
									require.NotEqual(t, messages[i-1].FlowID, e.FlowID, "closed tuple must create a fresh legitimate flow")
								}
							}
						})
					}
				}
			}
		})
	}
}

func coreIPPackets(t *testing.T, name string) []gopacket.Packet {
	t.Helper()
	raw, err := trafficfixture.ReadFile("core-adversarial/captures/" + name + ".pcap")
	require.NoError(t, err)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	var out []gopacket.Packet
	for {
		b, ci, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		p := gopacket.NewPacket(b, protocolPacketDecoder{decoder: r.LinkType()}, gopacket.Default)
		p.Metadata().CaptureInfo = ci
		out = append(out, p)
	}
	return out
}

func coreFragmentParser(events *[]*ProtocolEvent, limit, collections int) *binParser {
	return &binParser{budget: ParserBudget{MaxCollectionElements: collections}, config: BinParserConfig{MaxBufferedBytes: limit, OnEvent: func(e *ProtocolEvent) { *events = append(*events, e) }}}
}

func TestCoreFragmentHeaderAndLength(t *testing.T) {
	for _, name := range []string{"ipv4-options", "ipv6-preserve-destination-options", "ipv4-total-length-options0-extra0", "ipv4-total-length-options4-extra0", "ipv6-payload-length-extra0"} {
		t.Run(name, func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 128)
			packets := coreIPPackets(t, name)
			_, consumed := a.networkPacket(packets[0])
			require.True(t, consumed)
			q, consumed := a.networkPacket(packets[1])
			require.False(t, consumed)
			require.NotNil(t, q)
			answer, err := trafficfixture.ReadFile("core-adversarial/answers/" + name + ".json")
			require.NoError(t, err)
			var expected struct {
				NetworkSHA string `json:"network_sha256"`
			}
			require.NoError(t, json.Unmarshal(answer, &expected))
			require.NotEmpty(t, expected.NetworkSHA)
			require.Equal(t, expected.NetworkSHA, fmt.Sprintf("%x", sha256.Sum256(q.Data())), "entire independently constructed reassembled IP packet, including checksum and extensions")
			first := packets[0].NetworkLayer().LayerContents()
			switch ip := q.NetworkLayer().(type) {
			case *layers.IPv4:
				require.Equal(t, byte(37), ip.TTL)
				require.Equal(t, byte(0x28), ip.TOS)
				require.Equal(t, first[20:], ip.LayerContents()[20:])
				require.EqualValues(t, len(q.Data()), ip.Length)
				require.Zero(t, ip.FragOffset)
				require.Zero(t, ip.Flags&layers.IPv4MoreFragments)
				if strings.Contains(name, "total-length") {
					require.Equal(t, 65535, len(q.Data()))
				}
			case *layers.IPv6:
				require.Equal(t, byte(37), ip.HopLimit)
				require.Equal(t, uint8(0x2a), ip.TrafficClass)
				require.Equal(t, uint32(0x12345), ip.FlowLabel)
				require.Equal(t, layers.IPProtocolIPv6Destination, ip.NextHeader)
				next := byte(17)
				if strings.Contains(name, "payload-length") {
					next = 253
				}
				require.Equal(t, []byte{next, 0, 1, 4, 0, 0, 0, 0}, q.Data()[40:48])
				require.EqualValues(t, len(q.Data())-40, ip.Length)
				if strings.Contains(name, "payload-length") {
					require.Equal(t, 65535, len(q.Data())-40)
				}
			default:
				t.Fatal("missing reconstructed network header")
			}
			require.Nil(t, q.Layer(layers.LayerTypeIPv6Fragment))
			require.Zero(t, a.buffered.Load())
			require.Empty(t, events)
			a.closeFragments()
			require.Zero(t, a.buffered.Load())
		})
	}
}

func TestCoreFragmentQuarantineBudgetAndDeadline(t *testing.T) {
	for _, limit := range []int{127, 128, 240} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, limit, 2)
			bad := coreIPPackets(t, "ipv6-nonfinal-length")[0]
			good := coreIPPackets(t, "ipv6-in-order")
			for i := 0; i < 200; i++ {
				// Many distinct rejected IDs cannot grow state past either cap.
				raw := bytes.Clone(bad.Data())
				binary.BigEndian.PutUint32(raw[44:48], uint32(i+9))
				p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw}, gopacket.Default)
				p.Metadata().CaptureInfo = bad.Metadata().CaptureInfo
				a.networkPacket(p)
				require.LessOrEqual(t, len(a.fragments.sets), 2)
				require.LessOrEqual(t, a.buffered.Load(), int64(limit))
				require.GreaterOrEqual(t, a.buffered.Load(), int64(0))
			}
			for _, p := range good {
				q, consumed := a.networkPacket(p)
				require.True(t, consumed)
				require.Nil(t, q)
			}
			atomic := coreIPPackets(t, "ipv6-atomic-independent")[1]
			q, consumed := a.networkPacket(atomic)
			require.False(t, consumed)
			require.NotNil(t, q, "atomic remains independent even when quarantine budget is exhausted")
			a.closeFragments()
			require.Zero(t, a.buffered.Load())
			require.Empty(t, a.fragments.sets)
		})
	}
	t.Run("deadline-does-not-extend-and-domain-isolation", func(t *testing.T) {
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 32)
		packets := coreIPPackets(t, "ipv6-overlap-quarantine")
		for _, p := range packets[:4] {
			a.networkPacket(p)
		}
		require.Equal(t, int64(128), a.buffered.Load())
		good := coreIPPackets(t, "ipv6-in-order")
		for _, p := range good {
			ci := p.Metadata().CaptureInfo
			ci.InterfaceIndex = 2
			ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 2}}})
			p.Metadata().CaptureInfo = ci
		}
		a.networkPacket(good[0])
		q, consumed := a.networkPacket(good[1])
		require.False(t, consumed)
		require.NotNil(t, q)
		good = coreIPPackets(t, "ipv6-in-order")
		for _, p := range good {
			ci := p.Metadata().CaptureInfo
			ci.Timestamp = ci.Timestamp.Add(31 * time.Second)
			p.Metadata().CaptureInfo = ci
		}
		a.networkPacket(good[0])
		q, consumed = a.networkPacket(good[1])
		require.False(t, consumed)
		require.NotNil(t, q)
		require.Len(t, events, 1)
		require.Equal(t, "FragmentOverlapRejected", events[0].ExpertCode)
		a.closeFragments()
		require.Zero(t, a.buffered.Load())
	})
}

func TestCoreSessionLifecycleFromSealedWire(t *testing.T) {
	for _, tc := range []struct{ name, transport, protocol string }{{"ipv4-udp-ip-padding", "udp", "dns"}, {"ipv4-tcp-wrap-reorder-duplicate", "tcp", "mqtt"}} {
		t.Run(tc.protocol, func(t *testing.T) {
			var wire []byte
			packets := coreIPPackets(t, tc.name)
			if tc.protocol == "dns" {
				wire = bytes.Clone(packets[0].TransportLayer().LayerPayload())
			} else {
				wire = append(bytes.Clone(packets[3].TransportLayer().LayerPayload()), packets[1].TransportLayer().LayerPayload()...)
			}
			for _, chunk := range []int{1, 7, 64} {
				s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport(tc.transport))
				require.NoError(t, err)
				for i := 0; i < 3; i++ {
					p := s.Probe(wire)
					require.Equal(t, ProbeAccept, p.Verdict)
					require.Equal(t, tc.protocol, p.Protocol)
				}
				require.Zero(t, s.Stats().InputBytes)
				require.Zero(t, s.Stats().Messages)
				require.Zero(t, s.Stats().BufferedBytes)
				for _, dir := range []int{-1, 2, 100} {
					r := s.Feed(dir, time.Unix(1, 0), wire)
					require.Zero(t, r.Consumed)
					require.Empty(t, r.Events)
					require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				}
				var events []*ProtocolEvent
				owned := bytes.Clone(wire)
				if tc.transport == "udp" {
					chunk = len(owned)
				} // Datagram boundaries cannot be arbitrarily resegmented.
				for at := 0; at < len(owned); {
					n := min(chunk, len(owned)-at)
					part := bytes.Clone(owned[at : at+n])
					r := s.Feed(0, time.Unix(2, 0), part)
					require.Equal(t, n, r.Consumed)
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
					events = append(events, r.Events...)
					clear(part)
					at += n
				}
				clear(owned)
				require.Len(t, events, 1)
				require.Equal(t, tc.protocol, events[0].Protocol)
				require.Equal(t, wire, events[0].Raw)
				_, err = events[0].GetFields()
				require.NoError(t, err)
				closeEvents := s.Close("test")
				before := s.Stats()
				require.Zero(t, before.BufferedBytes)
				require.Zero(t, before.Flows)
				require.Empty(t, s.Close("again"))
				require.Equal(t, before, s.Stats())
				r := s.Feed(0, time.Unix(3, 0), wire)
				require.Zero(t, r.Consumed)
				require.Empty(t, r.Events)
				require.Equal(t, "closed", r.State)
				require.Equal(t, ErrFatalSessionError, r.Err.Kind)
				require.Equal(t, before, s.Stats())
				require.Equal(t, wire, events[0].Raw)
				for _, e := range closeEvents {
					require.NotEqual(t, "decoded", e.Status, "Close cannot invent a second complete message")
				}
			}
		})
	}
}

// The capture supplies the referenced request bytes; global event IDs may vary
// with unrelated concurrent flows, but must still resolve to the same request.
func TestCoreLogicalReferenceValidation(t *testing.T) {
	wire, err := trafficfixture.ReadFile("testdata/protocol-sessions/upstream/rtsp.pcap")
	require.NoError(t, err)
	var setup *ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(wire), WithOnProtocolMessage(func(e *ProtocolEvent) {
		if e.Protocol == "rtsp" && bytes.HasPrefix(e.Raw, []byte("SETUP ")) {
			setup = e
		}
	})))
	require.NotNil(t, setup)
	a, b := *setup, *setup
	a.ID, b.ID = 41, 93
	for _, key := range []string{"RTSP SETUP Request Event IDs", "RTSP SETUP Event IDs"} {
		left := smallLogicalReferences(t, map[string]any{key: []uint64{41}}, map[uint64]*ProtocolEvent{41: &a})
		right := smallLogicalReferences(t, map[string]any{key: []uint64{93}}, map[uint64]*ProtocolEvent{93: &b})
		require.Equal(t, left, right, "capture-local allocation order is not request identity")
	}
	for _, id := range []uint64{0, 999} {
		_, err := referencedProtocolPDUs("RTSP SETUP Request Event IDs", []uint64{id}, map[uint64]*ProtocolEvent{41: &a})
		require.Error(t, err, "a missing association must fail even on both sides")
	}
	wrong := a
	wrong.Protocol = "sip"
	_, err = referencedProtocolPDUs("RTSP SETUP Request Event IDs", []uint64{41}, map[uint64]*ProtocolEvent{41: &wrong})
	require.Error(t, err)
	wrong = a
	wrong.Raw = []byte("OPTIONS rtsp://example.test/ RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	_, err = referencedProtocolPDUs("RTSP SETUP Request Event IDs", []uint64{41}, map[uint64]*ProtocolEvent{41: &wrong})
	require.Error(t, err, "a different RTSP method cannot serve as the SETUP request")
	wrong.Raw = append(bytes.Clone(a.Raw), 'x')
	left, err := referencedProtocolPDUs("RTSP SETUP Request Event IDs", []uint64{41}, map[uint64]*ProtocolEvent{41: &a})
	require.NoError(t, err)
	right, err := referencedProtocolPDUs("RTSP SETUP Request Event IDs", []uint64{41}, map[uint64]*ProtocolEvent{41: &wrong})
	require.NoError(t, err)
	require.NotEqual(t, left, right, "a different wire target must never normalize as the same request")

}
