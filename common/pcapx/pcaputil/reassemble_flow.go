package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"net/http"
	"sync"
	"time"
)

// TrafficFrame is a tcp frame
type TrafficFrame struct {
	ConnHash  string // connection local -> remote
	Seq       uint32
	Payload   []byte
	Timestamp time.Time
	Done      bool

	Connection *TrafficConnection
}

// TrafficFlow is a tcp flow
// lifecycle is created -> data-feeding -> closed(fin/rst/timeout)
// OnFrame: frame -> flow -> connection
// OnClosed: reason(fin/rst/timeout) -> flow
// OnCreated: flow created
type TrafficFlow struct {
	// ClientConn
	IsIpv4              bool
	IsIpv6              bool
	IsEthernetLinkLayer bool
	HardwareSrcMac      string
	HardwareDstMac      string
	ClientConn          *TrafficConnection
	ServerConn          *TrafficConnection
	Hash                string
	Index               uint64
	// no three-way handshake detected
	IsHalfOpen bool

	ctx    context.Context
	cancel context.CancelFunc

	pool *TrafficPool

	frames []*TrafficFrame

	createdOnce *sync.Once
	closedOnce  *sync.Once

	onCloseHandler         func(reason TrafficFlowCloseReason, frame *TrafficFlow)
	onDataFrameReassembled func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	onDataFrameArrived     func(*TrafficFlow, *TrafficConnection, *TrafficFrame)

	httpflowMutex *sync.Mutex
	httpflowWg    *sync.WaitGroup
	requestQueue  *omap.OrderedMap[string, *http.Request]
	responseQueue *omap.OrderedMap[string, *http.Response]
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

func (t *TrafficFlow) ShiftFlow() (*http.Request, *http.Response) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	req := t.requestQueue.Shift()
	rsp := t.responseQueue.Shift()
	return req, rsp
}

func (t *TrafficFlow) CanShiftHTTPFlow() bool {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	return t.requestQueue.Len() > 0 || t.responseQueue.Len() > 0
}

func (t *TrafficFlow) AutoTriggerHTTPFlow(h func(*TrafficFlow, *http.Request, *http.Response)) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	if t.requestQueue.Len() > 0 && t.responseQueue.Len() > 0 {
		req := t.requestQueue.Shift()
		rsp := t.responseQueue.Shift()
		h(t, req, rsp)
	}
}

func (t *TrafficFlow) ForceShutdownConnection() {
	t.ServerConn.Close()
	t.ClientConn.Close()
	t.httpflowWg.Wait()
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
		log.Warnf("writing frame to a closed flow: %v (%#v)", t.String(), frame.Payload)
	}
}

func (t *TrafficFlow) init(
	handle func(*TrafficFlow),
	onReassembledFrame []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onArrivedFrame []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onClose func(reason TrafficFlowCloseReason, flow *TrafficFlow),
) {
	t.createdOnce.Do(func() {
		if handle == nil {
			return
		}
		handle(t)
	})
	t.onDataFrameReassembled = func(flow *TrafficFlow, connection *TrafficConnection, frame *TrafficFrame) {
		for _, i := range onReassembledFrame {
			i(flow, connection, frame)
		}
	}
	t.onDataFrameArrived = func(flow *TrafficFlow, connection *TrafficConnection, frame *TrafficFrame) {
		for _, i := range onArrivedFrame {
			i(flow, connection, frame)
		}
	}
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
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	t.requestQueue.Push(req)
}

func (t *TrafficFlow) StashHTTPResponse(rsp *http.Response) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	t.responseQueue.Push(rsp)
}
