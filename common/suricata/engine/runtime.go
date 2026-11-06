package engine

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type flowTag struct {
	plan      *CompiledRulePlan
	remaining uint64
	expiresAt time.Time
	unit      string
}

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
	tlsMatched      map[ruleDirection]bool
	httpNextOffset  [2]int
	tags            []flowTag

	// tcpPacketBytes counts unique TCP payload bytes by sequence number. It
	// backs stream_size for tcp-pkt rules, where stream frames arrive after the
	// packet event that triggered the rule.
	tcpPacketBytes [2]uint64
	tcpNextSeq     [2]uint32
	tcpSeqKnown    [2]bool

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
	f.observeTCPBytes(direction, tcp)
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

func (f *flowRuntime) observeTCPBytes(direction int, tcp *layers.TCP) {
	if tcp == nil || direction < 0 || direction > 1 {
		return
	}
	payload := tcp.Payload
	if tcp.SYN {
		next := tcp.Seq + 1
		f.tcpNextSeq[direction] = next
		f.tcpSeqKnown[direction] = true
		if len(payload) > 0 {
			f.tcpPacketBytes[direction] += uint64(len(payload))
			f.tcpNextSeq[direction] = next + uint32(len(payload))
		}
		return
	}
	if len(payload) == 0 {
		return
	}
	if !f.tcpSeqKnown[direction] {
		f.tcpNextSeq[direction] = tcp.Seq
		f.tcpSeqKnown[direction] = true
	}
	next := f.tcpNextSeq[direction]
	if int32(tcp.Seq-next) >= 0 {
		f.tcpPacketBytes[direction] += uint64(len(payload))
		f.tcpNextSeq[direction] = tcp.Seq + uint32(len(payload))
		return
	}
	end := tcp.Seq + uint32(len(payload))
	if overlap := int32(end - next); overlap > 0 {
		f.tcpPacketBytes[direction] += uint64(overlap)
		f.tcpNextSeq[direction] = end
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
	return state.NewFlowKey(epoch, stateCaptureDomain(flow.Key.Domain), flow.Key.Generation, protocol, first, second), nil
}

func stateCaptureDomain(domain pcaputil.CaptureDomain) state.CaptureDomain {
	return state.CaptureDomain{
		Sensor:    domain.Encapsulation,
		Interface: fmt.Sprint(domain.Interface),
		ID:        uint64(domain.Section),
	}
}
