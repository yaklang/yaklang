package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_MITM_AUTH(t *testing.T) {
	test := assert.New(t)
	_ = test

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\na"))

	p := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host:            "127.0.0.1",
		Port:            uint32(p),
		ProxyUsername:   "admin" + "@/",
		ProxyPassword:   "@/:",
		EnableProxyAuth: true,
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		msg := string(rsp.GetMessage().GetMessage())
		if strings.Contains(msg, `starting mitm server`) {
			proxy := fmt.Sprintf("http://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("admin@/"),
				codec.QueryEscape("@/:"),
				p,
			)
			conn, err := netx.DialX(utils.HostPort(host, port), netx.DialX_WithProxy(proxy))
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
		}
	}

}
