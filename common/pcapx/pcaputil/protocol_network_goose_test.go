package pcaputil

import (
	"bytes"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func replayGOOSECapture(t *testing.T, capture []byte) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(capture), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	require.NoError(t, err)
	return events
}

func oneFrameEthernetCapture(t *testing.T, etherType layers.EthernetType, payload []byte) []byte {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriter(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	eth := &layers.Ethernet{
		SrcMAC:       []byte{2, 0, 0, 0, 0, 1},
		DstMAC:       []byte{1, 12, 205, 1, 0, 1},
		EthernetType: etherType,
	}
	buf := gopacket.NewSerializeBuffer()
	serial := []gopacket.SerializableLayer{eth, gopacket.Payload(payload)}
	if etherType == layers.EthernetTypeIPv4 {
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
		udp := &layers.UDP{SrcPort: 3478, DstPort: 3478}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		serial = []gopacket.SerializableLayer{eth, ip, udp, gopacket.Payload(payload)}
	}
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
	raw := buf.Bytes()
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	return capture.Bytes()
}

func TestReplayPcapFileRecognizesGOOSEEtherType(t *testing.T) {
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures", "power-02-goose.pcapng")
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcapFile(path, WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	})))

	var goose []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "goose" {
			goose = append(goose, event)
		}
	}
	require.Len(t, goose, 2, "both GOOSE frames from the real pcapng must traverse packet ingress")
	for i, event := range goose {
		require.Equal(t, "decoded", event.Status)
		require.Equal(t, "ethernet", event.Transport)
		require.Equal(t, "iec61850-goose", event.Profile)
		require.Equal(t, "ether-type-and-wire-signature", event.Admission)
		require.Equal(t, "iec61850", event.Rule)
		require.Equal(t, "GOOSE", event.Entry)
		require.Equal(t, "captured", event.SourceBytes.Kind)
		require.Len(t, event.SourceBytes.PacketRefs, 1)
		require.NotEmpty(t, event.Fields)
		require.Equal(t, uint16(0x3000), event.Fields["APPID"])
		require.Equal(t, 0x3000, event.Session["APPID"])
		require.Equal(t, "LABP1/LLN0$GO$gcb1", event.Session["Control Block Reference"])
		require.Equal(t, "LABP1/LLN0$ds1", event.Session["Dataset"])
		require.Equal(t, "LAB-GOOSE-1", event.Session["GOOSE ID"])
		require.Equal(t, 5+i, event.Session["State Number"])
		require.Equal(t, []bool{i == 1}, event.Session["Boolean"])
	}
}

func TestReplayPcapRejectsGOOSELikeEthernetFrames(t *testing.T) {
	valid := goosePDU()
	wrongEtherType := replayGOOSECapture(t, oneFrameEthernetCapture(t, layers.EthernetTypeIPv4, valid))
	badPDU := append([]byte(nil), valid...)
	badPDU[8] = 0x60 // not the GOOSE goosePdu tag
	wrongPDU := replayGOOSECapture(t, oneFrameEthernetCapture(t, layers.EthernetType(0x88b8), badPDU))

	for _, events := range [][]*ProtocolEvent{wrongEtherType, wrongPDU} {
		for _, event := range events {
			require.NotEqual(t, "goose", event.Protocol, "a near-match must not be claimed as GOOSE")
		}
	}
}
