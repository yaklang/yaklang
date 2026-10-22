package pcapdb

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestProtocolBodySummaryCompressedSensitiveBody(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"username":"alice","access_token":"redacted-for-test"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	raw := []byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: body.example\r\nContent-Type: application/json\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", compressed.Len()))
	raw = append(raw, compressed.Bytes()...)
	summary, err := summarizeHTTPBody(PCAPProtocolMessage{Model: gormModel(1), Protocol: "http", Status: "decoded", Data: raw, DataLength: len(raw)}, "")
	require.NoError(t, err)
	require.Equal(t, "POST", summary.Method)
	require.True(t, summary.Compressed)
	require.True(t, summary.TransferComplete)
	require.True(t, summary.DecodedComplete)
	require.Greater(t, summary.WireBodyBytes, summary.DecodedBodyBytes)
	require.NotEmpty(t, summary.WireBodySHA256)
	require.NotEmpty(t, summary.DecodedBodySHA256)
	require.NotEqual(t, summary.WireBodySHA256, summary.DecodedBodySHA256)
	require.True(t, summary.Sensitive)
	require.Contains(t, summary.SensitiveIndicators, "sensitive-body-field")
}

func TestProtocolBodySummaryBinaryBody(t *testing.T) {
	body := []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0x7f}
	raw := []byte(fmt.Sprintf("POST /binary HTTP/1.1\r\nHost: body.example\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", len(body)))
	raw = append(raw, body...)
	summary, err := summarizeHTTPBody(PCAPProtocolMessage{Model: gormModel(2), Protocol: "http", Status: "decoded", Data: raw, DataLength: len(raw)}, "")
	require.NoError(t, err)
	require.True(t, summary.Binary)
	require.False(t, summary.TextLike)
	require.Equal(t, 1, summary.NULBytes)
	require.Equal(t, "unknown-binary", summary.Magic)
	require.True(t, summary.TransferComplete)
}

func TestProtocolBodySummaryTruncatedBodyHasNoHash(t *testing.T) {
	raw := []byte("POST /truncated HTTP/1.1\r\nHost: body.example\r\nContent-Length: 100\r\n\r\nshort")
	summary, err := summarizeHTTPBody(PCAPProtocolMessage{Model: gormModel(3), Protocol: "http", Status: "incomplete", Data: raw, DataLength: len(raw)}, "")
	require.NoError(t, err)
	require.False(t, summary.TransferComplete)
	require.False(t, summary.DecodedComplete)
	require.Empty(t, summary.WireBodySHA256)
	require.Empty(t, summary.DecodedBodySHA256)
	require.NotEmpty(t, summary.Limitations)
}

func gormModel(id uint) gorm.Model {
	return gorm.Model{ID: id}
}
