package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"sync"
)

// TrafficFrame is a tcp frame
type TrafficFrame struct {
	ConnHash string // connection local -> remote
	Seq      uint32
	Payload  []byte
	Done     bool

	Connection *TrafficConnection
}

// TrafficFlow is a tcp flow
// lifecycle is created -> data-feeding -> closed(fin/rst/timeout)
// OnFrame: frame -> flow -> connection
// OnClosed: reason(fin/rst/timeout) -> flow
// OnCreated: flow created
type TrafficFlow struct {
	// ClientConn
	ClientConn *TrafficConnection
	ServerConn *TrafficConnection
	Hash       string
	Index      uint64

	ctx    context.Context
	cancel context.CancelFunc

	pool *TrafficPool

	frames []*TrafficFrame

	createdOnce *sync.Once
	closedOnce  *sync.Once

	onCloseHandler         func(reason TrafficFlowCloseReason, frame *TrafficFlow)
	onDataFrameReassembled func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	onDataFrameArrived     func(*TrafficFlow, *TrafficConnection, *TrafficFrame)

	StashedHTTPRequest []*http.Request
}

func (t *TrafficFlow) IsClosed() bool {
	select {
	case <-t.ctx.Done():
		t.triggerCloseEvent(TrafficFlowCloseReason_CTX_CANCEL)
		return true
	default:
		if t.ServerConn.IsClosed() && t.ClientConn.IsClosed() {
			t.triggerCloseEvent(TrafficFlowCloseReason_FIN)
			t.cancel()
			return true
		}
		return false
	}
}

func (t *TrafficFlow) String() string {
	return fmt.Sprintf("stream[%3d]: %v <-> %v", t.Index, t.ClientConn.localAddr, t.ServerConn.localAddr)
}

func (t *TrafficFlow) feed(packet *layers.TCP) {
	if t != nil {
		if t.pool != nil {
			t.pool.flowCache.Set(t.Hash, t)
		}
	}

	if t.ClientConn.localPort == int(packet.SrcPort) {
		t.ClientConn.FeedClient(packet)
	} else {
		t.ServerConn.FeedServer(packet)
	}
}

func (t *TrafficFlow) onFrame(frame *TrafficFrame) {
	if t.onDataFrameArrived != nil {
		t.onDataFrameArrived(t, frame.Connection, frame)
	}

	if len(t.frames) > 0 {
		lastFrame := t.frames[len(t.frames)-1]
		if lastFrame.ConnHash != frame.ConnHash {
			if t.onDataFrameReassembled != nil {
				t.onDataFrameReassembled(t, lastFrame.Connection, lastFrame)
			}
			t.frames = append(t.frames, frame)
		} else {
			lastFrame.Payload = append(lastFrame.Payload, frame.Payload...)
		}
	} else {
		t.frames = append(t.frames, frame)
	}

	if t.IsClosed() {
		if len(t.frames) > 0 && t.onDataFrameReassembled != nil {
			lastFrame := t.frames[len(t.frames)-1]
			t.onDataFrameReassembled(t, lastFrame.Connection, lastFrame)
		}
		log.Warnf("writing frame to a closed flow: %v", t.String())
	}
}

func (t *TrafficFlow) init(
	handle func(*TrafficFlow),
	onReassembledFrame func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onArrivedFrame func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onClose func(reason TrafficFlowCloseReason, flow *TrafficFlow),
) {
	t.createdOnce.Do(func() {
		if handle == nil {
			return
		}
		handle(t)
	})
	t.onDataFrameReassembled = onReassembledFrame
	t.onDataFrameArrived = onArrivedFrame
	t.onCloseHandler = onClose
}

type TrafficFlowCloseReason string

const (
	TrafficFlowCloseReason_FIN        TrafficFlowCloseReason = "fin"
	TrafficFlowCloseReason_RST        TrafficFlowCloseReason = "rst"
	TrafficFlowCloseReason_CTX_CANCEL TrafficFlowCloseReason = "ctx-canceled"
	TrafficFlowCloseReason_INACTIVE   TrafficFlowCloseReason = "inactive"
)

func (t *TrafficFlow) triggerCloseEvent(reason TrafficFlowCloseReason) {
	t.closedOnce.Do(func() {
		if t.onCloseHandler == nil {
			return
		}
		t.onCloseHandler(reason, t)
	})
}

func (t *TrafficFlow) onCloseFlow(h func(reason TrafficFlowCloseReason, frame *TrafficFlow)) {
	t.onCloseHandler = h
}

func (t *TrafficFlow) StashHTTPRequest(req *http.Request) {
	t.StashedHTTPRequest = append(t.StashedHTTPRequest, req)
}

func (t *TrafficFlow) FetchStashedHTTPRequest() *http.Request {
	if len(t.StashedHTTPRequest) > 0 {
		req := t.StashedHTTPRequest[0]
		t.StashedHTTPRequest = t.StashedHTTPRequest[1:]
		return req
	}
	return nil
}
