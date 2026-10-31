package pcaputil

import (
	"time"

	"github.com/gopacket/gopacket/layers"
)

// One payload-free candidate per flow. Every retained opening observation uses
// a pending-segment slot, including retransmissions, before allocating metadata.
// Packet buffers, option slices and fragment-reference arrays are never retained.
type tcpOpening struct {
	sequence      uint32
	window        uint16
	direction     int
	hardwareKnown bool
	hardwareSrc   string
	hardwareDst   string
	observations  []tcpOpeningObservation
}

type tcpOpeningObservation struct {
	captured time.Time
	ref      PacketReference
}

func (f *TrafficFlow) pendingOpeningSegments(sender *TrafficConnection) int {
	if f.opening == nil || (f.opening.direction == 0) != (sender == f.ClientConn) {
		return 0
	}
	return len(f.opening.observations)
}

func (f *TrafficFlow) openingMatches(sender *TrafficConnection, tcp *layers.TCP) bool {
	return f.opening != nil && tcp.SYN && !tcp.ACK && f.opening.sequence == tcp.Seq && (f.opening.direction == 0) == (sender == f.ClientConn)
}

// Closing a direction or a retained tuple does not dispose its candidate: a
// delayed old FIN/RST can arrive between the new SYN and its confirmation.
// Disposal, replacement and successful handoff each release these slots once.
func (f *TrafficFlow) discardOpening() {
	if candidate := f.opening; candidate != nil {
		f.opening = nil
		if f.pool != nil {
			f.pool.releasePending(0, len(candidate.observations))
		}
	}
}

func (f *TrafficFlow) observeOpening(ethernet *layers.Ethernet, sender *TrafficConnection, tcp *layers.TCP, evidence captureEvidence, ts time.Time) (*tcpOpening, bool) {
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
		if len(tcp.Payload) != 0 || evidence.tcpMissingTail != 0 || len(evidence.refs) > 1 || len(evidence.parents) != 0 || evidence.carrierOwned {
			f.discardOpening()
			return nil, true
		}
		ref := evidence.Ref
		if len(evidence.refs) == 1 {
			ref = evidence.refs[0]
		}
		if ref.Domain != f.key.domain {
			f.discardOpening()
			return nil, true
		}
		if !f.openingMatches(sender, tcp) {
			f.discardOpening()
		}
		if f.opening != nil {
			last := f.opening.observations[len(f.opening.observations)-1]
			if last.ref == ref && last.captured.Equal(ts) {
				return nil, true
			}
		}
		p := f.pool
		if len(sender.waitGroup)+f.pendingOpeningSegments(sender) >= p.options.MaxPendingSegments || !p.reservePending(0, 1) {
			f.discardOpening()
			if p.counters != nil {
				p.counters.limits.Add(1)
			} else {
				p.singleDiagnostics.limits.Add(1)
			}
			p.reassemblyFailure("TCP opening observation metadata exceeds the pending-segment budget")
			return nil, true
		}
		if f.opening == nil {
			f.opening = &tcpOpening{sequence: tcp.Seq, window: tcp.Window, direction: direction}
			if ethernet != nil && len(ethernet.SrcMAC) == 6 && len(ethernet.DstMAC) == 6 {
				f.opening.hardwareKnown = true
				f.opening.hardwareSrc, f.opening.hardwareDst = ethernet.SrcMAC.String(), ethernet.DstMAC.String()
			}
		}
		f.opening.observations = append(f.opening.observations, tcpOpeningObservation{captured: ts, ref: ref})
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
	if tcp.ACK && tcp.Seq != sender.isn {
		// An unsupported/over-budget opening has no preserved origin. Its
		// later unconfirmed SYN-ACK cannot poison the established cursor.
		return nil, true
	}
	return nil, false
}
