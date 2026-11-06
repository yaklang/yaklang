package engine

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func (p *CompiledRulePlan) acceptsView(view InspectionView) bool {
	if p.Rule != nil {
		if rule.UsesRawBytes(p.Rule) {
			return view == ViewNormalizedPacket
		}
		if len(p.StreamSize) > 0 && p.Rule.Protocol != "tcp-pkt" {
			return view == ViewStream
		}
		switch p.Rule.Protocol {
		case "http":
			// Stateful HTTP rules inspect the ordered per-direction TCP stream
			// so split requests/responses are parsed as one message. Explicit
			// stateless/no_stream rules stay on the packet lane.
			if p.Flow != nil && !p.Flow.Stateless && !p.Flow.NoStream {
				return view == ViewStream
			}
			return view == ViewNormalizedPacket
		case "tcp", "smtp", "ftp", "ftp-data", "smb", "ssh", "dcerpc", "tcp-stream":
			// handled below
		case "tls":
			// Stateful TLS handshake fields (SNI, JA3, certificates) are parsed
			// from the ordered stream so split records are visible. Explicit
			// stateless/no_stream rules keep the packet view.
			if p.Flow != nil && !p.Flow.Stateless && !p.Flow.NoStream {
				return view == ViewStream
			}
			return view == ViewNormalizedPacket
		case "tcp-pkt":
			// tcp-pkt is explicitly packet-scoped, including stateless rules
			// and rules without payload content.
			return view == ViewNormalizedPacket
		default:
			// DNS/UDP/ICMP are evaluated from packet buffers.
			return view == ViewNormalizedPacket
		}
	}
	if p.Flow != nil {
		if p.Flow.OnlyStream {
			return view == ViewStream
		}
		if p.Flow.NoStream || p.Flow.Stateless {
			return view == ViewNormalizedPacket
		}
	}
	if hasPayloadRule(p.Rule) {
		return view == ViewStream
	}
	return view == ViewNormalizedPacket
}

func (p *CompiledRulePlan) matchStreamSize(flow *flowRuntime, packetView bool) bool {
	if len(p.StreamSize) == 0 {
		return true
	}
	if flow == nil || !flow.roleConfirmed {
		return false
	}
	if flow.transport != 0 && flow.transport != uint8(layers.IPProtocolTCP) {
		return false
	}
	var clientBytes, serverBytes int
	if packetView {
		clientBytes = int(flow.tcpPacketBytes[flow.synDirection])
		serverBytes = int(flow.tcpPacketBytes[1-flow.synDirection])
	} else {
		clientBytes = len(flow.stream[flow.synDirection])
		serverBytes = len(flow.stream[1-flow.synDirection])
	}
	for _, metric := range p.StreamSize {
		if metric == nil || metric.Range == nil {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(metric.Direction)) {
		case "client":
			if !metric.Range.Match(clientBytes) {
				return false
			}
		case "server":
			if !metric.Range.Match(serverBytes) {
				return false
			}
		case "both":
			if !metric.Range.Match(clientBytes) || !metric.Range.Match(serverBytes) {
				return false
			}
		case "either":
			if !metric.Range.Match(clientBytes) && !metric.Range.Match(serverBytes) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (p *CompiledRulePlan) match(event InspectionEvent, flow *flowRuntime, streamPayload []byte) (bool, int, error) {
	matcher := p.acquireMatcher()
	defer p.releaseMatcher(matcher)
	switch event.View {
	case ViewNormalizedPacket:
		if event.Packet == nil {
			return false, 0, fmt.Errorf("packet reference could not be joined to a flow")
		}
		matched, err := matcher.MatchPackageWithError(event.Packet)
		return matched, 0, err
	case ViewStream:
		if event.Packet == nil {
			return false, 0, fmt.Errorf("stream has no associated header packet")
		}
		if flow == nil || event.Direction < 0 || event.Direction > 1 || flow.streamOverflow[event.Direction] {
			return false, 0, nil
		}
		return matcher.MatchStreamEvidence(event.Packet, streamPayload)
	default:
		return false, 0, nil
	}
}

func (p *CompiledRulePlan) hasNewStreamEvidence(stream []byte, cursor int) bool {
	if p.Rule == nil || p.Rule.ContentRuleConfig == nil {
		return false
	}
	for _, content := range p.Rule.ContentRuleConfig.ContentRules {
		if content == nil || len(content.Content) == 0 || content.Negative || content.PCREParsed != nil {
			continue
		}
		start := cursor - len(content.Content) + 1
		if start < 0 {
			start = 0
		}
		haystack, needle := stream[start:], content.Content
		if content.Nocase {
			haystack, needle = bytes.ToLower(haystack), bytes.ToLower(needle)
		}
		if index := bytes.Index(haystack, needle); index >= 0 && start+index+len(needle) > cursor {
			return true
		}
	}
	// PCRE and state-only rules need the full matcher to decide whether new data
	// produces a match; advancing the cursor still prevents old-only replays.
	return cursor < len(stream)
}

func hasPayloadRule(current *rule.Rule) bool {
	return current != nil && current.ContentRuleConfig != nil && len(current.ContentRuleConfig.ContentRules) > 0
}

func matchFlow(config *rule.FlowRule, flow *flowRuntime, direction int) bool {
	if config == nil {
		return true
	}
	if config.Established && !flow.established {
		return false
	}
	if config.NotEstablished && flow.established {
		return false
	}
	if config.ToServer || config.ToClient {
		toServer, known := flow.toServer(direction)
		if !known || config.ToServer && !toServer || config.ToClient && toServer {
			return false
		}
	}
	return true
}
