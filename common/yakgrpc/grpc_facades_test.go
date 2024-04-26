package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
	"time"
)

func TestServer_RegisterFacadesHTTP(t *testing.T) {
	c, err := NewLocalClientWithReverseServer()
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
func TestGenClass(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		return
	}
	randClassName := utils.RandStringBytes(8)
	port := utils.GetRandomAvailableTCPPort()
	stream, err := c.StartFacadesWithYsoObject(
		context.Background(),
		&ypb.StartFacadesWithYsoParams{
			Token:               "xxx",
			ReversePort:         int32(port),
			ReverseHost:         "127.0.0.1",
			GenerateClassParams: &ypb.YsoOptionsRequerst{},
		},
	)
	if err != nil {
		log.Error(err)
		return
	}
	go func() {
		for {
			stream.Recv()
		}
	}()
	utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port), 3)
	_, err = c.ApplyClassToFacades(context.Background(), &ypb.ApplyClassToFacadesParamsWithVerbose{
		Token: "xxx",
		GenerateClassParams: &ypb.YsoOptionsRequerstWithVerbose{
			Gadget: "None",
			Class:  "DNSLog",
			Options: []*ypb.YsoClassGeneraterOptionsWithVerbose{
				{
					Key:   "className",
					Value: randClassName,
				},
				{
					Key:   "domain",
					Value: "aaa",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/%s.class", port, randClassName))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	if !bytes.Contains(raw, []byte(randClassName)) {
		t.Fatal("not found")
	}
}
