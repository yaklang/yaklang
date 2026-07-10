package yakgrpc

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_EncodeHTTPPacketContent_TextAndPosition(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	packet := "POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello"

	t.Run("body base64", func(t *testing.T) {
		rsp, err := client.EncodeHTTPPacketContent(context.Background(), &ypb.EncodeHTTPPacketContentRequest{
			Text:         packet,
			Position:     "body",
			EncodingType: "base64",
		})
		require.NoError(t, err)
		require.Empty(t, rsp.GetError())
		require.Equal(t, "aGVsbG8=", rsp.GetEncodedText())
	})

	t.Run("header md5", func(t *testing.T) {
		rsp, err := client.EncodeHTTPPacketContent(context.Background(), &ypb.EncodeHTTPPacketContentRequest{
			Text:         packet,
			Position:     "header",
			EncodingType: "md5",
		})
		require.NoError(t, err)
		require.Empty(t, rsp.GetError())
		require.NotEmpty(t, rsp.GetEncodedText())
	})
}

func TestGRPCMUSTPASS_EncodeHTTPPacketContent_HTTPFlowId(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	flow := &schema.HTTPFlow{
		Request: strconvQuote(`GET / HTTP/1.1\r\nHost: example.com\r\n\r\nworld`),
		IsHTTPS: false,
		Url:     "http://example.com/",
		Method:  "GET",
	}
	err = yakit.InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
	require.NoError(t, err)

	rsp, err := client.EncodeHTTPPacketContent(context.Background(), &ypb.EncodeHTTPPacketContentRequest{
		HTTPFlowId:   int64(flow.ID),
		IsRequest:    true,
		Position:     "body",
		EncodingType: "base64",
	})
	require.NoError(t, err)
	require.Empty(t, rsp.GetError())
	require.Equal(t, "d29ybGQ=", rsp.GetEncodedText())
}

func TestGRPCMUSTPASS_EncodeHTTPPacketContent_SaveToFile(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	rsp, err := client.EncodeHTTPPacketContent(context.Background(), &ypb.EncodeHTTPPacketContentRequest{
		Text:         "hello",
		EncodingType: "base64",
		SaveToFile:   true,
	})
	require.NoError(t, err)
	require.Empty(t, rsp.GetError())
	require.Empty(t, rsp.GetEncodedText())
	require.NotEmpty(t, rsp.GetSavedPath())
	require.NotEmpty(t, rsp.GetSavedDir())
	require.True(t, strings.HasPrefix(rsp.GetSavedDir(), consts.GetDefaultYakitBaseTempDir()))

	data, err := os.ReadFile(rsp.GetSavedPath())
	require.NoError(t, err)
	require.Equal(t, "aGVsbG8=", string(data))
	_ = os.Remove(rsp.GetSavedPath())
}

func TestGRPCMUSTPASS_EncodeHTTPPacketContent_SaveToCustomPath(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	dir := t.TempDir()
	target := dir + string(os.PathSeparator) + "encoded.txt"

	rsp, err := client.EncodeHTTPPacketContent(context.Background(), &ypb.EncodeHTTPPacketContentRequest{
		Text:         "hello",
		EncodingType: "base64",
		SaveToFile:   true,
		FilePath:     target,
	})
	require.NoError(t, err)
	require.Empty(t, rsp.GetError())
	require.Equal(t, target, rsp.GetSavedPath())
	require.Equal(t, dir, rsp.GetSavedDir())
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
