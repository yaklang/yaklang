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

type h2ClientProfile struct {
	settings          []http2.Setting
	windowUpdate      uint32
	pseudoHeaderOrder []string
	headersEndStream  bool
	headersPriority   http2.PriorityParam
	sawEmptyDataFrame bool
}

func (p *h2ClientProfile) akamai() string {
	settings := make([]string, 0, len(p.settings))
	for _, setting := range p.settings {
		settings = append(settings, fmt.Sprintf("%d:%d", setting.ID, setting.Val))
	}
	order := make([]string, 0, len(p.pseudoHeaderOrder))
	for _, name := range p.pseudoHeaderOrder {
		order = append(order, strings.TrimPrefix(name, ":")[:1])
	}
	return fmt.Sprintf("%s|%d|0|%s", strings.Join(settings, ";"), p.windowUpdate, strings.Join(order, ","))
}

func chromeFingerprintTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
	}
}

func serveH2Fingerprint(t *testing.T) (string, <-chan *h2ClientProfile) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", chromeFingerprintTLSConfig(t))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	result := make(chan *h2ClientProfile, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(conn, preface); err != nil {
			return
		}
		profile := &h2ClientProfile{}
		framer := http2.NewFramer(conn, conn)
		decoder := hpack.NewDecoder(defaultHeaderTableSize, func(field hpack.HeaderField) {
			if strings.HasPrefix(field.Name, ":") {
				profile.pseudoHeaderOrder = append(profile.pseudoHeaderOrder, field.Name)
			}
		})
		_ = framer.WriteSettings()

		for {
			frame, err := framer.ReadFrame()
			if err != nil {
				return
			}
			switch current := frame.(type) {
			case *http2.SettingsFrame:
				if current.IsAck() {
					continue
				}
				_ = current.ForeachSetting(func(setting http2.Setting) error {
					profile.settings = append(profile.settings, setting)
					return nil
				})
				_ = framer.WriteSettingsAck()
			case *http2.WindowUpdateFrame:
				if current.StreamID == 0 && profile.windowUpdate == 0 {
					profile.windowUpdate = current.Increment
				}
			case *http2.HeadersFrame:
				_, _ = decoder.Write(current.HeaderBlockFragment())
				profile.headersEndStream = current.StreamEnded()
				profile.headersPriority = current.Priority
				if current.StreamEnded() {
					writeFingerprintResponse(framer, current.StreamID)
					result <- profile
					return
				}
			case *http2.DataFrame:
				if len(current.Data()) == 0 && current.StreamEnded() {
					profile.sawEmptyDataFrame = true
					writeFingerprintResponse(framer, current.StreamID)
					result <- profile
					return
				}
			}
		}
	}()
	return listener.Addr().String(), result
}

func writeFingerprintResponse(framer *http2.Framer, streamID uint32) {
	var block []byte
	encoder := hpack.NewEncoder(&byteSliceAppender{target: &block})
	_ = encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndHeaders:    true,
		EndStream:     true,
	})
}

type byteSliceAppender struct {
	target *[]byte
}

func (w *byteSliceAppender) Write(p []byte) (int, error) {
	*w.target = append(*w.target, p...)
	return len(p), nil
}

func requestH2Fingerprint(t *testing.T, chrome bool) *h2ClientProfile {
	t.Helper()
	address, profileChannel := serveH2Fingerprint(t)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	packet := []byte("GET /api/all HTTP/1.1\r\nHost: " + address + "\r\nUser-Agent: test\r\n\r\n")
	options := []LowhttpOpt{
		WithHost(host),
		WithPort(utils.InterfaceToInt(port)),
		WithHttps(true),
		WithHttp2(true),
		WithPacketBytes(packet),
		WithTimeout(15 * time.Second),
	}
	if chrome {
		options = append(options, WithTLSFingerprint("chrome-151"))
	}
	go func() {
		_, _ = HTTPWithoutRedirect(options...)
	}()

	select {
	case profile := <-profileChannel:
		return profile
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for HTTP/2 client frames")
		return nil
	}
}

func TestChromeH2Fingerprint(t *testing.T) {
	profile := requestH2Fingerprint(t, true)
	if got := profile.akamai(); got != chromeAkamaiFingerprint {
		t.Fatalf("Akamai fingerprint mismatch\n want %s\n  got %s", chromeAkamaiFingerprint, got)
	}
	if !profile.headersEndStream {
		t.Error("Chrome HEADERS frame must carry END_STREAM")
	}
	if profile.sawEmptyDataFrame {
		t.Error("Chrome request without a body must not send an empty DATA frame")
	}
	if !profile.headersPriority.Exclusive || profile.headersPriority.StreamDep != 0 || profile.headersPriority.Weight != chromeH2HeadersWeight-1 {
		t.Errorf("HEADERS priority = %+v", profile.headersPriority)
	}
}

func TestDefaultH2FingerprintUnchanged(t *testing.T) {
	profile := requestH2Fingerprint(t, false)
	if profile.headersEndStream {
		t.Error("default behavior unexpectedly set END_STREAM on HEADERS")
	}
	if !profile.sawEmptyDataFrame {
		t.Error("default behavior must retain the empty DATA frame")
	}
	if !profile.headersPriority.IsZero() {
		t.Errorf("default behavior unexpectedly set priority: %+v", profile.headersPriority)
	}
	if got := profile.akamai(); got == chromeAkamaiFingerprint {
		t.Error("default behavior unexpectedly produced the Chrome fingerprint")
	}
}
