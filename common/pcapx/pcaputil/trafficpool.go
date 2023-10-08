package pcaputil

import (
	"context"
	"github.com/ReneKroon/ttlcache"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type TrafficPool struct {
	// map<string, *TrafficConnection>
	pool               *sync.Map
	ctx                context.Context
	currentStreamIndex uint64

	flowCache *ttlcache.Cache

	onFlowCreated                   func(flow *TrafficFlow)
	onFlowClosed                    func(reason TrafficFlowCloseReason, flow *TrafficFlow)
	onFlowFrameDataFrameArrived     []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
	onFlowFrameDataFrameReassembled []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
}

func NewTrafficPool(ctx context.Context) *TrafficPool {
	pool := &TrafficPool{pool: new(sync.Map), ctx: ctx}
	fCache := ttlcache.NewCache()
	fCache.SetExpirationCallback(func(key string, value interface{}) {
		pool.pool.Delete(key)
		flow, ok := value.(*TrafficFlow)
		if !ok {
			return
		}
		flow.triggerCloseEvent(TrafficFlowCloseReason_INACTIVE)
		flow.cancel()
		flow.ServerConn.Close()
		flow.ClientConn.Close()
	})
	fCache.SetTTL(30 * time.Second)
	pool.flowCache = fCache
	return pool
}

func (p *TrafficPool) nextStream() uint64 {
	return atomic.AddUint64(&p.currentStreamIndex, 1)
}

func (p *TrafficPool) Feed(ethernetLayer *layers.Ethernet, networkLayer gopacket.SerializableLayer, transportLayer *layers.TCP) {
	var networkStr string
	var srcIP net.IP
	var dstIP net.IP
	var srcPort = int(transportLayer.SrcPort)
	var dstPort = int(transportLayer.DstPort)
	var isIpv4 = false
	var isIpv6 = false
	switch ret := networkLayer.(type) {
	case *layers.IPv4:
		networkStr = "tcp4"
		srcIP = ret.SrcIP
		dstIP = ret.DstIP
		isIpv4 = true
	case *layers.IPv6:
		networkStr = "tcp6"
		srcIP = ret.SrcIP
		dstIP = ret.DstIP
		isIpv6 = true
	default:
		return
	}

	var srcAddrString = utils.HostPort(srcIP.String(), srcPort)
	var dstAddrString = utils.HostPort(dstIP.String(), dstPort)
	var hash = p.flowhash(networkStr, srcAddrString, dstAddrString)
	var flow *TrafficFlow

	if ret, ok := p.pool.Load(hash); !ok {
		var fitFlow = func(flow *TrafficFlow) {
			flow.Hash = hash
			flow.IsIpv4 = isIpv4
			flow.IsIpv6 = isIpv6
			flow.ClientConn.localPort = srcPort
			flow.ClientConn.localIP = srcIP
			flow.ClientConn.remotePort = dstPort
			flow.ClientConn.remoteIP = dstIP
			if ethernetLayer != nil {
				flow.IsEthernetLinkLayer = true
				flow.HardwareSrcMac = ethernetLayer.SrcMAC.String()
				flow.HardwareDstMac = ethernetLayer.DstMAC.String()
			}
		}

		// half open
		if transportLayer.Payload != nil && transportLayer.PSH {
			flow, err := p.NewFlow(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			fitFlow(flow)
			flow.feed(transportLayer)
			flow.IsHalfOpen = true
			p.pool.Store(hash, flow)
			flow.init(p.onFlowCreated, p.onFlowFrameDataFrameReassembled, p.onFlowFrameDataFrameArrived, p.onFlowClosed)
			return
		}

		// SYN && !ACK -> start a conn
		if transportLayer.SYN && !transportLayer.ACK {
			flow, err := p.NewFlow(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			fitFlow(flow)
			p.pool.Store(hash, flow)
			flow.init(p.onFlowCreated, p.onFlowFrameDataFrameReassembled, p.onFlowFrameDataFrameArrived, p.onFlowClosed)
			return
		}
		return
	} else {
		flow = ret.(*TrafficFlow)
	}
	if flow == nil {
		return
	}
	flow.feed(transportLayer)
}

func (p *TrafficPool) flowhash(netType, srcAddr, dstAddr string) string {
	hashMaterial := []string{netType, srcAddr, dstAddr}
	sort.Strings(hashMaterial)
	return codec.Sha256(strings.Join(hashMaterial, "-"))
}
