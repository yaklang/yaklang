package pcaputil

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RTSP/UDP associations are created only from a matched successful SETUP
// response. The RTSP control tuple identifies the default media addresses;
// explicit source/destination parameters in the selected Transport override
// those defaults after strict IP validation.
type rtspUDPTransport struct {
	profile                                      string
	clientRTP, clientRTCP, serverRTP, serverRTCP uint16
	clientAddress, serverAddress                 string
}

type rtspUDPTransportOffer struct {
	valid                 bool
	profile               string
	clientRTP, clientRTCP uint16
}

type rtspTransportFields struct {
	profile string
	params  map[string]string
	flags   map[string]bool
}

// splitRTSPTransportParts splits comma/semicolon-delimited Transport syntax
// without treating delimiters inside a quoted parameter as separators.
func splitRTSPTransportParts(value string, separator byte) ([]string, error) {
	parts := make([]string, 0, 4)
	start := 0
	quoted, escaped := false, false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escaped {
			escaped = false
			continue
		}
		if quoted && c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if c == separator && !quoted {
			parts = append(parts, strings.TrimSpace(value[start:i]))
			start = i + 1
		}
	}
	if quoted || escaped {
		return nil, fmt.Errorf("rtsp: unterminated quoted Transport parameter")
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts, nil
}

func parseRTSPTransportFields(values []string) (rtspTransportFields, error) {
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return rtspTransportFields{}, fmt.Errorf("rtsp: missing or ambiguous Transport header")
	}
	alternatives, err := splitRTSPTransportParts(values[0], ',')
	if err != nil || len(alternatives) != 1 || alternatives[0] == "" {
		return rtspTransportFields{}, fmt.Errorf("rtsp: ambiguous Transport alternatives")
	}
	parts, err := splitRTSPTransportParts(alternatives[0], ';')
	if err != nil || len(parts) == 0 || parts[0] == "" {
		return rtspTransportFields{}, fmt.Errorf("rtsp: invalid Transport syntax")
	}
	out := rtspTransportFields{profile: strings.ToUpper(strings.TrimSpace(parts[0])), params: make(map[string]string), flags: make(map[string]bool)}
	for _, part := range parts[1:] {
		if part == "" {
			return rtspTransportFields{}, fmt.Errorf("rtsp: empty Transport parameter")
		}
		kv := strings.SplitN(part, "=", 2)
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		if key == "" || out.flags[key] {
			return rtspTransportFields{}, fmt.Errorf("rtsp: duplicate or empty Transport parameter")
		}
		if _, exists := out.params[key]; exists {
			return rtspTransportFields{}, fmt.Errorf("rtsp: duplicate Transport parameter %q", key)
		}
		if len(kv) == 1 {
			out.flags[key] = true
			continue
		}
		value := strings.TrimSpace(kv[1])
		if value == "" {
			return rtspTransportFields{}, fmt.Errorf("rtsp: empty Transport parameter value")
		}
		if value[0] == '"' {
			if len(value) < 2 || value[len(value)-1] != '"' {
				return rtspTransportFields{}, fmt.Errorf("rtsp: invalid quoted Transport parameter")
			}
			value = value[1 : len(value)-1]
			if strings.ContainsAny(value, "\"\\") {
				return rtspTransportFields{}, fmt.Errorf("rtsp: escaped Transport parameters are unsupported")
			}
		}
		out.params[key] = value
	}
	return out, nil
}

func rtspUDPProfile(profile string) bool {
	return profile == "RTP/AVP" || profile == "RTP/AVP/UDP"
}

func parseRTSPPortPair(value string) (uint16, uint16, bool) {
	pair := strings.Split(value, "-")
	if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		return 0, 0, false
	}
	a, errA := strconv.ParseUint(pair[0], 10, 16)
	b, errB := strconv.ParseUint(pair[1], 10, 16)
	if errA != nil || errB != nil || a == 0 || b == 0 || a == b {
		return 0, 0, false
	}
	return uint16(a), uint16(b), true
}

func parseRTSPClientTransportOffer(values []string) rtspUDPTransportOffer {
	fields, err := parseRTSPTransportFields(values)
	if err != nil || !rtspUDPProfile(fields.profile) || !fields.flags["unicast"] || fields.flags["multicast"] || fields.flags["rtcp-mux"] {
		return rtspUDPTransportOffer{}
	}
	ports, ok := fields.params["client_port"]
	if !ok {
		return rtspUDPTransportOffer{}
	}
	rtp, rtcp, ok := parseRTSPPortPair(ports)
	if !ok {
		return rtspUDPTransportOffer{}
	}
	return rtspUDPTransportOffer{valid: true, profile: fields.profile, clientRTP: rtp, clientRTCP: rtcp}
}

// resolveRTSPUDPTransport implements only the captured unicast RTP/AVP UDP
// pair. Every other, duplicate, or incomplete negotiation remains unmapped.
func resolveRTSPUDPTransport(offer rtspUDPTransportOffer, values []string) (string, *rtspUDPTransport) {
	if len(values) == 0 {
		return "transport-not-observed", nil
	}
	fields, err := parseRTSPTransportFields(values)
	if err != nil {
		return "ambiguous-or-invalid-transport", nil
	}
	if !rtspUDPProfile(fields.profile) {
		return "non-udp-transport", nil
	}
	if !offer.valid {
		return "request-udp-offer-not-observed", nil
	}
	if fields.profile != offer.profile {
		return "request-profile-mismatch", nil
	}
	if !fields.flags["unicast"] || fields.flags["multicast"] || fields.flags["rtcp-mux"] {
		return "unsupported-transport-mode", nil
	}
	clientPair, clientOK := fields.params["client_port"]
	serverPair, serverOK := fields.params["server_port"]
	if !clientOK || !serverOK {
		return "incomplete-port-pair", nil
	}
	clientRTP, clientRTCP, ok := parseRTSPPortPair(clientPair)
	if !ok {
		return "invalid-client-port-pair", nil
	}
	serverRTP, serverRTCP, ok := parseRTSPPortPair(serverPair)
	if !ok {
		return "invalid-server-port-pair", nil
	}
	if clientRTP != offer.clientRTP || clientRTCP != offer.clientRTCP {
		return "client-port-offer-mismatch", nil
	}
	transport := &rtspUDPTransport{profile: fields.profile, clientRTP: clientRTP, clientRTCP: clientRTCP, serverRTP: serverRTP, serverRTCP: serverRTCP}
	if value := fields.params["destination"]; value != "" {
		ip := net.ParseIP(strings.Trim(value, "[]"))
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return "invalid-destination-address", nil
		}
		transport.clientAddress = ip.String()
	}
	if value := fields.params["source"]; value != "" {
		ip := net.ParseIP(strings.Trim(value, "[]"))
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return "invalid-source-address", nil
		}
		transport.serverAddress = ip.String()
	}
	return "mapped", transport
}

func (t rtspUDPTransport) sessionValue() map[string]any {
	return map[string]any{
		"Profile": t.profile, "Client RTP Port": int(t.clientRTP), "Client RTCP Port": int(t.clientRTCP),
		"Server RTP Port": int(t.serverRTP), "Server RTCP Port": int(t.serverRTCP),
		"Client Media Address": t.clientAddress, "Server Media Address": t.serverAddress,
	}
}

type rtspMediaTuple struct {
	domain   CaptureDomain
	src, dst string
}

type rtspMediaAssociation struct {
	domain                                       CaptureDomain
	session                                      string
	profile                                      string
	clientRTP, clientRTCP, serverRTP, serverRTCP string
	eventIDs                                     []uint64
	packetRefs                                   []PacketReference
	expires                                      time.Time
	cost                                         int64
}

type rtspMediaBinding struct {
	association *rtspMediaAssociation
	rtcp        bool
}

type rtspMediaMatch struct {
	association *rtspMediaAssociation
	rtcp        bool
	ambiguous   bool
}

type rtspMediaRegistry struct {
	mu      sync.Mutex
	entries []*rtspMediaAssociation
	index   map[rtspMediaTuple][]rtspMediaBinding
}

func normalizedRTSPHostPort(address string) (string, bool) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", false
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", false
	}
	ip := net.ParseIP(strings.SplitN(host, "%", 2)[0])
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return "", false
	}
	return net.JoinHostPort(ip.String(), port), true
}

func (a *binParser) observeRTSPMedia(e *ProtocolEvent) {
	if e == nil || e.Protocol != "rtsp" || e.Session == nil {
		return
	}
	if request, ok := e.Session[rtspInternalRequestEvidenceKey].(*rtspRequest); ok {
		delete(e.Session, rtspInternalRequestEvidenceKey)
		if request.owner != nil {
			request.owner.recordRequestEvidence(request, e, sessionCollectionLimit(a.budget.MaxCollectionElements))
		}
	}
	sid, _ := e.Session["Session ID"].(string)
	if e.Session["RTSP Session Teardown"] == true && sid != "" {
		a.removeRTSPMediaSession(e.Domain, sid)
		return
	}
	transport, ok := e.Session["UDP Media Transport"].(map[string]any)
	if !ok || sid == "" || e.Session["Packet Name"] != "Response" || e.Session["In Reply To"] != "SETUP" {
		return
	}
	serverEndpoint, serverOK := normalizedRTSPHostPort(e.Source)
	clientEndpoint, clientOK := normalizedRTSPHostPort(e.Destination)
	if !serverOK || !clientOK {
		e.Session["UDP Media Association Status"] = "control-address-unusable"
		return
	}
	clientHost, _, _ := net.SplitHostPort(clientEndpoint)
	serverHost, _, _ := net.SplitHostPort(serverEndpoint)
	if value, _ := transport["Client Media Address"].(string); value != "" {
		clientHost = value
	}
	if value, _ := transport["Server Media Address"].(string); value != "" {
		serverHost = value
	}
	clientRTP, okCRTP := transport["Client RTP Port"].(int)
	clientRTCP, okCRTCP := transport["Client RTCP Port"].(int)
	serverRTP, okSRTP := transport["Server RTP Port"].(int)
	serverRTCP, okSRTCP := transport["Server RTCP Port"].(int)
	if !okCRTP || !okCRTCP || !okSRTP || !okSRTCP {
		e.Session["UDP Media Association Status"] = "incomplete-transport"
		return
	}
	maxEvidence := sessionCollectionLimit(a.budget.MaxCollectionElements)
	requestEventIDs, _ := e.Session["RTSP SETUP Request Event IDs"].([]uint64)
	requestPacketRefs, _ := e.Session["RTSP SETUP Request Packet References"].([]PacketReference)
	eventIDs := make([]uint64, 0, len(requestEventIDs)+1)
	for _, id := range requestEventIDs {
		if id == 0 || containsUint64(eventIDs, id) {
			continue
		}
		eventIDs = append(eventIDs, id)
	}
	if e.ID != 0 && !containsUint64(eventIDs, e.ID) {
		eventIDs = append(eventIDs, e.ID)
	}
	packetRefs := make([]PacketReference, 0, len(requestPacketRefs)+len(e.SourceBytes.PacketRefs))
	for _, ref := range append(append([]PacketReference(nil), requestPacketRefs...), e.SourceBytes.PacketRefs...) {
		found := false
		for _, existing := range packetRefs {
			if existing == ref {
				found = true
				break
			}
		}
		if !found {
			packetRefs = append(packetRefs, ref)
		}
	}
	if len(eventIDs) > maxEvidence || len(packetRefs) > maxEvidence {
		e.Session["UDP Media Association Status"] = "resource-limit"
		return
	}
	assoc := &rtspMediaAssociation{
		domain: e.Domain, session: sid,
		profile:   fmt.Sprint(transport["Profile"]),
		clientRTP: net.JoinHostPort(clientHost, strconv.Itoa(clientRTP)), clientRTCP: net.JoinHostPort(clientHost, strconv.Itoa(clientRTCP)),
		serverRTP: net.JoinHostPort(serverHost, strconv.Itoa(serverRTP)), serverRTCP: net.JoinHostPort(serverHost, strconv.Itoa(serverRTCP)),
		eventIDs: eventIDs, packetRefs: packetRefs,
	}
	now := e.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	assoc.expires = now.Add(sipMediaAssociationTTL)
	assoc.cost = int64(256 + len(sid) + len(assoc.clientRTP) + len(assoc.clientRTCP) + len(assoc.serverRTP) + len(assoc.serverRTCP) + len(assoc.packetRefs)*24 + len(assoc.eventIDs)*8)
	if !a.addRTSPMediaAssociation(assoc, now) {
		e.Session["UDP Media Association Status"] = "resource-limit"
		return
	}
	e.Session["UDP Media Association Status"] = "mapped"
}

func rtspMediaKeys(a *rtspMediaAssociation) []rtspMediaTuple {
	return []rtspMediaTuple{
		{a.domain, a.clientRTP, a.serverRTP}, {a.domain, a.serverRTP, a.clientRTP},
		{a.domain, a.clientRTCP, a.serverRTCP}, {a.domain, a.serverRTCP, a.clientRTCP},
	}
}

func sameRTSPMediaTuple(a, b *rtspMediaAssociation) bool {
	return a.domain == b.domain && a.session == b.session && a.profile == b.profile && a.clientRTP == b.clientRTP && a.clientRTCP == b.clientRTCP && a.serverRTP == b.serverRTP && a.serverRTCP == b.serverRTCP
}

func (a *binParser) addRTSPMediaAssociation(next *rtspMediaAssociation, now time.Time) bool {
	if a.rtspMedia == nil {
		a.rtspMedia = &rtspMediaRegistry{index: make(map[rtspMediaTuple][]rtspMediaBinding)}
	}
	r := a.rtspMedia
	r.mu.Lock()
	defer r.mu.Unlock()
	a.pruneRTSPMediaLocked(r, now)
	for _, current := range r.entries {
		if !sameRTSPMediaTuple(current, next) {
			continue
		}
		if next.expires.After(current.expires) {
			current.expires = next.expires
		}
		for _, id := range next.eventIDs {
			found := false
			for _, existing := range current.eventIDs {
				found = found || existing == id
			}
			if !found && a.reserveMediaBytesLocked(8) {
				current.cost += 8
				current.eventIDs = append(current.eventIDs, id)
			}
		}
		for _, ref := range next.packetRefs {
			found := false
			for _, existing := range current.packetRefs {
				found = found || existing == ref
			}
			if !found && a.reserveMediaBytesLocked(24) {
				current.cost += 24
				current.packetRefs = append(current.packetRefs, ref)
			}
		}
		return true
	}
	if len(r.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveMediaBytesLocked(next.cost) {
		return false
	}
	next.eventIDs = append([]uint64(nil), next.eventIDs...)
	next.packetRefs = append([]PacketReference(nil), next.packetRefs...)
	r.entries = append(r.entries, next)
	for _, key := range rtspMediaKeys(next) {
		r.index[key] = append(r.index[key], rtspMediaBinding{association: next, rtcp: key.src == next.clientRTCP || key.src == next.serverRTCP})
	}
	return true
}

func (a *binParser) pruneRTSPMediaLocked(r *rtspMediaRegistry, now time.Time) {
	if now.IsZero() {
		return
	}
	kept := r.entries[:0]
	for _, association := range r.entries {
		if association.expires.IsZero() || now.Before(association.expires) {
			kept = append(kept, association)
			continue
		}
		for _, key := range rtspMediaKeys(association) {
			bindings := r.index[key]
			for i := 0; i < len(bindings); i++ {
				if bindings[i].association == association {
					bindings = append(bindings[:i], bindings[i+1:]...)
					break
				}
			}
			if len(bindings) == 0 {
				delete(r.index, key)
			} else {
				r.index[key] = bindings
			}
		}
		a.buffered.Add(-association.cost)
	}
	r.entries = kept
}

func (a *binParser) matchRTSPMedia(domain CaptureDomain, source, destination string, now time.Time) rtspMediaMatch {
	if a.rtspMedia == nil {
		return rtspMediaMatch{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	src, okSrc := normalizedRTSPHostPort(source)
	dst, okDst := normalizedRTSPHostPort(destination)
	if !okSrc || !okDst {
		return rtspMediaMatch{}
	}
	r := a.rtspMedia
	r.mu.Lock()
	defer r.mu.Unlock()
	a.pruneRTSPMediaLocked(r, now)
	bindings := r.index[rtspMediaTuple{domain: domain, src: src, dst: dst}]
	if len(bindings) == 0 {
		return rtspMediaMatch{}
	}
	var match rtspMediaMatch
	for _, binding := range bindings {
		if match.association == nil {
			match = rtspMediaMatch{association: binding.association, rtcp: binding.rtcp}
			continue
		}
		if match.association != binding.association || match.rtcp != binding.rtcp {
			match.ambiguous = true
		}
	}
	if match.association != nil && !match.ambiguous {
		// Duplicate SETUP observations may append evidence while another UDP
		// worker is decoding. Return an immutable snapshot after releasing the
		// registry lock.
		copyOfAssociation := *match.association
		copyOfAssociation.eventIDs = append([]uint64(nil), match.association.eventIDs...)
		copyOfAssociation.packetRefs = append([]PacketReference(nil), match.association.packetRefs...)
		match.association = &copyOfAssociation
	}
	return match
}

func (a *binParser) decodeRTSPMediaDatagram(e *ProtocolEvent, wire []byte, match rtspMediaMatch) bool {
	if match.ambiguous {
		e.Protocol, e.Profile, e.Admission = "rtp", "rtp-udp", "ambiguous-captured-rtsp-transport"
		e.Completeness, e.Rule, e.Entry = "context-required", "application-layer.rtp", "RTP/RTCP"
		err := protocolError(ErrContextRequired, "ambiguous RTSP SETUP media port mapping")
		a.finishProtocolDatagram(e, wire, nil, err)
		e.Session = map[string]any{"Association Status": "ambiguous-rtsp-transport", "Media Data Status": "not-decoded"}
		e.semanticFields = nil
		return true
	}
	if match.association == nil {
		return false
	}
	if !a.decodeRTPDatagram(e, wire, true, match.rtcp, sipMediaMatch{status: "rtsp-transport-matched"}) {
		return false
	}
	association := match.association
	e.Admission = "captured-rtsp-setup-port-mapping"
	if e.Session == nil {
		e.Session = map[string]any{}
	}
	e.Session["Association Status"] = "matched-rtsp-transport"
	e.Session["Association Confidence"] = "captured RTSP SETUP client_port/server_port mapping; RTP encoding and clock rate not inferred"
	e.Session["Associated RTSP Session ID"] = association.session
	e.Session["RTSP Transport Profile"] = association.profile
	e.Session["RTSP SETUP Event IDs"] = append([]uint64(nil), association.eventIDs...)
	e.Session["RTSP SETUP Packet References"] = append([]PacketReference(nil), association.packetRefs...)
	if e.Source == association.clientRTP || e.Source == association.clientRTCP {
		e.Session["RTSP Media Direction"] = "client-to-server"
	} else {
		e.Session["RTSP Media Direction"] = "server-to-client"
	}
	e.semanticFields = cloneSession(e.Session)
	return true
}

func (a *binParser) removeRTSPMediaSession(domain CaptureDomain, session string) {
	if a.rtspMedia == nil || session == "" {
		return
	}
	r := a.rtspMedia
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.entries[:0]
	for _, association := range r.entries {
		if association.domain != domain || association.session != session {
			kept = append(kept, association)
			continue
		}
		for _, key := range rtspMediaKeys(association) {
			bindings := r.index[key]
			for i := 0; i < len(bindings); i++ {
				if bindings[i].association == association {
					bindings = append(bindings[:i], bindings[i+1:]...)
					break
				}
			}
			if len(bindings) == 0 {
				delete(r.index, key)
			} else {
				r.index[key] = bindings
			}
		}
		a.buffered.Add(-association.cost)
	}
	r.entries = kept
}

func (a *binParser) closeRTSPMedia() {
	if a.rtspMedia == nil {
		return
	}
	r := a.rtspMedia
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, association := range r.entries {
		a.buffered.Add(-association.cost)
	}
	r.entries = nil
	r.index = make(map[rtspMediaTuple][]rtspMediaBinding)
}
