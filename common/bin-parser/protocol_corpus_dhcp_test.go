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
)

func TestProtocolCorpusDHCPFullCapture(t *testing.T) {
	const capturePath = "testdata/protocol-corpus/captures/wireshark-tests/wireshark-dhcp.pcap"
	captureData := readProtocolCorpusFile(t, ".", capturePath)
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	expectedXID := []uint64{0x00003d1d, 0x00003d1d, 0x00003d1e, 0x00003d1e}
	expectedOperation := []uint64{1, 2, 1, 2}
	expectedMessageType := []uint64{1, 2, 3, 5}
	expectedSourcePort := []layers.UDPPort{68, 67, 68, 67}
	expectedDestinationPort := []layers.UDPPort{67, 68, 67, 68}

	frameNumber := 0
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		frameNumber++
		require.LessOrEqual(t, frameNumber, len(expectedXID))

		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		require.Nilf(t, packet.ErrorLayer(), "frame %d independent decode", frameNumber)
		udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Truef(t, ok, "frame %d UDP", frameNumber)
		require.Equal(t, expectedSourcePort[frameNumber-1], udp.SrcPort)
		require.Equal(t, expectedDestinationPort[frameNumber-1], udp.DstPort)

		input := append([]byte(nil), udp.Payload...)
		bounded := newProtocolCorpusBoundedReader(input)
		node, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "application-layer/dhcp.yaml", EntryNode: "DHCP", Layer: "L7",
		})
		require.NoErrorf(t, parseErr, "frame %d", frameNumber)
		require.Zero(t, bounded.Len(), "frame %d DHCP bytes unread", frameNumber)
		terminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
		require.NoErrorf(t, coverageErr, "frame %d", frameNumber)
		require.Positive(t, terminals)
		protocolCorpusRequireValue(t, node, "Operation", expectedOperation[frameNumber-1])
		protocolCorpusRequireValue(t, node, "Xid", expectedXID[frameNumber-1])
		protocolCorpusRequireValue(t, node, "Message Type", expectedMessageType[frameNumber-1])
	}

	require.Equal(t, len(expectedXID), frameNumber)
}
