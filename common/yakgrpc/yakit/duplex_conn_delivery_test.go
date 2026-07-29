package yakit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServerPushSlowClientDoesNotBlockOtherClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	slowStarted := make(chan struct{}, 1)
	releaseSlow := make(chan struct{})
	fastReceived := make(chan *ypb.DuplexConnectionResponse, 1)

	registerServerPushCallback("slow-client", ctx, 2, func(response *ypb.DuplexConnectionResponse) error {
		slowStarted <- struct{}{}
		<-releaseSlow
		return nil
	})
	registerServerPushCallback("fast-client", ctx, 2, func(response *ypb.DuplexConnectionResponse) error {
		fastReceived <- response
		return nil
	})
	t.Cleanup(func() {
		UnRegisterServerPushCallback("slow-client")
		UnRegisterServerPushCallback("fast-client")
	})

	response := &ypb.DuplexConnectionResponse{MessageType: ServerPushType_HttpFlow, Timestamp: 1}
	startedAt := time.Now()
	broadcastRaw(response)
	require.Less(t, time.Since(startedAt), 50*time.Millisecond)

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow client sender did not start")
	}
	select {
	case received := <-fastReceived:
		require.Same(t, response, received)
	case <-time.After(time.Second):
		t.Fatal("fast client was blocked by slow client")
	}

	unregisterStartedAt := time.Now()
	UnRegisterServerPushCallback("slow-client")
	require.Less(t, time.Since(unregisterStartedAt), 50*time.Millisecond)
	close(releaseSlow)
}

func TestServerPushQueueKeepsLatestCoalescedInvalidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	releaseFirst := make(chan struct{})
	sent := make(chan int64, 4)
	description := newServerPushDescription("coalescing-client", 1, func(response *ypb.DuplexConnectionResponse) error {
		sent <- response.GetTimestamp()
		if response.GetTimestamp() == 1 {
			<-releaseFirst
		}
		return nil
	})
	go description.run(ctx)
	t.Cleanup(description.stop)

	require.True(t, description.enqueue(&ypb.DuplexConnectionResponse{
		MessageType: ServerPushType_HttpFlow,
		Timestamp:   1,
	}))
	require.Equal(t, int64(1), receiveServerPushTimestamp(t, sent))
	require.True(t, description.enqueue(&ypb.DuplexConnectionResponse{
		MessageType: ServerPushType_HttpFlow,
		Timestamp:   2,
	}))
	require.True(t, description.enqueue(&ypb.DuplexConnectionResponse{
		MessageType: ServerPushType_HttpFlow,
		Timestamp:   3,
	}))
	require.True(t, description.enqueue(&ypb.DuplexConnectionResponse{
		MessageType: ServerPushType_HttpFlow,
		Timestamp:   4,
	}))

	close(releaseFirst)
	require.Equal(t, int64(2), receiveServerPushTimestamp(t, sent))
	require.Equal(t, int64(4), receiveServerPushTimestamp(t, sent))
	select {
	case unexpected := <-sent:
		t.Fatalf("unexpected superseded notification: %d", unexpected)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestHTTPFlowCommittedIsCoalescibleForSlowShadowConsumers(t *testing.T) {
	require.True(t, isCoalescibleServerPush(ServerPushType_HTTPFlowCommitted))
}

func TestServerPushClientSerializesSends(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var (
		mutex      sync.Mutex
		inSend     bool
		concurrent bool
		deliveries []int64
	)
	description := newServerPushDescription("serial-client", 16, func(response *ypb.DuplexConnectionResponse) error {
		mutex.Lock()
		if inSend {
			concurrent = true
		}
		inSend = true
		mutex.Unlock()
		time.Sleep(time.Millisecond)
		mutex.Lock()
		deliveries = append(deliveries, response.GetTimestamp())
		inSend = false
		mutex.Unlock()
		return nil
	})
	go description.run(ctx)
	t.Cleanup(description.stop)

	for timestamp := int64(1); timestamp <= 8; timestamp++ {
		require.True(t, description.enqueue(&ypb.DuplexConnectionResponse{
			MessageType: "ordered",
			Timestamp:   timestamp,
		}))
	}
	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return len(deliveries) == 8
	}, time.Second, 5*time.Millisecond)

	mutex.Lock()
	defer mutex.Unlock()
	require.False(t, concurrent)
	require.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7, 8}, deliveries)
}

func receiveServerPushTimestamp(t *testing.T, values <-chan int64) int64 {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server push delivery")
		return 0
	}
}

func BenchmarkServerPushSlowConsumerIsolation(b *testing.B) {
	response := &ypb.DuplexConnectionResponse{MessageType: ServerPushType_HttpFlow, Timestamp: 1}

	b.Run("legacy-synchronous-send", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Sleep(100 * time.Microsecond)
			_ = response
		}
	})

	b.Run("bounded-per-client-queue", func(b *testing.B) {
		ctx, cancel := context.WithCancel(context.Background())
		description := newServerPushDescription("benchmark-client", 8, func(*ypb.DuplexConnectionResponse) error {
			time.Sleep(100 * time.Microsecond)
			return nil
		})
		go description.run(ctx)
		b.Cleanup(func() {
			description.stop()
			cancel()
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			description.enqueue(response)
		}
	})
}
