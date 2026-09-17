package pcaputil

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/algorithm"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var flowPool = &sync.Pool{ // TrafficFlow
	New: func() any {
		return &TrafficFlow{
			requestQueue:  algorithm.NewQueue[*http.Request](),
			responseQueue: algorithm.NewQueue[*http.Response](),
			frames:        make([]*TrafficFrame, 0),
		}
	},
}

// TrafficFrame is a tcp frame
type TrafficFrame struct {
	Timestamp  time.Time
	Connection *TrafficConnection
	ConnHash   string // connection local -> remote
	Payload    []byte
	Seq        uint32
	Done       bool
}

// TrafficFlow is a tcp flow
// lifecycle is created -> data-feeding -> closed(fin/rst/timeout)
// OnFrame: frame -> flow -> connection
// OnClosed: reason(fin/rst/timeout) -> flow
// OnCreated: flow created
type TrafficFlow struct {
	binState               *binFlow
	key                    flowKey
	frameMu                sync.Mutex
	httpStarted            sync.Once
	closed                 atomic.Bool
	ClientConn             *TrafficConnection
	createdOnce            sync.Once
	pool                   *TrafficPool
	requestQueue           *algorithm.Queue[*http.Request]
	ServerConn             *TrafficConnection
	httpflowWg             sync.WaitGroup
	httpflowMutex          sync.Mutex
	onDataFrameArrived     func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	onDataFrameReassembled func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	responseQueue          *algorithm.Queue[*http.Response]
	onCloseHandler         func(reason TrafficFlowCloseReason, frame *TrafficFlow)
	closeNotified          atomic.Bool
	Hash                   string
	HardwareSrcMac         string
	HardwareDstMac         string
	frames                 []*TrafficFrame
	streamCapacity         int
	Index                  uint64
	IsHalfOpen             bool
	IsIpv6                 bool
	IsEthernetLinkLayer    bool
	IsIpv4                 bool
}

// Context cancellation is shared by the capture; individual flow/direction
// state does not need a context tree or registration in its parent's map.
func (t *TrafficFlow) stopped() bool {
	return t.closed.Load() || t.pool == nil || t.pool.canceled()
}

func (t *TrafficFlow) IsClosed() bool {
	if t.pool == nil {
		// already released
		return true
	}

	if t.stopped() {
		t.triggerCloseEvent(TrafficFlowCloseReason_CTX_CANCEL)
		return true
	}
	if t.ServerConn.IsClosed() && t.ClientConn.IsClosed() {
		t.closed.Store(true)
		t.triggerCloseEvent(TrafficFlowCloseReason_FIN)
		return true
	}
	return false
}

func (t *TrafficFlow) ShiftFlow() (*http.Request, *http.Response) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	req, _ := t.requestQueue.Dequeue()
	rsp, _ := t.responseQueue.Dequeue()
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
		req, _ := t.requestQueue.Dequeue()
		rsp, _ := t.responseQueue.Dequeue()
		rsp.Request = req
		if req != nil && rsp != nil {
			if offset := codec.Atoi(rsp.Header.Get(tsconst)); offset > 0 {
				httpctx.SetResponseTimestamp(rsp, t.GetHTTPResponseConnection().timestamps.at(offset))
			}
		}

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

// flushFrame detaches the accumulated frame before callbacks so previously
// delivered frames are immutable and callbacks may close their flow.
func (t *TrafficFlow) flushFrame() {
	t.frameMu.Lock()
	var frame *TrafficFrame
	if len(t.frames) > 0 {
		frame = t.frames[0]
		t.frames = nil
		frame.Done = true
	}
	t.frameMu.Unlock()
	if frame != nil && t.onDataFrameReassembled != nil {
		t.onDataFrameReassembled(t, frame.Connection, frame)
	}
}

func (t *TrafficFlow) onFrame(frame *TrafficFrame) {
	if t.pool.captureConf != nil && t.pool.captureConf.binParser != nil {
		if t.binState == nil {
			t.binState = t.pool.captureConf.binParser.newFlow(t)
		}
		direction := 0
		if frame.Connection != t.ClientConn {
			direction = 1
		}
		t.binState.feed(direction, frame.Payload, frame.Timestamp)
	}
	// Arrived callbacks may retain frames. Do not give them packet-source storage
	// or the same object that is subsequently extended for a reassembled frame.
	if t.onDataFrameArrived != nil {
		arrived := *frame
		arrived.Payload = append([]byte(nil), frame.Payload...)
		t.onDataFrameArrived(t, frame.Connection, &arrived)
	}
	if t.onDataFrameReassembled == nil {
		return
	}
	t.frameMu.Lock()
	switchDirection := len(t.frames) > 0 && t.frames[0].Connection != frame.Connection
	t.frameMu.Unlock()
	if switchDirection {
		t.flushFrame()
	}
	payload, seq := frame.Payload, frame.Seq
	for len(payload) > 0 {
		t.frameMu.Lock()
		if t.closeNotified.Load() {
			t.frameMu.Unlock()
			return
		}
		if len(t.frames) == 0 {
			t.frames = []*TrafficFrame{{ConnHash: frame.ConnHash, Seq: seq, Timestamp: frame.Timestamp, Connection: frame.Connection}}
			if t.pool.options.Stream && t.streamCapacity > 0 {
				t.frames[0].Payload = make([]byte, 0, t.streamCapacity)
			}
		}
		current := t.frames[0]
		n := len(payload)
		limit := t.pool.options.MaxFrameBytes
		if t.pool.options.Stream && n > limit-len(current.Payload) {
			n = limit - len(current.Payload)
		}
		if need := len(current.Payload) + n; t.pool.options.Stream && need > cap(current.Payload) {
			// Grow geometrically up to the chunk limit. Ordinary append growth
			// repeatedly copies large chunks and can overshoot the configured size.
			capacity := cap(current.Payload) * 2
			if capacity < need {
				capacity = need
			}
			if capacity > limit {
				capacity = limit
			}
			grown := make([]byte, len(current.Payload), capacity)
			copy(grown, current.Payload)
			current.Payload = grown
		}
		current.Payload = append(current.Payload, payload[:n]...)
		full := t.pool.options.Stream && len(current.Payload) >= limit
		if full {
			// A full chunk establishes the useful allocation size for this flow.
			// Later chunks need one allocation and no geometric growth/copying.
			t.streamCapacity = limit
		}
		t.frameMu.Unlock()
		payload, seq = payload[n:], seq+uint32(n)
		if full {
			t.flushFrame()
		}
	}
}

func (t *TrafficFlow) init(
	handle func(*TrafficFlow),
	onReassembledFrame []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onArrivedFrame []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame),
	onClose func(reason TrafficFlowCloseReason, flow *TrafficFlow),
) {
	if len(onReassembledFrame) > 0 {
		t.onDataFrameReassembled = func(flow *TrafficFlow, connection *TrafficConnection, frame *TrafficFrame) {
			for _, h := range onReassembledFrame {
				h(flow, connection, frame)
			}
		}
	}
	if len(onArrivedFrame) > 0 {
		t.onDataFrameArrived = func(flow *TrafficFlow, connection *TrafficConnection, frame *TrafficFrame) {
			for _, h := range onArrivedFrame {
				h(flow, connection, frame)
			}
		}
	}
	t.onCloseHandler = onClose
	t.createdOnce.Do(func() {
		if handle != nil {
			handle(t)
		}
	})
}

type TrafficFlowCloseReason string

const (
	TrafficFlowCloseReason_FIN            TrafficFlowCloseReason = "fin"
	TrafficFlowCloseReason_RST            TrafficFlowCloseReason = "rst"
	TrafficFlowCloseReason_CTX_CANCEL     TrafficFlowCloseReason = "ctx-canceled"
	TrafficFlowCloseReason_INACTIVE       TrafficFlowCloseReason = "inactive"
	TrafficFlowCloseReason_RESOURCE_LIMIT TrafficFlowCloseReason = "resource-limit"
)

func (t *TrafficFlow) triggerCloseEvent(reason TrafficFlowCloseReason) {
	if !t.closeNotified.CompareAndSwap(false, true) {
		return
	}
	if t.binState != nil {
		t.binState.close(reason)
	}
	if reason == TrafficFlowCloseReason_RESOURCE_LIMIT {
		if t.pool.counters != nil {
			t.pool.counters.limits.Add(1)
		} else {
			t.pool.singleDiagnostics.limits.Add(1)
		}
		t.pool.reassemblyFailure("TCP stream closed by a reassembly resource limit")
	}
	t.flushFrame()
	if t.onCloseHandler != nil {
		t.onCloseHandler(reason, t)
	}
}

func (t *TrafficFlow) Release() {
	if t.binState != nil {
		t.binState.close(TrafficFlowCloseReason_CTX_CANCEL)
		t.binState = nil
	}
	t.pool.flowCache.Remove(t.key)
	// t.ClientConn.Close()
	// t.ServerConn.Close()
	t.ClientConn, t.ServerConn = nil, nil
	t.createdOnce = sync.Once{}
	t.pool = nil
	t.closed.Store(false)
	t.requestQueue.Clear()
	t.responseQueue.Clear()
	t.httpflowWg = sync.WaitGroup{}
	t.httpflowMutex = sync.Mutex{}
	t.onDataFrameArrived, t.onDataFrameReassembled, t.onCloseHandler = nil, nil, nil
	t.closeNotified.Store(false)
	t.Hash, t.HardwareSrcMac, t.HardwareDstMac = "", "", ""
	t.frames = make([]*TrafficFrame, 0)
	t.Index = 0
	t.streamCapacity = 0
	t.key = flowKey{}
	t.httpStarted = sync.Once{}
	t.IsHalfOpen, t.IsEthernetLinkLayer, t.IsIpv4, t.IsIpv6 = false, false, false, false

	flowPool.Put(t)
}

func (t *TrafficFlow) onCloseFlow(h func(reason TrafficFlowCloseReason, frame *TrafficFlow)) {
	t.onCloseHandler = h
}

func (t *TrafficFlow) StashHTTPRequest(req *http.Request) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	if offset := httpctx.GetRequestReaderOffset(req); offset > 0 {
		httpctx.SetRequestTimestamp(req, t.GetHTTPRequestConnection().timestamps.at(offset))
	}
	t.requestQueue.Enqueue(req)
}

func (t *TrafficFlow) StashHTTPResponse(rsp *http.Response) {
	t.httpflowMutex.Lock()
	defer t.httpflowMutex.Unlock()
	t.responseQueue.Enqueue(rsp)
}

func (t *TrafficFlow) GetHTTPRequestConnection() *TrafficConnection {
	if !t.ClientConn.IsMarkedAsHttpPacket() {
		return nil
	}
	if t.ClientConn.IsHttpRequestConn() {
		return t.ClientConn
	}
	return t.ServerConn
}

func (t *TrafficFlow) GetHTTPResponseConnection() *TrafficConnection {
	if !t.ClientConn.IsMarkedAsHttpPacket() {
		return nil
	}
	if t.ClientConn.IsHttpRequestConn() {
		return t.ServerConn
	}
	return t.ClientConn
}

func (t *TrafficFlow) Close() { t.closeWithReason(TrafficFlowCloseReason_RST) }

func (t *TrafficFlow) closeWithReason(reason TrafficFlowCloseReason) {
	t.closed.Store(true)
	t.ServerConn.Close()
	t.ClientConn.Close()
	t.triggerCloseEvent(reason)
}
