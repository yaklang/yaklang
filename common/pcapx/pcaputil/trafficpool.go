package pcaputil

import (
	"context"
	"github.com/ReneKroon/ttlcache"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type trafficPool struct {
	// map<string, *TrafficConnection>
	pool               *sync.Map
	ctx                context.Context
	currentStreamIndex uint64

	flowCache *ttlcache.Cache

	onFlowCreated func(flow *TrafficFlow)
}

func NewTrafficPool(ctx context.Context) *trafficPool {
	pool := &trafficPool{pool: new(sync.Map), ctx: ctx}
	fCache := ttlcache.NewCache()
	fCache.SetExpirationCallback(func(key string, value interface{}) {
		pool.pool.Delete(key)
		flow, ok := value.(*TrafficFlow)
		if !ok {
			return
		}
		flow.cancel()
		flow.ServerConn.Close()
		flow.ClientConn.Close()
	})
	fCache.SetTTL(time.Minute)
	pool.flowCache = fCache
	return pool
}

func (p *trafficPool) nextStream() uint64 {
	return atomic.AddUint64(&p.currentStreamIndex, 1)
}

func (p *trafficPool) Feed(networkLayerFlow gopacket.Flow, networkLayer gopacket.SerializableLayer, transportLayer *layers.TCP) {
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
	var hash = p.flowhash(networkStr, srcAddrString, dstAddrString)
	var flow *TrafficFlow
	if ret, ok := p.pool.Load(hash); !ok {
		// no reason  ...
		if transportLayer.Payload != nil && transportLayer.PSH {
			flow, err := p.NewFlow(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			flow.Hash = hash
			flow.ClientConn.localPort = srcPort
			flow.ClientConn.remotePort = dstPort
			flow.Feed(transportLayer)
			p.pool.Store(hash, flow)
			if p.onFlowCreated != nil {
				p.onFlowCreated(flow)
			}
			return
		}
		// SYN && !ACK -> start a conn
		if transportLayer.SYN && !transportLayer.ACK {
			flow, err := p.NewFlow(networkStr, srcAddrString, dstAddrString)
			if err != nil {
				log.Errorf("create new connection failed: %s", err)
				return
			}
			flow.Hash = hash
			flow.ClientConn.localPort = srcPort
			flow.ClientConn.remotePort = dstPort
			p.pool.Store(hash, flow)
			if p.onFlowCreated != nil {
				p.onFlowCreated(flow)
			}
			return
		}
		return
	} else {
		flow = ret.(*TrafficFlow)
	}
	if flow == nil {
		return
	}
	flow.Feed(transportLayer)
}

func (p *trafficPool) flowhash(netType, srcAddr, dstAddr string) string {
	hashMaterial := []string{netType, srcAddr, dstAddr}
	sort.Strings(hashMaterial)
	return codec.Sha256(strings.Join(hashMaterial, "-"))
}
