package minimartian

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"testing"
)

func TestProxyHandleBuffersResetBetweenConnections(t *testing.T) {
	firstInput := bytes.NewBufferString("first")
	firstOutput := new(bytes.Buffer)
	first := acquireProxyHandleBuffers(firstInput, firstOutput)
	if _, err := first.brw.Reader.ReadString('t'); err != nil {
		t.Fatalf("read first input: %v", err)
	}
	if _, err := first.brw.WriteString("old"); err != nil {
		t.Fatalf("write first output: %v", err)
	}
	releaseProxyHandleBuffers(first)

	secondInput := bytes.NewBufferString("second")
	secondOutput := new(bytes.Buffer)
	second := acquireProxyHandleBuffers(secondInput, secondOutput)
	t.Cleanup(func() {
		releaseProxyHandleBuffers(second)
	})

	got, err := second.brw.Reader.ReadString('d')
	if err != nil {
		t.Fatalf("read second input: %v", err)
	}
	if got != "second" {
		t.Fatalf("read stale buffered input: got %q", got)
	}
	if _, err := second.brw.WriteString("new"); err != nil {
		t.Fatalf("write second output: %v", err)
	}
	if err := second.brw.Flush(); err != nil {
		t.Fatalf("flush second output: %v", err)
	}
	if got := secondOutput.String(); got != "new" {
		t.Fatalf("write to stale output: got %q", got)
	}
	if firstOutput.Len() != 0 {
		t.Fatalf("unflushed output from released connection was flushed: %q", firstOutput.String())
	}
}

func TestReleaseProxyHandleContextIsIdempotent(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, err := CreateProxyHandleContext(context.Background(), server)
	if err != nil {
		t.Fatalf("create proxy handle context: %v", err)
	}
	if ctx.Session().proxyHandleBuffers == nil {
		t.Fatal("expected pooled buffers to be owned by session")
	}

	releaseProxyHandleContext(ctx)
	if ctx.Session().proxyHandleBuffers != nil {
		t.Fatal("expected pooled buffers to be detached after release")
	}
	releaseProxyHandleContext(ctx)
}

var benchmarkProxyHandleReadWriter *bufio.ReadWriter

func BenchmarkProxyHandleBuffers(b *testing.B) {
	b.Run("allocate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkProxyHandleReadWriter = bufio.NewReadWriter(
				bufio.NewReader(emptyProxyHandleReader{}),
				bufio.NewWriter(io.Discard),
			)
		}
	})
	b.Run("pooled", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buffers := acquireProxyHandleBuffers(emptyProxyHandleReader{}, io.Discard)
			benchmarkProxyHandleReadWriter = buffers.brw
			releaseProxyHandleBuffers(buffers)
		}
	})
}
