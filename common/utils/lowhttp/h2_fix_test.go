package lowhttp

import (
	"sync"
	"testing"
	"time"
)

func TestWindowSizeControl_CondLinkedToMutex(t *testing.T) {
	wc := newControl(100)
	done := make(chan struct{})

	go func() {
		wc.decreaseWindowSize(150) // blocks: 100 - 150 = -50
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("should block when window <= 0")
	default:
	}

	wc.increaseWindowSize(100) // -50 + 100 = 50 > 0 → unblock

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("should unblock after increase")
	}
}

func TestWindowSizeControl_AdjustWindowSize(t *testing.T) {
	wc := newControl(100)
	done := make(chan struct{})

	go func() {
		wc.decreaseWindowSize(150) // blocks
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("should block")
	default:
	}

	wc.adjustWindowSize(100) // unblock via SETTINGS delta

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("adjustWindowSize should wake blocked goroutine")
	}
}

func TestWindowSizeControl_ConcurrentSafety(t *testing.T) {
	wc := newControl(10000)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				wc.decreaseWindowSize(10)
				wc.increaseWindowSize(10)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock detected")
	}
}

func TestStreamID_UniqueAndOdd(t *testing.T) {
	h2Conn := &http2ClientConn{
		mu:              new(sync.Mutex),
		currentStreamID: 1,
	}

	const N = 100
	seen := make(map[uint32]struct{}, N)
	for i := 0; i < N; i++ {
		id := h2Conn.getNewStreamID()
		if id%2 == 0 {
			t.Fatalf("client stream ID must be odd, got %d", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate stream ID: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != N {
		t.Fatalf("expected %d unique IDs, got %d", N, len(seen))
	}
}
