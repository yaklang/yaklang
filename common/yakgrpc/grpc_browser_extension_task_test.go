package yakgrpc

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/metadata"
)

type browserTaskTestStream struct {
	ctx    context.Context
	mu     sync.Mutex
	events []*ypb.BrowserExtensionTaskEvent
}

func (s *browserTaskTestStream) Send(event *ypb.BrowserExtensionTaskEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *browserTaskTestStream) SetHeader(metadata.MD) error  { return nil }
func (s *browserTaskTestStream) SendHeader(metadata.MD) error { return nil }
func (s *browserTaskTestStream) SetTrailer(metadata.MD)       {}
func (s *browserTaskTestStream) Context() context.Context     { return s.ctx }
func (s *browserTaskTestStream) SendMsg(interface{}) error    { return nil }
func (s *browserTaskTestStream) RecvMsg(interface{}) error    { return nil }

func TestExecuteBrowserYakTaskStreamsYakitOutput(t *testing.T) {
	manager, err := browser.NewExtensionBridgeManager(
		browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	stream := &browserTaskTestStream{ctx: context.Background()}
	emitter := &browserTaskEmitter{stream: stream, taskID: "task-test", deviceID: "device-test"}
	server := &Server{browserBridge: manager, browserTasks: make(chan struct{}, 1)}

	err = server.executeBrowserYakTask(context.Background(), []byte(`{"code":"yakit.Info(\"hello browser task\")"}`), emitter)
	require.NoError(t, err)
	require.NotEmpty(t, stream.events)
	require.Equal(t, "log", stream.events[0].GetType())
	require.Contains(t, stream.events[0].GetMessage(), "hello browser task")
}

func TestExecuteBrowserExtensionTaskRejectsUnknownDevice(t *testing.T) {
	manager, err := browser.NewExtensionBridgeManager(
		browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	stream := &browserTaskTestStream{ctx: context.Background()}
	server := &Server{browserBridge: manager, browserTasks: make(chan struct{}, 1)}

	err = server.ExecuteBrowserExtensionTask(&ypb.BrowserExtensionTaskRequest{
		TaskId: "task-test", DeviceId: "unknown-device", Schema: "capability.call",
		Payload: []byte(`{"method":"browser.tabs","params":{}}`),
	}, stream)
	require.NoError(t, err)
	require.Len(t, stream.events, 1)
	require.Equal(t, "error", stream.events[0].GetType())
	require.Contains(t, stream.events[0].GetMessage(), "not found")
}

func TestDecodeBrowserTaskPayloadRejectsUnknownAndTrailingFields(t *testing.T) {
	var input struct {
		ID string `json:"id"`
	}
	require.ErrorContains(t, decodeBrowserTaskPayload(
		[]byte(`{"id":"workspace-1","unknown":true}`),
		&input,
	), "unknown field")
	require.ErrorContains(t, decodeBrowserTaskPayload(
		[]byte(`{"id":"workspace-1"} {"id":"workspace-2"}`),
		&input,
	), "trailing data")
}

func TestBrowserCapabilityAndYakTaskPayloadsRejectUnknownFields(t *testing.T) {
	manager, err := browser.NewExtensionBridgeManager(
		browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	emitter := &browserTaskEmitter{
		stream:   &browserTaskTestStream{ctx: context.Background()},
		taskID:   "task-test",
		deviceID: "device-test",
	}
	server := &Server{browserBridge: manager, browserTasks: make(chan struct{}, 1)}

	err = server.executeBrowserCapabilityTask(
		context.Background(),
		[]byte(`{"method":"browser.tabs","params":{},"legacy":true}`),
		emitter,
	)
	require.ErrorContains(t, err, "unknown field")
	err = server.executeBrowserYakTask(
		context.Background(),
		[]byte(`{"code":"yakit.Info(1)","legacy":true}`),
		emitter,
	)
	require.ErrorContains(t, err, "unknown field")
}
