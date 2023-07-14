package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestH2Hijack(t *testing.T) {
	h2Host, h2Port := utils.DebugMockHTTP2(context.Background(), func(req []byte) []byte {
		return req
	})
	h2Addr := utils.HostPort(h2Host, h2Port)
	_, err := yak.NewScriptEngine(10).ExecuteEx(`
rsp,req = poc.HTTP(getParam("packet"), poc.http2(true), poc.https(true))~
dump(rsp)
dump(req)
`, map[string]any{
		"packet": `GET / HTTP/2.0
User-Agent: 111
Host: ` + h2Addr,
	})
	if err != nil {
		panic(err)
	}
}

func TestGRPCMUSTPASS_MITM(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		panic(err)
	}

	var (
		started           bool // MITM正常启动（此时MITM开启HTTP2支持）
		passthroughTested bool // Mock的普通HTTP服务器正常工作
		echoTested        bool // 将MITM作为代理向mock的http服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H1请求
		gzipAutoDecode    bool // 将MITM作为代理向mock的http服务器发包 同时客户端发包被gzip编码 mitm正常处理 mock服务器正常处理 说明整个流程正确处理了gzip编码的情况
		chunkDecode       bool // 将MITM作为代理向mock的http服务器发包 同时客户端发包被gzip编码 且使用chunk编码 mitm正常处理 mock服务器正常处理 说明整个流程正确处理了gzip+chunk编码的情况
		h2Test            bool // 将MITM作为代理向mock的http2服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H2请求和响应
	)

	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		passthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})

	log.Infof("start to mock server: %v", utils.HostPort(mockHost, mockPort))
	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	_ = proxy

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer func() {
		cancel()
	}()

	/* H2 */
	h2Host, h2Port := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
		return req
	})
	h2Addr := utils.HostPort(h2Host, h2Port)
	// 测试我们的h2 mock服务器是否正常工作
	_, err = yak.NewScriptEngine(10).ExecuteEx(`
rsp,req = poc.HTTP(getParam("packet"), poc.http2(true), poc.https(true))~
dump(rsp)
dump(req)
`, map[string]any{
		"packet": `GET / HTTP/2.0
User-Agent: 111
Host: ` + h2Addr,
	})
	if err != nil {
		panic(err)
	}

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(rPort),
		Recover:          true,
		Forward:          true,
		SetAutoForward:   true,
		AutoForwardValue: true,
		EnableHttp2:      true,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			started = true
			go func() {
				defer func() {
					wg.Done()
					cancel()
					if err := recover(); err != nil {
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				var token = utils.RandStringBytes(100)
				var params = map[string]any{
					"packet": lowhttp.FixHTTPRequestOut(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHost, mockPort))),
					"host":  mockHost,
					"port":  mockPort,
					"proxy": proxy,
					"token": token,
				}
				spew.Dump(params)
				// 将MITM作为代理向mock的http服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H1请求
				_, err := yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
dump(rsp)
dump(req)
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				echoTested = true

				var tokenRaw, _ = utils.GzipCompress([]byte(token))
				params["packet"] = lowhttp.ReplaceHTTPPacketBody(utils.InterfaceToBytes(params["packet"]), tokenRaw, false)
				params["packet"] = lowhttp.ReplaceHTTPPacketHeader(utils.InterfaceToBytes(params["packet"]), "Content-Encoding", "gzip")
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
dump(rsp)
dump(req)
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				gzipAutoDecode = true

				tokenRaw, _ = utils.GzipCompress([]byte(token))
				params["packet"] = lowhttp.ReplaceHTTPPacketBody(utils.InterfaceToBytes(params["packet"]), tokenRaw, false)
				params["packet"] = lowhttp.ReplaceHTTPPacketHeader(utils.InterfaceToBytes(params["packet"]), "Content-Encoding", "gzip")
				params["packet"] = lowhttp.HTTPPacketForceChunked(utils.InterfaceToBytes(params["packet"]))

				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.retryTimes(3))~
dump(rsp)
if rsp.Contains(getParam("token")) {
		println("chunk + gzip auto decode success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				chunkDecode = true

				tokenRaw = []byte(token)
				params["h2packet"] = lowhttp.ReplaceHTTPPacketBody([]byte(`GET /mitm/test/h2/token/`+token+` HTTP/2.0
Host: `+h2Addr+`
D: 1
`), tokenRaw, false)
				params["h2host"] = h2Host
				params["h2port"] = h2Port

				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet h2")
packet := getParam("h2packet")
println("-------------------------------------------------------------------------------------")
println("-------------------------------------------------------------------------------------")
println("-------------------------------------------------------------------------------------")
println("-------------------------------------------------------------------------------------")
println("-------------------------------------------------------------------------------------")
println("-------------------------------------------------------------------------------------")
dump(packet)
retry := 10
var rsp, req, err
for retry >0{
	rsp, req, err = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.http2(true), poc.https(true))
	if err != nil{
		retry = retry -1
		sleep(0.5)
		continue
	}
	break
}

dump(rsp)
if rsp.Contains(getParam("token")) {
		println("h2 auto decode success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				// 使用协程进行并发查询
				done := make(chan struct{})
				defer close(done)

				go func() {
					for {
						_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
							SearchURL: "/mitm/test/h2/token/" + token,
						})
						if err != nil {
							panic(err)
						}
						spew.Dump(flows)
						if len(flows) > 0 {
							h2Test = true
						}
						if h2Test {
							done <- struct{}{}
							break
						}
					}
				}()
				select {
				case <-ctx.Done():
					log.Warn("flow history not fully found")
					break
				case <-done:
					log.Infof("flow history all found")
					break
				}
			}()
		}
		spew.Dump(rsp)
	}
	wg.Wait()

	if !started {
		panic("MITM NOT STARTED!")
	}

	if !passthroughTested {
		panic("MITM PASSTHROUGH TEST FAILED")
	}

	if !echoTested {
		panic("MITM ECHO TEST FAILED")
	}

	if !gzipAutoDecode {
		panic("GZIP AUTO DECODE FAILED")
	}

	if !chunkDecode {
		panic("CHUNK DECODE FAILED")
	}

	if !h2Test {
		panic("H2 TEST FAILED")
	}
}

func TestGRPCMUSTPASS_MITM_GM(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		panic(err)
	}

	var (
		started                bool // MITM正常启动（此时MITM开启HTTP2支持）
		gmPassthroughTested    bool // Mock的GM-HTTPS服务器正常工作
		httpPassthroughTested  bool // Mock的HTTP服务器正常工作
		httpsPassthroughTested bool // Mock的HTTPS服务器正常工作
		httpTest               bool // 将开启了GM支持的MITM作为代理向mock的HTTP服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理HTTP请求和响应
		httpsTest              bool // 将开启了GM支持的MITM作为代理向mock的HTTPS服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理Vanilla-HTTPS请求和响应
		gmTest                 bool // 将开启了GM支持的MITM作为代理向mock的GM-HTTPS服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理GM-HTTPS请求和响应
	)

	var mockGMHost, mockGMPort = utils.DebugMockGMHTTP(context.Background(), func(req []byte) []byte {
		gmPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})
	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		httpPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})
	var mockHttpsHost, mockHttpsPort = utils.DebugMockHTTPSEx(func(req []byte) []byte {
		httpsPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() {
		cancel()
	}()

	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	_ = proxy

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(rPort),
		Recover:          true,
		Forward:          true,
		SetAutoForward:   true,
		AutoForwardValue: true,
		EnableGMTLS:      true,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			println("----------------------")
			started = true
			go func() {
				defer func() {
					wg.Done()
					cancel()
					if err := recover(); err != nil {
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				var token = utils.RandStringBytes(100)
				var params = map[string]any{
					"packet": lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /GMTLS`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockGMHost, mockGMPort)),
					"proxy": proxy,
					"token": token,
				}
				spew.Dump(params)

				params["gmHost"] = mockGMHost
				params["gmPort"] = mockGMPort
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("gmHost"), getParam("gmPort")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.https(true))~
dump(rsp)
dump(req)
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				gmPassthroughTested = true

				params["packet"] = lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /HTTPS`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHttpsHost, mockHttpsPort))
				params["httpsHost"] = mockHttpsHost
				params["httpsPort"] = mockHttpsPort
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("httpsHost"), getParam("httpsPort")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.https(true))~
dump(rsp)
dump(req)
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				httpsPassthroughTested = true

				params["packet"] = lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /HTTP`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHost, mockPort))
				params["host"] = mockHost
				params["port"] = mockPort
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
dump(host, port, packet)
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
dump(rsp)
dump(req)
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("not pass!")
}
`, params)
				if err != nil {
					panic(err)
				}
				httpsPassthroughTested = true

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// 使用协程进行并发查询
				done := make(chan struct{})
				defer close(done)

				go func() {
					for {
						_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
							SearchURL: "/GMTLS" + token,
						})
						if err != nil {
							panic(err)
						}

						if len(flows) > 0 {
							gmTest = true
						}

						_, flows, err = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
							SearchURL: "/HTTPS" + token,
						})
						if err != nil {
							panic(err)
						}

						if len(flows) > 0 {
							httpsTest = true
						}

						// 执行查询操作
						_, flows, err = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
							SearchURL: "/HTTP" + token,
						})
						if err != nil {
							panic(err)
						}

						if len(flows) > 0 {
							httpTest = true
						}
						if gmTest && httpsTest && httpTest {
							done <- struct{}{}
							break
						}
					}
				}()

				select {
				case <-ctx.Done():
					log.Warn("flow history not fully found")
					break
				case <-done:
					log.Infof("flow history all found")
					break
				}

			}()

		}
		spew.Dump(rsp)
	}
	wg.Wait()

	if !started {
		panic("MITM NOT STARTED!")
	}

	if !gmPassthroughTested {
		panic("GM PassthroughTEST FAILED")
	}

	if !gmTest {
		panic("GM TEST FAILED")
	}

	if !httpsPassthroughTested {
		panic("HTTPS PassthroughTEST FAILED")
	}

	if !httpsTest {
		panic("HTTPS TEST FAILED")
	}

	if !httpPassthroughTested {
		panic("HTTP PassthroughTEST FAILED")
	}

	if !httpTest {
		panic("HTTP TEST FAILED")
	}

}
