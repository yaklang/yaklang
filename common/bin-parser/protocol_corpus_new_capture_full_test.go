package bin_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusNewPositiveCapturesUseEveryFrame(t *testing.T) {
	t.Run("RADIUS", func(t *testing.T) {
		packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/tcpdump/tcpdump-radius.pcap")
		require.Len(t, packets, 4)
		wantCode := []uint64{1, 11, 1, 2}
		wantID := []uint64{5, 5, 6, 6}
		wantLength := []uint64{139, 109, 174, 97}
		for index, frame := range packets {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
			require.Nilf(t, packet.ErrorLayer(), "frame %d", index+1)
			udpLayer := packet.Layer(layers.LayerTypeUDP)
			require.NotNilf(t, udpLayer, "frame %d", index+1)
			input := append([]byte(nil), udpLayer.(*layers.UDP).Payload...)
			node := protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.radius", "RADIUS")
			protocolCorpusRequireValue(t, node, "Code", wantCode[index])
			protocolCorpusRequireValue(t, node, "Identifier", wantID[index])
			protocolCorpusRequireValue(t, node, "Length", wantLength[index])
		}
	})

	t.Run("LLDP and enclosing frames", func(t *testing.T) {
		packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/tcpdump/tcpdump-lldp.pcap")
		require.Len(t, packets, 12)
		lldpCount := 0
		otherCount := 0
		for index, frame := range packets {
			protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
			require.Nilf(t, packet.ErrorLayer(), "frame %d", index+1)
			ethernetLayer := packet.Layer(layers.LayerTypeEthernet)
			require.NotNilf(t, ethernetLayer, "frame %d", index+1)
			ethernet := ethernetLayer.(*layers.Ethernet)
			if uint16(ethernet.EthernetType) != 0x88cc {
				otherCount++
				continue
			}
			lldpCount++
			input := append([]byte(nil), ethernet.Payload...)
			node := protocolCorpusRequireBoundedRuleParse(t, input, "lldp", "LLDP")
			protocolCorpusRequireValue(t, node, "Chassis ID Subtype", uint64(4))
			protocolCorpusRequireValue(t, node, "TTL", uint64(120))
			wantPortSubtype := uint64(1)
			if index == 3 || index == 5 || index == 9 || index == 11 {
				wantPortSubtype = 7
			}
			protocolCorpusRequireValue(t, node, "Port ID Subtype", wantPortSubtype)
		}
		require.Equal(t, 8, lldpCount)
		require.Equal(t, 4, otherCount)
	})

	t.Run("RadioTap", func(t *testing.T) {
		packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/tcpdump/tcpdump-radiotap.pcap")
		require.Len(t, packets, 26)
		for index, frame := range packets {
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "application-layer.extended_protocols", "RadioTapHeader")
			protocolCorpusRequireValue(t, node, "Version", uint64(0))
			headerLength := protocolCorpusFindNode(node, "Header Length")
			require.NotNilf(t, headerLength, "frame %d", index+1)
			value, err := headerLength.Result()
			require.NoError(t, err)
			length := uintVal(t, value)
			require.Containsf(t, []uint64{83, 89, 93}, length, "frame %d", index+1)
			require.Less(t, length, uint64(len(frame)))
		}
	})
}

func TestProtocolCorpusRadiusBoundaryRejectsEveryFrame(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-radius-classification-boundary.pcapng")
	require.Len(t, packets, 10)
	wantCode := []byte{8, 4, 0, 0, 0, 0, 0, 0, 0, 0}
	wantErrorContains := []string{
		"radius: unknown code",
		"radius: length exceeds 4096",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
		"radius: unknown code",
	}
	for index, frame := range packets {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		require.Nilf(t, packet.ErrorLayer(), "frame %d", index+1)
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		require.NotNilf(t, udpLayer, "frame %d", index+1)
		input := append([]byte(nil), udpLayer.(*layers.UDP).Payload...)
		require.GreaterOrEqualf(t, len(input), 4, "frame %d", index+1)
		require.Equalf(t, wantCode[index], input[0], "frame %d code", index+1)
		if index == 1 {
			require.Equalf(t, uint16(21219), binary.BigEndian.Uint16(input[2:4]), "frame %d declared length", index+1)
		}
		reader := newProtocolCorpusBoundedReader(input)
		_, err := parser.ParseBinary(reader, "application-layer.radius", "RADIUS")
		require.Errorf(t, err, "frame %d was accepted as RADIUS", index+1)
		require.Containsf(t, err.Error(), "parse node RADIUS error", "frame %d failed outside the declared parser entry", index+1)
		require.Containsf(t, err.Error(), "YakVM Panic:", "frame %d was not rejected by a field-value invariant", index+1)
		require.Containsf(t, err.Error(), wantErrorContains[index], "frame %d failed for an unexpected reason", index+1)
	}
}

func protocolCorpusRequireBoundedRuleParse(t *testing.T, input []byte, rule, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParseWithConfig(t, input, rule, entry, nil)
}

func protocolCorpusRequireBoundedRuleParseWithConfig(t *testing.T, input []byte, rule, entry string, config map[string]any) *base.Node {
	t.Helper()
	require.NotEmpty(t, input)
	reader := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinaryWithConfig(reader, rule, config, entry)
	require.NoError(t, err)
	require.NotNil(t, node)
	value, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, value)
	covered, err := protocolCorpusTerminalCoverage(node, input)
	require.NoError(t, err)
	require.Positive(t, covered)
	require.Zero(t, reader.Len())
	return node
}

func protocolCorpusAuditPackets(t *testing.T, relativePath string) [][]byte {
	t.Helper()
	data := readProtocolCorpusFile(t, ".", relativePath)
	require.GreaterOrEqual(t, len(data), 4)
	var packetReader protocolCorpusPacketReader
	if bytes.Equal(data[:4], []byte{'\x0a', '\x0d', '\x0d', '\x0a'}) {
		reader, err := pcapgo.NewNgReader(bytes.NewReader(data), pcapgo.NgReaderOptions{SkipUnknownVersion: true})
		require.NoError(t, err)
		packetReader = reader
	} else {
		reader, err := pcapgo.NewReader(bytes.NewReader(data))
		if err != nil {
			normalized, ok := normalizeProtocolCorpusLegacyPcapHeader(data)
			require.True(t, ok)
			reader, err = pcapgo.NewReader(bytes.NewReader(normalized))
			require.NoError(t, err)
		}
		packetReader = reader
	}

	var packets [][]byte
	for {
		packet, _, err := packetReader.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		packets = append(packets, append([]byte(nil), packet...))
	}
	return packets
}
