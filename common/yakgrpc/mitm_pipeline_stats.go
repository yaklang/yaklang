//go:build !yakit_exclude

package yakgrpc

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mitmPipelineStage uint8

const (
	mitmPipelineStageRequestProcessing mitmPipelineStage = iota + 1
	mitmPipelineStageManualRequest
	mitmPipelineStageUpstream
	mitmPipelineStageLocalResponse
	mitmPipelineStageResponseProcessing
	mitmPipelineStageManualResponse
)

type mitmPipelineRequestState struct {
	stage       mitmPipelineStage
	stageAt     time.Time
	dispatched  bool
	manualDepth int
}

// mitmPipelineTracker owns lightweight, process-local metrics for one MITMV2
// stream. Active maps contain only unfinished work, so long-running sessions do
// not accumulate completed requests or flows.
type mitmPipelineTracker struct {
	sessionID string
	startedAt time.Time
	now       func() time.Time

	requestTotal           atomic.Uint64
	dispatchTotal          atomic.Uint64
	upstreamCompletedTotal atomic.Uint64
	responseMirroredTotal  atomic.Uint64
	droppedTotal           atomic.Uint64
	flowBuiltTotal         atomic.Uint64
	persistEnqueuedTotal   atomic.Uint64
	persistedTotal         atomic.Uint64
	persistFailedTotal     atomic.Uint64

	mu       sync.Mutex
	requests map[*http.Request]*mitmPipelineRequestState
	persists map[*schema.HTTPFlow]time.Time
}

func newMITMPipelineTracker(sessionID string) *mitmPipelineTracker {
	now := time.Now
	return &mitmPipelineTracker{
		sessionID: sessionID,
		startedAt: now(),
		now:       now,
		requests:  make(map[*http.Request]*mitmPipelineRequestState),
		persists:  make(map[*schema.HTTPFlow]time.Time),
	}
}

func (t *mitmPipelineTracker) requestObserved(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	now := t.now()
	t.mu.Lock()
	if _, exists := t.requests[req]; !exists {
		t.requests[req] = &mitmPipelineRequestState{
			stage:   mitmPipelineStageRequestProcessing,
			stageAt: now,
		}
		t.requestTotal.Add(1)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) manualWaitStarted(req *http.Request, response bool) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		state.manualDepth++
		if response {
			state.stage = mitmPipelineStageManualResponse
		} else {
			state.stage = mitmPipelineStageManualRequest
		}
		state.stageAt = t.now()
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) manualWaitFinished(req *http.Request, response bool) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		if state.manualDepth > 0 {
			state.manualDepth--
		}
		if response {
			state.stage = mitmPipelineStageResponseProcessing
		} else {
			state.stage = mitmPipelineStageRequestProcessing
		}
		state.stageAt = t.now()
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) requestDispatched(req *http.Request, localResponse bool) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		state.stageAt = t.now()
		if localResponse {
			state.stage = mitmPipelineStageLocalResponse
		} else {
			state.stage = mitmPipelineStageUpstream
			if !state.dispatched {
				state.dispatched = true
				t.dispatchTotal.Add(1)
			}
		}
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) upstreamCompleted(req *http.Request, succeeded bool) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		if succeeded && state.dispatched && state.stage == mitmPipelineStageUpstream {
			t.upstreamCompletedTotal.Add(1)
		}
		state.stage = mitmPipelineStageResponseProcessing
		state.stageAt = t.now()
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) responseMirrored(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		t.responseMirroredTotal.Add(1)
		state.stage = mitmPipelineStageResponseProcessing
		state.stageAt = t.now()
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) responseProcessingFinished(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	delete(t.requests, req)
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) requestDropped(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if _, exists := t.requests[req]; exists {
		delete(t.requests, req)
		t.droppedTotal.Add(1)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) flowBuilt() {
	if t != nil {
		t.flowBuiltTotal.Add(1)
	}
}

func (t *mitmPipelineTracker) persistEnqueued(flow *schema.HTTPFlow) {
	if t == nil || flow == nil {
		return
	}
	t.mu.Lock()
	if _, exists := t.persists[flow]; !exists {
		t.persists[flow] = t.now()
		t.persistEnqueuedTotal.Add(1)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) persistFinished(flow *schema.HTTPFlow, success bool) {
	if t == nil || flow == nil {
		return
	}
	t.mu.Lock()
	if _, exists := t.persists[flow]; exists {
		delete(t.persists, flow)
		if success {
			t.persistedTotal.Add(1)
		} else {
			t.persistFailedTotal.Add(1)
		}
	}
	t.mu.Unlock()
}

func ageMilliseconds(now, startedAt time.Time) int64 {
	if startedAt.IsZero() || now.Before(startedAt) {
		return 0
	}
	return now.Sub(startedAt).Milliseconds()
}

func updateOldestAge(current *int64, candidate int64) {
	if candidate > *current {
		*current = candidate
	}
}

func (t *mitmPipelineTracker) snapshot(dbQueueDepth, dbQueueCapacity int) *ypb.MITMPipelineStats {
	if t == nil {
		return nil
	}
	now := t.now()
	stats := &ypb.MITMPipelineStats{
		Version:                    1,
		SessionId:                  t.sessionID,
		SessionStartedAtUnixMs:     t.startedAt.UnixMilli(),
		GeneratedAtUnixMs:          now.UnixMilli(),
		RequestTotal:               t.requestTotal.Load(),
		DispatchTotal:              t.dispatchTotal.Load(),
		UpstreamCompletedTotal:     t.upstreamCompletedTotal.Load(),
		ResponseMirroredTotal:      t.responseMirroredTotal.Load(),
		DroppedTotal:               t.droppedTotal.Load(),
		FlowBuiltTotal:             t.flowBuiltTotal.Load(),
		PersistEnqueuedTotal:       t.persistEnqueuedTotal.Load(),
		PersistedTotal:             t.persistedTotal.Load(),
		PersistFailedTotal:         t.persistFailedTotal.Load(),
		DatabaseWriteQueueDepth:    int64(dbQueueDepth),
		DatabaseWriteQueueCapacity: int64(dbQueueCapacity),
	}

	t.mu.Lock()
	stats.ActiveTotal = int64(len(t.requests))
	for _, state := range t.requests {
		age := ageMilliseconds(now, state.stageAt)
		switch state.stage {
		case mitmPipelineStageRequestProcessing:
			stats.PreDispatchActive++
			updateOldestAge(&stats.OldestPreDispatchAgeMs, age)
		case mitmPipelineStageManualRequest, mitmPipelineStageManualResponse:
			stats.ManualActive++
			updateOldestAge(&stats.OldestManualAgeMs, age)
		case mitmPipelineStageUpstream:
			stats.UpstreamActive++
			updateOldestAge(&stats.OldestUpstreamAgeMs, age)
		case mitmPipelineStageLocalResponse, mitmPipelineStageResponseProcessing:
			stats.ResponseProcessingActive++
			updateOldestAge(&stats.OldestResponseProcessingAgeMs, age)
		}
	}
	stats.PersistActive = int64(len(t.persists))
	for _, startedAt := range t.persists {
		updateOldestAge(&stats.OldestPersistAgeMs, ageMilliseconds(now, startedAt))
	}
	t.mu.Unlock()
	return stats
}
