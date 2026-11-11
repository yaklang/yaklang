//go:build !yakit_exclude

package yakgrpc

import (
	"math/bits"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	mitmHistorySeconds = 180
	mitmMetricSeconds  = 60
	mitmHistogramBins  = 25
)

type mitmStoragePoint struct {
	at, pending, blocked                    int64
	requests, dispatched, upstreamCompleted uint64
	submitted, persisted, persistFailed     uint64
	trace                                   *mitmTrace
}

type mitmLatencyHistogram [mitmHistogramBins]uint64

func (h *mitmLatencyHistogram) observe(elapsed time.Duration) {
	// Buckets are inclusive upper bounds of 0, 1, 2, 4, 8, ... ms.
	ms := uint64(max(0, elapsed.Round(time.Millisecond).Milliseconds()))
	if ms == 0 {
		h[0]++
		return
	}
	h[min(1+bits.Len64(ms-1), len(h)-1)]++
}

func (h *mitmLatencyHistogram) p95() (int64, uint64) {
	var total uint64
	for _, count := range h {
		total += count
	}
	if total == 0 {
		return 0, 0
	}
	target := total - total/20
	var cumulative uint64
	for i, count := range h {
		cumulative += count
		if cumulative >= target {
			if i == 0 {
				return 0, total
			}
			return 1 << (i - 1), total
		}
	}
	return 0, total
}

type mitmMetricsBucket struct {
	second            int64
	submitToStart     mitmLatencyHistogram
	queueWait         mitmLatencyHistogram
	write             mitmLatencyHistogram
	wsHandshakeFailed uint64
}

func (t *mitmPipelineTracker) metricBucket(now time.Time) *mitmMetricsBucket {
	second := now.Unix()
	b := &t.metrics[second%mitmMetricSeconds]
	if b.second != second {
		*b = mitmMetricsBucket{second: second}
	}
	return b
}

func (t *mitmPipelineTracker) summarizeMetrics(now time.Time, d *ypb.MITMPipelineDiagnostics) {
	var wait, queue, write mitmLatencyHistogram
	for i := range t.metrics {
		b := &t.metrics[i]
		if now.Unix()-b.second >= mitmMetricSeconds || b.second > now.Unix() {
			continue
		}
		d.WebSocketHandshakeFailedLastMinute += b.wsHandshakeFailed
		for bin := range wait {
			wait[bin] += b.submitToStart[bin]
			queue[bin] += b.queueWait[bin]
			write[bin] += b.write[bin]
		}
	}
	d.PersistSubmitToStartP95Ms, d.PersistSubmitToStartSamples = wait.p95()
	d.PersistWriteP95Ms, d.PersistWriteSamples = write.p95()
	d.PersistQueueWaitP95Ms, d.PersistQueueWaitSamples = queue.p95()
	cutoff := now.Add(-mitmHistorySeconds * time.Second)
	t.slow.expire(cutoff, mitmTraceSlow)
	t.failed.expire(cutoff, mitmTraceFailed)
}

type mitmPersistState struct {
	trace       *mitmTrace
	submittedAt time.Time
	blockedAt   time.Time
	acceptedAt  time.Time
	startedAt   time.Time
}

func (w *mitmPersistState) stage() (mitmPipelineStage, time.Time) {
	switch {
	case !w.startedAt.IsZero():
		return mitmPipelineStagePersistWrite, w.startedAt
	case !w.blockedAt.IsZero():
		return mitmPipelineStagePersistBlocked, w.blockedAt
	case !w.acceptedAt.IsZero():
		return mitmPipelineStagePersistQueued, w.acceptedAt
	default:
		return mitmPipelineStagePersistSubmit, w.submittedAt
	}
}

func (w *mitmPersistState) endStage(now time.Time) {
	if w.trace != nil {
		stage, at := w.stage()
		w.trace.span(stage, at, now, 0, "")
	}
}

func (t *mitmPipelineTracker) writeObserver(req *http.Request) yakit.HTTPFlowWriteObserver {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	write := &mitmPersistState{}
	if state := t.requests[req]; state != nil {
		write.trace = &state.trace
	}
	t.nextWriteID++
	id := t.nextWriteID
	t.persists[id] = write
	// Queued callbacks retain the lookup ID. Closing the session releases the
	// trace and request references even if the shared writer is still busy.
	return func(stage yakit.HTTPFlowWriteStage, flowID uint, err error) {
		t.observeWrite(id, stage, flowID, err)
	}
}

func (t *mitmPipelineTracker) observeWrite(id uint64, stage yakit.HTTPFlowWriteStage, flowID uint, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.persists[id]
	if w == nil {
		return
	}
	now := t.now()
	switch stage {
	case yakit.HTTPFlowWriteSubmitting:
		w.submittedAt = now
		t.persistEnqueuedTotal.Add(1)
	case yakit.HTTPFlowWriteBlocked:
		w.blockedAt = now
	case yakit.HTTPFlowWriteAccepted:
		if w.startedAt.IsZero() {
			w.endStage(now)
			w.acceptedAt = now
			w.blockedAt = time.Time{}
		}
	case yakit.HTTPFlowWriteStarted:
		w.endStage(now)
		w.startedAt = now
		w.blockedAt = time.Time{}
		bucket := t.metricBucket(now)
		bucket.submitToStart.observe(now.Sub(w.submittedAt))
		if !w.acceptedAt.IsZero() {
			bucket.queueWait.observe(now.Sub(w.acceptedAt))
		}
	case yakit.HTTPFlowWriteFinished:
		w.endStage(now)
		delete(t.persists, id)
		if !w.startedAt.IsZero() {
			t.metricBucket(now).write.observe(now.Sub(w.startedAt))
		}
		if err == nil && flowID > 0 {
			t.persistedTotal.Add(1)
		} else {
			t.persistFailedTotal.Add(1)
		}
		if w.trace != nil {
			if flowID > 0 {
				w.trace.head.flowID = uint64(flowID)
			} else if w.trace.head.errorCode == "" {
				w.trace.head.outcome = "persist_failed"
				w.trace.head.errorCode = "save_failed"
			}
			w.trace.slow = w.trace.slow || now.Sub(w.submittedAt) >= mitmSlowThreshold
			t.retainTrace(w.trace)
		}
	}
}
