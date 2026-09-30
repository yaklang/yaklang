package yakgrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// The receive loop remains the only reader of the gRPC stream. Tests wait for
// the matching control acknowledgement before sending traffic through MITM.
type mitmTestAcknowledgement[T any] struct {
	mu    sync.Mutex
	match func(T) bool
	done  chan struct{}
}

func (a *mitmTestAcknowledgement[T]) observe(msg T) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.match != nil && a.match(msg) {
		close(a.done)
		a.match = nil
	}
}
func (a *mitmTestAcknowledgement[T]) wait(ctx context.Context, send func() error, match func(T) bool) error {
	a.mu.Lock()
	a.match, a.done = match, make(chan struct{})
	done := a.done
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.match = nil; a.mu.Unlock() }()
	if err := send(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HTTP mirroring enqueues writes before returning its response. A queue barrier
// therefore observes completion, including the absence of a filtered flow.
func waitMITMFlowWrites(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	require.NoError(t, yakit.EnqueueDBSave(func(_ *gorm.DB) error { close(done); return nil }))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("MITM database save queue did not drain")
	}
}

type mitmTestClient struct {
	ypb.Yak_MITMClient
	ack mitmTestAcknowledgement[*ypb.MITMResponse]
}

func sameMITMTestReplacers(expected, actual []*ypb.MITMContentReplacer) bool {
	if len(expected) != len(actual) {
		return false
	}
	// The server sorts rules by Index before acknowledging them.
	used := make([]bool, len(actual))
	for _, want := range expected {
		matched := false
		for i, got := range actual {
			if !used[i] && proto.Equal(want, got) {
				used[i], matched = true, true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (c *mitmTestClient) sendAndWait(req *ypb.MITMRequest) error {
	return c.ack.wait(c.Context(), func() error { return c.Send(req) }, func(msg *ypb.MITMResponse) bool {
		if req.GetUpdateFilter() {
			return msg.GetJustFilter() && proto.Equal(req.GetFilterData(), msg.GetFilterData())
		}
		if req.GetSetContentReplacers() {
			return msg.GetJustContentReplacer() && sameMITMTestReplacers(req.GetReplacers(), msg.GetReplacers())
		}
		if req.GetSetYakScript() {
			return msg.GetGetCurrentHook() && len(msg.GetHooks()) > 0
		}
		return false
	})
}
func sendMITMTestControl(t *testing.T, client ypb.Yak_MITMClient, req *ypb.MITMRequest) {
	t.Helper()
	require.NoError(t, client.(*mitmTestClient).sendAndWait(req))
}

type mitmV2TestClient struct {
	ypb.Yak_MITMV2Client
	ack mitmTestAcknowledgement[*ypb.MITMV2Response]
}

func (c *mitmV2TestClient) sendAndWait(req *ypb.MITMV2Request) error {
	return c.ack.wait(c.Context(), func() error { return c.Send(req) }, func(msg *ypb.MITMV2Response) bool {
		if req.GetUpdateFilter() {
			return msg.GetJustFilter() && proto.Equal(req.GetFilterData(), msg.GetFilterData())
		}
		if req.GetSetContentReplacers() {
			return msg.GetJustContentReplacer() && sameMITMTestReplacers(req.GetReplacers(), msg.GetReplacers())
		}
		if req.GetSetYakScript() {
			return msg.GetGetCurrentHook() && len(msg.GetHooks()) > 0
		}
		return false
	})
}
func sendMITMV2TestControl(t *testing.T, client ypb.Yak_MITMV2Client, req *ypb.MITMV2Request) {
	t.Helper()
	require.NoError(t, client.(*mitmV2TestClient).sendAndWait(req))
}
