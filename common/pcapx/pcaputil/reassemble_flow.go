package pcaputil

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/algorithm"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var flowPool = &sync.Pool{ // TrafficFlow
	New: func() any {
		return &TrafficFlow{
			createdOnce:       new(sync.Once),
			triggerClosedOnce: new(sync.Once),
			httpflowMutex:     new(sync.Mutex),
			httpflowWg:        new(sync.WaitGroup),
			requestQueue:      algorithm.NewQueue[*http.Request](),
			responseQueue:     algorithm.NewQueue[*http.Response](),
			frames:            make([]*TrafficFrame, 0),
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
	ctx                    context.Context
	ClientConn             *TrafficConnection
	createdOnce            *sync.Once
	pool                   *TrafficPool
	cancel                 context.CancelFunc
	requestQueue           *algorithm.Queue[*http.Request]
	ServerConn             *TrafficConnection
	httpflowWg             *sync.WaitGroup
	httpflowMutex          *sync.Mutex
	onDataFrameArrived     func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	onDataFrameReassembled func(*TrafficFlow, *TrafficConnection, *TrafficFrame)
	responseQueue          *algorithm.Queue[*http.Response]
	onCloseHandler         func(reason TrafficFlowCloseReason, frame *TrafficFlow)
	triggerClosedOnce      *sync.Once
	Hash                   string
	HardwareSrcMac         string
	HardwareDstMac         string
	frames                 []*TrafficFrame
	Index                  uint64
	IsHalfOpen             bool
	IsIpv6                 bool
	IsEthernetLinkLayer    bool
	IsIpv4                 bool
}

func (t *TrafficFlow) IsClosed() bool {
	if t.ctx == nil {
		// already released
		return true
	}

	select {
	case <-t.ctx.Done():
		t.triggerCloseEvent(TrafficFlowCloseReason_CTX_CANCEL)
		return true
	default:
		if t.ServerConn.IsClosed() && t.ClientConn.IsClosed() {
			t.cancel()
			t.triggerCloseEvent(TrafficFlowCloseReason_FIN)
			return true
		}
		return false
	}
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
				count := 0
				t.GetHTTPResponseConnection().frames.ForEach(func(tf *TrafficFrame) {
					count += len(tf.Payload)
					if count >= offset {
						httpctx.SetResponseTimestamp(rsp, tf.Timestamp)
						return
					}
				})
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

func (t *TrafficFlow) feed(packet *layers.TCP, ts time.Time) {
	if t != nil {
		if t.pool != nil {
			t.pool.flowCache.Set(t.Hash, t)
		}
	}

	if t.ClientConn.localPort == int(packet.SrcPort) {
		t.ClientConn.FeedClient(packet, ts)
	} else {
		t.ServerConn.FeedServer(packet, ts)
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
	t.triggerClosedOnce.Do(func() {
		if t.onCloseHandler != nil {
			t.onCloseHandler(reason, t)
		}
	})
}

func (t *TrafficFlow) Release() {
	t.pool.flowCache.Remove(t.Hash)
	t.ctx = nil
	// t.ClientConn.Close()
	// t.ServerConn.Close()
	t.ClientConn, t.ServerConn = nil, nil
	t.createdOnce = new(sync.Once)
	t.pool = nil
	t.cancel = nil
	t.requestQueue.Clear()
	t.responseQueue.Clear()
	t.httpflowWg = new(sync.WaitGroup)
	t.httpflowMutex = new(sync.Mutex)
	t.onDataFrameArrived, t.onDataFrameReassembled, t.onCloseHandler = nil, nil, nil
	t.triggerClosedOnce = new(sync.Once)
	t.Hash, t.HardwareSrcMac, t.HardwareDstMac = "", "", ""
	t.frames = make([]*TrafficFrame, 0)
	t.Index = 0
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
		// have offset
		count := 0
		t.GetHTTPRequestConnection().frames.ForEach(func(tf *TrafficFrame) {
			count += len(tf.Payload)
			if count >= offset {
				httpctx.SetRequestTimestamp(req, tf.Timestamp)
				return
			}
		})
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

func (t *TrafficFlow) Close() {
	t.cancel()
	t.ServerConn.Close()
	t.ClientConn.Close()
	t.triggerCloseEvent(TrafficFlowCloseReason_RST)
}
