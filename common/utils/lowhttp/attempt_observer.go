package lowhttp

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"sync"
	"time"
)

// HTTPAttempt is an observation of one transport attempt, not a saved HTTPFlow.
// Packet/file fields are runtime-local evidence and must never be put on an event bus.
type HTTPAttempt struct {
	TurnID             string
	RuntimeID          string
	ToolCallID         string
	AgentID            string
	StartedAt          time.Time
	FinishedAt         time.Time
	HTTPS              bool
	Request            []byte
	Response           []byte
	RequestHeaderFile  string
	RequestBodyFile    string
	ResponseHeaderFile string
	ResponseBodyFile   string
	Error              error
}

type HTTPAttemptObserver func(HTTPAttempt) func(HTTPAttempt)
type attemptObserverContextKey struct{}
type scopedAttemptObserver struct {
	observer            HTTPAttemptObserver
	toolCallID, agentID string
}

var attemptObservers sync.Map

func WithHTTPAttemptObserver(ctx context.Context, observer HTTPAttemptObserver) context.Context {
	return context.WithValue(ctx, attemptObserverContextKey{}, observer)
}

func HasHTTPAttemptObserver(ctx context.Context) bool {
	observer, _ := ctx.Value(attemptObserverContextKey{}).(HTTPAttemptObserver)
	return observer != nil
}

// BindHTTPAttemptObserver bridges tool runtime IDs to the owning engine context.
// It is deliberately opt-in: ordinary clients and model-provider traffic are excluded.
func BindHTTPAttemptObserver(ctx context.Context, runtimeID, agentID string) func() {
	observer, _ := ctx.Value(attemptObserverContextKey{}).(HTTPAttemptObserver)
	if observer == nil || runtimeID == "" {
		return func() {}
	}
	entry := &scopedAttemptObserver{observer: observer, toolCallID: runtimeID, agentID: agentID}
	attemptObservers.Store(runtimeID, entry)
	return func() { attemptObservers.CompareAndDelete(runtimeID, entry) }
}

func observeHTTPAttempt(tr *transportRequest) func(*transportResult, error) {
	entry, ok := attemptObservers.Load(tr.option.RuntimeId)
	if !ok {
		return func(*transportResult, error) {}
	}
	scoped := entry.(*scopedAttemptObserver)
	tr.attemptRequestPacket = tr.packet
	tr.attemptResponsePacket = nil
	if tr.reqIns != nil {
		httpctx.SetResponseTooLargeHeaderFile(tr.reqIns, "")
		httpctx.SetResponseTooLargeBodyFile(tr.reqIns, "")
	}
	event := HTTPAttempt{RuntimeID: tr.option.RuntimeId, ToolCallID: scoped.toolCallID, AgentID: scoped.agentID, StartedAt: time.Now(), HTTPS: tr.option.Https, Request: bytes.Clone(tr.packet)}
	finish := scoped.observer(event)
	return func(result *transportResult, err error) {
		if finish == nil {
			return
		}
		// Authentication and reconnect paths reuse transport buffers. Observers
		// own immutable snapshots after the callback returns, not those buffers.
		event.Request = bytes.Clone(tr.attemptRequestPacket)
		event.FinishedAt = time.Now()
		event.Error = err
		response := tr.attemptResponsePacket
		if result != nil && len(result.rawBytes) > 0 {
			response = result.rawBytes
		}
		event.Response = bytes.Clone(response)
		if tr.reqIns != nil {
			event.RequestHeaderFile = httpctx.GetRequestTooLargeHeaderFile(tr.reqIns)
			event.RequestBodyFile = httpctx.GetRequestTooLargeBodyFile(tr.reqIns)
			event.ResponseHeaderFile = httpctx.GetResponseTooLargeHeaderFile(tr.reqIns)
			event.ResponseBodyFile = httpctx.GetResponseTooLargeBodyFile(tr.reqIns)
		}
		finish(event)
	}
}
