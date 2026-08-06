package minimartian

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
)

type proxyHandleContextIOTestAddr string

func (a proxyHandleContextIOTestAddr) Network() string { return "test" }
func (a proxyHandleContextIOTestAddr) String() string  { return string(a) }

type proxyHandleContextIOTestConn struct {
	reader           *bytes.Reader
	readPacketStart  *byte
	writePacketStart *byte
	deadlineCalls    atomic.Int64
}

func newProxyHandleContextIOTestConn(input []byte) *proxyHandleContextIOTestConn {
	return &proxyHandleContextIOTestConn{reader: bytes.NewReader(input)}
}

func (c *proxyHandleContextIOTestConn) Read(packet []byte) (int, error) {
	if len(packet) > 0 {
		c.readPacketStart = &packet[0]
	}
	return c.reader.Read(packet)
}

func (c *proxyHandleContextIOTestConn) Write(packet []byte) (int, error) {
	if len(packet) > 0 {
		c.writePacketStart = &packet[0]
	}
	return len(packet), nil
}

func (c *proxyHandleContextIOTestConn) Close() error { return nil }
func (c *proxyHandleContextIOTestConn) LocalAddr() net.Addr {
	return proxyHandleContextIOTestAddr("local")
}
func (c *proxyHandleContextIOTestConn) RemoteAddr() net.Addr {
	return proxyHandleContextIOTestAddr("remote")
}
func (c *proxyHandleContextIOTestConn) SetDeadline(time.Time) error {
	c.deadlineCalls.Add(1)
	return nil
}
func (c *proxyHandleContextIOTestConn) SetReadDeadline(time.Time) error  { return nil }
func (c *proxyHandleContextIOTestConn) SetWriteDeadline(time.Time) error { return nil }

func TestProxyHandleContextIODirectPacketTransfer(t *testing.T) {
	const packetSize = 64 * 1024
	input := bytes.Repeat([]byte("r"), packetSize)
	conn := newProxyHandleContextIOTestConn(input)
	proxyCtx, err := CreateProxyHandleContext(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseProxyHandleContext(proxyCtx)

	readPacket := make([]byte, packetSize)
	if _, err := io.ReadFull(proxyCtx.Session().brw.Reader, readPacket); err != nil {
		t.Fatal(err)
	}
	if conn.readPacketStart != &readPacket[0] {
		t.Fatal("proxy reader copied the destination packet")
	}

	writePacket := bytes.Repeat([]byte("w"), packetSize)
	if _, err := proxyCtx.Session().brw.Writer.Write(writePacket); err != nil {
		t.Fatal(err)
	}
	if err := proxyCtx.Session().brw.Writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if conn.writePacketStart != &writePacket[0] {
		t.Fatal("proxy writer copied the source packet")
	}
}

func TestProxyHandleContextIOCancellationUnblocksRead(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	proxyCtx, err := CreateProxyHandleContext(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseProxyHandleContext(proxyCtx)

	readResult := make(chan error, 1)
	go func() {
		_, readErr := proxyCtx.Session().brw.Reader.ReadByte()
		readResult <- readErr
	}()
	cancel()

	select {
	case readErr := <-readResult:
		if !errors.Is(readErr, context.Canceled) {
			t.Fatalf("read error = %v, want context canceled", readErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context cancellation did not unblock the proxy read")
	}
}

func TestReleaseProxyHandleContextStopsDeadlineWatcher(t *testing.T) {
	conn := newProxyHandleContextIOTestConn(nil)
	ctx, cancel := context.WithCancel(context.Background())
	proxyCtx, err := CreateProxyHandleContext(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}

	releaseProxyHandleContext(proxyCtx)
	cancel()
	if calls := conn.deadlineCalls.Load(); calls != 0 {
		t.Fatalf("released context changed connection deadline %d times", calls)
	}
}

var benchmarkProxyHandleContextIOPacket []byte

func BenchmarkProxyHandleContextIO(b *testing.B) {
	readBody := bytes.Repeat([]byte("r"), 64*1024)
	writeBody := bytes.Repeat([]byte("w"), 256*1024)

	for _, test := range []struct {
		name       string
		wrapReader func(io.Reader) io.Reader
		wrapWriter func(io.Writer) io.Writer
	}{
		{
			name: "legacy_per_operation_context_io",
			wrapReader: func(reader io.Reader) io.Reader {
				return ctxio.NewReader(context.Background(), reader)
			},
			wrapWriter: func(writer io.Writer) io.Writer {
				return ctxio.NewWriter(context.Background(), writer)
			},
		},
		{
			name:       "connection_bound_direct_io",
			wrapReader: func(reader io.Reader) io.Reader { return reader },
			wrapWriter: func(writer io.Writer) io.Writer { return writer },
		},
	} {
		b.Run(test.name+"/read-64k", func(b *testing.B) {
			source := bytes.NewReader(readBody)
			buffers := acquireProxyHandleBuffers(test.wrapReader(source), io.Discard)
			defer releaseProxyHandleBuffers(buffers)
			packet := make([]byte, len(readBody))
			b.ReportAllocs()
			b.SetBytes(int64(len(readBody)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				source.Reset(readBody)
				if _, err := io.ReadFull(buffers.brw.Reader, packet); err != nil {
					b.Fatal(err)
				}
				benchmarkProxyHandleContextIOPacket = packet
			}
		})

		b.Run(test.name+"/write-256k", func(b *testing.B) {
			buffers := acquireProxyHandleBuffers(bytes.NewReader(nil), test.wrapWriter(io.Discard))
			defer releaseProxyHandleBuffers(buffers)
			b.ReportAllocs()
			b.SetBytes(int64(len(writeBody)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := buffers.brw.Writer.Write(writeBody); err != nil {
					b.Fatal(err)
				}
				if err := buffers.brw.Writer.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
