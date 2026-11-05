package engine

import (
	"net/netip"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type InspectionView uint8

const (
	ViewPhysicalPacket InspectionView = iota + 1
	ViewNormalizedPacket
	ViewStream
	ViewTransaction
	ViewFlowCreated
	ViewFlowClosed
)

type FlowKey struct {
	Domain     pcaputil.CaptureDomain
	Generation uint64
}

type Endpoint struct {
	Address netip.Addr
	Port    uint16
}

type FlowSnapshot struct {
	Key         FlowKey
	First       Endpoint
	Second      Endpoint
	IPv4        bool
	IPv6        bool
	HalfOpen    bool
	PendingGaps bool
}

type InspectionEvent struct {
	View        InspectionView
	Timestamp   time.Time
	Flow        FlowSnapshot
	Direction   int
	Packet      gopacket.Packet
	PacketData  []byte
	Payload     []byte
	Sequence    uint32
	References  []pcaputil.PacketReference
	Protocol    *pcaputil.ProtocolEvent
	CloseReason pcaputil.TrafficFlowCloseReason
}

func (e InspectionEvent) FlowKey() FlowKey { return e.Flow.Key }
