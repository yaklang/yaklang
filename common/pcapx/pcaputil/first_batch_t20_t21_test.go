package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

const t20CapturePath = "testdata/protocol-sessions/first-batch-oracles/sip-sipp-rtp-duplex-loopback.pcap"
const t20OraclePath = "testdata/protocol-sessions/first-batch-oracles/sip-sipp-rtp-tshark-oracle.tsv"
const t20CaptureSHA256 = "1e59eebe6462dbf469fa3145597bd9251c4b0447b756845efc10ef48e4104798"
const t20OracleSHA256 = "3b4862ccea0b25f62ec26515b4ae1d80b8698f49639d2dc2f1daebba6e542c6f"

func TestFirstBatchT20(t *testing.T) {
	t.Run("native-duplex-capture-and-independent-oracle", testT20NativeCapture)
	t.Run("upstream-stun-turn-udp-and-channel-context-oracle", testT20STUNTurnCapture)
	t.Run("nonstandard-udp-and-decode-as-context-boundary", testT20UDPAdmission)
	t.Run("sip-fork-dialog-and-ttl", testT20SIPState)
	t.Run("offerless-invite-sdp-awaits-ack-answer", TestProtocolSessionSIPOfferlessInviteNegotiatesOnlyAfterACK)
	t.Run("sip-tcp-content-length-all-splits", TestProtocolSessionSIPFragmentation)
	t.Run("sip-udp-content-length-datagram-boundary", testT20SIPDatagramLength)
	t.Run("sdp-answer-and-capture-domain-association", testT20SDPAssociationIntegrity)
	t.Run("rtcp-compound-bounds", testT20RTCP)
	t.Run("rtp-clock-and-rtcp-mux-collision", testT20RTPClockAndCollision)
	t.Run("rtp-sequence-wrap-jitter-ssrc-and-sr-rr", func(t *testing.T) {
		TestProtocolSessionRTPSequenceJitterAndRTCP(t)
		TestProtocolSessionRTPWrapMultipleSSRCAndSDPMap(t)
	})
	t.Run("rtsp-interleaved-carrier", testT20RTSPInterleaved)
	t.Run("rtsp-udp-media-carrier", testT20RTSPUDPTransport)
	t.Run("rtsp-negotiation-and-evidence-boundaries", func(t *testing.T) {
		TestT20RTSPInterleavedRequiresOfferedPair(t)
		TestT20RTSPInterleavedChannelReuseIsAtomic(t)
		TestT20RTSPUDPAssociationLinksRequestAndResponseEvidence(t)
	})
	t.Run("rtp-rtcp-resource-boundaries", func(t *testing.T) {
		TestProtocolSessionRTCPCollectionBudgets(t)
		TestProtocolSessionRTPInstalledReservationCallbackChargesSourceHistory(t)
		TestProtocolSessionRTPAddsSDPMapsWithinRetainedMemoryBudget(t)
	})
	t.Run("unknown-stun-method-is-not-turn", TestSTUNUnknownMethodIsNotMisclassifiedAsTURN)
}

func TestFirstBatchT20T21Manifest(t *testing.T) {
	const dir = "testdata/protocol-sessions/first-batch-oracles"
	wire, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Samples []struct {
			ID             string `json:"id"`
			Path           string `json:"path"`
			RepositoryPath string `json:"repository_path"`
			SHA256         string `json:"sha256"`
			SizeBytes      int    `json:"size_bytes"`
			Oracle         struct {
				Path      string `json:"path"`
				SHA256    string `json:"sha256"`
				SizeBytes int    `json:"size_bytes"`
			} `json:"oracle_file"`
			AuthOracle struct {
				Path      string `json:"path"`
				SHA256    string `json:"sha256"`
				SizeBytes int    `json:"size_bytes"`
			} `json:"auth_oracle_file"`
		} `json:"samples"`
		Oracles []struct {
			Tool  string   `json:"tool"`
			Files []string `json:"files"`
		} `json:"oracles"`
	}
	require.NoError(t, json.Unmarshal(wire, &manifest))
	require.Len(t, manifest.Samples, 11, "three SNMP captures plus SIP/RTP, SMB2, LDAP, Kerberos, and four DCE/RPC evidence samples")
	for _, sample := range manifest.Samples {
		path := filepath.Join(dir, sample.Path)
		if sample.RepositoryPath != "" {
			path = filepath.Join("..", "..", "..", sample.RepositoryPath)
		}
		capture, err := os.ReadFile(path)
		require.NoError(t, err, "%s", sample.ID)
		sum := sha256.Sum256(capture)
		require.Equal(t, sample.SHA256, hex.EncodeToString(sum[:]), "%s capture", sample.ID)
		require.Equal(t, sample.SizeBytes, len(capture), "%s capture", sample.ID)
		if sample.Oracle.Path != "" {
			oracle, err := os.ReadFile(filepath.Join(dir, sample.Oracle.Path))
			require.NoError(t, err, "%s oracle", sample.ID)
			oracleSum := sha256.Sum256(oracle)
			require.Equal(t, sample.Oracle.SHA256, hex.EncodeToString(oracleSum[:]), "%s oracle", sample.ID)
			require.Equal(t, sample.Oracle.SizeBytes, len(oracle), "%s oracle", sample.ID)
		}
		if sample.AuthOracle.Path != "" {
			oracle, err := os.ReadFile(filepath.Join(dir, sample.AuthOracle.Path))
			require.NoError(t, err, "%s authentication oracle", sample.ID)
			oracleSum := sha256.Sum256(oracle)
			require.Equal(t, sample.AuthOracle.SHA256, hex.EncodeToString(oracleSum[:]), "%s authentication oracle", sample.ID)
			require.Equal(t, sample.AuthOracle.SizeBytes, len(oracle), "%s authentication oracle", sample.ID)
		}
	}
	require.Len(t, manifest.Oracles, 1)
	require.Equal(t, "Wireshark TShark 4.4.8", manifest.Oracles[0].Tool)
	for _, name := range manifest.Oracles[0].Files {
		oracle, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err, "%s", name)
		require.NotEmpty(t, oracle)
	}
}

func testT20STUNTurnCapture(t *testing.T) {
	const captureHash = "023e94f28a34cc760a440c4db9c81a40f61a0b216372ac67afb267b565e9a606"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/stun-ndpi-tshark-oracle.tsv"
	const oracleHash = "7dab29b2362e4c9d6ee4dda37699f397f5b6904dd470e707755db7cb4805629e"
	wire := binCorpusBytes(t, "ndpi/ndpi-stun.pcap")
	sum := sha256.Sum256(wire)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 201, t21PcapRecords(t, wire))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "frame.protocols", "udp.srcport", "udp.dstport", "stun.type", "stun.type.method", "stun.type.class", "stun.channel", "stun.length"}, oracle[0])
	require.Len(t, oracle, 202)
	want := make(map[uint64][]string)
	for _, row := range oracle[1:] {
		if !strings.Contains(row[1], ":udp:stun") || strings.Contains(row[1], ":icmp:") {
			continue
		}
		frame, err := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, err)
		want[frame] = row
	}
	require.Greater(t, len(want), 100, "independent TShark oracle must cover real UDP STUN and TURN traffic")

	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			events, _, err := binReplay(t, wire, workers, WithBinParserDeferred(deferred))
			require.NoError(t, err)
			got := make(map[uint64]*BinParserEvent)
			for _, event := range events {
				if event.Transport != "udp" || event.Protocol != "stun" && event.Protocol != "turn" || len(event.SourceBytes.PacketRefs) != 1 {
					continue
				}
				frame := event.SourceBytes.PacketRefs[0].Number
				if _, ok := want[frame]; !ok {
					continue
				}
				require.Nil(t, got[frame], "one UDP datagram may yield only one TURN/STUN event")
				got[frame] = event
			}
			require.Len(t, got, len(want), "native UDP session path must cover every TShark STUN/TURN datagram")
			for frame, row := range want {
				event := got[frame]
				require.NotNil(t, event)
				if event.Status == "decoded" || event.Status == "deferred" {
					require.Equal(t, deferred, event.Status == "deferred", "worker=%d frame=%d status=%s", workers, frame, event.Status)
				}
				require.Contains(t, []string{"decoded", "deferred", "malformed", "context-required"}, event.Status)
				fields, err := event.GetFields()
				if event.Status == "decoded" || event.Status == "deferred" || event.Status == "context-required" {
					require.NoError(t, err)
					require.NotEmpty(t, fields)
					if event.Protocol == "turn" {
						require.Equal(t, "turn-rfc8656-udp", event.Profile)
					} else {
						require.Equal(t, "stun-rfc8489-udp", event.Profile)
					}
				} else if event.Status == "malformed" {
					require.Contains(t, event.Error, "fingerprint mismatch")
					continue
				} else {
					require.Equal(t, "context-required", event.Status)
				}
				if row[4] == "" && row[7] != "" {
					wantChannel, parseErr := strconv.ParseUint(row[7], 0, 16)
					require.NoError(t, parseErr)
					require.Equal(t, "turn", event.Protocol)
					require.Equal(t, "ChannelData", event.Session["Packet Name"])
					require.Equal(t, uint16(wantChannel), event.Session["Channel Number"])
					if event.Status == "context-required" {
						require.Equal(t, "channel binding was not observed", event.Session["Context Missing"])
					} else {
						require.Equal(t, true, event.Session["Matched"])
						require.NotEmpty(t, event.Session["Peer Address"])
					}
				} else if row[4] != "" {
					wireType, parseErr := strconv.ParseUint(row[4], 0, 16)
					require.NoError(t, parseErr)
					class := uint8((wireType>>4)&1 | (wireType>>7)&2)
					require.Equal(t, uint16(class), event.Session["Class"], "TShark's independent wire type/class at frame %d", frame)
					methodText := strings.TrimPrefix(row[5], "0x")
					method, parseErr := strconv.ParseUint(methodText, 16, 16)
					require.NoError(t, parseErr)
					require.Equal(t, stunName(uint16(method)), event.Session["Packet Name"])
				}
			}
		}
	}
}

func t20UDPCapture(t *testing.T, srcIP, dstIP net.IP, srcPort, dstPort uint16, messages ...[]byte) []byte {
	t.Helper()
	var capture bytes.Buffer
	writer := pcapgo.NewWriter(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, message := range messages {
		packetSrcIP, packetDstIP := srcIP, dstIP
		packetSrcPort, packetDstPort := srcPort, dstPort
		if i%2 == 1 {
			packetSrcIP, packetDstIP = packetDstIP, packetSrcIP
			packetSrcPort, packetDstPort = packetDstPort, packetSrcPort
		}
		eth := &layers.Ethernet{SrcMAC: []byte{2, 0, 0, 0, 0, 1}, DstMAC: []byte{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: packetSrcIP.To4(), DstIP: packetDstIP.To4()}
		udp := &layers.UDP{SrcPort: layers.UDPPort(packetSrcPort), DstPort: layers.UDPPort(packetDstPort)}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(message)))
		raw := buffer.Bytes()
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(1_700_000_000, int64(i)), CaptureLength: len(raw), Length: len(raw)}
		require.NoError(t, writer.WritePacket(ci, raw))
	}
	return capture.Bytes()
}

func testT20UDPAdmission(t *testing.T) {
	const port = 49_382
	request := stunTestMessage(1, 0, 41)
	response := stunTestMessage(1, 2, 41)
	capture := t20UDPCapture(t, net.ParseIP("198.51.100.11"), net.ParseIP("203.0.113.19"), 41_707, port, request, response)
	for _, explicit := range []bool{false, true} {
		for _, deferred := range []bool{false, true} {
			options := []CaptureOption{WithBinParserDeferred(deferred)}
			if explicit {
				options = append(options, WithProtocolDecodeAs("udp", port, "stun"))
			}
			events, _, err := binReplay(t, capture, 1, options...)
			require.NoError(t, err)
			require.Len(t, events, 2)
			for _, event := range events {
				require.Equal(t, "stun", event.Protocol)
				require.Equal(t, "stun-rfc8489-udp", event.Profile)
				require.Equal(t, "message", event.Completeness)
				require.Equal(t, "udp", event.Transport)
				require.Equal(t, deferred, event.Status == "deferred")
				require.Equal(t, explicit, event.Admission == "explicit-decode-as")
			}
			require.Equal(t, true, events[1].Session["Matched"])
		}
	}

	channelOnly := t20UDPCapture(t, net.ParseIP("198.51.100.11"), net.ParseIP("203.0.113.19"), 41_707, port, []byte{0x40, 0x01, 0, 1, 42})
	events, stats, err := binReplay(t, channelOnly, 1, WithProtocolDecodeAs("udp", port, "turn"))
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "turn", events[0].Protocol)
	require.Equal(t, "turn-rfc8656-udp", events[0].Profile)
	require.Equal(t, "context-required", events[0].Status)
	require.Equal(t, "ChannelData", events[0].Session["Packet Name"])
	require.Equal(t, "channel binding was not observed", events[0].Session["Context Missing"])
	require.EqualValues(t, 1, stats.ContextRequired)

	coap := []byte{0x40, 0x01, 0x00, 0x01} // valid CoAP v1 GET, code and message id
	coapCapture := t20UDPCapture(t, net.ParseIP("198.51.100.11"), net.ParseIP("203.0.113.19"), 41_707, port, coap)
	coapEvents, _, err := binReplay(t, coapCapture, 1)
	require.NoError(t, err)
	require.Len(t, coapEvents, 1)
	require.Equal(t, "coap", coapEvents[0].Protocol, "0x40 CoAP version bits alone are not TURN ChannelData evidence")
}

func testT20SIPDatagramLength(t *testing.T) {
	const port = 5060
	header := "OPTIONS sip:b@example.test SIP/2.0\r\nVia: SIP/2.0/UDP a.example.test;branch=z9hG4bK-udp-cl\r\n" +
		"From: <sip:a@example.test>;tag=a\r\nTo: <sip:b@example.test>\r\nCall-ID: udp-cl@example.test\r\nCSeq: 1 OPTIONS\r\n"
	validEmpty := []byte(header + "Content-Length: 0\r\n\r\n")
	validBody := []byte(header + "Content-Length: 3\r\n\r\nabc")
	shortBody := []byte(header + "Content-Length: 4\r\n\r\nabc")
	longBody := []byte(header + "Content-Length: 2\r\n\r\nabc")
	missingLength := []byte(strings.TrimSuffix(header, "CSeq: 1 OPTIONS\r\n") + "CSeq: 1 OPTIONS\r\n\r\nbody")
	conflicting := []byte(header + "Content-Length: 3\r\nl: 2\r\n\r\nabc")
	capture := t20UDPCapture(t, net.ParseIP("198.51.100.11"), net.ParseIP("203.0.113.19"), port, 41_707, validEmpty, validBody, shortBody, longBody, missingLength, conflicting)
	events, stats, err := binReplay(t, capture, 1)
	require.NoError(t, err)
	require.Len(t, events, 6)
	for i, event := range events {
		if i < 2 {
			require.Equal(t, "decoded", event.Status)
			require.Equal(t, "sip", event.Protocol)
			continue
		}
		require.Equal(t, "malformed", event.Status, "datagram %d with conflicting/mismatched body length", i)
		if i == 5 {
			require.Contains(t, event.Error, "header", "conflicting compact/full length fields are rejected during header validation")
		} else {
			require.Contains(t, event.Error, "Content-Length")
		}
	}
	require.EqualValues(t, 4, stats.Malformed)
}

func testT20SDPAssociationIntegrity(t *testing.T) {
	budget := DefaultParserBudget()
	parser := &binParser{budget: budget, config: BinParserConfig{MaxMessageBytes: budget.MaxMessageBytes, MaxBufferedBytes: budget.MaxBufferedBytes}}
	body := []byte("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 40000 RTP/AVP 0\r\n")
	sdp, err := sipParseSDP(body, 16)
	require.NoError(t, err)
	response := &ProtocolEvent{ID: 1, Protocol: "sip", Domain: CaptureDomain{Interface: 0}, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 1}}}, Session: map[string]any{
		"Packet Name": "Response", "Association Status": "missing-request", "Call-ID": "same-call", "From Tag": "from", "To Tag": "to", "SDP": sdp,
	}}
	parser.observeSIPMedia(response)
	require.Empty(t, parser.mediaEntries, "an unmatched SDP response must not create a strong RTP mapping")

	offerA := &ProtocolEvent{ID: 2, Protocol: "sip", Domain: CaptureDomain{Interface: 0}, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 2}}}, Timestamp: time.Unix(3, 0), Session: map[string]any{
		"Packet Name": "INVITE", "Method": "INVITE", "Call-ID": "same-call", "From Tag": "from", "To Tag": "", "SDP": sdp,
	}}
	offerB := &ProtocolEvent{ID: 3, Protocol: "sip", Domain: CaptureDomain{Interface: 1}, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 3}}}, Timestamp: time.Unix(3, 0), Session: map[string]any{
		"Packet Name": "INVITE", "Method": "INVITE", "Call-ID": "same-call", "From Tag": "from", "To Tag": "", "SDP": sdp,
	}}
	parser.observeSIPMedia(offerA)
	parser.observeSIPMedia(offerB)
	require.Len(t, parser.mediaEntries, 2, "identical signaling tuples in distinct capture domains remain separate")
	a := parser.matchSDPMedia(offerA.Domain, "192.0.2.1:6000", "127.0.0.1:40000", 0, false, time.Unix(4, 0))
	b := parser.matchSDPMedia(offerB.Domain, "192.0.2.1:6000", "127.0.0.1:40000", 0, false, time.Unix(4, 0))
	require.Equal(t, "observed-offer", a.status)
	require.Equal(t, []uint64{2}, a.eventIDs)
	require.Equal(t, []PacketReference{{Number: 2}}, a.packetRefs)
	require.Equal(t, "observed-offer", b.status)
	require.Equal(t, []uint64{3}, b.eventIDs)
	require.Equal(t, []PacketReference{{Number: 3}}, b.packetRefs)

	huge := []byte("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 40000 RTP/AVP " + strings.Repeat("0 ", 100) + "\r\n")
	_, err = sipParseSDP(huge, 4)
	require.Error(t, err)
	perr, ok := err.(*ProtocolError)
	require.True(t, ok)
	require.Equal(t, ErrResourceExceeded, perr.Kind)
	tooManyLines := []byte("v=0\r\n" + strings.Repeat("s=x\r\n", 5))
	_, err = sipParseSDP(tooManyLines, 4)
	require.Error(t, err)
	perr, ok = err.(*ProtocolError)
	require.True(t, ok)
	require.Equal(t, ErrResourceExceeded, perr.Kind)
	parser.closeMediaAssociations()
	require.Zero(t, parser.buffered.Load())
}

func testT20NativeCapture(t *testing.T) {
	pcap, err := os.ReadFile(t20CapturePath)
	require.NoError(t, err)
	sum := sha256.Sum256(pcap)
	require.Equal(t, t20CaptureSHA256, hex.EncodeToString(sum[:]))

	oracleRaw, err := os.ReadFile(t20OraclePath)
	require.NoError(t, err)
	oracleHash := sha256.Sum256(oracleRaw)
	require.Equal(t, t20OracleSHA256, hex.EncodeToString(oracleHash[:]))
	rows := strings.Split(strings.TrimSuffix(string(oracleRaw), "\n"), "\n")
	require.Len(t, rows, 131, "header plus 130 independent TShark frame rows")
	require.Equal(t, "frame.number\t_ws.col.protocol\tframe.time_epoch\tip.src\tudp.srcport\tip.dst\tudp.dstport\tsip.Method\tsip.Status-Code\tsip.Call-ID\tsdp.connection_info.address\tsdp.media.port\trtp.p_type\trtp.seq\trtp.timestamp\trtp.ssrc\tudp.length\trtp.payload", rows[0])
	sipOracle, rtpOracle := 0, 0
	sipOracleByFrame := make(map[uint64][]string)
	rtpOracleByFrame := make(map[uint64][]string)
	var sipMethods, sipStatuses []string
	captureCallID := ""
	clientMediaEndpoint, serverMediaEndpoint := "", ""
	for _, row := range rows[1:] {
		fields := strings.Split(row, "\t")
		require.Len(t, fields, 18)
		if fields[7] != "" || fields[8] != "" {
			sipOracle++
			frame, parseErr := strconv.ParseUint(fields[0], 10, 64)
			require.NoError(t, parseErr)
			require.Contains(t, []string{"SIP", "SIP/SDP"}, fields[1])
			require.NotContains(t, sipOracleByFrame, frame, "one SIP message per oracle frame")
			sipOracleByFrame[frame] = fields
			if fields[7] != "" {
				require.Empty(t, fields[8], "request rows carry a method, not a response status")
				sipMethods = append(sipMethods, fields[7])
				if fields[7] == "INVITE" {
					captureCallID = fields[9]
					clientMediaEndpoint = net.JoinHostPort(fields[10], fields[11])
				}
			} else {
				require.Empty(t, fields[7], "response rows carry a status, not a request method")
				sipStatuses = append(sipStatuses, fields[8])
				if fields[8] == "200" {
					serverMediaEndpoint = net.JoinHostPort(fields[10], fields[11])
				}
			}
		}
		if fields[12] != "" {
			rtpOracle++
			frame, parseErr := strconv.ParseUint(fields[0], 10, 64)
			require.NoError(t, parseErr)
			require.Equal(t, "RTP", fields[1])
			require.NotContains(t, rtpOracleByFrame, frame, "one RTP datagram per oracle frame")
			rtpOracleByFrame[frame] = fields
			udpLength, parseErr := strconv.Atoi(fields[16])
			require.NoError(t, parseErr)
			rtpPayload, decodeErr := hex.DecodeString(fields[17])
			require.NoError(t, decodeErr, "TShark RTP payload at frame %d", frame)
			require.Equal(t, 160, len(rtpPayload), "independent RTP payload length at frame %d", frame)
			require.Equal(t, 180, udpLength, "UDP header + RTP header + media payload at frame %d", frame)
			require.Equal(t, bytes.Repeat([]byte{0xff}, 160), rtpPayload, "fixed test PCMU payload at frame %d", frame)
		}
	}
	require.Equal(t, 4, sipOracle)
	require.Equal(t, []string{"INVITE", "ACK"}, sipMethods, "the live capture contains these two SIP requests")
	require.Equal(t, []string{"180", "200"}, sipStatuses, "the live capture contains these two SIP responses")
	require.Equal(t, "127.0.0.1:32300", clientMediaEndpoint, "captured INVITE SDP endpoint from TShark")
	require.Equal(t, "127.0.0.1:32200", serverMediaEndpoint, "captured 200 SDP endpoint from TShark")
	require.Equal(t, 126, rtpOracle)

	reader, err := NewCaptureReader(bytes.NewReader(pcap))
	require.NoError(t, err)
	records := 0
	for {
		_, _, err := reader.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		records++
	}
	require.Equal(t, 130, records)

	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers-%d/deferred-%t", workers, deferred), func(t *testing.T) {
				events, stats, err := binReplay(t, pcap, workers, WithBinParserDeferred(deferred))
				require.NoError(t, err)
				counts := map[string]int{}
				sipByRef := map[uint64]*ProtocolEvent{}
				sipEventIDByRef := map[uint64]uint64{}
				rtpByDirection := [2]int{}
				payloadBytes := 0
				for _, event := range events {
					if event.Protocol == "sip" && len(event.SourceBytes.PacketRefs) == 1 {
						frame := event.SourceBytes.PacketRefs[0].Number
						require.Nil(t, sipByRef[frame], "one parser SIP event per captured message frame")
						sipByRef[frame] = event
						sipEventIDByRef[frame] = event.ID
					}
				}
				for _, event := range events {
					if event.Protocol != "sip" && event.Protocol != "rtp" {
						continue
					}
					wantStatus := "decoded"
					if deferred {
						wantStatus = "deferred"
					}
					require.Equal(t, wantStatus, event.Status, "%s: %s", event.Protocol, event.Error)
					fields, err := event.GetFields()
					require.NoError(t, err)
					require.NotEmpty(t, fields)
					require.Equal(t, "message", event.Completeness)
					require.Len(t, event.SourceBytes.PacketRefs, 1)
					require.NotZero(t, event.SourceBytes.PacketRefs[0].Number)
					counts[event.Protocol]++
					if event.Protocol == "sip" {
						frame := event.SourceBytes.PacketRefs[0].Number
						oracleRow, ok := sipOracleByFrame[frame]
						require.True(t, ok, "SIP parser event must correspond to a TShark message frame")
						require.Equal(t, oracleRow[9], event.Session["Call-ID"], "TShark Call-ID at frame %d", frame)
						require.Equal(t, net.JoinHostPort(oracleRow[3], oracleRow[4]), event.Source, "TShark SIP source tuple at frame %d", frame)
						require.Equal(t, net.JoinHostPort(oracleRow[5], oracleRow[6]), event.Destination, "TShark SIP destination tuple at frame %d", frame)
						if oracleRow[7] != "" {
							require.Equal(t, oracleRow[7], event.Session["Packet Name"], "TShark request method at frame %d", frame)
							require.Equal(t, oracleRow[7], event.Session["CSeq Method"], "TShark CSeq method at frame %d", frame)
							if oracleRow[7] == "INVITE" {
								require.Equal(t, uint32(1), event.Session["CSeq"])
							}
						} else {
							require.Equal(t, "Response", event.Session["Packet Name"])
							status, parseErr := strconv.Atoi(oracleRow[8])
							require.NoError(t, parseErr)
							require.Equal(t, int64(status), t21Int64(t, event.Session["Status"]), "TShark response status at frame %d", frame)
							require.Equal(t, "matched", event.Session["Association Status"])
						}
					} else {
						frame := event.SourceBytes.PacketRefs[0].Number
						oracleRow, ok := rtpOracleByFrame[frame]
						require.True(t, ok, "RTP parser event must correspond to an independently decoded TShark frame")
						require.Equal(t, net.JoinHostPort(oracleRow[3], oracleRow[4]), event.Source, "TShark RTP source tuple at frame %d", frame)
						require.Equal(t, net.JoinHostPort(oracleRow[5], oracleRow[6]), event.Destination, "TShark RTP destination tuple at frame %d", frame)
						payloadType, parseErr := strconv.ParseUint(oracleRow[12], 10, 8)
						require.NoError(t, parseErr)
						sequence, parseErr := strconv.ParseUint(oracleRow[13], 10, 16)
						require.NoError(t, parseErr)
						timestamp, parseErr := strconv.ParseUint(oracleRow[14], 10, 32)
						require.NoError(t, parseErr)
						ssrc, parseErr := strconv.ParseUint(strings.TrimPrefix(oracleRow[15], "0x"), 16, 32)
						require.NoError(t, parseErr)
						udpLength, parseErr := strconv.Atoi(oracleRow[16])
						require.NoError(t, parseErr)
						rtpPayload, decodeErr := hex.DecodeString(oracleRow[17])
						require.NoError(t, decodeErr)
						clientToServer := event.Source == clientMediaEndpoint && event.Destination == serverMediaEndpoint
						serverToClient := event.Source == serverMediaEndpoint && event.Destination == clientMediaEndpoint
						require.True(t, clientToServer || serverToClient, "TShark RTP datagram matches the negotiated offer/answer endpoints at frame %d", frame)
						require.Equal(t, uint8(payloadType), event.Session["Payload Type"], "TShark RTP payload type at frame %d", frame)
						require.Equal(t, uint16(sequence), event.Session["Sequence"], "TShark RTP sequence at frame %d", frame)
						require.Equal(t, uint32(timestamp), event.Session["Timestamp"], "TShark RTP timestamp at frame %d", frame)
						require.Equal(t, uint32(ssrc), event.Session["SSRC"], "TShark RTP SSRC at frame %d", frame)
						require.Equal(t, len(rtpPayload), event.Session["RTP Payload Length"], "TShark RTP payload length at frame %d", frame)
						require.Equal(t, 20+len(rtpPayload), udpLength, "UDP header + RTP header + media payload at frame %d", frame)
						rtpByDirection[event.Direction]++
						require.Equal(t, "matched", event.Session["Association Status"])
						require.Equal(t, captureCallID, event.Session["Associated Call-ID"], "RTP media association must resolve to the Call-ID independently observed on the SIP INVITE at frame %d", frame)
						require.Equal(t, "audio", event.Session["Associated Media Type"])
						require.Equal(t, uint8(0), event.Session["Payload Type"])
						require.Equal(t, "PCMU", event.Session["Payload Type Name"])
						require.Equal(t, 8000, event.Session["Clock Rate"])
						require.Equal(t, "observed SDP a=rtpmap", event.Session["Clock Rate Source"])
						require.Equal(t, 160, event.Session["RTP Payload Length"])
						payloadBytes += event.Session["RTP Payload Length"].(int)
						refs := event.Session["SDP Packet References"].([]PacketReference)
						require.Len(t, refs, 2, "offer and answer evidence must survive association")
						ids := event.Session["SDP Event IDs"].([]uint64)
						require.Len(t, ids, 2)
						for _, ref := range refs {
							require.Contains(t, []uint64{1, 3}, ref.Number)
							require.NotNil(t, sipByRef[ref.Number], "media evidence points to a SIP event")
							require.Contains(t, ids, sipEventIDByRef[ref.Number], "media event IDs must refer to the SIP evidence events")
						}
						endpoints := event.Session["SDP Media Endpoints"].([]string)
						require.Contains(t, endpoints, clientMediaEndpoint)
						require.Contains(t, endpoints, serverMediaEndpoint)
					}
				}
				require.Equal(t, map[string]int{"sip": 4, "rtp": 126}, counts)
				require.Len(t, sipByRef, len(sipOracleByFrame), "parser SIP frame coverage must match every independent TShark message row")
				require.Len(t, rtpOracleByFrame, counts["rtp"], "every captured RTP datagram is independently checked field by field")
				require.Equal(t, [2]int{63, 63}, rtpByDirection)
				require.Equal(t, 20160, payloadBytes, "only RTP payload bytes are counted, excluding the 12-byte header")
				require.EqualValues(t, 130, stats.Messages)
				require.Zero(t, stats.Malformed)
				require.Zero(t, stats.Incomplete)
				require.Zero(t, stats.Unknown)
				require.Zero(t, stats.BufferedBytes)
				if deferred {
					require.EqualValues(t, 130, stats.Deferred)
				} else {
					require.EqualValues(t, 130, stats.Decoded)
				}
			})
		}
	}
}

func testT20SIPState(t *testing.T) {
	const call = "Fork@Example.test"
	const fromA = "Alice <sip:a@example.test>;tag=A"
	const toB = "Bob <sip:b@example.test>;tag=B"
	const toC = "Bob <sip:c@example.test>;tag=C"
	const branch = "z9hG4bKinvite"
	now := time.Unix(100, 0)
	state := &binSIP{}
	invite := sipMsg("INVITE sip:b@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP a;branch=" + branch}, {"From", fromA}, {"To", "Bob <sip:b@example.test>"}, {"Call-ID", call}, {"CSeq", "7 INVITE"}}, "")
	info, err := state.consumeAt(invite, 16, now)
	require.NoError(t, err)
	require.Equal(t, "INVITE", info["Method"])
	for _, to := range []string{toB, toC} {
		resp := sipResp("180", "Ringing", branch, "7 INVITE", call, fromA, to)
		info, err = state.consumeAt(resp, 16, now.Add(time.Second))
		require.NoError(t, err)
		require.Equal(t, "INVITE", info["Matched Request"])
		final := sipResp("200", "OK", branch, "7 INVITE", call, fromA, to)
		info, err = state.consumeAt(final, 16, now.Add(2*time.Second))
		require.NoError(t, err)
		require.Equal(t, "confirmed", info["Dialog State"])
		ack := sipMsg("ACK sip:b@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP a;branch=z9hG4bKack" + to[len(to)-1:]}, {"From", fromA}, {"To", to}, {"Call-ID", call}, {"CSeq", "7 ACK"}}, "")
		info, err = state.consumeAt(ack, 16, now.Add(3*time.Second))
		require.NoError(t, err)
		require.Equal(t, "2xx-dialog", info["ACK For INVITE"])
		require.Equal(t, uint32(7), info["INVITE CSeq"])
	}
	// A BYE sent by the tagged peer reverses From/To. Canonical dialog keys
	// must still match the same observed pair and preserve termination state.
	bye := sipMsg("BYE sip:a@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP b;branch=z9hG4bKbye"}, {"From", "Bob <sip:b@example.test>;tag=B"}, {"To", "Alice <sip:a@example.test>;tag=A"}, {"Call-ID", call}, {"CSeq", "8 BYE"}}, "")
	info, err = state.consumeAt(bye, 16, now.Add(4*time.Second))
	require.NoError(t, err)
	require.Equal(t, "terminating", info["Dialog State"])

	// The transaction expires before the longer dialog TTL. A delayed 2xx ACK
	// remains correlatable from its dialog tags and INVITE CSeq.
	delayed := &binSIP{}
	require.NoError(t, func() error { _, e := delayed.consumeAt(invite, 16, now); return e }())
	_, err = delayed.consumeAt(sipResp("200", "OK", branch, "7 INVITE", call, fromA, toB), 16, now)
	require.NoError(t, err)
	info, err = delayed.consumeAt(sipMsg("ACK sip:b@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP a;branch=z9hG4bKack-late"}, {"From", fromA}, {"To", toB}, {"Call-ID", call}, {"CSeq", "7 ACK"}}, ""), 16, now.Add(sipTransactionStateTTL+time.Second))
	require.NoError(t, err)
	require.Equal(t, "2xx-dialog", info["ACK For INVITE"])
	_, err = delayed.consumeAt(bye, 16, now.Add(sipDialogStateTTL+time.Second))
	require.NoError(t, err)
	require.Empty(t, delayed.dialogs)

	// Call-ID octets are case-sensitive; a response with a folded identifier
	// cannot satisfy a different transaction.
	caseState := &binSIP{}
	caseInvite := bytes.Replace(invite, []byte(call), []byte("CaseSensitive@host"), 1)
	_, err = caseState.consumeAt(caseInvite, 8, now)
	require.NoError(t, err)
	caseResp := sipResp("200", "OK", branch, "7 INVITE", "casesensitive@host", fromA, toB)
	info, err = caseState.consumeAt(caseResp, 8, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, true, info["Unmatched"])

	budgetState := &binSIP{}
	_, err = budgetState.consumeAt(invite, 1, now)
	require.NoError(t, err)
	_, err = budgetState.consumeAt(sipResp("180", "Ringing", branch, "7 INVITE", call, fromA, toB), 1, now)
	require.NoError(t, err)
	_, err = budgetState.consumeAt(sipResp("180", "Ringing", branch, "7 INVITE", call, fromA, toC), 1, now)
	require.Error(t, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)

	cancelState := &binSIP{}
	_, err = cancelState.consumeAt(invite, 8, now)
	require.NoError(t, err)
	_, err = cancelState.consumeAt(sipResp("200", "OK", branch, "7 INVITE", call, fromA, toB), 8, now)
	require.NoError(t, err)
	cancel := sipMsg("CANCEL sip:b@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP a;branch=" + branch}, {"From", fromA}, {"To", "Bob <sip:b@example.test>"}, {"Call-ID", call}, {"CSeq", "7 CANCEL"}}, "")
	info, err = cancelState.consumeAt(cancel, 8, now)
	require.NoError(t, err)
	require.NotEqual(t, "INVITE", info["Cancels"], "CANCEL after 2xx cannot be attributed as cancelling the completed INVITE")
}

func testT20RTCP(t *testing.T) {
	ssrc := uint32(0x01020304)
	sr := rtcpSR(ssrc, 0x11121314, 0x21222324, 2, 320)
	sdes := t20SDES(ssrc, "host-a")
	bye := t20BYE(ssrc, "done")
	compound := append(append(bytes.Clone(sr), sdes...), bye...)
	rtp := &binRTP{sources: map[uint32]*rtpSource{}}
	fields, err := rtp.consumeRTCPCompound(compound, 8)
	require.NoError(t, err)
	require.Equal(t, "Compound", fields["Packet Name"])
	require.Equal(t, 3, fields["Packet Count"])
	packets := fields["Compound Packets"].([]map[string]any)
	require.Equal(t, "SR", packets[0]["Packet Name"])
	require.Equal(t, "SDES", packets[1]["Packet Name"])
	chunks := packets[1]["SDES Chunks"].([]map[string]any)
	require.Equal(t, ssrc, chunks[0]["SSRC"])
	items := chunks[0]["Items"].([]map[string]any)
	require.Equal(t, "CNAME", items[0]["Type Name"])
	require.Equal(t, []byte("host-a"), items[0]["Value"])
	require.Equal(t, "BYE", packets[2]["Packet Name"])
	require.Equal(t, []byte("done"), packets[2]["Reason"])

	badSR := append(bytes.Clone(sr), 0)
	badSDES := bytes.Clone(sdes[:len(sdes)-1])
	badBYE := bytes.Clone(bye)
	badBYE[8] = 0xff
	paddedNonfinal := bytes.Clone(sr)
	paddedNonfinal[0] |= 0x20
	for name, bad := range map[string][]byte{
		"sender-report-trailing-byte": badSR,
		"sdes-truncated-padding":      badSDES,
		"bye-reason-overrun":          badBYE,
		"padding-nonfinal":            append(append(paddedNonfinal, sdes...), bye...),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rtp.consumeRTCPCompound(bad, 8)
			require.Error(t, err)
		})
	}
	_, err = rtp.consumeRTCPCompound(compound, 2)
	require.Error(t, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
}

func t20SDES(ssrc uint32, cname string) []byte {
	body := make([]byte, 4, 4+2+len(cname)+4)
	binary.BigEndian.PutUint32(body, ssrc)
	body = append(body, 1, byte(len(cname)))
	body = append(body, cname...)
	body = append(body, 0)
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	wire := make([]byte, 4, 4+len(body))
	wire[0], wire[1] = 0x81, 202
	binary.BigEndian.PutUint16(wire[2:], uint16((4+len(body))/4-1))
	return append(wire, body...)
}

func t20BYE(ssrc uint32, reason string) []byte {
	body := make([]byte, 4, 4+1+len(reason)+3)
	binary.BigEndian.PutUint32(body, ssrc)
	body = append(body, byte(len(reason)))
	body = append(body, reason...)
	for (4+len(body))%4 != 0 {
		body = append(body, 0)
	}
	wire := make([]byte, 4, 4+len(body))
	wire[0], wire[1] = 0x81, 203
	binary.BigEndian.PutUint16(wire[2:], uint16((4+len(body))/4-1))
	return append(wire, body...)
}

func testT20RTPClockAndCollision(t *testing.T) {
	rtp := &binRTP{sources: map[uint32]*rtpSource{}}
	ssrc := uint32(0xaabbccdd)
	first, err := rtp.consumeRTP(rtpPkt(96, 10, 100, ssrc), time.Unix(1, 0), 8)
	require.NoError(t, err)
	require.Equal(t, "clock-rate-unknown", first["Timing Status"])
	require.Nil(t, first["Jitter"], "dynamic RTP clock rate must not be guessed")
	rtp.payload, rtp.clocks = map[uint8]string{96: "opus"}, map[uint8]int{96: 48000}
	second, err := rtp.consumeRTP(rtpPkt(96, 11, 580, ssrc), time.Unix(1, 20_000_000), 8)
	require.NoError(t, err)
	require.Equal(t, 48000, second["Clock Rate"])
	require.Equal(t, "observed SDP a=rtpmap", second["Clock Rate Source"])
	require.Equal(t, float64(0), second["Jitter"], "newly observed clock starts a fresh timing baseline")

	pt72 := rtpPkt(72, 5000, 10, 0x12345678)
	pt72[1] |= 0x80 // marker plus payload type 72 aliases RTCP SR's octet 200
	pt72 = append(pt72, bytes.Repeat([]byte{0xff}, 16)...)
	a := t20MediaParser("192.0.2.2:40000", "192.0.2.2:40001", 72)
	e := &ProtocolEvent{Source: "192.0.2.1:60000", Destination: "192.0.2.2:40000", Timestamp: time.Unix(2, 0)}
	require.True(t, a.decodeNativeDatagram(e, pt72, 60000, 40000))
	require.Equal(t, "RTP", e.Entry, "separately signaled RTP port resolves the payload-type collision")
	require.Equal(t, uint8(72), e.Session["Payload Type"])
	require.Equal(t, "collision-call", e.Session["Associated Call-ID"])
	a.closeUDPSessions()
	a.closeMediaAssociations()
	require.Zero(t, a.buffered.Load())

	// When the same tuple is explicitly muxed, the colliding octet is parsed as
	// RTCP, as required by the negotiated RTP/RTCP mux profile.
	mux := t20MediaParser("192.0.2.2:40000", "192.0.2.2:40000", 72)
	srLike := rtpPkt(72, 6, 0x01020304, 0x11121314)
	srLike[1] |= 0x80
	srLike = append(srLike, make([]byte, 16)...)
	e = &ProtocolEvent{Source: "192.0.2.1:60000", Destination: "192.0.2.2:40000", Timestamp: time.Unix(2, 0)}
	require.True(t, mux.decodeNativeDatagram(e, srLike, 60000, 40000))
	require.Equal(t, "RTCP", e.Entry)
	require.Equal(t, "SR", e.Session["Packet Name"])
	mux.closeUDPSessions()
	mux.closeMediaAssociations()
	require.Zero(t, mux.buffered.Load())
}

func t20MediaParser(rtpEndpoint, rtcpEndpoint string, payload uint8) *binParser {
	budget := DefaultParserBudget()
	association := &sipMediaAssociation{domain: CaptureDomain{}, endpoint: rtpEndpoint, rtcpEndpoint: rtcpEndpoint, callID: "collision-call", mediaType: "audio", proto: "RTP/AVP", payload: payload, encoding: "dynamic-test", clockRate: 48000, negotiated: true, eventIDs: []uint64{11}, packetRefs: []PacketReference{{Number: 11}}}
	return &binParser{
		budget:       budget,
		config:       BinParserConfig{MaxMessageBytes: budget.MaxMessageBytes, MaxBufferedBytes: budget.MaxBufferedBytes, Deferred: true},
		mediaIndex:   map[mediaEndpointKey][]*sipMediaAssociation{{domain: CaptureDomain{}, endpoint: rtpEndpoint}: {association}},
		mediaEntries: []*sipMediaAssociation{association},
	}
}

func testT20RTSPInterleaved(t *testing.T) {
	steps := []sessionStep{{0, []byte("OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n")}, {1, []byte("RTSP/1.0 200 OK\r\nCSeq: 1\r\n\r\n")}, {0, []byte("SETUP rtsp://example.test/a RTSP/1.0\r\nCSeq: 2\r\nTransport: RTP/AVP/TCP;interleaved=0-1\r\n\r\n")}, {1, []byte("RTSP/1.0 200 OK\r\nCSeq: 2\r\nSession: test-session\r\nTransport: RTP/AVP/TCP;interleaved=0-1\r\n\r\n")}}
	collision := rtpPkt(72, 5000, 100, 1234)
	collision[1] |= 0x80
	packet := append([]byte{'$', 0, byte(len(collision) >> 8), byte(len(collision))}, collision...)
	steps = append(steps, sessionStep{1, packet})
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("cut-%d/deferred-%t", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "rtsp", steps, chunk, deferred)
				require.Len(t, events, 5)
				assertSessionEvents(t, events, "rtsp", deferred)
				require.Equal(t, true, events[4].Session["Matched"])
				require.Equal(t, "test-session", events[4].Session["Session ID"])
				media := events[4].Session["Media"].(map[string]any)
				require.Equal(t, uint8(72), media["Payload Type"])
			})
		}
	}
}

func TestFirstBatchT21(t *testing.T) {
	t.Run("live-samba-openldap-captures-and-tshark-oracle", testT21NativeCaptures)
	t.Run("upstream-kerberos-udp-tcp-and-tshark-oracle", testT21NativeKerberosCapture)
	t.Run("dcerpc-corpus-boundaries-and-v5-bind-oracle", testT21DCERPCOracles)
	t.Run("dcerpc-generated-full-session-and-independent-oracle", testT21DCERPCGeneratedFullSessionOracle)
	t.Run("smb-session-tree-file-scope-and-async", testT21SMBScopeAndAsync)
	t.Run("smb-transform-boundary-and-ciphertext-truncation", TestProtocolSessionSMB2CompoundAndTransform)
	t.Run("ldap-search-progress-abandon-and-sasl-opacity", testT21LDAPStateAndSASL)
	t.Run("dcerpc-bind-call-correlation-and-opaque-stub", testT21DCERPC)
	t.Run("dcerpc-fault-fixed-body-truncation", TestProtocolSessionDCERPCFaultRequiresCompleteFixedBody)
}

func testT21NativeCaptures(t *testing.T) {
	t.Run("samba-smb2", func(t *testing.T) {
		testT21NativeSMBCapture(t,
			"testdata/protocol-sessions/first-batch-oracles/smb2-samba-live.pcap",
			"testdata/protocol-sessions/first-batch-oracles/smb2-samba-tshark-oracle.tsv",
			"647ee8d3b64a4e70d2b6964481c95fc26d17e8952944f1786b5005781ca1f503",
			"c21b4769902f1e7fcc9e0b0e1b3912138637dda2d1ef4232df3c0cb721343894")
	})
	t.Run("openldap-starttls", func(t *testing.T) {
		testT21NativeLDAPCapture(t,
			"testdata/protocol-sessions/first-batch-oracles/ldap-openldap-starttls-live.pcap",
			"testdata/protocol-sessions/first-batch-oracles/ldap-openldap-tshark-oracle.tsv",
			"37d18bd8d943f09c1bbc73b9b5e479996cf557703cb31903d94001c21b6c9065",
			"c0ab74e0499d8b1e29ae82af836cce58b8d8f68088dc1ba11b71f20fa885a96e")
	})
}

func testT21NativeKerberosCapture(t *testing.T) {
	const captureHash = "bccc9bf683c262c7dacc455c73133980db6233066f8f24305213da85103496c2"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/kerberos-ndpi-tshark-oracle.tsv"
	const oracleHash = "d3eb4c09a5ee0f30e6852afabf4e49893f57fe95ec59798ec710a63b9dde741b"
	pcap := binCorpusBytes(t, "ndpi/ndpi-kerberos-login.pcap")
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 39, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.Protocol", "kerberos.msg_type", "kerberos.realm", "kerberos.CNameString", "kerberos.SNameString", "kerberos.etype"}, oracle[0])
	require.Len(t, oracle, 27, "header plus independently decoded Kerberos frames")
	want := make(map[uint64][]string, len(oracle)-1)
	for _, row := range oracle[1:] {
		frame, err := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, err)
		require.Equal(t, "KRB5", row[1])
		want[frame] = row
	}
	require.Equal(t, 26, len(want))

	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			events, stats, err := binReplay(t, pcap, workers, WithBinParserDeferred(deferred))
			require.NoError(t, err)
			got := make(map[uint64]*BinParserEvent)
			for _, event := range events {
				if event.Protocol != "kerberos" {
					continue
				}
				require.Contains(t, []string{"decoded", "deferred"}, event.Status, "worker=%d: %s", workers, event.Error)
				require.Equal(t, deferred, event.Status == "deferred")
				for _, ref := range event.SourceBytes.PacketRefs {
					if _, ok := want[ref.Number]; !ok {
						continue
					}
					require.Nil(t, got[ref.Number], "one packet may not create duplicate Kerberos events")
					got[ref.Number] = event
				}
			}
			require.Len(t, got, len(want), "actual Kerberos UDP/TCP records must match TShark frame coverage")
			require.Zero(t, stats.BufferedBytes)
			if deferred {
				require.EqualValues(t, len(want), stats.Deferred)
			} else {
				require.EqualValues(t, len(want), stats.Decoded)
			}
			for frame, row := range want {
				event := got[frame]
				require.NotNil(t, event)
				require.Equal(t, "kerberos", event.Protocol)
				require.Equal(t, "envelope", event.Completeness)
				wantTypeText := strings.Split(row[2], ",")[0]
				wantType, parseErr := strconv.ParseInt(wantTypeText, 10, 64)
				require.NoError(t, parseErr)
				decoded, decodeErr := event.Decode()
				require.NoError(t, decodeErr)
				metadata, ok := decoded["metadata"].(map[string]any)
				require.True(t, ok, "Kerberos semantic metadata must remain available in full and deferred modes")
				require.Equal(t, wantType, t21Int64(t, metadata["Message Type"]))
				require.Equal(t, false, metadata["Decryption Performed"])
				require.Equal(t, false, metadata["Peer Identity Validated"])
				require.Equal(t, false, metadata["Message Exchange Validated"])
				require.Greater(t, t21Int64(t, metadata["Cipher Byte Count"]), int64(0))
				require.Equal(t, frame == 31 || frame == 35, metadata["TCP Record Wrapped"])
				metadataJSON, marshalErr := json.Marshal(metadata)
				require.NoError(t, marshalErr)
				metadataText := string(metadataJSON)
				for _, column := range []int{3, 4, 5} {
					for _, value := range strings.Split(row[column], ",") {
						if value != "" {
							require.Contains(t, metadataText, value, "TShark field %q should remain visibly unverified at frame %d", row[column], frame)
						}
					}
				}
				decodedEtypes := map[int64]bool{}
				if fields, ok := metadata["Decoded Fields"].([]map[string]any); ok {
					for _, field := range fields {
						if field["Field"] == "Encryption Type Encoding" {
							decodedEtypes[t21Int64(t, field["Value"])] = true
						}
					}
				}
				for _, value := range strings.Split(row[6], ",") {
					if value == "" {
						continue
					}
					etype, parseErr := strconv.ParseInt(value, 10, 64)
					require.NoError(t, parseErr)
					require.True(t, decodedEtypes[etype], "TShark etype %d must match a decoded encryption-type field at frame %d", etype, frame)
				}
			}
		}
	}
}

func t21Int64(t testing.TB, value any) int64 {
	t.Helper()
	switch n := value.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	default:
		t.Fatalf("expected numeric field, got %T (%v)", value, value)
		return 0
	}
}

func t21ReadOracle(t *testing.T, path, wantHash string) [][]string {
	t.Helper()
	wire, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(wire)
	require.Equal(t, wantHash, hex.EncodeToString(sum[:]))
	r := csv.NewReader(bytes.NewReader(wire))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows
}

func t21PcapRecords(t *testing.T, wire []byte) int {
	t.Helper()
	r, err := NewCaptureReader(bytes.NewReader(wire))
	require.NoError(t, err)
	n := 0
	for {
		_, _, err = r.ReadPacketData()
		if err == io.EOF {
			return n
		}
		require.NoError(t, err)
		n++
	}
}

func testT21DCERPCOracles(t *testing.T) {
	t.Run("upstream-ndpi-v4-pnio-cm-unsupported-by-v5-profile", testT21DCERPCUpstreamV4Boundary)
	t.Run("upstream-derived-v5-epm-bind", testT21DCERPCGeneratedBindOracle)
	t.Run("generated-v5-truncated-bindack-negative", testT21DCERPCV5TruncatedBindAckOracle)
}

func testT21DCERPCUpstreamV4Boundary(t *testing.T) {
	const captureHash = "6fd0ffc3562c4381126026f80388ae247ab1527e07ce83132e48445f2d52db04"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/dcerpc-ndpi-tshark-oracle.tsv"
	const oracleHash = "b948227b44489499e4e55a25b73441469e6798866e30ce7a72c45e856fccd8c3"
	pcap := binCorpusBytes(t, "ndpi/ndpi-dcerpc.pcap")
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 16, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.protocol", "dcerpc.ver", "dcerpc.pkt_type", "dcerpc.dg_if_id", "dcerpc.opnum", "dcerpc.dg_frag_len"}, oracle[0])
	require.Len(t, oracle, 17, "header plus every upstream DCE/RPC datagram")

	protocolSession, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { protocolSession.Close("FIN") })
	reader, err := NewCaptureReader(bytes.NewReader(pcap))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())
	for i, row := range oracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		require.Equal(t, uint64(i+1), frame)
		require.Equal(t, "PNIO-CM", row[1], "TShark identifies the v4 interface as Profinet IO connection-management")
		require.Equal(t, "4", row[2], "this upstream file is connectionless DCE/RPC v4, not connection-oriented v5")
		require.Contains(t, []string{"0", "2"}, row[3])
		require.NotEmpty(t, row[4])
		_, _, err := reader.ReadPacketData()
		require.NoError(t, err)
	}
	_, _, err = reader.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)

	// Probe each actual UDP payload through the public connection-oriented
	// session boundary. The v4 sample is useful protocol evidence, but must not
	// be reported as decoded by this deliberately v5-only T21 profile.
	reader, err = NewCaptureReader(bytes.NewReader(pcap))
	require.NoError(t, err)
	for i := 0; i < 16; i++ {
		packet, _, readErr := reader.ReadPacketData()
		require.NoError(t, readErr)
		decoded := gopacket.NewPacket(packet, reader.LinkType(), gopacket.Default)
		udpLayer := decoded.Layer(layers.LayerTypeUDP)
		require.NotNil(t, udpLayer, "upstream frame %d should contain the observed UDP carrier", i+1)
		udp := udpLayer.(*layers.UDP)
		require.NotEmpty(t, udp.Payload)
		require.Equal(t, ProbeReject, protocolSession.Probe(udp.Payload).Verdict, "upstream frame %d is outside the v5 profile", i+1)
	}
	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	for _, event := range events {
		require.NotEqual(t, "dcerpc", event.Protocol, "the current stream parser must not mislabel connectionless v4 as v5")
	}
}

func testT21DCERPCGeneratedBindOracle(t *testing.T) {
	const captureHash = "10136caf10d9c60d703c6b902683b8f6410439b60bd2d410f8a247cd1fa2098b"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/dcerpc-gen-msrpc-epm-tshark-oracle.tsv"
	const oracleHash = "78d1ab188b55aa326d75cab047f9d1a29f194d54c52e98c34020e9ccda6daadd"
	pcap := binCorpusBytes(t, "generated-local/gen-msrpc-epm.pcap")
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 4, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.protocol", "dcerpc.ver", "dcerpc.pkt_type", "dcerpc.cn_call_id", "dcerpc.cn_ctx_id", "dcerpc.cn_bind_to_uuid"}, oracle[0])
	require.Len(t, oracle, 2, "header plus the single EPM Bind PDU; this sample has no BindAck or procedure call")
	require.Equal(t, []string{"4", "DCERPC", "5", "11", "1", "0", "e1af8308-5d1f-11c9-91a4-08002b14a0fa"}, oracle[1])

	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	var bind *ProtocolEvent
	for _, event := range events {
		if event.Protocol == "dcerpc" && event.Length > 0 {
			require.Nil(t, bind, "one exact DCE/RPC PDU span per TShark frame; close summaries are not PDUs")
			bind = event
		}
	}
	require.NotNil(t, bind)
	require.Equal(t, "decoded", bind.Status)
	require.Equal(t, []PacketReference{{Number: 4}}, bind.SourceBytes.PacketRefs)
	require.Equal(t, "Bind", bind.Session["Packet Name"])
	require.Equal(t, "EPM", bind.Session["Interface"])
	require.Equal(t, uint32(1), bind.Session["Call ID"])
	require.Equal(t, uint16(0), bind.Session["Context ID"])
}

func t21DCERPCFullSessionSteps() []sessionStep {
	return []sessionStep{
		{0, dcerpcBind(dcerpcEPM, 0, 1)},
		{1, dcerpcBindAck(1)},
		{0, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4})},
		{1, dcerpcResponse(2, 0, []byte{9, 9})},
		{0, dcerpcBind(dcerpcSRVSVC, 1, 3)},
		{1, dcerpcBindAck(3)},
		{0, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0})},
		{1, dcerpcResponse(4, 1, nil)},
	}
}

func testT21DCERPCGeneratedFullSessionOracle(t *testing.T) {
	const capturePath = "testdata/protocol-sessions/first-batch-oracles/dcerpc-generated-full-session.pcap"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/dcerpc-generated-full-session-tshark-oracle.tsv"
	const captureHash = "b287eccc15c7501056c4ed87f652dff923ec9d7fa6670ba70b98241a800e9a75"
	const oracleHash = "755949e7a939c65f3e84f7eaa6fa60dc1b0117e00aa3d427e1beabfde16bcace"

	generated := sessionTestPCAP(t, t21DCERPCFullSessionSteps(), 13500, 0, false, false)
	if os.Getenv("YAK_UPDATE_T21_DCERPC_SESSION_PCAP") == "1" {
		require.NoError(t, os.WriteFile(capturePath, generated, 0644))
		return
	}
	pcap, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	require.Equal(t, generated, pcap, "checked-in full-session PCAP must match the deterministic synthetic generator")
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 14, t21PcapRecords(t, pcap))

	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.protocol", "dcerpc.ver", "dcerpc.pkt_type", "dcerpc.cn_frag_len", "dcerpc.cn_call_id", "dcerpc.cn_ctx_id", "dcerpc.opnum", "dcerpc.cn_num_ctx_items", "dcerpc.cn_num_results", "dcerpc.cn_ack_result", "dcerpc.cn_ack_trans_id", "dcerpc.cn_ack_trans_ver", "dcerpc.cn_bind_to_uuid"}, oracle[0])
	require.Len(t, oracle, 9, "header plus each of the eight DCE/RPC PDUs")
	want := make(map[uint64][]string, len(oracle)-1)
	for _, row := range oracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		require.Equal(t, "5", row[2], "the generated session uses connection-oriented DCE/RPC v5")
		want[frame] = row
	}
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("oracle-replay/workers-%d/deferred-%t", workers, deferred), func(t *testing.T) {
				testT21DCERPCFullSessionReplayMatrix(t, pcap, want, workers, deferred)
			})
		}
	}

	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	got := make(map[uint64]*ProtocolEvent, len(want))
	for _, event := range events {
		if event.Protocol != "dcerpc" || event.Length == 0 {
			continue
		}
		require.Len(t, event.SourceBytes.PacketRefs, 1, "each generated DCE/RPC PDU must map to one captured TCP frame")
		frame := event.SourceBytes.PacketRefs[0].Number
		if _, ok := want[frame]; !ok {
			continue
		}
		require.Nil(t, got[frame], "one parser PDU event per independent TShark frame")
		got[frame] = event
	}
	require.Equal(t, []uint64{4, 5, 6, 7, 8, 9, 10, 11}, sortedPacketFrames(got))

	uuidToInterface := map[string]string{
		"e1af8308-5d1f-11c9-91a4-08002b14a0fa": "EPM",
		"4b324fc8-1670-01d3-1278-5a47bf6ee188": "SRVSVC",
	}
	ackContext := map[string]uint16{"1": 0, "3": 1}
	for frame, row := range want {
		event := got[frame]
		require.NotNil(t, event, "TShark PDU frame %d must have one parser event", frame)
		require.Equal(t, "decoded", event.Status)
		require.Equal(t, []PacketReference{{Number: frame}}, event.SourceBytes.PacketRefs)
		require.Equal(t, int64(mustT21Int(t, row[5])), t21Int64(t, event.Session["Call ID"]))
		switch row[3] {
		case "11": // Bind
			interfaceName := uuidToInterface[row[13]]
			require.NotEmpty(t, interfaceName, "TShark Bind UUID at frame %d", frame)
			require.Equal(t, "DCERPC", row[1])
			require.Equal(t, "Bind", event.Session["Packet Name"])
			require.Equal(t, interfaceName, event.Session["Interface"])
			require.Equal(t, uint16(mustT21Int(t, row[6])), event.Session["Context ID"])
			require.Equal(t, "1", row[8], "TShark reports one proposed presentation context")
			require.Equal(t, "proposed", event.Session["Context State"])
		case "12": // BindAck
			require.Equal(t, "DCERPC", row[1])
			require.Equal(t, "BindAck", event.Session["Packet Name"])
			require.Equal(t, "Bind", event.Session["Matched Request"])
			require.Equal(t, "1", row[9], "TShark reports one BindAck result")
			require.Equal(t, "0", row[10], "TShark reports the result accepted")
			require.Equal(t, "8a885d04-1ceb-11c9-9fe8-08002b104860", row[11], "TShark identifies the accepted NDR transfer syntax")
			require.Equal(t, "2", row[12], "TShark reports transfer syntax version 2")
			require.Contains(t, ackContext, row[5])
			require.Equal(t, []uint16{ackContext[row[5]]}, event.Session["Accepted Context IDs"], "accepted context must match the corresponding TShark Bind Call ID")
		case "0": // Request
			interfaceName := row[1]
			require.Contains(t, []string{"EPM", "SRVSVC"}, interfaceName)
			require.Equal(t, interfaceName, event.Session["Interface"])
			require.Equal(t, uint16(mustT21Int(t, row[6])), event.Session["Context ID"])
			require.Equal(t, uint16(mustT21Int(t, row[7])), event.Session["OpNum"])
			if interfaceName == "EPM" {
				require.Equal(t, "EPM.ept_map", event.Session["Packet Name"])
			} else {
				require.Equal(t, "SRVSVC.NetrShareEnum", event.Session["Packet Name"])
			}
		case "2": // Response
			require.Contains(t, []string{"EPM", "SRVSVC"}, row[1])
			require.Equal(t, uint16(mustT21Int(t, row[6])), event.Session["Context ID"])
			require.Equal(t, "matched", event.Session["Association Status"])
			if row[1] == "EPM" {
				require.Equal(t, "EPM.ept_map", event.Session["Matched Request"])
			} else {
				require.Equal(t, "SRVSVC.NetrShareEnum", event.Session["Matched Request"])
			}
		default:
			t.Fatalf("unexpected TShark DCE/RPC ptype %q at frame %d", row[3], frame)
		}
	}
}

func testT21DCERPCV5TruncatedBindAckOracle(t *testing.T) {
	const capturePath = "testdata/protocol-sessions/dcerpc-epm-srvsvc.pcap"
	const captureHash = "c23f3bec06a192aafa7079ddfb38ba37510a14f33b562220a05100efdb3cdf75"
	const oraclePath = "testdata/protocol-sessions/first-batch-oracles/dcerpc-epm-srvsvc-tshark-oracle.tsv"
	const oracleHash = "eae744e6b7baef9b9557d3efe83b21f51f535ce3c18673d61c5366536fc9f614"
	pcap, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 14, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.protocol", "dcerpc.ver", "dcerpc.pkt_type", "dcerpc.cn_call_id", "dcerpc.cn_ctx_id", "dcerpc.opnum", "dcerpc.cn_bind_to_uuid"}, oracle[0])
	require.Len(t, oracle, 9, "header plus all eight v5 PDUs")
	want := make(map[uint64][]string, len(oracle)-1)
	for _, row := range oracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		require.Equal(t, "5", row[2])
		want[frame] = row
	}

	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	got := make(map[uint64]*ProtocolEvent, len(want))
	for _, event := range events {
		if event.Protocol != "dcerpc" || event.Length == 0 {
			continue
		}
		require.Len(t, event.SourceBytes.PacketRefs, 1, "each oracle PDU is carried by one captured TCP packet")
		frame := event.SourceBytes.PacketRefs[0].Number
		if _, ok := want[frame]; !ok {
			continue
		}
		require.Nil(t, got[frame], "one PDU event per TShark frame")
		got[frame] = event
	}
	// This is a deliberately conservative negative: TShark sees eight PDUs,
	// but the generated BindAck frames carry only ten body bytes. The parser
	// emits the Bind and the malformed BindAck, then stops the stream rather
	// than promoting unaccepted presentation contexts or parsing later calls.
	require.Equal(t, []uint64{4, 5}, sortedPacketFrames(got), "only exact PDU events before and at the malformed BindAck are accepted")

	for frame, row := range want {
		if frame > 5 {
			continue
		}
		event := got[frame]
		require.NotNil(t, event)
		ptype, parseErr := strconv.Atoi(row[3])
		require.NoError(t, parseErr)
		require.Equal(t, "dcerpc", event.Protocol)
		switch row[3] {
		case "11":
			require.Equal(t, "DCERPC", row[1])
			require.Equal(t, "decoded", event.Status)
			require.Equal(t, uint64(frame), event.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, int64(mustT21Int(t, row[4])), t21Int64(t, event.Session["Call ID"]))
			require.Equal(t, "Bind", event.Session["Packet Name"])
			uuidToInterface := map[string]string{
				"e1af8308-5d1f-11c9-91a4-08002b14a0fa": "EPM",
				"4b324fc8-1670-01d3-1278-5a47bf6ee188": "SRVSVC",
			}
			wantInterface := uuidToInterface[row[7]]
			require.NotEmpty(t, wantInterface)
			require.Equal(t, wantInterface, event.Session["Interface"], "TShark abstract syntax at frame %d", frame)
		case "12":
			require.Equal(t, "DCERPC", row[1])
			require.Equal(t, "malformed", event.Status, "this checked-in generated sample has only a 10-byte BindAck body")
			require.Contains(t, event.Error, "truncated bind acknowledgement")
		default:
			t.Fatalf("unexpected parser event for unsupported post-BindAck frame %d, TShark PType %d", frame, ptype)
		}
	}
	for frame, row := range want {
		if frame <= 5 {
			continue
		}
		require.Contains(t, []string{"0", "2", "11", "12"}, row[3], "remaining TShark rows are later calls or the second Bind exchange")
		require.NotContains(t, got, frame, "the truncated BindAck prevents this fixture from serving as a procedure-session positive")
	}
}

func sortedPacketFrames(events map[uint64]*ProtocolEvent) []uint64 {
	frames := make([]uint64, 0, len(events))
	for frame := range events {
		frames = append(frames, frame)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i] < frames[j] })
	return frames
}

func mustT21Int(t *testing.T, value string) int {
	t.Helper()
	n, err := strconv.Atoi(value)
	require.NoError(t, err)
	return n
}

func testT21NativeSMBCapture(t *testing.T, capturePath, oraclePath, captureHash, oracleHash string) {
	pcap, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 52, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "smb2.cmd", "smb2.msg_id", "smb2.sesid", "smb2.tid", "smb2.filename", "smb2.nt_status", "smb2.flags.response"}, oracle[0])
	require.Len(t, oracle, 45, "TShark independently decoded 44 SMB2 PDUs")
	const authOraclePath = "testdata/protocol-sessions/first-batch-oracles/smb2-samba-ntlmssp-tshark-oracle.tsv"
	const authOracleHash = "fcdc2f6edb484d66ead67aacecf00e97720ca2f6dbc412ba235d3d7b5c383196"
	authOracle := t21ReadOracle(t, authOraclePath, authOracleHash)
	require.Equal(t, []string{"frame.number", "gss-api.OID", "spnego.MechType", "spnego.supportedMech", "ntlmssp.messagetype", "ntlmssp.challenge.target_name", "ntlmssp.ntlmserverchallenge", "ntlmssp.auth.username", "ntlmssp.auth.domain", "smb2.nt_status", "smb2.ses_flags.guest"}, authOracle[0])
	require.Len(t, authOracle, 6, "the live guest capture contains independently decoded SPNEGO/NTLMSSP exchange frames")
	authByFrame := make(map[uint64][]string, len(authOracle)-1)
	for _, row := range authOracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		authByFrame[frame] = row
	}
	require.Len(t, authByFrame, 5)
	require.Equal(t, "1.3.6.1.5.5.2", authByFrame[8][1], "TShark observes SPNEGO on live SMB2 Session Setup")
	require.Equal(t, "1.3.6.1.4.1.311.2.2.10", authByFrame[8][2], "SPNEGO offers NTLMSSP")
	require.Equal(t, "0x00000001", authByFrame[8][4], "client NTLMSSP Type 1 is observed")
	require.Equal(t, "1.3.6.1.4.1.311.2.2.10", authByFrame[9][3], "server selects NTLMSSP")
	require.Equal(t, "0x00000002", authByFrame[9][4], "server NTLMSSP Type 2 is observed")
	require.Equal(t, "SMBLDAP", authByFrame[9][5], "the isolated Samba challenge target is visible")
	require.NotEmpty(t, authByFrame[9][6], "the challenge is observed but is not credential proof")
	require.Equal(t, "0x00000003", authByFrame[10][4], "client NTLMSSP Type 3 is observed")
	require.Equal(t, "root", authByFrame[10][7], "the Type 3 username is a client assertion only")
	require.Equal(t, "WORKGROUP", authByFrame[10][8])
	require.Equal(t, "0x00000000", authByFrame[11][9], "SMB reports STATUS_SUCCESS")
	require.Equal(t, "True", authByFrame[11][10], "the successful session is explicitly guest-mapped")
	want := make(map[uint64][]string, len(oracle)-1)
	for _, row := range oracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		want[frame] = row
	}
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			events, stats, err := binReplay(t, pcap, workers, WithBinParserDeferred(deferred))
			require.NoError(t, err)
			got := make(map[uint64]*ProtocolEvent)
			for _, e := range events {
				if e.Protocol != "smb2" {
					continue
				}
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				frame := e.SourceBytes.PacketRefs[0].Number
				row, ok := want[frame]
				require.True(t, ok, "SMB parser emitted ungrounded PDU at frame %d", frame)
				_, repeated := got[frame]
				require.False(t, repeated, "more than one SMB PDU from frame %d was flattened")
				got[frame] = e
				require.Equal(t, deferred, e.Status == "deferred", "worker=%d frame=%d status=%s", workers, frame, e.Status)
				require.Contains(t, []string{"decoded", "deferred"}, e.Status, "worker=%d frame=%d: %s", workers, frame, e.Error)
				messageID, parseErr := strconv.ParseUint(row[2], 10, 64)
				require.NoError(t, parseErr)
				require.Equal(t, messageID, e.Session["Message ID"])
				command, parseErr := strconv.ParseUint(row[1], 10, 16)
				require.NoError(t, parseErr)
				require.Equal(t, uint16(command), e.Session["Command"])
				sessionID, parseErr := strconv.ParseUint(row[3], 0, 64)
				require.NoError(t, parseErr)
				require.Equal(t, sessionID, e.Session["Session ID"])
				treeID, parseErr := strconv.ParseUint(row[4], 0, 32)
				require.NoError(t, parseErr)
				require.Equal(t, uint32(treeID), e.Session["Tree ID"])
				if row[5] != "" && command == 5 {
					// The current SMB profile exports CREATE paths. TShark can also
					// infer smb2.filename for QUERY_INFO from prior state (e.g. ".");
					// that contextual field is not yet projected by this parser.
					require.Equal(t, row[5], e.Session["File Name"], "TShark filename at frame %d must match the decoded CREATE/RENAME path", frame)
				}
				if row[6] != "" {
					status, parseErr := strconv.ParseUint(row[6], 0, 32)
					require.NoError(t, parseErr)
					require.Equal(t, uint32(status), e.Session["Status"])
				}
				response := row[7] == "True"
				require.Equal(t, response, e.Session["Response"])
			}
			require.Len(t, got, len(want), "SMB2 PDU coverage must match the independent TShark oracle")
			for frame := range authByFrame {
				event := got[frame]
				require.NotNil(t, event, "authentication oracle frame %d must map to a captured SMB2 PDU", frame)
				require.NotEqual(t, true, event.Session["Identity Verified"], "the guest capture cannot verify the claimed identity")
			}
			require.EqualValues(t, len(want), stats.Messages)
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.Incomplete)
			require.Zero(t, stats.BufferedBytes)
		}
	}
}

func testT21NativeLDAPCapture(t *testing.T, capturePath, oraclePath, captureHash, oracleHash string) {
	pcap, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	sum := sha256.Sum256(pcap)
	require.Equal(t, captureHash, hex.EncodeToString(sum[:]))
	require.Equal(t, 51, t21PcapRecords(t, pcap))
	oracle := t21ReadOracle(t, oraclePath, oracleHash)
	require.Equal(t, []string{"frame.number", "_ws.col.protocol", "ldap.messageID", "ldap.protocolOp", "ldap.resultCode", "ldap.baseObject", "tls.handshake.type"}, oracle[0])
	wantLDAP := make(map[uint64][]string)
	wantTLS := make(map[uint64]string)
	for _, row := range oracle[1:] {
		frame, parseErr := strconv.ParseUint(row[0], 10, 64)
		require.NoError(t, parseErr)
		if row[2] != "" {
			require.NotContains(t, wantLDAP, frame, "LDAP oracle must contain one row per frame")
			wantLDAP[frame] = row
		}
		if row[6] != "" {
			require.NotContains(t, wantTLS, frame, "TLS oracle must contain one row per frame")
			wantTLS[frame] = row[6]
		}
	}
	require.Len(t, wantLDAP, 9)
	require.Equal(t, map[uint64]string{28: "1", 29: "2"}, wantTLS)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			events, stats, err := binReplay(t, pcap, workers, WithBinParserDeferred(deferred))
			require.NoError(t, err)
			gotLDAP := make(map[uint64]*ProtocolEvent)
			gotTLS := make(map[uint64]bool)
			commandNames := map[string]string{"0": "BindRequest", "1": "BindResponse", "2": "UnbindRequest", "3": "SearchRequest", "4": "SearchResultEntry", "5": "SearchResultDone", "23": "ExtendedRequest", "24": "ExtendedResponse"}
			for _, e := range events {
				if e.Protocol == "ldap" {
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					frame := e.SourceBytes.PacketRefs[0].Number
					row, ok := wantLDAP[frame]
					require.True(t, ok, "LDAP parser emitted ungrounded event at frame %d", frame)
					require.NotContains(t, gotLDAP, frame, "LDAP parser emitted duplicate events for frame %d", frame)
					gotLDAP[frame] = e
					require.Equal(t, deferred, e.Status == "deferred", "worker=%d frame=%d status=%s", workers, frame, e.Status)
					messageID, parseErr := strconv.ParseUint(row[2], 10, 64)
					require.NoError(t, parseErr)
					require.Equal(t, messageID, e.Session["Message ID"])
					require.Equal(t, commandNames[row[3]], e.Session["Message Name"])
					if row[5] != "" {
						fields, fieldErr := e.GetFields()
						require.NoError(t, fieldErr)
						encoded, marshalErr := json.Marshal(fields)
						require.NoError(t, marshalErr)
						require.Contains(t, string(encoded), row[5], "LDAP base/object values should match the TShark oracle")
					}
					if row[4] != "" {
						result, parseErr := strconv.ParseUint(row[4], 10, 64)
						require.NoError(t, parseErr)
						require.Equal(t, result, e.Session["Result Code"])
					}
				}
				if e.Protocol == "tls" && len(e.SourceBytes.PacketRefs) > 0 {
					for _, ref := range e.SourceBytes.PacketRefs {
						if _, ok := wantTLS[ref.Number]; ok {
							require.NotContains(t, gotTLS, ref.Number, "TLS parser emitted duplicate events for frame %d", ref.Number)
							gotTLS[ref.Number] = true
						}
					}
				}
			}
			require.Len(t, gotLDAP, len(wantLDAP), "LDAP PDUs must match the independent TShark oracle")
			require.Equal(t, map[uint64]bool{28: true, 29: true}, gotTLS, "successful StartTLS must hand subsequent records to the TLS profile")
			require.GreaterOrEqual(t, stats.Messages, uint64(len(wantLDAP)))
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.Incomplete)
			require.Zero(t, stats.BufferedBytes)
		}
	}
}

func t21Feed(t *testing.T, s ProtocolSession, dir int, wire []byte) *ProtocolEvent {
	t.Helper()
	r := s.Feed(dir, time.Unix(10, 0), wire)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	return r.Events[0]
}

func t21SetSMBScope(wire []byte, sessionID uint64, treeID uint32) {
	binary.LittleEndian.PutUint32(wire[4+36:4+40], treeID)
	binary.LittleEndian.PutUint64(wire[4+40:4+48], sessionID)
}

func t21SMBControl(command uint16, response bool, messageID, sessionID uint64, treeID uint32) []byte {
	flags := uint32(0)
	if response {
		flags = smb2FlagResp
	}
	body := make([]byte, 4)
	binary.LittleEndian.PutUint16(body, 4)
	pdu := append(smb2Hdr(command, flags, messageID, sessionID, treeID), body...)
	return smb2TCP(pdu)
}

func testT21SMBScopeAndAsync(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	t21Feed(t, s, 0, smb2NegotiateReq(0x0311))
	t21Feed(t, s, 1, smb2NegotiateResp(0x0311))
	create := smb2Create(35, 0x11, 1, "compound.txt")
	closeCompound := smb2Close(36, 0x11, 1, bytes.Repeat([]byte{0xff}, 16))
	createPDU, closePDU := bytes.Clone(create[4:]), bytes.Clone(closeCompound[4:])
	for len(createPDU)%8 != 0 {
		createPDU = append(createPDU, 0)
	}
	binary.LittleEndian.PutUint32(createPDU[20:24], uint32(len(createPDU)))
	binary.LittleEndian.PutUint32(closePDU[16:20], smb2FlagRelated)
	binary.LittleEndian.PutUint32(closePDU[36:40], ^uint32(0))
	binary.LittleEndian.PutUint64(closePDU[40:48], ^uint64(0))
	compound := t21Feed(t, s, 0, smb2TCP(append(createPDU, closePDU...)))
	require.Equal(t, true, compound.Session["Compound"])
	require.Equal(t, 2, compound.Session["Compound Count"])
	compoundCommands := compound.Session["Commands"].([]map[string]any)
	require.Equal(t, []string{"CREATE", "CLOSE"}, compound.Session["Command Names"])
	require.Equal(t, true, compoundCommands[1]["Inherited Session Context"])
	require.Equal(t, true, compoundCommands[1]["Inherited Tree Context"])
	require.Equal(t, true, compoundCommands[1]["Related File ID Placeholder"])
	sharedID := []byte{1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 3, 6, 9, 12}

	for _, tc := range []struct {
		sid, mid uint64
		tid      uint32
		path     string
	}{{0x11, 40, 1, "alpha.txt"}, {0x22, 41, 2, "beta.txt"}} {
		t21Feed(t, s, 0, smb2Create(tc.mid, tc.sid, tc.tid, tc.path))
		t21Feed(t, s, 1, smb2CreateResp(tc.mid, tc.sid, tc.tid, sharedID))
	}
	flow := s.(*captureSession).f
	require.Len(t, flow.smb2.files, 2, "equal FileId values in distinct session/tree scopes stay separate")

	for _, tc := range []struct {
		sid  uint64
		tid  uint32
		want string
	}{{0x11, 1, "alpha.txt"}, {0x22, 2, "beta.txt"}} {
		read := smb2Read(55, tc.sid, tc.tid, sharedID, 128)
		e := t21Feed(t, s, 0, read)
		require.Equal(t, "observed-open", e.Session["File Context Status"])
		require.Equal(t, tc.want, e.Session["File Context"])
	}
	wrong := smb2ReadResp(55, 0x11, 2, []byte("wrong scope"))
	e := t21Feed(t, s, 1, wrong)
	require.Equal(t, "missing-or-ambiguous-request", e.Session["Association Status"])
	require.Equal(t, "partial", e.Session["Context Level"])
	for _, tc := range []struct {
		sid  uint64
		tid  uint32
		name string
	}{{0x11, 1, "alpha.txt"}, {0x22, 2, "beta.txt"}} {
		e = t21Feed(t, s, 1, smb2ReadResp(55, tc.sid, tc.tid, []byte("result")))
		require.Equal(t, "READ", e.Session["Matched Request"])
		require.Equal(t, tc.name, e.Session["File Context"])
	}

	asyncRead := smb2Read(77, 0x11, 1, sharedID, 16)
	e = t21Feed(t, s, 0, asyncRead)
	require.Equal(t, "alpha.txt", e.Session["File Context"])
	asyncPending := smb2ReadResp(77, 0x11, 1, nil)
	binary.LittleEndian.PutUint32(asyncPending[4+8:4+12], 0x00000103) // STATUS_PENDING
	binary.LittleEndian.PutUint32(asyncPending[4+16:4+20], smb2FlagResp|smb2FlagAsync)
	binary.LittleEndian.PutUint64(asyncPending[4+32:4+40], 0xfeed0001)
	e = t21Feed(t, s, 1, asyncPending)
	require.Equal(t, true, e.Session["Operation Pending"])
	require.Equal(t, uint64(0xfeed0001), e.Session["Async ID"])
	badAsyncFinal := smb2ReadResp(77, 0x11, 1, []byte("wrong async context"))
	binary.LittleEndian.PutUint32(badAsyncFinal[4+16:4+20], smb2FlagResp|smb2FlagAsync)
	binary.LittleEndian.PutUint64(badAsyncFinal[4+32:4+40], 0xfeed0002)
	e = t21Feed(t, s, 1, badAsyncFinal)
	require.Equal(t, "missing-or-ambiguous-request", e.Session["Association Status"])
	require.Contains(t, flow.smb2.async, smb2AsyncKey{sessionID: 0x11, asyncID: 0xfeed0001}, "wrong AsyncId must not consume the outstanding request")
	asyncFinal := smb2ReadResp(77, 0x11, 1, []byte("final"))
	binary.LittleEndian.PutUint32(asyncFinal[4+16:4+20], smb2FlagResp|smb2FlagAsync)
	binary.LittleEndian.PutUint64(asyncFinal[4+32:4+40], 0xfeed0001)
	e = t21Feed(t, s, 1, asyncFinal)
	require.Equal(t, "READ", e.Session["Matched Request"])
	require.Equal(t, "alpha.txt", e.Session["File Context"])
	require.Empty(t, flow.smb2.async)

	closeReq := smb2Close(80, 0x11, 1, sharedID)
	t21Feed(t, s, 0, closeReq)
	e = t21Feed(t, s, 1, smb2CloseResp(80, 0x11, 1, sharedID))
	require.Equal(t, "closed", e.Session["File Context Status"])
	require.Len(t, flow.smb2.files, 1)
	_, secondTreeStillOpen := flow.smb2.files[smb2FileKey{sessionID: 0x22, treeID: 2, fileID: hex.EncodeToString(sharedID)}]
	require.True(t, secondTreeStillOpen, "closing the first scoped handle cannot clear another tree's equal FileId")
	_, err = flow.smb2.consume(t21SMBControl(4, false, 81, 0x22, 2), DefaultParserBudget().MaxCollectionElements)
	require.NoError(t, err)
	_, err = flow.smb2.consume(t21SMBControl(4, true, 81, 0x22, 2), DefaultParserBudget().MaxCollectionElements)
	require.NoError(t, err)
	require.Empty(t, flow.smb2.files, "TREE_DISCONNECT releases only its tree-scoped handles")
	flow.closeSession()
	require.Zero(t, s.Stats().BufferedBytes)

	transformSession, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { transformSession.Close("FIN") })
	transform := t21Feed(t, transformSession, 0, smb2Transform())
	require.Equal(t, true, transform.Session["Encrypted"])
	require.Equal(t, "transform", transform.Session["Packet Name"])
	require.Equal(t, "opaque", transform.Session["Content Visibility"])
}

func testT21LDAPStateAndSASL(t *testing.T) {
	search := mustHexSession(t, "301c020101631704000a01020a01000201000201000101008702636e3000")
	search[4] = 9
	entry := ldapSessionMsg(0x64, append(ldapSessionTLV(0x04, []byte("cn=sample")), 0x30, 0))
	entry[4] = 9
	referral := ldapSessionMsg(0x73, ldapSessionTLV(0x04, []byte("ldap://referral.invalid/dc=sample")))
	referral[4] = 9
	done := ldapSessionMsg(0x65, []byte{0x0a, 1, 0, 0x04, 0, 0x04, 0})
	done[4] = 9
	abandonSearch := bytes.Clone(search)
	abandonSearch[4] = 10
	abandon := ldapSessionMsg(0x50, []byte{10})
	abandon[4] = 11
	searchAfterAbandon := bytes.Clone(done)
	searchAfterAbandon[4] = 10

	steps := []sessionStep{{0, search}, {1, entry}, {1, referral}, {1, done}, {0, abandonSearch}, {0, abandon}, {1, searchAfterAbandon}}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("search/deferred-%t", deferred), func(t *testing.T) {
			events, _ := sessionTestFlow(t, "ldap", steps, 1, deferred)
			require.Len(t, events, 7)
			require.Equal(t, uint32(1), events[1].Session["Entries Seen"])
			require.Equal(t, "partial", events[1].Session["Operation Progress"])
			require.Equal(t, uint32(1), events[2].Session["References Seen"])
			require.Equal(t, "partial", events[2].Session["Operation Progress"])
			require.Equal(t, true, events[3].Session["Search Complete"])
			require.Equal(t, uint32(1), events[3].Session["Entries Returned"])
			require.Equal(t, uint32(1), events[3].Session["References Returned"])
			require.Equal(t, true, events[5].Session["Abandon Matched"])
			require.Equal(t, true, events[6].Session["Unsolicited"], "abandoned search cannot later complete")
		})
	}
	t.Run("rfc4511-unsolicited-extended-response-message-id-zero", func(t *testing.T) {
		// RFC 4511 requires a responseName on unsolicited messageID=0 extended
		// responses. This bounded LDAP-fields profile represents the OID as text.
		responseName := []byte("1.2.3")
		responseBody := append([]byte{0x0a, 1, 0, 0x04, 0, 0x04, 0}, ldapSessionTLV(0x8a, responseName)...)
		response := ldapSessionMsg(0x78, responseBody)
		response[4] = 0
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("deferred-%t", deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "ldap", []sessionStep{{1, response}}, 1, deferred)
				require.Len(t, events, 1)
				require.Contains(t, []string{"decoded", "deferred"}, events[0].Status, "%s", events[0].Error)
				require.Equal(t, true, events[0].Session["Unsolicited"])
				require.NotContains(t, events[0].Session, "Matched Request")
			})
		}
	})
	t.Run("controls-remain-raw-with-no-applied-semantics", func(t *testing.T) {
		request := mustHexSession(t, "301c020101631704000a01020a01000201000201000101008702636e3000")
		_, _, body, err := ldapOperation(request)
		require.NoError(t, err)
		controlBody := append(ldapSessionTLV(0x04, []byte("1.2.840.113556.1.4.319")), []byte{0x01, 0x01, 0xff}...)
		controlBody = append(controlBody, ldapSessionTLV(0x04, []byte{0x30, 0x05, 0x02, 0x01, 0x20, 0x04, 0x00})...)
		controls := ldapSessionTLV(0xa0, ldapSessionTLV(0x30, controlBody))
		inner := append([]byte{0x02, 0x01, 0x01}, ldapSessionTLV(0x63, bytes.Clone(body))...)
		request = ldapSessionTLV(0x30, append(inner, controls...))
		done := ldapSessionMsg(0x65, []byte{0x0a, 1, 0, 0x04, 0, 0x04, 0})
		events, _ := sessionTestFlow(t, "ldap", []sessionStep{{0, request}, {1, done}}, 1, false)
		require.Len(t, events, 2)
		require.Equal(t, true, events[1].Session["Operation Complete"])
		decoded, err := events[0].Decode()
		require.NoError(t, err)
		encoded, err := json.Marshal(decoded)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "1.2.840.113556.1.4.319")
		require.Contains(t, string(encoded), `"Control Value":"MAUCASAEAA=="`, "control value stays an opaque octet string")
		require.NotContains(t, string(encoded), "Control Semantics Applied", "no control behavior is claimed")
		require.NotContains(t, string(encoded), `"Page Size":32`, "the opaque paging value is not interpreted")
	})

	for _, resultCode := range []byte{0, 53} {
		t.Run(fmt.Sprintf("starttls-result-%d", resultCode), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("FIN") })
			request := ldapSessionMsg(0x77, ldapSessionTLV(0x80, []byte("1.3.6.1.4.1.1466.20037")))
			e := t21Feed(t, s, 0, request)
			require.Equal(t, true, e.Session["StartTLS"])
			response := ldapSessionMsg(0x78, []byte{0x0a, 1, resultCode, 0x04, 0, 0x04, 0})
			e = t21Feed(t, s, 1, response)
			require.Equal(t, resultCode == 0, e.Session["StartTLS"])
			if resultCode == 0 {
				require.Equal(t, "tls", s.(*captureSession).f.protocol)
				tlsClientHello := t21CapturedTLSClientHello(t)
				tlsEvent := t21Feed(t, s, 0, tlsClientHello)
				require.Equal(t, "tls", tlsEvent.Protocol, "StartTLS must switch the live framing path, not only annotate state")
			} else {
				require.Equal(t, "ldap", s.(*captureSession).f.protocol)
			}
		})
	}

	t.Run("search-result-userpassword-redaction", func(t *testing.T) {
		const secret = "LDAP-SENTINEL-SECRET"
		attribute := ldapSessionTLV(0x30, append(ldapSessionTLV(0x04, []byte("userPassword")), ldapSessionTLV(0x31, ldapSessionTLV(0x04, []byte(secret)))...))
		entryBody := append(ldapSessionTLV(0x04, []byte("cn=fixture")), ldapSessionTLV(0x30, attribute)...)
		entry := ldapSessionMsg(0x64, entryBody)
		for _, deferred := range []bool{false, true} {
			events, _ := sessionTestFlow(t, "ldap", []sessionStep{{1, entry}}, 1, deferred)
			require.Len(t, events, 1)
			decoded, err := events[0].Decode()
			require.NoError(t, err)
			encoded, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), secret)
			require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString([]byte(secret)))
			require.Contains(t, string(encoded), "userPassword", "attribute description remains useful while values are removed")
			require.Contains(t, string(encoded), `"Length":20`)
			require.Contains(t, string(encoded), "Redacted")
		}
	})

	mechanism := ldapSessionTLV(0x04, []byte("GSSAPI"))
	mechanism = append(mechanism, ldapSessionTLV(0x04, []byte("sasl-test-secret"))...)
	bindBody := append([]byte{2, 1, 3}, ldapSessionTLV(0x04, nil)...)
	bindBody = append(bindBody, ldapSessionTLV(0xa3, mechanism)...)
	bind := ldapSessionMsg(0x60, bindBody)
	bind[4] = 12
	bindResponse := ldapSessionMsg(0x61, []byte{0x0a, 1, 0, 0x04, 0, 0x04, 0})
	bindResponse[4] = 12
	protected := append([]byte{0, 0, 0, 6}, []byte("opaque")...)
	steps = []sessionStep{{0, bind}, {1, bindResponse}, {0, protected}}
	secret := []byte("sasl-test-secret")
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("sasl-layer/deferred-%t", deferred), func(t *testing.T) {
			s := &captureSession{}
			config := NewDefaultConfig()
			require.NoError(t, WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) {
				s.events = append(s.events, e)
			}})(config))
			require.NoError(t, config.prepareBinParser())
			config.binParser.flows.Add(1)
			s.f = &binFlow{
				a: config.binParser, id: 1,
				endpoints: [2]string{"192.0.2.1:40000", "192.0.2.2:389"},
				ports:     [2]uint16{40000, 389},
			}
			t.Cleanup(func() { s.Close("FIN") })

			requestResult := s.Feed(0, time.Unix(1, 0), bind)
			require.Nil(t, requestResult.Err)
			require.Len(t, requestResult.Events, 1)
			bindEvent := requestResult.Events[0]
			require.Equal(t, map[bool]string{true: "deferred", false: "decoded"}[deferred], bindEvent.Status)
			require.Equal(t, "GSSAPI", bindEvent.Session["SASL Mechanism"])
			decoded, err := bindEvent.Decode()
			require.NoError(t, err)
			encoded, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), string(secret), "clear SASL octets must not be in event projections")

			responseResult := s.Feed(1, time.Unix(1, 0), bindResponse)
			require.Nil(t, responseResult.Err)
			require.Len(t, responseResult.Events, 1)
			response := responseResult.Events[0]
			require.Equal(t, map[bool]string{true: "deferred", false: "decoded"}[deferred], response.Status)
			require.Equal(t, "success-result-observed", response.Session["Authentication Status"])
			require.Equal(t, false, response.Session["Identity Verified"])
			require.Equal(t, "bind-succeeded; selected security layer not observed", response.Session["SASL Layer State"])

			result := s.Feed(0, time.Unix(1, 0), protected)
			require.NotNil(t, result.Err)
			require.Equal(t, ErrContextRequired, result.Err.Kind)
			require.Len(t, result.Events, 1)
			require.NotEqual(t, "LDAPProtectedRecord", result.Events[0].Entry)
			require.Equal(t, "context-required", result.Events[0].Status)
			require.EqualValues(t, 1, s.Stats().ContextRequired)
			require.Zero(t, s.Stats().Malformed)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func t21CapturedTLSClientHello(t *testing.T) []byte {
	t.Helper()
	var hello []byte
	err := ReplayPcap(bytes.NewReader(binCorpusBytes(t, "ndpi/ndpi-http-connect.pcap")), WithBinParserDeferred(true), WithBinParser(func(e *BinParserEvent) {
		if hello == nil && e.Protocol == "tls" && len(e.Raw) > 9 && e.Raw[0] == 22 && e.Raw[5] == 1 {
			hello = bytes.Clone(e.Raw)
		}
	}))
	require.True(t, err == nil || strings.Contains(err.Error(), "TCP SYN changes an established initial sequence number"), "%v", err)
	require.NotEmpty(t, hello)
	return hello
}

func binReplayRequireNoError(t *testing.T, steps []sessionStep, chunk int, deferred bool) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }})(c))
	require.NoError(t, c.prepareBinParser())
	f := &binFlow{a: c.binParser, id: 1, endpoints: [2]string{"192.0.2.1:12345", "192.0.2.2:389"}, ports: [2]uint16{12345, 389}}
	for _, step := range steps {
		for wire := step.wire; len(wire) > 0; {
			n := len(wire)
			if chunk > 0 {
				n = min(n, chunk)
			}
			f.feed(step.dir, wire[:n], time.Unix(1, 0))
			wire = wire[n:]
		}
	}
	f.close(TrafficFlowCloseReason_FIN)
	require.Nil(t, f.a.err.Load())
	require.Zero(t, f.a.stats().BufferedBytes)
	return events, f.a.stats()
}

func testT21DCERPC(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(12, 0)
	r := s.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 100))
	require.Nil(t, r.Err)
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])
	r = s.Feed(1, ts, dcerpcBindAck(100))
	require.Nil(t, r.Err)
	r = s.Feed(0, ts, dcerpcRequest(101, 0, 3, []byte("opaque-stub")))
	require.Nil(t, r.Err)
	require.Equal(t, uint16(3), r.Events[0].Session["OpNum"])
	require.Equal(t, len("opaque-stub"), r.Events[0].Session["Stub Bytes"])
	require.NotContains(t, fmt.Sprint(r.Events[0].Session), "opaque-stub", "unknown RPC stub remains opaque")
	r = s.Feed(1, ts, dcerpcResponse(101, 0, []byte("reply")))
	require.Nil(t, r.Err)
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	first := dcerpcPDU(0, dcerpcFirst, 102, append([]byte{0, 0, 0, 0, 0, 0, 3, 0}, []byte("first-")...))
	last := dcerpcPDU(0, dcerpcLast, 102, []byte("last"))
	r = s.Feed(0, ts, first)
	require.Nil(t, r.Err)
	require.Equal(t, true, r.Events[0].Session["Fragment"])
	r = s.Feed(0, ts, last)
	require.Nil(t, r.Err)
	require.Equal(t, true, r.Events[0].Session["Reassembled"])
	require.Equal(t, len("first-last"), r.Events[0].Session["Stub Bytes"])
}
