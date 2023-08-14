package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"sync"
)

type TrafficPool struct {
	// map<string, *TrafficConnection>
	pool *sync.Map
	ctx  context.Context
}

func NewTrafficPool(ctx context.Context) *TrafficPool {
	return &TrafficPool{pool: new(sync.Map), ctx: ctx}
}

func (p *TrafficPool) Feed(networkLayerFlow gopacket.Flow, networkLayer gopacket.SerializableLayer, transportLayer *layers.TCP) {
	var networkStr string
	var srcIP string
	var dstIP string
	var srcPort = int(transportLayer.SrcPort)
	var dstPort = int(transportLayer.DstPort)
	switch ret := networkLayer.(type) {
	case *layers.IPv4:
		networkStr = "tcp4"
		srcIP = ret.SrcIP.String()
		dstIP = ret.DstIP.String()
	case *layers.IPv6:
		networkStr = "tcp6"
		srcIP = ret.SrcIP.String()
		dstIP = ret.DstIP.String()
	default:
		return
	}

	var srcAddrString = utils.HostPort(srcIP, srcPort)
	var dstAddrString = utils.HostPort(dstIP, dstPort)
	var hash = codec.Sha256(fmt.Sprintf("%s: %s->%s", networkStr, srcAddrString, dstAddrString))
	var conn *TrafficConnection
	if ret, ok := p.pool.Load(hash); !ok {
		// no reason  ...
		if transportLayer.Payload != nil && transportLayer.PSH {
			conn, err := p.NewConnection(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			conn.buf.Write(transportLayer.Payload)
			p.pool.Store(hash, conn)
			return
		}
		// SYN && !ACK -> start a conn
		if transportLayer.SYN && !transportLayer.ACK {
			conn, err := p.NewConnection(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			p.pool.Store(hash, conn)
			return
		}
		return
	} else {
		conn = ret.(*TrafficConnection)
	}
	if conn == nil {
		return
	}
	conn.Feed(transportLayer)
}

func (p *TrafficPool) NewConnection(netType string, srcAddr, dstAddr string) (*TrafficConnection, error) {
	ctx, cancel := context.WithCancel(p.ctx)
	_ = cancel
	dst, err := net.ResolveTCPAddr(netType, dstAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", dstAddr, err)
	}
	src, err := net.ResolveTCPAddr(netType, srcAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", srcAddr, err)
	}
	return &TrafficConnection{
		ctx:        ctx,
		cancel:     cancel,
		buf:        &(bytes.Buffer{}),
		remoteAddr: dst,
		localAddr:  src,
	}, nil
}

func (p *TrafficPool) CalcHash(networkLayerFlow gopacket.Flow, transportLayer *layers.TCP) (string, string) {
	vector := fmt.Sprintf("%v:%v -> %v:%v", networkLayerFlow.Src(), transportLayer.SrcPort, networkLayerFlow.Dst(), transportLayer.DstPort)
	return codec.Sha256(vector), vector
}

type TrafficConnection struct {
	isn        uint32
	currentSeq uint32
	nextSeq    uint32

	ctx        context.Context
	cancel     context.CancelFunc
	buf        *bytes.Buffer
	remoteAddr net.Addr
	localAddr  net.Addr
}

func (t *TrafficConnection) String() string {
	return fmt.Sprintf("%v -> %v", t.localAddr, t.remoteAddr)
}

func (t *TrafficConnection) Feed(tcp *layers.TCP) {
	if t.isn <= 0 {
		if tcp.PSH {
			t.isn = tcp.Seq - 1
			t.currentSeq = tcp.Seq
			t.nextSeq = t.currentSeq + uint32(len(tcp.Payload))
		} else {
			return
		}
	} else {
		var expect = t.nextSeq
		var got = tcp.Seq
		var offset = int64(got) - int64(expect)
		if t.nextSeq == tcp.Seq {
			t.currentSeq = tcp.Seq
			t.nextSeq = t.currentSeq + uint32(len(tcp.Payload))
		} else if t.nextSeq < tcp.Seq {
			log.Errorf("expect seq: %v, got seq: %v (<) offset: %v fin: %v", expect, got, offset, tcp.FIN)
		} else {
			log.Errorf("expect seq: %v, got seq: %v (<) offset: %v fin: %v", expect, got, offset, tcp.FIN)
		}
	}
	if len(tcp.Payload) > 0 {
		t.buf.Write(tcp.Payload)
	}
}

type TrafficFlow struct {
	// ClientConn
	ClientConn *TrafficConnection
	ServerConn *TrafficConnection
}
