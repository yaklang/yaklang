package yakgrpc

import (
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestTrafficFrameFieldsNestsPayloadUnderTransport(t *testing.T) {
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
	packet := pcapdb.Packet{
		CapturedLength: len(raw),
		OriginalLength: len(raw),
		LinkType:       int(layers.LinkTypeEthernet),
	}
	packet.ID = 7
	fields := trafficFrameFields(packet, raw)

	require.NotContains(t, trafficRootNames(fields), "Payload")
	ethernetRoot := trafficRoot(fields, "Ethernet")
	require.NotNil(t, ethernetRoot)
	require.Equal(t, "00:01:02:03:04:05 → 06:07:08:09:0a:0b", ethernetRoot.DisplayValue)
	ipv4Root := trafficRoot(fields, "IPv4")
	require.NotNil(t, ipv4Root)
	require.Equal(t, "10.0.0.1 → 10.0.0.2 · TCP · TTL 64", ipv4Root.DisplayValue)
	tcpRoot := trafficRoot(fields, "TCP")
	require.NotNil(t, tcpRoot)
	require.True(t, strings.HasPrefix(tcpRoot.DisplayValue, "12345 → 80 · [ACK] · Seq=1 Ack=2 Win=65535"))
	tcpFields, ok := tcpRoot.Value.([]trafficField)
	require.True(t, ok)
	require.Equal(t, "Payload", tcpFields[len(tcpFields)-1].Name)

	value, ok := tcpFields[len(tcpFields)-1].Value.(trafficFieldValue)
	require.True(t, ok)
	payloadOffset := len(raw) - len(payload)
	require.Equal(t, []int{payloadOffset * 8, len(raw) * 8}, value.Scope)

	nodes := pcapFrameFieldNodes(fields, "frame:7")
	var tcpNode *ypb.PCAPFieldNode
	for _, node := range nodes {
		if node.GetName() == "TCP" {
			tcpNode = node
			break
		}
	}
	require.NotNil(t, tcpNode)
	require.Equal(t, tcpRoot.DisplayValue, tcpNode.GetDisplayValue())
}

func trafficRoot(fields []trafficField, name string) *trafficField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}

func trafficRootNames(fields []trafficField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}
