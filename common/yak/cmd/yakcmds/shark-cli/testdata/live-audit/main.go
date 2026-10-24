// This explicit Windows/Npcap integration harness starts only loopback test
// services. It records new real socket traffic and session-owned NSS keys to a
// caller-selected output directory, never to the sealed regression corpus.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yaklang/pcap"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/sys/windows"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type lockedWriter struct {
	mu sync.Mutex
	io.Writer
}

func (w *lockedWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Writer.Write(b)
}

type operation struct {
	Protocol, Path, ValueHash, Local, Remote string
	Bytes                                    int
	Version, Suite                           uint16
	Reused                                   bool
}
type report struct {
	Nonce, Started, Finished, Device, Filter string
	Ports                                    map[string]int
	Operations                               []operation
	Frames, Bytes                            int
	Drops                                    int
	Proxy                                    map[string]int64
}

var result report

func note(protocol, path string, data []byte, local, remote string, state *tls.ConnectionState) {
	sum := sha256.Sum256(data)
	o := operation{Protocol: protocol, Path: path, Bytes: len(data), ValueHash: hex.EncodeToString(sum[:]), Local: local, Remote: remote}
	if state != nil {
		o.Version, o.Suite, o.Reused = state.Version, state.CipherSuite, state.DidResume
	}
	result.Operations = append(result.Operations, o)
}
func tcpListener() net.Listener { l, e := net.Listen("tcp", "127.0.0.1:0"); must(e); return l }
func udpSocket() *net.UDPConn {
	l, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(e)
	return l
}
func port(a net.Addr) int {
	_, p, e := net.SplitHostPort(a.String())
	must(e)
	n, e := strconv.Atoi(p)
	must(e)
	return n
}

type echoAPI interface{}

func bidi(_ interface{}, s grpc.ServerStream) error {
	for {
		v := &wrapperspb.StringValue{}
		if e := s.RecvMsg(v); e != nil {
			if e == io.EOF {
				return nil
			}
			return e
		}
		if e := s.SendMsg(wrapperspb.String(v.Value + "/reply")); e != nil {
			return e
		}
	}
}

func cert() (tls.Certificate, *x509.CertPool, []byte) {
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(e)
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	must(e)
	t := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "live-audit.invalid"}, DNSNames: []string{"live-audit.invalid"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, t, t, &k.PublicKey, k)
	must(e)
	key, e := x509.MarshalECPrivateKey(k)
	must(e)
	p := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	c, e := tls.X509KeyPair(p, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key}))
	must(e)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(p) {
		panic("certificate pool")
	}
	return c, roots, p
}

func dnsReply(q []byte) []byte {
	var m dnsmessage.Message
	must(m.Unpack(q))
	m.Header.Response = true
	m.Header.RecursionAvailable = true
	if strings.Contains(m.Questions[0].Name.String(), "missing") {
		m.Header.RCode = dnsmessage.RCodeNameError
	} else {
		m.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: m.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 30}, Body: &dnsmessage.AResource{A: [4]byte{192, 0, 2, 123}}}}
	}
	b, e := m.Pack()
	must(e)
	return b
}

type faultProxy struct {
	front, back               *net.UDPConn
	mu                        sync.Mutex
	client                    *net.UDPAddr
	packets, dropped, delayed atomic.Int64
	active                    atomic.Bool
}

func newProxy(target *net.UDPAddr) *faultProxy {
	back, e := net.DialUDP("udp", nil, target)
	must(e)
	p := &faultProxy{front: udpSocket(), back: back}
	forward := func(socket *net.UDPConn, target *net.UDPAddr, b []byte) {
		n := p.packets.Add(1)
		if p.active.Load() && n%13 == 0 {
			p.dropped.Add(1)
			return
		}
		send := func() {
			if target == nil {
				_, _ = socket.Write(b)
			} else {
				_, _ = socket.WriteToUDP(b, target)
			}
		}
		if p.active.Load() && n%7 == 0 {
			p.delayed.Add(1)
			time.AfterFunc(25*time.Millisecond, send)
		} else {
			send()
		}
	}
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := p.front.ReadFromUDP(b)
			if e != nil {
				return
			}
			p.mu.Lock()
			p.client = a
			p.mu.Unlock()
			forward(p.back, nil, bytes.Clone(b[:n]))
		}
	}()
	go func() {
		b := make([]byte, 65535)
		for {
			n, e := p.back.Read(b)
			if e != nil {
				return
			}
			p.mu.Lock()
			a := p.client
			p.mu.Unlock()
			if a != nil {
				forward(p.front, a, bytes.Clone(b[:n]))
			}
		}
	}()
	return p
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: live-audit OUTPUT_DIRECTORY")
	}
	out := os.Args[1]
	must(os.MkdirAll(out, 0700))
	var nonce [6]byte
	_, e := rand.Read(nonce[:])
	must(e)
	result = report{Nonce: hex.EncodeToString(nonce[:]), Started: time.Now().UTC().Format(time.RFC3339Nano), Device: `\Device\NPF_Loopback`, Ports: map[string]int{}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	keyfile, e := os.OpenFile(filepath.Join(out, "session.keys"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	defer keyfile.Close()
	keys := &lockedWriter{Writer: keyfile}
	certificate, roots, pemCert := cert()
	must(os.WriteFile(filepath.Join(out, "certificate.pem"), pemCert, 0600))
	serverConfig := func(version uint16) *tls.Config {
		return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: version, MaxVersion: version, NextProtos: []string{"http/1.1"}, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}}
	}
	clientConfig := func(version uint16) *tls.Config {
		return &tls.Config{RootCAs: roots, ServerName: "live-audit.invalid", MinVersion: version, MaxVersion: version, KeyLogWriter: keys, NextProtos: []string{"http/1.1"}, ClientSessionCache: tls.NewLRUClientSessionCache(8), CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}}
	}
	serveHTTP := func(w http.ResponseWriter, r *http.Request) {
		data, e := io.ReadAll(r.Body)
		must(e)
		w.Header().Set("X-Live-Audit", result.Nonce)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = fmt.Fprintf(w, "reply:%s:%s:", result.Nonce, r.URL.Path)
		_, _ = w.Write(data)
	}
	clear := tcpListener()
	result.Ports["http"] = port(clear.Addr())
	hs := &http.Server{Handler: http.HandlerFunc(serveHTTP)}
	go hs.Serve(clear)
	defer hs.Close()
	listeners := map[uint16]net.Listener{}
	for _, v := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		l := tcpListener()
		listeners[v] = l
		result.Ports[fmt.Sprintf("tls%d", v)] = port(l.Addr())
		s := &http.Server{Handler: http.HandlerFunc(serveHTTP)}
		go s.Serve(tls.NewListener(l, serverConfig(v)))
		defer s.Close()
	}
	grpcListener := tcpListener()
	result.Ports["grpc"] = port(grpcListener.Addr())
	gc := serverConfig(tls.VersionTLS13)
	gc.NextProtos = []string{"h2"}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(gc)))
	gs.RegisterService(&grpc.ServiceDesc{ServiceName: "live.Echo", HandlerType: (*echoAPI)(nil), Streams: []grpc.StreamDesc{{StreamName: "Bidi", Handler: bidi, ServerStreams: true, ClientStreams: true}}}, struct{}{})
	go gs.Serve(grpcListener)
	defer gs.Stop()
	dnsUDP, dnsTCP := udpSocket(), tcpListener()
	result.Ports["dns_udp"], result.Ports["dns_tcp"] = port(dnsUDP.LocalAddr()), port(dnsTCP.Addr())
	defer dnsUDP.Close()
	defer dnsTCP.Close()
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := dnsUDP.ReadFromUDP(b)
			if e != nil {
				return
			}
			_, _ = dnsUDP.WriteToUDP(dnsReply(bytes.Clone(b[:n])), a)
		}
	}()
	go func() {
		for {
			c, e := dnsTCP.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				for {
					var h [2]byte
					if _, e := io.ReadFull(c, h[:]); e != nil {
						return
					}
					b := make([]byte, binary.BigEndian.Uint16(h[:]))
					if _, e := io.ReadFull(c, b); e != nil {
						return
					}
					r := dnsReply(b)
					binary.BigEndian.PutUint16(h[:], uint16(len(r)))
					_, _ = c.Write(append(h[:], r...))
				}
			}()
		}
	}()
	mux := tcpListener()
	result.Ports["reuse_mux"] = port(mux.Addr())
	defer mux.Close()
	go func() {
		for {
			c, e := mux.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				b, e := br.Peek(1)
				if e != nil {
					return
				}
				if b[0] == 22 {
					tc := tls.Server(&prefixedConn{Conn: c, r: br}, serverConfig(tls.VersionTLS13))
					defer tc.Close()
					if tc.Handshake() != nil {
						return
					}
					req, e := http.ReadRequest(bufio.NewReader(tc))
					if e != nil {
						return
					}
					io.Copy(io.Discard, req.Body)
					body := "reuse-tls-" + result.Nonce
					fmt.Fprintf(tc, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
				} else {
					for i := 0; i < 3; i++ {
						line, e := br.ReadString('\n')
						if e != nil {
							return
						}
						if i == 2 && line != "PING\r\n" {
							return
						}
					}
					_, _ = c.Write([]byte("+PONG\r\n"))
				}
			}()
		}
	}()
	quicServerSocket, quicClientSocket := udpSocket(), udpSocket()
	result.Ports["quic_client"] = port(quicClientSocket.LocalAddr())
	result.Ports["quic_server"] = port(quicServerSocket.LocalAddr())
	h3cfg := serverConfig(tls.VersionTLS13)
	h3cfg.NextProtos = []string{"h3"}
	h3s := &http3.Server{TLSConfig: h3cfg, Handler: http.HandlerFunc(serveHTTP), QUICConfig: &quic.Config{MaxIdleTimeout: 10 * time.Second}}
	go h3s.Serve(quicServerSocket)
	defer h3s.Close()
	proxy := newProxy(quicServerSocket.LocalAddr().(*net.UDPAddr))
	defer proxy.front.Close()
	defer proxy.back.Close()
	result.Ports["quic_proxy"] = port(proxy.front.LocalAddr())
	qt := &quic.Transport{Conn: quicClientSocket}
	defer qt.Close()
	var terms []string
	for name, p := range result.Ports {
		if strings.HasPrefix(name, "quic_") {
			continue
		}
		if name == "dns_udp" {
			terms = append(terms, fmt.Sprintf("udp port %d", p))
		} else {
			terms = append(terms, fmt.Sprintf("tcp port %d", p))
		}
	}
	terms = append(terms, fmt.Sprintf("udp port %d", result.Ports["quic_client"]))
	result.Filter = "(" + strings.Join(terms, " or ") + ")"
	handle, e := pcap.OpenLive(result.Device, 65535, false, 100*time.Millisecond)
	must(e)
	defer handle.Close()
	must(handle.SetBPFFilter(result.Filter))
	pcapFile, e := os.OpenFile(filepath.Join(out, "live.pcap"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	defer pcapFile.Close()
	pw := pcapgo.NewWriterNanos(pcapFile)
	must(pw.WriteFileHeader(65535, handle.LinkType()))
	var stopped atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !stopped.Load() {
			b, ci, e := handle.ReadPacketData()
			if e == pcap.NextErrorTimeoutExpired {
				continue
			}
			must(e)
			must(pw.WritePacket(ci, b))
			result.Frames++
			result.Bytes += len(b)
		}
	}()
	time.Sleep(150 * time.Millisecond)
	for i := 0; i < 3; i++ {
		body := "GET /not-a-protocol\r\nSSH-2.0-body\r\n" + result.Nonce
		resp, e := http.Post("http://"+clear.Addr().String()+fmt.Sprintf("/plain/%d/%s", i, result.Nonce), "text/plain", strings.NewReader(body))
		must(e)
		data, e := io.ReadAll(resp.Body)
		must(e)
		resp.Body.Close()
		note("http", resp.Request.URL.Path, data, "", clear.Addr().String(), nil)
	}
	for _, v := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		cfg := clientConfig(v)
		transport := &http.Transport{TLSClientConfig: cfg, MaxConnsPerHost: 1}
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		for generation := 0; generation < 2; generation++ {
			for i := 0; i < 3; i++ {
				body := strings.Repeat("payload-"+result.Nonce+"/", i*1300+1)
				path := fmt.Sprintf("/tls/%x/%d/%d/%s", v, generation, i, result.Nonce)
				resp, e := client.Post("https://"+listeners[v].Addr().String()+path, "application/octet-stream", strings.NewReader(body))
				must(e)
				data, e := io.ReadAll(resp.Body)
				must(e)
				resp.Body.Close()
				note("tls-http1", path, data, "", listeners[v].Addr().String(), resp.TLS)
			}
			transport.CloseIdleConnections()
		}
	}
	cc, e := grpc.DialContext(ctx, grpcListener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(func() *tls.Config { c := clientConfig(tls.VersionTLS13); c.NextProtos = []string{"h2"}; return c }())), grpc.WithBlock())
	must(e)
	stream, e := cc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/live.Echo/Bidi")
	must(e)
	for i, n := range []int{1, 17, 4096, 32768, 70000} {
		value := wrapperspb.String(fmt.Sprintf("live-%s-%d-", result.Nonce, i) + strings.Repeat("q", n))
		sent, e := proto.Marshal(value)
		must(e)
		must(stream.SendMsg(value))
		reply := &wrapperspb.StringValue{}
		must(stream.RecvMsg(reply))
		if reply.Value != value.Value+"/reply" {
			panic("gRPC reply")
		}
		data, e := proto.Marshal(reply)
		must(e)
		note("grpc-request", "/live.Echo/Bidi", sent, "", grpcListener.Addr().String(), nil)
		note("grpc-response", "/live.Echo/Bidi", data, "", grpcListener.Addr().String(), nil)
	}
	must(stream.CloseSend())
	final := &wrapperspb.StringValue{}
	if e := stream.RecvMsg(final); e != io.EOF {
		panic(fmt.Sprint("gRPC close ", e))
	}
	cc.Close()
	for _, network := range []string{"udp", "tcp"} {
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("host-%d.%s.audit.invalid.", i, result.Nonce)
			if i == 2 {
				name = "missing." + name
			}
			nm, e := dnsmessage.NewName(name)
			must(e)
			q, e := (&dnsmessage.Message{Header: dnsmessage.Header{ID: uint16(0x7000 + i), RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: nm, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}).Pack()
			must(e)
			addr := dnsUDP.LocalAddr().String()
			if network == "tcp" {
				addr = dnsTCP.Addr().String()
			}
			c, e := net.DialTimeout(network, addr, 2*time.Second)
			must(e)
			c.SetDeadline(time.Now().Add(2 * time.Second))
			var answer []byte
			if network == "udp" {
				_, e = c.Write(q)
				must(e)
				b := make([]byte, 4096)
				n, e := c.Read(b)
				must(e)
				answer = b[:n]
			} else {
				h := make([]byte, 2)
				binary.BigEndian.PutUint16(h, uint16(len(q)))
				_, e = c.Write(append(h, q...))
				must(e)
				_, e = io.ReadFull(c, h)
				must(e)
				answer = make([]byte, binary.BigEndian.Uint16(h))
				_, e = io.ReadFull(c, answer)
				must(e)
			}
			note("dns-"+network, name, answer, c.LocalAddr().String(), c.RemoteAddr().String(), nil)
			c.Close()
		}
	}
	for generation := 0; generation < 3; generation++ {
		proxy.active.Store(generation == 1)
		h3client := &http3.Transport{TLSClientConfig: func() *tls.Config { c := clientConfig(tls.VersionTLS13); c.NextProtos = []string{"h3"}; return c }(), QUICConfig: &quic.Config{MaxIdleTimeout: 10 * time.Second}, Dial: func(ctx context.Context, _ string, c *tls.Config, q *quic.Config) (quic.EarlyConnection, error) {
			return qt.DialEarly(ctx, proxy.front.LocalAddr(), c, q)
		}}
		client := &http.Client{Transport: h3client, Timeout: 12 * time.Second}
		for i := 0; i < 4; i++ {
			path := fmt.Sprintf("/h3/%d/%d/%s", generation, i, result.Nonce)
			body := strings.Repeat("h3-"+result.Nonce+"/", i*180+1)
			resp, e := client.Post("https://"+quicServerSocket.LocalAddr().String()+path, "application/octet-stream", strings.NewReader(body))
			must(e)
			data, e := io.ReadAll(resp.Body)
			must(e)
			resp.Body.Close()
			if !bytes.HasSuffix(data, []byte(body)) {
				panic("H3 response")
			}
			note("http3", path, data, quicClientSocket.LocalAddr().String(), proxy.front.LocalAddr().String(), resp.TLS)
		}
		must(h3client.Close())
		time.Sleep(70 * time.Millisecond)
	}
	// The same real Windows TCP tuple is reused after an abortive close, first
	// for TLS and then for plaintext Redis. No pcap frames are fabricated.
	probe, e := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(e)
	reusePort := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	result.Ports["reuse_client"] = reusePort
	for generation := 0; generation < 2; generation++ {
		dialer := net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: reusePort}, Control: func(_, _ string, c syscall.RawConn) error {
			var err error
			if e := c.Control(func(fd uintptr) {
				err = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
			}); e != nil {
				return e
			}
			return err
		}}
		raw, e := dialer.DialContext(ctx, "tcp", mux.Addr().String())
		must(e)
		raw.SetDeadline(time.Now().Add(3 * time.Second))
		if generation == 0 {
			tc := tls.Client(raw, clientConfig(tls.VersionTLS13))
			must(tc.HandshakeContext(ctx))
			path := "/reuse/tls/" + result.Nonce
			_, e = fmt.Fprintf(tc, "GET %s HTTP/1.1\r\nHost: live-audit.invalid\r\n\r\n", path)
			must(e)
			resp, e := http.ReadResponse(bufio.NewReader(tc), nil)
			must(e)
			data, e := io.ReadAll(resp.Body)
			must(e)
			resp.Body.Close()
			state := tc.ConnectionState()
			note("tuple-reuse-tls", path, data, raw.LocalAddr().String(), raw.RemoteAddr().String(), &state)
		} else {
			_, e = raw.Write([]byte("*1\r\n$4\r\nPING\r\n"))
			must(e)
			data, e := bufio.NewReader(raw).ReadBytes('\n')
			must(e)
			if string(data) != "+PONG\r\n" {
				panic("Redis response")
			}
			note("tuple-reuse-redis", "PING", data, raw.LocalAddr().String(), raw.RemoteAddr().String(), nil)
		}
		must(raw.(*net.TCPConn).SetLinger(0))
		raw.Close()
		time.Sleep(80 * time.Millisecond)
	}
	time.Sleep(250 * time.Millisecond)
	stopped.Store(true)
	<-done
	stats, e := handle.Stats()
	must(e)
	result.Drops = stats.PacketsDropped
	result.Proxy = map[string]int64{"packets": proxy.packets.Load(), "dropped": proxy.dropped.Load(), "delayed": proxy.delayed.Load()}
	result.Finished = time.Now().UTC().Format(time.RFC3339Nano)
	data, e := json.MarshalIndent(result, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "manifest.json"), data, 0600))
	fmt.Printf("captured %d frames, %d bytes; %d operations; proxy drop=%d delay=%d; kernel drops=%d\n", result.Frames, result.Bytes, len(result.Operations), proxy.dropped.Load(), proxy.delayed.Load(), result.Drops)
}

type prefixedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *prefixedConn) Read(b []byte) (int, error) { return c.r.Read(b) }
