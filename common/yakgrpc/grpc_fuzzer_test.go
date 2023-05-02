package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yakgrpc/ypb"

	"github.com/davecgh/go-spew/spew"
)

func TestServer_HTTPFuzzerBIG(t *testing.T) {
	rPort := utils.GetRandomAvailableTCPPort()
	go yaklib.HTTPServer_Serve("127.0.0.1", rPort, yaklib.HTTPServer_ServeOpt_Callback(func(rsp http.ResponseWriter, req *http.Request) {
		rsp.Write(bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz\n"), 4000000))
	}))

	time.Sleep(2 * time.Second)

	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: %v 

`, utils.HostPort("127.0.0.1", rPort)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
	}
}

func TestServer_HTTPFuzzer(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET /{{rs(10,10,10)}} HTTP/1.1
Host: www.baidu.com

`,
		Concurrent:               10,
		IsHTTPS:                  true,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzerS2008(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET /{{yak(handle123|{{params(test)}})}} HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close


`,
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		HotPatchCode: `
handle123 = func(a) {
	println(a)
	return sprintf("--------------%v",codec.Md5(a))
}
`,
		HotPatchCodeWithParamGetter: `
__getParams__ = func() {
	return {"test": ["ab", "asdfasdfasdfasdf", 123]}
}

`,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer2(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		HistoryWebFuzzerId: 6,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer3(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.QueryHistoryHTTPFuzzerTask(context.Background(), &ypb.Empty{})
	if err != nil {
		panic(err)
	}

	spew.Dump(client)
}

func TestServer_HTTPFuzzerYYOA(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET /yyoa/DownExcelBeanServlet?contenttype=username&contentvalue=&state=1&per_id=0 HTTP/1.1
Host: 14.157.105.194:5002
Pragma: no-cache
Cache-Control: no-cache
DNT: 1
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.88 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Referer: http://222.133.23.90:9000/yyoa/ext/https/getSessionList.jsp?cmd=getAll
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7
Cookie: JSESSIONID=9A2AF446D35187ECF84CBE9B1254B0EE
sec-gpc: 1
Connection: close

`,
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPRequestMutateWithoutConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
		Request: []byte(`POST / HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9


`),
		FuzzMethods: []string{"GET"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(r.Result))
}

func TestServer_HTTPRequestMutateWithConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
		Request: []byte(`POST / HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close


`),
		FuzzMethods: []string{"GET"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(r.Result))
}
