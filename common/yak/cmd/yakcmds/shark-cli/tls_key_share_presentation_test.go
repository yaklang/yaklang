package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func keySharePresentationEvent(t *testing.T, missing bool) *pcaputil.ProtocolEvent {
	t.Helper()
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("done") })
	ch, sh := envelopeDisplayHello(true), envelopeDisplayHello(false)
	if missing {
		// Preserve the complete vector while withholding its actual key_share.
		found := false
		for at := 52; at < len(ch); at += 4 + int(binary.BigEndian.Uint16(ch[at+2:])) {
			if binary.BigEndian.Uint16(ch[at:]) == 51 {
				binary.BigEndian.PutUint16(ch[at:], 0xffa5)
				found = true
				break
			}
		}
		require.True(t, found)
	} else {
		o, err := pcaputil.ObserveTLSKeyShareServerHelloRecord(sh, 16)
		require.NoError(t, err)
		binary.BigEndian.PutUint16(sh[o.ExtensionOffset+4:], 0xfe00)
	}
	r := s.Feed(0, time.Unix(1, 0), ch)
	require.Nil(t, r.Err)
	r = s.Feed(1, time.Unix(2, 0), sh)
	require.NotNil(t, r.Err)
	require.Equal(t, pcaputil.ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.TCPFlowID = 11
	e.Domain = pcaputil.CaptureDomain{Interface: 7}
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(sh)), PacketRefs: []pcaputil.PacketReference{{Number: 6, Domain: e.Domain}}}
	return e
}

func TestTLSRejectedKeyShareProgressivePresentation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missingClientShare%t", missing), func(t *testing.T) {
			e := keySharePresentationEvent(t, missing)
			require.True(t, rejectedTLSServerHelloDiagnostic(e))
			_, err := e.GetFields()
			require.Error(t, err)
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TLS", row.Protocol)
			require.Equal(t, "rejected TLS ServerHello", row.ProtocolScope)
			require.Contains(t, row.Info, "was not offered in current ClientHello")
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, detail := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				text := detailLines(detail.fields)
				require.Contains(t, text, "TLS ServerHello (rejected)")
				require.Contains(t, text, "Rejected key_share selection")
				require.Contains(t, text, "selection not applied")
				require.NotContains(t, text, "Negotiated Version: TLSv1.3")
				found := false
				for _, field := range detail.fields {
					if strings.HasPrefix(field.text, "Current ClientHello match:") {
						require.False(t, field.hasRange)
					}
					if !strings.HasPrefix(field.text, "Declared key_share group:") {
						continue
					}
					found = true
					require.True(t, field.hasRange)
					wire := detail.sources[field.source-1].Data
					require.Equal(t, e.Raw, wire)
					want := uint16(0xfe00)
					if missing {
						want = 29
					}
					require.Equal(t, want, binary.BigEndian.Uint16(wire[field.start:field.end]))
				}
				require.True(t, found)
			}
		})
	}
}

func TestTLSRejectedKeySharePresentationRequiresQualifiedSource(t *testing.T) {
	for _, mode := range []string{"group", "reason", "digest", "domain", "authentication", "negotiation", "truncation"} {
		t.Run(mode, func(t *testing.T) {
			e := keySharePresentationEvent(t, false)
			switch mode {
			case "group":
				e.Raw = bytes.Clone(e.Raw)
				o, err := pcaputil.ObserveTLSKeyShareServerHelloRecord(e.Raw, 16)
				require.NoError(t, err)
				binary.BigEndian.PutUint16(e.Raw[o.ExtensionOffset+4:], 29)
				e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
			case "reason":
				e.Error += " unverified"
			case "digest":
				e.SourceBytes.SHA256 = strings.Repeat("0", 64)
			case "domain":
				e.SourceBytes.PacketRefs[0].Domain.Interface++
			case "authentication":
				e.Session["Authentication Verified"] = true
			case "negotiation":
				e.Session["Negotiated Version"] = 0x304
			case "truncation":
				e.Raw = e.Raw[:len(e.Raw)-1]
			}
			require.False(t, rejectedTLSServerHelloDiagnostic(e))
		})
	}
}
