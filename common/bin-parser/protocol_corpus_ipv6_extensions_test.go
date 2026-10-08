package bin_parser

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusIPv6EveryExtensionAndChain(t *testing.T) {
	const rule = "internet_protocol_version_6"
	const entry = "Internet Protocol Version 6"
	var hop []byte
	for _, id := range []string{"gen-ipv6-hbh", "gen-ipv6-dstopts", "gen-ipv6-routing", "gen-ipv6-frag"} {
		t.Run(id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/"+id+".pcap")
			require.Len(t, frames, 1)
			input := frames[0][14:]
			if id == "gen-ipv6-hbh" {
				hop = append([]byte(nil), input...)
			}
			varied := append([]byte(nil), input...)
			binary.BigEndian.PutUint32(varied[:4], 0x6abcdef1)
			if id == "gen-ipv6-frag" {
				binary.BigEndian.PutUint16(varied[42:44], 0x1234<<3|1)
				binary.BigEndian.PutUint32(varied[44:48], 0x89abcdef)
			}
			for _, data := range [][]byte{input, varied} {
				node := protocolCorpusRequireBoundedRuleParse(t, data, rule, entry)
				protocolCorpusIPv6Header(t, node, data)
				header := data[40:]
				var name string
				switch data[6] {
				case 0:
					name = "Hop By Hop Options"
				case 60:
					name = "Destination Options"
				case 43:
					name = "Routing Header"
				case 44:
					name = "Fragment Header"
				}
				extension := protocolCorpusFindNode(node, name)
				require.NotNil(t, extension)
				protocolCorpusRequireValue(t, extension, "Next Header", uint64(header[0]))
				switch data[6] {
				case 0, 60:
					protocolCorpusRequireValue(t, extension, "Header Extension Length", uint64(header[1]))
					protocolCorpusIPv6Options(t, extension, header[2:8*(int(header[1])+1)])
				case 43:
					protocolCorpusRequireValue(t, extension, "Header Extension Length", uint64(header[1]))
					protocolCorpusRequireValue(t, extension, "Routing Type", uint64(header[2]))
					protocolCorpusRequireValue(t, extension, "Segments Left", uint64(header[3]))
					protocolCorpusRequireValue(t, extension, "Reserved", uint64(binary.BigEndian.Uint32(header[4:8])))
					addresses := protocolCorpusNodesNamed(extension, "Address")
					require.Len(t, addresses, int(header[1])/2)
					for i, address := range addresses {
						protocolCorpusRequireValue(t, address, "Address", header[8+16*i:24+16*i])
					}
				case 44:
					flags := binary.BigEndian.Uint16(header[2:4])
					protocolCorpusRequireValue(t, extension, "Reserved", uint64(header[1]))
					protocolCorpusRequireValue(t, extension, "Fragment Offset", uint64(flags>>3))
					protocolCorpusRequireValue(t, extension, "Reserved Bits", uint64(flags>>1&3))
					protocolCorpusRequireValue(t, extension, "More Fragments", uint64(flags&1))
					protocolCorpusRequireValue(t, extension, "Identification", uint64(binary.BigEndian.Uint32(header[4:8])))
					protocolCorpusRequireValue(t, node, "Fragment Data", header[8:])
					require.Nil(t, protocolCorpusFindNode(node, "ICMPv6"), "an incomplete fragment was decoded as a whole message")
				}
			}
			// The IPv6 length covers the complete supplied frame, so every cut
			// before its end must be detected, not merely selected header cuts.
			for cut := 0; cut < len(input); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), rule, entry)
				require.Error(t, err, "truncation at %d", cut)
				require.NotContains(t, protocolCorpusFailureDiagnostic(err), "runtime error:")
			}
		})
	}
	require.NotEmpty(t, hop)
	// One packet exercises both repeated option headers, Pad1, nonempty PadN,
	// Router Alert, then an atomic Fragment header and a complete echo.
	options := []byte{60, 1, 0, 1, 1, 0, 5, 2, 0x12, 0x34, 1, 4, 0, 0, 0, 0}
	destination := []byte{44, 0, 1, 4, 0, 0, 0, 0}
	fragment := []byte{58, 0, 0, 0, 0x89, 0xab, 0xcd, 0xef}
	chain := append(append([]byte(nil), hop[:40]...), options...)
	chain = append(chain, destination...)
	chain = append(chain, fragment...)
	chain = append(chain, hop[48:]...)
	binary.BigEndian.PutUint16(chain[4:6], uint16(len(chain)-40))
	node := protocolCorpusRequireBoundedRuleParse(t, chain, rule, entry)
	protocolCorpusIPv6Options(t, protocolCorpusFindNode(node, "Hop By Hop Options"), options[2:])
	protocolCorpusIPv6Options(t, protocolCorpusFindNode(node, "Destination Options"), destination[2:])
	protocolCorpusRequireValue(t, node, "Identification", uint64(0x89abcdef))
	protocolCorpusRequireValue(t, protocolCorpusFindNode(node, "ICMPv6"), "Type", uint64(128))
	require.Nil(t, protocolCorpusFindNode(node, "Fragment Data"))
	for _, offset := range []int{41, 44} {
		bad := append([]byte(nil), chain...)
		bad[offset] = 255
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), rule, entry)
		require.Error(t, err, "invalid option length at %d", offset)
	}
}

func protocolCorpusIPv6Header(t *testing.T, node *base.Node, b []byte) {
	t.Helper()
	bits := binary.BigEndian.Uint32(b[:4])
	for field, value := range map[string]uint64{"Version": uint64(bits >> 28), "Traffic Class": uint64(bits >> 20 & 255), "Flow Label": uint64(bits & 0xfffff), "Payload Length": uint64(binary.BigEndian.Uint16(b[4:6])), "Next Header": uint64(b[6]), "Hop Limit": uint64(b[7])} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	protocolCorpusRequireValue(t, node, "Source", b[8:24])
	protocolCorpusRequireValue(t, node, "Destination", b[24:40])
}

func protocolCorpusIPv6Options(t *testing.T, node *base.Node, b []byte) {
	t.Helper()
	require.NotNil(t, node)
	options := protocolCorpusNodesNamed(node, "Option")
	index := 0
	for len(b) != 0 {
		require.Less(t, index, len(options))
		option := options[index]
		index++
		kind := b[0]
		protocolCorpusRequireValue(t, option, "Type", uint64(kind))
		b = b[1:]
		if kind == 0 {
			continue
		}
		require.NotEmpty(t, b)
		length := int(b[0])
		protocolCorpusRequireValue(t, option, "Length", uint64(length))
		b = b[1:]
		require.GreaterOrEqual(t, len(b), length)
		if kind == 5 {
			require.Equal(t, 2, length)
			protocolCorpusRequireValue(t, option, "Router Alert", uint64(binary.BigEndian.Uint16(b[:2])))
		} else if length > 0 {
			protocolCorpusRequireValue(t, option, "Value", b[:length])
		}
		b = b[length:]
	}
	require.Len(t, options, index)
}

func TestProtocolCorpusIPIPRetainsBothHeaders(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-ipip.pcap")
	require.Len(t, frames, 1)
	data := frames[0][14:]
	node := protocolCorpusRequireBoundedRuleParse(t, data, "internet_protocol", "Internet Protocol")
	protocolCorpusRequireValue(t, node, "Source", data[12:16])
	protocolCorpusRequireValue(t, node, "Destination", data[16:20])
	inner := protocolCorpusFindNode(node, "IPv4")
	require.NotNil(t, inner)
	protocolCorpusRequireValue(t, inner, "Source", data[32:36])
	protocolCorpusRequireValue(t, inner, "Destination", data[36:40])
	protocolCorpusRequireValue(t, inner, "Protocol", uint64(1))
	protocolCorpusRequireValue(t, protocolCorpusFindNode(inner, "ICMP"), "Type", uint64(8))
	for _, cut := range []int{19, 20, 39, len(data) - 1} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data[:cut]), "internet_protocol", "Internet Protocol")
			require.Error(t, err)
		})
	}
}
