package lowhttp

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

var reqPacket1_1 = []byte(`GET /test HTTP/1.1
Host: quic.nginx.org

`)

var reqPacket3_0 = []byte(`GET /test HTTP/1.1
Host: quic.nginx.org

`)

func TestHttp3Request(t *testing.T) {
	t.Skip("skip test")
	redirect, err := HTTPWithoutRedirect(WithPacketBytes(reqPacket3_0), WithTimeout(5))
	if err != nil {
		return
	}
	fmt.Printf("%s\n", redirect.RawPacket)
	require.Equal(t, 200, redirect.GetStatusCode())
	proto, _, _ := GetHTTPPacketFirstLine(redirect.RawPacket)
	require.Equal(t, "HTTP/3.0", proto)
}

func TestHttp3Request2(t *testing.T) {
	t.Skip("skip test")
	redirect, err := HTTPWithoutRedirect(WithPacketBytes(reqPacket1_1), WithTimeout(5), WithHttp3(true))
	if err != nil {
		return
	}
	fmt.Printf("%s\n", redirect.RawPacket)
	require.Equal(t, 200, redirect.GetStatusCode())
	proto, _, _ := GetHTTPPacketFirstLine(redirect.RawPacket)
	require.Equal(t, "HTTP/3.0", proto)
}
