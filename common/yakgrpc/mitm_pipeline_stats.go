//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mitmPipelineStage uint8

const (
	mitmPipelineStageRequestProcessing mitmPipelineStage = iota + 1
	mitmPipelineStageManualRequest
	mitmPipelineStageUpstream
	mitmPipelineStagePoolWait
	mitmPipelineStageDial
	mitmPipelineStageSend
	mitmPipelineStageFirstByte
	mitmPipelineStageRead
	mitmPipelineStageStreaming
	mitmPipelineStageLocalResponse
	mitmPipelineStageResponseProcessing
	mitmPipelineStageManualResponse
	mitmPipelineStageWebSocketOnline
	mitmPipelineStagePersistSubmit
	mitmPipelineStagePersistBlocked
	mitmPipelineStagePersistQueued
	mitmPipelineStagePersistWrite
	mitmPipelineStageCount
)

var mitmStageNames = [mitmPipelineStageCount]string{
	mitmPipelineStageRequestProcessing:  "request_processing",
	mitmPipelineStageManualRequest:      "manual_request",
	mitmPipelineStageUpstream:           "upstream",
	mitmPipelineStagePoolWait:           "pool_wait",
	mitmPipelineStageDial:               "dial",
	mitmPipelineStageSend:               "send",
	mitmPipelineStageFirstByte:          "first_byte",
	mitmPipelineStageRead:               "read",
	mitmPipelineStageStreaming:          "streaming",
	mitmPipelineStageLocalResponse:      "local_response",
	mitmPipelineStageResponseProcessing: "response_processing",
	mitmPipelineStageManualResponse:     "manual_response",
	mitmPipelineStageWebSocketOnline:    "websocket_online",
	mitmPipelineStagePersistSubmit:      "persist_submit",
	mitmPipelineStagePersistBlocked:     "persist_blocked",
	mitmPipelineStagePersistQueued:      "persist_queued",
	mitmPipelineStagePersistWrite:       "persist_write",
}

func (s mitmPipelineStage) isUpstream() bool {
	return s >= mitmPipelineStageUpstream && s <= mitmPipelineStageStreaming
}

type mitmPipelineRequestState struct {
	trace         mitmTrace
	dispatchedAt  time.Time
	upstreamDone  bool
	stopCancel    func() bool
	lastAttemptID uint64
}

// mitmPipelineTracker is scoped to one MITMV2 stream. Only unfinished work
// scales with traffic. Completed samples and aggregate history have fixed limits.
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

	mu          sync.Mutex
	closed      bool
	requests    map[*http.Request]*mitmPipelineRequestState
	persists    map[uint64]*mitmPersistState
	nextWriteID uint64
	history     [mitmHistorySeconds]mitmStoragePoint
	metrics     [mitmMetricSeconds]mitmMetricsBucket
	samples     mitmTraceSamples
	// Excluded traffic has its own bounded history so it cannot displace
	// slow and failed requests in the capture scope.
	filteredSamples mitmTraceSamples
}

func newMITMPipelineTracker(sessionID string) *mitmPipelineTracker {
	return &mitmPipelineTracker{
		sessionID: sessionID,
		startedAt: time.Now(),
		now:       time.Now,
		requests:  make(map[*http.Request]*mitmPipelineRequestState),
		persists:  make(map[uint64]*mitmPersistState),
	}
}

func (t *mitmPipelineTracker) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	for _, state := range t.requests {
		state.stopCancel()
		state.stopCancel = nil
	}
	t.requests = nil
	t.persists = nil
	clear(t.history[:])
	clear(t.metrics[:])
	t.samples = mitmTraceSamples{}
	t.filteredSamples = mitmTraceSamples{}
}

func (t *mitmPipelineTracker) requestObserved(req *http.Request, excludeWebSocket bool) {
	if t == nil || req == nil {
		return
	}
	ctx := req.Context()
	t.mu.Lock()
	if t.closed || t.requests[req] != nil {
		t.mu.Unlock()
		return
	}
	now := t.now()
	protocol := req.Proto
	filter := mitmFilterPending
	if httpctx.GetIsWebWebsocketRequest(req) {
		protocol = "WebSocket"
		if excludeWebSocket {
			filter = mitmFilterWebSocketExcluded
		}
	}
	state := &mitmPipelineRequestState{trace: mitmTrace{head: mitmTraceHead{
		id:             t.requestTotal.Add(1),
		host:           traceLabel(req.Host, 255),
		method:         traceLabel(req.Method, 24),
		protocol:       traceLabel(protocol, 16),
		connectionID:   httpctx.GetMITMConnectionID(req),
		chromeAutofill: protocol != "WebSocket" && isChromeAutofillRequest(req.Host, req.Method),
		stage:          mitmPipelineStageRequestProcessing,
		filter:         filter,
		startedAt:      now,
		stageAt:        now,
	}}}
	t.requests[req] = state
	// The original context outlives the child used for upstream I/O.
	state.stopCancel = context.AfterFunc(ctx, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.requests[req] == state {
			t.finishRequest(req, state, "canceled", mitmErrorCode(ctx.Err()))
		}
	})
	t.mu.Unlock()

	httpctx.SetUpstreamObserver(req, func(event httpctx.UpstreamEvent) {
		t.upstreamEvent(req, event)
	})
}

func (t *mitmPipelineTracker) observeFilterExclusion(req *http.Request, decision mitmFilterDecision) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.requests[req]; state != nil {
		head := &state.trace.head
		if decision == mitmFilterRequestExcluded || !head.filter.excluded() {
			head.filter = decision
		}
	}
}

func (t *mitmPipelineTracker) requestStage(req *http.Request, stage mitmPipelineStage) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		state.trace.transition(t.now(), stage)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) manualWaitStarted(req *http.Request, response bool) {
	stage := mitmPipelineStageManualRequest
	if response {
		stage = mitmPipelineStageManualResponse
	}
	t.requestStage(req, stage)
}

func (t *mitmPipelineTracker) manualWaitFinished(req *http.Request, response bool) {
	stage := mitmPipelineStageRequestProcessing
	if response {
		stage = mitmPipelineStageResponseProcessing
	}
	t.requestStage(req, stage)
}

func (t *mitmPipelineTracker) requestDispatched(req *http.Request, localResponse bool) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.requests[req]; state != nil {
		now := t.now()
		if localResponse {
			state.trace.transition(now, mitmPipelineStageLocalResponse)
			return
		}
		state.trace.transition(now, mitmPipelineStageUpstream)
		if state.dispatchedAt.IsZero() {
			state.dispatchedAt = now
			t.dispatchTotal.Add(1)
		}
	}
}

func (t *mitmPipelineTracker) upstreamCompleted(req *http.Request, succeeded bool, statusCode int) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.requests[req]; state != nil {
		if succeeded && !state.dispatchedAt.IsZero() && !state.upstreamDone {
			t.upstreamCompletedTotal.Add(1)
			state.upstreamDone = true
		}
		if statusCode > 0 && (succeeded || state.dispatchedAt.IsZero()) {
			state.trace.head.statusCode = int32(statusCode)
		}
		state.trace.transition(t.now(), mitmPipelineStageResponseProcessing)
	}
}

func (t *mitmPipelineTracker) responseMirrored(req *http.Request, decision mitmFilterDecision) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		t.responseMirroredTotal.Add(1)
		if decision != mitmFilterResponseExcluded || state.trace.head.filter != mitmFilterStaticJSExcluded {
			state.trace.head.filter = decision
		}
		state.trace.transition(t.now(), mitmPipelineStageResponseProcessing)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) responseProcessingFinished(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.requests[req]; state != nil {
		// The WebSocket mirror sees the upgrade response before the tunnel opens.
		if state.trace.head.protocol == "WebSocket" {
			return
		}
		t.finishRequest(req, state, "completed", "")
	}
}

func (t *mitmPipelineTracker) requestDropped(req *http.Request) {
	if t == nil || req == nil {
		return
	}
	t.mu.Lock()
	if state := t.requests[req]; state != nil {
		t.finishRequest(req, state, "dropped", "")
		t.droppedTotal.Add(1)
	}
	t.mu.Unlock()
}

func (t *mitmPipelineTracker) flowBuilt() {
	if t != nil {
		t.flowBuiltTotal.Add(1)
	}
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

func (t *mitmPipelineTracker) snapshot(dbQueueDepth, dbQueueCapacity int, taken int64) *ypb.MITMPipelineStats {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	now := t.now()
	diagnostics := &ypb.MITMPipelineDiagnostics{DatabaseWriteTakenUnfinished: taken}
	stats := &ypb.MITMPipelineStats{
		Version:                    4,
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
		Diagnostics:                diagnostics,
	}

	var counts, browserCounts, browserOldest, otherOldest [mitmPipelineStageCount]int64
	var filteredCounts, inScopeOtherCounts, inScopeOtherOldest [mitmPipelineStageCount]int64
	for _, state := range t.requests {
		head := &state.trace.head
		if head.stage == mitmPipelineStageWebSocketOnline {
			diagnostics.WebSocketOnline++
			continue
		}
		stats.ActiveTotal++
		age := ageMilliseconds(now, head.stageAt)
		if head.protocol != "WebSocket" {
			counts[head.stage]++
			if head.filter.excluded() {
				filteredCounts[head.stage]++
			}
			if head.chromeAutofill {
				browserCounts[head.stage]++
				updateOldestAge(&browserOldest[head.stage], age)
			} else {
				updateOldestAge(&otherOldest[head.stage], age)
				if !head.filter.excluded() {
					inScopeOtherCounts[head.stage]++
					updateOldestAge(&inScopeOtherOldest[head.stage], age)
				}
			}
		} else if !state.dispatchedAt.IsZero() {
			diagnostics.WebSocketHandshaking++
			handshakeAge := ageMilliseconds(now, state.dispatchedAt)
			updateOldestAge(&diagnostics.OldestWebSocketHandshakeAgeMs, handshakeAge)
			if !head.filter.excluded() {
				diagnostics.InScopeWebSocketHandshaking++
				updateOldestAge(&diagnostics.OldestInScopeWebSocketHandshakeAgeMs, handshakeAge)
			}
		}
		switch {
		case head.stage == mitmPipelineStageRequestProcessing:
			stats.PreDispatchActive++
			updateOldestAge(&stats.OldestPreDispatchAgeMs, age)
		case head.stage == mitmPipelineStageManualRequest || head.stage == mitmPipelineStageManualResponse:
			stats.ManualActive++
			updateOldestAge(&stats.OldestManualAgeMs, age)
		case head.stage.isUpstream():
			stats.UpstreamActive++
			updateOldestAge(&stats.OldestUpstreamAgeMs, ageMilliseconds(now, state.dispatchedAt))
		default:
			stats.ResponseProcessingActive++
			updateOldestAge(&stats.OldestResponseProcessingAgeMs, age)
		}
	}
	for stage := mitmPipelineStage(1); stage < mitmPipelineStageWebSocketOnline; stage++ {
		if counts[stage] > 0 {
			diagnostics.HTTPStages = append(diagnostics.HTTPStages, &ypb.MITMStageStats{
				Stage: mitmStageNames[stage], Count: counts[stage], OldestAgeMs: max(browserOldest[stage], otherOldest[stage]),
				BrowserBackgroundCount: browserCounts[stage], OldestBrowserBackgroundAgeMs: browserOldest[stage],
				OldestOtherAgeMs: otherOldest[stage],
				FilteredCount:    filteredCounts[stage], InScopeOtherCount: inScopeOtherCounts[stage],
				OldestInScopeOtherAgeMs: inScopeOtherOldest[stage],
			})
		}
	}
	var oldestWrite *mitmPersistState
	for _, write := range t.persists {
		if write.submittedAt.IsZero() {
			continue
		}
		stats.PersistActive++
		if write.trace != nil && (oldestWrite == nil || write.submittedAt.Before(oldestWrite.submittedAt)) {
			oldestWrite = write
		}
		updateOldestAge(&stats.OldestPersistAgeMs, ageMilliseconds(now, write.submittedAt))
		if !write.blockedAt.IsZero() {
			diagnostics.PersistBlocked++
			updateOldestAge(&diagnostics.OldestPersistBlockedAgeMs, ageMilliseconds(now, write.blockedAt))
		}
	}
	point := mitmStoragePoint{
		at: now.UnixMilli(), pending: stats.PersistActive, blocked: diagnostics.PersistBlocked,
		requests: stats.RequestTotal, dispatched: stats.DispatchTotal, upstreamCompleted: stats.UpstreamCompletedTotal,
		submitted: stats.PersistEnqueuedTotal, persisted: stats.PersistedTotal, persistFailed: stats.PersistFailedTotal,
	}
	if oldestWrite != nil {
		point.trace = oldestWrite.trace
	}
	slot := &t.history[now.Unix()%mitmHistorySeconds]
	if slot.at/1000 != now.Unix() || point.pending >= slot.pending {
		*slot = point
	}
	t.summarizeMetrics(now, diagnostics)
	return stats
}
