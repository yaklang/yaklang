package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
	"time"
)

func TestTLSAEADMaximumEnvelopePresentation(t *testing.T) {
	for _, version := range []uint16{0x303, 0x304} {
		t.Run(fmt.Sprintf("%04x", version), func(t *testing.T) {
			session, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
			require.NoError(t, err)
			defer session.Close("FIN")
			hello := func(client bool) []byte {
				if version == 0x304 {
					return envelopeDisplayHello(client)
				}
				body := append([]byte{3, 3}, bytes.Repeat([]byte{1}, 32)...)
				body = append(body, 0)
				kind := byte(2)
				if client {
					kind = 1
					body = append(body, 0, 2, 0, 0x9c, 1, 0)
				} else {
					body = append(body, 0, 0x9c, 0)
				}
				h := append([]byte{kind, 0, 0, byte(len(body))}, body...)
				w := []byte{22, 3, 3, 0, 0}
				binary.BigEndian.PutUint16(w[3:], uint16(len(h)))
				return append(w, h...)
			}
			require.Nil(t, session.Feed(0, time.Unix(1, 0), hello(true)).Err)
			sh := session.Feed(1, time.Unix(2, 0), hello(false))
			require.Nil(t, sh.Err)
			require.Equal(t, "decoded", sh.Events[0].Status)
			size := 16384 + 24 + 1
			if version == 0x304 {
				size = 16384 + 17 + 1
			}
			raw := []byte{23, 3, 3, 0, 0}
			binary.BigEndian.PutUint16(raw[3:], uint16(size))
			raw = append(raw, bytes.Repeat([]byte{1}, size)...)
			result := session.Feed(0, time.Unix(3, 0), raw)
			require.NotEmpty(t, result.Events)
			event := result.Events[0]
			observation, ok := event.NativeTLSCiphertextRefusal()
			require.True(t, ok)
			require.Equal(t, version, observation.Version)
			require.Equal(t, "malformed", event.Status)
			_, err = event.GetFields()
			require.Error(t, err)
			want := "TLSv1.2"
			if version == 0x304 {
				want = "TLSv1.3"
			}
			row := presentEvents(result.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, want, row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, "TLS ciphertext exceeds negotiated AEAD plaintext limit")
			require.NotContains(t, row.Info, "AEAD verified")
			fields, sources := protocolEventDetail(result.Events)
			for _, d := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, result.Events, false), workerMetadataDetail(t, result.Events, true)} {
				lines := detailLines(d.fields)
				require.Contains(t, lines, "TLS ciphertext exceeds negotiated AEAD plaintext limit")
				require.NotContains(t, lines, "identity: unconfirmed")
				found := 0
				for _, field := range d.fields {
					if strings.HasPrefix(field.text, "Record Length:") {
						found++
						require.True(t, field.hasRange)
						require.Positive(t, field.source)
						require.Equal(t, raw[3:5], d.sources[field.source-1].Data[field.start:field.end])
					}
					if strings.HasPrefix(field.text, "Error:") {
						require.False(t, field.hasRange)
					}
				}
				require.Equal(t, 1, found)
			}
			event.Protocol, event.Status, event.Error = "grpc", "decoded", "forged"
			event.Session = map[string]any{"Negotiated Version": uint16(0), "Authentication Verified": true}
			row = presentEvents(result.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.Equal(t, want, row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.NotContains(t, row.Info, "forged")
		})
	}
}
