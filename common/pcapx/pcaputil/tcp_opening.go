package pcaputil

import (
	"time"

	"github.com/gopacket/gopacket/layers"
)

// One fixed-size, payload-free candidate per flow. It never owns packet buffers,
// option slices or fragment-reference arrays, and cannot evict an established
// parser until the opposite endpoint supplies a matching new SYN-ACK.
type tcpOpening struct {
	sequence  uint32
	window    uint16
	direction int
	captured  time.Time
	ref       PacketReference
}

func (f *TrafficFlow) observeOpening(sender *TrafficConnection, tcp *layers.TCP, evidence captureEvidence, ts time.Time) (*tcpOpening, bool) {
	if !f.ClientConn.synSeen || !f.ServerConn.synSeen {
		return nil, false
	}
	direction, peer := 0, f.ServerConn
	if sender == f.ServerConn {
		direction, peer = 1, f.ClientConn
	}
	if !tcp.ACK && tcp.Seq != sender.isn {
		// TFO payload and multi-frame/tunneled carrier ownership need their
		// own retained-byte accounting. They do not certify this profile.
		if len(tcp.Payload) == 0 && evidence.tcpMissingTail == 0 && len(evidence.refs) <= 1 && len(evidence.parents) == 0 && !evidence.carrierOwned {
			if f.opening == nil || f.opening.sequence != tcp.Seq || f.opening.direction != direction {
				ref := evidence.Ref
				if len(evidence.refs) == 1 {
					ref = evidence.refs[0]
				}
				f.opening = &tcpOpening{sequence: tcp.Seq, window: tcp.Window, direction: direction, captured: ts, ref: ref}
			}
		}
		return nil, true
	}
	if candidate := f.opening; candidate != nil && tcp.ACK {
		if direction != candidate.direction && tcp.Ack == candidate.sequence+1 && tcp.Seq != sender.isn && len(tcp.Payload) == 0 && evidence.tcpMissingTail == 0 {
			return candidate, true
		}
		// Exact old-handshake retransmissions still belong to the old flow.
		// An unrelated SYN-ACK cannot damage that flow's receive cursor.
		if tcp.Seq != sender.isn || tcp.Ack != peer.isn+1 {
			return nil, true
		}
	}
	return nil, false
}
