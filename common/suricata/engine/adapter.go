package engine

import (
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type AdapterStats struct {
	PhysicalPackets   uint64
	NormalizedPackets uint64
	FlowPackets       uint64
	StreamFrames      uint64
	Transactions      uint64
	FlowsCreated      uint64
	FlowsClosed       uint64
	JoinHits          uint64
	JoinMisses        uint64
	JoinEvictions     uint64
}

type adapterCounters struct {
	physicalPackets   atomic.Uint64
	normalizedPackets atomic.Uint64
	flowPackets       atomic.Uint64
	streamFrames      atomic.Uint64
	transactions      atomic.Uint64
	flowsCreated      atomic.Uint64
	flowsClosed       atomic.Uint64
	joinHits          atomic.Uint64
	joinMisses        atomic.Uint64
	joinEvictions     atomic.Uint64
}

type packetSnapshot struct {
	packet gopacket.Packet
	data   []byte
}

type flowDirectionKey struct {
	flow      FlowKey
	direction int
}

type PcapAdapter struct {
	onEvent func(InspectionEvent)
	maxJoin int

	joinMu     sync.Mutex
	join       map[pcaputil.PacketReference]packetSnapshot
	joinOrder  []pcaputil.PacketReference
	flowPacket map[flowDirectionKey]packetSnapshot
	counters   adapterCounters
}

func NewPcapAdapter(maxPendingJoins int, onEvent func(InspectionEvent)) (*PcapAdapter, error) {
	if onEvent == nil {
		return nil, fmt.Errorf("nil inspection callback")
	}
	if maxPendingJoins <= 0 {
		maxPendingJoins = 8192
	}
	return &PcapAdapter{
		onEvent:    onEvent,
		maxJoin:    maxPendingJoins,
		join:       make(map[pcaputil.PacketReference]packetSnapshot),
		flowPacket: make(map[flowDirectionKey]packetSnapshot),
	}, nil
}

func (a *PcapAdapter) Options() []pcaputil.CaptureOption {
	return []pcaputil.CaptureOption{
		pcaputil.WithEveryPacket(a.onPhysicalPacket),
		pcaputil.WithBeforeTransportPacket(a.onNormalizedPacket),
		pcaputil.WithOnTrafficFlowCreated(a.onFlowCreated),
		pcaputil.WithOnTrafficFlowPacket(a.onFlowPacket),
		pcaputil.WithOnTrafficFlowOnDataFrameArrived(a.onStreamFrame),
		pcaputil.WithOnTrafficFlowClosed(a.onFlowClosed),
		pcaputil.WithOnProtocolMessage(a.onProtocolEvent),
	}
}

func (a *PcapAdapter) Stats() AdapterStats {
	return AdapterStats{
		PhysicalPackets:   a.counters.physicalPackets.Load(),
		NormalizedPackets: a.counters.normalizedPackets.Load(),
		FlowPackets:       a.counters.flowPackets.Load(),
		StreamFrames:      a.counters.streamFrames.Load(),
		Transactions:      a.counters.transactions.Load(),
		FlowsCreated:      a.counters.flowsCreated.Load(),
		FlowsClosed:       a.counters.flowsClosed.Load(),
		JoinHits:          a.counters.joinHits.Load(),
		JoinMisses:        a.counters.joinMisses.Load(),
		JoinEvictions:     a.counters.joinEvictions.Load(),
	}
}

func (a *PcapAdapter) onPhysicalPacket(packet gopacket.Packet) {
	a.counters.physicalPackets.Add(1)
	a.emitPacket(ViewPhysicalPacket, packet)
}

func (a *PcapAdapter) onNormalizedPacket(packet gopacket.Packet) {
	a.counters.normalizedPackets.Add(1)
	snapshot := ownPacket(packet)
	refs := pcaputil.CapturePacketReferences(packet)
	if len(refs) == 0 {
		ref := pcaputil.CapturePacketEvidence(packet)
		if ref.Number != 0 {
			refs = []pcaputil.PacketReference{ref}
		}
	}
	for _, ref := range refs {
		a.remember(ref, snapshot)
	}
	if packet.Layer(layers.LayerTypeTCP) != nil {
		return
	}
	a.onEvent(InspectionEvent{
		View:       ViewNormalizedPacket,
		Timestamp:  packet.Metadata().Timestamp,
		Packet:     snapshot.packet,
		PacketData: append([]byte(nil), snapshot.data...),
		References: append([]pcaputil.PacketReference(nil), refs...),
	})
}

func (a *PcapAdapter) emitPacket(view InspectionView, packet gopacket.Packet) {
	snapshot := ownPacket(packet)
	a.onEvent(InspectionEvent{
		View:       view,
		Timestamp:  packet.Metadata().Timestamp,
		Packet:     snapshot.packet,
		PacketData: snapshot.data,
		References: append([]pcaputil.PacketReference(nil), pcaputil.CapturePacketReferences(packet)...),
	})
}

func (a *PcapAdapter) onFlowCreated(flow *pcaputil.TrafficFlow) {
	a.counters.flowsCreated.Add(1)
	a.onEvent(InspectionEvent{View: ViewFlowCreated, Flow: snapshotFlow(flow)})
}

func (a *PcapAdapter) onFlowPacket(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, timestamp time.Time) {
	a.counters.flowPackets.Add(1)
	event := InspectionEvent{
		View:       ViewNormalizedPacket,
		Timestamp:  timestamp,
		Flow:       snapshotFlow(flow),
		Direction:  flowDirection(flow, conn),
		References: append([]pcaputil.PacketReference(nil), refs...),
	}
	for _, ref := range refs {
		if snapshot, ok := a.take(ref); ok {
			event.Packet = snapshot.packet
			event.PacketData = snapshot.data
			a.counters.joinHits.Add(1)
			break
		}
	}
	if event.Packet == nil {
		a.counters.joinMisses.Add(1)
	} else {
		a.joinMu.Lock()
		a.flowPacket[flowDirectionKey{flow: event.Flow.Key, direction: event.Direction}] = packetSnapshot{packet: event.Packet, data: append([]byte(nil), event.PacketData...)}
		a.joinMu.Unlock()
	}
	a.onEvent(event)
}

func (a *PcapAdapter) onStreamFrame(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
	a.counters.streamFrames.Add(1)
	flowSnapshot := snapshotFlow(flow)
	direction := flowDirection(flow, conn)
	a.joinMu.Lock()
	header := a.flowPacket[flowDirectionKey{flow: flowSnapshot.Key, direction: direction}]
	a.joinMu.Unlock()
	a.onEvent(InspectionEvent{
		View:       ViewStream,
		Timestamp:  frame.Timestamp,
		Flow:       flowSnapshot,
		Direction:  direction,
		Packet:     header.packet,
		PacketData: append([]byte(nil), header.data...),
		Payload:    append([]byte(nil), frame.Payload...),
		Sequence:   frame.Seq,
		References: frame.PacketReferences(),
	})
}

func (a *PcapAdapter) onFlowClosed(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
	a.counters.flowsClosed.Add(1)
	snapshot := snapshotFlow(flow)
	snapshot.PendingGaps = flow.HasPendingGaps()
	a.onEvent(InspectionEvent{View: ViewFlowClosed, Flow: snapshot, CloseReason: reason})
	a.joinMu.Lock()
	delete(a.flowPacket, flowDirectionKey{flow: snapshot.Key, direction: 0})
	delete(a.flowPacket, flowDirectionKey{flow: snapshot.Key, direction: 1})
	a.joinMu.Unlock()
}

func (a *PcapAdapter) onProtocolEvent(event *pcaputil.ProtocolEvent) {
	if event == nil {
		return
	}
	a.counters.transactions.Add(1)
	owned := *event
	owned.Raw = append([]byte(nil), event.Raw...)
	owned.SourceBytes.PacketRefs = append([]pcaputil.PacketReference(nil), event.SourceBytes.PacketRefs...)
	a.onEvent(InspectionEvent{
		View:       ViewTransaction,
		Timestamp:  event.Timestamp,
		Flow:       FlowSnapshot{Key: FlowKey{Domain: event.Domain, Generation: event.TCPFlowID}},
		Direction:  event.Direction,
		Payload:    append([]byte(nil), event.Raw...),
		References: append([]pcaputil.PacketReference(nil), event.SourceBytes.PacketRefs...),
		Protocol:   &owned,
	})
}

func (a *PcapAdapter) remember(ref pcaputil.PacketReference, snapshot packetSnapshot) {
	if ref.Number == 0 {
		return
	}
	a.joinMu.Lock()
	defer a.joinMu.Unlock()
	if _, exists := a.join[ref]; !exists {
		a.joinOrder = append(a.joinOrder, ref)
	}
	a.join[ref] = snapshot
	for len(a.join) > a.maxJoin && len(a.joinOrder) > 0 {
		oldest := a.joinOrder[0]
		a.joinOrder = a.joinOrder[1:]
		if _, exists := a.join[oldest]; exists {
			delete(a.join, oldest)
			a.counters.joinEvictions.Add(1)
		}
	}
}

func (a *PcapAdapter) take(ref pcaputil.PacketReference) (packetSnapshot, bool) {
	a.joinMu.Lock()
	defer a.joinMu.Unlock()
	snapshot, ok := a.join[ref]
	if ok {
		delete(a.join, ref)
	}
	if len(a.joinOrder) > a.maxJoin*2 {
		compacted := a.joinOrder[:0]
		for _, candidate := range a.joinOrder {
			if _, exists := a.join[candidate]; exists {
				compacted = append(compacted, candidate)
			}
		}
		a.joinOrder = compacted
	}
	return snapshot, ok
}

func ownPacket(packet gopacket.Packet) packetSnapshot {
	data := append([]byte(nil), packet.Data()...)
	decoder := gopacket.Decoder(layers.LayerTypeEthernet)
	if link := packet.LinkLayer(); link != nil {
		decoder = link.LayerType()
	} else if network := packet.NetworkLayer(); network != nil {
		decoder = network.LayerType()
	}
	owned := gopacket.NewPacket(data, decoder, gopacket.Default)
	owned.Metadata().CaptureInfo = packet.Metadata().CaptureInfo
	return packetSnapshot{packet: owned, data: data}
}

func snapshotFlow(flow *pcaputil.TrafficFlow) FlowSnapshot {
	if flow == nil {
		return FlowSnapshot{}
	}
	return FlowSnapshot{
		Key:      FlowKey{Domain: flow.CaptureDomain(), Generation: flow.Index},
		First:    snapshotEndpoint(flow.ClientConn),
		Second:   snapshotEndpoint(flow.ServerConn),
		IPv4:     flow.IsIpv4,
		IPv6:     flow.IsIpv6,
		HalfOpen: flow.IsHalfOpen,
	}
}

func snapshotEndpoint(conn *pcaputil.TrafficConnection) Endpoint {
	if conn == nil {
		return Endpoint{}
	}
	address, _ := netip.ParseAddr(conn.LocalIP().String())
	return Endpoint{Address: address, Port: uint16(conn.LocalPort())}
}

func flowDirection(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection) int {
	if flow != nil && conn == flow.ServerConn {
		return 1
	}
	return 0
}
