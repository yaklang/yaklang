// Run with tcpdump on lo0: tcp port 19543. Only ephemeral test keys are written.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
)

func main() {
	path := flag.String("keylog", "tls-hrr.keys", "test-only TLS secrets")
	flag.Parse()
	keys, err := os.Create(*path)
	must(err)
	defer keys.Close()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello-retry-authenticated\n") }))
	srv.Listener.Close()
	srv.Listener, err = net.Listen("tcp", "127.0.0.1:19543")
	must(err)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.CurveP256}, KeyLogWriter: keys}
	srv.StartTLS()
	defer srv.Close()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	response, err := client.Get(srv.URL + "/hello-retry")
	must(err)
	body, err := io.ReadAll(response.Body)
	must(err)
	response.Body.Close()
	if string(body) != "hello-retry-authenticated\n" {
		panic("response oracle mismatch")
	}
	fmt.Printf("Go TLS 1.3 P256-only server / X25519-first client: %s %s", response.Status, body)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
