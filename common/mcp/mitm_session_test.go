package mcp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type mitmTestResponse struct {
	response *ypb.MITMV2Response
	err      error
}

type controlledMITMStream struct {
	grpc.ClientStream
	ctx       context.Context
	sent      chan *ypb.MITMV2Request
	responses chan mitmTestResponse
	mu        sync.Mutex
	sendErr   error
	blockSend bool
}

func (s *controlledMITMStream) Context() context.Context { return s.ctx }
func (s *controlledMITMStream) Send(req *ypb.MITMV2Request) error {
	s.mu.Lock()
	err, block := s.sendErr, s.blockSend
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if block {
		<-s.ctx.Done()
		return s.ctx.Err()
	}
	select {
	case s.sent <- proto.Clone(req).(*ypb.MITMV2Request):
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}
func (s *controlledMITMStream) Recv() (*ypb.MITMV2Response, error) {
	select {
	case next := <-s.responses:
		return next.response, next.err
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

type controlledMITMClient struct {
	ypb.YakClient
	opened     chan *controlledMITMStream
	mu         sync.Mutex
	persistErr error
	saved      int
}

func (c *controlledMITMClient) GetProfileDatabase() *gorm.DB { return nil }
func (c *controlledMITMClient) MITMV2(ctx context.Context, _ ...grpc.CallOption) (ypb.Yak_MITMV2Client, error) {
	s := &controlledMITMStream{ctx: ctx, sent: make(chan *ypb.MITMV2Request, 8), responses: make(chan mitmTestResponse, 8)}
	c.opened <- s
	return s, nil
}
func (c *controlledMITMClient) SetMITMFilter(context.Context, *ypb.SetMITMFilterRequest, ...grpc.CallOption) (*ypb.SetMITMFilterResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saved++
	return &ypb.SetMITMFilterResponse{}, c.persistErr
}
func (c *controlledMITMClient) SetCurrentRules(context.Context, *ypb.MITMContentReplacers, ...grpc.CallOption) (*ypb.Empty, error) {
	return &ypb.Empty{}, nil
}

func newMITMOwnerForTest(t *testing.T, client *controlledMITMClient) *MCPServer {
	t.Helper()
	owner, err := NewMCPServer(WithEnableMITMToolSet(), WithGRPCClient(client))
	require.NoError(t, err)
	t.Cleanup(func() { owner.Close() })
	return owner
}

func callMITMToolAsync(owner *MCPServer, ctx context.Context, name string, args map[string]any) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := CallBuiltinTool(owner, ctx, name, args)
		done <- err
	}()
	return done
}

func receiveMITMTestValue[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for MITM test event")
		var zero T
		return zero
	}
}

func startOwnedMITMForTest(t *testing.T, owner *MCPServer, client *controlledMITMClient) *controlledMITMStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := callMITMToolAsync(owner, ctx, "start_mitm_v2", map[string]any{"host": "127.0.0.1", "port": 18080})
	stream := receiveMITMTestValue(t, client.opened)
	require.EqualValues(t, 18080, receiveMITMTestValue(t, stream.sent).GetPort())
	// Initial state messages arrive before the readiness message. They must
	// never be mistaken for an acknowledgement of a later update.
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, JustContentReplacer: true}}
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{HaveMessage: true, Message: &ypb.ExecResult{Message: []byte("starting mitm server")}}}
	require.NoError(t, receiveMITMTestValue(t, started))
	return stream
}

func mitmFilterTestArgs(path string) map[string]any {
	return map[string]any{"filterData": map[string]any{"excludeUri": []any{map[string]any{"matcherType": "word", "group": []string{path}}}}}
}

func TestMITMSessionControlAcknowledgement(t *testing.T) {
	client := &controlledMITMClient{opened: make(chan *controlledMITMStream, 4)}
	owner := newMITMOwnerForTest(t, client)
	stream := startOwnedMITMForTest(t, owner, client)
	require.NoError(t, stream.ctx.Err(), "the startup tool context must not own the proxy lifetime")
	updated := callMITMToolAsync(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/blocked"))
	request := receiveMITMTestValue(t, stream.sent)
	require.True(t, request.GetUpdateFilter())
	require.Equal(t, "/blocked", request.GetFilterData().ExcludeUri[0].Group[0])
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, FilterData: &ypb.MITMFilterData{}}}
	select {
	case err := <-updated:
		t.Fatalf("tool returned before its matching acknowledgement: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, FilterData: request.FilterData}}
	require.NoError(t, receiveMITMTestValue(t, updated))

	updated = callMITMToolAsync(owner, context.Background(), "set_current_rules", map[string]any{"rules": []any{}})
	request = receiveMITMTestValue(t, stream.sent)
	require.True(t, request.GetSetContentReplacers())
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustContentReplacer: true}}
	require.NoError(t, receiveMITMTestValue(t, updated))
	require.Empty(t, client.opened, "updates must reuse the existing stream")
	owner.Close()
	require.ErrorIs(t, stream.ctx.Err(), context.Canceled)

	_, err := CallBuiltinTool(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/closed"))
	require.ErrorContains(t, err, "MCP server is closed")
}

func TestMITMSessionConcurrentControls(t *testing.T) {
	client := &controlledMITMClient{opened: make(chan *controlledMITMStream, 4)}
	owner := newMITMOwnerForTest(t, client)
	stream := startOwnedMITMForTest(t, owner, client)
	first := callMITMToolAsync(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/first"))
	request := receiveMITMTestValue(t, stream.sent)
	queuedCtx, cancel := context.WithCancel(context.Background())
	queued := callMITMToolAsync(owner, queuedCtx, "set_mitm_filter", mitmFilterTestArgs("/cancelled"))
	cancel()
	require.ErrorIs(t, receiveMITMTestValue(t, queued), context.Canceled)
	require.NoError(t, stream.ctx.Err(), "cancelling an unsent command must leave the proxy running")
	second := callMITMToolAsync(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/second"))
	select {
	case <-stream.sent:
		t.Fatal("a second control was sent before the first was acknowledged")
	case <-time.After(30 * time.Millisecond):
	}
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, FilterData: request.FilterData}}
	require.NoError(t, receiveMITMTestValue(t, first))
	request = receiveMITMTestValue(t, stream.sent)
	require.Equal(t, "/second", request.FilterData.ExcludeUri[0].Group[0])
	stream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, FilterData: request.FilterData}}
	require.NoError(t, receiveMITMTestValue(t, second))
}

func TestMITMSessionOwnership(t *testing.T) {
	client := &controlledMITMClient{opened: make(chan *controlledMITMStream, 4)}
	first, second := newMITMOwnerForTest(t, client), newMITMOwnerForTest(t, client)
	firstStream := startOwnedMITMForTest(t, first, client)
	secondStream := startOwnedMITMForTest(t, second, client)
	updated := callMITMToolAsync(first, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/first-owner"))
	request := receiveMITMTestValue(t, firstStream.sent)
	firstStream.responses <- mitmTestResponse{response: &ypb.MITMV2Response{JustFilter: true, FilterData: request.FilterData}}
	require.NoError(t, receiveMITMTestValue(t, updated))
	require.Empty(t, secondStream.sent, "another MCP server's stream must not receive this command")
	first.Close()
	require.ErrorIs(t, firstStream.ctx.Err(), context.Canceled)
	require.NoError(t, secondStream.ctx.Err())
}

func TestMITMSessionFailures(t *testing.T) {
	for _, failure := range []string{"send", "receive", "ack timeout", "blocked send"} {
		t.Run(failure, func(t *testing.T) {
			client := &controlledMITMClient{opened: make(chan *controlledMITMStream, 4)}
			owner := newMITMOwnerForTest(t, client)
			stream := startOwnedMITMForTest(t, owner, client)
			if failure == "send" || failure == "blocked send" {
				stream.mu.Lock()
				stream.blockSend = failure == "blocked send"
				if failure == "send" {
					stream.sendErr = io.ErrClosedPipe
				}
				stream.mu.Unlock()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			updated := callMITMToolAsync(owner, ctx, "set_mitm_filter", mitmFilterTestArgs("/blocked"))
			if failure == "receive" || failure == "ack timeout" {
				receiveMITMTestValue(t, stream.sent)
				if failure == "receive" {
					stream.responses <- mitmTestResponse{err: io.EOF}
				}
			}
			require.Error(t, receiveMITMTestValue(t, updated))
			require.ErrorIs(t, stream.ctx.Err(), context.Canceled)
			owner.mitmMu.Lock()
			remaining := len(owner.mitmSessions)
			owner.mitmMu.Unlock()
			require.Zero(t, remaining, "failed streams must be removed")
			// A failed session must not poison a fresh start.
			require.NoError(t, startOwnedMITMForTest(t, owner, client).ctx.Err())
		})
	}
}

func TestMITMSessionPersistenceAndStartupFailure(t *testing.T) {
	client := &controlledMITMClient{opened: make(chan *controlledMITMStream, 4)}
	owner := newMITMOwnerForTest(t, client)
	_, err := CallBuiltinTool(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/before-start"))
	require.NoError(t, err)
	require.Empty(t, client.opened, "saving configuration must not start a proxy")
	stream := startOwnedMITMForTest(t, owner, client)
	client.mu.Lock()
	client.persistErr = errors.New("database write failed")
	client.mu.Unlock()
	_, err = CallBuiltinTool(owner, context.Background(), "set_mitm_filter", mitmFilterTestArgs("/not-saved"))
	require.ErrorContains(t, err, "database write failed")
	require.Empty(t, stream.sent)
	require.NoError(t, stream.ctx.Err())

	started := callMITMToolAsync(owner, context.Background(), "start_mitm_v2", map[string]any{"port": 18081})
	failed := receiveMITMTestValue(t, client.opened)
	receiveMITMTestValue(t, failed.sent)
	failed.responses <- mitmTestResponse{err: errors.New("address already in use")}
	require.ErrorContains(t, receiveMITMTestValue(t, started), "address already in use")
	require.ErrorIs(t, failed.ctx.Err(), context.Canceled)
}
