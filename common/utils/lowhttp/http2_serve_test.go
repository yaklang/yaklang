package lowhttp

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
)

func TestH2_Serve(t *testing.T) {
	var port int
	var lis net.Listener
	var err error
	for i := 0; i < 10; i++ {
		port = utils.GetRandomAvailableTCPPort()
		lis, err = net.Listen("tcp", utils.HostPort("127.0.0.1", port))
		if err != nil {
			t.Error(err)
			continue
		}
		break
	}
	if lis == nil {
		t.Fatal("lis is nil")
	}
	defer lis.Close()

	token1, token2 := utils.RandStringBytes(20), utils.RandStringBytes(200)
	checkPass := false
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				err = serveH2(conn, conn, withH2Handler(func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
					spew.Dump(header)
					if strings.Contains(string(header), "GET /"+token1+" HTTP/2") {
						checkPass = true
					}
					resp := []byte(`HTTP/2 200 OK
Content-Type: text/plain
Content-Length: 3

abc`)
					return resp, io.NopCloser(bytes.NewBufferString(token2)), nil
				}))
				if err != nil {
					return
				}
			}()
		}
	}()

	err = utils.WaitConnect(utils.HostPort("127.0.0.1", port), 5)
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := HTTPWithoutRedirect(WithHttps(false), WithHttp2(true), WithPacketBytes([]byte(`GET /`+token1+` HTTP/2
Host: www.example.com

abc`)), WithHost("127.0.0.1"), WithPort(port), WithRetryTimes(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp.RawPacket)

	if !checkPass {
		t.Fatal("checkPass failed (h2 server cannot serve)")
	}
	if !bytes.Contains(rsp.RawPacket, []byte(token2)) {
		t.Fatal("token2 not found in response")
	}
}
