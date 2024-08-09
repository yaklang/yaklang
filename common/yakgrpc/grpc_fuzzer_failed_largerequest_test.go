package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net"
	"testing"
)

func TestGRPCLargeFuzzerRequest_Failed(t *testing.T) {
	var crazyBody = "{{repeatstr(A|130000000)}}"

	// 构造一个很容易网络错误的东西
	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", "127.0.0.1:"+fmt.Sprint(port))
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()
	err = utils.WaitConnect(utils.HostPort("127.0.0.1", port), 4)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host := "127.0.0.1"
	token := utils.RandStringBytes(10)
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:     "GET /?token=" + token + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n" + crazyBody,
		ForceFuzz:   true,
		RepeatTimes: 20,
	})
	if err != nil {
		panic(err)
	}
	var count int
	for i := 0; i < 30; i++ {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if bytes.Contains(resp.RequestRaw, []byte("show chunked by yakit web fuzzer")) {
			count++
		}
	}
	assert.Equal(t, 20, count)
}
