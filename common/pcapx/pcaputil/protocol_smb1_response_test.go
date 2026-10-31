package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Independent MS-SMB 2.2.4.5.2.1 construction; no captured security token.
func smb1ExtendedResponse(blobLength int) []byte {
	w := make([]byte, 89+blobLength)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x72})
	w[13] = 0x98
	binary.LittleEndian.PutUint16(w[14:16], 0xc853)
	binary.LittleEndian.PutUint16(w[16:18], 0x1122)
	binary.LittleEndian.PutUint16(w[30:32], 0x7788)
	binary.LittleEndian.PutUint16(w[34:36], 7)
	w[36] = 17
	binary.LittleEndian.PutUint16(w[37:39], 1)
	w[39] = 3
	binary.LittleEndian.PutUint16(w[40:42], 50)
	binary.LittleEndian.PutUint16(w[42:44], 1)
	binary.LittleEndian.PutUint32(w[44:48], 65535)
	binary.LittleEndian.PutUint32(w[48:52], 65536)
	binary.LittleEndian.PutUint32(w[52:56], 0x11223344)
	binary.LittleEndian.PutUint32(w[56:60], 0x8000005c)
	binary.LittleEndian.PutUint64(w[60:68], 0xfedcba9876543210)
	binary.LittleEndian.PutUint16(w[68:70], 0xffc4) // signed -60 minutes.
	binary.LittleEndian.PutUint16(w[71:73], uint16(16+blobLength))
	for i := 73; i < len(w); i++ {
		w[i] = byte(i * 13)
	}
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func TestSMB1ExtendedResponseCaptureDomainSelectionIsolation(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		t.Run(fmt.Sprint(vlan), func(t *testing.T) {
			var events []*ProtocolEvent
			analyzer, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
			require.NoError(t, err)
			defer analyzer.Close()
			req, resp := smb1ExtendedOffer(), smb1ExtendedResponse(64)
			wires := [][]byte{req, resp, resp, req, resp, req, resp}
			domains, dirs := []int{0, 1, 0, 1, 1, 2, 2}, []int{0, 1, 1, 0, 1, 0, 1}
			seq := [3][2]uint32{{1, 1}, {1, 1}, {1, 1}}
			for i, wire := range wires {
				domain, dir := domains[i], dirs[i]
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
					iface = 0
					eth.EthernetType = layers.EthernetTypeDot1Q
					serial = []gopacket.SerializableLayer{eth, &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}, ip, tcp, gopacket.Payload(wire)}
				}
				buffer := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
				packet := buffer.Bytes()
				require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(packet), Length: len(packet), InterfaceIndex: iface}, layers.LinkTypeEthernet))
				clear(packet)
			}
			require.NoError(t, analyzer.Close())
			require.Len(t, events, 7)
			for i, event := range events {
				require.Equal(t, wires[i], event.Raw)
				_, err := event.GetFields()
				require.NoError(t, err)
			}
			for _, i := range []int{1, 4} {
				require.NotContains(t, events[i].Session, "Selected Dialect Name")
				require.Zero(t, events[i].ResponseTo)
			}
			for _, pair := range [][2]int{{0, 2}, {5, 6}} {
				require.Equal(t, "NT LM 0.12", events[pair[1]].Session["Selected Dialect Name"])
				require.Equal(t, events[pair[0]].ID, events[pair[1]].ResponseTo)
				require.Equal(t, events[pair[0]].FlowID, events[pair[1]].FlowID)
			}
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
			require.NotEqual(t, events[1].FlowID, events[5].FlowID)
		})
	}
}

func smb1ExtendedOffer() []byte {
	w := smb1UpgradeOffer("PC NETWORK PROGRAM 1.0", "NT LM 0.12", "SMB 2.002")
	binary.LittleEndian.PutUint16(w[16:18], 0x1122)
	binary.LittleEndian.PutUint16(w[30:32], 0x7788)
	return w
}

func smb1FeedComplete(t *testing.T, s ProtocolSession, dir int, w []byte) *ProtocolEvent {
	t.Helper()
	r := s.Feed(dir, time.Unix(100, 0), w)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, "smb", e.Protocol)
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, w, e.Raw)
	require.Equal(t, len(w), e.Length)
	return e
}

func TestSMB1ExtendedResponseCanonicalNativeAndOwnedSources(t *testing.T) {
	for _, blob := range []int{0, 1, 64, 65519} {
		t.Run(fmt.Sprint(blob), func(t *testing.T) {
			w := smb1ExtendedResponse(blob)
			original := bytes.Clone(w)
			want, err := parser.ParseStructured(w, "application-layer.cifs", "CIFSDirectTCPNegotiateExtendedResponse")
			require.NoError(t, err)
			got, err := decodeSMB1NegotiateResponseFields(w, ParserBudget{})
			require.NoError(t, err)
			require.Equal(t, want["fields"], got)
			body := got["Body"].(map[string]any)
			require.Equal(t, uint64(0xfedcba9876543210), body["System Time"])
			require.Equal(t, int16(-60), body["Server Time Zone"])
			require.Equal(t, uint16(blob+16), body["Byte Count"])
			require.Equal(t, original[73:89], body["Server GUID"])
			require.Equal(t, original[89:], body["Security Blob"])
			require.Equal(t, []any{uint64(60), uint64(68)}, body["Wire Field Ranges"].(map[string]any)["System Time"])
			require.Equal(t, []any{uint64(68), uint64(70)}, body["Wire Field Ranges"].(map[string]any)["Server Time Zone"])
			require.NotContains(t, got, "Selected Dialect Name")
			require.Equal(t, false, got["Dialect Negotiated"])
			bare, err := parser.ParseStructured(w[4:], "application-layer.cifs", "CIFSNegotiateExtendedResponse")
			require.NoError(t, err)
			bf := bare["fields"].(map[string]any)["Body"].(map[string]any)
			require.Equal(t, body["System Time"], bf["System Time"])
			require.Equal(t, []any{uint64(56), uint64(64)}, bf["Wire Field Ranges"].(map[string]any)["System Time"])
			clear(w)
			require.Equal(t, original[73:89], body["Server GUID"])
			body["Server GUID"].([]byte)[0] ^= 1
			fresh, err := decodeSMB1NegotiateResponseFields(original, DefaultParserBudget())
			require.NoError(t, err)
			require.Equal(t, original[73:89], fresh["Body"].(map[string]any)["Server GUID"])
		})
	}
}

func TestSMB1ExtendedResponseRequiresExactProfileAndCompleteRecord(t *testing.T) {
	w := smb1ExtendedResponse(64)
	for cut := 8; cut < len(w); cut++ {
		require.Equal(t, ProbeNeedMore, probeSMB1NegotiateExtendedResponse(w[:cut], 65608).Verdict, "prefix%d", cut)
		_, err := decodeSMB1NegotiateResponseFields(w[:cut], DefaultParserBudget())
		require.Error(t, err, "prefix%d", cut)
	}
	require.Equal(t, ProbeAccept, probeSMB1NegotiateExtendedResponse(w, 65608).Verdict)
	require.Equal(t, ProbeAccept, probeSMB1NegotiateExtendedResponse(append(bytes.Clone(w), w...), 65608).Verdict, "probe admits only first complete record")
	_, err := decodeSMB1NegotiateResponseFields(append(bytes.Clone(w), w...), DefaultParserBudget())
	require.Error(t, err, "canonical bounded entry cannot eat next record")
	for _, tc := range []struct {
		name string
		edit func([]byte)
	}{
		{"signature", func(b []byte) { b[6] ^= 1 }}, {"command", func(b []byte) { b[8] = 0x73 }},
		{"request", func(b []byte) { b[13] &= 0x7f }}, {"status", func(b []byte) { b[9] = 1 }},
		{"flags2", func(b []byte) { b[15] &= 0xf7 }}, {"word-count", func(b []byte) { b[36] = 16 }},
		{"capability", func(b []byte) { b[59] &= 0x7f }}, {"challenge", func(b []byte) { b[70] = 8 }},
		{"byte-count", func(b []byte) { b[71]-- }}, {"small-byte-count", func(b []byte) { binary.LittleEndian.PutUint16(b[71:73], 15) }},
		{"direct-zero", func(b []byte) { b[0] = 1 }}, {"direct-length", func(b []byte) { b[3]-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Clone(w)
			tc.edit(bad)
			require.Equal(t, ProbeReject, probeSMB1NegotiateExtendedResponse(bad, 65608).Verdict)
			_, err := decodeSMB1NegotiateResponseFields(bad, DefaultParserBudget())
			require.Error(t, err)
		})
	}
	_, err = decodeSMB1NegotiateFields(w, DefaultParserBudget())
	require.Error(t, err, "old request contract remains request only")
	for _, budget := range []ParserBudget{{MaxFrameBytes: len(w) - 1}, {MaxMessageBytes: len(w) - 1}, {MaxBufferedBytes: len(w) - 1}, {MaxRecursionDepth: 2}, {MaxCollectionElements: -1}} {
		_, err := decodeSMB1NegotiateResponseFields(w, budget)
		var p *ProtocolError
		require.ErrorAs(t, err, &p)
		require.Equal(t, ErrResourceExceeded, p.Kind)
	}
}

func TestProtocolSessionSMB1ExtendedResponseSplitRoleAndContext(t *testing.T) {
	for _, responseOnly := range []bool{false, true} {
		for _, chunk := range []int{1, 7, 64, 153, 4096} {
			t.Run(fmt.Sprintf("orphan%t/chunk%d", responseOnly, chunk), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				defer s.Close("FIN")
				var request *ProtocolEvent
				if !responseOnly {
					request = smb1FeedComplete(t, s, 0, smb1ExtendedOffer())
				}
				w := smb1ExtendedResponse(64)
				require.Equal(t, ProbeAccept, s.Probe(w).Verdict)
				var events []*ProtocolEvent
				for at := 0; at < len(w); at += chunk {
					r := s.Feed(1, time.Unix(101, 0), w[at:min(at+chunk, len(w))])
					require.Equal(t, min(at+chunk, len(w))-at, r.Consumed)
					if at+chunk >= len(w) {
						require.Nil(t, r.Err, "final fragment must reach the exact response boundary")
					}
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
						require.True(t, r.NeedMore)
					}
					events = append(events, r.Events...)
				}
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, w, e.Raw)
				require.Equal(t, len(w), e.Length)
				require.Equal(t, "CIFSDirectTCPNegotiateExtendedResponse", e.Entry)
				require.Equal(t, "cifs-negotiate-extended-response", e.Profile)
				require.Equal(t, 0, s.(*captureSession).f.smb2.client)
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, int16(-60), fields["Body"].(map[string]any)["Server Time Zone"])
				if responseOnly {
					require.Zero(t, e.ResponseTo)
					require.NotContains(t, fields, "Selected Dialect Name")
				} else {
					require.Equal(t, request.ID, e.ResponseTo)
					require.Equal(t, "NT LM 0.12", fields["Selected Dialect Name"])
					require.Equal(t, []byte("NT LM 0.12"), fields["Selected Dialect Raw Name Bytes"])
				}
				for _, name := range []string{"Selected Dialect Name", "Selected Dialect Raw Name Bytes", "Request Event ID", "Request PDU SHA256", "Matched Request"} {
					require.NotContains(t, fields["Wire Field Ranges"], name)
				}
				fields["Body"].(map[string]any)["Server GUID"].([]byte)[0] ^= 1
				e.Session["Body"].(map[string]any)["System Time"] = uint64(0)
				fresh, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, uint64(0xfedcba9876543210), fresh["Body"].(map[string]any)["System Time"])
				require.Equal(t, false, fresh["Session Established"])
				require.Equal(t, false, fresh["Authentication Verified"])
				require.Empty(t, s.(*captureSession).f.smb2.pending, "legacy MID never enters SMB2 namespace")
			})
		}
	}
}

func TestSMB1ExtendedResponseModernCarrierPhaseIsolation(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	smb1FeedComplete(t, s, 0, smb1ExtendedOffer())
	modern := s.Feed(0, time.Unix(101, 0), smb2NegotiateReq(0x0311))
	require.Nil(t, modern.Err)
	require.Len(t, modern.Events, 1)
	require.True(t, s.(*captureSession).f.smb2.smb1Modern)
	require.Nil(t, s.(*captureSession).f.smb2.smb1)
	modern = s.Feed(1, time.Unix(102, 0), smb2NegotiateResp(0x0311))
	require.Nil(t, modern.Err)
	require.Equal(t, uint16(0x0311), s.(*captureSession).f.smb2.dialect)
	late := smb1FeedComplete(t, s, 1, smb1ExtendedResponse(0))
	require.Zero(t, late.ResponseTo)
	require.NotContains(t, late.Session, "Selected Dialect Name")
	require.Contains(t, late.Session["Selection Status"], "SMB2 carrier already observed")
	require.Equal(t, uint16(0x0311), s.(*captureSession).f.smb2.dialect)
	// A decoded legacy selection cannot supply a modern request identity.
	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s2.Close("FIN")
	smb1FeedComplete(t, s2, 0, smb1ExtendedOffer())
	smb1FeedComplete(t, s2, 1, smb1ExtendedResponse(0))
	reply := smb2NegotiateResp(0x0311)
	binary.LittleEndian.PutUint64(reply[28:36], 7)
	unmatched := s2.Feed(1, time.Unix(103, 0), reply)
	require.Nil(t, unmatched.Err)
	require.Equal(t, true, unmatched.Events[0].Session["Unmatched"])
	require.Zero(t, s2.(*captureSession).f.smb2.dialect)
}

func TestSMB1BodyRefusedModernCarrierCannotBorrowOldOffer(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	smb1FeedComplete(t, s, 0, smb1ExtendedOffer())
	state := s.(*captureSession).f.smb2
	bad := smb2UnsupportedSetup()
	partial := s.Feed(0, time.Unix(100, 0), bad[:68])
	require.Empty(t, partial.Events)
	require.False(t, state.smb1Modern, "a partial record has no complete-envelope authority")
	require.NotNil(t, state.smb1.offer)
	r := s.Feed(0, time.Unix(101, 0), bad[68:])
	require.NotNil(t, r.Err)
	require.Equal(t, ErrUnsupportedVersion, r.Err.Kind)
	require.Len(t, r.Events, 1)
	require.Equal(t, "smb2-record-body-diagnostic", r.Events[0].Profile)
	require.Equal(t, "headers", r.Events[0].Completeness)
	require.Equal(t, false, r.Events[0].Session["State Updated"])
	require.Empty(t, state.pending)
	require.Empty(t, state.async)
	require.Empty(t, state.files)
	require.Zero(t, state.dialect)
	late := smb1FeedComplete(t, s, 1, smb1ExtendedResponse(0))
	require.NotContains(t, late.Session, "Selected Dialect Name")
	require.Zero(t, late.ResponseTo)
	require.True(t, state.smb1Modern)
	require.Empty(t, state.pending)
	require.Empty(t, state.async)
	require.Empty(t, state.files)
	require.Zero(t, state.dialect)
}

func TestSMB1ExtendedResponseNativeFrameBudgetIsExplicit(t *testing.T) {
	for _, budgetCase := range []struct {
		name        string
		budget      ParserBudget
		probeAccept bool
	}{
		{"frame", ParserBudget{MaxFrameBytes: 128}, false},
		{"message", ParserBudget{MaxMessageBytes: 128}, false},
		{"buffer", ParserBudget{MaxMessageBytes: 128, MaxBufferedBytes: 128}, false},
		{"depth", ParserBudget{MaxRecursionDepth: 2}, true},
	} {
		for _, chunk := range []int{64, 4096} {
			t.Run(fmt.Sprintf("%s/%d", budgetCase.name, chunk), func(t *testing.T) {
				s, err := NewProtocolSession(budgetCase.budget)
				require.NoError(t, err)
				defer s.Close("FIN")
				w := smb1ExtendedResponse(64)
				if budgetCase.probeAccept {
					require.Equal(t, ProbeAccept, s.Probe(w).Verdict)
				} else {
					require.NotEqual(t, ProbeAccept, s.Probe(w).Verdict, "public probe respects caller byte budget")
				}
				var events []*ProtocolEvent
				for at := 0; at < len(w); at += chunk {
					r := s.Feed(1, time.Unix(100, 0), w[at:min(at+chunk, len(w))])
					events = append(events, r.Events...)
				}
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "limited", e.Status)
				require.Equal(t, "smb", e.Protocol)
				_, err = e.GetFields()
				var p *ProtocolError
				require.ErrorAs(t, err, &p)
				require.Equal(t, ErrResourceExceeded, p.Kind)
				require.NotContains(t, e.Session, "Body")
				require.Zero(t, e.ResponseTo)
				e.Status, e.Error, e.Profile, e.Protocol = "decoded", "", "caller edited", "http"
				e.Fields, e.Session, e.Structured = map[string]any{"fabricated": true}, map[string]any{"Body": "fabricated"}, map[string]any{"fields": map[string]any{"fabricated": true}}
				_, err = e.GetFields()
				require.ErrorAs(t, err, &p)
				require.Equal(t, ErrResourceExceeded, p.Kind)
			})
		}
	}
}

func TestSMB1ExtendedResponseTCPRecoveryAndGenerationReuse(t *testing.T) {
	req, resp := smb1ExtendedOffer(), smb1ExtendedResponse(64)
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true},
		{seq: 100, data: string(req[:8])}, {seq: 164, data: string(req[64:])}, {seq: 108, data: string(req[8:64])},
		{seq: 1000, reverse: true, data: string(resp[:64])}, {seq: 1100, reverse: true, data: string(resp[100:])}, {seq: 1064, reverse: true, data: string(resp[64:100])},
		{seq: 1000, reverse: true, data: string(resp)}, // TCP retransmission, not another response.
		{seq: 100 + uint32(len(req)), data: string(req)}, {seq: 1000 + uint32(len(resp)), reverse: true, data: string(resp)},
		{seq: 100 + 2*uint32(len(req)), rst: true},
		{seq: 9000, syn: true}, {seq: 12000, syn: true, reverse: true},
		{seq: 9001, data: string(req)}, {seq: 12001, reverse: true, data: string(resp)}}
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, ipv6 := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/ipv6%t", workers, deferred, ipv6), func(t *testing.T) {
					events, stats, err := binReplay(t, binTestPcap(t, steps, 445, ipv6, true), workers, WithBinParserDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, 6, "retransmission does not duplicate a PDU")
					for i, event := range events {
						want := req
						if i%2 == 1 {
							want = resp
						}
						require.Equal(t, want, event.Raw)
						_, err := event.GetFields()
						require.NoError(t, err)
					}
					for _, i := range []int{1, 5} {
						require.Equal(t, "NT LM 0.12", events[i].Session["Selected Dialect Name"])
						require.Equal(t, events[i-1].ID, events[i].ResponseTo)
					}
					require.NotContains(t, events[3].Session, "Selected Dialect Name")
					require.Zero(t, events[3].ResponseTo)
					require.NotEqual(t, events[0].FlowID, events[4].FlowID)
					require.Equal(t, events[4].FlowID, events[5].FlowID)
					require.Len(t, events[1].SourceBytes.PacketRefs, 3)
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
	missing := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true}, {seq: 100, data: string(req)},
		{seq: 1000, reverse: true, data: string(resp[:64])}, {seq: 1100, reverse: true, data: string(resp[100:])},
		{seq: 1000 + uint32(len(resp)), reverse: true, fin: true}}
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, allow := range []bool{false, true} {
				events, stats, err := binReplay(t, binTestPcap(t, missing, 445, false, false), workers, WithBinParserDeferred(deferred), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allow}))
				if allow {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "TCP stream closed with an unfilled sequence gap (53 buffered bytes)")
				}
				require.Len(t, events, 3)
				require.Equal(t, resp[:64], events[1].Raw)
				require.Nil(t, events[1].SourceBytes.CompletedIn)
				require.Empty(t, events[2].Raw, "pending bytes are not fabricated into a contiguous response")
				require.EqualValues(t, 2, stats.Incomplete)
				for _, event := range events {
					require.NotContains(t, event.Session, "Selected Dialect Name")
				}
			}
		}
	}
}

func TestSMB1NegotiateStateChargesOwnedCapacityBeforeCommit(t *testing.T) {
	names := make([][]byte, 1, 1024)
	names[0] = make([]byte, 1, 2048)
	state := &smb1NegotiateState{offer: &smb1NegotiateOffer{names: names}}
	require.Equal(t, int64(128+128+1024*24+2048), state.ownedBytes())
	var dialects []string
	for i := 0; i < 257; i++ {
		dialects = append(dialects, "x")
	}
	w := smb1UpgradeOffer(dialects...)
	// Codec limits are deliberately larger than the aggregate retained-state
	// budget, so only the native reservation gate can reject this observation.
	a := &binParser{budget: DefaultParserBudget(), config: BinParserConfig{MaxBufferedBytes: 1024}}
	f := &binFlow{a: a}
	f.smb2 = &binSMB2{flow: f}
	e := &ProtocolEvent{Raw: w, Entry: "CIFSDirectTCP"}
	err := f.consumeSMB1Negotiate(0, e)
	var p *ProtocolError
	require.ErrorAs(t, err, &p)
	require.Equal(t, ErrResourceExceeded, p.Kind)
	require.Nil(t, f.smb2.smb1)
	require.Zero(t, f.sessionBytes)
	require.Zero(t, a.buffered.Load())
	a.config.MaxBufferedBytes = 1 << 20
	require.NoError(t, f.consumeSMB1Negotiate(0, e))
	require.Len(t, f.smb2.smb1.offer.names, len(dialects))
	require.Equal(t, len(dialects), cap(f.smb2.smb1.offer.names))
	reserved := f.sessionBytes
	require.NoError(t, f.smb2.reserveState(0))
	require.Equal(t, reserved, f.sessionBytes, "same target is an accounting no-op")
	f.closeSession()
	require.Zero(t, a.buffered.Load())
}

func TestSMB1UnselectedResponseFormIsUnsupportedAndCannotSelect(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	smb1FeedComplete(t, s, 0, smb1ExtendedOffer())
	response := smb1ExtendedResponse(64)
	response[15] &= 0xf7 // A nonextended form has its own grammar; this entry cannot decode it.
	result := s.Feed(1, time.Unix(103, 0), response)
	require.Len(t, result.Events, 1)
	e := result.Events[0]
	require.Equal(t, "context-required", e.Status)
	require.Equal(t, "smb-record-diagnostic", e.Profile)
	require.NotContains(t, e.Session, "Selected Dialect Name")
	require.Zero(t, e.ResponseTo)
	require.Equal(t, response[:64], e.Raw)
	require.Nil(t, e.SourceBytes.CompletedIn)
	_, err = e.GetFields()
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrUnsupportedFeature, typed.Kind)
	e.Status, e.Protocol = "decoded", "http"
	e.Fields = map[string]any{"invented": true}
	e.Structured = map[string]any{"fields": e.Fields}
	_, err = e.GetFields()
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrUnsupportedFeature, typed.Kind)
	require.Nil(t, s.(*captureSession).f.smb2, "unsupported body cannot retain a selected negotiation")
}

func TestSMB1UnselectedFormStillRequiresCompleteCommonEnvelope(t *testing.T) {
	for _, cut := range []int{39, 72, 100, 152} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("FIN")
			smb1FeedComplete(t, s, 0, smb1ExtendedOffer())
			wire := smb1ExtendedResponse(64)[:cut]
			wire[15] &= 0xf7
			binary.BigEndian.PutUint32(wire[:4], uint32(len(wire)-4))
			r := s.Feed(1, time.Unix(104, 0), wire)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "malformed", e.Status)
			_, err = e.GetFields()
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Zero(t, e.ResponseTo)
			require.NotContains(t, e.Session, "Selected Dialect Name")
		})
	}
}

func TestSMB1ExtendedResponsePIDMIDAndOneGenerationLifecycle(t *testing.T) {
	for _, which := range []string{"pid-high", "pid-low", "mid", "direction", "index", "wrong-dialect", "request-no-extended", "duplicate-request", "request-after-final", "orphan-first"} {
		t.Run(which, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("FIN")
			req, resp := smb1ExtendedOffer(), smb1ExtendedResponse(0)
			dir := 1
			if which == "orphan-first" {
				smb1FeedComplete(t, s, 1, resp)
			}
			if which == "request-no-extended" {
				req[15] &= 0xf7
			}
			request := smb1FeedComplete(t, s, 0, req)
			switch which {
			case "pid-high":
				resp[16] ^= 1
			case "pid-low":
				resp[30] ^= 1
			case "mid":
				resp[34] ^= 1
			case "direction":
				dir = 0
			case "index":
				binary.LittleEndian.PutUint16(resp[37:39], 0xffff)
			case "wrong-dialect":
				binary.LittleEndian.PutUint16(resp[37:39], 0)
			case "duplicate-request":
				smb1FeedComplete(t, s, 0, req)
			case "request-after-final":
				smb1FeedComplete(t, s, 1, resp)
				smb1FeedComplete(t, s, 0, req)
			}
			e := smb1FeedComplete(t, s, dir, resp)
			require.NotContains(t, e.Session, "Selected Dialect Name")
			require.Equal(t, false, e.Session["Dialect Negotiated"])
			if which == "index" || which == "wrong-dialect" || which == "request-no-extended" {
				require.Equal(t, request.ID, e.ResponseTo)
			} else {
				require.Zero(t, e.ResponseTo)
			}
			if which == "pid-high" || which == "pid-low" || which == "mid" || which == "direction" {
				good := smb1FeedComplete(t, s, 1, smb1ExtendedResponse(0))
				require.Equal(t, request.ID, good.ResponseTo)
				require.Equal(t, "NT LM 0.12", good.Session["Selected Dialect Name"])
			}
		})
	}
}
