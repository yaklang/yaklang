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

const ftpsCorpusCapture = "testdata/protocol-corpus/captures/ndpi/ndpi-ftps.pcap"

func TestProtocolCorpusFTPSExplicitNegotiation(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", ftpsCorpusCapture)
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	payloads := make(map[int][]byte)
	packetCount := 0
	dataFrames := 0
	controlFrames := 0
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errorLayer := packet.ErrorLayer(); errorLayer != nil {
			t.Fatalf("frame %d decode failed: %v", packetCount, errorLayer.Error())
		}
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		require.NotNilf(t, tcpLayer, "frame %d has no TCP layer", packetCount)
		payload := tcpLayer.(*layers.TCP).Payload
		if len(payload) == 0 {
			controlFrames++
			continue
		}
		dataFrames++
		payloads[packetCount] = append([]byte(nil), payload...)
	}

	require.Equal(t, 51, packetCount)
	require.Equal(t, 40, dataFrames)
	require.Equal(t, 11, controlFrames)

	auth := payloads[7]
	require.Equal(t, []byte("AUTH TLS\r\n"), auth)
	authNode := ftpsParseExact(t, auth, protocolCorpusParseContract{
		RuleFile: "application-layer/ftp.yaml", EntryNode: "FTPAuthTLS", Layer: "L7",
	})
	protocolCorpusRequireValue(t, authNode, "Command", "AUTH")
	protocolCorpusRequireValue(t, authNode, "Mechanism", "TLS")

	response := payloads[10]
	require.Equal(t, []byte("234 Proceed with negotiation.\r\n"), response)
	responseNode := ftpsParseExact(t, response, protocolCorpusParseContract{
		RuleFile: "application-layer/ftp.yaml", EntryNode: "FTP", Layer: "L7",
	})
	protocolCorpusRequireValue(t, responseNode, "Code", "234")
	protocolCorpusRequireValue(t, responseNode, "Message", "Proceed with negotiation.")

	clientHello := payloads[12]
	require.Len(t, clientHello, 150)
	tlsNode := ftpsParseExact(t, clientHello, protocolCorpusParseContract{
		RuleFile: "application-layer/tls.yaml", Layer: "L7",
	})
	protocolCorpusRequireValue(t, tlsNode, "ContentType", uint64(22))
	protocolCorpusRequireValue(t, tlsNode, "Version", uint64(0x0303))
	protocolCorpusRequireValue(t, tlsNode, "Length", uint64(145))
	protocolCorpusRequireValue(t, tlsNode, "Handshake Type", uint64(1))
	protocolCorpusRequireValue(t, tlsNode, "Legacy Version", uint64(0x0303))
}

func ftpsParseExact(t *testing.T, input []byte, contract protocolCorpusParseContract) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := protocolCorpusParseRule(reader, contract)
	require.NoError(t, err)
	require.Zero(t, reader.Len(), "parser left bytes unread")
	terminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
	require.NoError(t, coverageErr)
	require.Positive(t, terminals)
	return node
}
