package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSSHEncryptedByteSourceIsAnObservationThroughJSON(t *testing.T) {
	for _, retained := range []int{0, 48, 64} {
		for _, consumed := range []int{192, retained} {
			t.Run(fmt.Sprintf("retained%d/consumed%d", retained, consumed), func(t *testing.T) {
				e := &pcaputil.ProtocolEvent{ID: 3, Protocol: "ssh", Transport: "tcp", Status: "context-required", ExpertCode: "Encrypted", Length: consumed,
					Raw: bytes.Repeat([]byte{0x42}, retained), SourceBytes: pcaputil.ByteSource{Kind: "reassembled"}}
				wire, err := json.Marshal(e)
				require.NoError(t, err)
				var ipc pcaputil.ProtocolEvent
				require.NoError(t, json.Unmarshal(wire, &ipc))
				fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{&ipc})
				group := ""
				for _, f := range fields {
					if strings.HasPrefix(f.text, "SSH · ") && strings.Contains(f.text, "#3") {
						group = f.text
						break
					}
				}
				require.Contains(t, group, "observation #3", "NEWKEYS proves opaque transport context, not a complete decoded SSH message")
				require.NotContains(t, group, "PDU")
				if retained == 0 {
					require.Empty(t, sources, "do not invent byte sources for empty observations")
					return
				}
				require.Len(t, sources, 1)
				require.Equal(t, e.Raw, sources[0].Data)
				require.NotContains(t, sources[0].Name, "PDU")
				if consumed > retained {
					require.Equal(t, fmt.Sprintf("SSH · encrypted stream preview · %d/%d observed bytes", retained, consumed), sources[0].Name)
				} else {
					require.Equal(t, fmt.Sprintf("SSH · encrypted stream bytes · %d observed bytes", retained), sources[0].Name)
				}
				require.Equal(t, consumed, ipc.Length, "rendering must not replace logical stream consumption with preview length")
				require.Equal(t, "reassembled", ipc.SourceBytes.Kind)
				ipc.Raw[0] = 0
				require.Equal(t, byte(0x42), sources[0].Data[0], "byte-view ownership survives caller mutation")
			})
		}
	}
}

func TestSSHEncryptedByteSourceDoesNotRelabelOtherObservations(t *testing.T) {
	for _, tc := range []struct{ protocol, status, expert, kind string }{
		{"ssh", "decoded", "", "reassembled"},
		{"ssh", "malformed", "MalformedMessage", "reassembled"},
		{"ssh", "context-required", "NeedMore", "reassembled"},
		{"tls", "decoded", "", "decrypted"},
		{"quic", "context-required", "Encrypted", "captured"},
	} {
		t.Run(tc.protocol+"/"+tc.status+"/"+tc.expert, func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{ID: 3, Protocol: tc.protocol, Transport: "tcp", Status: tc.status, ExpertCode: tc.expert,
				Length: 192, Raw: bytes.Repeat([]byte{0x42}, 64), SourceBytes: pcaputil.ByteSource{Kind: tc.kind}}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, f := range fields {
				require.NotContains(t, f.text, "observation #3", "only the explicit SSH encrypted diagnostic selects the new source scope")
			}
			for _, source := range sources {
				require.NotContains(t, source.Name, "SSH · encrypted stream")
			}
		})
	}
}
