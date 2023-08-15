package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"net"
	"sort"
)

type futureFrame struct {
	Seq     uint32
	Len     int
	Payload []byte
}

type trafficConnection struct {
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

	flow *trafficFlow
}

func (t *trafficConnection) String() string {
	return fmt.Sprintf("%v -> %v", t.localAddr, t.remoteAddr)
}

func (t *trafficConnection) IsClosed() bool {
	select {
	case <-t.ctx.Done():
		return true
	default:
		return false
	}
}

func (t *trafficConnection) Close() bool {
	t.cancel()
	return t.IsClosed()
}

func (t *trafficConnection) Feed(tcp *layers.TCP) {
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
