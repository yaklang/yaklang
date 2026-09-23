package pcaputil

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sipMediaAssociationTTL = 10 * time.Minute

type mediaEndpointKey struct {
	domain   CaptureDomain
	endpoint string
}

type sipMediaAssociation struct {
	domain                   CaptureDomain
	endpoint, rtcpEndpoint   string
	callID, fromTag, toTag   string
	mediaType, proto, direct string
	payload                  uint8
	encoding                 string
	clockRate                int
	secure                   bool
	negotiated               bool
	packetRefs               []PacketReference
	eventIDs                 []uint64
	expires                  time.Time
	cost                     int64
}

type sipMediaMatch struct {
	status     string
	callID     string
	mediaType  string
	proto      string
	direction  string
	payload    uint8
	encoding   string
	clockRate  int
	secure     bool
	eventIDs   []uint64
	packetRefs []PacketReference
	endpoints  []string
}

func (a *binParser) observeSIPMedia(e *ProtocolEvent) {
	if e == nil || e.Protocol != "sip" || e.Session == nil {
		return
	}
	sdp, ok := e.Session["SDP"].(map[string]any)
	if !ok {
		return
	}
	media, _ := sdp["Media"].([]map[string]any)
	if len(media) == 0 {
		return
	}
	callID, _ := e.Session["Call-ID"].(string)
	if callID == "" {
		return
	}
	packetName, _ := e.Session["Packet Name"].(string)
	method, _ := e.Session["Method"].(string)
	if packetName == "Response" {
		if e.Session["Association Status"] != "matched" || e.Session["Matched Request"] != "INVITE" {
			return
		}
	} else {
		if method != "INVITE" && !(method == "ACK" && e.Session["Association Status"] == "invite-2xx-ack") {
			return
		}
	}
	if e.Session["Retransmission"] == true {
		return
	}
	fromTag, _ := e.Session["From Tag"].(string)
	toTag, _ := e.Session["To Tag"].(string)
	refs := append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
	var additions []*sipMediaAssociation
	for _, section := range media {
		mediaType, _ := section["Type"].(string)
		proto, _ := section["Proto"].(string)
		port, _ := section["Port Number"].(int)
		connection, _ := section["Connection"].(map[string]any)
		address, _ := connection["Address"].(string)
		if port <= 0 || port > 65535 || !strings.HasPrefix(strings.ToUpper(proto), "RTP/") || address == "" {
			continue
		}
		ip := net.ParseIP(address)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		direction, _ := section["Direction"].(string)
		if direction == "inactive" {
			continue
		}
		formats, _ := section["Payload Types"].([]int)
		maps, _ := section["RTP Maps"].([]map[string]any)
		mapByPayload := make(map[int]map[string]any, len(maps))
		for _, mapping := range maps {
			pt, ok := mapping["Payload Type"].(int)
			if ok {
				mapByPayload[pt] = mapping
			}
		}
		secure := strings.Contains(strings.ToUpper(proto), "/SAVP")
		rtpEndpoint := net.JoinHostPort(ip.String(), strconv.Itoa(port))
		rtcpEndpoint := ""
		if mux, _ := section["RTCP Mux"].(bool); mux {
			rtcpEndpoint = rtpEndpoint
		} else if rtcpPort, ok := section["RTCP Port Number"].(int); ok && rtcpPort > 0 && rtcpPort <= 65535 {
			rtcpConnection, _ := section["RTCP Connection"].(map[string]any)
			rtcpAddress, _ := rtcpConnection["Address"].(string)
			if rtcpAddress == "" {
				rtcpAddress = ip.String()
			}
			if rtcpIP := net.ParseIP(rtcpAddress); rtcpIP != nil && !rtcpIP.IsUnspecified() && !rtcpIP.IsMulticast() {
				rtcpEndpoint = net.JoinHostPort(rtcpIP.String(), strconv.Itoa(rtcpPort))
			}
		} else if port%2 == 0 && port < 65535 {
			// RFC 8866's default RTP/RTCP pairing uses the next port when no
			// explicit a=rtcp or rtcp-mux attribute is present.
			rtcpEndpoint = net.JoinHostPort(ip.String(), strconv.Itoa(port+1))
		}
		for _, format := range formats {
			if format < 0 || format > 127 {
				continue
			}
			pt := uint8(format)
			encoding, clock := "", 0
			if mapping := mapByPayload[format]; mapping != nil {
				encoding, _ = mapping["Encoding"].(string)
				clock, _ = mapping["Clock Rate"].(int)
			} else if format < 96 {
				encoding = rtpPayloadName(pt, nil)
				clock = rtpClock(pt)
			}
			cost := int64(288 + len(callID) + len(fromTag) + len(toTag) + len(rtpEndpoint) + len(rtcpEndpoint) + len(encoding) + len(refs)*24 + 8)
			additions = append(additions, &sipMediaAssociation{
				domain: e.Domain, endpoint: rtpEndpoint, rtcpEndpoint: rtcpEndpoint,
				callID: callID, fromTag: fromTag, toTag: toTag,
				mediaType: mediaType, proto: proto, direct: direction,
				payload: pt, encoding: encoding, clockRate: clock, secure: secure,
				packetRefs: append([]PacketReference(nil), refs...), eventIDs: []uint64{e.ID},
				expires: e.Timestamp.Add(sipMediaAssociationTTL), cost: cost,
			})
		}
	}
	if len(additions) == 0 {
		return
	}
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	if a.mediaIndex == nil {
		a.mediaIndex = make(map[mediaEndpointKey][]*sipMediaAssociation)
	}
	a.pruneMediaAssociationsLocked(e.Timestamp)
	accepted := 0
	for _, association := range additions {
		switch {
		case packetName == "Response":
			if offer := a.findSIPMediaOfferLocked(association, "", false); offer != nil {
				association.negotiated = true
				a.associateSIPOfferLocked(association)
			}
		case method == "ACK":
			// An offerless INVITE places the SDP offer in the 2xx response;
			// only its matching ACK answer completes offer/answer. Until then
			// the response endpoint remains an observed offer, not a negotiated
			// media mapping.
			if offer := a.findSIPMediaOfferLocked(association, association.toTag, true); offer != nil {
				if a.addSIPMediaEvidenceLocked(offer, association.eventIDs, association.packetRefs) {
					offer.negotiated = true
					association.negotiated = true
					appendSIPMediaEvidence(association, offer)
				}
			}
		}
		if a.mergeSIPMediaAssociationLocked(association) {
			accepted++
			continue
		}
		if len(a.mediaEntries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveMediaBytesLocked(association.cost) {
			continue
		}
		a.mediaEntries = append(a.mediaEntries, association)
		key := mediaEndpointKey{domain: association.domain, endpoint: association.endpoint}
		a.mediaIndex[key] = append(a.mediaIndex[key], association)
		if association.rtcpEndpoint != "" && association.rtcpEndpoint != association.endpoint {
			rtcpKey := mediaEndpointKey{domain: association.domain, endpoint: association.rtcpEndpoint}
			a.mediaIndex[rtcpKey] = append(a.mediaIndex[rtcpKey], association)
		}
		accepted++
	}
	if accepted > 0 {
		e.Session["SDP Media Candidates"] = accepted
		e.Session["SDP Media Evidence"] = "captured SIP/SDP c=, m=, and payload-type offer"
	}
}

func sameSIPMediaFormat(a, b *sipMediaAssociation) bool {
	return a.domain == b.domain && a.callID == b.callID && a.fromTag == b.fromTag &&
		a.mediaType == b.mediaType && strings.EqualFold(a.proto, b.proto) &&
		a.payload == b.payload && strings.EqualFold(a.encoding, b.encoding) &&
		a.clockRate == b.clockRate && a.secure == b.secure
}

// findSIPMediaOfferLocked returns an offer for this dialog and media format.
// Empty toTag identifies an offer in the INVITE; a tagged, unnegotiated entry
// identifies an offer sent in a 2xx response awaiting an ACK answer.
func (a *binParser) findSIPMediaOfferLocked(answer *sipMediaAssociation, toTag string, requireUnnegotiated bool) *sipMediaAssociation {
	for _, candidate := range a.mediaEntries {
		if candidate.toTag != toTag || !sameSIPMediaFormat(candidate, answer) || (requireUnnegotiated && candidate.negotiated) {
			continue
		}
		return candidate
	}
	return nil
}

func (a *binParser) addSIPMediaEvidenceLocked(target *sipMediaAssociation, eventIDs []uint64, refs []PacketReference) bool {
	for _, id := range eventIDs {
		if containsUint64(target.eventIDs, id) {
			continue
		}
		if !a.reserveMediaBytesLocked(8) {
			return false
		}
		target.cost += 8
		target.eventIDs = append(target.eventIDs, id)
	}
	for _, ref := range refs {
		if packetReferenceContains(target.packetRefs, ref) {
			continue
		}
		if !a.reserveMediaBytesLocked(24) {
			return false
		}
		target.cost += 24
		target.packetRefs = append(target.packetRefs, ref)
	}
	return true
}

func appendSIPMediaEvidence(target, source *sipMediaAssociation) {
	for _, id := range source.eventIDs {
		if containsUint64(target.eventIDs, id) {
			continue
		}
		target.cost += 8
		target.eventIDs = append(target.eventIDs, id)
	}
	for _, ref := range source.packetRefs {
		if packetReferenceContains(target.packetRefs, ref) {
			continue
		}
		target.cost += 24
		target.packetRefs = append(target.packetRefs, ref)
	}
}

// Once a tagged SDP answer is observed for an INVITE, attach the answer
// evidence and dialog tag to the matching untagged offer. The endpoints may
// differ: each peer advertises its own receive tuple. Both endpoint tuples
// therefore remain separately indexed but become negotiated together.
func (a *binParser) associateSIPOfferLocked(answer *sipMediaAssociation) {
	for _, candidate := range a.mediaEntries {
		if candidate.toTag == "" && sameSIPMediaFormat(candidate, answer) {
			if !a.reserveMediaBytesLocked(int64(len(answer.toTag))) {
				continue
			}
			candidate.toTag = answer.toTag
			candidate.cost += int64(len(answer.toTag))
			if a.addSIPMediaEvidenceLocked(candidate, answer.eventIDs, answer.packetRefs) {
				candidate.negotiated = true
			}
		}
	}
}

// A normal offer has no To-tag, while its answer adds one. Upgrade only an
// exact endpoint/format candidate. A later fork answer keeps a distinct tag
// pair and will therefore remain ambiguous when both branches reuse that media
// tuple.
func (a *binParser) mergeSIPMediaAssociationLocked(next *sipMediaAssociation) bool {
	for _, current := range a.mediaEntries {
		if current.domain != next.domain || current.callID != next.callID || current.fromTag != next.fromTag || current.endpoint != next.endpoint || current.rtcpEndpoint != next.rtcpEndpoint || current.mediaType != next.mediaType || current.proto != next.proto || current.payload != next.payload || current.encoding != next.encoding || current.clockRate != next.clockRate || current.secure != next.secure {
			continue
		}
		if current.toTag != "" && current.toTag != next.toTag {
			continue
		}
		if current.toTag == "" && next.toTag != "" {
			if !a.reserveMediaBytesLocked(int64(len(next.toTag))) {
				continue
			}
			current.toTag = next.toTag
			current.cost += int64(len(next.toTag))
		}
		current.negotiated = current.negotiated || next.negotiated
		if current.direct != next.direct && next.direct != "" {
			updatedDirect := current.direct
			if updatedDirect == "" || updatedDirect == "sendrecv" {
				updatedDirect = next.direct
			} else if next.direct != "sendrecv" && !mediaDirectionContains(updatedDirect, next.direct) {
				updatedDirect += "," + next.direct
			}
			if delta := int64(len(updatedDirect) - len(current.direct)); delta > 0 {
				if a.reserveMediaBytesLocked(delta) {
					current.cost += delta
				} else {
					updatedDirect = current.direct
				}
			}
			current.direct = updatedDirect
		}
		for _, eventID := range next.eventIDs {
			if containsUint64(current.eventIDs, eventID) {
				continue
			}
			if !a.reserveMediaBytesLocked(8) {
				return true
			}
			current.cost += 8
			current.eventIDs = append(current.eventIDs, eventID)
		}
		for _, ref := range next.packetRefs {
			if !packetReferenceContains(current.packetRefs, ref) {
				if !a.reserveMediaBytesLocked(24) {
					continue
				}
				current.cost += 24
				current.packetRefs = append(current.packetRefs, ref)
			}
		}
		if expiry := next.expires; expiry.After(current.expires) {
			current.expires = expiry
		}
		return true
	}
	return false
}

func packetReferenceContains(refs []PacketReference, candidate PacketReference) bool {
	for _, ref := range refs {
		if ref == candidate {
			return true
		}
	}
	return false
}

func containsUint64(values []uint64, candidate uint64) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (a *binParser) reserveMediaBytesLocked(cost int64) bool {
	if cost <= 0 {
		return true
	}
	limit := int64(a.config.MaxBufferedBytes)
	for {
		current := a.buffered.Load()
		if current+cost > limit {
			return false
		}
		if a.buffered.CompareAndSwap(current, current+cost) {
			for peak := a.peak.Load(); current+cost > peak; peak = a.peak.Load() {
				if a.peak.CompareAndSwap(peak, current+cost) {
					break
				}
			}
			return true
		}
	}
}

func (a *binParser) pruneMediaAssociationsLocked(now time.Time) {
	if now.IsZero() || len(a.mediaEntries) == 0 {
		return
	}
	kept := a.mediaEntries[:0]
	for _, association := range a.mediaEntries {
		if !association.expires.IsZero() && !now.Before(association.expires) {
			a.removeMediaIndexLocked(association)
			a.buffered.Add(-association.cost)
			continue
		}
		kept = append(kept, association)
	}
	a.mediaEntries = kept
}

func (a *binParser) removeMediaIndexLocked(association *sipMediaAssociation) {
	for _, endpoint := range []string{association.endpoint, association.rtcpEndpoint} {
		if endpoint == "" {
			continue
		}
		key := mediaEndpointKey{domain: association.domain, endpoint: endpoint}
		entries := a.mediaIndex[key]
		for i, candidate := range entries {
			if candidate == association {
				entries = append(entries[:i], entries[i+1:]...)
				break
			}
		}
		if len(entries) == 0 {
			delete(a.mediaIndex, key)
		} else {
			a.mediaIndex[key] = entries
		}
	}
}

func (a *binParser) matchSDPMedia(domain CaptureDomain, source, destination string, pt uint8, rtcp bool, now time.Time) sipMediaMatch {
	if a == nil {
		return sipMediaMatch{}
	}
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	a.pruneMediaAssociationsLocked(now)
	candidates := map[*sipMediaAssociation]struct{}{}
	for _, endpoint := range []string{normalizeMediaEndpoint(source), normalizeMediaEndpoint(destination)} {
		if endpoint == "" {
			continue
		}
		for _, association := range a.mediaIndex[mediaEndpointKey{domain: domain, endpoint: endpoint}] {
			payloadMatch := rtcp || association.payload == pt
			endpointMatch := !rtcp && endpoint == association.endpoint || rtcp && (endpoint == association.rtcpEndpoint || association.rtcpEndpoint == association.endpoint)
			if payloadMatch && endpointMatch {
				candidates[association] = struct{}{}
			}
		}
	}
	if len(candidates) == 0 {
		return sipMediaMatch{}
	}
	type group struct {
		key        string
		values     []*sipMediaAssociation
		match      sipMediaMatch
		endpoints  map[string]struct{}
		negotiated bool
	}
	groups := map[string]*group{}
	for association := range candidates {
		payload, encoding, clock := association.payload, strings.ToLower(association.encoding), association.clockRate
		if rtcp {
			payload, encoding, clock = 0, "", 0
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%d\x00%t\x00%s", association.callID, association.fromTag, association.toTag, association.mediaType, payload, encoding, clock, association.secure, strings.ToUpper(association.proto))
		g := groups[key]
		if g == nil {
			g = &group{key: key, endpoints: map[string]struct{}{}, negotiated: association.negotiated, match: sipMediaMatch{status: "observed-offer", callID: association.callID, mediaType: association.mediaType, proto: association.proto, payload: payload, encoding: association.encoding, clockRate: association.clockRate, secure: association.secure}}
			groups[key] = g
		} else {
			g.negotiated = g.negotiated && association.negotiated
			if rtcp && (g.match.clockRate != association.clockRate || !strings.EqualFold(g.match.encoding, association.encoding)) {
				// RTCP carries no RTP payload type. If one negotiated media section
				// advertises several clocks, keep the association but withhold a
				// guessed jitter conversion rate.
				g.match.clockRate = 0
				g.match.encoding = ""
			}
		}
		g.values = append(g.values, association)
		g.endpoints[association.endpoint] = struct{}{}
		if association.rtcpEndpoint != "" {
			g.endpoints[association.rtcpEndpoint] = struct{}{}
		}
		if association.direct != "" && !mediaDirectionContains(g.match.direction, association.direct) {
			if g.match.direction == "" {
				g.match.direction = association.direct
			} else {
				g.match.direction += "," + association.direct
			}
		}
		for _, eventID := range association.eventIDs {
			g.match.eventIDs = appendUniqueUint64(g.match.eventIDs, eventID)
		}
		for _, ref := range association.packetRefs {
			g.match.packetRefs = appendUniquePacketReference(g.match.packetRefs, ref)
		}
	}
	if len(groups) != 1 {
		return sipMediaMatch{status: "ambiguous-sdp-match"}
	}
	for _, g := range groups {
		if g.negotiated {
			g.match.status = "matched"
		}
		for endpoint := range g.endpoints {
			g.match.endpoints = append(g.match.endpoints, endpoint)
		}
		return g.match
	}
	return sipMediaMatch{}
}

func (a *binParser) decodeRTPDatagram(e *ProtocolEvent, wire []byte, explicit, rtcp bool, match sipMediaMatch) bool {
	classifiedRTCP, pt, parseErr := inspectRTPDatagramAs(wire, a.config.MaxMessageBytes, a.budget.MaxCollectionElements, rtcp)
	if parseErr == nil && classifiedRTCP != rtcp {
		parseErr = protocolError(ErrMalformedMessage, "rtp/rtcp class mismatch")
	}
	if parseErr != nil && !explicit {
		return false
	}
	e.Protocol = "rtp"
	e.Profile = "rtp-udp"
	e.Admission = "captured-sdp-endpoint-and-payload"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	e.Completeness = "message"
	e.Rule = "application-layer.rtp"
	if rtcp {
		e.Entry = "RTCP"
	} else {
		e.Entry = "RTP"
	}
	e.Raw = append([]byte(nil), wire...)
	if parseErr == nil {
		key := binUDPKey{a: "rtp/" + e.Source, b: "rtp/" + e.Destination, domain: e.Domain}
		if key.a > key.b {
			key.a, key.b = key.b, key.a
		}
		a.udpMu.Lock()
		if a.udpSessions == nil {
			a.udpSessions = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		} else if e.Timestamp.After(a.udpSessions.clock) {
			a.udpSessions.clock = e.Timestamp
		}
		store := a.udpSessions
		a.pruneRTPFlowsLocked(store)
		el := store.entries[key]
		var flow *binFlow
		if el == nil {
			if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
				parseErr = protocolError(ErrResourceExceeded, "UDP media-flow budget")
			} else {
				endpoints := []string{e.Source, e.Destination}
				sort.Strings(endpoints)
				flow = &binFlow{a: a, id: a.flows.Add(1), protocol: "rtp", rtp: &binRTP{sources: map[uint32]*rtpSource{}}, endpoints: [2]string{endpoints[0], endpoints[1]}}
				flow.rtp.maxBufferedBytes = a.config.MaxBufferedBytes
				flow.rtp.reserveSessionMemory = flow.reserveSession
				if parseErr = flow.reserveSession(512); parseErr == nil {
					el = store.lru.PushBack(&binUDPEntry{key: key, flow: flow, touched: store.clock})
					store.entries[key] = el
				}
			}
		}
		if el != nil && parseErr == nil {
			entry := el.Value.(*binUDPEntry)
			entry.touched = store.clock
			store.lru.MoveToBack(el)
			flow = entry.flow
			e.FlowID = flow.id
			if e.Source != flow.endpoints[0] {
				e.Direction = 1
			}
			flow.rtp.maxBufferedBytes = a.config.MaxBufferedBytes
			flow.rtp.reserveSessionMemory = flow.reserveSession
			useSDPMap := (match.status == "matched" || match.status == "observed-offer") && !rtcp && (match.payload != 0 || pt == 0)
			if useSDPMap {
				// Preflight the retained map-state delta before allocating the maps.
				// This can grow after packets for the same SSRC have already created
				// source history, so source-only accounting is not sufficient here.
				target := int64(512) + flow.rtp.retainedSourceBytes + 64 + 32
				if match.clockRate > 0 {
					target += 64 + 32
				}
				if parseErr = flow.rtp.reserveMemory(target); parseErr == nil {
					flow.rtp.payload = map[uint8]string{match.payload: match.encoding}
					flow.rtp.clocks = nil
					if match.clockRate > 0 {
						flow.rtp.clocks = map[uint8]int{match.payload: match.clockRate}
					}
				}
			} else {
				flow.rtp.payload, flow.rtp.clocks = nil, nil
			}
			if parseErr == nil {
				if rtcp {
					e.Session, parseErr = flow.rtp.consumeRTCPCompound(wire, a.budget.MaxCollectionElements)
				} else {
					e.Session, parseErr = flow.rtp.consumeRTP(wire, e.Timestamp, a.budget.MaxCollectionElements)
				}
			}
		}
		a.udpMu.Unlock()
	}
	if e.Session == nil {
		e.Session = map[string]any{"Packet Name": "RTP", "Version": 2, "Observation Scope": "datagram"}
	}
	if parseErr == nil {
		e.Session["Observation Scope"] = "media-flow"
		if explicit && match.status != "matched" {
			e.Session["Association Status"] = "explicit-selection"
			e.Session["Association Confidence"] = "low; no captured SDP required"
		} else {
			e.Session["Association Status"] = match.status
			if rtcp {
				e.Session["Association Confidence"] = "captured SIP/SDP RTCP endpoint"
			} else {
				e.Session["Association Confidence"] = "captured SIP/SDP endpoint and payload type"
			}
		}
		if match.status == "matched" {
			e.Session["Associated Call-ID"] = match.callID
			e.Session["Associated Media Type"] = match.mediaType
			e.Session["Negotiated Media Protocol"] = match.proto
			e.Session["Negotiated Direction"] = match.direction
			e.Session["SDP Event IDs"] = append([]uint64(nil), match.eventIDs...)
			e.Session["SDP Packet References"] = append([]PacketReference(nil), match.packetRefs...)
			e.Session["SDP Media Endpoints"] = append([]string(nil), match.endpoints...)
			if rtcp && match.clockRate > 0 {
				e.Session["Clock Rate"] = match.clockRate
				e.Session["Clock Rate Source"] = "observed SDP a=rtpmap"
				e.Session["RTCP Jitter Unit"] = "RTP timestamp ticks; divide by Clock Rate for seconds"
			}
			if match.secure {
				e.Session["Media Security"] = "SRTP"
				e.Session["Content Visibility"] = "encrypted"
				e.Session["RTP Payload Visibility"] = "opaque-encrypted"
				e.Session["Media Data Status"] = "srtp-payload-not-decrypted"
			}
		} else if match.status == "ambiguous-sdp-match" {
			e.Session["Association Confidence"] = "ambiguous; Call-ID not attached"
		} else if match.status == "observed-offer" {
			e.Session["Association Confidence"] = "SDP offer observed; matched response not captured"
			e.Session["SDP Offer Event IDs"] = append([]uint64(nil), match.eventIDs...)
			e.Session["SDP Offer Packet References"] = append([]PacketReference(nil), match.packetRefs...)
		}
		if rtcp {
			e.Summary = "RTCP " + fmt.Sprint(e.Session["Packet Name"])
		} else {
			if !match.secure {
				if headerLen, headerErr := rtpHeaderSize(wire); headerErr == nil && headerLen <= len(wire) {
					payloadLen := len(wire) - headerLen
					if wire[0]&0x20 != 0 {
						payloadLen -= int(wire[len(wire)-1])
					}
					e.Session["RTP Payload Length"] = payloadLen
					e.Session["Payload Length Basis"] = "RTP header and padding excluded; encrypted profiles omitted"
				}
			}
			e.Summary = "RTP " + fmt.Sprint(e.Session["Payload Type Name"]) + " sequence " + fmt.Sprint(e.Session["Sequence"])
			if match.clockRate == 0 && pt >= 96 {
				e.Session["Timing Status"] = "clock-rate-unknown"
				delete(e.Session, "Jitter")
			}
		}
		e.semanticFields = cloneSession(e.Session)
	} else {
		e.Session = nil
	}
	a.finishProtocolDatagram(e, wire, nil, parseErr)
	return true
}

func (r *binRTP) consumeRTCPCompound(raw []byte, maxElements int) (map[string]any, error) {
	limit := sessionCollectionLimit(maxElements)
	packets := make([]map[string]any, 0, min(limit, 2))
	for offset := 0; offset < len(raw); {
		n := rtpRTCPSize(raw[offset:])
		if n < 8 || offset+n > len(raw) {
			return nil, protocolError(ErrMalformedMessage, "rtcp: invalid compound member length")
		}
		member := raw[offset : offset+n]
		if err := validateRTCPPacketInCompound(member, offset+n == len(raw)); err != nil {
			return nil, err
		}
		if len(packets) >= limit {
			return nil, protocolError(ErrResourceExceeded, "rtcp: compound packet budget exceeded")
		}
		packet, err := r.consumeRTCPBounded(member, maxElements)
		if err != nil {
			return nil, err
		}
		packets = append(packets, packet)
		offset += n
	}
	if len(packets) == 1 {
		return packets[0], nil
	}
	return map[string]any{"Packet Name": "Compound", "Version": 2, "Compound Packets": packets, "Packet Count": len(packets), "Context Level": "observed", "Network Loss": false, "Loss Kind": "none"}, nil
}

func (a *binParser) pruneRTPFlowsLocked(store *binUDPStore) {
	if store == nil || store.clock.IsZero() {
		return
	}
	for el := store.lru.Front(); el != nil; {
		next := el.Next()
		entry := el.Value.(*binUDPEntry)
		if strings.HasPrefix(entry.key.a, "rtp/") && store.clock.Sub(entry.touched) >= sipMediaAssociationTTL {
			entry.flow.closeSession()
			delete(store.entries, entry.key)
			store.lru.Remove(el)
		}
		el = next
	}
}

func inspectRTPDatagram(wire []byte, maxMessage, maxElements int) (rtcp bool, pt uint8, err error) {
	if len(wire) == 0 || len(wire) > maxMessage || wire[0]>>6 != 2 {
		return false, 0, protocolError(ErrMalformedMessage, "rtp: invalid datagram version or size")
	}
	if rtpIsRTCP(wire) {
		return inspectRTPDatagramAs(wire, maxMessage, maxElements, true)
	}
	return inspectRTPDatagramAs(wire, maxMessage, maxElements, false)
}

func inspectRTPDatagramAs(wire []byte, maxMessage, maxElements int, rtcp bool) (bool, uint8, error) {
	if len(wire) == 0 || len(wire) > maxMessage || wire[0]>>6 != 2 {
		return rtcp, 0, protocolError(ErrMalformedMessage, "rtp: invalid datagram version or size")
	}
	if rtcp {
		if !rtpIsRTCP(wire) {
			return true, 0, protocolError(ErrMalformedMessage, "rtcp: payload type is outside RTCP range")
		}
		if err := validateRTCPCompound(wire, maxElements); err != nil {
			return true, 0, err
		}
		return true, 0, nil
	}
	n, headerErr := rtpHeaderSize(wire)
	if headerErr != nil || n > len(wire) {
		return false, 0, protocolError(ErrMalformedMessage, "rtp: invalid header or extension")
	}
	if wire[0]&0x20 != 0 {
		padding := int(wire[len(wire)-1])
		if padding == 0 || padding > len(wire)-n {
			return false, 0, protocolError(ErrMalformedMessage, "rtp: invalid padding")
		}
	}
	return false, wire[1] & 0x7f, nil
}

func validateRTCPPacket(wire []byte) error {
	return validateRTCPPacketInCompound(wire, true)
}

func validateRTCPCompound(wire []byte, maxElements int) error {
	count := 0
	for off := 0; off < len(wire); {
		if len(wire)-off < 4 || wire[off]>>6 != 2 || !rtpIsRTCP(wire[off:]) {
			return protocolError(ErrMalformedMessage, "rtcp: invalid compound member")
		}
		n := rtpRTCPSize(wire[off:])
		if n < 8 || n > len(wire)-off {
			return protocolError(ErrMalformedMessage, "rtcp: invalid member length")
		}
		if err := validateRTCPPacketInCompound(wire[off:off+n], off+n == len(wire)); err != nil {
			return err
		}
		off += n
		count++
		if count > sessionCollectionLimit(maxElements) {
			return protocolError(ErrResourceExceeded, "rtcp: compound packet budget exceeded")
		}
	}
	return nil
}

func validateRTCPPacketInCompound(wire []byte, final bool) error {
	if len(wire) < 8 || wire[0]>>6 != 2 || !rtpIsRTCP(wire) {
		return protocolError(ErrMalformedMessage, "rtcp: invalid header")
	}
	n := rtpRTCPSize(wire)
	if n != len(wire) {
		return protocolError(ErrMalformedMessage, "rtcp: packet length mismatch")
	}
	if wire[0]&0x20 != 0 {
		if !final {
			return protocolError(ErrMalformedMessage, "rtcp: padding bit is only valid on the final compound member")
		}
		padding := int(wire[len(wire)-1])
		if padding == 0 || padding > len(wire)-4 {
			return protocolError(ErrMalformedMessage, "rtcp: invalid padding")
		}
	}
	pt, rc := wire[1], int(wire[0]&0x1f)
	dataEnd := len(wire)
	if wire[0]&0x20 != 0 {
		dataEnd -= int(wire[len(wire)-1])
	}
	switch pt {
	case 200: // Sender Report.
		if dataEnd != 28+24*rc {
			return protocolError(ErrMalformedMessage, "rtcp: sender report length does not match report count")
		}
	case 201: // Receiver Report.
		if dataEnd != 8+24*rc {
			return protocolError(ErrMalformedMessage, "rtcp: receiver report length does not match report count")
		}
	case 202: // Source Description: validate the advertised chunk headers and items.
		off := 4
		for chunk := 0; chunk < rc; chunk++ {
			if off+4 > dataEnd {
				return protocolError(ErrMalformedMessage, "rtcp: truncated sdes chunk")
			}
			off += 4 // SSRC/CSRC
			for {
				if off >= dataEnd {
					return protocolError(ErrMalformedMessage, "rtcp: unterminated sdes chunk")
				}
				item := wire[off]
				off++
				if item == 0 {
					for off%4 != 0 {
						if off >= dataEnd || wire[off] != 0 {
							return protocolError(ErrMalformedMessage, "rtcp: invalid sdes alignment")
						}
						off++
					}
					break
				}
				if off >= dataEnd || int(wire[off])+off+1 > dataEnd {
					return protocolError(ErrMalformedMessage, "rtcp: truncated sdes item")
				}
				off += 1 + int(wire[off])
			}
		}
		if off != dataEnd {
			return protocolError(ErrMalformedMessage, "rtcp: sdes length mismatch")
		}
	case 203: // BYE: SSRC list plus optional reason.
		if dataEnd < 4+4*rc {
			return protocolError(ErrMalformedMessage, "rtcp: truncated BYE sources")
		}
		if dataEnd > 4+4*rc {
			reasonLen := int(wire[4+4*rc])
			if 5+4*rc+reasonLen > dataEnd {
				return protocolError(ErrMalformedMessage, "rtcp: truncated BYE reason")
			}
			if remain := dataEnd - (5 + 4*rc + reasonLen); remain > 3 {
				return protocolError(ErrMalformedMessage, "rtcp: invalid BYE alignment length")
			} else {
				for i := dataEnd - remain; i < dataEnd; i++ {
					if wire[i] != 0 {
						return protocolError(ErrMalformedMessage, "rtcp: invalid BYE alignment")
					}
				}
			}
		}
	case 204: // APP requires the fixed SSRC and four-octet name.
		if dataEnd < 12 || (dataEnd-12)%4 != 0 {
			return protocolError(ErrMalformedMessage, "rtcp: truncated APP packet")
		}
	}
	return nil
}

func decodeRTCPSDES(wire []byte, dataEnd, maxElements int) ([]map[string]any, error) {
	limit := sessionCollectionLimit(maxElements)
	chunks := make([]map[string]any, 0, int(wire[0]&0x1f))
	items := 0
	off := 4
	for chunkIndex := 0; chunkIndex < int(wire[0]&0x1f); chunkIndex++ {
		if off+4 > dataEnd {
			return nil, protocolError(ErrMalformedMessage, "rtcp: truncated sdes chunk")
		}
		chunk := map[string]any{"SSRC": binary.BigEndian.Uint32(wire[off : off+4])}
		off += 4
		var chunkItems []map[string]any
		for {
			if off >= dataEnd {
				return nil, protocolError(ErrMalformedMessage, "rtcp: unterminated sdes chunk")
			}
			typ := wire[off]
			off++
			if typ == 0 {
				for off%4 != 0 {
					if off >= dataEnd || wire[off] != 0 {
						return nil, protocolError(ErrMalformedMessage, "rtcp: invalid sdes alignment")
					}
					off++
				}
				break
			}
			if off >= dataEnd {
				return nil, protocolError(ErrMalformedMessage, "rtcp: truncated sdes item")
			}
			n := int(wire[off])
			off++
			if n > dataEnd-off {
				return nil, protocolError(ErrMalformedMessage, "rtcp: truncated sdes item value")
			}
			items++
			if items > limit {
				return nil, protocolError(ErrResourceExceeded, "rtcp: SDES item budget exceeded")
			}
			chunkItems = append(chunkItems, map[string]any{"Type": typ, "Type Name": rtcpSDESItemName(typ), "Value": append([]byte(nil), wire[off:off+n]...)})
			off += n
		}
		chunks = append(chunks, map[string]any{"SSRC": chunk["SSRC"], "Items": chunkItems})
		if len(chunks) > limit {
			return nil, protocolError(ErrResourceExceeded, "rtcp: SDES chunk budget exceeded")
		}
	}
	if off != dataEnd {
		return nil, protocolError(ErrMalformedMessage, "rtcp: sdes trailing data")
	}
	return chunks, nil
}

func rtcpSDESItemName(typ byte) string {
	switch typ {
	case 1:
		return "CNAME"
	case 2:
		return "NAME"
	case 3:
		return "EMAIL"
	case 4:
		return "PHONE"
	case 5:
		return "LOC"
	case 6:
		return "TOOL"
	case 7:
		return "NOTE"
	case 8:
		return "PRIV"
	default:
		return "unknown"
	}
}

func normalizeMediaEndpoint(endpoint string) string {
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return ""
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return ""
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return ""
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(port))
}

func appendUniqueUint64(dst []uint64, value uint64) []uint64 {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func appendUniquePacketReference(dst []PacketReference, value PacketReference) []PacketReference {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func mediaDirectionContains(current, value string) bool {
	for _, direction := range strings.Split(current, ",") {
		if direction == value {
			return true
		}
	}
	return false
}

func (a *binParser) closeMediaAssociations() {
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	for _, association := range a.mediaEntries {
		a.buffered.Add(-association.cost)
	}
	a.mediaEntries = nil
	a.mediaIndex = nil
}
