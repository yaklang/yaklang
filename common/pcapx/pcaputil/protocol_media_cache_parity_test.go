package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestSDPMediaEquivalentSpellingIsDeterministic(t *testing.T) {
	now := time.Unix(1790000000, 0)
	for _, codec := range []string{"PCMA", "opus"} {
		t.Run(codec, func(t *testing.T) {
			base := &sipMediaAssociation{endpoint: "192.0.2.1:5000", callID: "mvp-call", mediaType: "audio", proto: "RTP/AVP", payload: 96, encoding: codec, clockRate: 8000, negotiated: true, expires: now.Add(time.Hour)}
			other := *base
			if codec == "PCMA" {
				other.encoding = "pcma"
			}
			other.proto = "rtp/avp"
			parser := &binParser{mediaIndex: map[mediaEndpointKey][]*sipMediaAssociation{{endpoint: base.endpoint}: {base, &other}}, mediaEntries: []*sipMediaAssociation{base, &other}}
			for i := 0; i < 512; i++ {
				match := parser.matchSDPMedia(CaptureDomain{}, base.endpoint, "192.0.2.2:6000", 96, false, now)
				require.Equal(t, "matched", match.status)
				require.Equal(t, codec, match.encoding)
				require.Equal(t, "RTP/AVP", match.proto)
			}
		})
	}
}

func TestRTSPMediaFinalSemanticCacheParity(t *testing.T) {
	now := time.Unix(1790000000, 0)
	association := &rtspMediaAssociation{session: "mvp-camera", profile: "RTP/AVP/UDP", clientRTP: "192.0.2.1:5000", clientRTCP: "192.0.2.1:5001", serverRTP: "192.0.2.2:6000", serverRTCP: "192.0.2.2:6001", eventIDs: []uint64{41, 42}, packetRefs: []PacketReference{{Number: 1}, {Number: 2}}}
	for _, rtcp := range []bool{false, true} {
		var reference map[string]any
		for _, deferred := range []bool{false, true} {
			budget := DefaultParserBudget()
			parser := &binParser{budget: budget, config: BinParserConfig{MaxMessageBytes: budget.MaxMessageBytes, MaxBufferedBytes: budget.MaxBufferedBytes, Deferred: deferred}}
			wire := rtspUDPTestRTP(96, 1, 0x12345678)
			e := &ProtocolEvent{Transport: "udp", Source: association.serverRTP, Destination: association.clientRTP, Timestamp: now}
			if rtcp {
				wire, e.Source, e.Destination = rtspUDPTestRTCP(), association.serverRTCP, association.clientRTCP
			}
			require.True(t, parser.decodeRTSPMediaDatagram(e, wire, rtspMediaMatch{association: association, rtcp: rtcp}))
			if deferred {
				require.Nil(t, e.Structured)
			} else {
				require.NotNil(t, e.Structured)
			}
			for i := 0; i < 2; i++ {
				decoded, err := e.Decode()
				require.NoError(t, err)
				fields := decoded["fields"].(map[string]any)
				require.Equal(t, "matched-rtsp-transport", fields["Association Status"])
				require.Equal(t, e.Session, fields)
				if reference == nil {
					reference = fields
				} else {
					require.Equal(t, reference, fields)
				}
			}
			parser.closeUDPSessions()
			require.Zero(t, parser.buffered.Load())
		}
	}
}

func mediaSetupUDPCapture(t *testing.T) []byte {
	t.Helper()
	var capture bytes.Buffer
	writer := pcapgo.NewWriter(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	now := time.Unix(1790000000, 0)
	write := func(reverse bool, transport gopacket.SerializableLayer, payload []byte) {
		source, destination := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		if reverse {
			source, destination = destination, source
		}
		ethernet := &layers.Ethernet{SrcMAC: []byte{2, 0, 0, 0, 0, 1}, DstMAC: []byte{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: source, DstIP: destination}
		switch tr := transport.(type) {
		case *layers.TCP:
			ip.Protocol = layers.IPProtocolTCP
			require.NoError(t, tr.SetNetworkLayerForChecksum(ip))
		case *layers.UDP:
			ip.Protocol = layers.IPProtocolUDP
			require.NoError(t, tr.SetNetworkLayerForChecksum(ip))
		}
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ip, transport, gopacket.Payload(payload)))
		wire := buffer.Bytes()
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: now, CaptureLength: len(wire), Length: len(wire)}, wire))
		now = now.Add(time.Millisecond)
	}
	tcp := func(reverse bool, seq, ack uint32, syn bool, payload []byte) {
		source, destination := layers.TCPPort(49152), layers.TCPPort(554)
		if reverse {
			source, destination = destination, source
		}
		write(reverse, &layers.TCP{SrcPort: source, DstPort: destination, Seq: seq, Ack: ack, SYN: syn, ACK: ack != 0, PSH: len(payload) != 0, Window: 65535}, payload)
	}
	request := []byte("SETUP rtsp://mvp.invalid/live RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/UDP;unicast;client_port=5000-5001\r\n\r\n")
	response := []byte("RTSP/1.0 200 OK\r\nCSeq: 1\r\nSession: mvp-camera\r\nTransport: RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001\r\n\r\n")
	tcp(false, 1000, 0, true, nil)
	tcp(true, 2000, 1001, true, nil)
	tcp(false, 1001, 2001, false, nil)
	tcp(false, 1001, 2001, false, request)
	tcp(true, 2001, 1001+uint32(len(request)), false, response)
	write(true, &layers.UDP{SrcPort: 6000, DstPort: 5000}, rtspUDPTestRTP(96, 1, 0x12345678))
	write(true, &layers.UDP{SrcPort: 6001, DstPort: 5001}, rtspUDPTestRTCP())
	return capture.Bytes()
}

func TestRTSPSetupPrecedesUDPInCaptureOrder(t *testing.T) {
	capture := mediaSetupUDPCapture(t)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observer := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%v/observer%v", workers, deferred, observer), func(t *testing.T) {
					var mu sync.Mutex
					var events []*ProtocolEvent
					var stats ProtocolStats
					options := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
					if observer {
						options = append(options, WithEveryPacket(func(gopacket.Packet) {}))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(capture), options...))
					require.Len(t, events, 4)
					var mediaCount int
					for _, e := range events {
						if e.Transport != "udp" {
							continue
						}
						mediaCount++
						require.Equal(t, "rtp", e.Protocol, "TCP SETUP must be visible to a subsequently captured UDP packet")
						fields, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, "matched-rtsp-transport", fields["Association Status"])
						require.Equal(t, "mvp-camera", fields["Associated RTSP Session ID"])
						require.Equal(t, []PacketReference{{Number: 4}, {Number: 5}}, fields["RTSP SETUP Packet References"])
					}
					require.Equal(t, 2, mediaCount)
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
}

// Both field answers and wire order come from the independent synthetic
// carrier. Event IDs are checked as links to the actual SETUP events instead
// of hard-coding allocation order across worker configurations.
func assertMediaMVPEvents(t testing.TB, raw json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var answer struct {
		Messages []struct {
			Protocol string         `json:"protocol"`
			Packet   uint64         `json:"packet"`
			Raw      string         `json:"raw_hex"`
			Fields   map[string]any `json:"exact_session_fields"`
		} `json:"messages"`
		RequestPacket  uint64 `json:"rtsp_setup_request_packet"`
		ResponsePacket uint64 `json:"rtsp_setup_response_packet"`
	}
	require.NoError(t, json.Unmarshal(raw, &answer))
	ordered := append([]*ProtocolEvent(nil), events...)
	packetNumber := func(e *ProtocolEvent) uint64 {
		require.NotEmpty(t, e.SourceBytes.PacketRefs)
		return e.SourceBytes.PacketRefs[0].Number
	}
	sort.Slice(ordered, func(i, j int) bool { return packetNumber(ordered[i]) < packetNumber(ordered[j]) })
	require.Len(t, ordered, len(answer.Messages))
	byPacket := map[uint64]*ProtocolEvent{}
	for _, e := range ordered {
		byPacket[packetNumber(e)] = e
	}
	request, response := byPacket[answer.RequestPacket], byPacket[answer.ResponsePacket]
	require.NotNil(t, request)
	require.NotNil(t, response)
	for i, e := range ordered {
		want := answer.Messages[i]
		require.Equal(t, want.Packet, packetNumber(e), "independent capture source order")
		require.Equal(t, want.Protocol, e.Protocol)
		require.Equal(t, want.Raw, hex.EncodeToString(e.Raw))
		if want.Protocol == "" {
			require.Equal(t, "unrecognized", e.Status, "later SETUP must not classify an earlier UDP packet")
			require.Empty(t, e.Session)
			continue
		}
		require.Contains(t, []string{"decoded", "deferred"}, e.Status)
		fields, err := e.GetFields()
		require.NoError(t, err)
		owned := cloneSession(e.Session)
		if e == response {
			require.Equal(t, []uint64{request.ID}, owned["RTSP SETUP Request Event IDs"])
			delete(owned, "RTSP SETUP Request Event IDs")
		}
		if e.Protocol == "rtp" {
			require.Equal(t, []uint64{request.ID, response.ID}, owned["RTSP SETUP Event IDs"])
			delete(owned, "RTSP SETUP Event IDs")
			require.Equal(t, e.Session, fields, "full cache and deferred fields must use the final RTSP association")
			require.Zero(t, e.ResponseTo, "RTSP mapping provides context, not a media transaction")
		}
		actualJSON, err := json.Marshal(owned)
		require.NoError(t, err)
		expectedJSON, err := json.Marshal(want.Fields)
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), string(actualJSON), "all observed semantic fields from independent wire facts")
	}
}
