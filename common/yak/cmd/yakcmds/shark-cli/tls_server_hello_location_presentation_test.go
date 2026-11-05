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

func misplacedServerHelloPresentation(t *testing.T, kind uint16, data []byte) *pcaputil.ProtocolEvent {
	t.Helper()
	raw := envelopeDisplayHello(false)
	raw = binary.BigEndian.AppendUint16(raw, kind)
	raw = binary.BigEndian.AppendUint16(raw, uint16(len(data)))
	raw = append(raw, data...)
	binary.BigEndian.PutUint16(raw[47:], binary.BigEndian.Uint16(raw[47:])+uint16(len(data)+4))
	binary.BigEndian.PutUint16(raw[3:], uint16(len(raw)-5))
	n := len(raw) - 9
	raw[6], raw[7], raw[8] = byte(n>>16), byte(n>>8), byte(n)
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("done") })
	r := s.Feed(1, time.Unix(10, 0), raw)
	require.NotNil(t, r.Err)
	require.Equal(t, pcaputil.ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.TCPFlowID = 5
	e.Domain = pcaputil.CaptureDomain{Interface: 7}
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), PacketRefs: []pcaputil.PacketReference{{Number: 11, Domain: e.Domain}}}
	return e
}

func TestTLSMisplacedServerHelloExtensionProgressivePresentation(t *testing.T) {
	for _, c := range []struct {
		kind uint16
		name string
		data []byte
	}{
		{16, "application_layer_protocol_negotiation", []byte{0, 3, 2, 'h', '2'}},
		{44, "cookie", []byte{0, 1, 7}},
		{13, "signature_algorithms", []byte{0, 2, 4, 3}},
		{57, "quic_transport_parameters", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := misplacedServerHelloPresentation(t, c.kind, c.data)
			_, err := e.GetFields()
			require.Error(t, err, "a rejected declaration cannot become successful query fields")
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TLS", row.Protocol)
			require.Equal(t, "rejected TLS ServerHello", row.ProtocolScope)
			require.Contains(t, row.Info, "misplaced "+c.name)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				text := detailLines(view.fields)
				require.Contains(t, text, "TLS ServerHello (rejected)")
				require.Contains(t, text, "Misplaced ServerHello extension declaration")
				require.Contains(t, text, "selection not applied")
				require.NotContains(t, text, "Negotiated Version: TLSv1.3")
				require.NotContains(t, text, "handshake not captured")
				found := false
				for _, field := range view.fields {
					if strings.HasPrefix(field.text, "Extension location:") {
						require.False(t, field.hasRange)
					}
					if field.text != fmt.Sprintf("Extension type: %s (%d)", c.name, c.kind) {
						continue
					}
					found = true
					require.True(t, field.hasRange)
					require.Positive(t, field.source)
					wire := view.sources[field.source-1].Data
					require.Equal(t, e.Raw, wire)
					require.Equal(t, c.kind, binary.BigEndian.Uint16(wire[field.start:field.end]))
				}
				require.True(t, found)
			}
		})
	}
}

func TestTLSMisplacedServerHelloPresentationRejectsUnqualifiedEvidence(t *testing.T) {
	for _, mode := range []string{"reason", "type", "source", "domain", "truncation", "authentication", "negotiation"} {
		t.Run(mode, func(t *testing.T) {
			e := misplacedServerHelloPresentation(t, 16, []byte{0, 3, 2, 'h', '2'})
			switch mode {
			case "reason":
				e.Error = strings.Replace(e.Error, "(16)", "(44)", 1)
			case "type":
				o, err := pcaputil.ObserveTLSMisplacedServerHelloExtensionRecord(e.Raw, 16)
				require.NoError(t, err)
				e.Raw = bytes.Clone(e.Raw)
				binary.BigEndian.PutUint16(e.Raw[o.ExtensionOffset:], 0x7a7a)
				e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
			case "source":
				e.SourceBytes.SHA256 = strings.Repeat("0", 64)
			case "domain":
				e.SourceBytes.PacketRefs[0].Domain.Interface++
			case "truncation":
				e.Raw = e.Raw[:len(e.Raw)-1]
			case "authentication":
				e.Session["Authentication Verified"] = true
			case "negotiation":
				e.Session["Negotiated Version"] = 0x304
			}
			require.False(t, rejectedTLSServerHelloDiagnostic(e))
		})
	}
}
