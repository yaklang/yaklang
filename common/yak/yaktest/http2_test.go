package yaktest

import (
	"fmt"
	tls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"net/http"
	"testing"
	"time"
)

func TestHttp2C(t *testing.T) {
	h2s := &http2.Server{}
	var handler = h2c.NewHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bytes, _ := utils.HttpDumpWithBody(request, true)
		if !utils.MatchAnyOfSubString(bytes, "HTTP/2") {
			writer.Write([]byte("no h2"))
			writer.WriteHeader(201)
			return
		}
		writer.Write([]byte("HELLO H2c!"))
	}), h2s)
	h1s := &http.Server{Addr: ":8881", Handler: handler}
	println("http://127.0.0.1:8881")
	h1s.ListenAndServe()
	time.Sleep(1 * time.Second)

	var client = http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
		},
		Timeout: 10 * time.Second,
	}
	rsp, err := client.Get("http://127.0.0.1")
	if err != nil {
		panic(err)
	}
	var rspRaw, rspErr = utils.HttpDumpWithBody(rsp, true)
	if rspErr != nil {
		panic(rspErr)
	}
	println(string(rspRaw))
}

func TestHttp2TLS(t *testing.T) {
	ca, caKey, _ := tlsutils.GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	tlsCert, tlsKey, _ := tlsutils.SignServerCrtNKeyEx(ca, caKey, "Server", false)
	clientCert, clientKey, _ := tlsutils.SignClientCrtNKeyEx(ca, caKey, "Client", false)
	serverConfig, _ := tlsutils.GetX509ServerTlsConfigWithAuth(ca, tlsCert, tlsKey, false)
	serverConfig.NextProtos = append(serverConfig.NextProtos, http2.NextProtoTLS)
	clientConfig, _ := tlsutils.GetX509MutualAuthClientTlsConfig(clientCert, clientKey, ca)
	clientConfig.NextProtos = append(clientConfig.NextProtos, http2.NextProtoTLS)

	if serverConfig == nil || clientConfig == nil {
		panic("no config tls")
	}

	srv := &http.Server{Addr: ":8882", Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r, err := utils.HttpDumpWithBody(request, true)
		if err != nil {
			println(string(err.Error()))
		}
		println(string(r))
		writer.Write([]byte("HELLO HTTP2"))
	})}
	var err = http2.ConfigureServer(srv, &http2.Server{})
	if err != nil {
		panic(err)
	}

	l, err := tls.Listen("tcp", ":8882", serverConfig)
	if err != nil {
		panic(err)
	}
	u := "https://127.0.0.1:8882"
	println("HTTP2", u)
	go func() {
		println("START TO SERVE HTTP2")
		srv.Serve(l)
	}()
	time.Sleep(1 * time.Hour)

	//time.Sleep(1 * time.Hour)

	h2c := http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: clientConfig,
		},
		Timeout: 10 * time.Second,
	}
	rsp, err := h2c.Get(u)
	if err != nil {
		panic(err)
	}
	var rspRaw, rspErr = utils.HttpDumpWithBody(rsp, true)
	if rspErr != nil {
		panic(rspErr)
	}
	println(string(rspRaw))
}

func TestMisc_PocHTTP2(t *testing.T) {
	go TestHttp2TLS(t)
	time.Sleep(1 * time.Second)

	c := http.Client{Transport: &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	var rsp, err = c.Get("https://127.0.0.1:8882")
	if err != nil {
		panic(err)
	}
	utils.HttpShow(rsp)

	//	cases := []YakTestCase{
	//		{
	//			Name: "测试数据库",
	//			Src: fmt.Sprintf(`
	//rsp, req, err = poc.HTTP("GET / HTTP/2.0\r\nHost: 127.0.0.1:8882\r\nUser-Agent: testAgent\r\n", poc.https(true)); println(string(rsp))
	//`),
	//		},
	//	}
	//
	//	Run("测试数据库链接", t, cases...)
}

func TestMisc_PocHTTP2_POC(t *testing.T) {
	go TestHttp2TLS(t)
	time.Sleep(1 * time.Second)

	cases := []YakTestCase{
		{
			Name: "测试数据库",
			Src: fmt.Sprintf(`
env.Set("HTTP_PROXY", "")
rsp, req, err = poc.HTTP("GET / HTTP/2.0\r\nHost: 127.0.0.1:8882\r\nUser-Agent: testAgent\r\n", poc.https(true)); println(string(rsp))
	`),
		},
	}

	Run("测试数据库链接", t, cases...)
}
