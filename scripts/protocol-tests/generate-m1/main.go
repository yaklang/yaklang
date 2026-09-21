// Generate local, authorized M1 capture traffic. Run tcpdump on lo0 around this
// program; the generated key log belongs only to these ephemeral test sessions.
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"
)

type lockedWriter struct {
	sync.Mutex
	w io.Writer
}

func (w *lockedWriter) Write(b []byte) (int, error) { w.Lock(); defer w.Unlock(); return w.w.Write(b) }
func main() {
	defer webTraffic()
	out := flag.String("keylog", "m1-tls.keys", "authorized test key log")
	flag.Parse()
	file, err := os.Create(*out)
	must(err)
	defer file.Close()
	keys := &lockedWriter{w: file}
	for i, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 19440+i))
		must(err)
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ProtoMajor != 2 {
				panic("expected HTTP/2")
			}
			w.Header().Set("Content-Type", "application/grpc")
			w.Header().Set("Trailer", "Grpc-Status")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			for j := 0; j < 3; j++ {
				var h [5]byte
				_, err := io.ReadFull(r.Body, h[:])
				must(err)
				payload := make([]byte, binary.BigEndian.Uint32(h[1:]))
				_, err = io.ReadFull(r.Body, payload)
				must(err)
				if !bytes.Equal(payload, []byte{10, 3, 'm', '1', byte('0' + j)}) {
					panic("unexpected message")
				}
				_, err = w.Write(append(h[:], payload...))
				must(err)
				w.(http.Flusher).Flush()
			}
			w.Header().Set("Grpc-Status", "0")
		}))
		srv.Listener.Close()
		srv.Listener = listener
		srv.EnableHTTP2 = true
		srv.TLS = &tls.Config{MinVersion: version, MaxVersion: version, KeyLogWriter: keys, CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}}
		srv.StartTLS()
		tr := &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "m1.example", MinVersion: version, MaxVersion: version, KeyLogWriter: keys, CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}}}
		rd, wr := io.Pipe()
		req, err := http.NewRequest("POST", srv.URL+"/m1.Echo/Bidi", rd)
		must(err)
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("TE", "trailers")
		done := make(chan error, 1)
		go func() {
			for j := 0; j < 3; j++ {
				_, err := wr.Write([]byte{0, 0, 0, 0, 5, 10, 3, 'm', '1', byte('0' + j)})
				if err != nil {
					done <- err
					return
				}
				time.Sleep(30 * time.Millisecond)
			}
			done <- wr.Close()
		}()
		rsp, err := tr.RoundTrip(req)
		must(err)
		b, err := io.ReadAll(rsp.Body)
		must(err)
		must(rsp.Body.Close())
		must(<-done)
		if len(b) != 30 || rsp.Trailer.Get("Grpc-Status") != "0" {
			panic("echo oracle mismatch")
		}
		tr.CloseIdleConnections()
		srv.Close()
		fmt.Printf("TLS %x h2 bidi: 3 request + 3 response messages, grpc-status=0\n", version)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
