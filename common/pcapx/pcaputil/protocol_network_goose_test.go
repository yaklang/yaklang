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
	"github.com/yaklang/yaklang/internal/trafficfixture"
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

// Whole selected session answer from power-02-goose.pcapng's literal BER,
// independently cross-checked with Wireshark4.2.5 PDML. This checks history
// ownership; it does not infer publisher identity or GOOSE retransmit state.
func gooseHistoryBooleanOwnership(t *testing.T) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("../../bin-parser/testdata/winlab5013/captures/power-02-goose.pcapng")
	require.NoError(t, err)
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, _ := discoveryReplay(t, raw, workers, deferred, observe)
		require.Len(t, events, 2)
		history, err := NewBinParserInspector(2, 1<<20)
		require.NoError(t, err)
		for i, e := range events {
			expected := map[string]any{"Packet Name": "GOOSE", "APPID": 0x3000, "Length": 96, "Simulation": false, "Needs Commissioning": false, "Control Block Reference": "LABP1/LLN0$GO$gcb1", "Dataset": "LABP1/LLN0$ds1", "GOOSE ID": "LAB-GOOSE-1", "State Number": 5 + i, "Sequence Number": 0, "Dataset Entry Count": 1, "Boolean": []bool{i == 1}}
			require.Equal(t, "goose", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, expected, e.Session)
			testLegacyRuleQueryOwnership(t, e)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			history.OnEvent(e)
			e.Session["Boolean"].([]bool)[0] = i != 1
			testOwnedSessionQueries(t, e, expected)
			detail, err := history.Details(e.ID)
			require.NoError(t, err)
			require.Equal(t, expected, detail.Session)
			require.Equal(t, e.Raw, detail.Raw)
			require.Equal(t, e.SourceBytes, detail.SourceBytes)
			detail.Session["Boolean"].([]bool)[0] = i != 1
			decoded, err := detail.Decode()
			require.NoError(t, err)
			decoded["session"].(map[string]any)["Boolean"].([]bool)[0] = i != 1
			again, err := history.Details(e.ID)
			require.NoError(t, err)
			require.Equal(t, expected, again.Session)
		}
		require.Len(t, history.Rows("goose", 0), 2)
		require.Zero(t, history.Evicted())
	})
}

// Retained logical charge: one map64 + key7 + slot32 + slice24 + three
// boolean values3 =130. A129-byte inspector must refuse before cloning.
func gooseBooleanHistoryBudget(t *testing.T) {
	t.Helper()
	e := &ProtocolEvent{ID: 1, Protocol: "goose", Session: map[string]any{"Boolean": []bool{false, true, false}}}
	for _, limit := range []int{129, 130} {
		history, err := NewBinParserInspector(1, limit)
		require.NoError(t, err)
		history.OnEvent(e)
		if limit == 129 {
			require.Empty(t, history.Rows("goose", 0))
			require.EqualValues(t, 1, history.Evicted())
		} else {
			require.Len(t, history.Rows("goose", 0), 1)
			detail, err := history.Details(1)
			require.NoError(t, err)
			require.Equal(t, e.Session, detail.Session)
			require.Zero(t, history.Evicted())
		}
	}
}
func TestProtocolInspectorBooleanVectorBudget(t *testing.T) {
	gooseBooleanHistoryBudget(t)
}

// The pinned ITI capture's eight complete selected session projections were
// independently decoded by Wireshark 4.2.5. The bit payloads are four 0000
// values per message; this does not infer publisher identity or pad-bit semantics.
func gooseHistoryBitsOwnership(t *testing.T) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-corpus/captures/iti-ics/iti-goose.pcap")
	require.NoError(t, err)
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, _ := discoveryReplay(t, raw, workers, deferred, observe)
		require.Len(t, events, 8)
		history, err := NewBinParserInspector(8, 1<<20)
		require.NoError(t, err)
		lengths := []int{145, 145, 145, 144, 144, 144, 144, 145}
		sequences := []int{10, 11, 12, 1, 2, 3, 4, 5}
		for i, e := range events {
			expected := map[string]any{"Packet Name": "GOOSE", "APPID": 1, "Length": lengths[i], "Simulation": false, "Needs Commissioning": false, "Control Block Reference": "GEDeviceF650/LLN0$GO$gcb01", "Dataset": "GEDeviceF650/LLN0$GOOSE1", "GOOSE ID": "F650_GOOSE1", "State Number": 1, "Sequence Number": sequences[i], "Dataset Entry Count": 8, "Boolean": []bool{false, false, false, false}, "Bits": [][]byte{{0, 0}, {0, 0}, {0, 0}, {0, 0}}}
			require.Equal(t, "goose", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, expected, e.Session)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			history.OnEvent(e)
			bits := e.Session["Bits"].([][]byte)
			bits[0] = []byte{0xff}
			bits[1][0] = 0xff
			testOwnedSessionQueries(t, e, expected)
			detail, err := history.Details(e.ID)
			require.NoError(t, err)
			require.Equal(t, expected, detail.Session)
			require.Equal(t, e.Raw, detail.Raw)
			require.Equal(t, e.SourceBytes, detail.SourceBytes)
			detail.Session["Bits"].([][]byte)[2][0] = 0xff
			decoded, err := detail.Decode()
			require.NoError(t, err)
			decoded["session"].(map[string]any)["Bits"].([][]byte)[3][0] = 0xff
			again, err := history.Details(e.ID)
			require.NoError(t, err)
			require.Equal(t, expected, again.Session)
		}
		require.Len(t, history.Rows("goose", 0), 8)
		require.Zero(t, history.Evicted())
	})
}

func gooseBitsHistoryBudget(t *testing.T) {
	t.Helper()
	// Map64 + key4 + slot32 + outer slice24 + three inner headers72 +
	// two owned payload bytes2 =198; spare caller capacity is not retained.
	large := make([]byte, 2, 4096)
	copy(large, []byte{0xa0, 0x00})
	e := &ProtocolEvent{ID: 1, Protocol: "goose", Session: map[string]any{"Bits": [][]byte{nil, {}, large}}}
	for _, limit := range []int{197, 198} {
		history, err := NewBinParserInspector(1, limit)
		require.NoError(t, err)
		history.OnEvent(e)
		if limit == 197 {
			require.Empty(t, history.Rows("goose", 0))
			require.EqualValues(t, 1, history.Evicted())
		} else {
			detail, err := history.Details(1)
			require.NoError(t, err)
			require.Equal(t, e.Session, detail.Session)
			bits := detail.Session["Bits"].([][]byte)
			require.Nil(t, bits[0])
			require.NotNil(t, bits[1])
			require.Equal(t, len(bits[2]), cap(bits[2]))
			require.Zero(t, history.Evicted())
		}
	}
}

func TestProtocolInspectorBitStringVectorBudget(t *testing.T) {
	gooseBitsHistoryBudget(t)
}
