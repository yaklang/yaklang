package lowhttp

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestHTTP2BodyStreamHandlerDoesNotSignalReusedStream(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseOldHandler := make(chan struct{})

	stream := &http2ClientStream{
		option: &LowhttpExecConfig{
			BodyStreamReaderHandler: func([]byte, io.ReadCloser) {
				close(handlerStarted)
				<-releaseOldHandler
			},
		},
		bodyStreamReader: io.NopCloser(strings.NewReader("")),
		bodyStreamDone:   make(chan struct{}),
	}

	oldDone := stream.bodyStreamDone
	stream.startBodyStreamHandler(nil)

	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("old body stream handler did not start")
	}

	// Simulate newStream resetting this http2ClientStream after sync.Pool reuse.
	newDone := make(chan struct{})
	stream.bodyStreamDone = newDone
	close(releaseOldHandler)

	select {
	case <-newDone:
		t.Fatal("old handler closed the reused stream's completion channel")
	case <-oldDone:
		// The old handler must only signal the channel that belonged to it.
	case <-time.After(time.Second):
		t.Fatal("old handler did not signal completion")
	}
}
