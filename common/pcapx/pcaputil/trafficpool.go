package pcaputil

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type TrafficPool struct {
	ctx           context.Context
	captureConf   *CaptureConfig
	flowCache     *utils.Cache[*TrafficFlow]
	onFlowCreated func(flow *TrafficFlow)
	onFlowClosed  func(reason TrafficFlowCloseReason, flow *TrafficFlow)
	// internal field, not for user
	_onHTTPFlow                     func(flow *TrafficFlow, r *http.Request, response *http.Response)
	onFlowFrameDataFrameArrived     []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
	onFlowFrameDataFrameReassembled []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
	currentStreamIndex              uint64
}

func NewTrafficPool(ctx context.Context) *TrafficPool {
	pool := &TrafficPool{ctx: ctx}
	fCache := utils.NewTTLCache[*TrafficFlow](30 * time.Second)
	fCache.SetExpirationCallback(func(key string, flow *TrafficFlow) {
		flow.Close()
	})
	pool.flowCache = fCache
	return pool
}

func (p *TrafficPool) AddWaitGroupDelta(delta int) {
	p.captureConf.wg.Add(delta)
}

func (p *TrafficPool) Done() {
	p.captureConf.wg.Done()
}

func (p *TrafficPool) nextStream() uint64 {
	return atomic.AddUint64(&p.currentStreamIndex, 1)
}

func (p *TrafficPool) Feed(ethernetLayer *layers.Ethernet, networkLayer gopacket.SerializableLayer, transportLayer *layers.TCP, tss ...time.Time) {
	var networkStr string
	var srcIP net.IP
	var dstIP net.IP
	srcPort := int(transportLayer.SrcPort)
	dstPort := int(transportLayer.DstPort)
	isIpv4 := false
	isIpv6 := false

	ts := time.Now()
	if len(tss) > 0 {
		ts = tss[0]
	}

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

	srcAddrString := utils.HostPort(srcIP.String(), srcPort)
	dstAddrString := utils.HostPort(dstIP.String(), dstPort)
	hash := p.flowhash(networkStr, srcAddrString, dstAddrString)
	var (
		flow *TrafficFlow
		ok   bool
	)

	if flow, ok = p.flowCache.Get(hash); !ok {
		fitFlow := func(flow *TrafficFlow) {
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
			flow.feed(transportLayer, ts)
			flow.IsHalfOpen = true
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
			flow.init(p.onFlowCreated, p.onFlowFrameDataFrameReassembled, p.onFlowFrameDataFrameArrived, p.onFlowClosed)
			return
		}
		return
	}

	if flow == nil {
		return
	}
	flow.feed(transportLayer, ts)
}

func (p *TrafficPool) flowhash(netType, srcAddr, dstAddr string) string {
	hashMaterial := []string{netType, srcAddr, dstAddr}
	sort.Strings(hashMaterial)
	return codec.Sha256(strings.Join(hashMaterial, "-"))
}
