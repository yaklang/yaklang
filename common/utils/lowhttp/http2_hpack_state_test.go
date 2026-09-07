package lowhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func encodeHPACKBlockForCanceledStreamTest(t *testing.T, encoder *hpack.Encoder, output *bytes.Buffer, fields ...hpack.HeaderField) []byte {
	t.Helper()
	output.Reset()
	for _, field := range fields {
		if err := encoder.WriteField(field); err != nil {
			t.Fatalf("encode HPACK field: %v", err)
		}
	}
	return append([]byte(nil), output.Bytes()...)
}

func newH2ReadLoopTestConn(t *testing.T, output io.Writer) *http2ClientConn {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	conn := &http2ClientConn{
		conn:              client,
		mu:                new(sync.Mutex),
		streams:           make(map[uint32]*http2ClientStream),
		hDec:              hpack.NewDecoder(4096, nil),
		fr:                http2.NewFramer(output, nil),
		frWriteMutex:      new(sync.Mutex),
		closeCh:           make(chan struct{}),
		idleTimeout:       time.Hour,
		idleTimer:         time.NewTimer(time.Hour),
		maxStreamsCount:   100,
		headerListMaxSize: ^uint32(0),
		initialWindowSize: 65535,
		sendWindow:        65535,
		maxFrameSize:      defaultMaxFrameSize,
		http2StreamPool:   &sync.Pool{New: func() any { return new(http2ClientStream) }},
	}
	conn.streamsCond = sync.NewCond(conn.mu)
	t.Cleanup(func() { conn.idleTimer.Stop() })
	return conn
}

func newH2ReadLoopTestStream(t *testing.T, conn *http2ClientConn, id uint32) *http2ClientStream {
	t.Helper()
	stream, err := conn.newStream(&http.Request{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.mu.Lock()
	stream.ID = id
	conn.streams[id] = stream
	conn.mu.Unlock()
	return stream
}

func TestH2HPACKCancellationBetweenFragmentsAndStreamReuse(t *testing.T) {
	for _, cancelBeforeHeaders := range []bool{true, false} {
		name := "between_fragments"
		if cancelBeforeHeaders {
			name = "before_headers"
		}
		t.Run(name, func(t *testing.T) {
			conn := newH2ReadLoopTestConn(t, io.Discard)
			rl := &http2ClientConnReadLoop{h2Conn: conn}
			var encoded bytes.Buffer
			encoder := hpack.NewEncoder(&encoded)
			shared := hpack.HeaderField{Name: "x-hpack-shared", Value: "survives-cancellation"}
			block := encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
				hpack.HeaderField{Name: ":status", Value: "200"}, shared)
			stream := newH2ReadLoopTestStream(t, conn, 1)
			if cancelBeforeHeaders {
				stream.abort()
			}
			reader := http2.NewFramer(io.Discard, bytes.NewReader(writeSplitHeaderFramesForCanceledStreamTest(t, 1, block[:1], block[1:])))
			frame, err := reader.ReadFrame()
			if err != nil {
				t.Fatal(err)
			}
			rl.processHeaders(frame.(*http2.HeadersFrame))
			if !cancelBeforeHeaders {
				stream.abort()
			}
			// Obtain the next stream before finishing the abandoned header block.
			// The previous object has already been cleared and returned to sync.Pool.
			next := newH2ReadLoopTestStream(t, conn, 3)
			frame, err = reader.ReadFrame()
			if err != nil {
				t.Fatal(err)
			}
			rl.processContinuation(frame.(*http2.ContinuationFrame))
			if next.readHeaderEnd || len(next.resp.Header) != 0 {
				t.Fatal("abandoned header block was delivered to the new stream")
			}
			block = encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
				hpack.HeaderField{Name: ":status", Value: "200"}, shared)
			rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 3, block, true, true))
			if got := next.resp.Header.Get(shared.Name); got != shared.Value {
				t.Fatalf("shared header = %q, want %q", got, shared.Value)
			}
			if !next.readEndStream.Load() || next.resp.StatusCode != http.StatusOK {
				t.Fatal("new stream did not finish successfully")
			}
		})
	}
}

func TestH2FragmentedHeadersPreserveStatusAndEndStream(t *testing.T) {
	conn := newH2ReadLoopTestConn(t, io.Discard)
	rl := &http2ClientConnReadLoop{h2Conn: conn}
	stream := newH2ReadLoopTestStream(t, conn, 1)
	var encoded, wire bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	block := encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "404"},
		hpack.HeaderField{Name: "x-split", Value: "yes"})
	writer := http2.NewFramer(&wire, nil)
	if err := writer.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, EndStream: true, BlockFragment: block[:1]}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteContinuation(1, true, block[1:]); err != nil {
		t.Fatal(err)
	}
	reader := http2.NewFramer(io.Discard, &wire)
	frame, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	rl.processHeaders(frame.(*http2.HeadersFrame))
	if stream.readEndStream.Load() {
		t.Fatal("stream ended before the header block was decoded")
	}
	frame, err = reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	rl.processContinuation(frame.(*http2.ContinuationFrame))
	if !stream.readEndStream.Load() || stream.resp.StatusCode != http.StatusNotFound || stream.resp.Header.Get("x-split") != "yes" {
		t.Fatalf("fragmented response not completed correctly: %#v", stream.resp)
	}
}

func TestH2HPACKDecodeErrorClosesConnectionAndWakesStreams(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "active"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			conn := newH2ReadLoopTestConn(t, io.Discard)
			rl := &http2ClientConnReadLoop{h2Conn: conn}
			broken := newH2ReadLoopTestStream(t, conn, 1)
			if canceled {
				broken.abort()
			}
			other := newH2ReadLoopTestStream(t, conn, 3)
			result := make(chan error, 1)
			go func() { _, _, err := other.waitResponse(context.Background(), 5*time.Second); result <- err }()
			// Indexed field zero is invalid HPACK on both active and canceled streams.
			rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, []byte{0x80}, true, true))
			select {
			case err := <-result:
				if !errors.Is(err, http2.ConnectionError(http2.ErrCodeCompression)) {
					t.Fatalf("response error = %v, want COMPRESSION_ERROR", err)
				}
				if errors.Is(err, errH2ConnClosed) {
					t.Fatal("compression error must not retry on the broken connection")
				}
			case <-time.After(time.Second):
				t.Fatal("other stream waited for timeout after HPACK corruption")
			}
			if !conn.isClosed() {
				t.Fatal("corrupted connection remained reusable")
			}
		})
	}
}

func readHeadersFrameForCanceledStreamTest(t *testing.T, streamID uint32, block []byte, endHeaders, endStream bool) *http2.HeadersFrame {
	t.Helper()
	var wire bytes.Buffer
	writer := http2.NewFramer(&wire, nil)
	if err := writer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndHeaders:    endHeaders,
		EndStream:     endStream,
	}); err != nil {
		t.Fatalf("write HEADERS frame: %v", err)
	}
	reader := http2.NewFramer(io.Discard, &wire)
	frame, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("read HEADERS frame: %v", err)
	}
	return frame.(*http2.HeadersFrame)
}

func writeSplitHeaderFramesForCanceledStreamTest(t *testing.T, streamID uint32, first, second []byte) []byte {
	t.Helper()
	var wire bytes.Buffer
	writer := http2.NewFramer(&wire, nil)
	if err := writer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: first,
		EndHeaders:    false,
	}); err != nil {
		t.Fatalf("write HEADERS frame: %v", err)
	}
	if err := writer.WriteContinuation(streamID, true, second); err != nil {
		t.Fatalf("write CONTINUATION frame: %v", err)
	}
	return wire.Bytes()
}

func TestH2CanceledStreamStillUpdatesHPACKState(t *testing.T) {
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	sharedHeader := hpack.HeaderField{Name: "x-hpack-shared", Value: "dynamic-value"}
	firstBlock := encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"},
		sharedHeader,
	)
	secondBlock := encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"},
		sharedHeader,
	)

	if _, err := hpack.NewDecoder(defaultHeaderTableSize, nil).DecodeFull(secondBlock); err == nil {
		t.Fatal("test HPACK block does not depend on prior connection state")
	}

	conn := &http2ClientConn{
		mu:      new(sync.Mutex),
		streams: make(map[uint32]*http2ClientStream),
		hDec:    hpack.NewDecoder(defaultHeaderTableSize, nil),
	}
	stream := &http2ClientStream{
		ID:                     3,
		h2Conn:                 conn,
		resp:                   &http.Response{ProtoMajor: 2, Header: make(http.Header)},
		bodyBuffer:             new(bytes.Buffer),
		readEndStreamSignal:    make(chan struct{}, 1),
		callbackLock:           new(sync.Mutex),
		firstFrameCallbackOnce: sync.Once{},
	}
	conn.streams[stream.ID] = stream
	rl := &http2ClientConnReadLoop{h2Conn: conn}

	split := len(firstBlock) / 2
	wire := bytes.NewReader(writeSplitHeaderFramesForCanceledStreamTest(t, 1, firstBlock[:split], firstBlock[split:]))
	reader := http2.NewFramer(io.Discard, wire)
	firstFrame, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("read HEADERS frame: %v", err)
	}
	rl.processHeaders(firstFrame.(*http2.HeadersFrame))
	secondFrame, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("read CONTINUATION frame: %v", err)
	}
	rl.processContinuation(secondFrame.(*http2.ContinuationFrame))

	rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, stream.ID, secondBlock, true, true))
	if got := stream.resp.Header.Get(sharedHeader.Name); got != sharedHeader.Value {
		t.Fatalf("decoded shared header = %q, want %q", got, sharedHeader.Value)
	}
	if stream.resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", stream.resp.StatusCode, http.StatusOK)
	}
}
