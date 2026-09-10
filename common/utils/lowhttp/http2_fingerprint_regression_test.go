package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// Use the production connection initializer so advertised settings and local
// receive limits cannot drift unnoticed. Capture writes without a socket reader.
func newH2FingerprintRegressionConn(t *testing.T, profile string, output io.Writer) *http2ClientConn {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	pc := &persistConn{
		conn:     client,
		p:        &LowHttpConnPool{ctx: context.Background()},
		cacheKey: &connectKey{http2Fingerprint: profile},
	}
	pc.h2Conn()
	c := pc.alt
	c.pc = nil
	c.bw = bufio.NewWriter(output)
	c.fr = http2.NewFramer(c.bw, nil)
	return c
}

func captureH2FingerprintSettings(t *testing.T, c *http2ClientConn, wire *bytes.Buffer) map[http2.SettingID]uint32 {
	t.Helper()
	if err := c.preface(); err != nil {
		t.Fatal(err)
	}
	if got := string(wire.Next(len(http2.ClientPreface))); got != http2.ClientPreface {
		t.Fatalf("invalid client preface: %q", got)
	}
	settings := map[http2.SettingID]uint32{
		http2.SettingHeaderTableSize:   4096,
		http2.SettingInitialWindowSize: 65535,
	}
	fr := http2.NewFramer(io.Discard, wire)
	for {
		frame, err := fr.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if sf, ok := frame.(*http2.SettingsFrame); ok {
			if err := sf.ForeachSetting(func(s http2.Setting) error {
				settings[s.ID] = s.Val
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return settings
}

func TestH2FingerprintAdvertisedHPACKTable(t *testing.T) {
	for _, profile := range []string{"", HTTP2FingerprintChrome151} {
		t.Run(profile, func(t *testing.T) {
			var wire, block bytes.Buffer
			c := newH2FingerprintRegressionConn(t, profile, &wire)
			size := captureH2FingerprintSettings(t, c, &wire)[http2.SettingHeaderTableSize]
			encoder := hpack.NewEncoder(&block)
			encoder.SetMaxDynamicTableSizeLimit(size)
			encoder.SetMaxDynamicTableSize(size)
			// Keep more than 4 KiB of entries in the Chrome table, then reuse
			// them in the next response to exercise connection-level state.
			fields := []hpack.HeaderField{{Name: ":status", Value: "200"}}
			for i := 0; i < 80; i++ {
				fields = append(fields, hpack.HeaderField{Name: "x-shared", Value: strings.Repeat("x", 60) + string(rune('A'+i))})
			}
			for response := 0; response < 2; response++ {
				block.Reset()
				for _, field := range fields {
					if err := encoder.WriteField(field); err != nil {
						t.Fatal(err)
					}
				}
				got, err := c.hDec.DecodeFull(block.Bytes())
				if err != nil {
					t.Fatalf("response %d using advertised table size %d rejected: %v", response, size, err)
				}
				if len(got) != len(fields) {
					t.Fatalf("decoded %d fields, want %d", len(got), len(fields))
				}
				for i := range fields {
					if got[i] != fields[i] {
						t.Fatalf("field %d = %+v, want %+v", i, got[i], fields[i])
					}
				}
			}
			block.Reset()
			encoder.SetMaxDynamicTableSizeLimit(size + 1)
			encoder.SetMaxDynamicTableSize(size + 1)
			if err := encoder.WriteField(fields[0]); err != nil {
				t.Fatal(err)
			}
			if _, err := c.hDec.DecodeFull(block.Bytes()); err == nil {
				t.Fatal("accepted table size above the advertised limit")
			}
		})
	}
}

func TestH2FingerprintLargeRequestHeaders(t *testing.T) {
	for _, profile := range []string{"", HTTP2FingerprintChrome151} {
		t.Run(profile, func(t *testing.T) {
			var wire bytes.Buffer
			c := newH2FingerprintRegressionConn(t, profile, &wire)
			req, _ := http.NewRequest("GET", "http://h2.test/", nil)
			cookie := strings.Repeat("abcdefgh", 5000)
			packet := []byte("GET / HTTP/1.1\r\nHost: h2.test\r\nCookie: " + cookie + "\r\n\r\n")
			cs, err := c.newStream(req, packet, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := cs.doRequest(); err != nil {
				t.Fatal(err)
			}
			fr := http2.NewFramer(io.Discard, &wire)
			fr.SetMaxReadFrameSize(defaultMaxFrameSize)
			var block bytes.Buffer
			var continuations int
			var ended bool
			for {
				frame, err := fr.ReadFrame()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("peer rejected request framing: %v", err)
				}
				switch f := frame.(type) {
				case *http2.HeadersFrame:
					block.Write(f.HeaderBlockFragment())
					ended = f.HeadersEnded()
					if profile != "" && (!f.HasPriority() || !f.StreamEnded()) {
						t.Fatal("Chrome HEADERS lost priority or END_STREAM")
					}
				case *http2.ContinuationFrame:
					continuations++
					block.Write(f.HeaderBlockFragment())
					ended = f.HeadersEnded()
				}
			}
			if continuations == 0 || !ended {
				t.Fatal("large header block did not finish through CONTINUATION")
			}
			fields, err := hpack.NewDecoder(4096, nil).DecodeFull(block.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range fields {
				if field.Name == "cookie" && field.Value == cookie {
					return
				}
			}
			t.Fatal("fragmentation lost request cookie")
		})
	}
}

func TestH2FingerprintStreamingReceiveWindow(t *testing.T) {
	for _, profile := range []string{"", HTTP2FingerprintChrome151} {
		t.Run(profile, func(t *testing.T) {
			var wire bytes.Buffer
			c := newH2FingerprintRegressionConn(t, profile, &wire)
			window := int(captureH2FingerprintSettings(t, c, &wire)[http2.SettingInitialWindowSize])
			req, _ := http.NewRequest("GET", "http://h2.test/", nil)
			cs, err := c.newStream(req, nil, &LowhttpExecConfig{
				NoBodyBuffer:            true,
				BodyStreamReaderHandler: func([]byte, io.ReadCloser) {},
			})
			if err != nil {
				t.Fatal(err)
			}
			// Leave the consumer paused. Use the production-created body pipe,
			// but do not start its handler until after receiving the test data.
			cs.ID = 1
			c.streams[1] = cs
			c.currentStreamID = 3
			cs.readHeaderEnd = true
			cs.resp.StatusCode = 200
			t.Cleanup(func() { cs.abort() })
			rl := &http2ClientConnReadLoop{h2Conn: c}
			payload := bytes.Repeat([]byte("x"), defaultMaxFrameSize)
			for received := 0; received < window; received += len(payload) {
				rl.processData(readDataFrameForCanceledStreamTest(t, 1, payload, nil))
				if cs.streamErr != nil {
					t.Fatalf("legal response failed after %d of %d advertised bytes: %v", received+len(payload), window, cs.streamErr)
				}
			}
			fr := http2.NewFramer(io.Discard, &wire)
			var returned uint32
			for {
				frame, err := fr.ReadFrame()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				update, ok := frame.(*http2.WindowUpdateFrame)
				if !ok || update.StreamID != 0 {
					t.Fatalf("unconsumed body returned stream credit or failed: %v", frame)
				}
				returned += update.Increment
			}
			if returned != uint32(window) {
				t.Fatalf("returned connection credit %d, want %d", returned, window)
			}
			// The queue must remain bounded even after increasing the profile limit.
			if _, err := cs.bodyStreamWriter.Write([]byte{1}); !errors.Is(err, errH2BodyWindowExceeded) {
				t.Fatalf("pipe accepted data beyond advertised window: %v", err)
			}
			consumed := make([]byte, 32<<10)
			if _, err := io.ReadFull(cs.bodyStreamReader, consumed); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(consumed, bytes.Repeat([]byte("x"), len(consumed))) {
				t.Fatal("streaming body corrupted")
			}
			c.consumeStreamBytes(1, len(consumed))
			frame, err := fr.ReadFrame()
			if err != nil {
				t.Fatal(err)
			}
			update, ok := frame.(*http2.WindowUpdateFrame)
			if !ok || update.StreamID != 1 || update.Increment != uint32(len(consumed)) {
				t.Fatalf("consumption did not return stream credit: %v", frame)
			}
			for i := 0; i < len(consumed)/len(payload); i++ {
				rl.processData(readDataFrameForCanceledStreamTest(t, 1, payload, nil))
			}
			if cs.streamErr != nil || cs.recvWindow != 0 {
				t.Fatalf("refilling returned credit failed: window=%d, err=%v", cs.recvWindow, cs.streamErr)
			}
		})
	}
}
