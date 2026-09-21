package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func webTraffic() {
	listener, err := net.Listen("tcp", "127.0.0.1:19442")
	must(err)
	up := websocket.Upgrader{EnableCompression: true}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			c, err := up.Upgrade(w, r, nil)
			must(err)
			defer c.Close()
			for i := 0; i < 2; i++ {
				typ, b, err := c.ReadMessage()
				must(err)
				must(c.WriteMessage(typ, b))
			}
			return
		}
		w.Header().Set("Link", "</m1>; rel=preload")
		w.WriteHeader(103)
		w.Header().Set("Trailer", "X-M1")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		if r.Method != "HEAD" {
			_, err := io.WriteString(w, "M1 chunked body")
			must(err)
		}
		w.Header().Set("X-M1", "done")
	})}
	go srv.Serve(listener)
	client := &http.Client{}
	for _, method := range []string{"HEAD", "GET"} {
		req, err := http.NewRequest(method, "http://127.0.0.1:19442/body", nil)
		must(err)
		rsp, err := client.Do(req)
		must(err)
		b, err := io.ReadAll(rsp.Body)
		must(err)
		rsp.Body.Close()
		if method == "GET" && string(b) != "M1 chunked body" {
			panic("HTTP body oracle")
		}
	}
	d := websocket.Dialer{EnableCompression: true}
	c, _, err := d.Dial("ws://127.0.0.1:19442/ws", nil)
	must(err)
	for i := 0; i < 2; i++ {
		msg := strings.Repeat(fmt.Sprintf("M1 websocket %d ", i), 100)
		must(c.WriteMessage(websocket.TextMessage, []byte(msg)))
		typ, b, err := c.ReadMessage()
		must(err)
		if typ != websocket.TextMessage || string(b) != msg {
			panic("WS echo oracle")
		}
	}
	c.Close()
	client.CloseIdleConnections()
	srv.Close()
	time.Sleep(100 * time.Millisecond)
	fmt.Println("HTTP HEAD/GET with 103, chunked+trailers; WebSocket deflate 2+2 messages")
}
