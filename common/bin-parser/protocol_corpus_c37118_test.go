package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Layout cross-checks: GPA's publisher-maintained C37.118 implementation,
// CommonFrameHeader.cs, ConfigurationFrame1.cs, ConfigurationCell.cs and
// PhasorValueBase.cs (raw integer versus engineering-unit distinction).
// https://github.com/GridProtectionAlliance/gsf/tree/master/Source/Libraries/GSF.PhasorProtocols/IEEEC37_118
// The two original CFG-2 records define every captured data field. The test
// selects context by directed transport endpoints, not a global device ID.
type c37118Record struct {
	number      int
	frame, wire []byte
	flow        string
	tcp         bool
}

func c37118Records(t *testing.T) []c37118Record {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-c37118.pcap")
	require.Len(t, frames, 778)
	var records []c37118Record
	controls := 0
	counts := map[string]int{}
	for i, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer(), "frame %d", i+1)
		ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		require.Equal(t, uint8(5), ip.IHL)
		require.Equal(t, 14+int(ip.Length), len(frame))
		r := c37118Record{number: i + 1, frame: frame}
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			tcp := tcpLayer.(*layers.TCP)
			if len(tcp.Payload) == 0 {
				controls++
				continue
			}
			require.Equal(t, uint8(8), tcp.DataOffset)
			r.tcp, r.wire = true, tcp.Payload
			r.flow = fmt.Sprintf("tcp/%s/%d/%s/%d", ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort)
			require.Equal(t, frame[66:], r.wire)
		} else {
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			r.wire = udp.Payload
			r.flow = fmt.Sprintf("udp/%s/%d/%s/%d", ip.SrcIP, udp.SrcPort, ip.DstIP, udp.DstPort)
			require.Equal(t, int(udp.Length)-8, len(r.wire))
			require.Equal(t, frame[42:], r.wire)
		}
		require.Equal(t, int(binary.BigEndian.Uint16(r.wire[2:])), len(r.wire), "each record is one complete frame")
		counts[fmt.Sprintf("%t/%d/%d", r.tcp, r.wire[1]>>4, len(r.wire))]++
		records = append(records, r)
	}
	require.Equal(t, 161, controls)
	require.Equal(t, map[string]int{"true/4/18": 3, "true/3/134": 1, "true/0/54": 252, "false/4/18": 4, "false/3/374": 1, "false/0/48": 356}, counts)
	return records
}

func c37118Message(t *testing.T, n *base.Node) *stream_parser.C37118Message {
	t.Helper()
	if nested := protocolCorpusFindNode(n, "C37118"); nested != nil {
		n = nested
	}
	info, ok := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, info["CRC Valid"])
	m, ok := info["C37.118 Message"].(*stream_parser.C37118Message)
	require.True(t, ok)
	require.Equal(t, m.BodyDecoded, info["Body Decoded"])
	require.Equal(t, m.ConfigurationRequired, info["Configuration Required"])
	return m
}

func c37118RequireCommon(t *testing.T, n *base.Node, wire []byte) *stream_parser.C37118Message {
	t.Helper()
	if nested := protocolCorpusFindNode(n, "C37118"); nested != nil {
		n = nested
	}
	m := c37118Message(t, n)
	for field, value := range map[string]uint64{"Sync": 0xaa, "Sync Reserved": 0, "Frame Type": uint64(wire[1] >> 4), "Version": 1, "Frame Size": uint64(len(wire)), "ID Code": uint64(binary.BigEndian.Uint16(wire[4:])), "Second Of Century": uint64(binary.BigEndian.Uint32(wire[6:])), "Time Quality": uint64(wire[10]), "Fraction Of Second": uint64(binary.BigEndian.Uint32(wire[10:]) & 0xffffff), "Checksum": uint64(binary.BigEndian.Uint16(wire[len(wire)-2:]))} {
		protocolCorpusRequireValue(t, n, field, value)
	}
	require.Equal(t, wire[1]>>4, m.FrameType)
	require.Equal(t, uint8(1), m.Version)
	require.Equal(t, uint16(len(wire)), m.FrameSize)
	require.Equal(t, binary.BigEndian.Uint16(wire[4:]), m.IDCode)
	require.Equal(t, binary.BigEndian.Uint32(wire[6:]), m.SecondOfCentury)
	require.Equal(t, wire[10], m.TimeQuality)
	require.Equal(t, binary.BigEndian.Uint32(wire[10:])&0xffffff, m.FractionOfSecond)
	require.Equal(t, binary.BigEndian.Uint16(wire[len(wire)-2:]), m.Checksum)
	return m
}

func c37118RequireConfiguration(t *testing.T, m *stream_parser.C37118Message, tcp bool) {
	t.Helper()
	want := stream_parser.C37118Configuration{TimeBase: 1000000, DataRate: 50}
	p := stream_parser.C37118PMUConfiguration{StationName: fmt.Sprintf("%-16s", "PMU1"), IDCode: 61, Format: 7, NominalFrequencyFlags: 1, ConfigCount: 1}
	names := []string{"VA", "VB", "VC"}
	unit := stream_parser.C37118Unit{Type: 0, Scale: 1220}
	if tcp {
		want.TimeBase = 0xffffff
		p.StationName, p.IDCode, p.Format, p.ConfigCount = fmt.Sprintf("%-16s", "Blue PMU"), 241, 6, 89
		names = []string{"V1LPM", "VALPM", "VBLPM", "VCLPM"}
		unit.Scale = 1
	} else {
		labels := make([]string, 16)
		for i := range labels {
			labels[i] = fmt.Sprintf("%-16s", fmt.Sprintf("Dig Channel %d", i+1))
		}
		p.DigitalNames = [][]string{labels}
		p.DigitalUnits = []stream_parser.C37118DigitalUnit{{NormalMask: 0, ValidMask: 0}}
	}
	for _, name := range names {
		p.PhasorNames = append(p.PhasorNames, fmt.Sprintf("%-16s", name))
		p.PhasorUnits = append(p.PhasorUnits, unit)
	}
	want.PMUs = []stream_parser.C37118PMUConfiguration{p}
	require.Equal(t, &want, m.Configuration)
	require.True(t, m.BodyDecoded)
	require.False(t, m.ConfigurationRequired)
}

func c37118RequireData(t *testing.T, m *stream_parser.C37118Message, wire []byte, tcp bool) {
	t.Helper()
	require.True(t, m.BodyDecoded)
	require.False(t, m.ConfigurationRequired)
	require.Empty(t, m.OpaqueBody)
	require.Len(t, m.Data, 1)
	d := m.Data[0]
	id, status, phasors := uint16(61), uint16(0), 3
	if tcp {
		id, status, phasors = 241, 0x0800, 4
	}
	require.Equal(t, id, d.IDCode)
	require.Equal(t, status, d.Status)
	require.Equal(t, !tcp, d.Polar)
	require.Len(t, d.Phasors, phasors)
	// Offsets and widths come from the independently pinned CFG-2 layouts,
	// not the decoder's returned counts, lengths, or any reconstructed frame.
	for i := 0; i < phasors; i++ {
		for j, n := range []stream_parser.C37118Number{d.Phasors[i].First, d.Phasors[i].Second} {
			bits := binary.BigEndian.Uint32(wire[16+i*8+j*4:])
			require.Equal(t, stream_parser.C37118Number{RawBits: bits, Width: 32, FloatingPoint: true, Signed: true, Value: float64(math.Float32frombits(bits))}, n)
		}
	}
	for i, n := range []stream_parser.C37118Number{d.Frequency, d.FrequencyDerivative} {
		bits := binary.BigEndian.Uint16(wire[16+phasors*8+i*2:])
		require.Equal(t, stream_parser.C37118Number{RawBits: uint32(bits), Width: 16, Signed: true, Value: float64(int16(bits))}, n)
	}
	require.Empty(t, d.Analogs)
	if tcp {
		require.Empty(t, d.Digitals)
	} else {
		require.Equal(t, []uint16{binary.BigEndian.Uint16(wire[44:])}, d.Digitals)
	}
}

func TestProtocolCorpusC37118EveryRecord(t *testing.T) {
	configs := map[string][]byte{}
	commands := map[int]uint16{4: 5, 8: 2, 413: 1, 418: 1, 419: 5, 421: 2, 778: 1}
	for _, r := range c37118Records(t) {
		cfg := map[string]any{}
		if r.wire[1]>>4 == 0 {
			require.NotEmpty(t, configs[r.flow])
			cfg["c37118Configuration"] = configs[r.flow]
		}
		n := protocolCorpusRequireBoundedRuleParseWithConfig(t, r.wire, "application-layer.c37118", "C37118", cfg)
		m := c37118RequireCommon(t, n, r.wire)
		switch m.FrameType {
		case 3:
			c37118RequireConfiguration(t, m, r.tcp)
			configs[r.flow] = append([]byte(nil), r.wire...)
		case 4:
			require.Equal(t, commands[r.number], m.Command, "record %d", r.number)
			protocolCorpusRequireValue(t, n, "Command", uint64(commands[r.number]))
			require.True(t, m.BodyDecoded)
		case 0:
			c37118RequireData(t, m, r.wire, r.tcp)
			unknown := protocolCorpusRequireBoundedRuleParse(t, r.wire, "application-layer.c37118", "C37118")
			u := c37118Message(t, unknown)
			require.False(t, u.BodyDecoded)
			require.True(t, u.ConfigurationRequired)
			require.Empty(t, u.Data)
			require.Equal(t, r.wire[14:len(r.wire)-2], u.OpaqueBody)
		}
	}
	require.Len(t, configs, 2)
}

func TestProtocolCorpusC37118EveryShortPrefix(t *testing.T) {
	count := 0
	for _, r := range c37118Records(t) {
		for cut := 0; cut < len(r.wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(r.wire[:cut]), "application-layer.c37118", "C37118")
			require.Error(t, err, "record %d prefix %d", r.number, cut)
			count++
		}
	}
	require.Equal(t, 31330, count)
}

func TestProtocolCorpusC37118PublicDispatchEveryRecord(t *testing.T) {
	controls := 0
	for _, frame := range protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-c37118.pcap") {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		if tcp, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP); ok && len(tcp.Payload) == 0 {
			n := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Nil(t, protocolCorpusFindNode(n, "C37118"))
			controls++
		}
	}
	require.Equal(t, 161, controls)
	configs := map[string][]byte{}
	for _, r := range c37118Records(t) {
		cfg := map[string]any{}
		if r.wire[1]>>4 == 0 {
			require.NotEmpty(t, configs[r.flow])
			cfg["c37118Configuration"] = configs[r.flow]
		}
		n := protocolCorpusRequireBoundedRuleParseWithConfig(t, r.frame, "ethernet", "Ethernet", cfg)
		require.NotNil(t, protocolCorpusFindNode(n, "C37118"), "record %d", r.number)
		m := c37118RequireCommon(t, n, r.wire)
		if m.FrameType == 3 {
			c37118RequireConfiguration(t, m, r.tcp)
			configs[r.flow] = append([]byte(nil), r.wire...)
		} else if m.FrameType == 0 {
			c37118RequireData(t, m, r.wire, r.tcp)
			unknown := protocolCorpusRequireBoundedRuleParse(t, r.frame, "ethernet", "Ethernet")
			u := c37118Message(t, unknown)
			require.True(t, u.ConfigurationRequired)
			require.False(t, u.BodyDecoded)
			require.Equal(t, r.wire[14:len(r.wire)-2], u.OpaqueBody)
		}
	}
	// A caller's wrong association is detectable when its wire identity or
	// layout differs. Endpoints are not present in a C37.118 frame: the caller
	// must select the association; equal IDs alone do not prove matching flows.
	var wrong []byte
	for _, r := range c37118Records(t) {
		if r.tcp && r.wire[1]>>4 == 3 {
			wrong = r.wire
		}
		if !r.tcp && r.wire[1]>>4 == 0 {
			// Even with the envelope ID made equal, the four-phasor layout is
			// incompatible with this three-phasor + digital-word data frame.
			matchedID := append([]byte(nil), wrong...)
			binary.BigEndian.PutUint16(matchedID[4:], 60)
			c37118TestSeal(matchedID)
			for _, tc := range []struct {
				cfg        []byte
				diagnostic string
			}{{wrong, "context"}, {matchedID, "CFG-2"}} {
				config := map[string]any{"c37118Configuration": tc.cfg}
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(r.wire), "application-layer.c37118", config, "C37118")
				require.ErrorContains(t, err, tc.diagnostic)
				// Port-based transport detection uses a transactional candidate;
				// an invalid application frame remains explicitly undecoded bytes.
				n := protocolCorpusRequireBoundedRuleParseWithConfig(t, r.frame, "ethernet", "Ethernet", config)
				require.Nil(t, protocolCorpusFindNode(n, "C37118"))
				protocolCorpusRequireValue(t, n, "Remaining Payload", r.wire)
			}
			break
		}
	}
}

// Independent byte-fold CRC implementation used only for negative variants.
// The reference check value for ASCII "123456789" is 0x29b1.
func c37118TestCRC(data []byte) uint16 {
	crc := uint16(0xffff)
	for _, b := range data {
		crc = (crc >> 8) | (crc << 8)
		crc ^= uint16(b)
		crc ^= (crc & 0xff) >> 4
		crc ^= crc << 12
		crc ^= (crc & 0xff) << 5
	}
	return crc
}

func c37118TestSeal(wire []byte) []byte {
	binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)))
	binary.BigEndian.PutUint16(wire[len(wire)-2:], c37118TestCRC(wire[:len(wire)-2]))
	return wire
}

func TestProtocolCorpusC37118LengthsCRCAndStreamBoundaries(t *testing.T) {
	require.Equal(t, uint16(0x29b1), c37118TestCRC([]byte("123456789")))
	seen := map[string]bool{}
	for _, r := range c37118Records(t) {
		require.Equal(t, binary.BigEndian.Uint16(r.wire[len(r.wire)-2:]), c37118TestCRC(r.wire[:len(r.wire)-2]), "original record %d CRC", r.number)
		key := fmt.Sprintf("%t/%d", r.tcp, r.wire[1]>>4)
		if seen[key] {
			continue
		}
		seen[key] = true
		for cut := 0; cut < len(r.wire); cut++ {
			_, err := parser.ParseBinary(bytes.NewReader(r.wire[:cut]), "application-layer.c37118", "C37118")
			require.Error(t, err, "unbounded prefix %d", cut)
		}
		for pos := range r.wire {
			bad := append([]byte(nil), r.wire...)
			bad[pos] ^= 1
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.c37118", "C37118")
			require.Error(t, err, "changed byte %d", pos)
		}
		for _, delta := range []int{-1, 1} {
			bad := append([]byte(nil), r.wire...)
			binary.BigEndian.PutUint16(bad[2:], uint16(len(bad)+delta))
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.c37118", "C37118")
			require.ErrorContains(t, err, "boundary")
		}
		reader := bytes.NewReader(append(append([]byte(nil), r.wire...), 0xde, 0xad))
		_, err := parser.ParseBinary(reader, "application-layer.c37118", "C37118")
		require.NoError(t, err)
		require.Equal(t, 2, reader.Len())
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(append(append([]byte(nil), r.wire...), 0)), "application-layer.c37118", "C37118")
		require.ErrorContains(t, err, "boundary")
		if r.wire[1]>>4 == 3 {
			// PMU count plus each PHNMR/ANNMR/DGNMR count in these one-PMU CFGs.
			for _, pos := range []int{18, 40, 42, 44} {
				for _, count := range []uint16{0, 2, 65535} {
					if count == binary.BigEndian.Uint16(r.wire[pos:]) {
						continue
					}
					bad := append([]byte(nil), r.wire...)
					binary.BigEndian.PutUint16(bad[pos:], count)
					c37118TestSeal(bad)
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.c37118", "C37118")
					require.Error(t, err, "count field %d value %d", pos, count)
				}
			}
		}
	}
}

func TestProtocolCorpusC37118ContextAndParallelIsolation(t *testing.T) {
	var tcpCfg, udpCfg, tcpData, udpData []byte
	for _, r := range c37118Records(t) {
		if r.wire[1]>>4 == 3 {
			if r.tcp {
				tcpCfg = r.wire
			} else {
				udpCfg = r.wire
			}
		}
		if r.wire[1]>>4 == 0 {
			if r.tcp {
				tcpData = r.wire
			} else {
				udpData = r.wire
			}
		}
	}
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprintf("worker-%d", i), func(t *testing.T) {
			t.Parallel()
			for run := 0; run < 8; run++ {
				for _, tc := range []struct {
					wire, cfg, wrong []byte
					tcp              bool
				}{{tcpData, tcpCfg, udpCfg, true}, {udpData, udpCfg, tcpCfg, false}} {
					cfg := map[string]any{"c37118Configuration": tc.cfg}
					n := protocolCorpusRequireBoundedRuleParseWithConfig(t, tc.wire, "application-layer.c37118", "C37118", cfg)
					c37118RequireData(t, c37118Message(t, n), tc.wire, tc.tcp)
					_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(tc.wire), "application-layer.c37118", map[string]any{"c37118Configuration": tc.wrong}, "C37118")
					require.ErrorContains(t, err, "context")
					unknown := protocolCorpusRequireBoundedRuleParse(t, tc.wire, "application-layer.c37118", "C37118")
					require.True(t, c37118Message(t, unknown).ConfigurationRequired)
				}
			}
		})
	}
}

func TestProtocolCorpusC37118OriginalCommandVector(t *testing.T) {
	wire, err := hex.DecodeString("aa41001200f100000000000000000005d7d0")
	require.NoError(t, err)
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.c37118", "C37118")
	m := c37118Message(t, n)
	require.Equal(t, uint16(5), m.Command)
	require.Equal(t, uint16(0xd7d0), m.Checksum)
}
