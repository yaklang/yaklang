package yakgrpc

import (
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	httpFlowLiveHeartbeatInterval = 2 * time.Second
	httpFlowLiveMaxSessionIDBytes = 128
)

func (s *Server) SubscribeHTTPFlows(
	req *ypb.SubscribeHTTPFlowsRequest,
	stream ypb.Yak_SubscribeHTTPFlowsServer,
) error {
	if req == nil {
		req = &ypb.SubscribeHTTPFlowsRequest{}
	}
	if len(req.GetSessionId()) > httpFlowLiveMaxSessionIDBytes {
		return grpcstatus.Error(codes.InvalidArgument, "HTTP flow live session id exceeds 128 bytes")
	}
	binding := consts.CaptureProjectDatabaseBinding()
	databaseIdentity := yakit.HTTPFlowDatabaseIdentity(binding.Path)
	protocolVersion := req.GetProtocolVersion()
	if protocolVersion == 0 {
		protocolVersion = yakit.HTTPFlowLiveProtocolVersion
	}

	if protocolVersion != yakit.HTTPFlowLiveProtocolVersion {
		return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, yakit.SnapshotHTTPFlowLive(databaseIdentity, binding.Generation), &yakit.HTTPFlowLiveGap{
			Reason:            "unsupported_protocol",
			RequestedSequence: req.GetLastSeenSequence(),
		})
	}
	if req.GetDatabaseIdentity() == "" || req.GetProjectGeneration() == 0 {
		return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, yakit.SnapshotHTTPFlowLive(databaseIdentity, binding.Generation), &yakit.HTTPFlowLiveGap{
			Reason:            "bootstrap_required",
			RequestedSequence: req.GetLastSeenSequence(),
		})
	}
	if req.GetDatabaseIdentity() != databaseIdentity || req.GetProjectGeneration() != binding.Generation {
		return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, yakit.SnapshotHTTPFlowLive(databaseIdentity, binding.Generation), &yakit.HTTPFlowLiveGap{
			Reason:            "project_changed",
			RequestedSequence: req.GetLastSeenSequence(),
		})
	}
	if !isSupportedHTTPFlowLiveFilter(req.GetFilter()) {
		return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, yakit.SnapshotHTTPFlowLive(databaseIdentity, binding.Generation), &yakit.HTTPFlowLiveGap{
			Reason:            "unsupported_filter",
			RequestedSequence: req.GetLastSeenSequence(),
		})
	}

	subscription, replay, gap := yakit.SubscribeHTTPFlowLive(
		databaseIdentity,
		binding.Generation,
		req.GetLastSeenSequence(),
		req.GetLastSeenId(),
	)
	if gap != nil {
		return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, subscriptionState(subscription), gap)
	}
	defer subscription.Close()
	delivery := newHTTPFlowLiveDeliveryCursor(subscription.InitialState())

	for _, record := range replay {
		if err := sendHTTPFlowLiveRecord(stream, req.GetSessionId(), record, true); err != nil {
			return err
		}
	}
	if err := sendHTTPFlowLiveHeartbeat(stream, req.GetSessionId(), databaseIdentity, binding.Generation, delivery.heartbeatState(subscription.Snapshot())); err != nil {
		return err
	}

	ticker := time.NewTicker(httpFlowLiveHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case record := <-subscription.Events():
			if err := sendHTTPFlowLiveRecord(stream, req.GetSessionId(), record, false); err != nil {
				return err
			}
			delivery.observe(record)
		case liveGap := <-subscription.Gaps():
			return sendHTTPFlowLiveGap(stream, req, databaseIdentity, binding.Generation, subscription.Snapshot(), &liveGap)
		case <-ticker.C:
			current := consts.CaptureProjectDatabaseBinding()
			currentIdentity := yakit.HTTPFlowDatabaseIdentity(current.Path)
			if currentIdentity != databaseIdentity || current.Generation != binding.Generation {
				return sendHTTPFlowLiveGap(stream, req, currentIdentity, current.Generation, yakit.SnapshotHTTPFlowLive(currentIdentity, current.Generation), &yakit.HTTPFlowLiveGap{
					Reason:            "project_changed",
					RequestedSequence: req.GetLastSeenSequence(),
				})
			}
			if err := sendHTTPFlowLiveHeartbeat(stream, req.GetSessionId(), databaseIdentity, binding.Generation, delivery.heartbeatState(subscription.Snapshot())); err != nil {
				return err
			}
		}
	}
}

type httpFlowLiveDeliveryCursor struct {
	latestSequence uint64
	highWaterID    uint64
}

func newHTTPFlowLiveDeliveryCursor(initial yakit.HTTPFlowLiveState) *httpFlowLiveDeliveryCursor {
	return &httpFlowLiveDeliveryCursor{
		latestSequence: initial.LatestSequence,
		highWaterID:    initial.HighWaterID,
	}
}

func (c *httpFlowLiveDeliveryCursor) observe(record yakit.HTTPFlowLiveRecord) {
	if c == nil {
		return
	}
	c.latestSequence = record.Sequence
	c.highWaterID = record.HighWaterID
}

func (c *httpFlowLiveDeliveryCursor) heartbeatState(broker yakit.HTTPFlowLiveState) yakit.HTTPFlowLiveState {
	if c == nil {
		return yakit.HTTPFlowLiveState{}
	}
	broker.LatestSequence = c.latestSequence
	broker.HighWaterID = c.highWaterID
	return broker
}

func sendHTTPFlowLiveRecord(
	stream ypb.Yak_SubscribeHTTPFlowsServer,
	sessionID string,
	record yakit.HTTPFlowLiveRecord,
	replayed bool,
) error {
	return stream.Send(&ypb.HTTPFlowLiveEvent{
		ProtocolVersion:         yakit.HTTPFlowLiveProtocolVersion,
		Type:                    ypb.HTTPFlowLiveEventType_HTTP_FLOW_LIVE_EVENT_TYPE_COMMITTED,
		Sequence:                record.Sequence,
		ProjectGeneration:       record.ProjectGeneration,
		DatabaseIdentity:        record.DatabaseIdentity,
		ServerAtUnixMs:          time.Now().UnixMilli(),
		CommittedAtUnixMs:       record.CommittedAtUnixMs,
		HighWaterId:             record.HighWaterID,
		Flow:                    toHTTPFlowLiveSummary(&record.Flow),
		SessionId:               sessionID,
		Replayed:                replayed,
		RequestHijackAtUnixMs:   record.RequestHijackAtUnixMs,
		ResponseMirrorAtUnixMs:  record.ResponseMirrorAtUnixMs,
		FlowBuiltAtUnixMs:       record.FlowBuiltAtUnixMs,
		PersistEnqueuedAtUnixMs: record.PersistEnqueuedAtUnixMs,
		PersistStartedAtUnixMs:  record.PersistStartedAtUnixMs,
	})
}

func sendHTTPFlowLiveHeartbeat(
	stream ypb.Yak_SubscribeHTTPFlowsServer,
	sessionID string,
	databaseIdentity string,
	projectGeneration uint64,
	state yakit.HTTPFlowLiveState,
) error {
	return stream.Send(&ypb.HTTPFlowLiveEvent{
		ProtocolVersion:   yakit.HTTPFlowLiveProtocolVersion,
		Type:              ypb.HTTPFlowLiveEventType_HTTP_FLOW_LIVE_EVENT_TYPE_HEARTBEAT,
		Sequence:          state.LatestSequence,
		ProjectGeneration: projectGeneration,
		DatabaseIdentity:  databaseIdentity,
		ServerAtUnixMs:    time.Now().UnixMilli(),
		HighWaterId:       state.HighWaterID,
		SessionId:         sessionID,
	})
}

func sendHTTPFlowLiveGap(
	stream ypb.Yak_SubscribeHTTPFlowsServer,
	req *ypb.SubscribeHTTPFlowsRequest,
	databaseIdentity string,
	projectGeneration uint64,
	state yakit.HTTPFlowLiveState,
	gap *yakit.HTTPFlowLiveGap,
) error {
	if gap == nil {
		gap = &yakit.HTTPFlowLiveGap{}
	}
	if gap.OldestAvailableSequence == 0 {
		gap.OldestAvailableSequence = state.OldestAvailableSequence
	}
	if gap.LatestSequence == 0 {
		gap.LatestSequence = state.LatestSequence
	}
	if gap.HighWaterID == 0 {
		gap.HighWaterID = state.HighWaterID
	}
	return stream.Send(&ypb.HTTPFlowLiveEvent{
		ProtocolVersion:   yakit.HTTPFlowLiveProtocolVersion,
		Type:              ypb.HTTPFlowLiveEventType_HTTP_FLOW_LIVE_EVENT_TYPE_GAP,
		Sequence:          gap.LatestSequence,
		ProjectGeneration: projectGeneration,
		DatabaseIdentity:  databaseIdentity,
		ServerAtUnixMs:    time.Now().UnixMilli(),
		HighWaterId:       gap.HighWaterID,
		SessionId:         req.GetSessionId(),
		Gap: &ypb.HTTPFlowLiveGap{
			Reason:                  httpFlowLiveGapReasonToProto(gap.Reason),
			RequestedSequence:       gap.RequestedSequence,
			OldestAvailableSequence: gap.OldestAvailableSequence,
			LatestSequence:          gap.LatestSequence,
			HighWaterId:             gap.HighWaterID,
		},
	})
}

func isSupportedHTTPFlowLiveFilter(filter *ypb.HTTPFlowLiveFilter) bool {
	if filter == nil {
		return true
	}
	sourceType := strings.TrimSpace(filter.GetSourceType())
	return sourceType == "" || sourceType == schema.HTTPFlow_SourceType_MITM
}

func toHTTPFlowLiveSummary(flow *schema.HTTPFlow) *ypb.HTTPFlowLiveSummary {
	if flow == nil {
		return nil
	}
	utf8Safe := func(value string) string {
		return utils.EscapeInvalidUTF8Byte([]byte(value))
	}
	byteSize := func(size int64) string {
		if size < 0 {
			size = 0
		}
		return utf8Safe(utils.ByteSize(uint64(size)))
	}

	host, port, _ := utils.ParseStringToHostPort(flow.Url)
	hostPort := utils.HostPort(host, port)
	if host == "" {
		host = flow.Host
	}
	htmlTitle := ""
	if flow.HtmlTitle.Valid {
		htmlTitle = flow.HtmlTitle.String
	}
	createdAt := flow.CreatedAt.Unix()
	updatedAt := flow.UpdatedAt.Unix()
	if flow.CreatedAt.IsZero() {
		createdAt = 0
	}
	if flow.UpdatedAt.IsZero() {
		updatedAt = 0
	}

	return &ypb.HTTPFlowLiveSummary{
		Id:                         uint64(flow.ID),
		IsHTTPS:                    flow.IsHTTPS,
		Url:                        utf8Safe(flow.Url),
		SourceType:                 utf8Safe(flow.SourceType),
		Path:                       utf8Safe(flow.Path),
		Method:                     utf8Safe(flow.Method),
		BodyLength:                 flow.BodyLength,
		BodySizeVerbose:            byteSize(flow.BodyLength),
		RequestLength:              flow.RequestLength,
		RequestSizeVerbose:         byteSize(flow.RequestLength),
		ContentType:                utf8Safe(flow.ContentType),
		StatusCode:                 flow.StatusCode,
		GetParamsTotal:             int64(flow.GetParamsTotal),
		PostParamsTotal:            int64(flow.PostParamsTotal),
		CookieParamsTotal:          int64(flow.CookieParamsTotal),
		UpdatedAt:                  updatedAt,
		CreatedAt:                  createdAt,
		Hash:                       utf8Safe(flow.Hash),
		HostPort:                   utf8Safe(hostPort),
		IPAddress:                  utf8Safe(flow.IPAddress),
		HtmlTitle:                  utf8Safe(htmlTitle),
		Tags:                       utf8Safe(flow.Tags),
		NoFixContentLength:         flow.NoFixContentLength,
		IsWebsocket:                flow.IsWebsocket,
		WebsocketHash:              utf8Safe(flow.WebsocketHash),
		IsReadTooSlowResponse:      flow.IsReadTooSlowResponse,
		IsTooLargeResponse:         flow.IsTooLargeResponse,
		TooLargeResponseHeaderFile: utf8Safe(flow.TooLargeResponseHeaderFile),
		TooLargeResponseBodyFile:   utf8Safe(flow.TooLargeResponseBodyFile),
		DurationMs:                 flow.Duration / int64(time.Millisecond),
		HiddenIndex:                utf8Safe(flow.HiddenIndex),
		FromPlugin:                 utf8Safe(flow.FromPlugin),
		Host:                       utf8Safe(host),
		PathSuffix:                 utf8Safe(flow.PathSuffix),
		IsTooLargeRequest:          flow.IsTooLargeRequest,
		TooLargeRequestHeaderFile:  utf8Safe(flow.TooLargeRequestHeaderFile),
		TooLargeRequestBodyFile:    utf8Safe(flow.TooLargeRequestBodyFile),
		IsRequestOversize:          flow.IsRequestOversize || flow.IsTooLargeRequest,
	}
}

func httpFlowLiveGapReasonToProto(reason yakit.HTTPFlowLiveGapReason) ypb.HTTPFlowLiveGapReason {
	switch reason {
	case "bootstrap_required":
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_BOOTSTRAP_REQUIRED
	case "project_changed":
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_PROJECT_CHANGED
	case yakit.HTTPFlowLiveGapReplayWindowExceeded:
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_REPLAY_WINDOW_EXCEEDED
	case yakit.HTTPFlowLiveGapSlowConsumer:
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_SLOW_CONSUMER
	case yakit.HTTPFlowLiveGapCursorAhead:
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_CURSOR_AHEAD
	case "unsupported_protocol":
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_UNSUPPORTED_PROTOCOL
	case "unsupported_filter":
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_UNSUPPORTED_FILTER
	case yakit.HTTPFlowLiveGapProjectEvicted:
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_PROJECT_EVICTED
	default:
		return ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_UNSPECIFIED
	}
}

func subscriptionState(subscription *yakit.HTTPFlowLiveSubscription) yakit.HTTPFlowLiveState {
	if subscription == nil {
		return yakit.HTTPFlowLiveState{}
	}
	return subscription.Snapshot()
}
