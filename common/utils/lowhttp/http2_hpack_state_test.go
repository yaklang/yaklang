package lowhttp

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func encodeHPACKBlock(t *testing.T, encoder *hpack.Encoder, output *bytes.Buffer, fields ...hpack.HeaderField) []byte {
	t.Helper()
	output.Reset()
	for _, field := range fields {
		if err := encoder.WriteField(field); err != nil {
			t.Fatalf("encode HPACK field: %v", err)
		}
	}
	return append([]byte(nil), output.Bytes()...)
}

func readHeadersFrameForTest(t *testing.T, streamID uint32, block []byte, endHeaders, endStream bool) *http2.HeadersFrame {
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

func writeSplitHeaderFramesForTest(t *testing.T, streamID uint32, first, second []byte) []byte {
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
	firstBlock := encodeHPACKBlock(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"},
		sharedHeader,
	)
	secondBlock := encodeHPACKBlock(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"},
		sharedHeader,
	)

	// The second block deliberately references the dynamic-table entry created
	// by the first block, so decoding it without the canceled stream must fail.
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

	// Stream 1 has already been removed after cancellation. Its header block is
	// split across HEADERS and CONTINUATION to exercise the discard path fully.
	split := len(firstBlock) / 2
	wire := bytes.NewReader(writeSplitHeaderFramesForTest(t, 1, firstBlock[:split], firstBlock[split:]))
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

	rl.processHeaders(readHeadersFrameForTest(t, stream.ID, secondBlock, true, true))
	if got := stream.resp.Header.Get(sharedHeader.Name); got != sharedHeader.Value {
		t.Fatalf("decoded shared header = %q, want %q", got, sharedHeader.Value)
	}
	if stream.resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", stream.resp.StatusCode, http.StatusOK)
	}
}
