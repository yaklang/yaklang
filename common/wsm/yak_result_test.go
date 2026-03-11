package wsm

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBehinderEchoResultDecodeFormYakKeepsWholeByteSlice(t *testing.T) {
	manager := &Behinder{
		PayloadScriptContent: `
wsmPayloadDecoder = func(reqBody) {
    return codec.DecodeBase64(string(reqBody))~
}
`,
	}
	expected := []byte(`{"status":"c3VjY2Vzcw==","msg":"aGVsbG8="}`)
	raw := []byte(base64.StdEncoding.EncodeToString(expected))

	decoded, err := manager.EchoResultDecodeFormYak(raw)
	require.NoError(t, err)
	require.Equal(t, expected, decoded)
}

func TestYakResultToBytesJoinsYakByteArrays(t *testing.T) {
	raw := yakResultToBytes([]interface{}{123, 34, 97, 34, 58, 49, 125})
	require.Equal(t, []byte(`{"a":1}`), raw)
}
