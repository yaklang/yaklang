package bin_parser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICBinaryHeaderFieldsSurviveJSONAndVersionSpecificTokenSelection(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		w := []byte{0xc0, 0, 0, 0, 1, 3, 0xff, 0, 0xc3, 2, 0xfe, 0x80, 2, 0xff, 0xfe, 2, 0, 0}
		if v2 {
			copy(w[:5], []byte{0xd0, 0x6b, 0x33, 0x43, 0xcf})
		}
		want, err := structuredNodeReference(w, "application-layer.quic", "QUIC")
		require.NoError(t, err)
		got, err := ParseStructured(w, "application-layer.quic", "QUIC")
		require.NoError(t, err)
		require.Equal(t, want, got)
		f := got["fields"].(map[string]any)
		require.Equal(t, []byte{0xff, 0, 0xc3}, f["DCID"])
		require.Equal(t, []byte{0xfe, 0x80}, f["SCID"])
		require.Equal(t, []byte{0xff, 0xfe}, f["QUICToken"].(map[string]any)["Token"])
		encoded, err := json.Marshal(f)
		require.NoError(t, err)
		var ipc map[string]any
		require.NoError(t, json.Unmarshal(encoded, &ipc))
		b, err := base64.StdEncoding.DecodeString(ipc["DCID"].(string))
		require.NoError(t, err)
		require.Equal(t, []byte{0xff, 0, 0xc3}, b)
		clear(w)
		require.Equal(t, []byte{0xff, 0, 0xc3}, f["DCID"], "projected identifiers own their source octets")
	}
	// QUICv2 type0 is Retry, not Initial: its token has no Initial length prefix.
	retry := append([]byte{0xc0, 0x6b, 0x33, 0x43, 0xcf, 0, 0}, bytes.Repeat([]byte{1}, 17)...)
	f, err := ParseStructured(retry, "application-layer.quic", "QUIC")
	require.NoError(t, err)
	require.NotContains(t, f["fields"], "QUICToken")
}
