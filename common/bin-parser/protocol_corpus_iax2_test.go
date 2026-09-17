package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusIAX2Fields(t *testing.T, node *base.Node, wire []byte) (string, int) {
	t.Helper()
	if nested := protocolCorpusFindNode(node, "IAX2"); nested != nil {
		node = nested
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	source := binary.BigEndian.Uint16(wire)
	protocolCorpusRequireValue(t, node, "Source Word", uint64(source))
	require.Equal(t, source&0x8000 != 0, info["Full Frame"])
	require.EqualValues(t, source&0x7fff, info["Source Call"])
	if source&0x8000 == 0 {
		require.NotZero(t, source)
		require.Equal(t, "mini-voice", info["Frame Kind"])
		require.Equal(t, true, info["Media Context Required"])
		protocolCorpusRequireValue(t, node, "Short Timestamp", uint64(binary.BigEndian.Uint16(wire[2:])))
		protocolCorpusRequireValue(t, node, "Frame Data", wire[4:])
		return "mini", 0
	}
	require.Equal(t, "full", info["Frame Kind"])
	destination := binary.BigEndian.Uint16(wire[2:])
	protocolCorpusRequireValue(t, node, "Destination Word", uint64(destination))
	require.EqualValues(t, destination&0x7fff, info["Destination Call"])
	require.Equal(t, destination&0x8000 != 0, info["Retransmission"])
	protocolCorpusRequireValue(t, node, "Timestamp", uint64(binary.BigEndian.Uint32(wire[4:])))
	protocolCorpusRequireValue(t, node, "Outbound Sequence", uint64(wire[8]))
	protocolCorpusRequireValue(t, node, "Inbound Sequence", uint64(wire[9]))
	protocolCorpusRequireValue(t, node, "Frame Type", uint64(wire[10]))
	protocolCorpusRequireValue(t, node, "Subclass", uint64(wire[11]))
	require.False(t, wire[11]&0x80 != 0, "original full frames use uncompressed subclass values")
	require.Equal(t, false, info["Subclass Is Power"])
	require.EqualValues(t, wire[11], info["Subclass Value"])
	if wire[10] != 6 {
		if len(wire) > 12 {
			protocolCorpusRequireValue(t, node, "Frame Data", wire[12:])
		}
		return "full", 0
	}
	var elements []*base.Node
	if list := protocolCorpusFindNode(node, "Information Elements"); list != nil {
		elements = list.Children
	}
	index := 0
	for offset := 12; offset < len(wire); index++ {
		require.LessOrEqual(t, offset+2, len(wire))
		kind, size := wire[offset], int(wire[offset+1])
		require.LessOrEqual(t, offset+2+size, len(wire))
		require.Less(t, index, len(elements))
		child, body := elements[index], wire[offset+2:offset+2+size]
		protocolCorpusRequireValue(t, child, "IE Type", uint64(kind))
		protocolCorpusRequireValue(t, child, "IE Length", uint64(size))
		switch kind {
		case 11, 12:
			require.Len(t, body, 2)
			protocolCorpusRequireValue(t, child, "Unsigned 16", uint64(binary.BigEndian.Uint16(body)))
		case 31, 255:
			require.Len(t, body, 4)
			value := binary.BigEndian.Uint32(body)
			protocolCorpusRequireValue(t, child, "Unsigned 32", uint64(value))
			if kind == 31 {
				year, month, day := int(value>>25)+2000, int(value>>21&15), int(value>>16&31)
				hour, minute, second := int(value>>11&31), int(value>>5&63), int(value&31)*2
				stamp := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
				require.Equal(t, "2005-08-12T10:46:44Z", stamp.Format(time.RFC3339))
				parts := reflect.ValueOf(child.Cfg.GetItem("additionInfo").(map[string]any)["Date Components"])
				require.Equal(t, 6, parts.Len())
				for i, want := range []int{year, month, day, hour, minute, second} {
					require.EqualValues(t, want, parts.Index(i).Interface())
				}
			} else {
				require.Equal(t, uint32(2), value, "original data-format IE is not a voice codec assertion")
			}
		case 1, 2, 4, 10:
			require.Equal(t, string(body), child.Cfg.GetItem("additionInfo").(map[string]any)["Text Value"])
			if len(body) > 0 {
				protocolCorpusRequireValue(t, child, "Text", string(body))
			}
		default:
			t.Fatalf("unexpected captured IE type %d", kind)
		}
		offset += 2 + size
	}
	require.Len(t, elements, index)
	return "full", index
}

func TestProtocolCorpusIAX2EveryCapturedFieldAndBoundary(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-iax2.pcap")
	require.Len(t, frames, 50)
	counts, types, elements := map[string]int{}, map[byte]int{}, 0
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			require.Equal(t, int(udp.Length)-8, len(udp.Payload))
			wire := udp.Payload
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.iax2", "IAX2")
			kind, count := protocolCorpusIAX2Fields(t, node, wire)
			counts[kind]++
			elements += count
			if kind == "full" {
				types[wire[10]]++
			}
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.NotNil(t, protocolCorpusFindNode(envelope, "IAX2"), "UDP port dispatch must reach the application rule")
			protocolCorpusIAX2Fields(t, envelope, wire)
			// Media and IE sequences have an external UDP length, not an
			// invented trailing marker. Require that original boundary for cuts.
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.iax2", map[string]any{"iax2DatagramLength": len(wire)}, "IAX2")
				require.Error(t, err)
			}
			headerSize := 4
			if kind == "full" {
				headerSize = 12
			}
			for cut := 0; cut < headerSize; cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.iax2", "IAX2")
				require.Error(t, err)
			}
		})
	}
	require.Equal(t, map[string]int{"full": 10, "mini": 40}, counts)
	require.Equal(t, map[byte]int{2: 2, 4: 1, 6: 7}, types)
	require.Equal(t, 8, elements)
}

func iax2FullFixture(kind, subclass byte, body ...byte) []byte {
	return append([]byte{0x80, 4, 0, 23, 1, 2, 3, 4, 255, 254, kind, subclass}, body...)
}

func TestProtocolCorpusIAX2AdditionalFrameFormsAndLimits(t *testing.T) {
	for _, subclass := range []byte{0x81, 0x9f, 0xff} {
		node := protocolCorpusRequireBoundedRuleParse(t, iax2FullFixture(2, subclass, 1, 2), "application-layer.iax2", "IAX2")
		info := node.Cfg.GetItem("additionInfo").(map[string]any)
		require.EqualValues(t, subclass&127, info["Subclass Exponent"])
		require.NotContains(t, info, "Subclass Value", "large exponents cannot silently wrap")
	}
	video := []byte{0, 0, 0x80, 23, 0x80, 42, 1, 2, 3}
	node := protocolCorpusRequireBoundedRuleParse(t, video, "application-layer.iax2", "IAX2")
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, "mini-video", info["Frame Kind"])
	require.EqualValues(t, 23, info["Source Call"])
	require.EqualValues(t, 42, info["Video Timestamp"])
	require.Equal(t, true, info["Video Marker"])
	protocolCorpusRequireValue(t, node, "Frame Data", video[6:])
	for _, subclass := range []byte{0x41, 0xc1} {
		videoNode := protocolCorpusRequireBoundedRuleParse(t, iax2FullFixture(3, subclass, 0x12), "application-layer.iax2", "IAX2")
		videoInfo := videoNode.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, true, videoInfo["Video Marker"])
		require.EqualValues(t, 1, videoInfo["Subclass Code"])
		require.Equal(t, subclass&0x80 != 0, videoInfo["Subclass Is Power"])
	}
	for _, timestamped := range []bool{true, false, true} {
		wire := []byte{0, 0, 1, 0, 1, 2, 3, 4}
		if timestamped {
			wire[3] = 1
			wire = append(wire, []byte{0, 2, 0, 4, 0x12, 0x34, 1, 2, 0, 1, 0x80, 23, 0x23, 0x45, 3}...)
		} else {
			wire = append(wire, []byte{0, 4, 0, 2, 1, 2, 0x80, 23, 0, 1, 3}...)
		}
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.iax2", "IAX2", map[string]any{"iax2TrunkTimestamps": !timestamped})
		info := node.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, "trunk", info["Frame Kind"])
		require.Equal(t, timestamped, info["Trunk Timestamps"])
		records := protocolCorpusFindNode(node, "Trunk Records").Children
		require.Len(t, records, 2)
		for i, call := range []uint64{4, 23} {
			recordInfo := records[i].Cfg.GetItem("additionInfo").(map[string]any)
			require.EqualValues(t, call, recordInfo["Source Call"])
			require.Equal(t, i == 1, recordInfo["Retransmission"])
			require.Equal(t, true, recordInfo["Media Context Required"])
			protocolCorpusRequireValue(t, records[i], "Call Word", call+uint64(i)*0x8000)
			protocolCorpusRequireValue(t, records[i], "Data Length", uint64(2-i))
			if timestamped {
				protocolCorpusRequireValue(t, records[i], "Short Timestamp", []uint64{0x1234, 0x2345}[i])
			}
			protocolCorpusRequireValue(t, records[i], "Frame Data", [][]byte{{1, 2}, {3}}[i])
		}
		for cut := 8; cut < len(wire); cut++ {
			// A complete first trunk record is a valid smaller datagram; all
			// other cuts must be rejected by the record's own length fields.
			firstEnd := 14
			if timestamped {
				firstEnd = 16
			}
			if cut == firstEnd {
				continue
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.iax2", "IAX2")
			require.Error(t, err)
		}
	}
	valid := iax2FullFixture(6, 1, 11, 2, 0, 2, 4, 0, 60, 3, 1, 2, 3, 11, 2, 0, 2)
	node = protocolCorpusRequireBoundedRuleParse(t, valid, "application-layer.iax2", "IAX2")
	elems := protocolCorpusFindNode(node, "Information Elements").Children
	require.Len(t, elems, 4, "preserve repeated and zero-length elements")
	require.Equal(t, "", elems[1].Cfg.GetItem("additionInfo").(map[string]any)["Text Value"])
	protocolCorpusRequireValue(t, elems[2], "Value Bytes", []byte{1, 2, 3})
	for i, bad := range [][]byte{
		iax2FullFixture(6, 4, 1, 0), iax2FullFixture(6, 0), iax2FullFixture(6, 0x81), iax2FullFixture(0, 1), iax2FullFixture(13, 1),
		iax2FullFixture(6, 1, 11, 1, 2), iax2FullFixture(6, 1, 11, 2, 0, 1), iax2FullFixture(6, 1, 1, 3, 1, 2), iax2FullFixture(6, 1, 1),
		iax2FullFixture(6, 1, 31, 4, 0, 0, 0, 0), iax2FullFixture(6, 1, 25, 1, 0),
		{0x80, 0, 0, 1, 0, 0, 0, 0, 0, 0, 6, 4}, {0, 0, 0x80, 0, 0, 1},
		{0, 0, 2, 0, 0, 0, 0, 1}, {0, 0, 1, 2, 0, 0, 0, 1}, {0, 0, 1, 0, 0, 0, 0, 1},
		{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0},
	} {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.iax2", "IAX2")
			require.Error(t, err)
		})
	}
	for _, config := range []map[string]any{{"iax2DatagramLimit": 0}, {"iax2DatagramLimit": len(valid) - 1}, {"iax2Encrypted": true}} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(valid), "application-layer.iax2", config, "IAX2")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewReader(valid), "application-layer.iax2", "IAX2")
	require.ErrorContains(t, err, "boundary")
}

func TestProtocolCorpusIAX2InformationElementScalarWidths(t *testing.T) {
	// Wire tags and widths are from RFC 5456 and the pinned dissector, not
	// inferred from whether the parser happens to return a value.
	widths := map[int][]byte{
		1: {23, 38, 39, 42},
		2: {11, 12, 14, 19, 20, 21, 24, 34, 40, 41, 43, 49},
		4: {8, 9, 27, 35, 37, 46, 47, 48, 50, 51, 255},
	}
	for width, kinds := range widths {
		for _, kind := range kinds {
			t.Run(fmt.Sprintf("tag-%d", kind), func(t *testing.T) {
				body := append([]byte{kind, byte(width)}, bytes.Repeat([]byte{0x80}, width)...)
				if kind == 11 {
					body[2], body[3] = 0, 2
				}
				node := protocolCorpusRequireBoundedRuleParse(t, iax2FullFixture(6, 1, body...), "application-layer.iax2", "IAX2")
				var want uint64
				for _, b := range body[2:] {
					want = want<<8 | uint64(b)
				}
				protocolCorpusRequireValue(t, node, fmt.Sprintf("Unsigned %d", width*8), want)
				for _, changed := range []byte{byte(width - 1), byte(width + 1)} {
					invalid := append([]byte{kind, changed}, bytes.Repeat([]byte{0}, int(changed))...)
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(iax2FullFixture(6, 1, invalid...)), "application-layer.iax2", "IAX2")
					require.ErrorContains(t, err, "fixed information-element length")
				}
			})
		}
	}
	for _, kind := range []byte{1, 2, 3, 4, 5, 6, 7, 10, 13, 15, 16, 17, 22, 26, 28, 32, 33, 45, 52} {
		for _, value := range []string{"", "test", "文本"} {
			body := append([]byte{kind, byte(len(value))}, []byte(value)...)
			node := protocolCorpusRequireBoundedRuleParse(t, iax2FullFixture(6, 1, body...), "application-layer.iax2", "IAX2")
			element := protocolCorpusFindNode(node, "Information Elements").Children[0]
			require.Equal(t, value, element.Cfg.GetItem("additionInfo").(map[string]any)["Text Value"])
		}
	}
	for _, date := range []struct {
		year, month, day, hour, minute, second int
		valid                                  bool
	}{
		{2000, 2, 29, 23, 59, 58, true}, {2100, 2, 29, 0, 0, 0, false},
		{2024, 2, 29, 0, 0, 0, true}, {2023, 2, 29, 0, 0, 0, false},
		{2024, 4, 31, 0, 0, 0, false}, {2024, 13, 1, 0, 0, 0, false},
		{2024, 1, 0, 0, 0, 0, false}, {2024, 1, 1, 24, 0, 0, false},
		{2024, 1, 1, 0, 60, 0, false}, {2024, 1, 1, 0, 0, 60, false},
	} {
		packed := uint32(date.year-2000)<<25 | uint32(date.month)<<21 | uint32(date.day)<<16 | uint32(date.hour)<<11 | uint32(date.minute)<<5 | uint32(date.second/2)
		body := make([]byte, 6)
		body[0], body[1] = 31, 4
		binary.BigEndian.PutUint32(body[2:], packed)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(iax2FullFixture(6, 1, body...)), "application-layer.iax2", "IAX2")
		if date.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
