package yakit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestHTTPFlowLiveBrokerReplaysByIDAndSequence(t *testing.T) {
	broker := newHTTPFlowLiveBroker(2, 2)
	identity := "project-a"
	for id := uint64(1); id <= 3; id++ {
		record, ok := broker.publishCommitted(httpFlowLiveTestFlow(id, identity, 7))
		require.True(t, ok)
		require.Equal(t, id, record.Sequence)
		require.Empty(t, record.Flow.Request)
		require.Empty(t, record.Flow.Response)
		require.Equal(t, int64(10), record.RequestHijackAtUnixMs)
		require.Equal(t, int64(20), record.ResponseMirrorAtUnixMs)
		require.Equal(t, int64(30), record.FlowBuiltAtUnixMs)
		require.Equal(t, int64(40), record.PersistEnqueuedAtUnixMs)
		require.Equal(t, int64(50), record.PersistStartedAtUnixMs)
	}

	subscription, replay, gap := broker.subscribe(identity, 7, 0, 1)
	require.Nil(t, gap)
	require.NotNil(t, subscription)
	t.Cleanup(subscription.Close)
	require.Equal(t, []uint64{2, 3}, liveRecordIDs(replay))

	sequenceSubscription, sequenceReplay, gap := broker.subscribe(identity, 7, 2, 0)
	require.Nil(t, gap)
	require.NotNil(t, sequenceSubscription)
	t.Cleanup(sequenceSubscription.Close)
	require.Equal(t, []uint64{3}, liveRecordIDs(sequenceReplay))
	require.Equal(t, uint64(3), sequenceSubscription.InitialState().LatestSequence)
}

func TestHTTPFlowLiveBrokerIDCursorReplaysContiguousSequence(t *testing.T) {
	broker := newHTTPFlowLiveBroker(8, 8)
	identity := "project-id-order"
	for _, id := range []uint64{1, 3, 2} {
		_, ok := broker.publishCommitted(httpFlowLiveTestFlow(id, identity, 8))
		require.True(t, ok)
	}

	subscription, replay, gap := broker.subscribe(identity, 8, 0, 2)
	require.Nil(t, gap)
	require.NotNil(t, subscription)
	t.Cleanup(subscription.Close)
	require.Equal(t, []uint64{3, 2}, liveRecordIDs(replay))
	require.Equal(t, []uint64{2, 3}, liveRecordSequences(replay))
	require.Equal(t, HTTPFlowLiveState{
		OldestAvailableSequence: 1,
		LatestSequence:          3,
		HighWaterID:             3,
	}, subscription.InitialState())
}

func TestHTTPFlowLiveBrokerInitialStatePrecedesQueuedLiveEvents(t *testing.T) {
	broker := newHTTPFlowLiveBroker(8, 8)
	identity := "project-handoff"
	_, ok := broker.publishCommitted(httpFlowLiveTestFlow(1, identity, 12))
	require.True(t, ok)

	subscription, replay, gap := broker.subscribe(identity, 12, 0, 1)
	require.Nil(t, gap)
	require.Empty(t, replay)
	require.NotNil(t, subscription)
	t.Cleanup(subscription.Close)
	require.Equal(t, uint64(1), subscription.InitialState().LatestSequence)

	_, ok = broker.publishCommitted(httpFlowLiveTestFlow(2, identity, 12))
	require.True(t, ok)
	require.Equal(t, uint64(1), subscription.InitialState().LatestSequence)
	require.Equal(t, uint64(2), subscription.Snapshot().LatestSequence)
	select {
	case record := <-subscription.Events():
		require.Equal(t, uint64(2), record.Sequence)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first post-cutoff live record")
	}
}

func TestHTTPFlowLiveBrokerReturnsGapOutsideReplayWindow(t *testing.T) {
	broker := newHTTPFlowLiveBroker(2, 2)
	identity := "project-gap"
	for id := uint64(1); id <= 3; id++ {
		_, ok := broker.publishCommitted(httpFlowLiveTestFlow(id, identity, 9))
		require.True(t, ok)
	}

	subscription, replay, gap := broker.subscribe(identity, 9, 0, 0)
	require.Nil(t, subscription)
	require.Empty(t, replay)
	require.NotNil(t, gap)
	require.Equal(t, HTTPFlowLiveGapReplayWindowExceeded, gap.Reason)
	require.Equal(t, uint64(2), gap.OldestAvailableSequence)
	require.Equal(t, uint64(3), gap.LatestSequence)
	require.Equal(t, uint64(3), gap.HighWaterID)

	_, _, sequenceGap := broker.subscribe(identity, 9, 4, 3)
	require.NotNil(t, sequenceGap)
	require.Equal(t, HTTPFlowLiveGapCursorAhead, sequenceGap.Reason)
}

func TestHTTPFlowLiveBrokerSignalsSlowConsumerInsteadOfDroppingSilently(t *testing.T) {
	broker := newHTTPFlowLiveBroker(8, 1)
	identity := "project-slow"
	subscription, replay, gap := broker.subscribe(identity, 11, 0, 0)
	require.Nil(t, gap)
	require.Empty(t, replay)
	require.NotNil(t, subscription)
	t.Cleanup(subscription.Close)

	_, ok := broker.publishCommitted(httpFlowLiveTestFlow(1, identity, 11))
	require.True(t, ok)
	_, ok = broker.publishCommitted(httpFlowLiveTestFlow(2, identity, 11))
	require.True(t, ok)

	select {
	case liveGap := <-subscription.Gaps():
		require.Equal(t, HTTPFlowLiveGapSlowConsumer, liveGap.Reason)
		require.Equal(t, uint64(2), liveGap.LatestSequence)
		require.Equal(t, uint64(2), liveGap.HighWaterID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for explicit slow-consumer gap")
	}
}

func TestHTTPFlowLiveBrokerEvictsProjectsWithExplicitGap(t *testing.T) {
	broker := newHTTPFlowLiveBroker(4, 1)
	subscription, _, gap := broker.subscribe("old-project", 1, 0, 0)
	require.Nil(t, gap)
	require.NotNil(t, subscription)
	t.Cleanup(subscription.Close)

	for generation := uint64(2); generation <= httpFlowLiveRecentProjectSlotCount+1; generation++ {
		identity := "project-" + string(rune('a'+generation))
		_, ok := broker.publishCommitted(httpFlowLiveTestFlow(generation, identity, generation))
		require.True(t, ok)
	}

	select {
	case liveGap := <-subscription.Gaps():
		require.Equal(t, HTTPFlowLiveGapProjectEvicted, liveGap.Reason)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for project eviction gap")
	}
}

func TestHTTPFlowLiveBrokerConcurrentPublishSubscribeAndSnapshot(t *testing.T) {
	const (
		publisherCount = 4
		flowsPerWorker = 100
	)
	broker := newHTTPFlowLiveBroker(publisherCount*flowsPerWorker, 8)
	identity := "project-concurrent"
	generation := uint64(17)
	records := make(chan HTTPFlowLiveRecord, publisherCount*flowsPerWorker)
	var nextID atomic.Uint64
	var publishers sync.WaitGroup

	for range publisherCount {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for range flowsPerWorker {
				id := nextID.Add(1)
				record, ok := broker.publishCommitted(httpFlowLiveTestFlow(id, identity, generation))
				if ok {
					records <- record
				}
			}
		}()
	}

	var observers sync.WaitGroup
	for range 2 {
		observers.Add(1)
		go func() {
			defer observers.Done()
			for range flowsPerWorker {
				state := broker.snapshot(identity, generation)
				subscription, _, gap := broker.subscribe(
					identity,
					generation,
					state.LatestSequence,
					state.HighWaterID,
				)
				if gap == nil && subscription != nil {
					subscription.Snapshot()
					subscription.Close()
				}
			}
		}()
	}

	publishers.Wait()
	observers.Wait()
	close(records)

	seenSequences := make(map[uint64]struct{}, publisherCount*flowsPerWorker)
	for record := range records {
		seenSequences[record.Sequence] = struct{}{}
	}
	require.Len(t, seenSequences, publisherCount*flowsPerWorker)
	state := broker.snapshot(identity, generation)
	require.Equal(t, uint64(publisherCount*flowsPerWorker), state.LatestSequence)
	require.Equal(t, uint64(publisherCount*flowsPerWorker), state.HighWaterID)
}

func httpFlowLiveTestFlow(id uint64, identity string, generation uint64) *schema.HTTPFlow {
	flow := &schema.HTTPFlow{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Request:    "large-request-must-not-be-retained",
		Response:   "large-response-must-not-be-retained",
		Payload:    "payload-must-not-be-retained",
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			RequestHijackAtUnixMs:   10,
			ResponseMirrorAtUnixMs:  20,
			FlowBuiltAtUnixMs:       30,
			PersistEnqueuedAtUnixMs: 40,
			PersistStartedAtUnixMs:  50,
			DatabaseIdentity:        identity,
			ProjectGeneration:       generation,
			PersistedAtUnixMs:       time.Now().UnixMilli(),
		},
	}
	flow.ID = uint(id)
	return flow
}

func liveRecordIDs(records []HTTPFlowLiveRecord) []uint64 {
	ids := make([]uint64, 0, len(records))
	for _, record := range records {
		ids = append(ids, uint64(record.Flow.ID))
	}
	return ids
}

func liveRecordSequences(records []HTTPFlowLiveRecord) []uint64 {
	sequences := make([]uint64, 0, len(records))
	for _, record := range records {
		sequences = append(sequences, record.Sequence)
	}
	return sequences
}
