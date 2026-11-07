package engine

import (
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/match"
)

// MatchRaw parses an Ethernet frame and evaluates stateless rules.
func (e *StatefulEngine) MatchRaw(raw []byte) (bool, error) {
	if e == nil || e.closed.Load() || len(raw) == 0 {
		return false, nil
	}
	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.NoCopy)
	return e.MatchPacket(packet)
}

// MatchPacket evaluates stateless rules against one packet. Rules that need a
// flowRuntime (direction, established, flowbits, threshold) are skipped; use
// Handle with flow events for those. Alerts are available through Handle.
func (e *StatefulEngine) MatchPacket(packet gopacket.Packet) (bool, error) {
	if e == nil || e.closed.Load() || packet == nil {
		return false, nil
	}
	matched := false
	for _, plan := range e.plans {
		if !plan.stateless() {
			continue
		}
		matcher := plan.acquireMatcher()
		currentMatched, err := matcher.MatchPackageWithError(packet)
		plan.releaseMatcher(matcher)
		if err != nil {
			return false, fmt.Errorf("sid=%d: %w", plan.Rule.Sid, err)
		}
		if currentMatched {
			matched = true
		}
	}
	return matched, nil
}

// MatchStream evaluates stateless TCP rules against ordered stream bytes while
// retaining packet headers. Non-TCP rules are evaluated by MatchPacket.
func (e *StatefulEngine) MatchStream(packet gopacket.Packet, payload []byte) (bool, error) {
	if e == nil || e.closed.Load() || packet == nil {
		return false, nil
	}
	matched := false
	for _, plan := range e.plans {
		if !plan.stateless() || plan.Rule.Protocol != "tcp" {
			continue
		}
		matcher := plan.acquireMatcher()
		currentMatched, _, err := matcher.MatchStreamEvidence(packet, payload)
		plan.releaseMatcher(matcher)
		if err != nil {
			return false, fmt.Errorf("sid=%d: %w", plan.Rule.Sid, err)
		}
		if currentMatched {
			matched = true
		}
	}
	return matched, nil
}

// MatchHTTPFlow evaluates stateless HTTP rules against a request/response flow.
// It mirrors the legacy matcher's MatchHTTPFlow behavior while sharing the
// engine's compiled rules.
func (e *StatefulEngine) MatchHTTPFlow(flow *match.HttpFlow) (bool, error) {
	if e == nil || e.closed.Load() || flow == nil {
		return false, nil
	}
	matched := false
	for _, packet := range flow.ToRequestPacket() {
		currentMatched, err := e.MatchPacket(packet)
		if err != nil {
			return false, err
		}
		if currentMatched {
			matched = true
		}
	}
	return matched, nil
}
