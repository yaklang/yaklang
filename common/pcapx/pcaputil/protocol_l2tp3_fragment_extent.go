package pcaputil

import (
	"fmt"

	"github.com/gopacket/gopacket"
)

// Created only at the immediate completion of a direct IPv6->Fragment->115
// datagram. It is not serialized, propagated through new tunnel packets, or
// inferred from public event fields/SID. The first range was actually accepted.
type l2tp3IPv6FragmentPacket struct {
	gopacket.Packet
	firstPayloadBytes int
}

type l2tp3FirstHeaderExtent struct{ firstPayloadBytes int }

// Caller holds l2tp3Mu and already matched exactly one receiver binding using
// complete reassembled cookie, domain, receiver, source peer, SID and transport.
// Check the selected data-header extent before FlowID, sequence or child bytes
// are committed. Unknown/retired/unselected contexts keep their current policy.
func l2tp3SelectedFirstHeaderError(v *l2tp3Session, b l2tp3Binding, first []l2tp3FirstHeaderExtent, m map[string]any) error {
	if len(first) != 1 || b.unknownCookie || v.phase != 3 || !v.proposal[0] || !v.proposal[1] || v.c.closed || v.closed || v.c.refusal != "" || v.refusal != "" {
		return nil
	}
	p := v.props[b.receiver]
	if len(p.Cookie) != 0 && len(p.Cookie) != 4 && len(p.Cookie) != 8 || p.Sublayer > 1 || p.Sequencing > 2 || p.Sequencing != 0 && p.Sublayer != 1 || p.Pseudowire != 4 && p.Pseudowire != 5 {
		return nil
	}
	required := 4 + len(p.Cookie)
	if p.Sublayer == 1 {
		required += 4
	}
	if first[0].firstPayloadBytes >= required {
		return nil
	}
	m["IPv6 First Fragment Payload Bytes"] = first[0].firstPayloadBytes
	m["Required L2TP Data Header Bytes"] = required
	m["First Fragment Header Scope"] = "private unique current receiver cookie/sublayer profile; no child admission"
	m["Payload Decoded"] = false
	return protocolError(ErrMalformedMessage, fmt.Sprintf("IPv6 first fragment has %d of %d selected L2TPv3 data header bytes", first[0].firstPayloadBytes, required))
}
