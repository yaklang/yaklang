package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusPPPoESessionAndLegacyPPP(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-pppoe-session.pcap")
	require.Len(t, frames, 1)
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	icmp := packet.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)
	input := frames[0][14:]
	require.Equal(t, len(input)-6, int(binary.BigEndian.Uint16(input[4:6])))
	for _, padding := range [][]byte{nil, {0, 0, 0, 0, 0xaa, 0xbb}} {
		data := append(append([]byte(nil), input...), padding...)
		node := protocolCorpusRequireBoundedRuleParse(t, data, "pppoe", "PPPoE")
		for field, want := range map[string]uint64{"VersionType": 0x11, "Code": 0, "Session ID": 1, "Length": uint64(len(input) - 6), "Protocol": 0x21} {
			protocolCorpusRequireValue(t, node, field, want)
		}
		protocolCorpusRequireValue(t, node, "Source", []byte(ip.SrcIP))
		protocolCorpusRequireValue(t, node, "Destination", []byte(ip.DstIP))
		protocolCorpusRequireValue(t, node, "Total Length", uint64(ip.Length))
		protocolCorpusRequireValue(t, node, "Identifier", uint64(icmp.Id))
		protocolCorpusRequireValue(t, node, "Sequence Number", uint64(icmp.Seq))
		if len(padding) != 0 {
			protocolCorpusRequireValue(t, node, "Link Padding", padding)
		}
	}
	// A streaming caller has not supplied a frame boundary. Do not allocate
	// an unbounded padding field or consume the following message as padding.
	reader := bytes.NewReader(append(append([]byte(nil), input...), 0xaa, 0xbb))
	_, err := parser.ParseBinary(reader, "pppoe", "PPPoE")
	require.NoError(t, err)
	require.Equal(t, 2, reader.Len())
	// Preserve the original unit fixture as a rejection test: the HDLC bytes
	// ff:03 cannot be a PPPoE Protocol-ID, even though they belong in PPP.
	legacyInvalid := mustHex(t, "110000010004ff03002d")
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(legacyInvalid), "pppoe", "PPPoE")
	require.Error(t, err)
	require.Contains(t, protocolCorpusFailureDiagnostic(err), "invalid uncompressed protocol field")
	standalone := append([]byte{0xff, 0x03}, input[6:]...)
	readerPPP := newProtocolCorpusBoundedReader(standalone)
	legacy, err := parser.ParseBinary(readerPPP, "ppp") // default entry remains HDLC PPP
	require.NoError(t, err)
	require.Zero(t, readerPPP.Len())
	protocolCorpusRequireValue(t, legacy, "Address", uint64(0xff))
	protocolCorpusRequireValue(t, legacy, "Control", uint64(3))
	protocolCorpusRequireValue(t, legacy, "Protocol", uint64(0x21))
	protocolCorpusRequireValue(t, legacy, "Source", []byte(ip.SrcIP))
	for _, cut := range []int{0, 1, 2, 3, 5, 6, 7, len(input) - 1} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), "pppoe", "PPPoE")
		require.Error(t, err, "truncated PPPoE accepted at %d", cut)
	}
}

func TestProtocolCorpusLACPEveryField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-lacp.pcap")
	require.Len(t, frames, 1)
	input := frames[0][14:]
	require.Len(t, input, 110)
	varied := append([]byte(nil), input...)
	// Nonzero identities prevent an all-zero fixture from concealing offsets.
	for _, start := range []int{2, 22} {
		for i := start + 2; i < start+17; i++ {
			varied[i] = byte(i)
		}
	}
	varied[44], varied[45] = 0x12, 0x34
	for _, data := range [][]byte{input, varied} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "link_aggregation", "LACP")
		protocolCorpusRequireValue(t, node, "Subtype", uint64(data[0]))
		protocolCorpusRequireValue(t, node, "Version", uint64(data[1]))
		for index, name := range []string{"Actor", "Partner"} {
			identity := protocolCorpusFindNode(node, name)
			require.NotNil(t, identity)
			b := data[2+index*20 : 22+index*20]
			for field, offset := range map[string]int{"Type": 0, "Length": 1, "State": 16} {
				protocolCorpusRequireValue(t, identity, field, uint64(b[offset]))
			}
			for field, offset := range map[string]int{"System Priority": 2, "Key": 10, "Port Priority": 12, "Port": 14} {
				protocolCorpusRequireValue(t, identity, field, uint64(binary.BigEndian.Uint16(b[offset:offset+2])))
			}
			protocolCorpusRequireValue(t, identity, "System ID", b[4:10])
			protocolCorpusRequireValue(t, identity, "Reserved", b[17:20])
		}
		protocolCorpusRequireValue(t, node, "Collector Type", uint64(data[42]))
		protocolCorpusRequireValue(t, node, "Collector Length", uint64(data[43]))
		protocolCorpusRequireValue(t, node, "Collector Maximum Delay", uint64(binary.BigEndian.Uint16(data[44:46])))
		protocolCorpusRequireValue(t, node, "Collector Reserved", data[46:58])
		protocolCorpusRequireValue(t, node, "Terminator Type", uint64(data[58]))
		protocolCorpusRequireValue(t, node, "Terminator Length", uint64(data[59]))
		protocolCorpusRequireValue(t, node, "Reserved Padding", data[60:])
	}
	for _, offset := range []int{3, 23, 43, 59} {
		invalid := append([]byte(nil), input...)
		invalid[offset]++
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(invalid), "link_aggregation", "LACP")
		require.Error(t, err, "invalid TLV length at %d accepted", offset)
	}
	for _, cut := range []int{0, 1, 2, 21, 41, 57, 59, 109} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), "link_aggregation", "LACP")
		require.Error(t, err, "truncated LACP accepted at %d", cut)
	}
}

func TestProtocolCorpusNetFlowV5EveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-netflow-v5.pcap")
	require.Len(t, frames, 1)
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	input := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	require.Len(t, input, 72)
	// Also exercise repeated reference instances, nonzero sampling bit fields,
	// and records that differ from one another.
	twoRecords := append(append([]byte(nil), input...), input[24:]...)
	binary.BigEndian.PutUint16(twoRecords[2:4], 2)
	binary.BigEndian.PutUint16(twoRecords[22:24], 0x8123)
	for i := 72; i < len(twoRecords); i++ {
		twoRecords[i] = byte(i)
	}
	twoRecords[108], twoRecords[116], twoRecords[117], twoRecords[118], twoRecords[119] = 0, 24, 16, 0, 0
	for _, data := range [][]byte{input, twoRecords} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "application-layer.netflow_v5", "NetFlowV5")
		protocolCorpusRequireValue(t, node, "Version", uint64(5))
		count := int(binary.BigEndian.Uint16(data[2:4]))
		protocolCorpusRequireValue(t, node, "Count", uint64(count))
		for field, offset := range map[string]int{"System Uptime": 4, "Unix Seconds": 8, "Unix Nanoseconds": 12, "Flow Sequence": 16} {
			protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(data[offset:offset+4])))
		}
		protocolCorpusRequireValue(t, node, "Engine Type", uint64(data[20]))
		protocolCorpusRequireValue(t, node, "Engine ID", uint64(data[21]))
		sampling := binary.BigEndian.Uint16(data[22:24])
		protocolCorpusRequireValue(t, node, "Sampling Mode", uint64(sampling>>14))
		protocolCorpusRequireValue(t, node, "Sampling Interval", uint64(sampling&0x3fff))
		records := protocolCorpusNodesNamed(node, "Record")
		require.Len(t, records, count)
		for index, record := range records {
			protocolCorpusNetFlowV5Record(t, record, data[24+48*index:72+48*index])
		}
	}
	for _, count := range []uint16{0, 2, 31, 65535} {
		invalid := append([]byte(nil), input...)
		binary.BigEndian.PutUint16(invalid[2:4], count)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(invalid), "application-layer.netflow_v5", "NetFlowV5")
		require.Error(t, err)
		require.Contains(t, protocolCorpusFailureDiagnostic(err), "record count or message length mismatch")
	}
	for _, cut := range []int{0, 1, 3, 23, 24, 71} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), "application-layer.netflow_v5", "NetFlowV5")
		require.Error(t, err, "truncated NetFlow accepted at %d", cut)
	}
}

func protocolCorpusNetFlowV5Record(t *testing.T, node *base.Node, b []byte) {
	t.Helper()
	require.Len(t, b, 48)
	for field, offset := range map[string]int{"Source Address": 0, "Destination Address": 4, "Next Hop": 8} {
		protocolCorpusRequireValue(t, node, field, b[offset:offset+4])
	}
	for field, offset := range map[string]int{"Input Interface": 12, "Output Interface": 14, "Source Port": 32, "Destination Port": 34, "Source AS": 40, "Destination AS": 42, "Padding 2": 46} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(b[offset:offset+2])))
	}
	for field, offset := range map[string]int{"Packet Count": 16, "Octet Count": 20, "First Uptime": 24, "Last Uptime": 28} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(b[offset:offset+4])))
	}
	for field, offset := range map[string]int{"Padding 1": 36, "TCP Flags": 37, "Protocol": 38, "Type Of Service": 39, "Source Mask": 44, "Destination Mask": 45} {
		protocolCorpusRequireValue(t, node, field, uint64(b[offset]))
	}
}

func TestProtocolCorpusRIPngEveryRoute(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-ripng.pcap")
	require.Len(t, frames, 1)
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	input := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	require.Equal(t, []byte{1, 1, 0, 0}, input)
	response := append([]byte{2, 1, 0, 0}, make([]byte, 60)...)
	// Next-hop followed by two different routes, including metric 16.
	copy(response[4:20], mustHex(t, "fe800000000000000000000000000001"))
	response[23] = 255
	copy(response[24:40], mustHex(t, "20010db8000100000000000000000000"))
	response[40], response[41], response[42], response[43] = 0x12, 0x34, 64, 1
	copy(response[44:60], mustHex(t, "20010db8000200000000000000000000"))
	response[60], response[61], response[62], response[63] = 0x56, 0x78, 48, 16
	for _, data := range [][]byte{input, response} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "ripng", "RIPng")
		protocolCorpusRequireValue(t, node, "Command", uint64(data[0]))
		protocolCorpusRequireValue(t, node, "Version", uint64(1))
		protocolCorpusRequireValue(t, node, "Reserved", uint64(0))
		entries := protocolCorpusNodesNamed(node, "Entry")
		require.Len(t, entries, (len(data)-4)/20)
		for i, entry := range entries {
			b := data[4+20*i : 24+20*i]
			protocolCorpusRequireValue(t, entry, "Prefix", b[:16])
			protocolCorpusRequireValue(t, entry, "Route Tag", uint64(binary.BigEndian.Uint16(b[16:18])))
			protocolCorpusRequireValue(t, entry, "Prefix Length", uint64(b[18]))
			protocolCorpusRequireValue(t, entry, "Metric", uint64(b[19]))
		}
	}
	for offset, value := range map[int]byte{0: 3, 1: 2, 42: 129, 43: 17} {
		bad := append([]byte(nil), response...)
		bad[offset] = value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "ripng", "RIPng")
		require.Error(t, err)
	}
	for _, cut := range []int{0, 1, 3, 5, 23, 25, 63} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(response[:cut]), "ripng", "RIPng")
		require.Error(t, err, "partial RIPng entry accepted at %d", cut)
	}
}

func TestProtocolCorpusGeneveEveryOptionAndInnerMessage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-geneve.pcap")
	require.Len(t, frames, 1)
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	// Decode UDP independently of support for the application in gopacket.
	input := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	options := []byte{0x12, 0x34, 0x80, 1, 0x11, 0x22, 0x33, 0x44, 0x56, 0x78, 2, 0}
	varied := append(append(append([]byte(nil), input[:8]...), options...), input[8:]...)
	varied[0], varied[1] = 3, 0xc0
	varied[4], varied[5], varied[6] = 0xab, 0xcd, 0xef
	for _, data := range [][]byte{input, varied} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "geneve", "Geneve")
		for field, value := range map[string]uint64{"Version": 0, "Option Length": uint64(data[0] & 63), "OAM": uint64(data[1] >> 7), "Critical Options": uint64(data[1] >> 6 & 1), "Protocol Type": uint64(binary.BigEndian.Uint16(data[2:4])), "VNI": uint64(data[4])<<16 | uint64(data[5])<<8 | uint64(data[6])} {
			protocolCorpusRequireValue(t, node, field, value)
		}
		end := 8 + int(data[0]&63)*4
		parsed := protocolCorpusNodesNamed(node, "Option")
		index := 0
		for start := 8; start < end; {
			require.Less(t, index, len(parsed))
			length := int(data[start+3]&31) * 4
			option := parsed[index]
			protocolCorpusRequireValue(t, option, "Class", uint64(binary.BigEndian.Uint16(data[start:start+2])))
			protocolCorpusRequireValue(t, option, "Type", uint64(data[start+2]))
			protocolCorpusRequireValue(t, option, "Length", uint64(length/4))
			protocolCorpusRequireValue(t, option, "Reserved", uint64(data[start+3]>>5))
			if length > 0 {
				protocolCorpusRequireValue(t, option, "Data", data[start+4:start+4+length])
			}
			start += 4 + length
			index++
		}
		require.Len(t, parsed, index)
		ip := protocolCorpusFindNode(node, "IPv4")
		require.NotNil(t, ip)
		protocolCorpusRequireValue(t, ip, "Source", data[end+12:end+16])
		protocolCorpusRequireValue(t, ip, "Destination", data[end+16:end+20])
		protocolCorpusRequireValue(t, protocolCorpusFindNode(ip, "ICMP"), "Type", uint64(8))
	}
	for offset, value := range map[int]byte{0: 0x40, 1: 0, 11: 31} {
		bad := append([]byte(nil), varied...)
		bad[offset] = value
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "geneve", "Geneve")
		require.Error(t, err)
		require.Contains(t, protocolCorpusFailureDiagnostic(err), "geneve:")
	}
	for _, cut := range []int{0, 1, 7, 8, 11, len(varied) - 1} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(varied[:cut]), "geneve", "Geneve")
		require.Error(t, err, "truncated Geneve accepted at %d", cut)
	}
}
