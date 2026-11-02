package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Authored bounded option controls; the original fixed header and inner query
// are complete, while the enclosing datagram has no admissible wire extent.
func ipipOptionsWire(t *testing.T, optionType, optionLength uint8, id uint16, next uint8) []byte {
	t.Helper()
	profile := "ipip"
	if next == 41 {
		profile = "6in4"
	}
	original := tunnelAdmissionWire(t, profile, 1, "198.51.100.1", false, 1, false)
	wire := append(append(bytes.Clone(original[:34]), optionType, optionLength, 0, 0), original[34:]...)
	wire[14], wire[23] = 0x46, next
	binary.BigEndian.PutUint16(wire[18:20], id)
	return zeroLengthFragmentWire(t, wire, 14)
}

func TestIPv4IPIPOptionErrorPreservesLiteralHeaderAndStrictAccounting(t *testing.T) {
	for _, option := range [][2]uint8{{7, 0}, {7, 20}, {148, 1}} {
		for _, incomplete := range []bool{false, true} {
			for _, file := range []bool{false, true} {
				t.Run(fmt.Sprintf("option%d/%d/incomplete%t/file%t", option[0], option[1], incomplete, file), func(t *testing.T) {
					wire := ipipOptionsWire(t, option[0], option[1], 0x1234, 4)
					input := icmpFragmentCapture(t, [][]byte{wire})
					var events []*ProtocolEvent
					var stats TCPReassemblyStats
					var native ProtocolStats
					observedDefault := false
					opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: incomplete}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnProtocolStats(func(s ProtocolStats) { native = s }), WithEveryPacket(func(p gopacket.Packet) {
						ip, ok := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
						require.True(t, ok)
						require.Zero(t, ip.Version, "the original observer must retain the dependency's partial decode")
						require.Zero(t, ip.Protocol)
						require.Zero(t, ip.Id)
						observedDefault = true
					})}
					var err error
					if file {
						path := filepath.Join(t.TempDir(), "options.pcap")
						require.NoError(t, os.WriteFile(path, input, 0600))
						err = ReplayPcapFile(path, opts...)
					} else {
						err = ReplayPcap(bytes.NewReader(input), opts...)
					}
					if incomplete {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.Contains(t, err.Error(), "option")
					}
					require.True(t, observedDefault)
					require.EqualValues(t, 1, stats.DecodeErrors, "the original decoder error must be counted exactly once in either mode")
					require.Zero(t, stats.DeliveredBytes)
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
					require.Equal(t, "malformed", e.Status)
					require.EqualValues(t, 4, e.Session["Observed IP Version"])
					require.EqualValues(t, 4, e.Session["Observed Next Protocol"])
					require.EqualValues(t, 0x1234, e.Session["Observed Fragment ID"])
					require.EqualValues(t, 24, e.Session["Captured Header Bytes"])
					require.Equal(t, false, e.Session["Parent Header Decode Complete"])
					require.Contains(t, e.Session["Parent Header Decode Error"], "option")
					require.Empty(t, e.Raw)
					require.Nil(t, e.SourceBytes.CompletedIn)
					// This is a header diagnostic, not an admitted message boundary.
					require.EqualValues(t, 0, native.Messages)
					require.EqualValues(t, 0, native.Decoded)
					require.EqualValues(t, 1, native.Malformed)
					require.EqualValues(t, 1, stats.CapturedPackets)
					require.EqualValues(t, len(wire), stats.CapturedBytes)
					_, fieldsErr := e.GetFields()
					require.Error(t, fieldsErr, "literal header observations do not authorize a complete rule/PDU")
				})
			}
		}
	}
}

func TestIPv4IPIPOptionErrorDoesNotSelectOtherCarriers(t *testing.T) {
	for _, next := range []uint8{6, 17, 47} {
		t.Run(fmt.Sprint(next), func(t *testing.T) {
			wire := ipipOptionsWire(t, 7, 0, 0x1234, next)
			var events []*ProtocolEvent
			err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
			require.Error(t, err)
			for _, e := range events {
				require.NotEqual(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
				require.NotEqual(t, "decoded", e.Status)
			}
		})
	}
}

func TestIPv4IPIPOptionErrorCannotCreateTCPFlow(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, protocols := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/protocols%t", workers, protocols), func(t *testing.T) {
				wire := ipipOptionsWire(t, 7, 0, 0x1234, 4)
				flows := 0
				var mu sync.Mutex
				var stats TCPReassemblyStats
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ })}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), opts...))
				require.Zero(t, flows)
				require.Zero(t, stats.DeliveredBytes)
				require.EqualValues(t, 1, stats.DecodeErrors)
			})
		}
	}
}

func FuzzIPv4IPIPLiteralHeaderWithOptions(f *testing.F) {
	for _, seed := range []struct {
		typ, length, next uint8
		id                uint16
	}{{7, 0, 4, 0x1234}, {7, 20, 41, 65535}, {1, 0, 4, 0}, {46, 4, 4, 1024}} {
		f.Add(seed.typ, seed.length, seed.id, seed.next)
	}
	f.Fuzz(func(t *testing.T, typ, length uint8, id uint16, next uint8) {
		if next != 4 && next != 41 {
			return
		}
		wire := ipipOptionsWire(t, typ, length, id, next)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(wire), Length: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 17}}})
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 2048, 2)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		a.networkPacket(packet)
		require.Len(t, events, 1)
		e := events[0]
		require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
		require.EqualValues(t, 4, e.Session["Observed IP Version"])
		require.EqualValues(t, next, e.Session["Observed Next Protocol"])
		require.EqualValues(t, id, e.Session["Observed Fragment ID"])
		require.EqualValues(t, 24, e.Session["Captured Header Bytes"])
		require.Empty(t, e.Raw)
		require.Nil(t, e.SourceBytes.CompletedIn)
		require.Empty(t, a.fragments.sets)
		require.Zero(t, e.TCPFlowID)
	})
}
