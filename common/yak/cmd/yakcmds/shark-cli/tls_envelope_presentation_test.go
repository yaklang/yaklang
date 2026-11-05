package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func envelopeDisplayHello(client bool) []byte {
	body := append([]byte{3, 3}, bytes.Repeat([]byte{1}, 32)...)
	body = append(body, 0)
	typ := byte(2)
	extensions := []byte{0, 43, 0, 2, 3, 4, 0, 51, 0, 5, 0, 29, 0, 1, 1}
	if client {
		typ = 1
		body = append(body, 0, 2, 0x13, 1, 1, 0)
		extensions = []byte{0, 43, 0, 3, 2, 3, 4, 0, 10, 0, 6, 0, 4, 0, 29, 0, 23, 0, 51, 0, 7, 0, 5, 0, 29, 0, 1, 1}
	} else {
		body = append(body, 0x13, 1, 0)
	}
	body = binary.BigEndian.AppendUint16(body, uint16(len(extensions)))
	body = append(body, extensions...)
	handshake := append([]byte{typ, 0, 0, byte(len(body))}, body...)
	wire := []byte{22, 3, 3}
	wire = binary.BigEndian.AppendUint16(wire, uint16(len(handshake)))
	return append(wire, handshake...)
}

func TestTLSImpossibleEnvelopePresentationKeepsIdentityAndLiteralHeader(t *testing.T) {
	for _, version := range []byte{1, 3} {
		t.Run(fmt.Sprintf("legacy-version%d", version), func(t *testing.T) {
			s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("FIN") })
			ch := s.Feed(0, time.Unix(1, 0), envelopeDisplayHello(true))
			sh := s.Feed(1, time.Unix(2, 0), envelopeDisplayHello(false))
			require.Nil(t, ch.Err)
			require.Nil(t, sh.Err)
			require.Len(t, ch.Events, 1)
			require.Len(t, sh.Events, 1)
			require.Equal(t, "decoded", sh.Events[0].Status)
			raw := []byte{23, 3, version, 0, 3, 1, 2, 3}
			r := s.Feed(0, time.Unix(3, 0), raw)
			require.NotEmpty(t, r.Events)
			e := r.Events[0]
			require.Equal(t, "malformed", e.Status)
			require.Equal(t, "tls", e.Protocol)
			require.False(t, e.Session["Authentication Verified"].(bool))
			require.Equal(t, uint16(0x304), e.Session["Negotiated Version"])
			why := "TLS ciphertext shorter than negotiated AEAD envelope"
			if version == 1 {
				why = "TLS 1.3 protected record legacy version must be 0x0303"
			}
			require.Contains(t, e.Error, why)
			_, err = e.GetFields()
			require.Error(t, err)
			row := presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TLSv1.3", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, why)
			require.Contains(t, row.ProtocolStack, "TLSv1.3")
			require.NotContains(t, row.Info, "AEAD verified")
			fields, sources := protocolEventDetail(r.Events)
			for _, detail := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, r.Events, false), workerMetadataDetail(t, r.Events, true)} {
				text := detailLines(detail.fields)
				require.Contains(t, text, why)
				require.NotContains(t, text, "handshake not captured")
				require.NotContains(t, text, "TLS? candidate")
				found := 0
				for _, f := range detail.fields {
					if strings.HasPrefix(f.text, "Record Length:") {
						found++
						require.True(t, f.hasRange)
						require.Positive(t, f.source)
						require.Equal(t, 3, f.start)
						require.Equal(t, 5, f.end)
						require.Equal(t, []byte{0, 3}, detail.sources[f.source-1].Data[f.start:f.end])
					}
					if strings.HasPrefix(f.text, "Error:") {
						require.False(t, f.hasRange)
					}
				}
				require.Equal(t, 1, found)
			}
			e.Protocol, e.Status, e.Error = "grpc", "decoded", "forged"
			e.Session = map[string]any{"Negotiated Version": uint16(0x303), "Authentication Verified": true}
			e.Fields, e.Structured = e.Session, map[string]any{"fields": e.Session}
			row = presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.Equal(t, "TLSv1.3", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, why)
			require.NotContains(t, row.Info, "forged")
			fields, _ = protocolEventDetail(r.Events)
			require.Contains(t, detailLines(fields), why)
			require.NotContains(t, detailLines(fields), "forged")
		})
	}
}

func TestWCFTLSImpossibleEnvelopePresentationRetainsRecordHeader(t *testing.T) {
	s := wcfTLSDisplayTestSession(t)
	require.Nil(t, s.Feed(0, time.Unix(3, 0), envelopeDisplayHello(true)).Err)
	require.Nil(t, s.Feed(1, time.Unix(4, 0), envelopeDisplayHello(false)).Err)
	r := s.Feed(0, time.Unix(5, 0), []byte{23, 3, 3, 0, 3, 1, 2, 3})
	require.NotEmpty(t, r.Events)
	e := r.Events[0]
	_, ok := e.NativeTLSCiphertextRefusal()
	require.True(t, ok)
	_, ok = e.NativeWCFTLSCarrier()
	require.True(t, ok)
	_, err := e.GetFields()
	require.Error(t, err)
	row := presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Equal(t, "rejected TLS ciphertext envelope", row.ProtocolScope)
	require.Contains(t, row.ProtocolStack, "WCF/MC-NMF")
	fields, sources := protocolEventDetail(r.Events)
	for _, d := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, r.Events, false), workerMetadataDetail(t, r.Events, true)} {
		text := detailLines(d.fields)
		require.Contains(t, text, "TLS ciphertext shorter than negotiated AEAD envelope")
		require.NotContains(t, text, "no complete record admitted")
		require.NotContains(t, text, "record identity: unconfirmed")
		found := 0
		for _, field := range d.fields {
			if strings.HasPrefix(field.text, "Record Length:") {
				found++
				require.True(t, field.hasRange)
				require.Positive(t, field.source)
				require.Equal(t, []byte{0, 3}, d.sources[field.source-1].Data[field.start:field.end])
			}
		}
		require.Equal(t, 1, found)
	}
}
