package aimem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
)

func TestAsyncMemoryInitializationDoesNotBlockAndPublishesBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	started := make(chan struct{})
	backendCtx, closeBackend := context.WithCancel(context.Background())
	backend := &AIMemoryTriage{ctx: backendCtx, cancel: closeBackend}
	memory := newAsyncAIMemory(ctx, "test-session", func() (*AIMemoryTriage, error) {
		close(started)
		<-release
		return backend, nil
	})
	require.Equal(t, "test-session", memory.GetSessionID())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initialization did not start")
	}
	select {
	case <-memory.ready:
		t.Fatal("initializer should still be blocked")
	default:
	}

	waitCtx, stopWait := context.WithCancel(context.Background())
	stopWait()
	_, err := memory.WaitReady(waitCtx)
	require.ErrorIs(t, err, context.Canceled)
	close(release)
	readyCtx, stopReady := context.WithTimeout(ctx, time.Second)
	defer stopReady()
	got, err := memory.WaitReady(readyCtx)
	require.NoError(t, err)
	require.Same(t, backend, got)
	require.NoError(t, memory.Close())
	select {
	case <-memory.closed:
	case <-time.After(time.Second):
		t.Fatal("backend was not closed")
	}
	require.ErrorIs(t, backendCtx.Err(), context.Canceled)
}

func TestAsyncMemoryCloseBeforeInitializationFinishes(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	backendCtx, closeBackend := context.WithCancel(context.Background())
	memory := newAsyncAIMemory(context.Background(), "slow-session", func() (*AIMemoryTriage, error) {
		close(started)
		<-release
		return &AIMemoryTriage{ctx: backendCtx, cancel: closeBackend}, nil
	})
	<-started
	require.NoError(t, memory.Close())
	_, err := memory.SearchMemory("query", 100)
	require.ErrorIs(t, err, context.Canceled)
	close(release)
	select {
	case <-memory.closed:
	case <-time.After(time.Second):
		t.Fatal("late backend was not closed")
	}
	require.ErrorIs(t, backendCtx.Err(), context.Canceled)
}

func TestAsyncMemoryInitializationErrorsReachOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expected := errors.New("backend unavailable")
	memory := newAsyncAIMemory(ctx, "failed-session", func() (*AIMemoryTriage, error) { return nil, expected })
	_, err := memory.SearchMemoryWithoutAI("query", 100)
	require.ErrorIs(t, err, expected)
	require.ErrorIs(t, memory.HandleMemory("remember this"), expected)
	_, err = memory.SearchArchivedBatches(ctx, nil)
	require.ErrorIs(t, err, expected)
}

func TestAsyncMemoryEmptyBackendClose(t *testing.T) {
	backend := &AIMemoryHNSWBackend{}
	require.NoError(t, backend.Close())
	backend.graph.Store(hnsw.NewGraph[string]())
	require.NoError(t, backend.Close(), "closing a fast session with no memories must not attempt to export an empty graph")
}
