package lowhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// Exercise the actual read loop, request writer, cancellation and stream pool
// against a peer that finishes an in-flight response after receiving RST_STREAM.
func TestH2CanceledResponseKeepsSharedConnectionUsable(t *testing.T) {
	client, peer := net.Pipe()
	pool := h2PoolFor(context.Background(), time.Minute)
	t.Cleanup(pool.Clear)
	pc := &persistConn{conn: client, p: pool}
	pc.h2Conn()
	conn := pc.alt
	conn.pc = nil // this test owns the connection directly, outside h2ConnMap
	go conn.readLoop()
	peerDone := make(chan struct{})
	t.Cleanup(func() {
		conn.setClose()
		peer.Close()
		conn.idleTimer.Stop()
		select {
		case <-conn.readLoopExited:
		case <-time.After(time.Second):
			t.Error("client read loop did not exit")
		}
		select {
		case <-peerDone:
		case <-time.After(time.Second):
			t.Error("peer read loop did not exit")
		}
	})
	if err := peer.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	requests := make(chan uint32, 4)
	resets := make(chan uint32, 4)
	credit := make(chan uint32, 256)
	peerErrors := make(chan error, 1)
	go func() {
		defer close(peerDone)
		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(peer, preface); err != nil {
			peerErrors <- err
			return
		}
		if string(preface) != http2.ClientPreface {
			peerErrors <- errors.New("invalid client preface")
			return
		}
		reader := http2.NewFramer(io.Discard, peer)
		for {
			frame, err := reader.ReadFrame()
			if err != nil {
				return
			}
			switch f := frame.(type) {
			case *http2.DataFrame:
				if f.StreamEnded() {
					requests <- f.StreamID
				}
			case *http2.RSTStreamFrame:
				resets <- f.StreamID
			case *http2.WindowUpdateFrame:
				if f.StreamID == 0 {
					credit <- f.Increment
				}
			}
		}
	}()
	waitID := func(ch <-chan uint32) uint32 {
		t.Helper()
		select {
		case id := <-ch:
			return id
		case err := <-peerErrors:
			t.Fatal(err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for peer frame")
		}
		return 0
	}
	if err := conn.preface(); err != nil {
		t.Fatal(err)
	}
	if got := waitID(credit); got != defaultStreamReceiveWindowSize-65535 {
		t.Fatalf("initial credit = %d", got)
	}
	writer := http2.NewFramer(peer, nil)
	if err := writer.WriteSettings(); err != nil {
		t.Fatal(err)
	}
	type response struct {
		packet []byte
		err    error
	}
	startRequest := func(ctx context.Context, firstHeader func()) <-chan response {
		t.Helper()
		result := make(chan response, 1)
		go func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "http://h2.test/", nil)
			if err != nil {
				result <- response{err: err}
				return
			}
			stream, err := conn.newStream(req, []byte("GET / HTTP/2\r\nHost: h2.test\r\n\r\n"), nil)
			if err != nil {
				result <- response{err: err}
				return
			}
			stream.SetReadFirstFrameCallback(firstHeader)
			if err := stream.doRequest(); err != nil {
				stream.abort()
				result <- response{err: err}
				return
			}
			_, packet, err := stream.waitResponse(ctx, 5*time.Second)
			result <- response{packet, err}
		}()
		return result
	}
	waitResponse := func(ch <-chan response) response {
		t.Helper()
		select {
		case result := <-ch:
			return result
		case <-time.After(2 * time.Second):
			t.Fatal("request did not complete promptly")
		}
		return response{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstHeader := make(chan struct{})
	canceled := startRequest(ctx, func() { close(firstHeader) })
	oldID := waitID(requests)
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	shared := hpack.HeaderField{Name: "x-hpack-shared", Value: "from-canceled-stream"}
	block := encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"}, shared)
	if err := writer.WriteHeaders(http2.HeadersFrameParam{StreamID: oldID, BlockFragment: block[:1]}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstHeader:
	case <-time.After(time.Second):
		t.Fatal("first header fragment was not processed")
	}
	cancel()
	if got := waitID(resets); got != oldID {
		t.Fatalf("reset stream = %d, want %d", got, oldID)
	}
	if result := waitResponse(canceled); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancel error = %v", result.err)
	}
	active := startRequest(context.Background(), nil)
	activeID := waitID(requests)
	if activeID != oldID+2 {
		t.Fatalf("next stream = %d, want same connection stream %d", activeID, oldID+2)
	}
	if err := writer.WriteContinuation(oldID, true, block[1:]); err != nil {
		t.Fatal(err)
	}

	// A full connection window of in-flight padded DATA must be credited back.
	// Otherwise the peer cannot send the active response on this connection.
	data, padding := bytes.Repeat([]byte("x"), 16352), make([]byte, 31)
	for remaining := defaultStreamReceiveWindowSize; remaining > 0; remaining -= defaultMaxFrameSize {
		if err := writer.WriteDataPadded(oldID, false, data, padding); err != nil {
			t.Fatal(err)
		}
	}
	var returned uint32
	for returned < defaultStreamReceiveWindowSize {
		returned += waitID(credit)
	}
	if returned != defaultStreamReceiveWindowSize {
		t.Fatalf("returned credit = %d", returned)
	}
	block = encodeHPACKBlockForCanceledStreamTest(t, encoder, &encoded,
		hpack.HeaderField{Name: ":status", Value: "200"}, shared)
	if err := writer.WriteHeaders(http2.HeadersFrameParam{StreamID: activeID, EndHeaders: true, BlockFragment: block}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteData(activeID, true, []byte("ok")); err != nil {
		t.Fatal(err)
	}
	result := waitResponse(active)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !bytes.Contains(result.packet, []byte(shared.Value)) || !bytes.HasSuffix(result.packet, []byte("\r\n\r\nok")) {
		t.Fatalf("active response corrupted: %s", result.packet)
	}
	if conn.isClosed() {
		t.Fatal("stream cancellation closed the shared connection")
	}
	conn.mu.Lock()
	activeCount, retainedCount := conn.activeStreams, len(conn.streams)
	conn.mu.Unlock()
	if activeCount != 0 || retainedCount != 0 {
		t.Fatalf("streams leaked: active=%d retained=%d", activeCount, retainedCount)
	}
}
