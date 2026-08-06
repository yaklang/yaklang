package yakit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPFlowCommittedRequiresSubscriptionAndSuccessfulCommit(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	resetHTTPFlowLiveBrokerForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)
	t.Cleanup(resetHTTPFlowLiveBrokerForTest)

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	subscribed := make(chan *ypb.DuplexConnectionResponse, 2)
	unsubscribed := make(chan *ypb.DuplexConnectionResponse, 1)
	registerServerPushCallback("committed-subscribed", ctx, 4, func(response *ypb.DuplexConnectionResponse) error {
		subscribed <- response
		return nil
	})
	registerServerPushCallback("committed-unsubscribed", ctx, 4, func(response *ypb.DuplexConnectionResponse) error {
		unsubscribed <- response
		return nil
	})
	t.Cleanup(func() {
		UnRegisterServerPushCallback("committed-subscribed")
		UnRegisterServerPushCallback("committed-unsubscribed")
	})
	require.True(t, SetServerPushSubscription("committed-subscribed", ServerPushType_HTTPFlowCommitted, true))

	identity := HTTPFlowDatabaseIdentity("committed-project.db")
	flow := committedTestFlow("committed-success", identity, 7)
	require.NoError(t, InsertHTTPFlow(db, flow))
	require.NotZero(t, flow.ID)

	response := receiveServerPushType(t, subscribed, ServerPushType_HTTPFlowCommitted)
	var event HTTPFlowCommittedEvent
	require.NoError(t, json.Unmarshal(response.GetData(), &event))
	require.Equal(t, uint32(1), event.Version)
	require.Equal(t, uint64(flow.ID), event.ID)
	require.Equal(t, uint64(flow.ID), event.HighWaterID)
	require.Equal(t, uint64(7), event.ProjectGeneration)
	require.Equal(t, identity, event.DatabaseIdentity)
	require.Equal(t, flow.RuntimeTiming.PersistedAtUnixMs, event.CommittedAtUnixMs)
	requireNoServerPushType(t, unsubscribed, ServerPushType_HTTPFlowCommitted, 30*time.Millisecond)

	// Reusing the unique hash makes the SQL fail. Failed writes must not emit a
	// committed event even though the caller supplied complete runtime timing.
	failed := committedTestFlow("committed-success", identity, 7)
	require.Error(t, InsertHTTPFlow(db, failed))
	requireNoServerPushType(t, subscribed, ServerPushType_HTTPFlowCommitted, 30*time.Millisecond)
}

func TestSubscriberBroadcastSkipsLazyEventBuildWithoutSubscribers(t *testing.T) {
	subscription := t.Name()
	built := 0
	build := func() any {
		built++
		return map[string]any{"id": built}
	}

	require.False(t, broadcastDataToSubscribersLazy(subscription, "test/lazy", build))
	require.Zero(t, built)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	responses := make(chan *ypb.DuplexConnectionResponse, 1)
	registerServerPushCallback(t.Name(), ctx, 2, func(response *ypb.DuplexConnectionResponse) error {
		responses <- response
		return nil
	})
	t.Cleanup(func() { UnRegisterServerPushCallback(t.Name()) })
	require.True(t, SetServerPushSubscription(t.Name(), subscription, true))
	require.True(t, broadcastDataToSubscribersLazy(subscription, "test/lazy", build))
	require.Equal(t, 1, built)
	require.Equal(t, "test/lazy", receiveServerPushType(t, responses, "test/lazy").GetMessageType())

	require.True(t, SetServerPushSubscription(t.Name(), subscription, false))
	require.False(t, broadcastDataToSubscribersLazy(subscription, "test/lazy", build))
	require.Equal(t, 1, built)
}

func receiveServerPushType(
	t *testing.T,
	responses <-chan *ypb.DuplexConnectionResponse,
	messageType string,
) *ypb.DuplexConnectionResponse {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case response := <-responses:
			if response.GetMessageType() == messageType {
				return response
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for server push type %s", messageType)
			return nil
		}
	}
}

func requireNoServerPushType(
	t *testing.T,
	responses <-chan *ypb.DuplexConnectionResponse,
	messageType string,
	wait time.Duration,
) {
	t.Helper()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case response := <-responses:
			if response.GetMessageType() == messageType {
				t.Fatalf("unexpected server push type %s", messageType)
			}
		case <-timer.C:
			return
		}
	}
}

func committedTestFlow(hash, identity string, generation uint64) *schema.HTTPFlow {
	return &schema.HTTPFlow{
		Hash:       hash,
		SourceType: schema.HTTPFlow_SourceType_MITM,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:  identity,
			ProjectGeneration: generation,
			FlowBuiltAtUnixMs: time.Now().Add(-time.Second).UnixMilli(),
		},
	}
}
