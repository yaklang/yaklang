package engine

import (
	"bytes"
	"net/netip"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// udpTupleKey identifies one UDP pseudo-flow independent of packet direction.
type udpTupleKey struct {
	Domain pcaputil.CaptureDomain
	A      string
	B      string
}

func canonicalUDPTuple(domain pcaputil.CaptureDomain, first, second netip.AddrPort) udpTupleKey {
	a, b := first.String(), second.String()
	if a > b {
		a, b = b, a
	}
	return udpTupleKey{Domain: domain, A: a, B: b}
}

// attachUDPFlow turns a normalized UDP packet into a stable pseudo-flow. The
// first observed endpoint pair defines packet direction; DNS QR identifies the
// semantic client direction, while other UDP traffic keeps the first-packet
// fallback. Reverse packets reuse the same FlowKey and set direction 1.
func (e *StatefulEngine) attachUDPFlow(event InspectionEvent) InspectionEvent {
	if event.View != ViewNormalizedPacket || event.Packet == nil || event.Flow.Key.Generation != 0 {
		return event
	}
	udp, ok := event.Packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if !ok {
		return event
	}

	var src, dst netip.Addr
	var ipv4, ipv6 bool
	switch network := event.Packet.NetworkLayer().(type) {
	case *layers.IPv4:
		src, _ = netip.AddrFromSlice(network.SrcIP)
		dst, _ = netip.AddrFromSlice(network.DstIP)
		ipv4 = true
	case *layers.IPv6:
		src, _ = netip.AddrFromSlice(network.SrcIP)
		dst, _ = netip.AddrFromSlice(network.DstIP)
		ipv6 = true
	default:
		return event
	}
	src, dst = src.Unmap(), dst.Unmap()
	if !src.IsValid() || !dst.IsValid() {
		return event
	}

	srcPort, dstPort := uint16(udp.SrcPort), uint16(udp.DstPort)
	first := netip.AddrPortFrom(src, srcPort)
	second := netip.AddrPortFrom(dst, dstPort)
	domain := eventUDPDomain(event)
	tuple := canonicalUDPTuple(domain, first, second)
	now := eventTime(event)

	e.flowsMu.Lock()
	key, exists := e.udpIndex[tuple]
	var flow *flowRuntime
	if exists {
		flow = e.flows[key]
	}
	if flow == nil {
		if len(e.flows) >= e.maxFlows {
			e.flowsMu.Unlock()
			return event
		}
		key = FlowKey{Domain: domain, Generation: e.udpGeneration.Add(1)}
		flow = &flowRuntime{
			transport:       uint8(layers.IPProtocolUDP),
			clientDirection: udpClientDirection(event, udp),
			roleConfirmed:   true,
			udpTuple:        tuple,
			streamCursor:    make(map[ruleDirection]int),
			snapshot: FlowSnapshot{
				Key:        key,
				First:      Endpoint{Address: src, Port: srcPort},
				Second:     Endpoint{Address: dst, Port: dstPort},
				IPv4:       ipv4,
				IPv6:       ipv6,
				IPProtocol: uint8(layers.IPProtocolUDP),
			},
		}
		e.flows[key] = flow
		e.udpIndex[tuple] = key
	}

	direction := 0
	if src == flow.snapshot.Second.Address && srcPort == flow.snapshot.Second.Port &&
		dst == flow.snapshot.First.Address && dstPort == flow.snapshot.First.Port {
		direction = 1
	}
	flow.mu.Lock()
	flow.observeUDP(direction, now)
	snapshot := flow.snapshot
	flow.mu.Unlock()
	e.flowsMu.Unlock()

	event.Flow = snapshot
	event.Direction = direction
	return event
}

func udpClientDirection(event InspectionEvent, udp *layers.UDP) int {
	layer := event.Packet.Layer(layers.LayerTypeDNS)
	dns, ok := layer.(*layers.DNS)
	if !ok || !bytes.Equal(dns.LayerContents(), udp.Payload) {
		return 0
	}
	if dns.QR {
		return 1
	}
	return 0
}

func eventUDPDomain(event InspectionEvent) pcaputil.CaptureDomain {
	for _, reference := range event.References {
		return reference.Domain
	}
	return event.Flow.Key.Domain
}

// cleanupUDPFlows removes idle UDP pseudo-flows and their state-store entries.
func (e *StatefulEngine) cleanupUDPFlows(now time.Time) {
	e.flowsMu.Lock()
	var expired []FlowSnapshot
	for key, flow := range e.flows {
		flow.mu.Lock()
		transport := flow.transport
		lastSeen := flow.lastSeen
		snapshot := flow.snapshot
		flow.mu.Unlock()
		if transport != uint8(layers.IPProtocolUDP) {
			continue
		}
		if lastSeen.IsZero() || now.Before(lastSeen) || now.Sub(lastSeen) <= e.udpTimeout {
			continue
		}
		if e.flows[key] != flow {
			continue
		}
		delete(e.flows, key)
		if e.udpIndex[flow.udpTuple] == key {
			delete(e.udpIndex, flow.udpTuple)
		}
		expired = append(expired, snapshot)
	}
	e.flowsMu.Unlock()

	for _, snapshot := range expired {
		if key, err := stateFlowKey(e.epoch, snapshot); err == nil {
			_, _ = e.store.DeleteFlow(key)
		}
	}
}
