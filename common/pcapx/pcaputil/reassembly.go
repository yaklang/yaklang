package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sort"
)

type futureFrame struct {
	Seq     uint32
	Len     int
	Payload []byte
}

// TrafficFlow is a tcp flow
type TrafficFlow struct {
	// ClientConn
	ClientConn *TrafficConnection
	ServerConn *TrafficConnection
	Hash       string
	Index      uint64

	ctx    context.Context
	cancel context.CancelFunc

	pool *trafficPool
}

func (t *TrafficFlow) String() string {
	return fmt.Sprintf("stream[%3d]: %v <-> %v", t.Index, t.ClientConn.localAddr, t.ServerConn.localAddr)
}

func (t *TrafficFlow) Feed(packet *layers.TCP) {
	if t != nil {
		if t.pool != nil {
			t.pool.flowCache.Set(t.Hash, t)
		}
	}
	if !packet.ACK && !packet.FIN && !packet.SYN && len(packet.Payload) <= 0 && !packet.RST {
		return
	}

	if t.ClientConn.localPort == int(packet.SrcPort) {
		t.ClientConn.Feed(packet)
	} else {
		t.ServerConn.Feed(packet)
	}
}

// TrafficConnection is a tcp connection
type TrafficConnection struct {
	isn        uint32
	currentSeq uint32
	nextSeq    uint32
	initialed  bool

	ctx        context.Context
	cancel     context.CancelFunc
	buf        *bytes.Buffer
	remoteAddr net.Addr
	remotePort int
	localAddr  net.Addr
	localPort  int

	waitGroup []*futureFrame

	Flow *TrafficFlow
}

func (t *TrafficConnection) Read(buf []byte) (int, error) {
	return t.buf.Read(buf)
}

func (t *TrafficConnection) String() string {
	return fmt.Sprintf("%v -> %v", t.localAddr, t.remoteAddr)
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
	return t.IsClosed()
}

func (t *TrafficConnection) Feed(tcp *layers.TCP) {
	if t.IsClosed() {
		return
	}

	// handle
	if tcp.SYN {
		t.isn = tcp.Seq
		t.currentSeq = tcp.Seq
		t.nextSeq = tcp.Seq + 1
		t.initialed = true
		return
	} else if tcp.FIN || tcp.RST {
		t.cancel()
		return
	}

	if len(tcp.Payload) == 0 {
		return
	}

	var haveData = tcp.PSH || tcp.ACK
	if haveData && !t.initialed {
		t.isn = tcp.Seq
		t.currentSeq = tcp.Seq
		t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
		t.initialed = true
		t.buf.Write(tcp.Payload)
		return
	}

	// check seq
	if t.initialed {
		if tcp.Seq == t.nextSeq && haveData {
			t.buf.Write(tcp.Payload)
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + uint32(len(tcp.Payload))
			t.buf.Write(tcp.Payload)
			var count int
			for _, frame := range t.waitGroup {
				if frame.Seq == t.nextSeq {
					t.buf.Write(frame.Payload)
					t.currentSeq = frame.Seq
					t.nextSeq = frame.Seq + uint32(frame.Len)
					count++
					continue
				} else {
					break
				}
			}
			if count > 0 {
				t.waitGroup = t.waitGroup[count:]
			}
			return
		}

		// abnormal
		if !haveData {
			t.currentSeq = tcp.Seq
			t.nextSeq = tcp.Seq + 1
			return
		}

		if tcp.Seq > t.nextSeq {
			t.waitGroup = append(t.waitGroup, &futureFrame{
				Seq:     tcp.Seq,
				Len:     len(tcp.Payload),
				Payload: tcp.Payload,
			})
			sort.SliceStable(t.waitGroup, func(i, j int) bool {
				return t.waitGroup[i].Seq < t.waitGroup[j].Seq
			})
			return
		} else {
			log.Debugf("retry... expect: %v, got: %v(%v) - (%v -> %v) Packet(%-4dbytes):  PSH: %v ACK: %v",
				t.nextSeq, tcp.Seq, int64(tcp.Seq)-int64(t.nextSeq),
				t.localAddr, t.remoteAddr, len(tcp.Payload), tcp.PSH, tcp.ACK,
			)
			return
		}
	}
	log.Debugf("unknown *(%v -> %v) Packet(%-6d bytes):  PSH:%v ACK:%v", t.localAddr, t.remoteAddr, len(tcp.Payload), tcp.PSH, tcp.ACK)
}

func (p *trafficPool) NewFlow(netType string, srcAddr, dstAddr string) (*TrafficFlow, error) {

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
	c2sConn := &TrafficConnection{
		buf:        &(bytes.Buffer{}),
		remoteAddr: dst,
		localAddr:  src,
	}
	c2sConn.ctx, c2sConn.cancel = context.WithCancel(flowCtx)
	s2cConn := &TrafficConnection{
		buf:        &(bytes.Buffer{}),
		remoteAddr: src,
		localAddr:  dst,
	}
	s2cConn.ctx, s2cConn.cancel = context.WithCancel(flowCtx)

	// bind flow
	flow := &TrafficFlow{
		ClientConn: c2sConn,
		ServerConn: s2cConn,
		Index:      p.nextStream(),
		ctx:        flowCtx,
		cancel:     cancel,
		pool:       p,
	}
	c2sConn.Flow = flow
	s2cConn.Flow = flow
	p.flowCache.Set(flow.Hash, flow)
	log.Debugf("%v is open", flow.String())
	return flow, nil
}
