package pcaputil

import (
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
