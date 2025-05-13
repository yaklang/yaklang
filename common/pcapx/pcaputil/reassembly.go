package pcaputil

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/algorithm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var connectionPool = &sync.Pool{ // TrafficConnection
	New: func() any {
		return &TrafficConnection{
			frames: algorithm.NewQueue[*TrafficFrame](),
		}
	},
}

type futureFrame struct {
	Payload []byte
	Len     int
	Seq     uint32
	FIN     bool
}

// TrafficConnection is a tcp connection
type TrafficConnection struct {
	localAddr            net.Addr
	remoteAddr           net.Addr
	ctx                  context.Context
	writer               *bufpipe.PipeWriter
	Flow                 *TrafficFlow
	frames               *algorithm.Queue[*TrafficFrame]
	cancel               context.CancelFunc
	reader               *bufpipe.PipeReader
	remoteIP             net.IP
	localIP              net.IP
	waitGroup            []*futureFrame
	remotePort           int
	localPort            int
	isn                  uint32
	nextSeq              uint32
	currentSeq           uint32
	waitACK              bool
	initialed            bool
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
		t.Flow.Close()
	}
	return t.IsClosed()
}

func (t *TrafficConnection) Release() {
	t.localAddr = nil
	t.remoteAddr = nil
	t.ctx = nil
	t.reader, t.writer = nil, nil
	t.Flow = nil
	t.frames.Clear()
	t.cancel = nil
	t.localIP, t.remoteIP = nil, nil
	t.waitGroup = make([]*futureFrame, 0)
	t.localPort, t.remotePort = 0, 0
	t.isn, t.nextSeq, t.currentSeq = 0, 0, 0
	t.waitACK, t.initialed, t.initHttpPacketDirect, t.isHttpRequestConn = false, false, false, false

	connectionPool.Put(t)
}

func (t *TrafficConnection) Write(b []byte, seq int64, ts time.Time) (int, error) {
	// log.Infof("write %v bytes to %v => %v total: %v", len(b), t.String(), len(b), t.buf.Len())
	if ts.IsZero() {
		ts = time.Now()
	}
	frame := &TrafficFrame{
		ConnHash:   t.Hash(),
		Seq:        uint32(seq),
		Payload:    b,
		Timestamp:  ts,
		Connection: t,
	}
	t.frames.Enqueue(frame)
	t.Flow.onFrame(frame)

	n, err := t.writer.Write(b)
	if err != nil {
		log.Errorf("write %v bytes to %v failed: %s", len(b), t.String(), err)
		return n, err
	}
	return n, err
}

func (t *TrafficConnection) _feedHandlePayload(tcp *layers.TCP, debug func(string), ts time.Time) {
	// flow is created
	haveBody := len(tcp.Payload) > 0
	if tcp.Seq == t.nextSeq {
		if haveBody {
			t.Write(tcp.Payload, int64(tcp.Seq), ts)
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
			count := 0
			for _, frame := range t.waitGroup {
				if frame.Seq == t.nextSeq {
					if frame.FIN {
						// debug("close by fin(cached)")
						t.nextSeq = tcp.Seq + 1
						t.Close()
						break
					}

					t.Write(frame.Payload, int64(tcp.Seq), ts)
					t.nextSeq = frame.Seq + uint32(frame.Len)
					count++
					continue
				}
				break
			}
			if count > 0 {
				// debug(fmt.Sprintf("use cached frames in WaitGroup[%v]", count))
				t.waitGroup = t.waitGroup[count:]
			}

			return
		}

		if tcp.FIN {
			t.currentSeq = tcp.Seq
			// debug("close by fin")
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
			// debug(fmt.Sprintf("future packet cached[%v]", len(t.waitGroup)))
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

func (t *TrafficConnection) FeedServer(tcp *layers.TCP, ts time.Time) {
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
		// debug("close(rst)")
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
			t.Write(tcp.Payload, int64(tcp.Seq), ts)
		}
		return
	}

	t._feedHandlePayload(tcp, debug, ts)
}

func (t *TrafficConnection) FeedClient(tcp *layers.TCP, ts time.Time) {
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
			t.Write(tcp.Payload, int64(tcp.Seq), ts)
		}
		return
	}

	t._feedHandlePayload(tcp, debug, ts)
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

	c2sConn := connectionPool.Get().(*TrafficConnection)
	{
		c2sConn.reader = clientReader
		c2sConn.writer = clientWriter
		c2sConn.localAddr = src
		c2sConn.remoteAddr = dst
		c2sConn.ctx, c2sConn.cancel = context.WithCancel(flowCtx)
	}

	s2cConn := connectionPool.Get().(*TrafficConnection)
	{
		s2cConn.reader = serverReader
		s2cConn.writer = serverWriter
		s2cConn.localAddr = dst
		s2cConn.remoteAddr = src
		s2cConn.ctx, s2cConn.cancel = context.WithCancel(flowCtx)

	}

	// bind flow
	flow := flowPool.Get().(*TrafficFlow)
	{
		flow.ClientConn = c2sConn
		flow.ServerConn = s2cConn
		flow.Index = p.nextStream()
		flow.ctx = flowCtx
		flow.cancel = cancel
		flow.pool = p
		flow.Hash = p.flowhash(netType, srcAddr, dstAddr)
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
