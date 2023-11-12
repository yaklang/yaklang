package lowhttp

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"strings"
	"testing"
)

func TestH2_Serve(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort("127.0.0.1", port))
	if err != nil {
		t.Error(err)
		t.FailNow()
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
		}
	}()

	rsp, err := HTTPWithoutRedirect(WithHttps(false), WithHttp2(true), WithPacketBytes([]byte(`GET /`+token1+` HTTP/2
Host: www.example.com

abc`)), WithHost("127.0.0.1"), WithPort(port))
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
