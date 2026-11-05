package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"
	"unsafe"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func smb2QueryRequest(mid, sid uint64, tid uint32, typ, class byte, input []byte) []byte {
	body := make([]byte, 40)
	binary.LittleEndian.PutUint16(body, 41)
	body[2], body[3] = typ, class
	binary.LittleEndian.PutUint32(body[4:], 0x11223344)
	binary.LittleEndian.PutUint16(body[10:], 0xa1b2) // Reserved is literal, not a selector.
	binary.LittleEndian.PutUint32(body[16:], 0x10203040)
	binary.LittleEndian.PutUint32(body[20:], 5)
	copy(body[24:], []byte{0, 1, 2, 3, 4, 5, 6, 7, 0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7})
	if len(input) != 0 {
		binary.LittleEndian.PutUint16(body[8:], 108) // Four independently owned padding bytes.
		binary.LittleEndian.PutUint32(body[12:], uint32(len(input)))
		body = append(body, 0xaa, 0xbb, 0xcc, 0xdd)
		body = append(body, input...)
	}
	return smb2TCP(append(smb2Hdr(16, 0, mid, sid, tid), body...))
}

func smb2QueryResponse(mid, sid uint64, tid uint32, status uint32, output []byte) []byte {
	header := smb2Hdr(16, smb2FlagResp, mid, sid, tid)
	binary.LittleEndian.PutUint32(header[8:], status)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint16(body, 9)
	if len(output) != 0 {
		binary.LittleEndian.PutUint16(body[2:], 76)
		binary.LittleEndian.PutUint32(body[4:], uint32(len(output)))
		body = append(body, 0x81, 0x82, 0x83, 0x84)
		body = append(body, output...)
	}
	return smb2TCP(append(header, body...))
}

func smb2QueryNode(t *testing.T, wire []byte) *base.Node {
	t.Helper()
	node, err := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	for _, child := range node.Children {
		if child.Name == "Query Info" {
			return child
		}
	}
	t.Fatal("QUERY_INFO must be dispatched through the canonical entry")
	return nil
}

func TestSMB2QueryInfoNativeCanonicalLiteralSources(t *testing.T) {
	for _, test := range []struct {
		name string
		wire []byte
	}{
		{"request", smb2QueryRequest(0x15, 0x1122, 7, 1, 0x22, []byte{0, 2, 0xff, 0x80})},
		{"response", smb2QueryResponse(0x15, 0x1122, 7, 0, []byte{9, 0, 0x34, 0x12})},
		{"partial-output", smb2QueryResponse(0x15, 0x1122, 7, 0x80000005, []byte{7, 8, 9})},
		{"async-response", smb2AsyncWire(smb2QueryResponse(0x15, 0x1122, 0, 0, []byte{1, 2, 3}), 0x1234567890abcdef)},
		{"error", smb2ErrorResponse(16, 0x15, 0x1122, 7, 0xc0000023, []byte{0x80, 0, 0, 0})},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := bytes.Clone(test.wire)
			state := &binSMB2{client: 0}
			info, err := state.consumeFrom(1, test.wire, 64)
			require.NoError(t, err)
			body := info["Body"].(map[string]any)
			ranges := body["Wire Field Ranges"].(map[string]any)
			canonical := smb2QueryNode(t, test.wire)
			require.Len(t, canonical.Children, len(body)-1)
			for _, child := range canonical.Children {
				span := ranges[child.Name].([]any)
				lo, hi := span[0].(uint64), span[1].(uint64)
				pos := stream_parser.GetNodeResultPos(child)
				require.Equal(t, [2]uint64{(lo - 4) * 8, (hi - 4) * 8}, pos, child.Name)
				value := stream_parser.GetNodeResult(child)
				if b, ok := body[child.Name].([]byte); ok {
					require.Equal(t, original[lo:hi], b)
					require.Equal(t, b, value, child.Name)
				} else {
					require.EqualValues(t, body[child.Name], value, child.Name)
				}
			}
			for i := range test.wire {
				test.wire[i] ^= 0xff
			}
			for _, child := range canonical.Children {
				span := ranges[child.Name].([]any)
				if b, ok := body[child.Name].([]byte); ok {
					require.Equal(t, original[span[0].(uint64):span[1].(uint64)], stream_parser.GetNodeResult(child), "canonical owner survives caller mutation: %s", child.Name)
					require.NotEqual(t, test.wire[span[0].(uint64):span[1].(uint64)], b)
				}
			}
			for key, value := range body {
				if b, ok := value.([]byte); ok {
					span := ranges[key].([]any)
					require.Equal(t, original[span[0].(uint64):span[1].(uint64)], b, "owned %s", key)
				}
			}
			if info["Response"] == false {
				require.Equal(t, hex.EncodeToString(original[92:108]), info["FileId"])
				require.Equal(t, uint32(0x11223344), body["Output Buffer Length"], "request maximum is not input payload size")
			} else {
				require.NotContains(t, body, "Info Type")
				require.NotContains(t, body, "File Info Class")
				require.NotContains(t, body, "File ID")
				require.NotContains(t, info, "Request Info Type")
			}
		})
	}
}

func TestSMB2QueryInfoCanonicalAsyncHeaderHasNoInventedTree(t *testing.T) {
	wire := smb2AsyncWire(smb2QueryResponse(21, 0x1122, 0, 0, []byte{1, 2, 3}), 0x1234567890abcdef)
	node, err := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	published, err := node.Result()
	require.NoError(t, err)
	require.Nil(t, published.Child("ProcessId"), "unselected template nodes are not published fields")
	require.Nil(t, published.Child("TreeId"))
	require.NotNil(t, published.Child("AsyncId"))
	require.Equal(t, uint64(0x1234567890abcdef), published.Child("AsyncId").Value)
	projected := stream_parser.NodeToMap(node).(map[string]any)
	require.NotContains(t, projected, "ProcessId")
	require.NotContains(t, projected, "TreeId")
	found := false
	for _, child := range node.Children {
		if child.Name == "AsyncId" {
			found = true
			require.Equal(t, uint64(0x1234567890abcdef), stream_parser.GetNodeResult(child))
			require.Equal(t, [2]uint64{32 * 8, 40 * 8}, stream_parser.GetNodeResultPos(child))
		}
	}
	require.True(t, found)
}

func TestSMB2QueryInfoCanonicalRequestHeaderUnion(t *testing.T) {
	wire := smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil)
	binary.LittleEndian.PutUint16(wire[12:], 0x1234)
	binary.LittleEndian.PutUint16(wire[14:], 0x5678)
	node, err := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	value, err := node.Result()
	require.NoError(t, err)
	require.Nil(t, value.Child("Status"), "request channel/reserved bytes are not NTSTATUS")
	require.Nil(t, value.Child("SMB2HeaderFlags"), "the recovered peek is never published")
	require.EqualValues(t, 0x1234, value.Child("ChannelSequence").Value)
	require.EqualValues(t, 0x5678, value.Child("Reserved").Value)
	for _, child := range node.Children {
		switch child.Name {
		case "ChannelSequence":
			require.Equal(t, [2]uint64{8 * 8, 10 * 8}, stream_parser.GetNodeResultPos(child))
		case "Reserved":
			require.Equal(t, [2]uint64{10 * 8, 12 * 8}, stream_parser.GetNodeResultPos(child))
		}
	}
	info, err := (&binSMB2{}).consume(wire, 64)
	require.NoError(t, err)
	require.Equal(t, uint16(0x1234), info["Header"].(map[string]any)["Channel Sequence"])
	require.Equal(t, uint16(0x5678), info["Header"].(map[string]any)["Reserved"])
	require.NotContains(t, stream_parser.NodeToMap(node), "Status")
}

func TestSMB2QueryInfoIncompleteBodiesCannotConsumeRequests(t *testing.T) {
	for _, response := range []bool{false, true} {
		wire := smb2QueryRequest(21, 0x1122, 7, 1, 0x22, []byte{1, 2, 3, 4})
		if response {
			wire = smb2QueryResponse(21, 0x1122, 7, 0, []byte{1, 2, 3, 4})
		}
		for end := 64; end < len(wire)-4; end++ {
			t.Run(fmt.Sprintf("response-%t-cut-%d", response, end), func(t *testing.T) {
				state := &binSMB2{client: 0}
				_, err := state.consumeFrom(0, smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil), 64)
				require.NoError(t, err)
				partial := smb2TCP(bytes.Clone(wire[4 : 4+end]))
				_, err = state.consumeFrom(1, partial, 64)
				require.Error(t, err)
				require.Len(t, state.pending, 1)
				_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(partial[4:]), bits: uint64(len(partial)-4) * 8}, "application-layer.smb2", "SMB2")
				require.Error(t, err, "canonical must not silently accept a truncated buffer")
			})
		}
	}
}

func TestSMB2QueryInfoBoundsAndIgnoredZeroOffsets(t *testing.T) {
	for _, response := range []bool{false, true} {
		for _, offset := range []uint16{0, 1, 63, 64, 71, 103, 0xffff} {
			t.Run(fmt.Sprintf("response-%t-offset-%d", response, offset), func(t *testing.T) {
				wire := smb2QueryRequest(1, 2, 3, 0xfe, 0xff, nil)
				pair := 76 // Request InputBufferOffset including Direct TCP.
				if response {
					wire, pair = smb2QueryResponse(1, 2, 3, 0, nil), 70
				}
				binary.LittleEndian.PutUint16(wire[pair:], offset)
				_, err := (&binSMB2{}).consume(wire, 64)
				require.NoError(t, err, "zero length ignores its offset, including an unknown selector")
				smb2QueryNode(t, wire)
				length := pair + 4
				if response {
					length = pair + 2
				}
				binary.LittleEndian.PutUint32(wire[length:], 0xffffffff)
				_, err = (&binSMB2{}).consume(wire, 64)
				require.Error(t, err)
				_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
				require.Error(t, err)
			})
		}
	}
}

func TestSMB2QueryInfoSelectorAssociationAndReuse(t *testing.T) {
	for _, test := range []struct {
		name   string
		dir    int
		sid    uint64
		tid    uint32
		status uint32
	}{
		{"same-direction", 0, 0x1122, 7, 0}, {"other-session", 1, 0x1123, 7, 0}, {"other-tree", 1, 0x1122, 8, 0},
		{"zero-session-partial-output", 1, 0, 0, 0x80000005},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil), 64)
			require.NoError(t, err)
			info, err := state.consumeFrom(test.dir, smb2QueryResponse(21, test.sid, test.tid, test.status, []byte{1, 2, 3}), 64)
			require.NoError(t, err)
			require.NotContains(t, info, "Request Info Type")
			require.Len(t, state.pending, 1)
			for _, class := range []byte{0x22, 5} {
				matched, err := state.consumeFrom(1, smb2QueryResponse(21, 0x1122, 7, 0x80000005, []byte{1, 2, 3}), 64)
				require.NoError(t, err)
				require.Equal(t, "matched", matched["Association Status"])
				require.Equal(t, class, matched["Request File Info Class"])
				require.Equal(t, uint8(1), matched["Request Info Type"])
				require.Equal(t, uint32(0x10203040), matched["Request Additional Information"])
				require.Equal(t, uint64(21), matched["Request Message ID"])
				require.Empty(t, state.pending)
				body := matched["Body"].(map[string]any)
				require.NotContains(t, body, "Info Type")
				for key := range matched {
					if len(key) >= 8 && key[:8] == "Request " {
						require.NotContains(t, matched["Wire Field Ranges"], key)
					}
				}
				if class == 0x22 {
					_, err = state.consumeFrom(0, smb2QueryRequest(21, 0x1122, 7, 1, 5, nil), 64)
					require.NoError(t, err, "a completed MessageId may carry a different selector")
				}
			}
		})
	}
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil), 64)
	require.NoError(t, err)
	info, err := state.consumeFrom(1, smb2ErrorResponse(16, 21, 0, 0, 0xc0000203, nil), 64)
	require.NoError(t, err)
	require.Equal(t, "matched", info["Association Status"], "a complete lost-session ERROR retains the existing unique-MID fallback")
	require.Equal(t, true, info["Error Response"])
	require.Empty(t, state.pending)
	require.LessOrEqual(t, unsafe.Sizeof(smb2Pending{}), uintptr(160), "pending reservation must cover the retained selector")
}

func TestSMB2QueryInfoCompoundCommandBounds(t *testing.T) {
	first := smb2QueryRequest(1, 2, 3, 1, 4, nil)[4:]
	second := smb2QueryRequest(2, 2, 3, 2, 5, nil)[4:]
	binary.LittleEndian.PutUint32(first[20:], uint32(len(first)))
	wire := smb2TCP(append(first, second...))
	info, err := (&binSMB2{}).consume(wire, 64)
	require.NoError(t, err)
	commands := info["Commands"].([]map[string]any)
	for index, command := range commands {
		body := command["Body"].(map[string]any)
		require.Equal(t, byte(index+1), body["Info Type"])
		require.Equal(t, []any{uint64(70 + index*104), uint64(71 + index*104)}, body["Wire Field Ranges"].(map[string]any)["Info Type"])
	}
	smb2QueryNode(t, wire)
	// A declared buffer in command 0 cannot borrow command 1's header bytes.
	binary.LittleEndian.PutUint16(wire[76:], 104)
	binary.LittleEndian.PutUint32(wire[80:], 4)
	_, err = (&binSMB2{}).consume(wire, 64)
	require.Error(t, err)
	_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire[4:]), bits: uint64(len(wire)-4) * 8}, "application-layer.smb2", "SMB2")
	require.Error(t, err)
	for _, tail := range []int{0, 1, 8, 63} {
		short := smb2QueryRequest(1, 2, 3, 1, 4, nil)[4:]
		binary.LittleEndian.PutUint32(short[20:], uint32(len(short)))
		short = append(short, make([]byte, tail)...)
		_, err := smb2Walk(short, 64)
		require.Error(t, err, "a nonzero NextCommand needs the entire following header")
		_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(short), bits: uint64(len(short)) * 8}, "application-layer.smb2", "SMB2")
		require.Error(t, err, "canonical NextCommand cannot end at/near the message boundary")
	}
}

func TestSMB2QueryInfoBadStructuresAndErrorByteCount(t *testing.T) {
	for _, wire := range [][]byte{
		smb2QueryRequest(21, 0x1122, 7, 1, 4, nil),
		smb2QueryResponse(21, 0x1122, 7, 0, nil),
		smb2ErrorResponse(16, 21, 0x1122, 7, 0xc000000d, nil),
	} {
		bad := bytes.Clone(wire)
		binary.LittleEndian.PutUint16(bad[68:], 8)
		_, err := (&binSMB2{}).consume(bad, 64)
		require.Error(t, err)
		_, err = parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(bad[4:]), bits: uint64(len(bad)-4) * 8}, "application-layer.smb2", "SMB2")
		require.Error(t, err)
	}
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2QueryRequest(21, 0x1122, 7, 1, 4, nil), 64)
	require.NoError(t, err)
	badError := smb2ErrorResponse(16, 21, 0x1122, 7, 0xc0000023, []byte{1, 2, 3, 4})
	binary.LittleEndian.PutUint32(badError[72:], 0xffffffff)
	_, err = state.consumeFrom(1, badError, 64)
	require.Error(t, err)
	require.Len(t, state.pending, 1, "an invalid error must not finish the request")
	goodError := smb2ErrorResponse(16, 21, 0x1122, 7, 0xc0000023, []byte{1, 2, 3, 4})
	info, err := state.consumeFrom(1, goodError, 64)
	require.NoError(t, err)
	require.Equal(t, true, info["Error Response"])
	require.NotContains(t, info["Body"], "Output Buffer Length")
	require.Empty(t, state.pending)
}

func TestProtocolSessionSMB2QueryInfoSplitAndAsync(t *testing.T) {
	for _, width := range []int{1, 7, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			session, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: 128 << 10, MaxCollectionElements: 64})
			require.NoError(t, err)
			defer session.Close("test")
			smb2ChainFeed(t, session, 0, smb2NegotiateReq(0x0202))
			smb2ChainFeed(t, session, 1, smb2NegotiateResp(0x0202))
			feed := func(direction int, wire []byte) map[string]any {
				var events []*ProtocolEvent
				for at := 0; at < len(wire); at += width {
					result := session.Feed(direction, time.Unix(1, 0), wire[at:min(at+width, len(wire))])
					if result.Err != nil {
						require.Equal(t, ErrNeedMore, result.Err.Kind)
						require.True(t, result.NeedMore)
					}
					events = append(events, result.Events...)
				}
				require.Len(t, events, 1)
				require.Equal(t, "decoded", events[0].Status)
				return events[0].Session
			}
			feed(0, smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil))
			pending := smb2AsyncWire(smb2ErrorResponse(16, 21, 0x1122, 0, 0x103, nil), 99)
			info := feed(1, pending)
			require.Equal(t, true, info["Operation Pending"])
			final := smb2AsyncWire(smb2QueryResponse(0xffffffffffffffff, 0x1122, 0, 0, []byte{1, 2, 3, 4}), 99)
			info = feed(1, final)
			require.Equal(t, uint8(1), info["Request Info Type"])
			require.Equal(t, uint8(0x22), info["Request File Info Class"])
			require.Equal(t, uint32(7), info["Request Tree ID"])
			require.NotContains(t, info["Header"], "Tree ID", "async header has no literal TreeId")
			require.Empty(t, session.(*captureSession).f.smb2.pending)
			require.Empty(t, session.(*captureSession).f.smb2.async)
			session.Close("FIN")
			require.Zero(t, session.Stats().BufferedBytes)
		})
	}
}

func TestSMB2QueryInfoCaptureDomainSelectorIsolation(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		t.Run(fmt.Sprintf("vlan-%t", vlan), func(t *testing.T) {
			wires := [][]byte{
				smb2QueryRequest(21, 0x1122, 7, 1, 0x22, nil),
				smb2QueryResponse(21, 0x1122, 7, 0, []byte{1, 2, 3}),
				smb2QueryRequest(21, 0x1122, 7, 2, 5, nil),
				smb2QueryResponse(21, 0x1122, 7, 0, []byte{1, 2, 3}),
				smb2QueryResponse(21, 0x1122, 7, 0, []byte{1, 2, 3}),
			}
			domains, directions := []int{0, 1, 1, 1, 0}, []int{0, 1, 0, 1, 1}
			var events []*ProtocolEvent
			analyzer, err := NewPacketAnalyzer(WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
			require.NoError(t, err)
			defer analyzer.Close()
			seq := [2][2]uint32{{1, 1}, {1, 1}}
			for index, wire := range wires {
				domain, direction := domains[index], directions[index]
				src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
				sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
				if direction == 1 {
					src, dst, sport, dport = dst, src, dport, sport
				}
				ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
				ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
				tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][direction], ACK: true, PSH: true, Window: 4096}
				seq[domain][direction] += uint32(len(wire))
				require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
				buffer := gopacket.NewSerializeBuffer()
				options := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
				iface := domain
				if vlan {
					iface = 0
					ethernet.EthernetType = layers.EthernetTypeDot1Q
					tag := &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}
					require.NoError(t, gopacket.SerializeLayers(buffer, options, ethernet, tag, ip, tcp, gopacket.Payload(wire)))
				} else {
					require.NoError(t, gopacket.SerializeLayers(buffer, options, ethernet, ip, tcp, gopacket.Payload(wire)))
				}
				packet := buffer.Bytes()
				require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+index), 0), CaptureLength: len(packet), Length: len(packet), InterfaceIndex: iface}, layers.LinkTypeEthernet))
				clear(packet)
			}
			require.NoError(t, analyzer.Close())
			require.Len(t, events, len(wires))
			for index, event := range events {
				require.Equal(t, wires[index], event.Raw)
			}
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
			require.Equal(t, "missing-or-ambiguous-request", events[1].Session["Association Status"])
			require.NotContains(t, events[1].Session, "Request Info Type", "other-domain request must not leak")
			require.Equal(t, uint8(2), events[3].Session["Request Info Type"])
			require.Equal(t, uint8(5), events[3].Session["Request File Info Class"])
			require.Equal(t, uint8(1), events[4].Session["Request Info Type"])
			require.Equal(t, uint8(0x22), events[4].Session["Request File Info Class"])
		})
	}
}
