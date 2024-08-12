package lowhttp

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

/*
payload = codec.DecodeHex(`0905020700000000000001de27750004676d393900053830323832000a3131333330333635373500033136310006363030303333000a3136373731323230373600203537386334366339306433666264633063396231333631326637303963636432017e46355658644f663025324676305a70612532467545586f4a33447374675268253242656d5450374e513151577274646176547645557933384d253242454236517669343643384d527869763942396f464d485474344e563674443643513169544171775a562532425a6159253242504f6a6864583930784d42253246345875643235666f68655a64665a2532464f32694957505138413266396d4359574c6b32777a6752253242657a4d66533545434c586349563041583244394b494d575373706855646f5949526f6663386352704b4a696c52594b6a31763132306c4f49536e61446c25324679654e372532423730436b424e6b6a6a447574514e5454694c72727164675a795431724766673347536d725746372532424c7039613951527775325a6836714b2532467a724733545458577073797548316444574e484d656a6c6e484a3447373170727349652532422532466f3763456e6543516f4370784466253246654e4f6f6a63336661372532464d625a5543546725334425334400066c616e673d31`)~

poc.Websocket(`GET / HTTP/1.1
Host: guandougm99s80282.3syua.com:20005
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: no-cache
Connection: Upgrade
Cookie: PHPSESSID=tqp065evgrn6vnoq58p3n5bc8m; _gcl_au=1.1.814370406.1677121775; _ga=GA1.2.48537097.1677121775; _gid=GA1.2.905502530.1677121776; _gat_UA-119666012-17=1; _u_rem=1133036575%7Cag4bxxsygy03a%7C9076822521935e4fc6eeec7df8780decb74e6fcf.user; _ga_J7FW5SXQKB=GS1.1.1677121775.1.1.1677121833.0.0.0
Origin: https://gdgm99login.3syua.com
Pragma: no-cache
Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
Sec-WebSocket-Key: m9V3lM6k2zmTuwCMW8v7gQ==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36

`, poc.websocketOnClient(client => {
    client.Write(payload)
}), poc.websocketFromServer((result, refuse) => {
    dump(result)
}))

*/
//func TestWs1(t *testing.T) {
//	raw, _ := codec.DecodeHex(`0905020700000000000001de27750004676d393900053830323832000a3131333330333635373500033136310006363030303333000a3136373731323230373600203537386334366339306433666264633063396231333631326637303963636432017e46355658644f663025324676305a70612532467545586f4a33447374675268253242656d5450374e513151577274646176547645557933384d253242454236517669343643384d527869763942396f464d485474344e563674443643513169544171775a562532425a6159253242504f6a6864583930784d42253246345875643235666f68655a64665a2532464f32694957505138413266396d4359574c6b32777a6752253242657a4d66533545434c586349563041583244394b494d575373706855646f5949526f6663386352704b4a696c52594b6a31763132306c4f49536e61446c25324679654e372532423730436b424e6b6a6a447574514e5454694c72727164675a795431724766673347536d725746372532424c7039613951527775325a6836714b2532467a724733545458577073797548316444574e484d656a6c6e484a3447373170727349652532422532466f3763456e6543516f4370784466253246654e4f6f6a63336661372532464d625a5543546725334425334400066c616e673d31`)
//	packet := []byte(`GET / HTTP/1.1
//Host: guandougm99s80282.3syua.com:20005
//Accept-Encoding: gzip, deflate, br
//Accept-Language: zh-CN,zh;q=0.9
//Cache-Control: no-cache
//Connection: Upgrade
//Cookie: PHPSESSID=tqp065evgrn6vnoq58p3n5bc8m; _gcl_au=1.1.814370406.1677121775; _ga=GA1.2.48537097.1677121775; _gid=GA1.2.905502530.1677121776; _gat_UA-119666012-17=1; _u_rem=1133036575%7Cag4bxxsygy03a%7C9076822521935e4fc6eeec7df8780decb74e6fcf.user; _ga_J7FW5SXQKB=GS1.1.1677121775.1.1.1677121833.0.0.0
//Origin: https://gdgm99login.3syua.com
//Pragma: no-cache
//Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
//Sec-WebSocket-Key: m9V3lM6k2zmTuwCMW8v7gQ==
//Sec-WebSocket-Version: 13
//Upgrade: websocket
//User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36
//
//`)
//	client, err := NewWebsocketClient(packet, WithWebsocketFromServerHandler(func(bytes []byte) {
//		spew.Dump(bytes)
//	}))
//	if err != nil {
//		t.FailNow()
//	}
//	go func() {
//		client.Start()
//		client.Wait()
//	}()
//	time.Sleep(time.Second)
//	client.WriteBinary(raw)
//	time.Sleep(time.Second)
//}

func generateAutobahnTestsuiteReport(host string, port int) {
	packet := []byte(`GET /updateReports?agent=my_websocket_client HTTP/1.1
Host: 172.24.179.200:9001
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
Connection: keep-alive, Upgrade
Upgrade: websocket

`)
	c, err := NewWebsocketClient(
		packet,
		WithWebsocketTLS(false),
		WithWebsocketHost(host),
		WithWebsocketPort(port),
	)
	if err != nil {
		panic(err)
	}

	c.Start()
	c.Wait()
}

func TestWEbsocket_AutobahnTestsuite(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
	}

	testServerHostPort := os.Getenv("Autobahn_Server_HostPort")
	if testServerHostPort == "" {
		t.SkipNow()
	}
	host, port, err := utils.ParseStringToHostPort(testServerHostPort)
	require.NoError(t, err)

	generatePacket := func(casetuple string) []byte {
		return []byte(fmt.Sprintf(`GET /runCase?casetuple=%s&agent=yaklang-webscoket-client HTTP/1.1
Host: %s
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
Connection: keep-alive, Upgrade
Upgrade: websocket

`, casetuple, utils.HostPort(host, port)))
	}
	runTestCase := func(t *testing.T, casetuple string, callback func(c *WebsocketClient, bytes []byte, frame []*Frame), opts ...WebsocketClientOpt) error {
		log.Infof("running case %s", casetuple)
		opts = append(opts,
			WithWebsocketFromServerHandlerEx(callback),
			WithWebsocketTLS(false),
			WithWebsocketHost(host),
			WithWebsocketPort(port),
			WithWebsocketStrictMode(true))

		t.Helper()
		c, err := NewWebsocketClient(
			generatePacket(casetuple),
			opts...,
		)
		require.NoError(t, err)

		c.Start()
		c.Wait()
		return nil
	}
	echoCallback := func(c *WebsocketClient, bytes []byte, frame []*Frame) {
		err := c.WriteEx(bytes, frame[0].Type())
		require.NoError(t, err)
	}

	t.Run("Case1", func(t *testing.T) {
		// text
		for i := 1; i <= 8; i++ {
			runTestCase(t, fmt.Sprintf("1.1.%d", i), echoCallback)
		}
		// binary
		for i := 1; i <= 8; i++ {
			runTestCase(t, fmt.Sprintf("1.2.%d", i), echoCallback)
		}
	})

	// ping-pong
	t.Run("Case2", func(t *testing.T) {
		for i := 1; i <= 11; i++ {
			runTestCase(t, fmt.Sprintf("2.%d", i), func(c *WebsocketClient, bytes []byte, frame []*Frame) {
			})
		}
	})

	// rsv
	t.Run("Case3", func(t *testing.T) {
		for i := 1; i <= 7; i++ {
			runTestCase(t, fmt.Sprintf("3.%d", i), echoCallback)
		}
	})

	// opcodes
	t.Run("Case4", func(t *testing.T) {
		// reserved non control
		for i := 1; i <= 5; i++ {
			runTestCase(t, fmt.Sprintf("4.1.%d", i), echoCallback)
		}
		// reserved control
		for i := 1; i <= 5; i++ {
			runTestCase(t, fmt.Sprintf("4.2.%d", i), echoCallback)
		}
	})

	// fragmentation
	t.Run("Case5", func(t *testing.T) {
		// reserved non control
		for i := 1; i <= 20; i++ {
			runTestCase(t, fmt.Sprintf("5.%d", i), echoCallback)
		}
	})

	t.Run("Case6", func(t *testing.T) {
		cases := map[int]int{
			1:  3,  // 6.1 Valid UTF-8 with zero payload fragments
			2:  4,  // 6.2 Valid UTF-8 unfragmented, fragmented on code-points and within code-points
			3:  2,  // 6.3 Invalid UTF-8 differently fragmented
			4:  4,  // 6.4 Fail-fast on invalid UTF-8
			5:  5,  // 6.5 Some valid UTF-8 sequences
			6:  11, // 6.6 All prefixes of a valid UTF-8 string that contains multi-byte code points
			7:  4,  // 6.7 First possible sequence of a certain length
			8:  2,  // 6.8 First possible sequence length 5/6 (invalid codepoints)
			9:  4,  // 6.9 Last possible sequence of a certain length
			10: 3,  // 6.10 Last possible sequence length 4/5/6 (invalid codepoints)
			11: 5,  // 6.11 Other boundary conditions
			12: 8,  // 6.12 Unexpected continuation bytes
			13: 5,  // 6.13 Lonely start characters
			14: 10, // 6.14 Sequences with last continuation byte missing
			15: 1,  // 6.15 Concatenation of incomplete sequences
			16: 3,  // 6.16 Impossible bytes
			17: 5,  // 6.17 Examples of an overlong ASCII character
			18: 5,  // 6.18 Maximum overlong sequences
			19: 5,  // 6.19 Overlong representation of the NUL character
			20: 7,  // 6.20 Single UTF-16 surrogates
			21: 8,  // 6.21 Paired UTF-16 surrogates
			22: 34, // 6.22 Non-character code points (valid UTF-8)
			23: 7,  // 6.23 Unicode specials (i.e. replacement char)
		}

		for i, j := range cases {
			for k := 1; k <= j; k++ {
				runTestCase(t, fmt.Sprintf("6.%d.%d", i, k), echoCallback)
			}
		}
	})

	t.Run("Case7", func(t *testing.T) {
		cases := map[int]int{
			1:  6,  // 7.1 Basic close behavior (fuzzer initiated)
			3:  6,  // 7.3 Close frame structure: payload length (fuzzer initiated)
			5:  1,  // 7.5 Close frame structure: payload value (fuzzer initiated)
			7:  13, // 7.7 Close frame structure: valid close codes (fuzzer initiated)
			9:  9,  // 7.9 Close frame structure: invalid close codes (fuzzer initiated)
			13: 2,  // 7.13 Informational close information (fuzzer initiated)
		}

		for i, j := range cases {
			for k := 1; k <= j; k++ {
				runTestCase(t, fmt.Sprintf("7.%d.%d", i, k), echoCallback)
			}
		}
	})

	t.Run("Case9", func(t *testing.T) {
		cases := map[int]int{
			1: 6, // 9.1 Text Message (increasing size)
			2: 6, // 9.2 Binary Message (increasing size)
			3: 9, // 9.3 Fragmented Text Message (fixed size, increasing fragment size)
			4: 9, // 9.4 Fragmented Binary Message (fixed size, increasing fragment size)
			5: 6, // 9.5 Text Message (fixed size, increasing chop size)
			6: 6, // 9.6 Binary Message (fixed size, increasing chop size)
			7: 6, // 9.7 Text Message Roundtrip Time (fixed number, increasing size)
			8: 6, // 9.8 Binary Message Roundtrip Time (fixed number, increasing size)
		}

		for i, j := range cases {
			for k := 1; k <= j; k++ {
				runTestCase(t, fmt.Sprintf("9.%d.%d", i, k), echoCallback)
			}
		}
	})

	t.Run("Case10", func(t *testing.T) {
		runTestCase(t, "10.1.1", echoCallback)
	})

	t.Run("Case12", func(t *testing.T) {
		cases := map[int]int{
			1: 18, // 12.1 Large JSON data file (utf8, 194056 bytes)
			2: 18, // 12.2 Lena Picture, Bitmap 512x512 bw (binary, 263222 bytes)
			3: 18, // 12.3 Human readable text, Goethe's Faust I (German) (binary, 222218 bytes)
			4: 18, // 12.4 Large HTML file (utf8, 263527 bytes)
			5: 18, // 12.5 A larger PDF (binary, 1042328 bytes)
		}

		for i, j := range cases {
			for k := 1; k <= j; k++ {
				runTestCase(t, fmt.Sprintf("12.%d.%d", i, k), echoCallback, WithWebsocketCompress(true))
			}
		}
	})

	// We skip tests related to requestMaxWindowBits as that is unimplemented due
	// to limitations in compress/flate. See https://github.com/golang/go/issues/3155
	// "13.3.*", "13.4.*", "13.5.*", "13.6.*"
	// Reference: nhooyr.io/websocket/autobahn_test.go:37:1
	t.Run("Case13", func(t *testing.T) {
		cases := map[int]int{
			1: 18, // 13.1 Large JSON data file (utf8, 194056 bytes) - client offers (requestNoContextTakeover, requestMaxWindowBits): [(False, 0)] / server accept (requestNoContextTakeover, requestMaxWindowBits): [(False, 0)]
			2: 18, // 13.2 Large JSON data file (utf8, 194056 bytes) - client offers (requestNoContextTakeover, requestMaxWindowBits): [(True, 0)] / server accept (requestNoContextTakeover, requestMaxWindowBits): [(True, 0)]
			7: 18, // 13.7 Large JSON data file (utf8, 194056 bytes) - client offers (requestNoContextTakeover, requestMaxWindowBits): [(True, 9), (True, 0), (False, 0)] / server accept (requestNoContextTakeover, requestMaxWindowBits): [(True, 9), (True, 0), (False, 0)]
		}

		for i, j := range cases {
			for k := 1; k <= j; k++ {
				runTestCase(t, fmt.Sprintf("13.%d.%d", i, k), echoCallback, WithWebsocketCompressionContextTakeover(true))
			}
		}
	})

	generateAutobahnTestsuiteReport(host, port)
}

// func TestWebsocket_Gorilla_Client(t *testing.T) {
// 	t.SkipNow()

// 	u := url.URL{Scheme: "ws", Host: "172.24.179.200:9001", Path: "/runCase", RawQuery: "casetuple=12.1.1&agent=my_websocket_client"}

// 	c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{
// 		"Sec-WebSocket-Extensions": []string{"permessage-deflate"},
// 	})
// 	if err != nil {
// 		log.Fatal("dial:", err)
// 	}
// 	defer c.Close()

// 	done := make(chan struct{})

// 	go func() {
// 		defer close(done)
// 		for {
// 			_, message, err := c.ReadMessage()
// 			if err != nil {
// 				log.Println("read:", err)
// 				return
// 			}
// 			log.Printf("recv: %s", message)

// 			err = c.WriteMessage(websocket.TextMessage, message)
// 			if err != nil {
// 				log.Println("write:", err)
// 				return
// 			}
// 		}
// 	}()

// 	ticker := time.NewTicker(time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-done:
// 			return
// 		}
// 	}
// }

//
//func TestWebsocket_ServerWSS(t *testing.T) {
//	var upgrader = websocket.Upgrader{}
//
//	f, err := os.CreateTemp("", "test-*.html")
//	if err != nil {
//		panic(err)
//	}
//	f.Write([]byte(`<!DOCTYPE html>
//<html>
//<head>
//    <meta charset="UTF-8"/>
//    <title>Sample of websocket with golang</title>
//	<script
//	  src="https://code.jquery.com/jquery-2.2.4.js"
//	  integrity="sha256-iT6Q9iMJYuQiMWNd9lDyBUStIq/8PuOW33aOqmvFpqI="
//	  crossorigin="anonymous"></script>
//    <!--<script src="http://apps.bdimg.com/libs/jquery/2.1.4/jquery.min.js"></script>-->
//    <script>
//        $(function() {
//            var ws = new WebSocket('wss://' + window.location.host + '/ws');
//            ws.onmessage = function(e) {
//                $('<li>').text(event.data).appendTo($ul);
//            ws.send('{"message":"这是来自html的数据"}');
//            };
//            var $ul = $('#msg-list');
//        });
//    </script>
//</head>
//<body>
//<ul id="msg-list"></ul>
//</body>
//</html>`))
//	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		http.ServeFile(w, r, f.Name())
//	})
//	http.Handle("/", index)
//	http.Handle("/index.html", index)
//	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
//		ws, err := upgrader.Upgrade(w, r, nil)
//		if err != nil {
//			log.Errorf("upgrade failed: %s", err)
//			return
//		}
//		defer ws.Close()
//
//		go func() {
//			for {
//				_, msg, err := ws.ReadMessage()
//				if err != nil {
//					log.Errorf("read msg failed: %s", err)
//					return
//				}
//				fmt.Printf("server recv from client: %s\n", msg)
//			}
//		}()
//
//		for {
//			time.Sleep(time.Second)
//			ws.WriteJSON(map[string]interface{}{
//				"message": fmt.Sprintf("Golang Websocket Message: %v", time.Now()),
//			})
//		}
//	})
//
//	lis, err := net.Listen("tcp", ":8885")
//	if err != nil {
//		panic(err)
//	}
//
//	server := &httptest.Server{
//		Listener: lis,
//		Config:   &http.Server{Handler: http.DefaultServeMux},
//	}
//	server.StartTLS()
//
//	println(server.URL)
//	for {
//		time.Sleep(1 * time.Second)
//	}
//
//	//rootCrt, rootKey, _ := tlsutils.GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
//	//serverCrt, serverKey, _ := tlsutils.SignServerCrtNKeyEx(rootCrt, rootKey, "server", false)
//	//tlsConfig, _ := tlsutils.GetX509ServerTlsConfigWithAuth(rootCrt, serverCrt, serverKey, false)
//	//tlsConfig.InsecureSkipVerify = true
//	//tlsConfig.MinVersion = tls.VersionSSL30
//	//tlsConfig.MaxVersion = tls.VersionTLS13
//	//server := http.Server{
//	//	Addr:      ":8885",
//	//	TLSConfig: tlsConfig,
//	//}
//	//err = server.ListenAndServe()
//	//if err != nil {
//	//	panic(err)
//	//}
//}
//
//func TestWsClient(t *testing.T) {
//	c, err := NewWebsocketClient([]byte(`
//GET /ws HTTP/1.1
//Host: v1ll4n.local:8885
//Accept-Encoding: gzip, deflate, br
//Accept-Language: zh-CN,zh;q=0.9
//Cache-Control: no-cache
//Connection: Upgrade
//Cookie: PHPSESSID=upube8i55iuim3khf5bnvttab7; security=low
//Origin: https://v1ll4n.local:8885
//Pragma: no-cache
//Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
//Sec-WebSocket-Key: 62HzcscpHVLdq0MlgjMA/A==
//Sec-WebSocket-Version: 13
//Upgrade: websocket
//User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36
//`), WithWebsocketTLS(true), WithWebsocketFromServerHandler(func(bytes []byte) {
//		spew.Dump(bytes)
//	}))
//	if err != nil {
//		panic(err)
//	}
//
//	n, err := c.Write([]byte(`{"Hello": "World"}`))
//	if err != nil {
//		panic(err)
//	}
//	spew.Dump(n)
//	time.Sleep(time.Second)
//	c.Start()
//	go func() {
//		time.Sleep(10 * time.Second)
//		c.Stop()
//	}()
//	c.Wait()
//}
