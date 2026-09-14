package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func adversarialWire(seq uint32, flags byte, options, payload []byte) []byte {
	raw := make([]byte, 54+len(options)+len(payload))
	binary.BigEndian.PutUint16(raw[12:14], 0x0800)
	raw[14], raw[22], raw[23] = 0x45, 64, 6
	binary.BigEndian.PutUint16(raw[16:18], uint16(len(raw)-14))
	copy(raw[26:30], []byte{192, 0, 2, 1})
	copy(raw[30:34], []byte{192, 0, 2, 2})
	binary.BigEndian.PutUint16(raw[34:36], 12345)
	binary.BigEndian.PutUint16(raw[36:38], 80)
	binary.BigEndian.PutUint32(raw[38:42], seq)
	raw[46], raw[47] = byte((20+len(options))/4)<<4, flags
	copy(raw[54:], options)
	copy(raw[54+len(options):], payload)
	return raw
}

func encapsulateAdversarial(raw []byte, kind string) ([]byte, layers.LinkType) {
	switch kind {
	case "raw":
		return bytes.Clone(raw[14:]), layers.LinkTypeRaw
	case "vlan":
		out := append(bytes.Clone(raw[:12]), 0x81, 0x00, 0, 42)
		return append(out, raw[12:]...), layers.LinkTypeEthernet
	case "ipv6":
		out := make([]byte, 54)
		copy(out, raw[:14])
		out[12], out[13] = 0x86, 0xdd
		out[14], out[20], out[21] = 0x60, 6, 64
		binary.BigEndian.PutUint16(out[18:20], uint16(len(raw)-34))
		copy(out[22:38], []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
		copy(out[38:54], out[22:38])
		out[53] = 2
		return append(out, raw[34:]...), layers.LinkTypeEthernet
	default:
		return bytes.Clone(raw), layers.LinkTypeEthernet
	}
}

func TestTCPMalformedWireRecovery(t *testing.T) {
	cases := []struct {
		name    string
		options []byte
		mutate  func([]byte) []byte
	}{
		{"offset_below_minimum", nil, func(b []byte) []byte { b[46] = 0x40; return b }},
		{"offset_past_capture", nil, func(b []byte) []byte { b[46] = 0xf0; return b }},
		{"option_zero_length", []byte{2, 0, 0, 0}, nil},
		{"option_length_one", []byte{2, 1, 0, 0}, nil},
		{"option_exceeds_header", []byte{2, 40, 0, 0}, nil},
		{"option_last_byte_without_length", []byte{1, 1, 1, 2}, nil},
		{"mptcp_last_byte", []byte{1, 1, 1, 30}, nil},
		{"mptcp_missing_subtype", []byte{1, 1, 30, 2}, nil},
		{"mptcp_oversized", []byte{30, 255, 0, 0}, nil},
		{"mptcp_truncated_dss", []byte{1, 30, 3, 0x20}, nil},
	}
	for _, kind := range []string{"ethernet", "vlan", "raw", "ipv6"} {
		for _, tc := range cases {
			for _, workers := range []int{1, 4} {
				for _, full := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/workers=%d/full=%v", kind, tc.name, workers, full), func(t *testing.T) {
						bad := adversarialWire(100, 0x10, tc.options, []byte("EVIL"))
						if tc.mutate != nil {
							bad = tc.mutate(bad)
						}
						frames := [][]byte{adversarialWire(99, 2, nil, nil), bad, adversarialWire(100, 0x11, nil, []byte("SAFE"))}
						var link layers.LinkType
						for i := range frames {
							frames[i], link = encapsulateAdversarial(frames[i], kind)
						}
						fixture := classicFixture(binary.LittleEndian, false, 65535, frames...)
						binary.LittleEndian.PutUint32(fixture[20:24], uint32(link))
						file := filepath.Join(t.TempDir(), "malformed.pcap")
						require.NoError(t, os.WriteFile(file, fixture, 0600))
						var got bytes.Buffer
						var stats TCPReassemblyStats
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(13), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) }), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })}
						if full {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
						}
						err := OpenPcapFile(file, opts...)
						require.Equal(t, "SAFE", got.String())
						require.Error(t, err)
						if workers > 1 {
							require.Equal(t, stats.AcceptedPackets, stats.ProcessedPackets)
							require.Positive(t, stats.DecodeErrors)
							require.Zero(t, stats.CallbackPanics)
						}
					})
				}
			}
		}
	}
}

func TestTCPWireCompatibility(t *testing.T) {
	cases := []struct {
		name    string
		options []byte
		mutate  func([]byte)
	}{
		{"unknown_option", []byte{254, 4, 0x12, 0x34}, nil},
		{"nop_padding", []byte{1, 1, 1, 1}, nil},
		{"eol_ignores_padding", []byte{0, 30, 255, 0}, nil},
		{"max_header_options", bytes.Repeat([]byte{1}, 40), nil},
		{"zero_window_probe", nil, func(b []byte) { b[48], b[49] = 0, 0 }},
		{"ecn_urgent_reserved", nil, func(b []byte) { b[47] |= 0xe8; b[46] |= 0x0f; b[52], b[53] = 0xff, 0xff }},
		{"capture_checksum_offload", nil, func(b []byte) { b[50], b[51] = 0x12, 0x34 }},
		{"ipv4_zero_length_tso", nil, func(b []byte) { b[16], b[17] = 0, 0 }},
	}
	for _, tc := range cases {
		for _, workers := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/workers=%d", tc.name, workers), func(t *testing.T) {
				raw := adversarialWire(100, 0x11, tc.options, []byte("SAFE"))
				if tc.mutate != nil {
					tc.mutate(raw)
				}
				file := filepath.Join(t.TempDir(), "compatible.pcap")
				require.NoError(t, os.WriteFile(file, classicFixture(binary.LittleEndian, false, 65535, adversarialWire(99, 2, nil, nil), raw), 0600))
				var got bytes.Buffer
				require.NoError(t, OpenPcapFile(file, WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(13), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })))
				require.Equal(t, "SAFE", got.String())
			})
		}
	}
}

func TestTCPMalformedNetworkRecovery(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		mutate func([]byte) []byte
	}{
		{"ipv4_wrong_version", "ethernet", func(b []byte) []byte { b[14] = 0x65; return b }},
		{"ipv4_short_ihl", "ethernet", func(b []byte) []byte { b[14] = 0x44; return b }},
		{"ipv4_ihl_exceeds_packet", "ethernet", func(b []byte) []byte { b[14] = 0x4f; return b }},
		{"ipv4_total_less_than_header", "ethernet", func(b []byte) []byte { binary.BigEndian.PutUint16(b[16:18], 19); return b }},
		{"ipv4_payload_truncated", "ethernet", func(b []byte) []byte { binary.BigEndian.PutUint16(b[16:18], 200); return b }},
		{"ipv6_wrong_version", "ipv6", func(b []byte) []byte { b[14] = 0x40; return b }},
		{"ipv6_payload_truncated", "ipv6", func(b []byte) []byte { binary.BigEndian.PutUint16(b[18:20], 200); return b }},
		{"ipv6_header_truncated", "ipv6", func(b []byte) []byte { return b[:35] }},
	}
	for _, tc := range cases {
		for _, workers := range []int{1, 4} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/workers=%d/full=%v", tc.name, workers, full), func(t *testing.T) {
					var frames [][]byte
					var link layers.LinkType
					for _, b := range [][]byte{adversarialWire(99, 2, nil, nil), adversarialWire(100, 0x10, nil, []byte("EVIL")), adversarialWire(100, 0x11, nil, []byte("SAFE"))} {
						raw, l := encapsulateAdversarial(b, tc.kind)
						frames = append(frames, raw)
						link = l
					}
					frames[1] = tc.mutate(frames[1])
					fixture := classicFixture(binary.LittleEndian, false, 65535, frames...)
					binary.LittleEndian.PutUint32(fixture[20:24], uint32(link))
					file := filepath.Join(t.TempDir(), "network.pcap")
					require.NoError(t, os.WriteFile(file, fixture, 0600))
					var got bytes.Buffer
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(13), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })}
					if full {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					require.Error(t, OpenPcapFile(file, opts...))
					require.Equal(t, "SAFE", got.String())
				})
			}
		}
	}
}

func TestTCPFragmentsNeverBecomeStreamBytes(t *testing.T) {
	for _, workers := range []int{1, 4} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/full=%v", workers, full), func(t *testing.T) {
				first := adversarialWire(100, 0x10, nil, []byte("EVIL"))
				first[20] = 0x20
				later := bytes.Clone(first)
				later[20] = 0
				later[21] = 3
				file := filepath.Join(t.TempDir(), "fragments.pcap")
				require.NoError(t, os.WriteFile(file, classicFixture(binary.LittleEndian, false, 65535, adversarialWire(99, 2, nil, nil), first, later, adversarialWire(100, 0x11, nil, []byte("SAFE"))), 0600))
				var got bytes.Buffer
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(13), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })}
				if full {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				_ = OpenPcapFile(file, opts...)
				require.Equal(t, "SAFE", got.String())
			})
		}
	}
}

func TestTCPUnrelatedMalformedUDPIsIgnored(t *testing.T) {
	udp := adversarialWire(0, 0, nil, nil)[:45]
	udp[23] = 17
	binary.BigEndian.PutUint16(udp[16:18], 31)
	binary.BigEndian.PutUint16(udp[36:38], 53)
	binary.BigEndian.PutUint16(udp[38:40], 11)
	for _, workers := range []int{1, 4} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/full=%v", workers, full), func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "udp.pcap")
				require.NoError(t, os.WriteFile(file, classicFixture(binary.LittleEndian, false, 65535, udp, adversarialWire(99, 2, nil, nil), adversarialWire(100, 0x11, nil, []byte("SAFE"))), 0600))
				var got bytes.Buffer
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(13), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })}
				if full {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				require.NoError(t, OpenPcapFile(file, opts...))
				require.Equal(t, "SAFE", got.String())
			})
		}
	}
}
