package lowhttp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const chromeAkamaiFingerprint = "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p"

// h2ClientProfile 是从客户端首批帧里还原出来的 akamai 指纹素材
type h2ClientProfile struct {
	settings          []http2.Setting
	windowUpdate      uint32
	pseudoHeaderOrder []string
	headersEndStream  bool
	headersPriority   http2.PriorityParam
	sawEmptyDataFrame bool
}

func (p *h2ClientProfile) akamai() string {
	parts := make([]string, 0, len(p.settings))
	for _, s := range p.settings {
		parts = append(parts, fmt.Sprintf("%d:%d", s.ID, s.Val))
	}
	order := make([]string, 0, len(p.pseudoHeaderOrder))
	for _, name := range p.pseudoHeaderOrder {
		order = append(order, strings.TrimPrefix(name, ":")[:1])
	}
	return fmt.Sprintf("%s|%d|0|%s", strings.Join(parts, ";"), p.windowUpdate, strings.Join(order, ","))
}

func selfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
	}
}

// serveH2AndProfile 起一个只读客户端帧的 h2 服务端，返回还原出的客户端指纹
func serveH2AndProfile(t *testing.T) (addr string, profile <-chan *h2ClientProfile) {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", selfSignedTLSConfig(t))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	result := make(chan *h2ClientProfile, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(15 * time.Second))

		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(conn, preface); err != nil {
			return
		}

		p := &h2ClientProfile{}
		fr := http2.NewFramer(conn, conn)
		dec := hpack.NewDecoder(defaultHeaderTableSize, func(f hpack.HeaderField) {
			if strings.HasPrefix(f.Name, ":") {
				p.pseudoHeaderOrder = append(p.pseudoHeaderOrder, f.Name)
			}
		})
		fr.WriteSettings()

		for {
			frame, err := fr.ReadFrame()
			if err != nil {
				break
			}
			switch f := frame.(type) {
			case *http2.SettingsFrame:
				if f.IsAck() {
					continue
				}
				f.ForeachSetting(func(s http2.Setting) error {
					p.settings = append(p.settings, s)
					return nil
				})
				fr.WriteSettingsAck()
			case *http2.WindowUpdateFrame:
				if f.StreamID == 0 && p.windowUpdate == 0 {
					p.windowUpdate = f.Increment
				}
			case *http2.HeadersFrame:
				dec.Write(f.HeaderBlockFragment())
				p.headersEndStream = f.StreamEnded()
				p.headersPriority = f.Priority
				if f.StreamEnded() {
					writeEmptyH2Response(fr, f.StreamID)
					result <- p
					return
				}
			case *http2.DataFrame:
				if len(f.Data()) == 0 && f.StreamEnded() {
					p.sawEmptyDataFrame = true
					writeEmptyH2Response(fr, f.StreamID)
					result <- p
					return
				}
			}
		}
	}()
	return ln.Addr().String(), result
}

func writeEmptyH2Response(fr *http2.Framer, streamID uint32) {
	var buf []byte
	enc := hpack.NewEncoder(newByteSliceWriter(&buf))
	enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: buf,
		EndHeaders:    true,
		EndStream:     true,
	})
}

type byteSliceWriter struct{ buf *[]byte }

func newByteSliceWriter(buf *[]byte) *byteSliceWriter { return &byteSliceWriter{buf: buf} }

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func requestH2Profile(t *testing.T, chromeFingerprint bool) *h2ClientProfile {
	t.Helper()
	addr, profileCh := serveH2AndProfile(t)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	packet := []byte("GET /api/all HTTP/1.1\r\nHost: " + addr + "\r\nUser-Agent: test\r\n\r\n")
	opts := []LowhttpOpt{
		WithHost(host),
		WithPort(utils.InterfaceToInt(port)),
		WithHttps(true),
		WithHttp2(true),
		WithPacketBytes(packet),
		WithTimeout(15 * time.Second),
	}
	if chromeFingerprint {
		opts = append(opts, WithRandomJA3FingerPrint(true))
	}
	go HTTPWithoutRedirect(opts...)

	select {
	case p := <-profileCh:
		return p
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for client frames")
		return nil
	}
}

func TestChromeH2Fingerprint_Akamai(t *testing.T) {
	p := requestH2Profile(t, true)
	if got := p.akamai(); got != chromeAkamaiFingerprint {
		t.Fatalf("akamai fingerprint mismatch\n want %s\n  got %s", chromeAkamaiFingerprint, got)
	}
}

func TestChromeH2Fingerprint_HeadersFrame(t *testing.T) {
	p := requestH2Profile(t, true)
	if !p.headersEndStream {
		t.Error("HEADERS 帧应带 END_STREAM")
	}
	if p.sawEmptyDataFrame {
		t.Error("无 body 时不应补发空 DATA 帧")
	}
	if !p.headersPriority.Exclusive || p.headersPriority.StreamDep != 0 || p.headersPriority.Weight != chromeH2HeadersWeight-1 {
		t.Errorf("HEADERS priority = %+v, want exclusive dep=0 weight=%d", p.headersPriority, chromeH2HeadersWeight-1)
	}
}

// 未开启指纹时保持原有行为不变
func TestDefaultH2Fingerprint_Unchanged(t *testing.T) {
	p := requestH2Profile(t, false)
	if p.headersEndStream {
		t.Error("默认行为不应在 HEADERS 上带 END_STREAM")
	}
	if !p.sawEmptyDataFrame {
		t.Error("默认行为应补发空 DATA 帧结束流")
	}
	if !p.headersPriority.IsZero() {
		t.Errorf("默认行为不应带 PRIORITY, got %+v", p.headersPriority)
	}
	if got := p.akamai(); got == chromeAkamaiFingerprint {
		t.Error("默认行为不应产生 Chrome 指纹")
	}
}
