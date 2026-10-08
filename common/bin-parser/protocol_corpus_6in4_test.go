package bin_parser

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const sixInFourCorpusCapture = "testdata/protocol-corpus/captures/ndpi/ndpi-6in4.pcap"

func TestProtocolCorpus6in4AllPackets(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", sixInFourCorpusCapture)
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	packetCount := 0
	nextHeaders := make(map[uint64]int)
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++

		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
		require.NotNilf(t, ipv4Layer, "frame %d has no outer IPv4 layer", packetCount)
		ipv4 := ipv4Layer.(*layers.IPv4)
		require.Equalf(t, layers.IPProtocolIPv6, ipv4.Protocol, "frame %d outer protocol", packetCount)
		require.GreaterOrEqualf(t, len(ipv4.Payload), 40, "frame %d inner IPv6 packet", packetCount)
		require.Equalf(t, uint8(6), ipv4.Payload[0]>>4, "frame %d inner version", packetCount)
		if ipv4.Payload[6] == 17 {
			require.GreaterOrEqualf(t, len(ipv4.Payload), 48, "frame %d inner UDP packet", packetCount)
			dnsInput := ipv4.Payload[48:]
			dnsReader := newProtocolCorpusBoundedReader(dnsInput)
			_, dnsErr := protocolCorpusParseRule(dnsReader, protocolCorpusParseContract{
				RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7",
			})
			require.NoErrorf(t, dnsErr, "frame %d inner DNS message", packetCount)
			require.Zero(t, dnsReader.Len(), "frame %d inner DNS bytes unread", packetCount)
		}

		input := protocolCorpusLayerAndPayload(t, ipv4, "outer IPv4")
		bounded := newProtocolCorpusBoundedReader(input)
		node, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "internet_protocol.yaml", EntryNode: "Internet Protocol", Layer: "L3",
		})
		require.NoErrorf(t, parseErr, "frame %d", packetCount)
		require.Zero(t, bounded.Len(), "frame %d left bytes unread", packetCount)
		terminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
		require.NoErrorf(t, coverageErr, "frame %d", packetCount)
		require.Positivef(t, terminals, "frame %d produced no terminal fields", packetCount)
		protocolCorpusRequireValue(t, node, "Protocol", uint64(41))

		ipv6 := sixInFourFindNode(node, "IPv6")
		require.NotNilf(t, ipv6, "frame %d has no nested IPv6 result", packetCount)
		protocolCorpusRequireValue(t, ipv6, "Version", uint64(6))
		nextHeader := protocolCorpusFindNode(ipv6, "Next Header")
		require.NotNilf(t, nextHeader, "frame %d has no IPv6 next-header field", packetCount)
		value, valueErr := nextHeader.Result()
		require.NoError(t, valueErr)
		nextHeaders[uintVal(t, value)]++
	}

	require.Equal(t, 127, packetCount)
	require.Equal(t, map[uint64]int{
		6:  75,
		17: 4,
		58: 48,
	}, nextHeaders)
}

func TestProtocolCorpus6in4EDNSAdditionalRecord(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", sixInFourCorpusCapture)
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)

	var dnsInput []byte
	for frameNumber := 1; frameNumber <= 18; frameNumber++ {
		frame, _, readErr := reader.ReadPacketData()
		require.NoErrorf(t, readErr, "read frame %d", frameNumber)
		if frameNumber != 18 {
			continue
		}
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		ipv4, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		require.True(t, ok)
		require.Equal(t, layers.IPProtocolIPv6, ipv4.Protocol)
		require.GreaterOrEqual(t, len(ipv4.Payload), 48)
		require.Equal(t, uint8(17), ipv4.Payload[6], "inner IPv6 next header")
		dnsInput = append([]byte(nil), ipv4.Payload[48:]...)
	}

	require.Equal(t, []byte{
		0x1a, 0x02, 0x00, 0x10, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x04, 's', 't', 'a', 'r',
		0x04, 'c', '1', '0', 'r', 0x08, 'f', 'a', 'c', 'e', 'b', 'o', 'o', 'k',
		0x03, 'c', 'o', 'm', 0x00, 0x00, 0x1c, 0x00, 0x01,
		0x00, 0x00, 0x29, 0x10, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00,
	}, dnsInput, "frame 18 inner DNS payload changed")

	t.Run("exact fields and complete consumption", func(t *testing.T) {
		bounded := newProtocolCorpusBoundedReader(dnsInput)
		node, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7",
		})
		require.NoError(t, parseErr)
		require.Zero(t, bounded.Len())
		terminals, coverageErr := protocolCorpusTerminalCoverage(node, dnsInput)
		require.NoError(t, coverageErr)
		require.Positive(t, terminals)

		value, valueErr := node.Result()
		require.NoError(t, valueErr)
		header := mustChild(t, value, "Header")
		require.Equal(t, uint64(0x1a02), uintVal(t, header.Child("ID")))
		require.Equal(t, uint64(0x0010), uintVal(t, header.Child("Flags")))
		require.Equal(t, uint64(1), uintVal(t, header.Child("Questions")))
		require.Equal(t, uint64(0), uintVal(t, header.Child("Answer RRs")))
		require.Equal(t, uint64(0), uintVal(t, header.Child("Authority RRs")))
		require.Equal(t, uint64(1), uintVal(t, header.Child("Additional RRs")))

		questions := mustChild(t, value, "Questions").Children()
		require.Len(t, questions, 1)
		labels := mustChild(t, questions[0], "Name").Children()
		require.GreaterOrEqual(t, len(labels), 5)
		require.Equal(t, "star", strVal(t, labels[0].Child("Text")))
		require.Equal(t, "c10r", strVal(t, labels[1].Child("Text")))
		require.Equal(t, "facebook", strVal(t, labels[2].Child("Text")))
		require.Equal(t, "com", strVal(t, labels[3].Child("Text")))
		require.Equal(t, uint64(0), uintVal(t, labels[4].Child("Count")))
		require.Equal(t, uint64(28), uintVal(t, questions[0].Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, questions[0].Child("Class")))

		additional := mustChild(t, value, "Additional").Children()
		require.Len(t, additional, 1)
		opt := additional[0]
		optName := mustChild(t, opt, "Name", "Labels").Children()
		require.Len(t, optName, 1)
		require.Equal(t, uint64(0), uintVal(t, optName[0].Child("Count")))
		require.Equal(t, uint64(41), uintVal(t, opt.Child("Type")))
		require.Equal(t, uint64(4096), uintVal(t, opt.Child("Class")))
		require.Equal(t, uint64(0x00008000), uintVal(t, opt.Child("TTL")))
		require.Equal(t, uint64(0), uintVal(t, opt.Child("RDLength")))
	})

	t.Run("truncated OPT record is rejected", func(t *testing.T) {
		bounded := newProtocolCorpusBoundedReader(dnsInput[:len(dnsInput)-1])
		_, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7",
		})
		require.Error(t, parseErr)
	})

	t.Run("declared additional count is enforced", func(t *testing.T) {
		declaresTwo := append([]byte(nil), dnsInput...)
		declaresTwo[10], declaresTwo[11] = 0x00, 0x02
		bounded := newProtocolCorpusBoundedReader(declaresTwo)
		_, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "application-layer/dns.yaml", EntryNode: "DNS", Layer: "L7",
		})
		require.Error(t, parseErr)
	})
}

func sixInFourFindNode(node *base.Node, name string) *base.Node {
	if node.Name == name && protocolCorpusNodeHasResult(node) {
		return node
	}
	for _, child := range node.Children {
		if found := sixInFourFindNode(child, name); found != nil {
			return found
		}
	}
	return nil
}
