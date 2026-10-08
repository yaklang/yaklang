package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func replayGOOSECapture(t *testing.T, capture []byte, options ...CaptureOption) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	options = append(options, WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	err := ReplayPcap(bytes.NewReader(capture), options...)
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
	require.NoError(t, replayFixtureFile(t, path, WithOnProtocolMessage(func(event *ProtocolEvent) {
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

func TestReplayPcapGOOSEVLANCarriers(t *testing.T) {
	// Synthetic Ethernet/802.1Q/802.1ad headers carry the same IECGoosePdu.
	// TCI contains PCP=4, VID=10; QinQ adds an outer service VID=100.
	// These fixtures exercise ingress, not independently captured device traffic.
	valid := goosePDU()
	for _, tc := range []struct {
		name   string
		typ    layers.EthernetType
		tags   []byte
		domain string
	}{
		{"untagged", 0x88b8, nil, ""},
		{"priority-tagged", 0x8100, []byte{0x80, 0, 0x88, 0xb8}, "/vlan:0"},
		{"802.1Q", 0x8100, []byte{0x80, 10, 0x88, 0xb8}, "/vlan:10"},
		{"802.1ad", 0x88a8, []byte{0, 100, 0x81, 0, 0x80, 10, 0x88, 0xb8}, "/vlan:100/vlan:10"},
	} {
		for _, workers := range []int{1, 2, 4} {
			t.Run(fmt.Sprintf("%s/workers-%d", tc.name, workers), func(t *testing.T) {
				payload := append(bytes.Clone(tc.tags), valid...)
				events := replayGOOSECapture(t, oneFrameEthernetCapture(t, tc.typ, payload), WithTCPReassemblyWorkers(workers))
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "goose", e.Protocol)
				require.Equal(t, "decoded", e.Status, e.Error)
				require.Equal(t, valid, e.Raw, "VLAN tags must not enter the GOOSE PDU")
				require.Equal(t, tc.domain, e.Domain.Encapsulation)
				require.Equal(t, "02:00:00:00:00:01", e.Source)
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)

				bad := bytes.Clone(payload)
				bad[len(tc.tags)+8] = 0x60
				for _, event := range replayGOOSECapture(t, oneFrameEthernetCapture(t, tc.typ, bad), WithTCPReassemblyWorkers(workers)) {
					require.NotEqual(t, "goose", event.Protocol, "VLAN evidence cannot override a failed GOOSE probe")
				}
				capture := oneFrameEthernetCapture(t, tc.typ, payload)
				for _, cut := range []int{0, 1, 3, len(tc.tags), len(tc.tags) + 8, len(tc.tags) + 9, len(payload) - 1} {
					// A real snaplen cut: preserve original wire length, shorten
					// both the stored frame and its pcap included-length field.
					truncated := bytes.Clone(capture[:40+14+cut])
					binary.LittleEndian.PutUint32(truncated[32:36], uint32(14+cut))
					var truncatedEvents []*ProtocolEvent
					_ = ReplayPcap(bytes.NewReader(truncated), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(event *ProtocolEvent) { truncatedEvents = append(truncatedEvents, event) }))
					for _, event := range truncatedEvents {
						require.NotEqual(t, "decoded", event.Status, "cut %d must not become a complete GOOSE message", cut)
					}
				}
			})
		}
	}
	for _, payload := range [][]byte{
		{0x80, 10, 0x88}, // truncated VLAN header
		append([]byte{0x80, 10, 0x88, 0xba}, valid...), // Sampled Values EtherType, not GOOSE
	} {
		for _, workers := range []int{1, 2, 4} {
			var events []*ProtocolEvent
			_ = ReplayPcap(bytes.NewReader(oneFrameEthernetCapture(t, 0x8100, payload)), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
			for _, event := range events {
				require.NotEqual(t, "goose", event.Protocol)
			}
		}
	}
}

func gooseOptionalFieldsFixture(t *testing.T, omit byte) []byte {
	t.Helper()
	wire := goosePDU()
	_, body, _, err := berRead(wire, 8)
	require.NoError(t, err)
	var filtered []byte
	for at := 0; at < len(body); {
		tag, _, next, err := berRead(body, at)
		require.NoError(t, err)
		// Bits 0..2 select goID, simulation, and ndsCom respectively.
		drop := tag == 0x83 && omit&1 != 0 || tag == 0x87 && omit&2 != 0 || tag == 0x89 && omit&4 != 0
		if !drop {
			filtered = append(filtered, body[at:next]...)
		}
		at = next
	}
	result := append(bytes.Clone(wire[:8]), berPut(0x61, filtered)...)
	binary.BigEndian.PutUint16(result[2:4], uint16(len(result)))
	return result
}

func TestReplayPcapGOOSEOptionalAndDefaultFields(t *testing.T) {
	// Synthetic variants of goosePDU(), using the IECGoosePdu ASN.1:
	// goID OPTIONAL; simulation and ndsCom BOOLEAN DEFAULT FALSE.
	// https://github.com/wireshark/wireshark/blob/4f63ea0eae68cf6facea31604994f1a339e43640/epan/dissectors/asn1/goose/goose.asn
	for omit := byte(0); omit < 8; omit++ {
		wire := gooseOptionalFieldsFixture(t, omit)
		events := replayGOOSECapture(t, oneFrameEthernetCapture(t, 0x88b8, wire))
		require.Len(t, events, 1, "omission mask %d", omit)
		e := events[0]
		require.Equal(t, "decoded", e.Status, "omission mask %d: %s", omit, e.Error)
		require.Equal(t, "dataset1", e.Session["Dataset"])
		require.Equal(t, []bool{true}, e.Session["Boolean"])
		require.Equal(t, false, e.Session["Simulation"])
		require.Equal(t, false, e.Session["Needs Commissioning"])
		if omit&1 != 0 {
			require.NotContains(t, e.Session, "GOOSE ID")
		} else {
			require.Equal(t, "goid1", e.Session["GOOSE ID"])
		}
	}
	// BER BOOLEAN accepts any nonzero content octet as TRUE.
	truthy := goosePDU()
	for at := 10; at < len(truthy); {
		tag, _, next, err := berRead(truthy, at)
		require.NoError(t, err)
		if tag == 0x87 || tag == 0x89 {
			truthy[next-1] = 0xff
		}
		at = next
	}
	events := replayGOOSECapture(t, oneFrameEthernetCapture(t, 0x88b8, truthy))
	require.Len(t, events, 1)
	require.Equal(t, "decoded", events[0].Status, events[0].Error)
	require.Equal(t, true, events[0].Session["Simulation"])
	require.Equal(t, true, events[0].Session["Needs Commissioning"])
}
