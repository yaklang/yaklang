package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func tcpFailureBody(header int, options, payload []byte) []byte {
	body := make([]byte, 20+len(options)+len(payload))
	binary.BigEndian.PutUint16(body[:2], 12345)
	binary.BigEndian.PutUint16(body[2:4], 443)
	binary.BigEndian.PutUint32(body[4:8], 0x01020304)
	binary.BigEndian.PutUint32(body[8:12], 0x0a0b0c0d)
	body[12], body[13] = byte(header/4)<<4, 0x18
	binary.BigEndian.PutUint16(body[14:16], 4096)
	binary.BigEndian.PutUint16(body[16:18], 0x3344)
	copy(body[20:], options)
	copy(body[20+len(options):], payload)
	return body
}

func tcpFailureRaw(body []byte) *capturedPacket {
	// An EOL and its captured nonzero padding exercise the same own-IP
	// boundary as the public witnesses; padding is not another option.
	wire := make([]byte, 14+24+len(body))
	copy(wire[:12], []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1})
	binary.BigEndian.PutUint16(wire[12:14], 0x0800)
	ip := wire[14:38]
	ip[0], ip[8], ip[9] = 0x46, 64, 6
	binary.BigEndian.PutUint16(ip[2:4], uint16(24+len(body)))
	copy(ip[12:16], []byte{198, 51, 100, 1})
	copy(ip[16:20], []byte{198, 51, 100, 2})
	copy(ip[20:24], []byte{0, 7, 5, 9})
	copy(wire[38:], body)
	return &capturedPacket{number: 1, link: layers.LinkTypeEthernet, data: wire, ci: gopacket.CaptureInfo{CaptureLength: len(wire), Length: len(wire)}}
}

func tcpFailureDecoded(raw *capturedPacket) gopacket.Packet {
	return gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
}

func requireTCPFailureRanges(t *testing.T, raw *capturedPacket, fields []field) {
	t.Helper()
	for _, f := range fields {
		require.Zero(t, f.source, "rejected TCP cannot synthesize a logical source")
		if f.hasRange {
			require.GreaterOrEqual(t, f.start, 0, f.text)
			require.Greater(t, f.end, f.start, f.text)
			require.LessOrEqual(t, f.end, len(raw.data), f.text)
		}
	}
}

func TestTCPHeaderFailureIncompleteOptionsNeverClaimEmptyPayload(t *testing.T) {
	raw := tcpFailureRaw(tcpFailureBody(60, []byte{1, 1, 1, 1, 1, 1, 1}, nil))
	p := tcpFailureDecoded(raw)
	require.NotNil(t, p.ErrorLayer())
	b := inspectTCPHeaderFailure(raw, p)
	require.NotNil(t, b)
	require.Equal(t, 27, b.available)
	require.Equal(t, 60, b.required)
	require.False(t, b.headerComplete)
	row := summarize(raw)
	require.Equal(t, "TCP", row.Protocol)
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Contains(t, row.Info, "27/60")
	require.Contains(t, row.Info, "payload boundary unknown")
	require.NotContains(t, row.Info, "Payload=0")
	require.Equal(t, []string{"Ethernet", "IPv4", "TCP"}, row.ProtocolStack)
	require.Equal(t, "198.51.100.1", row.Source)
	require.Equal(t, "198.51.100.2", row.Destination)
	deep, quick := detail(raw), quickDetail(raw)
	require.Equal(t, deep.fields[1:], quick.fields[1:], "the same captured prefix must survive deep decode")
	requireTCPFailureRanges(t, raw, deep.fields)
	require.Empty(t, deep.sources)
	require.NotContains(t, detailLines(deep.fields), "YAML dissector")
	require.NotContains(t, detailLines(deep.fields), "Dissector failed")
	var sourcePort, sequence field
	for _, f := range deep.fields {
		if f.text == "Source Port: 12345" {
			sourcePort = f
		}
		if f.text == "Sequence Number: 16909060" {
			sequence = f
		}
	}
	require.True(t, sourcePort.hasRange)
	require.Equal(t, [2]int{38, 40}, [2]int{sourcePort.start, sourcePort.end})
	require.Equal(t, [2]int{42, 46}, [2]int{sequence.start, sequence.end})
}

func TestTCPHeaderFailureFixedPrefixShowsOnlyCompleteWords(t *testing.T) {
	full := tcpFailureBody(60, nil, nil)
	for count := 1; count < 20; count++ {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			raw := tcpFailureRaw(full[:count])
			b := inspectTCPHeaderFailure(raw, tcpFailureDecoded(raw))
			require.NotNil(t, b)
			d := detail(raw)
			require.Equal(t, "TCP", d.protocol)
			requireTCPFailureRanges(t, raw, d.fields)
			text := detailLines(d.fields)
			for _, entry := range []struct {
				label string
				end   int
			}{{"Source Port:", 2}, {"Destination Port:", 4}, {"Sequence Number:", 8}, {"Acknowledgment Number:", 12}, {"Data Offset (32-bit words):", 13}, {"TCP Flags (reserved bits excluded):", 14}, {"Window:", 16}, {"Checksum (unverified):", 18}, {"Urgent Pointer:", 20}} {
				if count >= entry.end {
					require.Contains(t, text, entry.label)
				} else {
					require.NotContains(t, text, entry.label)
				}
			}
			require.Contains(t, text, "TCP Payload Boundary: unknown")
			require.NotContains(t, text, "Captured TCP Payload: 0")
		})
	}
}

func TestTCPHeaderFailureInvalidOffsetDoesNotCopyWiresharkPayloadArithmetic(t *testing.T) {
	raw := tcpFailureRaw(tcpFailureBody(12, nil, []byte("123456")))
	row := summarize(raw)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Contains(t, row.Info, "3 words")
	require.Contains(t, row.Info, "payload boundary unknown")
	require.NotContains(t, row.Info, "payload=14")
	require.NotContains(t, detailLines(detail(raw).fields), "Captured TCP Payload:")
}

func TestTCPHeaderFailureMPTCPDimensionsAvoidPartialOptionFormatter(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		option       []byte
	}{
		{"MP_CAPABLE5", "MP_CAPABLE", []byte{30, 5, 0, 0, 0}},
		{"MP_CAPABLE9", "MP_CAPABLE", []byte{30, 9, 0, 0, 0, 0, 0, 0, 0}},
		{"MP_JOIN30", "MP_JOIN", append([]byte{30, 30, 0x10, 0}, make([]byte, 26)...)},
		{"MP_FAIL14", "MP_FAIL", append([]byte{30, 14, 0x60, 0}, make([]byte, 10)...)},
		{"DSS6NoMapping", "DSS", []byte{30, 6, 0x20, 0, 0, 0}},
		{"DSS6MissingMapping", "DSS", []byte{30, 6, 0x20, 6, 0, 0}},
		{"MP_PRIO5", "MP_PRIO", []byte{30, 5, 0x50, 0, 0}},
		{"SubtypeUnavailable", "subtype unavailable", []byte{30, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := append([]byte(nil), tc.option...)
			for len(options)%4 != 0 {
				options = append(options, 0)
			}
			raw := tcpFailureRaw(tcpFailureBody(20+len(options), options, []byte{0xde, 0xad, 0xbe}))
			require.NotNil(t, tcpFailureDecoded(raw).ErrorLayer())
			row := summarize(raw)
			require.Equal(t, "TCP", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, tc.reason)
			require.Contains(t, row.Info, "captured TCP payload=3")
			d, q := detail(raw), quickDetail(raw)
			require.Equal(t, d.fields[1:], q.fields[1:])
			requireTCPFailureRanges(t, raw, d.fields)
			require.Empty(t, d.sources)
			require.NotContains(t, detailLines(d.fields), "Dissector failed")
			require.NotContains(t, detailLines(d.fields), "YAML dissector")
		})
	}
}

func TestTCPHeaderFailureMalformedGenericOptionKeepsKnownPayloadBoundary(t *testing.T) {
	for _, options := range [][]byte{{2, 0, 0, 0}, {254, 1, 0, 0}, {254, 9, 1, 1}, {1, 1, 1, 254}} {
		raw := tcpFailureRaw(tcpFailureBody(24, options, []byte{1, 2}))
		row := summarize(raw)
		require.NotNil(t, row.tcpHeaderFailure)
		require.Equal(t, "malformed", row.ProtocolStatus)
		require.Contains(t, row.Info, "captured TCP payload=2")
		require.NotContains(t, row.Info, "payload boundary unknown")
	}
}

func TestTCPHeaderFailureLeavesCompleteTCPAndApplicationChildErrorsAlone(t *testing.T) {
	for _, options := range [][]byte{
		nil, {2, 4, 5, 0xb4}, {254, 4, 0, 1}, {30, 4, 1, 0}, {30, 4, 0x20, 0}, {30, 4, 0xf0, 0}, {30, 8, 0x34, 1, 192, 0, 2, 1},
	} {
		raw := tcpFailureRaw(tcpFailureBody(20+len(options), options, nil))
		p := tcpFailureDecoded(raw)
		require.Nil(t, p.ErrorLayer())
		require.Nil(t, inspectTCPHeaderFailure(raw, p))
	}
	// A valid base TCP header followed by an incomplete DNS child is not a
	// transport failure. The child error cannot select our private boundary.
	raw := tcpFailureRaw(tcpFailureBody(20, nil, []byte{1}))
	binary.BigEndian.PutUint16(raw.data[40:42], 53)
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{DecodeStreamsAsDatagrams: true})
	require.NotNil(t, p.ErrorLayer())
	require.Nil(t, inspectTCPHeaderFailure(raw, p))
	// Public labels and connection hints do not substitute for own bad bytes.
	raw.application, raw.evidence, raw.connectionStack = "TLS", "validated protocol events", []string{"TLSv1.3", "HTTP/2"}
	require.Nil(t, inspectTCPHeaderFailure(raw, p))
}

func TestTCPHeaderFailureDoesNotBorrowIPPaddingOrInventCaptureLoss(t *testing.T) {
	raw := tcpFailureRaw(tcpFailureBody(60, nil, nil))
	raw.data = append(raw.data, make([]byte, 64)...)
	raw.ci.Length, raw.ci.CaptureLength = len(raw.data), len(raw.data)
	row := summarize(raw)
	require.NotNil(t, row.tcpHeaderFailure)
	require.Equal(t, 20, row.tcpHeaderFailure.available, "all bytes after declared IPv4 extent are padding")
	require.Contains(t, row.Info, "20/60")
	require.NotContains(t, row.Info, "truncated capture")
	raw.ci.Length += 10
	require.Contains(t, summarize(raw).Info, "truncated capture")
	// Zero total length is explicitly an observed compatibility prefix, not
	// a fabricated zero-length TCP payload or valid IPv4 envelope.
	zero := tcpFailureRaw(tcpFailureBody(60, nil, nil))
	binary.BigEndian.PutUint16(zero.data[16:18], 0)
	row = summarize(zero)
	require.NotNil(t, row.tcpHeaderFailure)
	require.Contains(t, row.Info, "legacy captured-prefix observation")
	require.Contains(t, detailLines(detail(zero).fields), "not a valid declared IPv4 extent")
}

func TestTCPHeaderFailureRequiresOwnSelectedParentAndLiteralPacket(t *testing.T) {
	base := tcpFailureRaw(tcpFailureBody(60, nil, nil))
	for _, change := range []func(*capturedPacket){
		func(r *capturedPacket) { r.data[14] = 0x66 },
		func(r *capturedPacket) { r.data[23] = 17 },
		func(r *capturedPacket) { r.data[23] = 1 },  // ICMP quoted TCP is not our own header.
		func(r *capturedPacket) { r.data[23] = 97 }, // Unselected EtherIP child.
		func(r *capturedPacket) { r.data[20] = 0x20 },
		func(r *capturedPacket) { binary.BigEndian.PutUint16(r.data[16:18], 23) },
		func(r *capturedPacket) { r.data[34], r.data[35] = 2, 1 },
	} {
		raw := *base
		raw.data = bytes.Clone(base.data)
		change(&raw)
		require.Nil(t, literalTCPHeaderFailure(&raw))
	}
	raw := *base
	raw.presentation.networkRefused = true
	require.Nil(t, inspectTCPHeaderFailure(&raw, tcpFailureDecoded(&raw)))
	p := tcpFailureDecoded(base)
	raw = *base
	raw.data = bytes.Clone(base.data)
	raw.data[42] ^= 1
	require.Nil(t, inspectTCPHeaderFailure(&raw, p), "a failure in another packet is not provenance")
}

func TestTCPHeaderFailureRawCookedLoopbackVLANAndIPv6Ranges(t *testing.T) {
	base := tcpFailureRaw(tcpFailureBody(60, nil, nil))
	ip := bytes.Clone(base.data[14:])
	vlan := append(bytes.Clone(base.data[:12]), 0x81, 0, 0, 7, 8, 0)
	vlan = append(vlan, ip...)
	sll := make([]byte, 16)
	binary.BigEndian.PutUint16(sll[14:16], 0x0800)
	sll2 := make([]byte, 20)
	binary.BigEndian.PutUint16(sll2[:2], 0x0800)
	ipv6 := make([]byte, 40+20)
	ipv6[0], ipv6[6], ipv6[7] = 0x60, 6, 64
	binary.BigEndian.PutUint16(ipv6[4:6], 20)
	ipv6[8], ipv6[9], ipv6[23] = 0x20, 1, 1
	ipv6[24], ipv6[25], ipv6[39] = 0x20, 1, 2
	copy(ipv6[40:], tcpFailureBody(60, nil, nil))
	for _, tc := range []struct {
		name  string
		link  layers.LinkType
		wire  []byte
		start int
	}{
		{"raw", layers.LinkTypeRaw, ip, 24},
		{"explicit4", layers.LinkTypeIPv4, ip, 24},
		{"loopLE", layers.LinkTypeNull, append([]byte{2, 0, 0, 0}, ip...), 28},
		{"loopBE", layers.LinkTypeLoop, append([]byte{0, 0, 0, 2}, ip...), 28},
		{"SLL", layers.LinkTypeLinuxSLL, append(sll, ip...), 40},
		{"SLL2", layers.LinkTypeLinuxSLL2, append(sll2, ip...), 44},
		{"VLAN", layers.LinkTypeEthernet, vlan, 42},
		{"explicit6", layers.LinkTypeIPv6, ipv6, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &capturedPacket{number: 1, data: tc.wire, link: tc.link, ci: gopacket.CaptureInfo{CaptureLength: len(tc.wire), Length: len(tc.wire)}}
			b := literalTCPHeaderFailure(raw)
			require.NotNil(t, b)
			require.Equal(t, tc.start, b.start)
			d := packetDetail{}
			b.appendFields(&d)
			requireTCPFailureRanges(t, raw, d.fields)
		})
	}
}

func TestTCPHeaderFailureCannotInheritConnectionProtocolOrNamespace(t *testing.T) {
	raw := tcpFailureRaw(tcpFailureBody(60, nil, nil))
	raw.application, raw.evidence = "TLS", "validated protocol events"
	raw.connectionStack = []string{"TLSv1.3", "HTTP/2", "gRPC"}
	raw.streamID, raw.conversationID, raw.tcpFlowID = 99, 98, 97
	row := summarize(raw)
	for i := 0; i < 2; i++ {
		row = applyApplication(row, "TLS", "validated protocol events", 99)
		require.Equal(t, "TCP", row.Protocol)
		require.Equal(t, "incomplete", row.ProtocolStatus)
		require.Zero(t, row.StreamID)
		require.Zero(t, row.ConversationID)
		require.Zero(t, row.TCPFlowID)
		require.Empty(t, row.ConnectionProtocol)
		require.Empty(t, row.ConnectionStack)
		require.NotContains(t, row.Info, "connection #")
	}
	store := &streamStore{}
	row.ConnectionStack, row.ConnectionProtocol = []string{"SSH"}, "SSH"
	row = store.projectConversation(row, 99)
	require.Empty(t, row.ConnectionStack)
	row.ConnectionProtocol = "QUIC"
	row = store.projectDatagram(row, raw)
	require.Empty(t, row.ConnectionProtocol)
}

func TestTCPHeaderFailureWorkerUsesSameSource0Tree(t *testing.T) {
	// MP_CAPABLE length5 previously caused LayerString on its nil semantic
	// pointer to replace the actual fields with "Dissector failed".
	raw := tcpFailureRaw(tcpFailureBody(28, []byte{30, 5, 0, 0, 0, 0, 0, 0}, []byte{1, 2, 3}))
	request, err := json.Marshal(decodeRequest{Number: 1, Data: raw.data, Length: len(raw.data), Link: raw.link})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
	index := strings.Index(output.String(), decoderFrame)
	require.GreaterOrEqual(t, index, 0)
	var response decodeResponse
	require.NoError(t, json.Unmarshal(output.Bytes()[index+len(decoderFrame):], &response))
	require.Equal(t, "TCP", response.Protocol)
	require.Empty(t, response.Sources)
	deep := detail(raw)
	require.Len(t, response.Fields, len(deep.fields))
	for i, f := range response.Fields {
		require.Equal(t, deep.fields[i].text, f.Text)
		require.Equal(t, deep.fields[i].start, f.Start)
		require.Equal(t, deep.fields[i].end, f.End)
		require.Zero(t, f.Source)
		require.NotContains(t, f.Text, "Dissector failed")
		require.NotContains(t, f.Text, "YAML dissector")
	}
}
