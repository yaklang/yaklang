package netx

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDialContextCancelledBeforeConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DialContextWithoutProxy(ctx, "127.0.0.1:1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestDialContextCancelsRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := DialX("127.0.0.1:12345", DialX_WithContext(ctx), DialX_WithDisableProxy(true), DialX_WithTimeoutRetryWait(time.Second), DialX_WithDialer(func(time.Duration, string) (net.Conn, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}))
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("retry wait ignored cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("dial continued after cancel")
	}
}

func TestDialContextExpiredBudget(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := DialContextWithoutProxy(ctx, "127.0.0.1:1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}
