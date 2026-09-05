package bin_parser

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// EAPOL-Key frame 87 of Wireshark SampleCaptures wpa-Induction.pcap
// (first message of the WPA 4-way handshake), as embedded in
// gopacket layers/eapol_test.go.
var eapolKeyFromWPAInduction = []byte{
	0x02, 0x03, 0x00, 0x75, 0x02, 0x00, 0x8a, 0x00,
	0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x3e, 0x8e, 0x96, 0x7d, 0xac, 0xd9, 0x60,
	0x32, 0x4c, 0xac, 0x5b, 0x6a, 0xa7, 0x21, 0x23,
	0x5b, 0xf5, 0x7b, 0x94, 0x97, 0x71, 0xc8, 0x67,
	0x98, 0x9f, 0x49, 0xd0, 0x4e, 0xd4, 0x7c, 0x69,
	0x33, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x16, 0xdd, 0x14, 0x00, 0x0f, 0xac,
	0x04, 0x59, 0x2d, 0xa8, 0x80, 0x96, 0xc4, 0x61,
	0xda, 0x24, 0x6c, 0x69, 0x00, 0x1e, 0x87, 0x7f,
	0x3d,
}

func eapolEthernetFrame(t *testing.T, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x01, 0x80, 0xc2, 0x00, 0x00, 0x03},
		EthernetType: layers.EthernetTypeEAPOL,
	}
	return serializeLayers(t, eth, gopacket.Payload(payload))
}

func TestEAPOLKeyFromWPAInductionCapture(t *testing.T) {
	n := parseRule(t, eapolKeyFromWPAInduction, "eapol", "EAPOL")
	require.Equal(t, uint64(2), uintVal(t, n.Child("Protocol Version")))
	require.Equal(t, uint64(3), uintVal(t, n.Child("Packet Type")))
	require.Equal(t, uint64(0x75), uintVal(t, n.Child("Body Length")))
	key := mustChild(t, n, "EAPOLKey")
	require.Equal(t, uint64(2), uintVal(t, key.Child("Descriptor Type")))
	require.Equal(t, uint64(16), uintVal(t, key.Child("Key Length")))
	require.Equal(t, uint64(22), uintVal(t, key.Child("Key Data Length")))
	require.Equal(t, []byte{0x3e, 0x8e, 0x96, 0x7d, 0xac, 0xd9, 0x60, 0x32}, bytesVal(t, key.Child("Key Nonce"))[:8])
	require.Len(t, bytesVal(t, key.Child("Key Data")), 22)

	eth := parseEthernet(t, eapolEthernetFrame(t, eapolKeyFromWPAInduction))
	wired := mustChild(t, eth, "EAPOL", "EAPOLKey")
	require.Equal(t, uint64(22), uintVal(t, wired.Child("Key Data Length")))
}

func TestEAPOLStartLogoffAndEAPPacket(t *testing.T) {
	start := []byte{0x01, 0x01, 0x00, 0x00}
	s := parseRule(t, start, "eapol", "EAPOL")
	require.Equal(t, uint64(1), uintVal(t, s.Child("Packet Type")))
	require.Equal(t, uint64(0), uintVal(t, s.Child("Body Length")))

	logoff := []byte{0x01, 0x02, 0x00, 0x00}
	require.Equal(t, uint64(2), uintVal(t, parseRule(t, logoff, "eapol", "EAPOL").Child("Packet Type")))

	// EAPOL wrapping EAP-Request Identity (IEEE 802.1X).
	eap := []byte{0x01, 0x00, 0x00, 0x05, 0x01, 0x01, 0x00, 0x05, 0x01}
	p := parseRule(t, eap, "eapol", "EAPOL")
	require.Equal(t, uint64(0), uintVal(t, p.Child("Packet Type")))
	pkt := mustChild(t, p, "EAPPacket")
	require.Equal(t, uint64(1), uintVal(t, pkt.Child("Code")))
	require.Equal(t, uint64(1), uintVal(t, pkt.Child("Identifier")))

	eth := parseEthernet(t, eapolEthernetFrame(t, start))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "EAPOL", "Packet Type")))
}

func TestEAPOLEdges(t *testing.T) {
	parseMustFail(t, nil, "eapol", "EAPOL")
	parseMustFail(t, []byte{0x02, 0x03}, "eapol", "EAPOL")
	parseMustFail(t, []byte{0x02, 0x03, 0x00, 0x75, 0x02}, "eapol", "EAPOL")
}

func TestEthernetIPARPTruncated(t *testing.T) {
	parseMustFail(t, nil, "ethernet")
	parseMustFail(t, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x11, 0x22}, "ethernet")
	// EtherType IPv4 but no IP header.
	shortIP := make([]byte, 14)
	shortIP[12] = 0x08
	parseMustFail(t, shortIP, "ethernet")
	parseMustFail(t, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x81, 0x00}, "ethernet")
}
