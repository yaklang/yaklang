package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// These are record-authentication observations, not decoded handshake fields.
func TestTLSHandshakeBoundaryDetailsKeepEvidenceUnderRecordDecoding(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: []byte{23, 3, 3, 0, 1, 42}, Session: map[string]any{
		"Record Type": byte(23), "Record Version": uint16(0x303), "Record Length": 1,
		"Negotiated Version": uint16(0x304), "Cipher Suite": uint16(0x1301),
		"Content Visibility": "authenticated-plaintext", "Authentication Verified": true, "Inner Content Type": byte(22),
		"Discarded Handshake Fragment Bytes": 7,
		"Handshake Boundary":                 "unavailable after authenticated key-epoch recovery",
		"Handshake Parsing":                  "unavailable after missing protected handshake bytes",
	}}
	before, err := json.Marshal(e)
	require.NoError(t, err)
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		decoding, connection := -1, -1
		for i, f := range view.fields {
			if f.text == "Record decoding" {
				decoding = i
			}
			if f.text == "Observed connection" {
				connection = i
			}
		}
		require.GreaterOrEqual(t, decoding, 0)
		require.Greater(t, connection, decoding)
		for _, name := range []string{"Discarded Handshake Fragment Bytes", "Handshake Boundary", "Handshake Parsing"} {
			found := 0
			for i, f := range view.fields {
				if strings.HasPrefix(f.text, name+":") {
					found++
					require.Greater(t, i, decoding)
					require.Less(t, i, connection)
					require.Equal(t, 2, f.depth)
					require.False(t, f.hasRange)
					require.Zero(t, f.source)
				}
			}
			require.Equal(t, 1, found, name)
		}
		require.Len(t, view.sources, 1)
		require.Equal(t, e.Raw, view.sources[0].Data)
	}
	after, err := json.Marshal(e)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
