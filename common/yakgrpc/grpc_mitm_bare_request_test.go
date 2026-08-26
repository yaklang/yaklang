//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func buildBareRequestTestPacket(contentType string, body []byte) []byte {
	header := "POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Type: " + contentType +
		"\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	return append([]byte(header), body...)
}

func renderBareRequestTestPacket(t *testing.T, packet []byte) []byte {
	t.Helper()
	rendered, err := mutate.FuzzTagExec(packet, mutate.Fuzz_WithEnableDangerousTag(), mutate.Fuzz_WithResultLimit(1))
	require.NoError(t, err)
	require.Len(t, rendered, 1)
	return []byte(rendered[0])
}

// The original request shown by HTTP History is an executable snapshot, not
// a lossy preview: oversized/big-binary bodies become file-backed tags, while
// small binary remains an editable {{unquote}} value.
func TestPrepareMITMBareRequestForStorage(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	previous := consts.GetGlobalMaxContentLength()
	const limit = 1024 * 1024
	consts.SetGlobalMaxContentLength(limit)
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(previous) })

	tests := []struct {
		name             string
		contentType      string
		body             []byte
		wantExternalized bool
		wantResourceTag  bool
		wantUnquote      bool
	}{
		{
			name:             "text body above D is losslessly externalized",
			contentType:      "text/plain",
			body:             bytes.Repeat([]byte("A"), limit+1),
			wantExternalized: true,
			wantResourceTag:  true,
		},
		{
			name:        "text body exactly at D stays inline",
			contentType: "text/plain",
			body:        bytes.Repeat([]byte("B"), limit),
		},
		{
			name:             "binary whose expanded representation exceeds D is externalized",
			contentType:      "application/octet-stream",
			body:             bytes.Repeat([]byte{0xff}, 300*1024),
			wantExternalized: true,
			wantResourceTag:  true,
		},
		{
			name:        "small binary body stays editable as unquote",
			contentType: "application/octet-stream",
			body:        []byte{0xff, 0x00, 'A'},
			wantUnquote: true,
		},
		{
			name:        "binary MIME with valid UTF8 stays raw",
			contentType: "application/pdf",
			body:        []byte("printable PDF bytes"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet := buildBareRequestTestPacket(tc.contentType, tc.body)
			stored, externalized, originalBodyLen, gotLimit, err := prepareMITMBareRequestForStorage(packet)
			require.NoError(t, err)
			t.Cleanup(func() { yakit.CleanupFuzzableHTTPRequestResources(stored) })
			require.Equal(t, tc.wantExternalized, externalized)
			require.Equal(t, len(tc.body), originalBodyLen)
			require.Equal(t, limit, gotLimit)
			require.Equal(t, tc.wantResourceTag, bytes.Contains(stored, []byte("{{file(")))
			require.Equal(t, tc.wantUnquote, bytes.Contains(stored, []byte("{{unquote(")))

			rendered := renderBareRequestTestPacket(t, stored)
			_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
			require.Equal(t, tc.body, renderedBody)
		})
	}
}
