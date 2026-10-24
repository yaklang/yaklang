package sharkcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func trustedStreamEvent(p *capturedPacket, protocol string) *pcaputil.ProtocolEvent {
	domain := pcaputil.PacketDomain(p.data, p.ci, p.link)
	return &pcaputil.ProtocolEvent{ID: p.number, Transport: "tcp", Protocol: protocol, Status: "decoded", Timestamp: p.ci.Timestamp, Domain: domain, Source: "192.0.2.1:45000", Destination: "198.51.100.2:12011", SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: p.number, Domain: domain}}}}
}

func TestConnectionIdentitySurvivesErrorsButNotReuseOrDomainChange(t *testing.T) {
	s := newStreamStore(8, 256)
	first := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(first)
	p := tcpPacket(t, 2, 101, "hello", false, false, false, false)
	s.add(p)
	e := trustedStreamEvent(p, "tls")
	// This test exercises trusted event routing, not TLS admission; real TLS
	// negotiation/key failures are covered by the sealed capture tests.
	e.Session = map[string]any{"Negotiated Version": uint16(0x304)}
	s.observeProtocol(e)
	old := first.streamID
	require.Equal(t, "TLSv1.3", s.snapshot(old).protocol)
	bad := *e
	bad.Status = "malformed"
	bad.Error = "tag mismatch"
	bad.Protocol = "mqtt"
	s.observeProtocol(&bad)
	require.Equal(t, []string{"TLSv1.3"}, s.snapshot(old).protocolStack)
	// Bounded message history can be evicted independently of scalar identity.
	s.history, _ = pcaputil.NewProtocolInspector(1, 64)
	s.history.OnEvent(e)
	require.Empty(t, s.history.Rows("", 0))
	ack := tcpPacket(t, 3, 106, "", false, false, false, false)
	s.add(ack)
	s.annotate(ack)
	row := s.projectConversation(summarize(ack), old)
	require.Equal(t, "TCP", row.Protocol)
	require.Equal(t, "TLSv1.3", row.ConnectionProtocol)
	require.Contains(t, conversationCaption(row), "connection:")
	reused := tcpPacket(t, 4, 500, "", false, true, false, false)
	s.add(reused)
	require.NotEqual(t, old, reused.streamID)
	s.observeProtocol(e) // a late event from the old generation cannot relabel it.
	require.Empty(t, s.snapshot(reused.streamID).protocolStack)
	other := tcpPacket(t, 5, 500, "", false, true, false, false)
	other.ci.InterfaceIndex = 1
	s.add(other)
	require.NotEqual(t, reused.streamID, other.streamID)
	s.observeProtocol(e)
	require.Empty(t, s.snapshot(other.streamID).protocolStack)
	s.finish()
}

func TestNativeConnectionStickinessOnACKAndMessageCacheEviction(t *testing.T) {
	for _, tc := range top30Cases() {
		t.Run(tc.name, func(t *testing.T) {
			packets, s := top30Replay(t, tc)
			for _, p := range packets {
				row := s.streams.projectConversation(summarize(p), p.streamID)
				if p.streamID == 0 || len(row.ConnectionStack) == 0 {
					continue
				}
				require.Equal(t, "validated protocol events", row.ConnectionEvidence)
				// ACKs retain connection context without manufacturing an app PDU.
				if len(p.eventIDs) == 0 && row.Protocol == "TCP" {
					require.NotEmpty(t, row.ConnectionProtocol)
					require.Contains(t, row.Info, "connection #")
				}
			}
		})
	}
}

func TestQUICConversationDisplayAndPlaintextByteOwnership(t *testing.T) {
	tc := top30Cases()[7]
	packets, s := top30Replay(t, tc)
	var selected *capturedPacket
	for _, p := range packets {
		if summarize(p).Protocol == "HTTP/3" {
			selected = p
			break
		}
	}
	require.NotNil(t, selected)
	require.NotZero(t, selected.conversationID)
	require.Zero(t, selected.streamID)
	v := datagramConversation(selected)
	require.NotNil(t, v)
	require.Equal(t, "udp", v.transport)
	require.Contains(t, v.protocolStack, "QUIC")
	require.Contains(t, v.protocolStack, "HTTP/3")
	require.Contains(t, v.description(), "UDP › QUIC")
	u := &tui{ring: newPacketRing(100), streams: s.streams, width: 180, height: 54, pane: 3, inspect: selected, stream: v, streamPacket: selected.number, streamApplication: true, streamPinned: true}
	u.ring.add(selected)
	var text strings.Builder
	for _, line := range u.streamLines() {
		text.WriteString(line.text + "\n")
	}
	require.Contains(t, text.String(), "packet AEAD verified")
	require.Contains(t, text.String(), ":path:")
	u.key("a", nil)
	text.Reset()
	for _, line := range u.streamLines() {
		text.WriteString(line.text + "\n")
	}
	require.Contains(t, text.String(), "datagram PDU #")
	require.NotContains(t, text.String(), "TCP payload beginning")
	var fields []field
	var sources []detailSource
	for _, p := range packets {
		if p.conversationID == selected.conversationID {
			f, d := protocolEventDetail(p.protocolEvents())
			fields = append(fields, f...)
			sources = append(sources, d...)
		}
	}
	plaintext := false
	for _, src := range sources {
		if strings.Contains(src.Name, "QUIC STREAM") {
			plaintext = true
			require.NotEmpty(t, src.Data)
			require.False(t, bytes.Equal(selected.data, src.Data))
		}
	}
	require.True(t, plaintext)
	require.NotEmpty(t, fields)
}

func TestQUICCIDEnvelopeAdmissionIsExplicitAndDomainBounded(t *testing.T) {
	s := newStreamStore(2, 1024)
	e := &pcaputil.ProtocolEvent{ID: 1, FlowID: 7, Transport: "udp", Protocol: "http3", Status: "decoded", Timestamp: time.Unix(1, 0)}
	s.observeProtocol(e)
	e2 := &pcaputil.ProtocolEvent{ID: 2, FlowID: 7, Transport: "udp", Protocol: "quic", Status: "context-required", Timestamp: time.Unix(2, 0), Admission: "observed-CID-protected-payload", ExpertCode: string(pcaputil.ErrEncrypted), Session: map[string]any{"Encrypted": true, "Key Reason": "application-keys-missing"}}
	s.observeProtocol(e2)
	row := presentEvents([]*pcaputil.ProtocolEvent{e2}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "QUIC", row.Protocol)
	require.Equal(t, "encrypted", row.Visibility)
	require.Equal(t, "connection envelope", row.ProtocolScope)
	raw := &capturedPacket{conversationID: 7}
	s.annotate(raw)
	require.Equal(t, []string{"QUIC", "HTTP/3"}, raw.connectionStack)
	other := &capturedPacket{conversationID: 7, reference: pcaputil.PacketReference{Domain: pcaputil.CaptureDomain{Interface: 1}}}
	s.annotate(other)
	require.Empty(t, other.connectionStack)
	unknown := *e2
	unknown.FlowID = 0
	require.Empty(t, presentEvents([]*pcaputil.ProtocolEvent{&unknown}).stack)
	for id := uint64(8); id < 30; id++ {
		copyEvent := *e
		copyEvent.FlowID, copyEvent.ID = id, id
		s.observeProtocol(&copyEvent)
	}
	require.Len(t, s.udpConversations, 2, "metadata remains bounded independently of payload retention")
}

func TestTLSCiphertextHeaderAloneCannotLockAConnection(t *testing.T) {
	s := newStreamStore(8, 1024)
	syn := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(syn)
	p := tcpPacket(t, 2, 101, "\x17\x03\x03\x00\x01x", false, false, false, false)
	s.add(p)
	e := trustedStreamEvent(p, "tls")
	e.Session = map[string]any{"Content Visibility": "encrypted", "Secret Lookup Status": "missing-context", "Record Type": uint8(23)}
	s.observeProtocol(e)
	require.Empty(t, s.snapshot(syn.streamID).protocolStack)
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLS?", row.Protocol)
	require.Equal(t, "unconfirmed", row.ProtocolStatus)
	require.Empty(t, row.ConnectionProtocol)
	require.Contains(t, row.Info, "session unconfirmed")
	// Once real handshake metadata has established TLS, an opaque later PDU
	// does not erase the connection identity or invent decrypted payload.
	e.Session["Negotiated Version"] = uint16(0x304)
	s.observeProtocol(e)
	delete(e.Session, "Negotiated Version")
	s.observeProtocol(e)
	require.Equal(t, []string{"TLSv1.3"}, s.snapshot(syn.streamID).protocolStack)
	confirmed := applyApplication(row, "TLSv1.3", "validated protocol events", syn.streamID)
	require.Equal(t, "TLSv1.3", confirmed.Protocol)
	require.NotContains(t, confirmed.Info, "session unconfirmed")
	s.finish()
}

func TestUDPMessageWindowIncludesSelectedRetainedPDU(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1024, 1<<20)
	require.NoError(t, err)
	for id := uint64(1); id <= 500; id++ {
		history.OnEvent(&pcaputil.ProtocolEvent{ID: id, FlowID: 7, Protocol: "quic", Transport: "udp", Status: "decoded", Source: "192.0.2.1:1", Destination: "192.0.2.2:2", Timestamp: time.Unix(int64(id), 0), Fields: map[string]any{"value": id}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: id}}}})
	}
	selected := &capturedPacket{conversationID: 7, history: history, eventIDs: []uint64{400}, reference: pcaputil.PacketReference{Number: 400}}
	v := datagramConversation(selected)
	require.Len(t, v.events, 128)
	require.NotEmpty(t, v.retentionNotice)
	found := false
	for _, e := range v.events {
		found = found || e.ID == 400
	}
	require.True(t, found)
}

func TestAuthenticationDetailsPrioritizeMeaningBeforeRawTree(t *testing.T) {
	packets, _ := top30Replay(t, top30Cases()[16])
	for _, p := range packets {
		if !strings.Contains(summarize(p).Info, "AUTHENTICATE") {
			continue
		}
		fields, _ := protocolEventDetail(p.protocolEvents())
		username, raw := -1, -1
		for i, f := range fields {
			if strings.HasPrefix(f.text, "User Name: root") {
				username = i
			}
			if strings.HasPrefix(f.text, "Decoded Fields[") && raw < 0 {
				raw = i
			}
		}
		require.NotEqual(t, -1, username)
		require.NotEqual(t, -1, raw)
		require.Less(t, username, raw)
		return
	}
	t.Fatal("missing real Type 3 authentication envelope")
}
