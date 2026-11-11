package httpctx

import (
	"net/http"
	"sync"
	"sync/atomic"
)

type UpstreamPhase uint8

const (
	UpstreamAttempt UpstreamPhase = iota + 1
	UpstreamPoolWait
	UpstreamDial
	UpstreamSend
	UpstreamFirstByte
	UpstreamRead
	UpstreamStreaming
	UpstreamFailed
	WebsocketOnline
	WebsocketClosed
)

type UpstreamEvent struct {
	Phase      UpstreamPhase
	AttemptID  uint64
	Protocol   string
	StatusCode int
	Err        error
}

type UpstreamObserver func(UpstreamEvent)

const upstreamObserverKey = "mitm-upstream-observer"
const mitmConnectionIDKey = "mitm-connection-id"

var upstreamAttemptID atomic.Uint64

func SetUpstreamObserver(req *http.Request, observer UpstreamObserver) {
	SetContextValueInfoFromRequest(req, upstreamObserverKey, observer)
}

func GetUpstreamObserver(req *http.Request) UpstreamObserver {
	if req != nil {
		if info, ok := req.Context().Value(REQUEST_CONTEXT_INFOMAP).(*sync.Map); ok {
			value, _ := info.Load(upstreamObserverKey)
			observer, _ := value.(UpstreamObserver)
			return observer
		}
	}
	return nil
}

func (o UpstreamObserver) BeginAttempt(protocol string) UpstreamObserver {
	if o == nil {
		return nil
	}
	id := upstreamAttemptID.Add(1)
	o(UpstreamEvent{Phase: UpstreamAttempt, Protocol: protocol, AttemptID: id})
	return func(event UpstreamEvent) {
		event.AttemptID = id
		o(event)
	}
}

func (o UpstreamObserver) Notify(event UpstreamEvent) {
	if o != nil {
		o(event)
	}
}

func (o UpstreamObserver) Phase(phase UpstreamPhase) {
	o.Notify(UpstreamEvent{Phase: phase})
}

func SetMITMConnectionID(req *http.Request, id string) {
	SetContextValueInfoFromRequest(req, mitmConnectionIDKey, id)
}

func GetMITMConnectionID(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, mitmConnectionIDKey)
}
