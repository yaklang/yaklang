package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Derive outer IPv4 fragments from existing authored carrier controls. The
// original inner bytes, ports and independent protocol answers stay unchanged.
func fragmentedTunnelCapture(t *testing.T, capture []byte, reverse bool) []byte {
	t.Helper()
	r, err := pcapgo.NewReader(bytes.NewReader(capture))
	require.NoError(t, err)
	var frames [][]byte
	for {
		raw, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		pieces := icmpFragmentFrames(t, raw, 32, false)
		if reverse {
			pieces[0], pieces[1] = pieces[1], pieces[0]
		}
		frames = append(frames, pieces...)
	}
	return icmpFragmentCapture(t, frames)
}

func testFragmentedTunnelDNS(t *testing.T) {
	for _, profile := range []string{"ipip", "6in4", "geneve"} {
		frame := tunnelAdmissionWire(t, profile, 5013, "198.51.100.1", false, 1, false)
		wire, err := hex.DecodeString("04d2010000010000000000000674756e6e656c04746573740000010001")
		require.NoError(t, err)
		want := map[string]any{"ID": uint16(1234), "Flags": uint16(256), "Response": false, "Opcode": uint16(0), "RCODE": uint16(0), "Truncated": false,
			"Questions": []map[string]any{{"Name": "tunnel.test", "Name Wire": wire[12:25], "Type": uint16(1), "Class": uint16(1), "Class Base": uint16(1), "QU": false}},
			"Answers":   []map[string]any{}, "Authority": []map[string]any{}, "Additional": []map[string]any{}}
		for _, order := range []string{"unfragmented", "ordered", "reverse", "missing"} {
			frames := [][]byte{frame}
			refs := []uint64{1}
			if order != "unfragmented" {
				frames = icmpFragmentFrames(t, frame, 32, false)
				refs = []uint64{1, 2}
			}
			if order == "reverse" {
				frames[0], frames[1] = frames[1], frames[0]
				refs = []uint64{2, 1}
			}
			if order == "missing" {
				frames = frames[:1]
			}
			capture := icmpFragmentCapture(t, frames)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("outer-fragments-dns/%s/%s/w%d/d%t/o%t", profile, order, workers, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, len(frames), seen)
							}
							require.Len(t, es, 1)
							e := es[0]
							if order == "missing" {
								require.Equal(t, "ip", e.Protocol)
								require.Equal(t, "FragmentCaptureEnded", e.ExpertCode)
								require.Zero(t, stats.Messages)
								require.Empty(t, e.Raw)
								require.Zero(t, e.ResponseTo)
								return
							}
							require.Equal(t, "dns", e.Protocol)
							status := "decoded"
							if deferred {
								status = "deferred"
							}
							require.Equal(t, status, e.Status)
							require.Empty(t, e.Error)
							require.Equal(t, wire, e.Raw)
							src, dst := "192.0.2.1:40000", "192.0.2.2:53"
							if profile == "6in4" {
								src, dst = "[2001:db8::1]:40000", "[2001:db8::2]:53"
							}
							require.Equal(t, src, e.Source)
							require.Equal(t, dst, e.Destination)
							domain := "/" + profile + ":198.51.100.1~198.51.100.2"
							if profile == "geneve" {
								domain += ":5013"
							}
							require.Equal(t, CaptureDomain{Encapsulation: domain}, e.Domain)
							require.Len(t, e.SourceBytes.PacketRefs, len(refs))
							for i, n := range refs {
								require.Equal(t, n, e.SourceBytes.PacketRefs[i].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[i].Domain)
							}
							require.Equal(t, time.Unix(int64(99+len(frames)), 0), e.Timestamp)
							require.EqualValues(t, 1, stats.Messages)
							f, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, want, f)
							require.Equal(t, want, e.Session["DNS"])
							require.Zero(t, e.ResponseTo)
							require.Equal(t, e.ID, e.TransactionID)
							require.Zero(t, e.FlowID)
							f["ID"] = uint16(999)
							clear(e.Raw)
							f, err = e.GetFields()
							require.NoError(t, err)
							require.Equal(t, want, f)
						})
					}
				}
			}
		}
	}
}

func testFragmentedTunnelHTTP(t *testing.T) {
	request := "GET /worker HTTP/1.1\r\nHost: tunnel.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"
	for _, profile := range []string{"ipip", "6in4", "gre", "vxlan", "geneve"} {
		original, err := trafficfixture.ReadFile("industrial-link-core/carriers/captures/tunnel-" + profile + "-http-worker-transaction.pcap")
		require.NoError(t, err)
		for _, reverse := range []bool{false, true} {
			capture := fragmentedTunnelCapture(t, original, reverse)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("outer-fragments-http/%s/reverse%t/w%d/d%t/o%t", profile, reverse, workers, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Len(t, es, 2)
							require.Equal(t, request, string(es[0].Raw))
							require.Equal(t, response, string(es[1].Raw))
							domain := "/" + profile + ":198.51.100.11~198.51.100.12"
							if profile == "gre" {
								domain += ":0"
							}
							if profile == "vxlan" || profile == "geneve" {
								domain += ":5013"
							}
							src, dst := "192.0.2.11:41001", "192.0.2.12:80"
							if profile == "6in4" {
								src, dst = "[2001:db8::11]:41001", "[2001:db8::12]:80"
							}
							require.Equal(t, src, es[0].Source)
							require.Equal(t, dst, es[0].Destination)
							require.Equal(t, dst, es[1].Source)
							require.Equal(t, src, es[1].Destination)
							require.NotZero(t, es[0].FlowID)
							require.Equal(t, es[0].FlowID, es[1].FlowID)
							require.Equal(t, es[0].ID, es[1].ResponseTo)
							require.Equal(t, es[0].ID, es[1].TransactionID)
							require.Zero(t, es[0].ResponseTo)
							for _, e := range es {
								require.Equal(t, domain, e.Domain.Encapsulation)
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								for _, ref := range e.SourceBytes.PacketRefs {
									require.Equal(t, e.Domain, ref.Domain)
								}
							}
							f, err := es[1].GetFields()
							require.NoError(t, err)
							assertMVPJSONFields(t, map[string]any{"Message": map[string]any{"HTTP Response": map[string]any{"FirstLine": map[string]any{"Version": "HTTP/1.1", "Status": "200", "Message": "OK"}, "Headers": []string{"Content-Length: 5", ""}, "Body": map[string]any{"Octets": "hello"}}}}, f)
						})
					}
				}
			}
		}
	}
}

// Nest independently complete outer fragments so the depth bound cannot reset
// after each reconstructed carrier. All physical contributors remain required.
func testNestedFragmentTunnelDepth(t *testing.T) {
	for _, depth := range []int{4, 5} {
		frames := [][]byte{tunnelAdmissionWire(t, "ipip", 5013, "198.51.100.1", false, 0, false)}
		for level := 0; level < depth; level++ {
			var next [][]byte
			for i, f := range frames {
				h := []byte{0x45, 0, 0, 0, 0, 0, 0, 0, 64, 4, 0, 0, 198, 51, 100, 1, 198, 51, 100, 2}
				binary.BigEndian.PutUint16(h[2:4], uint16(20+len(f)-14))
				binary.BigEndian.PutUint16(h[4:6], uint16(level*256+i))
				var sum uint32
				for j := 0; j < 20; j += 2 {
					sum += uint32(binary.BigEndian.Uint16(h[j : j+2]))
				}
				for sum>>16 != 0 {
					sum = (sum & 65535) + (sum >> 16)
				}
				binary.BigEndian.PutUint16(h[10:12], ^uint16(sum))
				outer := append(bytes.Clone(f[:14]), h...)
				outer = append(outer, f[14:]...)
				next = append(next, icmpFragmentFrames(t, outer, 8, false)...)
			}
			frames = next
		}
		capture := icmpFragmentCapture(t, frames)
		if depth == 4 {
			for _, limit := range []int{15, 16} {
				t.Run(fmt.Sprintf("nested-contributor-budget/%d", limit), func(t *testing.T) {
					var es []*ProtocolEvent
					a := coreFragmentParser(&es, 1<<20, limit)
					a.budget = DefaultParserBudget()
					a.budget.MaxCollectionElements = limit
					a.config.MaxMessageBytes = 1 << 20
					var complete []gopacket.Packet
					for i, f := range frames {
						p := gopacket.NewPacket(f, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
						ci := gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), Length: len(f), CaptureLength: len(f)}
						ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: uint64(i + 1)}})
						p.Metadata().CaptureInfo = ci
						q, handled := a.networkPacket(p)
						if !handled {
							complete = append(complete, q)
						}
					}
					a.closeFragments()
					require.Zero(t, a.buffered.Load())
					if limit == 15 {
						require.Empty(t, complete)
						require.Len(t, es, 1)
						require.Equal(t, "FragmentRangeBudget", es[0].ExpertCode)
						require.Equal(t, "malformed", es[0].Status)
						require.Empty(t, es[0].Raw)
					} else {
						require.Empty(t, es)
						require.Len(t, complete, 1)
						require.Len(t, evidenceFrom(complete[0].Metadata().CaptureInfo).refs, 16)
					}

				})
			}
		}

		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observe := range []bool{false, true} {
					t.Run(fmt.Sprintf("nested-outer-fragments/depth%d/w%d/d%t/o%t", depth, workers, deferred, observe), func(t *testing.T) {
						var es []*ProtocolEvent
						var stats ProtocolStats
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						seen := 0
						if observe {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
						if observe {
							require.Equal(t, len(frames), seen)
						}
						require.Zero(t, stats.CallbackPanics)
						require.Zero(t, stats.BufferedBytes)
						if depth == 5 {
							require.Len(t, es, 1)
							for _, e := range es {
								require.Equal(t, "ip", e.Protocol)
								require.Equal(t, "EncapsulationDepthOrProfile", e.ExpertCode)
								require.Len(t, e.SourceBytes.PacketRefs, len(frames))
								for i, ref := range e.SourceBytes.PacketRefs {
									require.EqualValues(t, i+1, ref.Number)
									require.Equal(t, e.Domain, ref.Domain)
								}
								require.Empty(t, e.Raw)
								require.Zero(t, e.ResponseTo)
							}
							require.Zero(t, stats.Messages)
							return
						}
						require.Len(t, es, 1)
						e := es[0]
						require.Equal(t, "dns", e.Protocol)
						expectedWire, err := hex.DecodeString("04d2010000010000000000000674756e6e656c04746573740000010001")
						require.NoError(t, err)
						require.Equal(t, expectedWire, e.Raw)
						require.Zero(t, e.ResponseTo)
						require.Equal(t, e.ID, e.TransactionID)
						expectedFields := map[string]any{"ID": uint16(1234), "Flags": uint16(256), "Response": false, "Opcode": uint16(0), "RCODE": uint16(0), "Truncated": false, "Questions": []map[string]any{{"Name": "tunnel.test", "Name Wire": expectedWire[12:25], "Type": uint16(1), "Class": uint16(1), "Class Base": uint16(1), "QU": false}}, "Answers": []map[string]any{}, "Authority": []map[string]any{}, "Additional": []map[string]any{}}
						fields, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, expectedFields, fields)
						require.Equal(t, expectedFields, e.Session["DNS"])

						require.Equal(t, "192.0.2.1:40000", e.Source)
						require.Equal(t, "192.0.2.2:53", e.Destination)
						want := ""
						for i := 0; i < depth; i++ {
							want += "/ipip:198.51.100.1~198.51.100.2"
						}
						require.Equal(t, want, e.Domain.Encapsulation)
						require.Len(t, e.SourceBytes.PacketRefs, len(frames))
						for i, ref := range e.SourceBytes.PacketRefs {
							require.EqualValues(t, i+1, ref.Number)
							require.Equal(t, e.Domain, ref.Domain)
						}
					})
				}
			}
		}
	}
}
