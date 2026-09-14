package lowhttp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
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

func h2FingerprintTLSConfig() *tls.Config {
	config := *utils.GetDefaultTLSConfig(5)
	config.NextProtos = []string{"h2"}
	return &config
}

func serveH2Fingerprint(t *testing.T) (string, <-chan *h2ClientProfile) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", h2FingerprintTLSConfig())
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
					writeH2FingerprintResponse(framer, current.StreamID)
					result <- profile
					return
				}
			case *http2.DataFrame:
				if len(current.Data()) == 0 && current.StreamEnded() {
					profile.sawEmptyDataFrame = true
					writeH2FingerprintResponse(framer, current.StreamID)
					result <- profile
					return
				}
			}
		}
	}()
	return listener.Addr().String(), result
}

func writeH2FingerprintResponse(framer *http2.Framer, streamID uint32) {
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	_ = encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	})
}

func requestH2Fingerprint(t *testing.T, tlsFingerprint bool, extra ...LowhttpOpt) *h2ClientProfile {
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
	if tlsFingerprint {
		options = append(options, WithTLSFingerprint("chrome-151"))
	}
	options = append(options, extra...)
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

func TestTLSFingerprintLeavesH2BehaviorUnchanged(t *testing.T) {
	native := requestH2Fingerprint(t, false)
	withTLSFingerprint := requestH2Fingerprint(t, true)

	if got, want := withTLSFingerprint.akamai(), native.akamai(); got != want {
		t.Fatalf("TLS fingerprint changed the HTTP/2 profile\n want %s\n  got %s", want, got)
	}
	if withTLSFingerprint.headersEndStream != native.headersEndStream ||
		withTLSFingerprint.headersPriority != native.headersPriority ||
		withTLSFingerprint.sawEmptyDataFrame != native.sawEmptyDataFrame {
		t.Fatalf("TLS fingerprint changed HTTP/2 frame behavior: native=%+v fingerprint=%+v", native, withTLSFingerprint)
	}
}

func TestDefaultH2CompatibilityBehavior(t *testing.T) {
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
}

func TestChromeH2Fingerprint(t *testing.T) {
	// Chrome framing is opt-in via WithHTTP2Fingerprint, so servers that reject
	// browser-style HEADERS keep the default framing untouched. See
	// TestDefaultH2CompatibilityBehavior for that guarantee.
	profile := requestH2Fingerprint(t, true, WithHTTP2Fingerprint(HTTP2FingerprintChrome151))
	if got := profile.akamai(); got != chromeAkamaiFingerprint {
		t.Fatalf("Akamai fingerprint mismatch\n want %s\n  got %s", chromeAkamaiFingerprint, got)
	}
	if !profile.headersEndStream || profile.sawEmptyDataFrame {
		t.Errorf("Chrome empty-request framing mismatch: %+v", profile)
	}
	if !profile.headersPriority.Exclusive || profile.headersPriority.StreamDep != 0 || profile.headersPriority.Weight != 255 {
		t.Errorf("HEADERS priority = %+v", profile.headersPriority)
	}
}

func TestHTTP2FingerprintIsolatesConnectionPool(t *testing.T) {
	base := &connectKey{scheme: "https", addr: "example.com:443", https: true}
	withProfile := *base
	withProfile.http2Fingerprint = HTTP2FingerprintChrome151

	if base.hash() == withProfile.hash() {
		t.Fatal("connectKey.hash() ignores http2Fingerprint: connections with different HTTP/2 framing would be reused for each other")
	}
}

func TestUnknownHTTP2FingerprintIsRejected(t *testing.T) {
	if err := ValidateHTTP2Profile(""); err != nil {
		t.Errorf("empty profile name must mean default framing, got %v", err)
	}
	if err := ValidateHTTP2Profile(HTTP2FingerprintChrome151); err != nil {
		t.Errorf("built-in profile must validate, got %v", err)
	}
	if err := ValidateHTTP2Profile("chrome-999"); err == nil {
		t.Error("unknown profile name must be rejected")
	}
}
