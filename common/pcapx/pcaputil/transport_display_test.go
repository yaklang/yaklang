package pcaputil

import (
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTransportDisplayHelpersShareFlagPortAndPayloadRules(t *testing.T) {
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, ACK: true, PSH: true}
	tcp.Payload = []byte("hello")
	require.Equal(t, []string{"ACK", "PSH"}, TCPFlagNames(tcp))
	require.Equal(t, "ACK, PSH", TCPFlagText(tcp))
	require.Equal(t, "[ACK, PSH]", TCPFlagBracketed(tcp))
	require.Equal(t, TransportPorts{Source: 12345, Destination: 80}, TCPTransportPorts(tcp))
	require.Equal(t, "12345 → 80", TCPTransportPorts(tcp).Arrow())
	require.Equal(t, "SrcPort=12345 DstPort=80", TCPTransportPorts(tcp).Labeled())
	require.Equal(t, len("hello"), TCPPayloadLength(tcp))

	udp := &layers.UDP{SrcPort: 5353, DstPort: 53, Length: 15}
	udp.Payload = []byte("dns-data")
	require.Equal(t, TransportPorts{Source: 5353, Destination: 53}, UDPTransportPorts(udp))
	require.Equal(t, "5353 → 53", UDPTransportPorts(udp).Arrow())
	require.Equal(t, "SrcPort=5353 DstPort=53", UDPTransportPorts(udp).Labeled())
	require.Equal(t, len("dns-data"), UDPPayloadLength(udp))
}
