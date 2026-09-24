package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusOSPFv3EveryHelloField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-ospfv3.pcap")
	require.Len(t, frames, 1)
	ip := frames[0][14:]
	input := ip[40:]
	require.Equal(t, byte(89), ip[6])
	pseudo := append(append([]byte(nil), ip[8:40]...), 0, 0, 0, byte(len(input)), 0, 0, 0, 89)
	require.Zero(t, protocolCorpusOnesComplement(append(pseudo, input...)), "independent IPv6 pseudo-header checksum")
	varied := append(append([]byte(nil), input...), 192, 0, 2, 1, 198, 51, 100, 2)
	binary.BigEndian.PutUint16(varied[2:4], uint16(len(varied)))
	for i := 4; i < 36; i++ {
		varied[i] = byte(i*7 + 3)
	}
	for _, data := range [][]byte{input, varied} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "ospfv3", "OSPFv3")
		protocolCorpusOSPFv3Header(t, node, data)
		for field, offset := range map[string]int{"Interface ID": 16} {
			protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(data[offset:offset+4])))
		}
		protocolCorpusRequireValue(t, node, "Router Priority", uint64(data[20]))
		protocolCorpusRequireValue(t, node, "Options", uint64(data[21])<<16|uint64(data[22])<<8|uint64(data[23]))
		for field, offset := range map[string]int{"Hello Interval": 24, "Router Dead Interval": 26} {
			protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(data[offset:offset+2])))
		}
		protocolCorpusRequireValue(t, node, "Designated Router", data[28:32])
		protocolCorpusRequireValue(t, node, "Backup Designated Router", data[32:36])
		neighbors := protocolCorpusNodesNamed(node, "Neighbor")
		require.Len(t, neighbors, (len(data)-36)/4)
		for i, neighbor := range neighbors {
			protocolCorpusRequireValue(t, neighbor, "Neighbor", data[36+4*i:40+4*i])
		}
		// The historical entry must route v3 separately from its v2 layout.
		protocolCorpusRequireBoundedRuleParse(t, data, "ospf", "OSPF")
		for cut := 0; cut < len(data); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data[:cut]), "ospfv3", "OSPFv3")
			require.Error(t, err, "truncated Hello at %d", cut)
		}
	}
	protocolCorpusRequireBoundedRuleParse(t, ip, "internet_protocol_version_6", "Internet Protocol Version 6")
	// A complete but non-integral neighbor entry must also fail.
	bad := append(append([]byte(nil), input...), 1)
	binary.BigEndian.PutUint16(bad[2:4], uint16(len(bad)))
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "ospfv3", "OSPFv3")
	require.Error(t, err)
}

func protocolCorpusOSPFv3Header(t *testing.T, node *base.Node, b []byte) {
	t.Helper()
	header := protocolCorpusFindNode(node, "Header")
	for field, offset := range map[string]int{"Version": 0, "Type": 1, "Instance ID": 14, "Reserved": 15} {
		protocolCorpusRequireValue(t, header, field, uint64(b[offset]))
	}
	for field, offset := range map[string]int{"Packet Length": 2, "Checksum": 12} {
		protocolCorpusRequireValue(t, header, field, uint64(binary.BigEndian.Uint16(b[offset:offset+2])))
	}
	protocolCorpusRequireValue(t, header, "Router ID", b[4:8])
	protocolCorpusRequireValue(t, header, "Area ID", b[8:12])
}

func TestProtocolCorpusOSPFv3OtherPacketEnvelopes(t *testing.T) {
	// Independent RFC 5340 A.3 examples with distinct identifiers and two LSA
	// headers. These exercise framing, not the semantics of an opaque LSA body.
	first := mustHex(t, "01232001112233445566778880000001abcd0018")
	second := mustHex(t, "04562002223344556677889980000002bcde0018")
	headers := append(append([]byte(nil), first...), second...)
	dd := append(mustHex(t, "0000013305dc000712345678"), headers...)
	requests := mustHex(t, "000020011122334455667788000020022233445566778899")
	update := append([]byte{0, 0, 0, 2}, first...)
	update = append(update, 1, 2, 3, 4)
	update = append(update, second...)
	update = append(update, 5, 6, 7, 8)
	for _, tc := range []struct {
		typ     byte
		body    []byte
		entries int
	}{{2, dd, 2}, {3, requests, 2}, {4, update, 2}, {5, headers, 2}} {
		data := append(mustHex(t, "03000000010203040506070812340200"), tc.body...)
		data[1] = tc.typ
		binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
		node := protocolCorpusRequireBoundedRuleParse(t, data, "ospfv3", "OSPFv3")
		protocolCorpusOSPFv3Header(t, node, data)
		if tc.typ == 2 {
			ddNode := protocolCorpusFindNode(node, "Database Description")
			for name, value := range map[string]uint64{"Reserved 1": 0, "Options": 0x133, "Interface MTU": 1500, "Reserved 2": 0, "Flags": 7, "DD Sequence": 0x12345678} {
				protocolCorpusRequireValue(t, ddNode, name, value)
			}
		}
		if tc.typ == 3 {
			records := protocolCorpusNodesNamed(node, "Request")
			require.Len(t, records, tc.entries)
			for i, record := range records {
				b := tc.body[i*12 : (i+1)*12]
				protocolCorpusRequireValue(t, record, "Reserved", uint64(binary.BigEndian.Uint16(b)))
				protocolCorpusRequireValue(t, record, "LS Type", uint64(binary.BigEndian.Uint16(b[2:])))
				protocolCorpusRequireValue(t, record, "Link State ID", b[4:8])
				protocolCorpusRequireValue(t, record, "Advertising Router", b[8:12])
			}
		} else {
			parsed := protocolCorpusNodesNamed(node, "Header")
			require.Len(t, parsed, 3) // one packet header and two LSA headers
			for i, b := range [][]byte{first, second} {
				header := parsed[i+1]
				for name, offset := range map[string]int{"LS Age": 0, "LS Type": 2, "Checksum": 16, "Length": 18} {
					protocolCorpusRequireValue(t, header, name, uint64(binary.BigEndian.Uint16(b[offset:])))
				}
				protocolCorpusRequireValue(t, header, "Link State ID", b[4:8])
				protocolCorpusRequireValue(t, header, "Advertising Router", b[8:12])
				protocolCorpusRequireValue(t, header, "Sequence", uint64(binary.BigEndian.Uint32(b[12:])))
			}
		}
		if tc.typ == 4 {
			protocolCorpusRequireValue(t, node, "LSA Count", uint64(2))
			bodies := protocolCorpusNodesNamed(node, "Body")
			require.Len(t, bodies, 2)
			protocolCorpusRequireValue(t, bodies[0], "Body", []byte{1, 2, 3, 4})
			protocolCorpusRequireValue(t, bodies[1], "Body", []byte{5, 6, 7, 8})
		}
		for cut := 0; cut < len(data); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data[:cut]), "ospfv3", "OSPFv3")
			require.Error(t, err)
		}
		// Claim the extra byte as part of the message: it cannot be hidden as
		// capture padding or silently dropped from a record list.
		bad := append(append([]byte(nil), data...), 0)
		binary.BigEndian.PutUint16(bad[2:4], uint16(len(bad)))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "ospfv3", "OSPFv3")
		require.Error(t, err)
	}
}

func TestProtocolCorpusJ1939EveryIdentifierField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-j1939.pcap")
	require.Len(t, frames, 1)
	for _, id := range []uint32{0x18f00400, 0x0cefab12, 0x1dff35fe} {
		for _, length := range []byte{0, 3, 8} {
			data := bytes.Clone(frames[0])
			binary.BigEndian.PutUint32(data, 0x80000000|id)
			data[4] = length
			for i := 8; i < 16; i++ {
				data[i] = byte(17*i + 3)
			}
			node := protocolCorpusRequireBoundedRuleParse(t, data, "j1939", "J1939")
			for name, value := range map[string]uint64{"Extended Flag": 1, "Remote Flag": 0, "Error Flag": 0, "Priority": uint64(id >> 26), "Reserved Bit": uint64(id >> 25 & 1), "Data Page": uint64(id >> 24 & 1), "PDU Format": uint64(id >> 16 & 255), "PDU Specific": uint64(id >> 8 & 255), "Source Address": uint64(id & 255), "Data Length": uint64(length), "Flags": 0, "Reserved": 0, "Length Code": 0} {
				protocolCorpusRequireValue(t, node, name, value)
			}
			if length > 0 {
				protocolCorpusRequireValue(t, node, "Data", data[8:8+int(length)])
			}
			if length < 8 {
				protocolCorpusRequireValue(t, node, "Padding", data[8+int(length):])
			}
			info := protocolCorpusFindNode(node, "Identifier").Cfg.GetItem("additionInfo").(map[string]any)
			pgn := id >> 8 & 0x3ffff
			if id>>16&255 < 240 {
				pgn &= 0x3ff00
				require.EqualValues(t, id>>8&255, info["Destination Address"])
				require.NotContains(t, info, "Group Extension")
			} else {
				require.EqualValues(t, id>>8&255, info["Group Extension"])
				require.NotContains(t, info, "Destination Address")
			}
			require.EqualValues(t, pgn, info["PGN"])
		}
	}
	// Validate the original bytes as well as the nonzero variants above.
	protocolCorpusRequireBoundedRuleParse(t, frames[0], "j1939", "J1939")
	for cut := 0; cut < len(frames[0]); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frames[0][:cut]), "j1939", "J1939")
		require.Error(t, err)
	}
	for _, mask := range []byte{0x80, 0x40, 0x20} {
		bad := bytes.Clone(frames[0])
		bad[0] ^= mask
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "j1939", "J1939")
		require.Error(t, err)
	}
}
