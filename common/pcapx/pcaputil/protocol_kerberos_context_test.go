package pcaputil

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKerberosContextPresentationPreservesMalformedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		raw     []byte
		context bool
	}{{[]byte{0x6b, 0}, true}, {[]byte{0x6b, 2, 4, 0}, true}, {[]byte{0x6b, 4, 0x30, 0}, false}, {[]byte{0x6b, 0, 0}, false}} {
		e := &ProtocolEvent{Protocol: "kerberos", Rule: "application-layer.kerberos_fields", Entry: "KerberosMessageFields", Status: "malformed", Error: "prior decoder diagnostic", Raw: tc.raw}
		e.finalizeEvidence()
		if tc.context {
			require.Equal(t, "context-required", e.Status)
			require.Equal(t, string(ErrContextRequired), e.ExpertCode)
			require.Empty(t, e.Error)
			for _, word := range []string{"error", "malformed", "失败"} {
				require.NotContains(t, strings.ToLower(e.Summary), word)
			}
			require.Equal(t, []any{"context-required"}, e.DisplayFields()["status"])
		} else {
			require.Equal(t, "malformed", e.Status)
			require.NotEmpty(t, e.Error)
		}
	}
}

func TestKerberosContextCountersAgreeAcrossDeferredTCPAndUDP(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, transport := range []string{"tcp", "udp"} {
			opaque := []byte{0x6b, 0}
			broken := []byte{0x6b, 4, 0x30, 0}
			var capture []byte
			if transport == "udp" {
				capture = sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: opaque}, {dir: 0, wire: broken}}, 88)
			} else {
				var messages []byte
				for _, raw := range [][]byte{opaque, broken} {
					var size [4]byte
					binary.BigEndian.PutUint32(size[:], uint32(len(raw)))
					messages = append(messages, size[:]...)
					messages = append(messages, raw...)
				}
				capture = binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(messages)}}, 88, false, false)
			}
			events, stats, err := binReplay(t, capture, 1, WithBinParserDeferred(deferred))
			require.NoError(t, err)
			require.Len(t, events, 2, transport)
			require.Equal(t, "context-required", events[0].Status)
			require.Empty(t, events[0].Error)
			require.Equal(t, []any{string(ErrContextRequired)}, events[0].DisplayFields()["expert"])
			require.Equal(t, "malformed", events[1].Status)
			require.EqualValues(t, 1, stats.ContextRequired)
			require.EqualValues(t, 1, stats.Malformed)
			require.Zero(t, stats.Deferred)
		}
	}
}
