package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func greasePresentationEvent(t *testing.T, kind string) *pcaputil.ProtocolEvent {
	t.Helper()
	raw := envelopeDisplayHello(false)
	switch kind {
	case "version":
		binary.BigEndian.PutUint16(raw[53:], 0x7a7a)
	case "cipher suite":
		binary.BigEndian.PutUint16(raw[44:], 0x7a7a)
	case "extension":
		raw = append(raw, 0x7a, 0x7a, 0, 1, 7)
	case "key_share group", "retry-group":
		binary.BigEndian.PutUint16(raw[59:], 0x7a7a)
		if kind == "retry-group" {
			random, err := hex.DecodeString("cf21ad74e59a6111be1d8c021e65b891c2a211167abb8c5e079e09e2c8a8339c")
			require.NoError(t, err)
			copy(raw[11:43], random)
			raw = raw[:61]
			binary.BigEndian.PutUint16(raw[57:], 2)
		}
	}
	binary.BigEndian.PutUint16(raw[47:], uint16(len(raw)-49))
	binary.BigEndian.PutUint16(raw[3:], uint16(len(raw)-5))
	n := len(raw) - 9
	raw[6], raw[7], raw[8] = byte(n>>16), byte(n>>8), byte(n)
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("done") })
	r := s.Feed(1, time.Unix(1, 0), raw)
	require.NotNil(t, r.Err)
	require.Equal(t, pcaputil.ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.TCPFlowID = 7
	e.Domain = pcaputil.CaptureDomain{Interface: 2}
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), PacketRefs: []pcaputil.PacketReference{{Number: 6, Domain: e.Domain}}}
	return e
}

func TestTLSGREASEProgressivePresentationKeepsRejectedLiteral(t *testing.T) {
	for _, kind := range []string{"version", "cipher suite", "extension", "key_share group", "retry-group"} {
		t.Run(kind, func(t *testing.T) {
			e := greasePresentationEvent(t, kind)
			require.True(t, rejectedTLSServerHelloDiagnostic(e))
			name := "ServerHello"
			if kind == "retry-group" {
				name = "HelloRetryRequest"
			}
			require.Equal(t, name, rejectedTLSHelloName(e))
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TLS", row.Protocol)
			require.Equal(t, "rejected TLS "+name, row.ProtocolScope)
			require.Contains(t, row.Info, "reserved GREASE")
			require.NotContains(t, row.ProtocolStack, "TLSv1.3")
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				text := detailLines(view.fields)
				require.Contains(t, text, "TLS "+name+" (rejected)")
				require.Contains(t, text, "Reserved GREASE selection")
				require.Contains(t, text, "cannot be negotiated")
				require.NotContains(t, text, "Negotiated Version: TLSv1.3")
				found := false
				for _, f := range view.fields {
					if strings.HasPrefix(f.text, "Selection admission:") {
						require.False(t, f.hasRange)
					}
					if !strings.HasPrefix(f.text, "Reserved ") || !strings.Contains(f.text, ": 0x7a7a") {
						continue
					}
					found = true
					require.True(t, f.hasRange)
					require.Positive(t, f.source)
					wire := view.sources[f.source-1].Data
					require.Equal(t, e.Raw, wire)
					require.Equal(t, uint16(0x7a7a), binary.BigEndian.Uint16(wire[f.start:f.end]))
				}
				require.True(t, found)
			}
		})
	}
}

func TestTLSGREASEPresentationRejectsMismatchedSourceOrRefusal(t *testing.T) {
	for _, mode := range []string{"reason", "value", "digest", "domain", "profile", "owner", "authentication", "negotiation", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			e := greasePresentationEvent(t, "version")
			switch mode {
			case "reason":
				e.Error += " unconfirmed"
			case "value":
				e.Raw = bytes.Clone(e.Raw)
				binary.BigEndian.PutUint16(e.Raw[53:], 0x0a0a)
				e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
			case "digest":
				e.SourceBytes.SHA256 = strings.Repeat("0", 64)
			case "domain":
				e.SourceBytes.PacketRefs[0].Domain.Interface++
			case "profile":
				e.Profile = "unknown"
			case "owner":
				e.TCPFlowID = 0
			case "authentication":
				e.Session["Authentication Verified"] = true
			case "negotiation":
				e.Session["Negotiated Version"] = 0x304
			case "truncated":
				e.Raw = e.Raw[:len(e.Raw)-1]
			}
			require.False(t, rejectedTLSServerHelloDiagnostic(e))
			require.Empty(t, rejectedTLSHelloName(e))
		})
	}
}
