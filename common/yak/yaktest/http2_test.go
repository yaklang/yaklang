package yaktest

import (
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"net/http"
	"testing"
	"time"
)

//func TestHttp2ClientManual(t *testing.T) {
//	conn, err := tls.Dial("tcp", "127.0.0.1:8882", &tls.Config{
//		Rand:                        nil,
//		Time:                        nil,
//		Certificates:                nil,
//		NameToCertificate:           nil,
//		GetCertificate:              nil,
//		GetClientCertificate:        nil,
//		GetConfigForClient:          nil,
//		VerifyPeerCertificate:       nil,
//		VerifyConnection:            nil,
//		RootCAs:                     nil,
//		NextProtos:                  []string{"h2"},
//		ServerName:                  "127.0.0.1",
//		ClientAuth:                  0,
//		ClientCAs:                   nil,
//		InsecureSkipVerify:          true,
//		CipherSuites:                nil,
//		PreferServerCipherSuites:    false,
//		SessionTicketsDisabled:      false,
//		SessionTicketKey:            [32]byte{},
//		ClientSessionCache:          nil,
//		MinVersion:                  0,
//		MaxVersion:                  0,
//		CurvePreferences:            nil,
//		DynamicRecordSizingDisabled: false,
//		Renegotiation:               0,
//		KeyLogWriter:                nil,
//	})
//	if err != nil {
//		panic(err)
//	}
//	bw, br := bufio.NewWriter(conn), bufio.NewReader(conn)
//	frame := http2.NewFramer(bw, br)
//
//
//
//	// 一般来说要写三个设置
//	// 第一个写用户 preface，新建链接必备
//	bw.Write([]byte(http2.ClientPreface))
//	frame.WriteSettings([]http2.Setting{
//		{ID: http2.SettingEnablePush, Val: 0},
//		{ID: http2.SettingInitialWindowSize, Val: 65535}, // 这是默认值
//		{ID: http2.SettingMaxConcurrentStreams, Val: 100},
//		//{ID: http2.SettingMaxHeaderListSize, Val: 1000},
//	}...)
//	frame.WriteWindowUpdate(0, transportDefaultConnFlow)
//	bw.Flush()
//
//	// 写请求包的头
//	var hbuf bytes.Buffer
//	henc := hpack.NewEncoder(&hbuf)
//
//	req, err := http.NewRequest("GET", "http://127.0.0.1:8881", http.NoBody)
//	if err != nil {
//		panic(err)
//	}
//	var contentLength int64 = 0
//	var trailers = commaSeparatedTrailers(req)
//	err = enumrateHeadersFromRequest("127.0.0.1", contentLength, req, henc)
//	if err != nil {
//		panic(err)
//	}
//	var id = 1
//	defaultMaxFrameSize := 16 << 10
//	endStream := contentLength <= 0 && trailers == ""
//	err = h2FramerWriteHeaders(frame, uint32(id), endStream, defaultMaxFrameSize, hbuf.Bytes())
//	if err != nil {
//		panic(err)
//	}
//	bw.Flush()
//
//	var headers []hpack.HeaderField
//	var endHeader bool
//	_ = endHeader
//	var endDataStream bool
//	var responseBody bytes.Buffer
//	for {
//		frameResponse, err := frame.ReadFrame()
//		if err != nil {
//			panic(err)
//		}
//		println(frameResponse.Header().String())
//
//		switch ret := frameResponse.(type) {
//		case *http2.HeadersFrame:
//			//case *http2.MetaHeadersFrame:
//			hpackHeader := ret.HeaderBlockFragment()
//			headers, err := hpack.NewDecoder(4096*4, func(f hpack.HeaderField) {
//				println()
//			}).DecodeFull(hpackHeader)
//			if err != nil {
//				log.Errorf("hpack decode failed: %s", err)
//			}
//			for _, h := range headers {
//				println(h.String())
//				headers = append(headers, h)
//			}
//			if ret.StreamEnded() {
//				endDataStream = true
//				endHeader = true
//			}
//
//			if ret.HeadersEnded() {
//				endHeader = true
//			}
//		case *http2.DataFrame:
//			responseBody.Write(ret.Data())
//			if ret.StreamEnded() {
//				endDataStream = true
//			}
//		}
//
//		if endDataStream {
//			break
//		}
//	}
//
//	var pseudoHeader []hpack.HeaderField
//	var regularHeader []hpack.HeaderField
//	var statusCode int
//	for _, i := range headers {
//		if i.IsPseudo() {
//			pseudoHeader = append(pseudoHeader, i)
//			if strings.ToLower(i.Name) == ":status" {
//				statusCode, _ = strconv.Atoi(i.Value)
//			}
//		} else {
//			regularHeader = append(regularHeader, i)
//		}
//	}
//	header := make(http.Header, len(regularHeader))
//	if statusCode <= 0 {
//		statusCode = 200
//	}
//	for _, rh := range regularHeader {
//		header.Add(rh.Name, rh.Value)
//	}
//
//	cl := responseBody.Len()
//	rsp := &http.Response{
//		Status:        http.StatusText(statusCode),
//		StatusCode:    statusCode,
//		Proto:         "HTTP/2.0",
//		ProtoMajor:    2,
//		Header:        header,
//		Body:          ioutil.NopCloser(&responseBody),
//		ContentLength: int64(cl),
//	}
//	rspBytes, _ := utils.HttpDumpWithBody(rsp, true)
//	println(string(rspBytes))
//
//	//var raw = utils.StableReader(br, 3*time.Second, 10000)
//	//spew.Dump(raw)
//}

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
