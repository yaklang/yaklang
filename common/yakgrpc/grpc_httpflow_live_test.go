package yakgrpc

import (
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSubscribeHTTPFlowsRejectsOversizedSessionID(t *testing.T) {
	stream := &recordingHTTPFlowLiveStream{ctx: context.Background()}
	err := (&Server{}).SubscribeHTTPFlows(&ypb.SubscribeHTTPFlowsRequest{
		SessionId: strings.Repeat("s", httpFlowLiveMaxSessionIDBytes+1),
	}, stream)

	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	require.Empty(t, stream.events)
}

func TestSendHTTPFlowLiveRecordUsesBodyFreeSchema(t *testing.T) {
	stream := &recordingHTTPFlowLiveStream{ctx: context.Background()}
	flow := schema.HTTPFlow{
		Url:         "https://example.test/path",
		Method:      "POST",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
		Request:     "request-body-must-not-cross-the-stream",
		Response:    "response-body-must-not-cross-the-stream",
		BodyLength:  4096,
		StatusCode:  200,
		ContentType: "text/html",
	}
	flow.ID = 42
	record := yakit.HTTPFlowLiveRecord{
		Sequence:                8,
		ProjectGeneration:       3,
		DatabaseIdentity:        "project-identity",
		RequestHijackAtUnixMs:   100,
		ResponseMirrorAtUnixMs:  110,
		FlowBuiltAtUnixMs:       120,
		PersistEnqueuedAtUnixMs: 130,
		PersistStartedAtUnixMs:  140,
		CommittedAtUnixMs:       150,
		HighWaterID:             42,
		Flow:                    flow,
	}

	require.NoError(t, sendHTTPFlowLiveRecord(stream, "session-a", record, true))
	require.Len(t, stream.events, 1)
	event := stream.events[0]
	require.Equal(t, ypb.HTTPFlowLiveEventType_HTTP_FLOW_LIVE_EVENT_TYPE_COMMITTED, event.GetType())
	require.Equal(t, uint64(8), event.GetSequence())
	require.Equal(t, uint64(42), event.GetHighWaterId())
	require.Equal(t, "session-a", event.GetSessionId())
	require.True(t, event.GetReplayed())
	require.NotNil(t, event.GetFlow())
	require.Equal(t, uint64(42), event.GetFlow().GetId())
	require.Nil(t, event.GetFlow().ProtoReflect().Descriptor().Fields().ByName("Request"))
	require.Nil(t, event.GetFlow().ProtoReflect().Descriptor().Fields().ByName("Response"))
	require.Equal(t, int64(4096), event.GetFlow().GetBodyLength())
	require.Equal(t, int64(100), event.GetRequestHijackAtUnixMs())
	require.Equal(t, int64(110), event.GetResponseMirrorAtUnixMs())
	require.Equal(t, int64(120), event.GetFlowBuiltAtUnixMs())
	require.Equal(t, int64(130), event.GetPersistEnqueuedAtUnixMs())
	require.Equal(t, int64(140), event.GetPersistStartedAtUnixMs())
	require.Equal(t, int64(150), event.GetCommittedAtUnixMs())
}

func TestSendHTTPFlowLiveGapIsStructured(t *testing.T) {
	stream := &recordingHTTPFlowLiveStream{ctx: context.Background()}
	req := &ypb.SubscribeHTTPFlowsRequest{SessionId: "session-gap", LastSeenSequence: 9}
	require.NoError(t, sendHTTPFlowLiveGap(
		stream,
		req,
		"current-project",
		5,
		yakit.HTTPFlowLiveState{OldestAvailableSequence: 12, LatestSequence: 20, HighWaterID: 88},
		&yakit.HTTPFlowLiveGap{Reason: yakit.HTTPFlowLiveGapReplayWindowExceeded, RequestedSequence: 9},
	))

	require.Len(t, stream.events, 1)
	event := stream.events[0]
	require.Equal(t, ypb.HTTPFlowLiveEventType_HTTP_FLOW_LIVE_EVENT_TYPE_GAP, event.GetType())
	require.Equal(t, "current-project", event.GetDatabaseIdentity())
	require.Equal(t, uint64(5), event.GetProjectGeneration())
	require.Equal(t, uint64(88), event.GetHighWaterId())
	require.Equal(t, ypb.HTTPFlowLiveGapReason_HTTP_FLOW_LIVE_GAP_REASON_REPLAY_WINDOW_EXCEEDED, event.GetGap().GetReason())
	require.Equal(t, uint64(12), event.GetGap().GetOldestAvailableSequence())
	require.Equal(t, uint64(20), event.GetGap().GetLatestSequence())
}

func TestHTTPFlowLiveDeliveryCursorDoesNotOvertakeQueuedRecords(t *testing.T) {
	cursor := newHTTPFlowLiveDeliveryCursor(yakit.HTTPFlowLiveState{
		OldestAvailableSequence: 1,
		LatestSequence:          8,
		HighWaterID:             42,
	})

	state := cursor.heartbeatState(yakit.HTTPFlowLiveState{
		OldestAvailableSequence: 1,
		LatestSequence:          12,
		HighWaterID:             46,
	})
	require.Equal(t, uint64(8), state.LatestSequence)
	require.Equal(t, uint64(42), state.HighWaterID)

	cursor.observe(yakit.HTTPFlowLiveRecord{Sequence: 9, HighWaterID: 43})
	state = cursor.heartbeatState(yakit.HTTPFlowLiveState{
		OldestAvailableSequence: 1,
		LatestSequence:          12,
		HighWaterID:             46,
	})
	require.Equal(t, uint64(9), state.LatestSequence)
	require.Equal(t, uint64(43), state.HighWaterID)
}

func TestHTTPFlowLiveFilterRejectsSilentSemanticMismatch(t *testing.T) {
	require.True(t, isSupportedHTTPFlowLiveFilter(nil))
	require.True(t, isSupportedHTTPFlowLiveFilter(&ypb.HTTPFlowLiveFilter{}))
	require.True(t, isSupportedHTTPFlowLiveFilter(&ypb.HTTPFlowLiveFilter{SourceType: "mitm"}))
	require.False(t, isSupportedHTTPFlowLiveFilter(&ypb.HTTPFlowLiveFilter{SourceType: "scan"}))
}

type recordingHTTPFlowLiveStream struct {
	ctx    context.Context
	events []*ypb.HTTPFlowLiveEvent
}

func (s *recordingHTTPFlowLiveStream) Send(event *ypb.HTTPFlowLiveEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingHTTPFlowLiveStream) SetHeader(metadata.MD) error  { return nil }
func (s *recordingHTTPFlowLiveStream) SendHeader(metadata.MD) error { return nil }
func (s *recordingHTTPFlowLiveStream) SetTrailer(metadata.MD)       {}
func (s *recordingHTTPFlowLiveStream) Context() context.Context     { return s.ctx }
func (s *recordingHTTPFlowLiveStream) SendMsg(any) error            { return nil }
func (s *recordingHTTPFlowLiveStream) RecvMsg(any) error            { return io.EOF }
