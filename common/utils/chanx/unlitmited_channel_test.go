package chanx

import (
	"context"
	"sync"
	"testing"
)

func TestUnlimitedChan_BasicSendReceive(t *testing.T) {
	ch := NewUnlimitedChan[int](context.Background(), 2)
	ch.FeedBlock(1)
	ch.FeedBlock(2)
	ch.FeedBlock(3)
	out := ch.OutputChannel()
	var results []int
	for i := 0; i < 3; i++ {
		results = append(results, <-out)
	}
	if results[0] != 1 || results[1] != 2 || results[2] != 3 {
		t.Errorf("unexpected results: %v", results)
	}
	ch.Close()
}

func TestUnlimitedChan_BufferOverflow(t *testing.T) {
	ch := NewUnlimitedChan[int](context.Background(), 1)
	for i := 0; i < 10; i++ {
		ch.FeedBlock(i)
	}
	out := ch.OutputChannel()
	var results []int
	for i := 0; i < 10; i++ {
		results = append(results, <-out)
	}
	for i := 0; i < 10; i++ {
		if results[i] != i {
			t.Errorf("buffer overflow test failed, got %v", results)
			break
		}
	}
	ch.Close()
}

func TestUnlimitedChan_ConcurrentFeed(t *testing.T) {
	ch := NewUnlimitedChan[int](context.Background(), 2)
	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			ch.FeedBlock(val)
		}(i)
	}
	wg.Wait()
	out := ch.OutputChannel()
	got := make(map[int]bool)
	for i := 0; i < 5; i++ {
		got[<-out] = true
	}
	for i := 0; i < 5; i++ {
		if !got[i] {
			t.Errorf("missing value %d in concurrent feed", i)
		}
	}
	ch.Close()
}

func TestUnlimitedChan_Close(t *testing.T) {
	ch := NewUnlimitedChan[int](context.Background(), 2)
	ch.FeedBlock(42)
	ch.Close()
	out := ch.OutputChannel()
	val, ok := <-out
	if !ok {
		t.Errorf("channel closed before reading value")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
	_, ok = <-out
	if ok {
		t.Errorf("channel should be closed")
	}
}
