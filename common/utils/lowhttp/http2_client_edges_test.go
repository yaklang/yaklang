package lowhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type h2FrameCapture func(http2.Frame)

func (f h2FrameCapture) Write(p []byte) (int, error) {
	fr := http2.NewFramer(io.Discard, bytes.NewReader(p))
	frame, err := fr.ReadFrame()
	if err != nil {
		return 0, err
	}
	f(frame)
	return len(p), nil
}

func TestH2UploadSmallWindowAndCancellation(t *testing.T) {
	for _, cancelUpload := range []bool{false, true} {
		name := "exact_credit"
		if cancelUpload {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			data := make(chan int, 10)
			c := newH2ReadLoopTestConn(t, h2FrameCapture(func(f http2.Frame) {
				if d, ok := f.(*http2.DataFrame); ok {
					data <- len(d.Data())
				}
			}))
			c.initialWindowSize, c.currentStreamID = 3, 1
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", "http://h2.test/", nil)
			cs, err := c.newStream(req, []byte("POST / HTTP/2\r\nHost: h2.test\r\nContent-Length: 8\r\n\r\n12345678"), nil)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cs.doRequest() }()
			select {
			case n := <-data:
				if n != 3 {
					t.Fatalf("first DATA = %d", n)
				}
			case <-time.After(time.Second):
				t.Fatal("small window was not used")
			}
			if cancelUpload {
				cancel()
			} else {
				rl := &http2ClientConnReadLoop{h2Conn: c}
				rl.processWindowUpdate(&http2.WindowUpdateFrame{FrameHeader: http2.FrameHeader{StreamID: 1}, Increment: 5})
				select {
				case n := <-data:
					if n != 5 {
						t.Fatalf("last DATA = %d", n)
					}
				case <-time.After(time.Second):
					t.Fatal("exact window credit deadlocked")
				}
			}
			select {
			case err := <-done:
				if cancelUpload && !errors.Is(err, context.Canceled) || !cancelUpload && err != nil {
					t.Fatalf("upload error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("upload did not stop")
			}
			cs.abort()
			if len(c.streams) != 0 || c.activeStreams != 0 {
				t.Fatal("upload retained its stream slot")
			}
		})
	}
}

func h2SettingsFrame(t *testing.T, settings ...http2.Setting) *http2.SettingsFrame {
	t.Helper()
	var wire bytes.Buffer
	w := http2.NewFramer(&wire, nil)
	if err := w.WriteSettings(settings...); err != nil {
		t.Fatal(err)
	}
	f, err := http2.NewFramer(io.Discard, &wire).ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	return f.(*http2.SettingsFrame)
}

func TestH2SettingsWakeStreamWaiters(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	c.maxStreamsCount = 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://h2.test/", nil)
	done := make(chan error, 1)
	go func() {
		cs, err := c.newStream(req, nil, nil)
		if err == nil {
			cs.abort()
		}
		done <- err
	}()
	rl := &http2ClientConnReadLoop{h2Conn: c}
	rl.processSettings(h2SettingsFrame(t, http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1}))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestH2PeerSettingsDoNotChangeResponseDecoder(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	rl := &http2ClientConnReadLoop{h2Conn: c}
	var buf bytes.Buffer
	encoder := hpack.NewEncoder(&buf)
	shared := hpack.HeaderField{Name: "x-shared", Value: "keep-decoder-table"}
	block := encodeHPACKBlockForCanceledStreamTest(t, encoder, &buf, hpack.HeaderField{Name: ":status", Value: "200"}, shared)
	rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block, true, true))
	rl.processSettings(h2SettingsFrame(t, http2.Setting{ID: http2.SettingHeaderTableSize, Val: 0}))
	cs := newH2ReadLoopTestStream(t, c, 3)
	block = encodeHPACKBlockForCanceledStreamTest(t, encoder, &buf, hpack.HeaderField{Name: ":status", Value: "200"}, shared)
	rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 3, block, true, true))
	if c.isClosed() || cs.resp.Header.Get(shared.Name) != shared.Value {
		t.Fatal("peer encoder setting destroyed decoder state")
	}
}

func TestH2ResponseHeaderSemantics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []hpack.HeaderField
		end    bool
		bad    bool
	}{
		{"missing_status", nil, true, true},
		{"bad_status", []hpack.HeaderField{{Name: ":status", Value: "20"}}, true, true},
		{"duplicate_status", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: ":status", Value: "200"}}, true, true},
		{"uppercase", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "Bad", Value: "x"}}, true, true},
		{"newline", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "x-bad", Value: "\r\ninjected"}}, true, true},
		{"whitespace", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "x-bad", Value: " padded "}}, true, true},
		{"signed_length", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-length", Value: "+0"}}, true, true},
		{"connection_specific", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "connection", Value: "close"}}, true, true},
		{"conflicting_lengths", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-length", Value: "1"}, {Name: "content-length", Value: "2"}}, true, true},
		{"truncated_body", []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-length", Value: "10"}}, true, true},
		{"headless_no_content", []hpack.HeaderField{{Name: ":status", Value: "204"}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newH2ReadLoopTestConn(t, io.Discard)
			cs := newH2ReadLoopTestStream(t, c, 1)
			rl := &http2ClientConnReadLoop{h2Conn: c}
			var buf bytes.Buffer
			block := encodeHPACKBlockForCanceledStreamTest(t, hpack.NewEncoder(&buf), &buf, tc.fields...)
			rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block, true, tc.end))
			if (cs.streamErr != nil) != tc.bad {
				t.Fatalf("stream error = %v", cs.streamErr)
			}
			if c.isClosed() {
				t.Fatal("stream error killed unrelated streams")
			}
		})
	}
}

func TestH2InformationalAndTrailers(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	cs := newH2ReadLoopTestStream(t, c, 1)
	rl := &http2ClientConnReadLoop{h2Conn: c}
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	send := func(end bool, fields ...hpack.HeaderField) {
		block := encodeHPACKBlockForCanceledStreamTest(t, enc, &buf, fields...)
		rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block, true, end))
	}
	send(false, hpack.HeaderField{Name: ":status", Value: "103"}, hpack.HeaderField{Name: "link", Value: "early"})
	if cs.headersHandled || cs.readHeaderEnd {
		t.Fatal("informational headers started body handler")
	}
	send(false, hpack.HeaderField{Name: ":status", Value: "200"}, hpack.HeaderField{Name: "content-length", Value: "0"})
	send(true, hpack.HeaderField{Name: "x-checksum", Value: "done"})
	if cs.streamErr != nil || !cs.readEndStream.Load() || cs.resp.Trailer.Get("x-checksum") != "done" || cs.resp.Header.Get("link") != "" {
		t.Fatalf("bad final response: %+v (%v)", cs.resp, cs.streamErr)
	}
}

func TestH2LimitsHeaderExpansion(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	cs := newH2ReadLoopTestStream(t, c, 1)
	rl := &http2ClientConnReadLoop{h2Conn: c}
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	fields := []hpack.HeaderField{{Name: ":status", Value: "200"}}
	for i := 0; i < 80000; i++ {
		fields = append(fields, hpack.HeaderField{Name: "x", Value: "x"})
	}
	block := encodeHPACKBlockForCanceledStreamTest(t, enc, &buf, fields...)
	for i := 0; i < len(block); i += defaultMaxFrameSize {
		end := i + defaultMaxFrameSize
		if end > len(block) {
			end = len(block)
		}
		if i == 0 {
			rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block[i:end], end == len(block), false))
		} else {
			rl.processHeaderFragment(block[i:end], end == len(block))
		}
	}
	if cs.streamErr == nil && !c.isClosed() {
		t.Fatal("oversized decoded header list accepted")
	}
	if len(rl.headerFields) != 0 {
		t.Fatal("oversized fields retained")
	}
}

func TestH2ResetAndGoAwayReturnErrors(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	rl := &http2ClientConnReadLoop{h2Conn: c}
	accepted := newH2ReadLoopTestStream(t, c, 1)
	refused := newH2ReadLoopTestStream(t, c, 3)
	rl.processGoAway(&http2.GoAwayFrame{LastStreamID: 1})
	if accepted.readEndStream.Load() || !refused.readEndStream.Load() || !h2RequestCanRetry(refused.req, refused.streamErr) {
		t.Fatal("GOAWAY did not isolate unprocessed streams")
	}
	rl.processResetStream(&http2.RSTStreamFrame{FrameHeader: http2.FrameHeader{StreamID: 1}, ErrCode: http2.ErrCodeCancel})
	if accepted.streamErr == nil {
		t.Fatal("reset returned success")
	}
	if h2RequestCanRetry(&http.Request{Method: "POST"}, errH2ConnClosed) {
		t.Fatal("ambiguous POST would be replayed")
	}
}

func TestH2HeaderBlocksBoundContinuationCount(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	rl := &http2ClientConnReadLoop{h2Conn: c}
	rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, nil, false, false))
	for i := 0; i < 1024; i++ {
		rl.processHeaderFragment(nil, false)
	}
	if !c.isClosed() {
		t.Fatal("unbounded empty CONTINUATION flood accepted")
	}
}

func TestH2UploadTimeoutIncludesZeroWindow(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	c.initialWindowSize = 0
	c.currentStreamID = 1
	req, _ := http.NewRequest("POST", "http://h2.test/", nil)
	cs, err := c.newStream(req, []byte("POST / HTTP/2\r\nHost: h2.test\r\n\r\nx"), &LowhttpExecConfig{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cs.doRequest() }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("upload timeout ignored")
	}
	cs.abort()
}

func TestH2RequestEncoderReusesDynamicTable(t *testing.T) {
	blocks := make(chan []byte, 2)
	c := newH2ReadLoopTestConn(t, h2FrameCapture(func(f http2.Frame) {
		if h, ok := f.(*http2.HeadersFrame); ok {
			blocks <- append([]byte(nil), h.HeaderBlockFragment()...)
		}
	}))
	c.currentStreamID = 1
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "http://h2.test/", nil)
		cs, err := c.newStream(req, []byte("GET / HTTP/2\r\nHost: h2.test\r\nX-Long: "+strings.Repeat("shared", 20)+"\r\n\r\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.doRequest(); err != nil {
			t.Fatal(err)
		}
		cs.releaseSlot()
		cs.recycle()
	}
	first, second := <-blocks, <-blocks
	if len(second) >= len(first) {
		t.Fatalf("no header compression reuse: first=%d second=%d", len(first), len(second))
	}
	dec := hpack.NewDecoder(4096, nil)
	if _, err := dec.DecodeFull(first); err != nil {
		t.Fatal(err)
	}
	if _, err := dec.DecodeFull(second); err != nil {
		t.Fatal(err)
	}
}
