package yakgrpc

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/metadata"
)

type cancelableAIReActServerStream struct {
	ctx   context.Context
	recvC chan *ypb.AIInputEvent
}

func newCancelableAIReActServerStream(ctx context.Context, first *ypb.AIInputEvent) *cancelableAIReActServerStream {
	stream := &cancelableAIReActServerStream{ctx: ctx, recvC: make(chan *ypb.AIInputEvent, 2)}
	stream.recvC <- first
	return stream
}

func (s *cancelableAIReActServerStream) Send(*ypb.AIOutputEvent) error { return s.ctx.Err() }

func (s *cancelableAIReActServerStream) Recv() (*ypb.AIInputEvent, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case event, ok := <-s.recvC:
		if !ok {
			return nil, io.EOF
		}
		return event, nil
	}
}

func (s *cancelableAIReActServerStream) SetHeader(metadata.MD) error  { return nil }
func (s *cancelableAIReActServerStream) SendHeader(metadata.MD) error { return nil }
func (s *cancelableAIReActServerStream) SetTrailer(metadata.MD)       {}
func (s *cancelableAIReActServerStream) Context() context.Context     { return s.ctx }

func (s *cancelableAIReActServerStream) SendMsg(message any) error {
	event, ok := message.(*ypb.AIOutputEvent)
	if !ok {
		return utils.Errorf("unexpected StartAIReAct response type %T", message)
	}
	return s.Send(event)
}

func (s *cancelableAIReActServerStream) RecvMsg(message any) error {
	target, ok := message.(*ypb.AIInputEvent)
	if !ok {
		return utils.Errorf("unexpected StartAIReAct request type %T", message)
	}
	event, err := s.Recv()
	if err != nil {
		return err
	}
	*target = *event
	return nil
}

type noOpTimelineArchiveStore struct{}

func (*noOpTimelineArchiveStore) ArchiveCompressedBatch(context.Context, *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	return nil, nil
}

func (*noOpTimelineArchiveStore) SearchArchivedBatches(context.Context, *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	return &aicommon.TimelineArchiveSearchResult{}, nil
}

func TestStartAIReActFrontendStreamCancelReleasesFreeInput(t *testing.T) {
	server := newScheduleTestServer(t)
	sessionID := "frontend-stream-cancel-" + uuid.NewString()
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	stream := newCancelableAIReActServerStream(streamCtx, &ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			TimelineSessionID:    sessionID,
			DisableAISearchForge: true,
			DisableToolUse:       true,
		},
	})

	callbackStarted := make(chan struct{})
	callbackExited := make(chan struct{})
	var startOnce sync.Once
	var exitOnce sync.Once
	blockingCallback := func(config aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		startOnce.Do(func() { close(callbackStarted) })
		<-config.GetContext().Done()
		exitOnce.Do(func() { close(callbackExited) })
		return nil, config.GetContext().Err()
	}
	mockKnowledgeManager, _ := aicommon.NewMockEKManagerAndToken()
	streamDone := make(chan error, 1)
	go func() {
		streamDone <- server.startAIReActWithOptions(
			stream,
			false,
			aicommon.WithAICallback(blockingCallback),
			aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
			aicommon.WithTimelineArchiveStore(&noOpTimelineArchiveStore{}),
			aicommon.WithEnhanceKnowledgeManager(mockKnowledgeManager),
			aicommon.WithDisallowMCPServers(true),
			aicommon.WithDisableSessionTitleGeneration(true),
			aicommon.WithDisableIntentRecognition(true),
			aicommon.WithDisablePerception(true),
			aicommon.WithDisableAutoSkills(true),
			aicommon.WithGenerateReport(false),
			aicommon.WithDisableDynamicPlanning(true),
			aicommon.WithPeriodicVerificationInterval(0),
			aicommon.WithDisableIncreaseIteration(true),
		)
	}()

	running, err := aireact.WaitRunningSession(sessionID, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, running.SendInputEvent(&ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "wait until the frontend stream is cancelled",
	}))
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("free-input task did not reach the AI callback")
	}
	activeTask := running.GetCurrentTask()
	require.NotNil(t, activeTask)

	// Electron's stream.cancel() cancels the server stream context. The ReAct
	// config and its active free-input task must inherit that cancellation.
	cancelStream()
	select {
	case <-callbackExited:
	case <-time.After(2 * time.Second):
		t.Fatal("AI callback did not exit after stream cancellation")
	}
	select {
	case <-activeTask.GetContext().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("free-input task context was not cancelled")
	}
	select {
	case err := <-streamDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("StartAIReAct did not return after stream cancellation")
	}
	require.Eventually(t, func() bool { return running.GetCurrentTask() == nil }, 3*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		_, ok := aireact.GetRunningSession(sessionID)
		return !ok
	}, time.Second, 10*time.Millisecond)
	require.False(t, aireact.IsSessionBusy(sessionID))
	require.False(t, aireact.IsSessionStarting(sessionID))
}
