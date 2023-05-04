package crep

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestMITM_WebsocketHijack(t *testing.T) {
	test := assert.New(t)

	rs, err := NewMITMServer(
		// MITM_SetWebsocketRequestMirrorRaw(func(req []byte) {
		// 	t.Logf("mirror websocket request: %s\n", req)
		// }),
		// MITM_SetWebsocketResponseMirrorRaw(func(req []byte) {
		// 	t.Logf("mirror websocket response: %s\n", req)
		// }),
		MITM_SetWebsocketHijackMode(true),
		MITM_SetWebsocketRequestHijackRaw(func(req []byte, r *http.Request, rspIns *http.Response, ts int64) []byte {
			t.Logf("hijack websocket request: %s\n", req)
			return req
		}),
		MITM_SetWebsocketResponseHijackRaw(func(rsp []byte, r *http.Request, rspIns *http.Response, ts int64) []byte {
			return []byte("hijack response\n")
		}),
	)
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	addr := ":55343"

	go func() {
		err := rs.Serve(context.Background(), addr)
		if err != nil {
			test.FailNow(err.Error())
		}
	}()
	time.Sleep(1 * time.Second)

	var upgrader = websocket.Upgrader{}

	f, err := os.CreateTemp("", "test-*.html")
	if err != nil {
		panic(err)
	}
	f.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8"/>
    <title>Sample of websocket with golang</title>
    <script src="http://apps.bdimg.com/libs/jquery/2.1.4/jquery.min.js"></script>
    <script>
        $(function() {
            var ws = new WebSocket('ws://' + window.location.host + '/ws');
            ws.onmessage = function(e) {
                $('<li>').text(event.data).appendTo($ul);
            ws.send('{"message":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}');
            };
            var $ul = $('#msg-list');
        });
    </script>
</head>
<body>
<ul id="msg-list"></ul>
</body>
</html>`))
	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, f.Name())
	})
	http.Handle("/", index)
	http.Handle("/index.html", index)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// msg := &RecvMessage{}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
			return
		}
		defer ws.Close()

		go func() {
			for {
				_, msg, err := ws.ReadMessage()
				if err != nil {
					panic(err)
					return
				}
				fmt.Printf("server recv from client: %s\n", msg)
			}
		}()

		for {
			time.Sleep(time.Second)
			ws.WriteJSON(map[string]interface{}{
				"message": fmt.Sprintf("Golang Websocket Message: %v", time.Now()),
			})
		}
	})

	err = http.ListenAndServe(":8884", nil)
	if err != nil {
		panic(err)
	}
}
