package sharkcli

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTLSHelloDependencyRefusalsKeepObservedProgressiveIdentity(t *testing.T) {
	for _, name := range []string{"missing-key-exchange", "key-share-length", "early-without-PSK"} {
		t.Run(name, func(t *testing.T) {
			raw, hello, why := envelopeDisplayHello(false), "ServerHello", "ServerHello requires key_share or pre_shared_key"
			switch name {
			case "missing-key-exchange":
				raw[55], raw[56] = 0x7a, 0x7a
			case "key-share-length":
				raw[61], raw[62] = 0, 2
				why = "ServerHello key_share length"
			case "early-without-PSK":
				raw, hello, why = envelopeDisplayHello(true), "ClientHello", "early_data offer requires pre_shared_key"
				raw = append(raw, 0, 42, 0, 0)
				binary.BigEndian.PutUint16(raw[50:], binary.BigEndian.Uint16(raw[50:])+4)
				binary.BigEndian.PutUint16(raw[3:], uint16(len(raw)-5))
				n := len(raw) - 9
				raw[6], raw[7], raw[8] = byte(n>>16), byte(n>>8), byte(n)
			}
			s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("test done") })
			r := s.Feed(1, time.Unix(2, 0), raw)
			require.NotNil(t, r.Err)
			require.Equal(t, pcaputil.ErrMalformedMessage, r.Err.Kind)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Contains(t, e.Error, why)
			e.TCPFlowID = 3
			e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), PacketRefs: []pcaputil.PacketReference{{Number: 1}}}
			_, err = e.GetFields()
			require.Error(t, err, "the observed refusal must not become a successful query")
			row := presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TLS", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Equal(t, "rejected TLS "+hello, row.ProtocolScope)
			require.Contains(t, row.Info, why)
			require.NotContains(t, row.Info, "AEAD verified")
			fields, sources := protocolEventDetail(r.Events)
			for _, detail := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, r.Events, false), workerMetadataDetail(t, r.Events, true)} {
				text := detailLines(detail.fields)
				require.Contains(t, text, "TLS "+hello+" (rejected)")
				require.Contains(t, text, why)
				require.NotContains(t, text, "handshake not captured")
				require.NotContains(t, text, "TLS? candidate")
				require.NotContains(t, text, "Negotiated Version: TLSv1.3")
			}
		})
	}
}
