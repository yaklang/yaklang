package utils

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHttpRequestFrameBuilder_3(t *testing.T) {
	packet := `
user-agent: A
:method

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	spew.Dump(fh)
	spew.Dump(h)
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "A", h[0][1])
	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "", fh[0][1])
}

func TestHttpRequestFrameBuilder_2(t *testing.T) {
	packet := `
user-agent: A

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	_ = fh
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "A", h[0][1])
	spew.Dump(h)
}

func TestHttpRequestFrameBuilder(t *testing.T) {
	packet := `
:method: <<<METHOD` + "\nGET\n123\n123\n123\n123\n" + `METHOD
:authority: www.baidu.com
:scheme: https
user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3

abcdefghijklmnopqrstuvwxyz`
	fh, h, _, _ := HTTPFrameParser(bytes.NewReader([]byte(packet)))
	require.Equal(t, 3, len(fh))
	require.Equal(t, 1, len(h))

	require.Equal(t, ":method", fh[0][0])
	require.Equal(t, "GET\n123\n123\n123\n123", fh[0][1])
	require.Equal(t, ":authority", fh[1][0])
	require.Equal(t, "www.baidu.com", fh[1][1])
	require.Equal(t, ":scheme", fh[2][0])
	require.Equal(t, "https", fh[2][1])
	require.Equal(t, "user-agent", h[0][0])
	require.Equal(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3", h[0][1])
}
