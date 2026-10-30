package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func smb2StandardOutput() []byte {
	wire := make([]byte, 24)
	binary.LittleEndian.PutUint64(wire, 0x1020304050607000)
	binary.LittleEndian.PutUint64(wire[8:], 0x11223344556677)
	binary.LittleEndian.PutUint32(wire[16:], 0xa1b2c3d4)
	wire[20], wire[21] = 0x80, 0xff                  // Every nonzero Boolean is true.
	binary.LittleEndian.PutUint16(wire[22:], 0xcafe) // Reserved is ignored, not rejected.
	return wire
}

func TestSMB2QueryOutputCanonicalNativeTypesAndOwnedSources(t *testing.T) {
	for _, test := range []struct {
		class byte
		entry string
		wire  []byte
		want  map[string]any
		spans map[string][2]uint64
	}{
		{7, "FileEaInformation", []byte{0x04, 0x03, 0x02, 0x81}, map[string]any{"EA Size": uint32(0x81020304)}, map[string][2]uint64{"EA Size": {0, 4}}},
		{5, "FileStandardInformation", smb2StandardOutput(), map[string]any{
			"Allocation Size": int64(0x1020304050607000), "End Of File": int64(0x11223344556677),
			"Number Of Links": uint32(0xa1b2c3d4), "Delete Pending": uint8(0x80), "Directory": uint8(0xff), "Reserved": uint16(0xcafe),
		}, map[string][2]uint64{"Allocation Size": {0, 8}, "End Of File": {8, 16}, "Number Of Links": {16, 20}, "Delete Pending": {20, 21}, "Directory": {21, 22}, "Reserved": {22, 24}}},
	} {
		t.Run(test.entry, func(t *testing.T) {
			owned := bytes.Clone(test.wire)
			direct, err := stream_parser.DecodeSMB2QueryInfoOutput(test.wire, 1, test.class)
			require.NoError(t, err)
			node, err := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(test.wire), bits: uint64(len(test.wire)) * 8}, "application-layer.smb2", test.entry)
			require.NoError(t, err)
			require.Len(t, node.Children, len(test.want))
			for _, child := range node.Children {
				require.Equal(t, test.want[child.Name], stream_parser.GetNodeResult(child), "canonical type/value: %s", child.Name)
				span := test.spans[child.Name]
				require.Equal(t, [2]uint64{span[0] * 8, span[1] * 8}, stream_parser.GetNodeResultPos(child))
				require.Equal(t, test.want[child.Name], direct[child.Name], "direct type/value: %s", child.Name)
			}
			state := &binSMB2{client: 0}
			_, err = state.consumeFrom(0, smb2QueryRequest(21, 17, 7, 1, test.class, nil), 64)
			require.NoError(t, err)
			response := smb2QueryResponse(21, 17, 7, 0, test.wire)
			info, err := state.consumeFrom(1, response, 64)
			require.NoError(t, err)
			require.Equal(t, test.entry, info["Output Information Type"])
			require.Equal(t, test.entry, info["Request Output Information Type"])
			body := info["Body"].(map[string]any)
			typed := body[test.entry].(map[string]any)
			require.Equal(t, []any{uint64(80), uint64(80 + len(owned))}, body["Wire Field Ranges"].(map[string]any)[test.entry])
			for name, want := range test.want {
				require.Equal(t, want, typed[name], "native type/value: %s", name)
				span := test.spans[name]
				require.Equal(t, []any{span[0] + 80, span[1] + 80}, typed["Wire Field Ranges"].(map[string]any)[name])
				require.Equal(t, owned[span[0]:span[1]], response[span[0]+80:span[1]+80])
			}
			if test.class == 5 {
				require.Equal(t, true, typed["Delete Pending State"])
				require.Equal(t, true, typed["Directory State"])
				for _, key := range []string{"Delete Pending State", "Directory State", "Output Information Type"} {
					require.NotContains(t, typed["Wire Field Ranges"], key, "interpretation is not an additional literal")
				}
			}
			before, err := json.Marshal(info)
			require.NoError(t, err)
			clear(response)
			clear(test.wire)
			require.Equal(t, owned, body["Output Buffer"], "raw output remains an owned source")
			after, err := json.Marshal(info)
			require.NoError(t, err)
			require.Equal(t, before, after)
			for _, child := range node.Children {
				require.Equal(t, test.want[child.Name], stream_parser.GetNodeResult(child), "canonical survives caller mutation")
			}
			// A stateless response has no class selector, even for a familiar length.
			unmatched, err := (&binSMB2{}).consumeFrom(1, smb2QueryResponse(21, 17, 7, 0, owned), 64)
			require.NoError(t, err)
			require.NotContains(t, unmatched, "Output Information Type")
			require.NotContains(t, unmatched["Body"], test.entry)
		})
	}
}

func TestSMB2QueryOutputFixedGrammarBoundaries(t *testing.T) {
	for _, class := range []byte{5, 7} {
		valid := smb2StandardOutput()
		entry := "FileStandardInformation"
		if class == 7 {
			valid, entry = []byte{1, 2, 3, 4}, "FileEaInformation"
		}
		for size := 0; size <= len(valid)+2; size++ {
			wire := append(bytes.Clone(valid), 0, 0)[:size]
			_, err := stream_parser.DecodeSMB2QueryInfoOutput(wire, 1, class)
			_, canonicalErr := parser.ParseBinary(&liveBoundedReader{Reader: bytes.NewReader(wire), bits: uint64(len(wire)) * 8}, "application-layer.smb2", entry)
			if size == len(valid) {
				require.NoError(t, err)
				require.NoError(t, canonicalErr)
			} else {
				require.Error(t, err, "%s size%d", entry, size)
				require.Error(t, canonicalErr, "%s size%d", entry, size)
			}
		}
	}
	for _, selector := range [][2]byte{{0, 5}, {2, 5}, {3, 7}, {1, 0}, {1, 22}, {1, 34}, {255, 255}} {
		_, err := stream_parser.DecodeSMB2QueryInfoOutput(smb2StandardOutput(), selector[0], selector[1])
		require.Error(t, err, "unsupported selectors cannot be inferred from buffer length")
	}
	for _, eof := range []uint64{0, math.MaxInt64, uint64(1) << 63, math.MaxUint64} {
		wire := smb2StandardOutput()
		binary.LittleEndian.PutUint64(wire, math.MaxUint64) // Signed literal, cluster size unverified.
		binary.LittleEndian.PutUint64(wire[8:], eof)
		decoded, err := stream_parser.DecodeSMB2QueryInfoOutput(wire, 1, 5)
		if eof > math.MaxInt64 {
			require.Error(t, err, "negative EOF violates the selected record grammar")
		} else {
			require.NoError(t, err)
			require.Equal(t, int64(-1), decoded["Allocation Size"])
			require.Equal(t, int64(eof), decoded["End Of File"])
		}
	}
}

func TestSMB2QueryOutputBooleanLiteralTable(t *testing.T) {
	for _, deleted := range []byte{0, 1, 2, 255} {
		for _, directory := range []byte{0, 1, 2, 255} {
			wire := smb2StandardOutput()
			wire[20], wire[21] = deleted, directory
			decoded, err := stream_parser.DecodeSMB2QueryInfoOutput(wire, 1, 5)
			require.NoError(t, err)
			require.Equal(t, deleted, decoded["Delete Pending"])
			require.Equal(t, directory, decoded["Directory"])
			require.Equal(t, deleted != 0, decoded["Delete Pending State"])
			require.Equal(t, directory != 0, decoded["Directory State"])
			require.Equal(t, uint16(0xcafe), decoded["Reserved"])
		}
	}
}

func TestSMB2QueryOutputRequestIdentityReuseAndAsync(t *testing.T) {
	for _, width := range []int{1, 7, 64, 4096} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			session, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer session.Close("test")
			feed := func(dir int, wire []byte) map[string]any {
				var events []*ProtocolEvent
				for at := 0; at < len(wire); at += width {
					result := session.Feed(dir, time.Unix(1, 0), wire[at:min(at+width, len(wire))])
					if result.Err != nil {
						require.Equal(t, ErrNeedMore, result.Err.Kind)
						require.True(t, result.NeedMore)
					}
					events = append(events, result.Events...)
				}
				require.Len(t, events, 1)
				require.Equal(t, "decoded", events[0].Status)
				require.Equal(t, wire, events[0].Raw)
				return events[0].Session
			}
			for _, class := range []byte{5, 7} { // The completed MID may be reused with a different selector.
				feed(0, smb2QueryRequest(21, 17, 7, 1, class, nil))
				output, entry := smb2StandardOutput(), "FileStandardInformation"
				if class == 7 {
					output, entry = []byte{0x44, 0x33, 0x22, 0x11}, "FileEaInformation"
				}
				for _, wrong := range []struct {
					dir      int
					mid, sid uint64
					tid      uint32
				}{{0, 21, 17, 7}, {1, 22, 17, 7}, {1, 21, 18, 7}, {1, 21, 17, 8}} {
					info := feed(wrong.dir, smb2QueryResponse(wrong.mid, wrong.sid, wrong.tid, 0, output))
					require.NotContains(t, info, "Output Information Type")
					require.NotContains(t, info["Body"], entry)
				}
				feed(1, smb2AsyncWire(smb2ErrorResponse(16, 21, 17, 0, 0x103, nil), 99))
				wrongAsync := feed(1, smb2AsyncWire(smb2QueryResponse(math.MaxUint64, 17, 0, 0, output), 100))
				require.NotContains(t, wrongAsync, "Output Information Type")
				info := feed(1, smb2AsyncWire(smb2QueryResponse(math.MaxUint64, 17, 0, 0, output), 99))
				require.Equal(t, entry, info["Output Information Type"])
				require.Equal(t, uint32(7), info["Request Tree ID"])
				require.NotContains(t, info["Header"], "Tree ID")
				require.Empty(t, session.(*captureSession).f.smb2.pending)
				require.Empty(t, session.(*captureSession).f.smb2.async)
			}
			session.Close("FIN")
			require.Zero(t, session.Stats().BufferedBytes)
		})
	}
}

func TestSMB2QueryOutputBadTypedRecordPreservesEnvelopeAndCompound(t *testing.T) {
	for _, status := range []uint32{0, 0x80000005} {
		for _, output := range [][]byte{nil, {1, 2, 3}, append(smb2StandardOutput(), 0)} {
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, smb2CompoundWire(smb2QueryRequest(21, 17, 7, 1, 5, nil), smb2QueryRequest(22, 17, 7, 1, 7, nil)), 64)
			require.NoError(t, err)
			wire := smb2CompoundWire(smb2QueryResponse(21, 17, 7, status, output), smb2QueryResponse(22, 17, 7, 0, []byte{1, 2, 3, 4}))
			info, err := state.consumeFrom(1, wire, 64)
			require.NoError(t, err, "a malformed selected record cannot erase proven outer commands")
			commands := info["Commands"].([]map[string]any)
			require.Len(t, commands, 2)
			require.NotContains(t, commands[0], "Output Information Type")
			require.Contains(t, commands[0]["Output Content Status"], "opaque")
			require.Equal(t, status == 0x80000005, commands[0]["Output Content Partial"])
			require.NotContains(t, commands[0]["Body"], "FileStandardInformation")
			if len(output) != 0 {
				require.Equal(t, output, commands[0]["Body"].(map[string]any)["Output Buffer"])
			}
			require.Equal(t, "FileEaInformation", commands[1]["Output Information Type"])
			typed := commands[1]["Body"].(map[string]any)["FileEaInformation"].(map[string]any)
			span := typed["Wire Field Ranges"].(map[string]any)["EA Size"].([]any)
			require.Equal(t, []byte{1, 2, 3, 4}, wire[span[0].(uint64):span[1].(uint64)], "compound offset is added exactly once")
			require.Empty(t, state.pending, "a complete final envelope finishes its own request even when typed content is bad")
		}
		state := &binSMB2{client: 0}
		_, err := state.consumeFrom(0, smb2QueryRequest(21, 17, 7, 1, 5, nil), 64)
		require.NoError(t, err)
		good := smb2QueryResponse(21, 17, 7, status, smb2StandardOutput())
		bad := bytes.Clone(good[:len(good)-1])
		bad = smb2TCP(bad[4:])
		_, err = state.consumeFrom(1, bad, 64)
		require.Error(t, err, "a physically incomplete declared output cannot finish the request")
		require.Len(t, state.pending, 1)
		info, err := state.consumeFrom(1, good, 64)
		require.NoError(t, err)
		require.Equal(t, "FileStandardInformation", info["Output Information Type"])
		require.Equal(t, status == 0x80000005, info["Output Content Partial"])
	}
}

func TestSMB2QueryOutputTCPRecoveryAndGenerationReuse(t *testing.T) {
	request := smb2QueryRequest(21, 17, 7, 1, 5, nil)
	response := smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput())
	eaRequest := smb2QueryRequest(21, 17, 7, 1, 7, nil)
	eaResponse := smb2QueryResponse(21, 17, 7, 0, []byte{1, 2, 3, 4})
	cseq, sseq := uint32(100+len(request)), uint32(1000+len(response))
	steps := []tcpStep{
		{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true},
		{seq: 100, data: string(request[:7])}, {seq: 164, data: string(request[64:])},
		{seq: 164, data: string(request[64:])}, {seq: 107, data: string(request[7:64])},
		{seq: 1000, reverse: true, data: string(response[:80])}, {seq: 1090, reverse: true, data: string(response[90:])},
		{seq: 1080, reverse: true, data: string(response[80:90])},
		{seq: cseq, data: string(eaRequest)}, {seq: sseq, reverse: true, data: string(eaResponse)},
		{seq: cseq + uint32(len(eaRequest)), fin: true}, {seq: sseq + uint32(len(eaResponse)), reverse: true, rst: true},
		{seq: 5000, syn: true}, {seq: 8000, reverse: true, syn: true},
		{seq: 8001, reverse: true, data: string(response)}, // Old MID/SID/TID in a new generation has no selector.
		{seq: 5001, data: string(eaRequest)}, {seq: 8001 + uint32(len(response)), reverse: true, data: string(eaResponse)},
		{seq: 5001 + uint32(len(eaRequest)), fin: true},
	}
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, ipv6 := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/ipv6%t", workers, deferred, ipv6), func(t *testing.T) {
					events, stats, err := binReplay(t, binTestPcap(t, steps, 445, ipv6, true), workers, WithBinParserDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, 7, "TCP retransmissions are not extra messages")
					wantRaw := [][]byte{request, response, eaRequest, eaResponse, response, eaRequest, eaResponse}
					for index, event := range events {
						_, err := event.GetFields()
						require.NoError(t, err)
						require.Equal(t, wantRaw[index], event.Raw, "complete ordered source%d", index)
						require.Equal(t, "smb2", event.Protocol)
					}
					require.Equal(t, "FileStandardInformation", events[1].Session["Output Information Type"])
					require.Equal(t, "FileEaInformation", events[3].Session["Output Information Type"])
					require.NotContains(t, events[4].Session, "Output Information Type")
					require.NotContains(t, events[4].Session, "Request Info Type")
					require.Equal(t, "FileEaInformation", events[6].Session["Output Information Type"])
					require.NotEqual(t, events[0].FlowID, events[4].FlowID)
					require.Equal(t, events[4].FlowID, events[6].FlowID)
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
	// A still-missing middle is not a complete selected record, even at FIN.
	lost := []tcpStep{{seq: 99, syn: true}, {seq: 999, reverse: true, syn: true}, {seq: 100, data: string(request)},
		{seq: 1000, reverse: true, data: string(response[:80])}, {seq: 1090, reverse: true, data: string(response[90:])},
		{seq: 1000 + uint32(len(response)), reverse: true, fin: true}}
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, allowIncomplete := range []bool{false, true} {
				events, stats, err := binReplay(t, binTestPcap(t, lost, 445, false, false), workers, WithBinParserDeferred(deferred), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allowIncomplete}))
				if allowIncomplete {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "TCP stream closed with an unfilled sequence gap (14 buffered bytes)")
				}
				require.Len(t, events, 3, "retain the request, exact incomplete prefix and separate TCP gap diagnostic")
				require.Equal(t, "incomplete", events[1].Status)
				require.Equal(t, response[:80], events[1].Raw)
				require.Contains(t, events[1].Summary, "exact message boundary was not reached")
				require.Nil(t, events[1].SourceBytes.CompletedIn)
				require.NotEmpty(t, events[1].SourceBytes.PacketRefs)
				require.Equal(t, "incomplete", events[2].Status)
				require.Empty(t, events[2].Raw, "undelivered pending TCP bytes are not fabricated as a contiguous PDU")
				require.Equal(t, "TCP stream closed with an unfilled sequence gap (14 buffered bytes)", events[2].Summary)
				require.EqualValues(t, 2, stats.Incomplete)
				for _, event := range events {
					require.NotContains(t, event.Session, "Output Information Type", "a gap cannot be filled with guessed file values")
				}
			}
		}
	}
}

func TestSMB2QueryOutputTypedCaptureDomainIsolation(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		t.Run(fmt.Sprintf("vlan%t", vlan), func(t *testing.T) {
			var events []*ProtocolEvent
			analyzer, err := NewPacketAnalyzer(WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
			require.NoError(t, err)
			defer analyzer.Close()
			wires := [][]byte{smb2QueryRequest(21, 17, 7, 1, 5, nil), smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput()),
				smb2QueryRequest(21, 17, 7, 1, 7, nil), smb2QueryResponse(21, 17, 7, 0, []byte{1, 2, 3, 4}),
				smb2QueryResponse(21, 17, 7, 0, smb2StandardOutput())}
			domains, directions := []int{0, 1, 1, 1, 0}, []int{0, 1, 0, 1, 1}
			seq := [2][2]uint32{{1, 1}, {1, 1}}
			for index, wire := range wires {
				domain, dir := domains[index], directions[index]
				src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
				sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
				if dir == 1 {
					src, dst, sport, dport = dst, src, dport, sport
				}
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
				ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
				tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][dir], ACK: true, PSH: true, Window: 4096}
				seq[domain][dir] += uint32(len(wire))
				require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
				serial := []gopacket.SerializableLayer{eth, ip, tcp, gopacket.Payload(wire)}
				iface := domain
				if vlan {
					iface, eth.EthernetType = 0, layers.EthernetTypeDot1Q
					serial = []gopacket.SerializableLayer{eth, &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}, ip, tcp, gopacket.Payload(wire)}
				}
				buffer := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
				packet := buffer.Bytes()
				require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(index+100), 0), CaptureLength: len(packet), Length: len(packet), InterfaceIndex: iface}, layers.LinkTypeEthernet))
				clear(packet)
			}
			require.NoError(t, analyzer.Close())
			require.Len(t, events, 5)
			require.NotContains(t, events[1].Session, "Output Information Type", "other interface/VLAN cannot lend its selector")
			require.Equal(t, "FileEaInformation", events[3].Session["Output Information Type"])
			require.Equal(t, "FileStandardInformation", events[4].Session["Output Information Type"])
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
		})
	}
}
