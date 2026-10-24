// Explicit integration check: record only this process's reserved source ports.
// Public HTTPS and HTTP/3 are optional network-dependent additions to the
// deterministic live loopback services. Certificates use the system root pool.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yaklang/pcap"
)

type lockedWriter struct {
	sync.Mutex
	io.Writer
}

func (w *lockedWriter) Write(b []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	return w.Writer.Write(b)
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
func main() {
	if len(os.Args) != 2 {
		panic("usage: internet-audit OUTPUT_DIRECTORY")
	}
	out := os.Args[1]
	must(os.Mkdir(out, 0700))
	keyfile, e := os.OpenFile(filepath.Join(out, "session.keys"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	defer keyfile.Close()
	keys := &lockedWriter{Writer: keyfile}
	l, e := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4zero})
	must(e)
	tcpPort := l.Addr().(*net.TCPAddr).Port
	u, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	must(e)
	defer u.Close()
	udpPort := u.LocalAddr().(*net.UDPAddr).Port
	devs, e := pcap.FindAllDevs()
	must(e)
	filter := fmt.Sprintf("tcp port %d or udp port %d", tcpPort, udpPort)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	captures := []map[string]any{}
	for i, dev := range devs {
		if len(dev.Addresses) == 0 {
			continue
		}
		h, err := pcap.OpenLive(dev.Name, 65535, false, 100*time.Millisecond)
		if err != nil {
			continue
		}
		if err = h.SetBPFFilter(filter); err != nil {
			h.Close()
			continue
		}
		filename := fmt.Sprintf("interface-%d.pcap", i)
		f, err := os.OpenFile(filepath.Join(out, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		must(err)
		w := pcapgo.NewWriterNanos(f)
		must(w.WriteFileHeader(65535, h.LinkType()))
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer h.Close()
			defer f.Close()
			frames := 0
			var readError string
			for {
				select {
				case <-stop:
					goto done
				default:
				}
				b, ci, err := h.ReadPacketData()
				if err == pcap.NextErrorTimeoutExpired {
					continue
				}
				if err != nil {
					readError = err.Error()
					break
				}
				if err = w.WritePacket(ci, b); err != nil {
					readError = err.Error()
					break
				}
				frames++
			}
		done:
			drops := 0
			if s, err := h.Stats(); err == nil {
				drops = s.PacketsDropped
			}
			mu.Lock()
			captures = append(captures, map[string]any{"file": filename, "device": dev.Name, "description": dev.Description, "frames": frames, "drops": drops, "error": readError})
			mu.Unlock()
		}()
	}
	must(l.Close())
	operations := []map[string]any{}
	dialer := &net.Dialer{Timeout: 8 * time.Second, LocalAddr: &net.TCPAddr{IP: net.IPv4zero, Port: tcpPort}}
	tr := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, KeyLogWriter: keys}}
	c := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	for i := 0; i < 2; i++ {
		r, err := c.Get("https://www.cloudflare.com/cdn-cgi/trace")
		op := map[string]any{"protocol": "https", "index": i}
		if err != nil {
			op["error"] = err.Error()
		} else {
			n, readErr := io.Copy(io.Discard, r.Body)
			r.Body.Close()
			op["bytes"], op["http"], op["status"], op["tls"], op["suite"], op["alpn"] = n, r.Proto, r.StatusCode, r.TLS.Version, r.TLS.CipherSuite, r.TLS.NegotiatedProtocol
			if readErr != nil {
				op["error"] = readErr.Error()
			}
		}
		operations = append(operations, op)
	}
	tr.CloseIdleConnections()
	qt := &quic.Transport{Conn: u}
	defer qt.Close()
	h3 := &http3.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, KeyLogWriter: keys}, Dial: func(ctx context.Context, addr string, cfg *tls.Config, qcfg *quic.Config) (quic.EarlyConnection, error) {
		a, err := net.ResolveUDPAddr("udp4", addr)
		if err != nil {
			return nil, err
		}
		return qt.DialEarly(ctx, a, cfg, qcfg)
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", "https://www.cloudflare.com/cdn-cgi/trace", nil)
	must(e)
	r, err := h3.RoundTrip(req)
	op := map[string]any{"protocol": "http3"}
	if err != nil {
		op["error"] = err.Error()
	} else {
		n, readErr := io.Copy(io.Discard, r.Body)
		r.Body.Close()
		op["bytes"], op["http"], op["status"] = n, r.Proto, r.StatusCode
		if readErr != nil {
			op["error"] = readErr.Error()
		}
	}
	operations = append(operations, op)
	h3.Close()
	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()
	manifest := map[string]any{"Filter": filter, "Ports": map[string]int{"tls_internet": tcpPort, "quic_client": udpPort}, "Operations": operations, "Captures": captures}
	b, e := json.MarshalIndent(manifest, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "manifest.json"), b, 0600))
	fmt.Println(string(b))
}
