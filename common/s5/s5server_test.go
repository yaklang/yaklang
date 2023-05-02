package s5

import (
	"io"
	"net"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestSocks5(t *testing.T) {
	lis, err := net.Listen("tcp", "0.0.0.0:7892")
	if err != nil {
		panic(err)
	}

	config, err := NewConfig()
	if err != nil {
		panic(err)
	}
	config.Debug = true
	config.HijackMode = false
	config.DownstreamHTTPProxy = `http://127.0.0.1:8083`

	go func() {
		time.Sleep(time.Second)
		log.Info("start to proxy to baidu.com ")
		rsp, err := lowhttp.SendHttpRequestWithRawPacketWithOptEx(
			lowhttp.WithPacket([]byte(`GET / HTTP/1.1
Host: www.baidu.com

`)), lowhttp.WithHttps(true), lowhttp.WithProxy("s5://127.0.0.1:7892"))
		time.Sleep(time.Second)
		if err != nil {
			panic(err)
		}
		_ = rsp
		//spew.Dump(rsp)
	}()

	for {
		log.Info("start to accept on [::]:7892")
		conn, err := lis.Accept()
		if err != nil {
			panic(err)
		}
		err = config.ServeConn(conn)
		if err != nil && err != io.EOF {
			panic(err)
		} else {
			break
		}
	}

	time.Sleep(3 * time.Second)
}
