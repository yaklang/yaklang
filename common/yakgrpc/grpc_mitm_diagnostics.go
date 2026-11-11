//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"sort"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func (s *Server) GetMITMDiagnostics(ctx context.Context, req *ypb.GetMITMDiagnosticsRequest) (*ypb.GetMITMDiagnosticsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcstatus.FromContextError(err).Err()
	}
	if req.GetSessionId() == "" || len(req.GetSessionId()) > 64 || len(req.GetConnectionId()) > 64 {
		return nil, grpcstatus.Error(codes.InvalidArgument, "invalid MITM session or connection ID")
	}
	if req.GetOffset() < 0 || req.GetLimit() < 0 || req.GetLimit() > 32 {
		return nil, grpcstatus.Error(codes.InvalidArgument, "invalid MITM trace page")
	}
	switch req.GetView() {
	case "storage", "active", "slow", "failed", "websocket":
	case "detail":
		if req.GetTraceId() == 0 {
			return nil, grpcstatus.Error(codes.InvalidArgument, "a MITM trace ID is required")
		}
	case "range":
		from, to := req.GetFromUnixMs(), req.GetToUnixMs()
		if from <= 0 || to < from || to-from > mitmHistorySeconds*1000 {
			return nil, grpcstatus.Error(codes.InvalidArgument, "invalid MITM trace time range")
		}
	default:
		return nil, grpcstatus.Error(codes.InvalidArgument, "unknown MITM diagnostics view")
	}
	value, ok := s.mitmDiagnostics.Load(req.GetSessionId())
	if !ok {
		return nil, grpcstatus.Error(codes.NotFound, "MITM session has ended")
	}
	return value.(*mitmPipelineTracker).diagnostics(req)
}

func (t *mitmPipelineTracker) diagnostics(req *ypb.GetMITMDiagnosticsRequest) (*ypb.GetMITMDiagnosticsResponse, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, grpcstatus.Error(codes.NotFound, "MITM session has ended")
	}
	now := t.now()
	response := &ypb.GetMITMDiagnosticsResponse{SessionId: t.sessionID, GeneratedAtUnixMs: now.UnixMilli()}
	switch req.GetView() {
	case "storage":
		cutoff := now.Add(-mitmHistorySeconds * time.Second).UnixMilli()
		for _, p := range t.history {
			if p.at > cutoff && p.at <= now.UnixMilli() {
				response.Storage = append(response.Storage, &ypb.MITMStorageSample{AtUnixMs: p.at, PersistPending: p.pending})
			}
		}
		t.mu.Unlock()
		sort.Slice(response.Storage, func(i, j int) bool { return response.Storage[i].AtUnixMs < response.Storage[j].AtUnixMs })
	case "detail":
		response.Detail = t.traceDetail(now, req.GetTraceId())
		t.mu.Unlock()
		if response.Detail == nil {
			return nil, grpcstatus.Error(codes.NotFound, "MITM trace is no longer available")
		}
	default:
		rows := t.traceRows(now, req)
		t.mu.Unlock()
		sort.Slice(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if req.GetView() == "active" || req.GetView() == "websocket" {
				if !a.stageAt.Equal(b.stageAt) {
					return a.stageAt.Before(b.stageAt)
				}
			} else if !a.endedAt.Equal(b.endedAt) {
				return a.endedAt.After(b.endedAt)
			}
			return a.id > b.id
		})
		response.Total = int32(len(rows))
		limit := int(req.GetLimit())
		if limit == 0 {
			limit = 20
		}
		from := min(int(req.GetOffset()), len(rows))
		for _, row := range rows[from:min(from+limit, len(rows))] {
			response.Traces = append(response.Traces, row.toProto())
		}
	}
	return response, nil
}

func (t *mitmPipelineTracker) traceRows(now time.Time, req *ypb.GetMITMDiagnosticsRequest) []mitmTraceHead {
	view := req.GetView()
	candidates := make(map[uint64]mitmTraceHead)
	matches := func(tr *mitmTrace) bool {
		if id := req.GetConnectionId(); id != "" && id != tr.head.connectionID {
			return false
		}
		if view == "range" {
			from, to := req.GetFromUnixMs(), req.GetToUnixMs()
			if at := tr.head.startedAt.UnixMilli(); at >= from && at <= to {
				return true
			}
			for _, span := range tr.spans[:tr.spanCount] {
				if span.stage >= mitmPipelineStagePersistSubmit && span.startedAt <= to && span.endedAt >= from {
					return true
				}
			}
			return false
		}
		return true
	}
	if view == "active" || view == "websocket" || view == "range" {
		for _, state := range t.requests {
			tr := &state.trace
			stage := tr.head.stage
			online := stage == mitmPipelineStageWebSocketOnline
			blocked := !online && stage != mitmPipelineStageStreaming && now.Sub(tr.head.stageAt) >= mitmSlowThreshold
			if matches(tr) && (view == "range" || view == "active" && blocked || view == "websocket" && online) {
				candidates[tr.head.id] = tr.head
			}
		}
	}
	if view == "active" || view == "range" {
		for _, write := range t.persists {
			if write.trace == nil || write.submittedAt.IsZero() {
				continue
			}
			tr := write.trace
			// A write can still be pending after the response is finished.
			if id := req.GetConnectionId(); id != "" && id != tr.head.connectionID {
				continue
			}
			if view == "range" {
				if write.submittedAt.UnixMilli() > req.GetToUnixMs() || now.UnixMilli() < req.GetFromUnixMs() {
					continue
				}
			} else if now.Sub(write.submittedAt) < mitmSlowThreshold {
				continue
			}
			head := tr.head
			head.stage, head.stageAt = write.stage()
			head.endedAt = time.Time{}
			candidates[head.id] = head
		}
	}
	addSamples := func(ring *mitmTraceRing) {
		cutoff := now.Add(-mitmHistorySeconds * time.Second)
		for _, tr := range ring.samples {
			if tr != nil && !tr.lastAt.Before(cutoff) && matches(tr) {
				if _, exists := candidates[tr.head.id]; !exists {
					candidates[tr.head.id] = tr.head
				}
			}
		}
	}
	if view == "slow" || view == "range" {
		addSamples(&t.slow)
	}
	if view == "failed" || view == "range" {
		addSamples(&t.failed)
	}
	if view == "range" {
		cutoff := now.Add(-mitmHistorySeconds * time.Second).UnixMilli()
		for _, point := range t.history {
			tr := point.trace
			if tr != nil && point.at > cutoff && point.at >= req.GetFromUnixMs() && point.at <= req.GetToUnixMs() &&
				(req.GetConnectionId() == "" || req.GetConnectionId() == tr.head.connectionID) {
				if _, exists := candidates[tr.head.id]; !exists {
					candidates[tr.head.id] = tr.head
				}
			}
		}
	}
	rows := make([]mitmTraceHead, 0, len(candidates))
	for _, head := range candidates {
		rows = append(rows, head)
	}
	return rows
}

func (t *mitmPipelineTracker) traceDetail(now time.Time, id uint64) *ypb.MITMTraceEntry {
	var trace *mitmTrace
	var active []*ypb.MITMTraceSpan
	var writeStage mitmPipelineStage
	var writeAt time.Time
	for _, state := range t.requests {
		if state.trace.head.id == id {
			trace = &state.trace
			head := &trace.head
			span := mitmTraceSpan{stage: head.stage, startedAt: head.stageAt.UnixMilli()}
			if head.stage.isUpstream() {
				span.attempt, span.protocol = head.attempts, head.protocol
			}
			active = append(active, span.toProto())
			break
		}
	}
	for _, write := range t.persists {
		if write.trace != nil && write.trace.head.id == id && !write.submittedAt.IsZero() {
			trace = write.trace
			writeStage, writeAt = write.stage()
			active = append(active, (&mitmTraceSpan{stage: writeStage, startedAt: writeAt.UnixMilli()}).toProto())
		}
	}
	if trace == nil {
		for _, ring := range []*mitmTraceRing{&t.slow, &t.failed} {
			for _, tr := range ring.samples {
				if tr != nil && tr.head.id == id && now.Sub(tr.lastAt) < mitmHistorySeconds*time.Second {
					trace = tr
					break
				}
			}
			if trace != nil {
				break
			}
		}
	}
	if trace == nil {
		cutoff := now.Add(-mitmHistorySeconds * time.Second).UnixMilli()
		for _, point := range t.history {
			if point.trace != nil && point.trace.head.id == id && point.at > cutoff {
				trace = point.trace
				break
			}
		}
	}
	if trace == nil {
		return nil
	}
	head := trace.head
	if writeStage != 0 && head.stage != mitmPipelineStageWebSocketOnline {
		head.stage, head.stageAt = writeStage, writeAt
		head.endedAt = time.Time{}
	}
	detail := head.toProto()
	detail.Spans = make([]*ypb.MITMTraceSpan, 0, trace.spanCount+len(active))
	for i := 0; i < trace.spanCount; i++ {
		detail.Spans = append(detail.Spans, trace.spans[i].toProto())
	}
	detail.Spans = append(detail.Spans, active...)
	sort.SliceStable(detail.Spans, func(i, j int) bool {
		return detail.Spans[i].StartedAtUnixMs < detail.Spans[j].StartedAtUnixMs
	})
	detail.SpansTruncated = trace.spansTruncated
	return detail
}
