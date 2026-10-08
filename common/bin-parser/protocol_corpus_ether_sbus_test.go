package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func protocolCorpusEtherSBusFields(t *testing.T, node *base.Node, wire, request []byte) *stream_parser.EtherSBusMessage {
	t.Helper()
	if nested := protocolCorpusFindNode(node, "EtherSBus"); nested != nil {
		node = nested
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["CRC Valid"])
	actual := info["Ether-S-Bus Message"].(*stream_parser.EtherSBusMessage)
	want := stream_parser.EtherSBusMessage{
		Length: binary.BigEndian.Uint32(wire), Version: wire[4], Protocol: wire[5],
		Sequence: binary.BigEndian.Uint16(wire[6:]), Attribute: wire[8],
		Checksum: binary.BigEndian.Uint16(wire[len(wire)-2:]), BodyDecoded: true,
	}
	for field, value := range map[string]uint64{
		"Length": uint64(want.Length), "Version": uint64(want.Version), "Protocol": uint64(want.Protocol),
		"Sequence": uint64(want.Sequence), "Attribute": uint64(want.Attribute), "Checksum": uint64(want.Checksum),
	} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	if want.Attribute == 0 {
		want.Destination, want.Command, want.CommandSource = wire[9], wire[10], "wire"
		protocolCorpusRequireValue(t, node, "Destination", uint64(wire[9]))
		protocolCorpusRequireValue(t, node, "Command", uint64(wire[10]))
		switch wire[10] {
		case 0x20:
			want.BodyKind = "version-request"
		case 0x1b:
			want.BodyKind = "status-request"
		case 0x1e, 0x47, 0x51:
			want.EncodedCount = wire[11]
			want.Count = uint16(wire[11]) + 1
			want.Address = uint32(wire[12])<<16 | uint32(wire[13])<<8 | uint32(wire[14])
			want.BodyKind = "word-read-request"
			if wire[10] == 0x47 {
				want.BodyKind = "byte-read-request"
			} else if wire[10] == 0x51 {
				want.BodyKind = "byte-write-request"
				want.Count = uint16(wire[11]) - 2
				want.Bytes = wire[15 : len(wire)-2]
			}
		default:
			t.Fatalf("unexpected original command 0x%x", wire[10])
		}
		if len(wire) > 13 {
			protocolCorpusRequireValue(t, node, "Body", wire[11:len(wire)-2])
		}
	} else {
		require.NotEmpty(t, request)
		require.Equal(t, binary.BigEndian.Uint16(request[6:]), want.Sequence)
		want.Command, want.CommandSource = request[10], "request"
		body := wire[9 : len(wire)-2]
		protocolCorpusRequireValue(t, node, "Body", body)
		if want.Attribute == 2 {
			require.Equal(t, byte(0x51), request[10])
			require.Len(t, body, 2)
			want.BodyKind, want.ACKCode = "ack-nak", binary.BigEndian.Uint16(body)
		} else {
			switch request[10] {
			case 0x20:
				require.Len(t, body, 9)
				want.BodyKind = "version-response"
				want.CPUType, want.FirmwareVersion, want.FirmwareSuffix = string(body[:5]), string(body[5:8]), body[8]
				require.Equal(t, "D2M48", want.CPUType)
				require.Equal(t, "020", want.FirmwareVersion)
			case 0x1b:
				require.Len(t, body, 1)
				want.BodyKind, want.CPUStatus = "status-response", body[0]
				require.Equal(t, byte(0x52), body[0])
			case 0x1e, 0x47:
				want.Count = uint16(request[11]) + 1
				want.Address = uint32(request[12])<<16 | uint32(request[13])<<8 | uint32(request[14])
				if request[10] == 0x47 {
					require.Len(t, body, int(want.Count))
					want.BodyKind, want.Bytes = "byte-read-response", body
				} else {
					require.Len(t, body, int(want.Count)*4)
					want.BodyKind = "word-read-response"
					for pos := 0; pos < len(body); pos += 4 {
						want.Words = append(want.Words, binary.BigEndian.Uint32(body[pos:]))
					}
				}
			default:
				t.Fatalf("unexpected original response command 0x%x", request[10])
			}
		}
	}
	require.Equal(t, want, *actual, "every decoded field must agree with original bytes and its independently selected request")
	return actual
}

func TestProtocolCorpusEtherSBusEveryRecordAndBoundary(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-ethersbus.pcap")
	require.Len(t, frames, 20)
	pending := map[string][]byte{}
	counts, words := map[string]int{}, 0
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			ip, udp := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4), packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			wire := udp.Payload
			require.Equal(t, int(udp.Length)-8, len(wire))
			sequence := binary.BigEndian.Uint16(wire[6:])
			requestKey := fmt.Sprintf("%s/%d/%s/%d/%d", ip.SrcIP, udp.SrcPort, ip.DstIP, udp.DstPort, sequence)
			responseKey := fmt.Sprintf("%s/%d/%s/%d/%d", ip.DstIP, udp.DstPort, ip.SrcIP, udp.SrcPort, sequence)
			config := map[string]any{}
			var request []byte
			if wire[8] == 0 {
				require.NotContains(t, pending, requestKey, "a conflicting reused sequence cannot be silently overwritten")
				pending[requestKey] = append([]byte(nil), wire...)
			} else {
				var ok bool
				request, ok = pending[responseKey]
				require.True(t, ok, "response must match an earlier same-peer request")
				config["etherSBusRequest"] = request
				delete(pending, responseKey)
			}
			node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.ether_sbus", "EtherSBus", config)
			message := protocolCorpusEtherSBusFields(t, node, wire, request)
			counts[message.BodyKind]++
			words += len(message.Words)
			envelope := protocolCorpusRequireBoundedRuleParseWithConfig(t, frame, "ethernet", "Ethernet", config)
			require.NotNil(t, protocolCorpusFindNode(envelope, "EtherSBus"))
			protocolCorpusEtherSBusFields(t, envelope, wire, request)
			if wire[8] == 1 {
				unknown := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ether_sbus", "EtherSBus")
				info := unknown.Cfg.GetItem("additionInfo").(map[string]any)
				require.Equal(t, true, info["Request Context Required"])
				require.Equal(t, false, info["Body Decoded"])
			}
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.ether_sbus", config, "EtherSBus")
				require.Error(t, err, "shorter prefix %d", cut)
			}
			for position := range wire {
				changed := append([]byte(nil), wire...)
				changed[position] ^= 1
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(changed), "application-layer.ether_sbus", config, "EtherSBus")
				require.Error(t, err, "corruption at byte %d", position)
			}
		})
	}
	require.Empty(t, pending)
	require.Equal(t, 24, words)
	require.Equal(t, map[string]int{
		"version-request": 2, "version-response": 2, "status-request": 2, "status-response": 2,
		"word-read-request": 4, "word-read-response": 4, "byte-read-request": 1, "byte-read-response": 1,
		"byte-write-request": 1, "ack-nak": 1,
	}, counts)
}

func TestProtocolCorpusEtherSBusConfigurationBounds(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-ethersbus.pcap")
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	wire := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	for _, limit := range []int{0, -1, len(wire) - 1} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "application-layer.ether_sbus", map[string]any{"etherSBusDatagramLimit": limit}, "EtherSBus")
		require.Error(t, err)
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, frames[0], "ethernet", "Ethernet", map[string]any{"etherSBusDatagramLimit": limit})
		require.Nil(t, protocolCorpusFindNode(node, "EtherSBus"), "imported rule cannot ignore caller limits")
		protocolCorpusRequireValue(t, node, "Remaining Payload", wire)
	}
	_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.ether_sbus", "EtherSBus")
	require.ErrorContains(t, err, "boundary")
}
