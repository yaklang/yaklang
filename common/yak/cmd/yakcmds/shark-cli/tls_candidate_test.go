package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestConnectionContinuationKeepsWorkerAndPacketEvidenceConsistent(t *testing.T) {
	for _, protocol := range []string{"TLSv1.3", "SSH", "MySQL"} {
		p := tcpPacket(t, 1, 100, "SSH-2.0-foreign-body\r\n", false, false, false, false)
		p.application, p.evidence = protocol, "validated protocol events"
		p.connectionStack = []string{protocol}
		p.opaqueTransport = protocol == "SSH"
		row := summarize(p)
		require.Equal(t, protocol, row.Protocol)
		require.Equal(t, "connection-associated bytes", row.ProtocolScope)
		require.Equal(t, "context-required", row.ProtocolStatus)
		req, err := json.Marshal(decodeRequest{Number: p.number, Data: p.data, Length: p.ci.Length, Link: p.link, ConnectionStack: p.connectionStack, ConnectionProtocol: p.application, ConnectionEvidence: p.evidence, OpaqueTransport: p.opaqueTransport})
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(req), &output))
		d, err := decodeWorkerOutput(output.Bytes(), p)
		require.NoError(t, err)
		require.Equal(t, row.Protocol, d.protocol)
		for _, f := range d.fields {
			if protocol != "SSH" && f.branch {
				require.NotEqual(t, "SSH", f.text, "foreign body signature cannot create a raw application subtree")
			}
		}
		ack := tcpPacket(t, 2, 122, "", false, false, false, false)
		ack.application, ack.evidence, ack.connectionStack = p.application, p.evidence, p.connectionStack
		require.Equal(t, "TCP", summarize(ack).Protocol)
	}
}

func TestTLSCandidateNamesEnvelopeWithoutConfirmingSession(t *testing.T) {
	for _, n := range []int{1, 55, 512} {
		wire := append([]byte{23, 3, 3, byte(n >> 8), byte(n)}, make([]byte, n)...)
		p := tcpPacket(t, 1, 100, string(wire), false, false, false, false)
		e := &pcaputil.ProtocolEvent{ID: 1, Transport: "tcp", Protocol: "tls", Status: "decoded", Raw: wire, Session: map[string]any{"Record Type": uint8(23), "Record Length": n, "Record Version": uint16(0x303), "Content Visibility": "encrypted", "Secret Lookup Status": "missing-context"}}
		p.events = []*pcaputil.ProtocolEvent{e}
		row := summarize(p)
		label, scope := "TLS?", "candidate record"
		if n >= 16 {
			label, scope = "TLS", "record envelope"
		}
		require.Equal(t, label, row.Protocol)
		require.Equal(t, scope, row.ProtocolScope)
		require.Empty(t, row.ConnectionProtocol)
		require.Contains(t, row.Info, "Application Data")
		require.Contains(t, row.Info, "handshake not captured")
		require.NotContains(t, row.Info, "TLS-like")
		require.NotContains(t, row.Info, "keys unavailable", "missing context differs from a missing key")
		d := detail(p)
		require.Equal(t, label, d.protocol)
		var text strings.Builder
		for _, f := range d.fields {
			text.WriteString(f.text + "\n")
			if f.branch && n < 16 {
				require.NotEqual(t, "TLS", f.text, "wire fallback must not contradict candidate identity")
			}
		}
		require.Contains(t, text.String(), "handshake")
		require.Contains(t, text.String(), "authentication has not been verified")
		if n >= 16 {
			require.NotContains(t, text.String(), "TLS? candidate")
		}
	}
}

func TestNegotiatedTLSWithoutKeysDoesNotBecomeCandidate(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "decoded", Session: map[string]any{"Negotiated Version": uint16(0x304), "Record Type": uint8(23), "Content Visibility": "encrypted", "Secret Lookup Status": "missing-key"}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Contains(t, row.Info, "key")
	require.NotContains(t, row.Info, "handshake not captured")
}

func TestRepeatedCompleteTLSRecordsKeepMidstreamCarrierWithoutInventingVersion(t *testing.T) {
	s := newStreamStore(8, 4096)
	defer s.finish()
	seq := uint32(100)
	for i := uint64(1); i <= 2; i++ {
		wire := append([]byte{23, 3, 3, 0, 32}, make([]byte, 32)...)
		p := tcpPacket(t, i, seq, string(wire), false, false, false, false)
		seq += uint32(len(wire))
		s.add(p)
		e := trustedStreamEvent(p, "tls")
		e.FlowID = 55
		e.Raw = wire
		e.Session = map[string]any{"Record Type": uint8(23), "Record Version": uint16(0x303), "Record Length": 32, "Content Visibility": "encrypted", "Secret Lookup Status": "missing-context"}
		s.observeProtocol(e)
		if i == 1 {
			require.Empty(t, s.snapshot(p.streamID).protocolStack)
		} else {
			require.Equal(t, []string{"TLS"}, s.snapshot(p.streamID).protocolStack)
		}
	}
	p := tcpPacket(t, 3, seq, "middle ciphertext", false, false, false, false)
	s.add(p)
	s.annotate(p)
	row := summarize(p)
	require.Equal(t, "TLS", row.Protocol)
	require.Equal(t, "TLS", row.ConnectionProtocol)
	require.Equal(t, "connection-associated bytes", row.ProtocolScope)
	require.Contains(t, row.ConnectionEvidence, "inferred")
	require.NotContains(t, row.Info, "TLSv1.2")
	ack := tcpPacket(t, 4, seq+17, "", false, false, false, false)
	s.add(ack)
	s.annotate(ack)
	require.Equal(t, "TCP", summarize(ack).Protocol)
	reuse := tcpPacket(t, 5, 9000, "", false, true, false, false)
	s.add(reuse)
	s.annotate(reuse)
	require.Empty(t, reuse.connectionStack)
}
