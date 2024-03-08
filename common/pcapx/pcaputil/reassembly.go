package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

type futureFrame struct {
	Seq     uint32
	Len     int
	Payload []byte
	FIN     bool
}

// TrafficConnection is a tcp connection
type TrafficConnection struct {
	isn        uint32
	currentSeq uint32
	nextSeq    uint32
	initialed  bool
	waitACK    bool

	ctx    context.Context
	cancel context.CancelFunc
	reader *utils.PipeReader
	writer *utils.PipeWriter

	remoteIP   net.IP
	remoteAddr net.Addr
	remotePort int
	localIP    net.IP
	localAddr  net.Addr
	localPort  int

	waitGroup []*futureFrame

	Flow *TrafficFlow

	initHttpPacketDirect bool
	isHttpRequestConn    bool
}

func (t *TrafficConnection) MarkAsHttpRequestConn(b bool) {
	if t.initHttpPacketDirect {
		return
	}
	t.initHttpPacketDirect = true
	t.isHttpRequestConn = b
}

func (t *TrafficConnection) IsMarkedAsHttpPacket() bool {
	return t.initHttpPacketDirect
}

func (t *TrafficConnection) IsHttpRequestConn() bool {
	return t.isHttpRequestConn
}

func (t *TrafficConnection) IsHttpResponseConn() bool {
	return !t.isHttpRequestConn
}

func (t *TrafficConnection) Read(buf []byte) (int, error) {
	return t.reader.Read(buf)
}

func (t *TrafficConnection) String() string {
	return fmt.Sprintf("%v -> %v", t.localAddr, t.remoteAddr)
}

func (t *TrafficConnection) LocalAddr() net.Addr {
	return t.localAddr
}

func (t *TrafficConnection) LocalIP() net.IP {
	return t.localIP
}

func (t *TrafficConnection) LocalPort() int {
	return t.localPort
}

func (t *TrafficConnection) RemoteAddr() net.Addr {
	return t.remoteAddr
}

func (t *TrafficConnection) RemoteIP() net.IP {
	return t.remoteIP
}

func (t *TrafficConnection) RemotePort() int {
	return t.remotePort
}

func (t *TrafficConnection) Hash() string {
	return codec.Sha256(t.String())
}

func (t *TrafficConnection) IsClosed() bool {
	select {
	case <-t.ctx.Done():
		return true
	default:
		return false
	}
}

func (t *TrafficConnection) Close() bool {
	t.cancel()
	t.reader.Close()
	t.writer.Close()
	return t.IsClosed()
}

func (t *TrafficConnection) CloseFlow() bool {
	t.cancel()
	if t.Flow != nil {
		t.Flow.triggerCloseEvent(TrafficFlowCloseReason_RST)
		t.Flow.cancel()
	}
	return t.IsClosed()
}

func (t *TrafficConnection) Write(b []byte, seq int64) (int, error) {
	// log.Infof("write %v bytes to %v => %v total: %v", len(b), t.String(), len(b), t.buf.Len())
	t.Flow.onFrame(&TrafficFrame{
		ConnHash:   t.Hash(),
		Seq:        uint32(seq),
		Payload:    b,
		Timestamp:  time.Now(),
		Connection: t,
	})

	n, err := t.writer.Write(b)
	if err != nil {
		log.Errorf("write %v bytes to %v failed: %s", len(b), t.String(), err)
		return n, err
	}
	return n, err
}

func (t *TrafficConnection) _feedHandlePayload(tcp *layers.TCP, debug func(string)) {
	// flow is created
	haveBody := len(tcp.Payload) > 0
	if tcp.Seq == t.nextSeq {
		if haveBody {
			t.Write(tcp.Payload, int64(tcp.Seq))
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
			var count = 0
			for _, frame := range t.waitGroup {
				if frame.Seq == t.nextSeq {
					if frame.FIN {
						//debug("close by fin(cached)")
						t.nextSeq = tcp.Seq + 1
						t.Close()
						break
					}

					t.Write(frame.Payload, int64(tcp.Seq))
					t.nextSeq = frame.Seq + uint32(frame.Len)
					count++
					continue
				}
				break
			}
			if count > 0 {
				//debug(fmt.Sprintf("use cached frames in WaitGroup[%v]", count))
				t.waitGroup = t.waitGroup[count:]
			}
			return
		}

		if tcp.FIN {
			t.currentSeq = tcp.Seq
			//debug("close by fin")
			t.nextSeq = tcp.Seq + 1
			t.Close()

			// trigger DataFrameReassembled when fin received and sent
			if t.Flow.IsClosed() {
				if len(t.Flow.frames) > 0 && t.Flow.onDataFrameReassembled != nil {
					lastFrame := t.Flow.frames[len(t.Flow.frames)-1]
					t.Flow.onDataFrameReassembled(t.Flow, lastFrame.Connection, lastFrame)
				}
			}
			return
		}
	} else if tcp.Seq > t.nextSeq {
		// future frame, put it into packet
		if haveBody {
			t.waitGroup = append(t.waitGroup, &futureFrame{
				Seq:     tcp.Seq,
				Len:     len(tcp.Payload),
				Payload: tcp.Payload,
			})
			sort.SliceStable(t.waitGroup, func(i, j int) bool {
				return t.waitGroup[i].Seq < t.waitGroup[j].Seq
			})
			//debug(fmt.Sprintf("future packet cached[%v]", len(t.waitGroup)))
			return
		}

		if tcp.FIN {
			t.waitGroup = append(t.waitGroup, &futureFrame{
				Seq:     tcp.Seq,
				Len:     len(tcp.Payload),
				Payload: tcp.Payload,
				FIN:     true,
			})
			sort.SliceStable(t.waitGroup, func(i, j int) bool {
				return t.waitGroup[i].Seq < t.waitGroup[j].Seq
			})
			// debug(fmt.Sprintf("future fin[%v]", len(t.waitGroup)))
			return
		}
	} else {
		// out-of-order frame, ignore
		return
	}

	log.Debugf("unknown *(%v -> %v) Packet(%-6d bytes):  PSH:%v ACK:%v", t.localAddr, t.remoteAddr, len(tcp.Payload), tcp.PSH, tcp.ACK)
}

func (t *TrafficConnection) FeedServer(tcp *layers.TCP) {
	//if t.IsClosed() {
	//	return
	//}

	debug := func(verbose string) {
		// future frame, put it into packet
		log.Infof(`*server* -> `+verbose+": expect: %v, got: %v(%v) - (%v -> %v) Packet(%-4dbytes):  SYN: %v PSH: %v ACK: %v FIN: %v",
			t.nextSeq, tcp.Seq, int64(tcp.Seq)-int64(t.nextSeq),
			t.localAddr, t.remoteAddr, len(tcp.Payload),
			tcp.SYN, tcp.PSH, tcp.ACK, tcp.FIN,
		)
	}
	_ = debug

	// syn-ack is initialized for server side
	if tcp.SYN && tcp.ACK {
		t.initialed = true
		t.isn = tcp.Seq
		t.nextSeq = tcp.Seq + 1
		return
	}

	if tcp.RST {
		//debug("close(rst)")
		t.CloseFlow()
		return
	}

	// not initialed, in-completed flow
	if !t.initialed {
		havePayload := len(tcp.Payload) > 0
		if havePayload {
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
			t.isn = tcp.Seq
			t.initialed = true
			t.Write(tcp.Payload, int64(tcp.Seq))
		}
		return
	}

	t._feedHandlePayload(tcp, debug)
}

func (t *TrafficConnection) FeedClient(tcp *layers.TCP) {
	debug := func(verbose string) {
		// future frame, put it into packet
		log.Infof(`*client*-> `+verbose+": expect: %v, got: %v(%v) - (%v -> %v) Packet(%-4dbytes):  SYN: %v PSH: %v ACK: %v",
			t.nextSeq, tcp.Seq, int64(tcp.Seq)-int64(t.nextSeq),
			t.localAddr, t.remoteAddr, len(tcp.Payload), tcp.SYN, tcp.PSH, tcp.ACK,
		)
	}
	_ = debug

	if t.IsClosed() {
		return
	}

	// SYN
	if tcp.SYN && !tcp.ACK {
		// ISN: initial sequence number
		t.waitACK = true
		t.isn = tcp.Seq
		t.nextSeq = tcp.Seq + 1
		return
	}

	if t.waitACK && tcp.ACK && tcp.Seq == t.nextSeq {
		t.initialed = true
		t.waitACK = false
		return
	}

	if tcp.RST {
		t.CloseFlow()
		return
	}

	// if in-completed flow, just handle psh
	if !t.initialed {
		havePayload := len(tcp.Payload) > 0
		if havePayload {
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
			t.isn = tcp.Seq
			t.initialed = true
			t.Write(tcp.Payload, int64(tcp.Seq))
		}
		return
	}

	t._feedHandlePayload(tcp, debug)
}

func (p *TrafficPool) NewFlow(netType string, srcAddr, dstAddr string) (*TrafficFlow, error) {

	flowCtx, cancel := context.WithCancel(p.ctx)
	_ = cancel

	dst, err := net.ResolveTCPAddr(netType, dstAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", dstAddr, err)
	}
	src, err := net.ResolveTCPAddr(netType, srcAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", srcAddr, err)
	}
	clientReader, clientWriter := utils.NewBufPipe(make([]byte, 0))
	serverReader, serverWriter := utils.NewBufPipe(make([]byte, 0))
	c2sConn := &TrafficConnection{
		reader:     clientReader,
		writer:     clientWriter,
		remoteAddr: dst,
		localAddr:  src,
	}
	c2sConn.ctx, c2sConn.cancel = context.WithCancel(flowCtx)
	s2cConn := &TrafficConnection{
		reader:     serverReader,
		writer:     serverWriter,
		remoteAddr: src,
		localAddr:  dst,
	}
	s2cConn.ctx, s2cConn.cancel = context.WithCancel(flowCtx)

	// bind flow
	flow := &TrafficFlow{
		ClientConn:    c2sConn,
		ServerConn:    s2cConn,
		Index:         p.nextStream(),
		ctx:           flowCtx,
		cancel:        cancel,
		pool:          p,
		createdOnce:   new(sync.Once),
		closedOnce:    new(sync.Once),
		httpflowMutex: new(sync.Mutex),
		httpflowWg:    new(sync.WaitGroup),
		requestQueue:  omap.NewOrderedMap(make(map[string]*http.Request)),
		responseQueue: omap.NewOrderedMap(make(map[string]*http.Response)),
	}
	c2sConn.Flow = flow
	s2cConn.Flow = flow
	p.flowCache.Set(flow.Hash, flow)
	log.Debugf("%v is open", flow.String())
	return flow, nil
}

func (c *TrafficConnection) GetBuffer() io.Reader {
	return c.reader
}
