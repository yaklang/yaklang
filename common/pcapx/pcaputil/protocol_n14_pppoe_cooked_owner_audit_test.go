package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

// Cooked headers retain only one link address. The missing peer cannot be
// inferred from the capture interface, PPPoE Session ID, inner IP or port.
func n14AuditCookedPPPoE(t *testing.T, ethernet []byte, link layers.LinkType) []byte {
	t.Helper()
	require.GreaterOrEqual(t, len(ethernet), 22)
	var h []byte
	switch link {
	case layers.LinkTypeLinuxSLL:
		h = make([]byte, 16)
		binary.BigEndian.PutUint16(h[2:4], 1)
		binary.BigEndian.PutUint16(h[4:6], 6)
		copy(h[6:12], ethernet[6:12])
		binary.BigEndian.PutUint16(h[14:16], 0x8864)
	case layers.LinkTypeLinuxSLL2:
		h = make([]byte, 20)
		binary.BigEndian.PutUint16(h[:2], 0x8864)
		binary.BigEndian.PutUint32(h[4:8], 1)
		binary.BigEndian.PutUint16(h[8:10], 1)
		h[11] = 6
		copy(h[12:18], ethernet[6:12])
	default:
		t.Fatalf("unexpected cooked link %d", link)
	}
	return append(h, ethernet[14:]...)
}

func n14AuditClassicCapture(t *testing.T, frames [][]byte, link layers.LinkType) []byte {
	t.Helper()
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, link))
	for i, raw := range frames {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*1000000), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	return bytes.Clone(b.Bytes())
}

func TestN14PPPoECookedCaptureHasUnconfirmedOwnerAndNoChildState(t *testing.T) {
	for _, link := range []layers.LinkType{layers.LinkTypeLinuxSLL, layers.LinkTypeLinuxSLL2} {
		for _, tcp := range []bool{false, true} {
			for _, fragmented := range []bool{false, true} {
				for _, workers := range []int{1, 4} {
					for _, protocols := range []bool{false, true} {
						t.Run(fmt.Sprintf("link%d/tcp%t/fragment%t/w%d/protocols%t", link, tcp, fragmented, workers, protocols), func(t *testing.T) {
							prefix, body, _ := n14AuditTransport(t, tcp, 0x5a4b)
							inners := [][]byte{append(prefix, body...)}
							if fragmented {
								inners = n14AuditFragments(t, "destination-after-fragment", tcp, false, 0x11223344, 0x5a4b)
							}
							var frames [][]byte
							for _, inner := range inners {
								frames = append(frames, n14AuditCookedPPPoE(t, n14AuditPPPoE(t, inner, 1, false, 0), link))
							}
							var conf *CaptureConfig
							var events []*ProtocolEvent
							var assembly TCPReassemblyStats
							var analysis ProtocolStats
							var mu sync.Mutex
							observed, flows, delivered := 0, 0, 0
							var udpAllocated bool
							var fragmentSets int
							var input uint64
							opts := []CaptureOption{
								func(c *CaptureConfig) error { conf = c; return nil },
								WithTCPReassemblyWorkers(workers),
								WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}),
								WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ }),
								WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
									mu.Lock()
									defer mu.Unlock()
									delivered += len(f.Payload)
								}),
								WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }),
								WithEveryPacket(func(p gopacket.Packet) {
									mu.Lock()
									defer mu.Unlock()
									require.Equal(t, frames[observed], p.Data())
									observed++
									parser := conf.binParser
									if parser == nil {
										parser = conf.transportNormalizer
									}
									require.NotNil(t, parser)
									parser.udpMu.Lock()
									udpAllocated = udpAllocated || parser.udpSessions != nil
									parser.udpMu.Unlock()
									parser.fragments.mu.Lock()
									fragmentSets += len(parser.fragments.sets)
									parser.fragments.mu.Unlock()
									input += parser.input.Load()
								}),
							}
							if protocols {
								opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { analysis = s }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditClassicCapture(t, frames, link)), opts...))
							require.Equal(t, len(frames), observed)
							require.Zero(t, assembly.DecodeErrors, "missing owner context is not malformed capture bytes")
							require.Zero(t, flows)
							require.Zero(t, delivered)
							require.Zero(t, assembly.DeliveredBytes)
							require.False(t, udpAllocated, "observe state before EOF can clear UDP owners")
							require.Zero(t, fragmentSets, "no fragment ranges, tombstones or clock effects are admitted")
							require.Zero(t, input)
							if !protocols {
								require.Empty(t, events)
								return
							}
							require.Zero(t, analysis.Messages)
							require.Zero(t, analysis.BufferedBytes)
							require.Len(t, events, len(frames))
							for i, e := range events {
								require.Equal(t, "ip", e.Protocol)
								require.Equal(t, "network-encapsulation-refusal", e.Profile)
								require.Equal(t, "PPPoEOwnerUnconfirmed", e.ExpertCode)
								require.Equal(t, "context-required", e.Status)
								require.Equal(t, "headers", e.Completeness)
								require.Contains(t, e.Summary, "PPPoE header observed")
								require.Contains(t, e.Summary, "owner unconfirmed")
								require.Equal(t, uint16(30), e.Session["PPPoE Session ID"])
								require.Equal(t, "unconfirmed", e.Session["PPPoE Session Owner"])
								require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
								require.Zero(t, e.FlowID)
								require.Zero(t, e.TCPFlowID)
								require.Empty(t, e.Raw)
								require.Empty(t, e.SourceBytes.SHA256)
								require.Nil(t, e.SourceBytes.CompletedIn)
								require.Equal(t, []PacketReference{{Number: uint64(i + 1), Domain: e.Domain}}, e.SourceBytes.PacketRefs)
							}
						})
					}
				}
			}
		}
	}
}
