package lowhttp

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"testing"

	"golang.org/x/net/http2"
)

func readDataFrameForCanceledStreamTest(t *testing.T, streamID uint32, data, padding []byte) *http2.DataFrame {
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

func TestH2DataWindowIncludesPadding(t *testing.T) {
	for _, state := range []string{"active", "ended", "removed"} {
		for _, payload := range []struct {
			name          string
			data, padding []byte
		}{
			{"plain", []byte("body"), nil},
			{"padded", []byte("body"), make([]byte, 31)},
			{"padding_only", nil, make([]byte, 31)},
			{"empty_padding_field", nil, []byte{}},
			{"empty", nil, nil},
		} {
			t.Run(state+"/"+payload.name, func(t *testing.T) {
				var output bytes.Buffer
				conn := newH2ReadLoopTestConn(t, &output)
				rl := &http2ClientConnReadLoop{h2Conn: conn}
				stream := newH2ReadLoopTestStream(t, conn, 1)
				stream.readHeaderEnd = true
				if state == "ended" {
					stream.setEndStream()
				}
				if state == "removed" {
					stream.releaseSlot()
				}
				frame := readDataFrameForCanceledStreamTest(t, 1, payload.data, payload.padding)
				frame.Flags |= http2.FlagDataEndStream
				rl.processData(frame)
				reader := http2.NewFramer(io.Discard, &output)
				if frame.Length > 0 {
					ids := []uint32{0}
					for _, id := range ids {
						written, err := reader.ReadFrame()
						if err != nil {
							t.Fatal(err)
						}
						update, ok := written.(*http2.WindowUpdateFrame)
						if !ok || update.StreamID != id || update.Increment != frame.Length {
							t.Fatalf("window update = %v, want stream %d credit %d", written, id, frame.Length)
						}
					}
				}
				if frame, err := reader.ReadFrame(); err != io.EOF {
					t.Fatalf("extra frame: %v, %v", frame, err)
				}
				wantBody := []byte(nil)
				if state == "active" {
					wantBody = payload.data
				}
				if !bytes.Equal(stream.bodyBuffer.Bytes(), wantBody) {
					t.Fatalf("body = %q, want %q", stream.bodyBuffer.Bytes(), wantBody)
				}
				if state == "active" && !stream.readEndStream.Load() {
					t.Fatal("final DATA did not end stream")
				}
			})
		}
	}
}

func TestH2FrameProcessingConcurrentWithCancelAndRecycle(t *testing.T) {
	conn := newH2ReadLoopTestConn(t, io.Discard)
	rl := &http2ClientConnReadLoop{h2Conn: conn}
	// Reuse pooled objects while late DATA, WINDOW_UPDATE and RST_STREAM race
	// with request cancellation. Frame processing must not touch recycled state.
	for i := uint32(0); i < 200; i++ {
		id := i*2 + 1
		stream := newH2ReadLoopTestStream(t, conn, id)
		stream.readHeaderEnd = true
		data := readDataFrameForCanceledStreamTest(t, id, []byte(fmt.Sprint(i)), nil)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			rl.processData(data)
			rl.processWindowUpdate(&http2.WindowUpdateFrame{FrameHeader: http2.FrameHeader{StreamID: id}, Increment: 1})
			rl.processResetStream(&http2.RSTStreamFrame{FrameHeader: http2.FrameHeader{StreamID: id}})
		}()
		go func() { defer wg.Done(); stream.abort() }()
		wg.Wait()
		if conn.streamByID(id) != nil {
			t.Fatal("canceled stream remained in the connection")
		}
	}
	if conn.activeStreams != 0 {
		t.Fatalf("active streams = %d", conn.activeStreams)
	}
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
	frame := readDataFrameForCanceledStreamTest(t, 1, data, padding)
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
