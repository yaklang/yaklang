//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const mitmSlowThreshold = 2 * time.Second
const mitmTraceSampleLimit = 128

type mitmTraceHead struct {
	id, flowID                  uint64
	host, method, protocol      string
	connectionID                string
	stage                       mitmPipelineStage
	chromeAutofill              bool
	outcome, errorCode          string
	statusCode                  int32
	startedAt, stageAt, endedAt time.Time
	attempts                    uint32
}

type mitmTraceSpan struct {
	stage              mitmPipelineStage
	startedAt, endedAt int64
	attempt            uint32
	protocol           string
}

type mitmTrace struct {
	head           mitmTraceHead
	spans          [20]mitmTraceSpan
	spanCount      int
	spansTruncated bool
	slow           bool
	lastAt         time.Time
	kept           uint8
}

func traceLabel(value string, max int) string {
	if len(value) > max {
		value = value[:max]
	}
	return strings.Clone(strings.ToValidUTF8(value, "�"))
}

// Match the known Chromium Autofill endpoint. The hostname alone does not identify the client.
func isChromeAutofillRequest(host, method string) bool {
	if method != http.MethodGet && method != http.MethodPost {
		return false
	}
	name, _, _ := strings.Cut(host, ":")
	return strings.EqualFold(strings.TrimSuffix(name, "."), "content-autofill.googleapis.com")
}

func (tr *mitmTrace) span(stage mitmPipelineStage, start, end time.Time, attempt uint32, protocol string) {
	if start.IsZero() {
		return
	}
	if stage != mitmPipelineStageWebSocketOnline && stage != mitmPipelineStageStreaming &&
		stage != mitmPipelineStageManualRequest && stage != mitmPipelineStageManualResponse &&
		end.Sub(start) >= mitmSlowThreshold {
		tr.slow = true
	}
	tr.lastAt = end
	span := mitmTraceSpan{stage: stage, startedAt: start.UnixMilli(), endedAt: end.UnixMilli(), attempt: attempt, protocol: protocol}
	if tr.spanCount == len(tr.spans) {
		// Keep the beginning and the latest stages even with frequent retries.
		copy(tr.spans[10:], tr.spans[11:])
		tr.spans[len(tr.spans)-1] = span
		tr.spansTruncated = true
		return
	}
	tr.spans[tr.spanCount] = span
	tr.spanCount++
}

func (tr *mitmTrace) endStage(now time.Time) {
	attempt, protocol := uint32(0), ""
	if tr.head.stage.isUpstream() {
		attempt, protocol = tr.head.attempts, tr.head.protocol
	}
	tr.span(tr.head.stage, tr.head.stageAt, now, attempt, protocol)
}

func (tr *mitmTrace) transition(now time.Time, stage mitmPipelineStage) {
	if tr.head.stage == stage {
		return
	}
	tr.endStage(now)
	tr.head.stage, tr.head.stageAt = stage, now
}

func (t *mitmPipelineTracker) upstreamEvent(req *http.Request, event httpctx.UpstreamEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.requests[req]
	if state == nil {
		return
	}
	tr := &state.trace
	head := &tr.head
	now := t.now()
	if event.Phase == httpctx.UpstreamAttempt {
		tr.endStage(now)
		head.stage, head.stageAt = mitmPipelineStageUpstream, now
		head.attempts++
		head.protocol = event.Protocol
		state.lastAttemptID = event.AttemptID
		return
	}
	if event.AttemptID != 0 && event.AttemptID != state.lastAttemptID {
		return
	}
	if event.StatusCode > 0 {
		head.statusCode = int32(event.StatusCode)
	}
	switch event.Phase {
	case httpctx.UpstreamPoolWait:
		tr.transition(now, mitmPipelineStagePoolWait)
	case httpctx.UpstreamDial:
		tr.transition(now, mitmPipelineStageDial)
	case httpctx.UpstreamSend:
		tr.transition(now, mitmPipelineStageSend)
	case httpctx.UpstreamFirstByte:
		// An early response may be observed by the read loop during upload.
		if head.stage == mitmPipelineStageSend {
			tr.transition(now, mitmPipelineStageFirstByte)
		}
	case httpctx.UpstreamRead:
		if head.stage.isUpstream() && head.stage != mitmPipelineStageStreaming {
			tr.transition(now, mitmPipelineStageRead)
		}
	case httpctx.UpstreamStreaming:
		if head.stage.isUpstream() {
			tr.transition(now, mitmPipelineStageStreaming)
		}
	case httpctx.UpstreamFailed:
		head.errorCode = mitmErrorCode(event.Err)
		head.outcome = "failed"
		if head.errorCode == "canceled" {
			head.outcome = "canceled"
		}
		if head.protocol == "WebSocket" {
			if head.outcome == "failed" {
				t.metricBucket(now).wsHandshakeFailed++
				if head.statusCode > 0 && head.errorCode == "upstream_error" {
					head.errorCode = "websocket_upgrade_failed"
				}
			}
			t.finishRequest(req, state, head.outcome, head.errorCode)
		}
	case httpctx.WebsocketOnline:
		head.statusCode = http.StatusSwitchingProtocols
		if !state.upstreamDone && !state.dispatchedAt.IsZero() {
			t.upstreamCompletedTotal.Add(1)
			state.upstreamDone = true
		}
		tr.transition(now, mitmPipelineStageWebSocketOnline)
	case httpctx.WebsocketClosed:
		outcome := "dropped"
		if head.stage == mitmPipelineStageWebSocketOnline {
			outcome = "closed"
			if event.Err != nil && !errors.Is(event.Err, io.EOF) && !errors.Is(event.Err, net.ErrClosed) {
				outcome = "failed"
			}
		}
		t.finishRequest(req, state, outcome, mitmErrorCode(event.Err))
	}
}

func (t *mitmPipelineTracker) finishRequest(req *http.Request, state *mitmPipelineRequestState, outcome, code string) {
	if state.stopCancel != nil {
		state.stopCancel()
		state.stopCancel = nil
	}
	delete(t.requests, req)
	tr := &state.trace
	now := t.now()
	tr.endStage(now)
	tr.head.endedAt = now
	if tr.head.outcome == "" {
		tr.head.outcome = outcome
		tr.head.errorCode = code
	}
	if tr.head.outcome == "completed" && tr.head.statusCode >= 400 {
		tr.head.outcome = "http_error"
	}
	t.retainTrace(tr)
}

func mitmErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var dns *net.DNSError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &network) && network.Timeout():
		return "timeout"
	case errors.As(err, &dns):
		return "dns"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "eof"
	default:
		return "upstream_error"
	}
}

const (
	mitmTraceSlow uint8 = 1 << iota
	mitmTraceFailed
)

type mitmTraceRing struct {
	samples [mitmTraceSampleLimit]*mitmTrace
	next    int
}

func (r *mitmTraceRing) add(tr *mitmTrace, flag uint8) {
	if tr.kept&flag != 0 {
		return
	}
	if old := r.samples[r.next]; old != nil {
		old.kept &^= flag
	}
	r.samples[r.next] = tr
	tr.kept |= flag
	r.next = (r.next + 1) % len(r.samples)
}

func (r *mitmTraceRing) expire(before time.Time, flag uint8) {
	for i, tr := range r.samples {
		if tr != nil && tr.lastAt.Before(before) {
			tr.kept &^= flag
			r.samples[i] = nil
		}
	}
}

func (t *mitmPipelineTracker) retainTrace(tr *mitmTrace) {
	if tr.slow {
		t.slow.add(tr, mitmTraceSlow)
	}
	switch tr.head.outcome {
	case "failed", "canceled", "http_error", "dropped", "persist_failed":
		t.failed.add(tr, mitmTraceFailed)
	}
}

func unixMsOrZero(ts time.Time) int64 {
	if ts.IsZero() {
		return 0
	}
	return ts.UnixMilli()
}

func (h mitmTraceHead) toProto() *ypb.MITMTraceEntry {
	entry := &ypb.MITMTraceEntry{
		TraceId: h.id, FlowId: h.flowID, Host: h.host, Method: h.method,
		Protocol: h.protocol, ConnectionId: h.connectionID,
		Stage: mitmStageNames[h.stage], Outcome: h.outcome, ErrorCode: h.errorCode,
		StartedAtUnixMs: h.startedAt.UnixMilli(), StageAtUnixMs: h.stageAt.UnixMilli(),
		FinishedAtUnixMs: unixMsOrZero(h.endedAt), Attempts: h.attempts, StatusCode: h.statusCode,
	}
	if h.chromeAutofill {
		entry.BrowserService = "chrome_autofill"
	}
	return entry
}

func (s mitmTraceSpan) toProto() *ypb.MITMTraceSpan {
	return &ypb.MITMTraceSpan{
		Stage: mitmStageNames[s.stage], StartedAtUnixMs: s.startedAt, FinishedAtUnixMs: s.endedAt,
		Attempt: s.attempt, Protocol: s.protocol,
	}
}
