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

// Constructed from MS-SMB's fixed words and bounded Unicode layout. Tokens are
// deliberately synthetic and are never interpreted as authentication proof.
func smb1SetupWire(response bool, mid, uid uint16, status uint32) []byte {
	base := 63
	if response {
		base = 47
	}
	w := make([]byte, base)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x73})
	binary.LittleEndian.PutUint32(w[9:13], status)
	binary.LittleEndian.PutUint16(w[14:16], 0xc803)
	binary.LittleEndian.PutUint16(w[16:18], 0x1234)
	binary.LittleEndian.PutUint16(w[30:32], 0x5678)
	binary.LittleEndian.PutUint16(w[32:34], uid)
	binary.LittleEndian.PutUint16(w[34:36], mid)
	w[36], w[37] = 12, 0xff
	if response {
		w[13], w[36] = 0x80, 4
		binary.LittleEndian.PutUint16(w[39:41], 412)
		binary.LittleEndian.PutUint16(w[43:45], 3)
	} else {
		binary.LittleEndian.PutUint16(w[41:43], 12288)
		binary.LittleEndian.PutUint16(w[43:45], 10)
		binary.LittleEndian.PutUint16(w[51:53], 3)
		binary.LittleEndian.PutUint32(w[57:61], 0x80000000)
	}
	w = append(w, 0x91, 0x82, 0x73)
	for _, s := range []string{"test-os", "test-lm"} {
		for _, b := range []byte(s) {
			w = append(w, b, 0)
		}
		w = append(w, 0, 0)
	}
	if !response {
		w = append(w, bytes.Repeat([]byte{0xa7}, 20)...)
	}
	binary.LittleEndian.PutUint16(w[base-2:base], uint16(len(w)-base))
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func smb1SetupSession(t *testing.T) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	return s
}

func TestSMB1SetupCanonicalNativeAndContinuation(t *testing.T) {
	s := smb1SetupSession(t)
	wires := [][]byte{smb1SetupWire(false, 2, 0, 0), smb1SetupWire(true, 2, 2048, 0xc0000016), smb1SetupWire(false, 3, 2048, 0), smb1SetupWire(true, 3, 2048, 0)}
	phases := []string{"request", "continuation required", "continuation request", "success observed"}
	var events []*ProtocolEvent
	for i, w := range wires {
		e := smb1FeedComplete(t, s, i%2, w)
		events = append(events, e)
		require.Equal(t, phases[i], e.Session["Setup Phase"])
		require.Contains(t, e.Summary, "SESSION_SETUP_ANDX")
		require.NotContains(t, e.Summary, "NEGOTIATE")
		canonical, err := parser.ParseStructured(w, "application-layer.cifs", e.Entry)
		require.NoError(t, err)
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, canonical["fields"].(map[string]any)["Body"], fields["Body"])
		require.Equal(t, canonical["fields"].(map[string]any)["Header"], fields["Header"])
		for _, key := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
			require.Equal(t, false, fields[key])
		}
		require.NotContains(t, fields, "Primary Domain")
		require.NotContains(t, fields["Body"], "Primary Domain")
	}
	require.Equal(t, events[0].ID, events[1].ResponseTo)
	require.Equal(t, events[2].ID, events[3].ResponseTo)
	require.Zero(t, events[2].ResponseTo)
	require.Equal(t, events[1].ID, events[2].Session["Continuation Response Event ID"])
	for i := range wires {
		clear(wires[i])
	}
	for _, e := range events {
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, "test-os", fields["Body"].(map[string]any)["Native OS"])
	}
}

func TestSMB1SetupSplitAndCoalescedMessages(t *testing.T) {
	for _, response := range []bool{false, true} {
		for _, chunk := range []int{1, 7, 64, 4096} {
			t.Run(fmt.Sprintf("response%v/chunk%d", response, chunk), func(t *testing.T) {
				s := smb1SetupSession(t)
				w := smb1SetupWire(response, 2, 2048, 0)
				require.Equal(t, ProbeAccept, s.Probe(w).Verdict)
				var events []*ProtocolEvent
				for at := 0; at < len(w); at += chunk {
					r := s.Feed(0, time.Unix(100, 0), w[at:min(at+chunk, len(w))])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
					if at+chunk < len(w) {
						require.Empty(t, r.Events)
					}
					events = append(events, r.Events...)
				}
				require.Len(t, events, 1)
				require.Equal(t, "decoded", events[0].Status)
				require.Equal(t, w, events[0].Raw)
				require.Zero(t, events[0].ResponseTo)
			})
		}
	}
	s := smb1SetupSession(t)
	a, b := smb1SetupWire(false, 2, 0, 0), smb1SetupWire(false, 3, 0, 0)
	r := s.Feed(0, time.Unix(100, 0), append(bytes.Clone(a), b...))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 2)
	require.Equal(t, a, r.Events[0].Raw)
	require.Equal(t, b, r.Events[1].Raw)
}

func TestSMB1SetupCannotBorrowOrphanDuplicateOrReusedContext(t *testing.T) {
	for _, kind := range []string{"orphan-first", "duplicate-request", "duplicate-response", "repeat-after-challenge", "completed-key-reuse", "foreign-UID", "foreign-PID", "foreign-direction"} {
		t.Run(kind, func(t *testing.T) {
			s := smb1SetupSession(t)
			req, challenge := smb1SetupWire(false, 2, 0, 0), smb1SetupWire(true, 2, 2048, 0xc0000016)
			if kind == "orphan-first" {
				e := smb1FeedComplete(t, s, 1, challenge)
				require.Zero(t, e.ResponseTo)
			}
			r := smb1FeedComplete(t, s, 0, req)
			if kind == "duplicate-request" {
				smb1FeedComplete(t, s, 0, req)
			}
			if kind == "foreign-PID" || kind == "foreign-direction" || kind == "foreign-UID" {
				bad := bytes.Clone(challenge)
				dir := 1
				if kind == "foreign-PID" {
					bad[16] ^= 1
				}
				if kind == "foreign-direction" {
					dir = 0
				}
				if kind == "foreign-UID" {
					binary.LittleEndian.PutUint16(bad[32:34], 0)
				}
				e := smb1FeedComplete(t, s, dir, bad)
				require.Zero(t, e.ResponseTo)
			}
			c := smb1FeedComplete(t, s, 1, challenge)
			if kind == "orphan-first" || kind == "duplicate-request" {
				require.Zero(t, c.ResponseTo)
			} else {
				require.Equal(t, r.ID, c.ResponseTo)
			}
			if kind == "duplicate-response" {
				e := smb1FeedComplete(t, s, 1, challenge)
				require.Zero(t, e.ResponseTo)
			}
			if kind == "repeat-after-challenge" {
				smb1FeedComplete(t, s, 0, req)
			}
			if kind == "completed-key-reuse" {
				smb1FeedComplete(t, s, 0, req)
				e := smb1FeedComplete(t, s, 1, challenge)
				require.Zero(t, e.ResponseTo)
			}
			continuation := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
			if kind == "foreign-PID" || kind == "foreign-direction" || kind == "foreign-UID" {
				require.Equal(t, "continuation request", continuation.Session["Setup Phase"])
				require.Equal(t, c.ID, continuation.Session["Continuation Response Event ID"])
			} else {
				require.NotContains(t, continuation.Session, "Continuation Response Event ID")
			}
		})
	}
}

func TestSMB1SetupUIDIsOpaqueAndMayContinueMultipleRounds(t *testing.T) {
	for _, uid := range []uint16{1, 2048, 0xfffe, 0xffff} {
		t.Run(fmt.Sprint(uid), func(t *testing.T) {
			s := smb1SetupSession(t)
			request := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
			for mid := uint16(2); mid < 5; mid++ {
				response := smb1FeedComplete(t, s, 1, smb1SetupWire(true, mid, uid, 0xc0000016))
				require.Equal(t, request.ID, response.ResponseTo)
				request = smb1FeedComplete(t, s, 0, smb1SetupWire(false, mid+1, uid, 0))
				require.Equal(t, response.ID, request.Session["Continuation Response Event ID"])
			}
			e := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 5, uid, 0))
			require.Equal(t, request.ID, e.ResponseTo)
			req := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 6, 0, 0))
			reused := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 6, uid, 0xc0000016))
			require.Equal(t, req.ID, reused.ResponseTo)
			late := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 7, uid, 0))
			require.NotContains(t, late.Session, "Continuation Response Event ID")
		})
	}
}

func TestSMB1SetupBudgetsAndPublicDTOEditsCannotChangeAuthority(t *testing.T) {
	w := smb1SetupWire(false, 2, 0, 0)
	for _, budget := range []ParserBudget{{MaxFrameBytes: len(w) - 1}, {MaxMessageBytes: len(w) - 1}, {MaxBufferedBytes: len(w) - 1}, {MaxRecursionDepth: 2}, {MaxCollectionElements: -1}} {
		_, err := decodeSMB1SessionSetupFields(w, budget, false)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrResourceExceeded, typed.Kind)
	}
	s := smb1SetupSession(t)
	req := smb1FeedComplete(t, s, 0, w)
	req.Session["Header"].(map[string]any)["User ID"] = uint16(999)
	req.Session["Header"].(map[string]any)["Multiplex ID"] = uint16(99)
	req.Protocol, req.Profile = "tcp", "edited"
	resp := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	require.Equal(t, req.ID, resp.ResponseTo)
	fields, err := resp.GetFields()
	require.NoError(t, err)
	fields["Body"].(map[string]any)["Security Blob"].([]byte)[0] ^= 1
	fresh, err := resp.GetFields()
	require.NoError(t, err)
	require.Equal(t, byte(0x91), fresh["Body"].(map[string]any)["Security Blob"].([]byte)[0])
}

func TestSMB1SetupUIDCollisionRetiresOldAndOutstandingContinuation(t *testing.T) {
	for _, status := range []uint32{0, 0xc0000016} {
		for _, outstanding := range []bool{false, true} {
			t.Run(fmt.Sprintf("status%x/pending%v", status, outstanding), func(t *testing.T) {
				s := smb1SetupSession(t)
				smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
				smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
				if outstanding {
					smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
				}
				smb1FeedComplete(t, s, 0, smb1SetupWire(false, 4, 0, 0))
				smb1FeedComplete(t, s, 1, smb1SetupWire(true, 4, 2048, status))
				if outstanding {
					smb1FeedComplete(t, s, 1, smb1SetupWire(true, 3, 2048, 0xc0000016))
				}
				next := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 5, 2048, 0))
				require.NotContains(t, next.Session, "Continuation Response Event ID", "a conflicting allocated UID must not keep/reseed either earlier lineage")
				require.Equal(t, "request", next.Session["Setup Phase"])
			})
		}
	}
}

func TestSMB1SetupDuplicateClaimCannotRetireIndependentPID(t *testing.T) {
	s := smb1SetupSession(t)
	aRequest := smb1SetupWire(false, 2, 0, 0)
	smb1FeedComplete(t, s, 0, aRequest)
	aResponse := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	bRequest := smb1SetupWire(false, 4, 0, 0)
	bRequest[16] ^= 1
	bResponse := smb1SetupWire(true, 4, 4096, 0)
	bResponse[16] ^= 1
	smb1FeedComplete(t, s, 0, bRequest)
	smb1FeedComplete(t, s, 1, bResponse)
	binary.LittleEndian.PutUint16(bRequest[32:34], 2048)
	smb1FeedComplete(t, s, 0, bRequest)
	next := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
	require.Equal(t, aResponse.ID, next.Session["Continuation Response Event ID"])
	final := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 3, 2048, 0))
	require.Equal(t, next.ID, final.ResponseTo)
}

func TestSMB1SetupFixedPrefixExceedsFrameBudget(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			s, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 64})
			require.NoError(t, err)
			defer s.Close("FIN")
			w := smb1SetupWire(true, 2, 2048, 0)
			start := 0
			if split {
				r := s.Feed(1, time.Unix(100, 0), w[:46])
				require.Empty(t, r.Events)
				start = 46
			}
			r := s.Feed(1, time.Unix(101, 0), w[start:47])
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Equal(t, "limited", r.Events[0].Status)
			_, err = r.Events[0].GetFields()
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrResourceExceeded, typed.Kind)
		})
	}
}

func TestSMB1SetupUnsupportedWindowsStringVariantIsNotMalformed(t *testing.T) {
	w := smb1SetupWire(true, 2, 2048, 0)
	// Empty blob, pad, NativeOS=A, single-zero empty NativeLanMan. MS-SMB
	// Appendix A documents this Windows variation; this profile leaves it out.
	w = append(w[:47], 0, 'A', 0, 0, 0, 0)
	binary.LittleEndian.PutUint16(w[43:45], 0)
	binary.LittleEndian.PutUint16(w[45:47], 6)
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	_, err := decodeSMB1SessionSetupFields(w, DefaultParserBudget(), true)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrUnsupportedFeature, typed.Kind)
}

func TestSMB1SetupModernBodyRefusalRetiresPendingUID(t *testing.T) {
	s := smb1SetupSession(t)
	smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
	smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	state := s.(*captureSession).f.smb2
	bad := smb2UnsupportedSetup()
	partial := s.Feed(0, time.Unix(100, 0), bad[:68])
	require.Empty(t, partial.Events)
	require.NotEmpty(t, state.smb1Setup.continuations)
	r := s.Feed(0, time.Unix(101, 0), bad[68:])
	require.NotNil(t, r.Err)
	require.Equal(t, ErrUnsupportedVersion, r.Err.Kind)
	require.True(t, state.smb1Modern)
	require.Nil(t, state.smb1Setup)
	late := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
	require.NotContains(t, late.Session, "Continuation Response Event ID")
	require.Empty(t, state.pending)
	require.Empty(t, state.async)
	require.Empty(t, state.files)
	require.Zero(t, state.dialect)
}

func TestSMB1SetupTCPRecoveryAndGenerationIsolation(t *testing.T) {
	req, challenge := smb1SetupWire(false, 2, 0, 0), smb1SetupWire(true, 2, 2048, 0xc0000016)
	cont, final := smb1SetupWire(false, 3, 2048, 0), smb1SetupWire(true, 3, 2048, 0)
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true},
		{seq: 100, data: string(req[:8])}, {seq: 164, data: string(req[64:])}, {seq: 108, data: string(req[8:64])},
		{seq: 1000, reverse: true, data: string(challenge[:47])}, {seq: 1064, reverse: true, data: string(challenge[64:])}, {seq: 1047, reverse: true, data: string(challenge[47:64])},
		{seq: 1000, reverse: true, data: string(challenge)},
		{seq: 100 + uint32(len(req)), data: string(cont)}, {seq: 1000 + uint32(len(challenge)), reverse: true, data: string(final)},
		{seq: 100 + uint32(len(req)+len(cont)), rst: true},
		{seq: 9000, syn: true}, {seq: 12000, syn: true, reverse: true},
		{seq: 12001, reverse: true, data: string(final)},
		{seq: 9001, data: string(req)}, {seq: 12001 + uint32(len(final)), reverse: true, data: string(challenge)},
		{seq: 9001 + uint32(len(req)), data: string(smb1SetupWire(false, 4, 2048, 0))},
		{seq: 12001 + uint32(len(final)+len(challenge)), reverse: true, data: string(smb1SetupWire(true, 4, 2048, 0))}}
	for _, workers := range []int{1, 3} {
		for _, deferred := range []bool{false, true} {
			for _, ipv6 := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%v/ipv6%v", workers, deferred, ipv6), func(t *testing.T) {
					events, stats, err := binReplay(t, binTestPcap(t, steps, 445, ipv6, true), workers, WithBinParserDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, 9, "TCP retransmission is not a second setup response")
					for _, pair := range [][2]int{{0, 1}, {2, 3}, {5, 6}, {7, 8}} {
						require.Equal(t, events[pair[0]].ID, events[pair[1]].ResponseTo)
					}
					require.Zero(t, events[4].ResponseTo)
					require.Equal(t, events[1].ID, events[2].Session["Continuation Response Event ID"])
					require.Equal(t, events[6].ID, events[7].Session["Continuation Response Event ID"])
					require.NotEqual(t, events[0].FlowID, events[4].FlowID)
					require.Len(t, events[1].SourceBytes.PacketRefs, 3)
					for _, e := range events {
						fields, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, false, fields["Authentication Verified"])
					}
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
	missing := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true}, {seq: 100, data: string(req)},
		{seq: 1000, reverse: true, data: string(challenge[:47])}, {seq: 1064, reverse: true, data: string(challenge[64:])}, {seq: 1000 + uint32(len(challenge)), reverse: true, fin: true}}
	for _, allow := range []bool{false, true} {
		events, stats, err := binReplay(t, binTestPcap(t, missing, 445, false, false), 1, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allow}))
		if allow {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "unfilled sequence gap")
		}
		require.GreaterOrEqual(t, len(events), 2)
		for _, e := range events[1:] {
			require.NotEqual(t, "decoded", e.Status)
			require.Zero(t, e.ResponseTo)
			require.Nil(t, e.SourceBytes.CompletedIn)
			require.NotContains(t, e.Session, "Continuation Response Event ID")
		}
		require.Zero(t, stats.BufferedBytes)
	}
}

func TestSMB1SetupCaptureDomainIsolation(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		t.Run(fmt.Sprint(vlan), func(t *testing.T) {
			var events []*ProtocolEvent
			a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
			require.NoError(t, err)
			defer a.Close()
			req, challenge := smb1SetupWire(false, 2, 0, 0), smb1SetupWire(true, 2, 2048, 0xc0000016)
			cont, final := smb1SetupWire(false, 3, 2048, 0), smb1SetupWire(true, 3, 2048, 0)
			wires := [][]byte{req, challenge, challenge, cont, cont, final, final}
			domains, dirs := []int{0, 1, 0, 1, 0, 1, 0}, []int{0, 1, 1, 0, 0, 1, 1}
			seq := [2][2]uint32{{1, 1}, {1, 1}}
			for i, w := range wires {
				domain, dir := domains[i], dirs[i]
				src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
				sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
				if dir == 1 {
					src, dst, sport, dport = dst, src, dport, sport
				}
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
				ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
				tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][dir], ACK: true, PSH: true, Window: 4096}
				seq[domain][dir] += uint32(len(w))
				require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
				serial := []gopacket.SerializableLayer{eth, ip, tcp, gopacket.Payload(w)}
				iface := domain
				if vlan {
					iface = 0
					eth.EthernetType = layers.EthernetTypeDot1Q
					serial = []gopacket.SerializableLayer{eth, &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}, ip, tcp, gopacket.Payload(w)}
				}
				b := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
				raw := b.Bytes()
				require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, layers.LinkTypeEthernet))
				clear(raw)
			}
			require.NoError(t, a.Close())
			require.Len(t, events, 7)
			require.Equal(t, events[0].ID, events[2].ResponseTo)
			require.Equal(t, events[2].ID, events[4].Session["Continuation Response Event ID"])
			require.Equal(t, events[4].ID, events[6].ResponseTo)
			for _, i := range []int{1, 3, 5} {
				require.Zero(t, events[i].ResponseTo)
				require.NotContains(t, events[i].Session, "Continuation Response Event ID")
			}
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
		})
	}
}

func TestSMB1SetupRefusedBodyRetiresOnlyItsLineageAndKeepsNextRecord(t *testing.T) {
	s := smb1SetupSession(t)
	smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
	bad := smb1SetupWire(true, 2, 2048, 0xc0000016)
	binary.LittleEndian.PutUint16(bad[50:52], 0xd800)
	late := smb1SetupWire(true, 2, 2048, 0xc0000016)
	r := s.Feed(1, time.Unix(102, 0), append(bytes.Clone(bad), late...))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 2)
	refused := r.Events[0]
	require.Equal(t, "malformed", refused.Status)
	require.Equal(t, "smb1-session-setup-body-diagnostic", refused.Profile)
	require.Equal(t, "headers", refused.Completeness)
	require.Equal(t, bad, refused.Raw)
	require.NotContains(t, refused.Session, "Body")
	require.Zero(t, refused.ResponseTo)
	require.Equal(t, "decoded", r.Events[1].Status)
	require.Zero(t, r.Events[1].ResponseTo)
	refused.Protocol, refused.Status = "http", "decoded"
	refused.Fields = map[string]any{"fake": true}
	refused.Structured = map[string]any{"fields": refused.Fields}
	_, err := refused.GetFields()
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrMalformedMessage, typed.Kind)
	next := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
	require.NotContains(t, next.Session, "Continuation Response Event ID")
	initial := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 4, 0, 0))
	response := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 4, 4096, 0xc0000016))
	require.Equal(t, initial.ID, response.ResponseTo)
	continuation := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 5, 4096, 0))
	require.Equal(t, response.ID, continuation.Session["Continuation Response Event ID"])

	s2 := smb1SetupSession(t)
	smb1FeedComplete(t, s2, 0, smb1SetupWire(false, 2, 0, 0))
	smb1FeedComplete(t, s2, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	badRequest := smb1SetupWire(false, 3, 2048, 0)
	binary.LittleEndian.PutUint16(badRequest[66:68], 0xdc00)
	r = s2.Feed(0, time.Unix(103, 0), badRequest)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	next = smb1FeedComplete(t, s2, 0, smb1SetupWire(false, 4, 2048, 0))
	require.NotContains(t, next.Session, "Continuation Response Event ID")
}

func TestSMB1SetupStateCopyBudgetRefusesBeforeCommitting(t *testing.T) {
	a := &binParser{budget: DefaultParserBudget(), config: BinParserConfig{MaxBufferedBytes: 1024}}
	f := &binFlow{a: a}
	f.smb2 = &binSMB2{flow: f}
	e := &ProtocolEvent{Raw: smb1SetupWire(false, 2, 0, 0), Entry: "CIFSDirectTCPSessionSetupExtendedRequest"}
	err := f.consumeSMB1Setup(0, e)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	require.Nil(t, f.smb2.smb1Setup)
	require.Zero(t, a.buffered.Load())
	require.Zero(t, f.sessionBytes)
	a.config.MaxBufferedBytes = 1 << 20
	require.NoError(t, f.consumeSMB1Setup(0, e))
	state, reserved := f.smb2.smb1Setup, f.sessionBytes
	require.Positive(t, reserved)
	a.config.MaxBufferedBytes = int(reserved + 1)
	err = f.consumeSMB1Setup(0, &ProtocolEvent{Raw: smb1SetupWire(false, 3, 0, 0), Entry: e.Entry})
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	require.Same(t, state, f.smb2.smb1Setup)
	require.Len(t, state.pending, 1)
	require.Equal(t, reserved, f.sessionBytes)
	f.closeSession()
	require.Zero(t, a.buffered.Load())
}
