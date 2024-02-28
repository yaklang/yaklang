package lowhttp

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
//		client.StartFromServer()
//		client.Wait()
//	}()
//	time.Sleep(time.Second)
//	client.WriteBinary(raw)
//	time.Sleep(time.Second)
//}

// func TestWEbsocket_Client(t *testing.T) {
// 	packet := []byte(`GET /ws HTTP/1.1
// Host: 172.24.72.17:8884
// Sec-WebSocket-Version: 13
// Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
// Sec-Websocket-Extensions: permessage-deflate; client_max_window_bits
// Connection: keep-alive, Upgrade
// Upgrade: websocket
// `)
// 	c, err := NewWebsocketClient(
// 		packet,
// 		WithWebsocketFromServerHandler(func(bytes []byte) {
// 			println(string(bytes))
// 		}),
// 		WithWebsocketHost("172.24.72.17"),
// 		WithWebsocketPort(8884),
// 	)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	go func() {
// 		for {
// 			time.Sleep(time.Second)
// 			c.WriteText([]byte(strings.Repeat("H", 126)))
// 		}
// 	}()

// 	c.StartFromServer()
// 	c.Wait()

// }

// func TestWebsocket_Server(t *testing.T) {
// 	var upgrader = websocket.Upgrader{}

// 	f, err := os.CreateTemp("", "test-*.html")
// 	if err != nil {
// 		panic(err)
// 	}
// 	f.Write([]byte(`<!DOCTYPE html>
// <html>
// <head>
//    <meta charset="UTF-8"/>
//    <title>Sample of websocket with golang</title>
// 	<script
// 	  src="https://code.jquery.com/jquery-2.2.4.js"
// 	  integrity="sha256-iT6Q9iMJYuQiMWNd9lDyBUStIq/8PuOW33aOqmvFpqI="
// 	  crossorigin="anonymous"></script>
//    <!--<script src="http://apps.bdimg.com/libs/jquery/2.1.4/jquery.min.js"></script>-->
//    <script>
//        $(function() {
//            var ws = new WebSocket('ws://' + window.location.host + '/ws');
//            ws.onmessage = function(e) {
//                $('<li>').text(event.data).appendTo($ul);
//            ws.send('{"message":"这是来自html的数据"}');
//            };
//            var $ul = $('#msg-list');
//        });
//    </script>
// </head>
// <body>
// <ul id="msg-list"></ul>
// </body>
// </html>`))
// 	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		http.ServeFile(w, r, f.Name())
// 	})
// 	http.Handle("/", index)
// 	http.Handle("/index.html", index)
// 	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
// 		ws, err := upgrader.Upgrade(w, r, nil)
// 		if err != nil {
// 			log.Errorf("upgrade failed: %s", err)
// 			return
// 		}
// 		defer ws.Close()

// 		go func() {
// 			for {
// 				_, msg, err := ws.ReadMessage()
// 				if err != nil {
// 					log.Errorf("read msg failed: %s", err)
// 					return
// 				}
// 				fmt.Printf("server recv from client: %s\n", msg)
// 			}
// 		}()

// 		for {
// 			time.Sleep(time.Second)
// 			ws.WriteJSON(map[string]interface{}{
// 				"message": fmt.Sprintf("Golang Websocket Message: %v", time.Now()),
// 			})
// 		}
// 	})

// 	err = http.ListenAndServe(":8884", nil)
// 	if err != nil {
// 		panic(err)
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
//	c.StartFromServer()
//	go func() {
//		time.Sleep(10 * time.Second)
//		c.Stop()
//	}()
//	c.Wait()
//}
