package pcaputil

import (
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestBuildFrameDisplayNodesNestsPayloadAndSummarizesLayers(t *testing.T) {
	ethernet := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 1, 2, 3, 4, 5},
		DstMAC:       net.HardwareAddr{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ipv4 := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IPv4(10, 0, 0, 1),
		DstIP:    net.IPv4(10, 0, 0, 2),
	}
	tcp := &layers.TCP{
		SrcPort:    12345,
		DstPort:    80,
		Seq:        1,
		Ack:        2,
		DataOffset: 5,
		ACK:        true,
		Window:     65535,
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ipv4))

	payload := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(
		buffer,
		gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		ethernet,
		ipv4,
		tcp,
		gopacket.Payload(payload),
	))
	raw := append([]byte(nil), buffer.Bytes()...)
	nodes := BuildFrameDisplayNodes(FrameDisplayInput{
		Raw: raw, LinkType: int(layers.LinkTypeEthernet), PacketID: 7,
		CapturedLength: len(raw), OriginalLength: len(raw),
	})

	require.NotContains(t, displayRootNames(nodes), "Payload")
	ethernetRoot := findDisplayRoot(nodes, "Ethernet")
	require.NotNil(t, ethernetRoot)
	require.Equal(t, "00:01:02:03:04:05 → 06:07:08:09:0a:0b", ethernetRoot.DisplayValue)
	ipv4Root := findDisplayRoot(nodes, "IPv4")
	require.NotNil(t, ipv4Root)
	require.Equal(t, "10.0.0.1 → 10.0.0.2 · TCP · TTL 64", ipv4Root.DisplayValue)
	tcpRoot := findDisplayRoot(nodes, "TCP")
	require.NotNil(t, tcpRoot)
	require.True(t, strings.HasPrefix(tcpRoot.DisplayValue, "12345 → 80 · [ACK] · Seq=1 Ack=2 Win=65535"))
	require.Equal(t, "Payload", tcpRoot.Children[len(tcpRoot.Children)-1].Name)

	payloadNode := tcpRoot.Children[len(tcpRoot.Children)-1]
	payloadOffset := len(raw) - len(payload)
	require.Equal(t, []DisplayRange{{BitOffset: int64(payloadOffset * 8), BitLength: int64(len(payload) * 8)}}, payloadNode.Ranges)
}

func TestBuildProtocolFieldDisplayNodesPreservesNestedShape(t *testing.T) {
	nodes := BuildProtocolFieldDisplayNodes(map[string]any{
		"Header": map[string]any{
			"Host": "example.com",
			"Port": uint16(80),
		},
	})
	require.Len(t, nodes, 1)
	require.Equal(t, "Header", nodes[0].Name)
	require.Equal(t, []string{"Host", "Port"}, displayNodeNames(nodes[0].Children))
	require.Equal(t, "example.com", nodes[0].Children[0].DisplayValue)
	require.Equal(t, "80", nodes[0].Children[1].DisplayValue)
}

func findDisplayRoot(nodes []DisplayNode, name string) *DisplayNode {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func displayRootNames(nodes []DisplayNode) []string {
	return displayNodeNames(nodes)
}

func displayNodeNames(nodes []DisplayNode) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}
	return names
}
