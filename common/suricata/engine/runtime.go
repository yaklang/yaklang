package engine

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type flowRuntime struct {
	mu sync.Mutex

	transport       uint8
	clientDirection int
	synDirection    int
	synSequence     uint32
	synAckSequence  uint32
	synSeen         bool
	synAckSeen      bool
	established     bool
	roleConfirmed   bool
	closed          bool
	incomplete      bool
	stream          [2][]byte
	streamOverflow  [2]bool
	streamCursor    map[ruleDirection]int
	httpNextOffset  [2]int

	// UDP pseudo-flow state. udpTuple and snapshot are immutable after the
	// flow is created; lastSeen drives idle eviction.
	seenDirections [2]bool
	lastSeen       time.Time
	udpTuple       udpTupleKey
	snapshot       FlowSnapshot
}

type ruleDirection struct {
	ruleIndex int
	direction int
}

func (f *flowRuntime) observeTCP(direction int, tcp *layers.TCP) {
	if tcp == nil {
		return
	}
	if tcp.SYN && !tcp.ACK {
		if !f.synSeen {
			f.synDirection = direction
			f.synSequence = tcp.Seq
			f.synSeen = true
			f.roleConfirmed = true
		}
		return
	}
	if f.synSeen && tcp.SYN && tcp.ACK && direction != f.synDirection && tcp.Ack == f.synSequence+1 {
		f.synAckSequence = tcp.Seq
		f.synAckSeen = true
		return
	}
	if f.synSeen && f.synAckSeen && tcp.ACK && !tcp.SYN && direction == f.synDirection && tcp.Ack == f.synAckSequence+1 {
		f.established = true
	}
	if tcp.FIN || tcp.RST {
		f.closed = true
	}
}

func (f *flowRuntime) toServer(direction int) (bool, bool) {
	if !f.roleConfirmed {
		return false, false
	}
	if f.transport == uint8(layers.IPProtocolUDP) {
		return direction == f.clientDirection, true
	}
	return direction == f.synDirection, true
}

func (f *flowRuntime) observeUDP(direction int, now time.Time) {
	if direction < 0 || direction > 1 {
		return
	}
	if !f.roleConfirmed {
		f.clientDirection = 0
		f.roleConfirmed = true
	}
	f.seenDirections[direction] = true
	f.lastSeen = now
	f.established = f.seenDirections[0] && f.seenDirections[1]
}

func stateFlowKey(epoch uint64, flow FlowSnapshot) (state.FlowKey, error) {
	first := netip.AddrPortFrom(flow.First.Address, flow.First.Port)
	second := netip.AddrPortFrom(flow.Second.Address, flow.Second.Port)
	if !first.IsValid() || !second.IsValid() || flow.Key.Generation == 0 {
		return state.FlowKey{}, fmt.Errorf("invalid flow snapshot")
	}
	protocol := flow.IPProtocol
	if protocol == 0 {
		protocol = uint8(layers.IPProtocolTCP)
	}
	domain := state.CaptureDomain{
		Sensor:    flow.Key.Domain.Encapsulation,
		Interface: fmt.Sprint(flow.Key.Domain.Interface),
		ID:        uint64(flow.Key.Domain.Section),
	}
	return state.NewFlowKey(epoch, domain, flow.Key.Generation, protocol, first, second), nil
}
