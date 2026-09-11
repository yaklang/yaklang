package yakgrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type imRuntimeStub struct {
	sessionruntime.ReActSessionRuntime
	mu         sync.Mutex
	requests   []sessionruntime.ConnectRequest
	onEvent    sessionruntime.ReActEventHandler
	connection *imConnectionStub
}

func (r *imRuntimeStub) Connect(_ context.Context, req sessionruntime.ConnectRequest, onEvent sessionruntime.ReActEventHandler) (sessionruntime.ReActConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	r.onEvent = onEvent
	return r.connection, nil
}

type imConnectionStub struct {
	mu        sync.Mutex
	inputs    []*ypb.AIInputEvent
	done      chan struct{}
	closeOnce sync.Once
}

func (c *imConnectionStub) Send(event *ypb.AIInputEvent) error {
	c.mu.Lock()
	c.inputs = append(c.inputs, event)
	c.mu.Unlock()
	return nil
}

func (c *imConnectionStub) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	return nil
}

func (c *imConnectionStub) Done() <-chan struct{} { return c.done }
func (c *imConnectionStub) CreatedRuntime() bool  { return true }

func TestIMAIReActBackendUsesSessionRuntimeDirectly(t *testing.T) {
	connection := &imConnectionStub{done: make(chan struct{})}
	runtime := &imRuntimeStub{connection: connection}
	backend := &imAIReActBackend{runtime: runtime}

	// Preserve the former in-process stream contract: Send only admits the
	// message; an invalid first message terminates the actor and is observed by
	// Recv rather than synchronously by Send.
	invalidStream, err := backend.StartAIReAct(context.Background())
	require.NoError(t, err)
	require.NoError(t, invalidStream.Send(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "too early"}))
	_, err = invalidStream.Recv()
	require.Error(t, err)
	require.Empty(t, runtime.requests)

	closedBeforeStart, err := backend.StartAIReAct(context.Background())
	require.NoError(t, err)
	require.NoError(t, closedBeforeStart.CloseSend())
	_, err = closedBeforeStart.Recv()
	require.ErrorContains(t, err, "recv first mgs failed")

	stream, err := backend.StartAIReAct(context.Background())
	require.NoError(t, err)

	params := &ypb.AIStartParams{TimelineSessionID: "im-runtime-session"}
	require.NoError(t, stream.Send(&ypb.AIInputEvent{IsStart: true, Params: params}))
	require.Eventually(t, func() bool {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return len(runtime.requests) == 1 && runtime.requests[0].StartParams == params && runtime.onEvent != nil
	}, time.Second, 10*time.Millisecond)

	runtime.mu.Lock()
	onEvent := runtime.onEvent
	runtime.mu.Unlock()
	require.NoError(t, onEvent(&schema.AiOutputEvent{NodeId: "runtime-event"}))
	output, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "runtime-event", output.GetNodeId())

	input := &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "forwarded"}
	require.NoError(t, stream.Send(input))
	require.Eventually(t, func() bool {
		connection.mu.Lock()
		defer connection.mu.Unlock()
		return len(connection.inputs) == 1 && connection.inputs[0] == input
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, stream.CloseSend())
	select {
	case <-connection.Done():
	case <-time.After(time.Second):
		t.Fatal("closing the IM stream did not close its runtime connection")
	}
}
