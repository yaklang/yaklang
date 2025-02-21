package yaklib

import (
	"bytes"
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"testing"
)

func TestMitmTun(t *testing.T) {
	t.SkipNow()
	mitm, err := buildTunMITM(
		mitmConfigHijackHTTPRequest(func(isHttps bool, u string, req []byte, modified func([]byte), dropped func()) {
		}), mitmConfigHijackHTTPResponse(func(isHttps bool, u string, rsp []byte, modified func([]byte), dropped func()) {
			modified(bytes.Replace(rsp, []byte("百度"), []byte("yak"), -1))
		}),
	)
	require.NoError(t, err)

	go func() {
		err := mitm.TunStart(context.Background())
		spew.Dump(err)
	}()

	err = mitm.AddHijackTarget("www.baidu.com")
	require.NoError(t, err)

	select {}
}

func TestDial(t *testing.T) {
	t.SkipNow()
	conn, err := net.DialTCP("tcp", &net.TCPAddr{
		IP: net.ParseIP("169.254.202.186"),
	}, &net.TCPAddr{
		IP:   net.ParseIP("183.2.172.42"),
		Port: 80,
	})
	require.NoError(t, err)
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"))
	resp, err := utils.ReadHTTPResponseFromBufioReader(conn, nil)
	require.NoError(t, err)
	spew.Dump(resp)
	select {}
}

func TestDial2(t *testing.T) {
	t.SkipNow()
	conn, err := net.Dial("tcp", "183.2.172.42:80")
	require.NoError(t, err)
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"))
	resp, err := utils.ReadHTTPResponseFromBufioReader(conn, nil)
	require.NoError(t, err)
	spew.Dump(resp)
	select {}
}
