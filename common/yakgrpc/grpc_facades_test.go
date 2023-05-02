package yakgrpc

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_RegisterFacadesHTTP(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := c.RegisterFacadesHTTP(context.Background(), &ypb.RegisterFacadesHTTPRequest{HTTPResponse: []byte(`HTTP/1.1 200 Ok
Content-Type: text/html

Hello World
aaa




`)})
	if err != nil {
		panic(err)
	}
	time.Sleep(time.Second)

	rspIns, err := http.Get(rsp.GetFacadesUrl())
	if err != nil {
		panic(err)
	}

	raw, _ := utils.HttpDumpWithBody(rspIns, true)
	if !bytes.Contains(raw, []byte(`Hello World`)) {
		panic(1)
	}

	//raw, _ := ioutil.ReadAll(rspIns.Body)
	//spew.Dump(raw)
}

func TestServer_StartFacades(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		return
	}

	stream, err := c.StartFacades(
		context.Background(),
		&ypb.StartFacadesParams{
			EnableDNSLogServer: true,
			DNSLogLocalPort:    853,
			ConnectParam: &ypb.GetTunnelServerExternalIPParams{
				Addr:   "127.0.0.1:64333",
				Secret: "",
			},
			DNSLogRemotePort: 53,
			ExternalDomain:   "hacker.com",
		},
	)
	if err != nil {
		log.Error(err)
		return
	}
	stream.Recv()
}
