package lowhttp

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"golang.org/x/net/http2"
)

func readDataFrameForTest(t *testing.T, streamID uint32, data, padding []byte) *http2.DataFrame {
	t.Helper()

	var wire bytes.Buffer
	writer := http2.NewFramer(&wire, nil)
	if err := writer.WriteDataPadded(streamID, false, data, padding); err != nil {
		t.Fatalf("write DATA frame: %v", err)
	}
	reader := http2.NewFramer(io.Discard, &wire)
	frame, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("read DATA frame: %v", err)
	}
	return frame.(*http2.DataFrame)
}

func TestH2CanceledStreamReturnsConnectionWindow(t *testing.T) {
	var controlWire bytes.Buffer
	conn := &http2ClientConn{
		mu:           new(sync.Mutex),
		streams:      make(map[uint32]*http2ClientStream),
		fr:           http2.NewFramer(&controlWire, nil),
		frWriteMutex: new(sync.Mutex),
	}
	rl := &http2ClientConnReadLoop{h2Conn: conn}

	data := bytes.Repeat([]byte("x"), 1024)
	padding := make([]byte, 31)
	frame := readDataFrameForTest(t, 1, data, padding)
	rl.processData(frame)

	reader := http2.NewFramer(io.Discard, &controlWire)
	written, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("read connection WINDOW_UPDATE: %v", err)
	}
	update, ok := written.(*http2.WindowUpdateFrame)
	if !ok {
		t.Fatalf("written frame type = %T, want *http2.WindowUpdateFrame", written)
	}
	if update.StreamID != 0 {
		t.Fatalf("WINDOW_UPDATE stream id = %d, want connection stream 0", update.StreamID)
	}
	if want := frame.Header().Length; update.Increment != want {
		t.Fatalf("WINDOW_UPDATE increment = %d, want full DATA frame payload %d", update.Increment, want)
	}
	if _, err := reader.ReadFrame(); err != io.EOF {
		t.Fatalf("unexpected additional control frame: %v", err)
	}
}
