package bin_parser

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const giopCorpusRule = "application-layer.iiop"

func giopCorpusHash(t *testing.T, input []byte, want string) {
	t.Helper()
	require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256(input)))
}

func giopCorpusWireTree(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	require.Equal(t, wire, NodeToBytes(node))
	_, err := protocolCorpusTerminalCoverage(node, wire)
	require.NoError(t, err)
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		value, err := n.Result()
		if n.Cfg.GetBool(stream_parser.CfgIsList) && len(n.Children) == 0 {
			// The existing formatter omits empty lists; the encoded count
			// and zero-length runtime container still record their presence.
			require.ErrorContains(t, err, "no result")
			require.Zero(t, stream_parser.CalcNodeConsumedLength(n))
			return
		}
		require.NoError(t, err, n.Name)
		require.Same(t, n, value.Origin, n.Name)
		for _, child := range n.Children {
			require.Same(t, n, child.Cfg.GetItem(base.CfgParent), child.Name)
			walk(child)
		}
	}
	walk(node)
}

func giopCorpusField(t *testing.T, node *base.Node, name string, begin, end int, want any) {
	t.Helper()
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field, name)
	require.Equal(t, [2]uint64{uint64(begin) * 8, uint64(end) * 8}, stream_parser.GetNodeResultPos(field), name)
	value, err := field.Result()
	require.NoError(t, err)
	if number, ok := want.(uint64); ok {
		require.Equal(t, number, uintVal(t, value), name)
	} else {
		require.Equal(t, want, value.Value, name)
	}
}

func giopCorpusCapturedMessage(t *testing.T, wire []byte, requestID uint64, reply, profile bool) {
	t.Helper()
	for _, entry := range []string{"GIOP", "GIOPCarrier"} {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, giopCorpusRule, entry)
		giopCorpusWireTree(t, node, wire)
		giopCorpusField(t, node, "Magic", 0, 4, []byte("GIOP"))
		giopCorpusField(t, node, "Major", 4, 5, uint64(1))
		giopCorpusField(t, node, "Minor", 5, 6, uint64(2))
		giopCorpusField(t, node, "Message Size", 8, 12, uint64(len(wire)-12))
		giopCorpusField(t, node, "Request ID", 12, 16, requestID)
		if reply {
			giopCorpusField(t, node, "Message Type", 7, 8, uint64(1))
			giopCorpusField(t, node, "Reply Status", 16, 20, uint64(0))
			giopCorpusField(t, node, "Service Context Count", 20, 24, uint64(0))
			giopCorpusField(t, node, "Stub Data", 24, len(wire), wire[24:])
		} else if profile {
			giopCorpusField(t, node, "Flags", 6, 7, uint64(1))
			giopCorpusField(t, node, "Response Flags", 16, 17, uint64(0))
			giopCorpusField(t, node, "Addr Disc", 20, 22, uint64(1))
			giopCorpusField(t, node, "Profile ID", 24, 28, uint64(3))
			giopCorpusField(t, node, "Profile Data Length", 28, 32, uint64(72))
			giopCorpusField(t, node, "Profile Data", 32, 104, wire[32:104])
			giopCorpusHash(t, wire[32:104], "9d2ec2b0a5ba663618ec51f2f81186991b6c8ff48c481461c1eae9c97186f4f4")
			giopCorpusField(t, node, "Op Len", 104, 108, uint64(20))
			giopCorpusField(t, node, "Operation", 108, 128, "receiveReliableData\x00")
			giopCorpusField(t, node, "Service Context Count", 128, 132, uint64(0))
			giopCorpusField(t, node, "Body Padding", 132, 136, wire[132:136])
			giopCorpusField(t, node, "Stub Data", 136, len(wire), wire[136:])
		} else {
			giopCorpusField(t, node, "Flags", 6, 7, uint64(0))
			giopCorpusField(t, node, "Response Flags", 16, 17, uint64(3))
			giopCorpusField(t, node, "Addr Disc", 20, 22, uint64(0))
			giopCorpusField(t, node, "Key Len", 24, 28, uint64(60))
			giopCorpusField(t, node, "Object Key", 28, 88, string(wire[28:88]))
			giopCorpusField(t, node, "Op Len", 88, 92, uint64(5))
			giopCorpusField(t, node, "Operation", 92, 97, "echo\x00")
			giopCorpusField(t, node, "Service Context Count", 100, 104, uint64(1))
			giopCorpusField(t, node, "Context ID", 104, 108, uint64(7))
			giopCorpusField(t, node, "Context Data Length", 108, 112, uint64(40))
			giopCorpusField(t, node, "Context Data", 112, 152, wire[112:152])
			giopCorpusField(t, node, "Stub Data", 152, len(wire), wire[152:])
		}
		message := protocolCorpusFindNode(node, "Magic").Cfg.GetItem(base.CfgParent).(*base.Node)
		info := message.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, true, info["Body Fields Validated"])
		require.Equal(t, false, info["Fragment Reassembly Performed"])
		require.Equal(t, false, info["Stub Semantics Decoded"])
	}
}

func TestProtocolCorpusGIOPAllOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/ndpi/ndpi-corba.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	giopCorpusHash(t, raw, "a4a0bfa2a212dc460da2a9ca01fa9c1fc562e7d24c7b2fd8d7424e6b85cd3289")
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 28)
	payloads := map[int][]byte{}
	var clientNext, serverNext uint32
	clientBytes, serverBytes, udpCount := 0, 0, 0
	for index, frame := range frames {
		frameNumber := index + 1
		t.Run(fmt.Sprintf("frame-%d", frameNumber), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			require.Zero(t, ip.FragOffset)
			require.Zero(t, ip.Flags&layers.IPv4MoreFragments)
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Equal(t, frame, NodeToBytes(envelope))
			if layer := packet.Layer(layers.LayerTypeTCP); layer != nil {
				tcp := layer.(*layers.TCP)
				payloads[frameNumber] = bytes.Clone(tcp.Payload)
				if len(tcp.Payload) == 0 {
					return
				}
				// Every data segment in each direction is contiguous: no gaps, overlap,
				// retransmissions or hidden bytes are removed during this test reassembly.
				if tcp.SrcPort == 42717 {
					if clientBytes == 0 {
						clientNext = tcp.Seq
					}
					require.Equal(t, clientNext, tcp.Seq)
					clientNext += uint32(len(tcp.Payload))
					clientBytes += len(tcp.Payload)
				} else {
					require.Equal(t, layers.TCPPort(56899), tcp.SrcPort)
					if serverBytes == 0 {
						serverNext = tcp.Seq
					}
					require.Equal(t, serverNext, tcp.Seq)
					serverNext += uint32(len(tcp.Payload))
					serverBytes += len(tcp.Payload)
				}
				require.Equal(t, 66, len(frame)-len(tcp.Payload))
				if frameNumber == 4 || frameNumber == 6 || frameNumber == 12 {
					giopCorpusCapturedMessage(t, tcp.Payload, map[int]uint64{4: 0, 6: 0, 12: 1}[frameNumber], frameNumber != 4, false)
					require.NotNil(t, protocolCorpusFindNode(envelope, "GIOP"))
				} else {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), giopCorpusRule, "GIOP")
					require.Error(t, err, "TCP segments and ZIOP are not complete GIOP PDUs")
					require.Nil(t, protocolCorpusFindNode(envelope, "GIOP"))
					protocolCorpusRequireValue(t, envelope, "Remaining Payload", tcp.Payload)
				}
			} else {
				udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
				require.Len(t, udp.Payload, 260)
				require.Equal(t, "MIOP", string(udp.Payload[:4]))
				require.Equal(t, []byte{0x10, 3, 228, 0}, udp.Payload[4:8])
				require.Equal(t, uint32(0), binary.LittleEndian.Uint32(udp.Payload[8:12]))
				require.Equal(t, uint32(1), binary.LittleEndian.Uint32(udp.Payload[12:16]))
				require.Equal(t, uint32(12), binary.LittleEndian.Uint32(udp.Payload[16:20]))
				require.Equal(t, uint32(udpCount), binary.LittleEndian.Uint32(udp.Payload[20:24]))
				require.Equal(t, make([]byte, 8), udp.Payload[24:32])
				giopCorpusCapturedMessage(t, udp.Payload[32:], uint64(udpCount), false, true)
				udpCount++
			}
		})
	}
	require.Equal(t, 18310, clientBytes)
	require.Equal(t, 4122, serverBytes)
	require.Equal(t, 10, udpCount)
	request := append(bytes.Clone(payloads[8]), payloads[10]...)
	require.Len(t, request, 4164)
	giopCorpusHash(t, request, "51852a51b504a96db05c71bf7c30b2efc517f206dbb89c2fa20f1db78771d438")
	giopCorpusCapturedMessage(t, request, 1, false, false)
	// Formal ZIOP has an original-body length and a length-prefixed compressed
	// sequence. This historical capture instead stores compressed size and raw
	// zlib bytes, which inflate to a complete GIOP message. Do not repair its wire
	// or claim it as a standard ZIOP positive. The explicit bounded transform is
	// a test oracle only, not implicit production reassembly/decompression.
	var legacy []byte
	for _, frame := range []int{14, 15, 17, 18} {
		legacy = append(legacy, payloads[frame]...)
	}
	require.Len(t, legacy, 13918)
	giopCorpusHash(t, legacy, "82e90f1ade4cb620a1e3fb4683ba1b1e7c91ffcc504a9d100a4975cd81f8617c")
	require.Equal(t, "ZIOP", string(legacy[:4]))
	require.Equal(t, uint32(13906), binary.BigEndian.Uint32(legacy[8:12]))
	require.Equal(t, uint16(4), binary.BigEndian.Uint16(legacy[12:14]))
	require.Equal(t, uint32(13898), binary.BigEndian.Uint32(legacy[16:20]))
	require.Equal(t, uint32(2027572701), binary.BigEndian.Uint32(legacy[20:24]))
	require.Greater(t, uint64(binary.BigEndian.Uint32(legacy[20:24])), uint64(len(legacy)-24))
	compressed := bytes.NewReader(legacy[20:])
	inflater, err := zlib.NewReader(compressed)
	require.NoError(t, err)
	decoded, err := io.ReadAll(io.LimitReader(inflater, 1048577))
	require.NoError(t, err)
	require.NoError(t, inflater.Close())
	require.Zero(t, compressed.Len())
	require.Len(t, decoded, 32164)
	giopCorpusHash(t, decoded, "3dc8b15fdd4e85293dae8fcaf50b8f4d675974ae090bcfa9584486a5b902b90d")
	giopCorpusCapturedMessage(t, decoded, 2, false, false)
}

func TestProtocolCorpusGIOPCompanionAndRetainedNegative(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-giop-valid.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	giopCorpusHash(t, raw, "81895a25a99b7c986a4760681bf112a7e32e4c03401eb9b8b7bdac0672fa3e08")
	expected := []string{
		"47494f500100000300000013000000010000000b4e616d6553657276696365",
		"47494f500101010313000000020000000b0000004e616d6553657276696365",
		"47494f50010200030000001700000003000000000000000b4e616d6553657276696365",
		"47494f50010301031700000004000000000000000b0000004e616d6553657276696365",
		"47494f50010200000000002000000001030000000000000000000000000000065f69735f6100000000000000",
		"47494f50010201010c000000010000000000000000000000",
	}
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 24)
	messages := 0
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		payload := packet.Layer(layers.LayerTypeTCP).(*layers.TCP).Payload
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(envelope))
		if index%4 != 3 {
			require.Empty(t, payload)
			continue
		}
		wire := mustHex(t, expected[messages])
		require.Equal(t, wire, payload)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, giopCorpusRule, "GIOP")
		giopCorpusWireTree(t, node, wire)
		if messages < 4 {
			protocolCorpusRequireValue(t, node, "Object Key", "NameService")
			value, err := protocolCorpusFindNode(node, "Request ID").Result()
			require.NoError(t, err)
			require.Equal(t, uint64(messages+1), uintVal(t, value))
			if messages < 2 {
				require.Nil(t, protocolCorpusFindNode(node, "Addr Disc"))
			} else {
				protocolCorpusRequireValue(t, node, "Addr Disc", uint64(0))
			}
		} else {
			protocolCorpusRequireValue(t, node, "Service Context Count", uint64(0))
		}
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), giopCorpusRule, "GIOP")
			require.Error(t, err)
		}
		messages++
	}
	require.Equal(t, 6, messages)
	originalPath := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-giop.pcap"
	original, err := os.ReadFile(originalPath)
	require.NoError(t, err)
	giopCorpusHash(t, original, "2858246115bdbc1f048f812471bdd9533b97c96cdd264ad223c7d795838a9611")
	originals := protocolCorpusAuditPackets(t, originalPath)
	require.Len(t, originals, 4)
	for index, frame := range originals {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		payload := packet.Layer(layers.LayerTypeTCP).(*layers.TCP).Payload
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(envelope))
		if index < 3 {
			require.Empty(t, payload)
			continue
		}
		require.Equal(t, mustHex(t, "47494f50010000000000000000000000"), payload)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), giopCorpusRule, "GIOP")
		require.ErrorContains(t, err, "giop: declared size does not fit message boundary")
		carrier := protocolCorpusRequireBoundedRuleParse(t, payload, giopCorpusRule, "GIOPCarrier")
		protocolCorpusRequireValue(t, carrier, "GIOP Payload", payload)
		require.Nil(t, protocolCorpusFindNode(carrier, "Magic"))
	}
}
