package sharkcli

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type protocolConversationKey struct {
	domain    pcaputil.CaptureDomain
	id        uint64
	transport string
}
type protocolConversationIdentity struct {
	key     protocolConversationKey
	stack   []string
	version uint64
	touched time.Time
}

type associationReference struct {
	id   uint64
	hash string
}
type associationObservation struct {
	id         uint64
	name, hash string
	refs       []pcaputil.PacketReference
}

func conversationKey(raw *capturedPacket) protocolConversationKey {
	domain := raw.reference.Domain
	if raw.conversationDomainSet {
		domain = raw.conversationDomain
	}
	transport := raw.conversationTransport
	if transport == "" {
		transport = "udp"
	} // historical packet snapshots
	return protocolConversationKey{domain: domain, id: raw.conversationID, transport: transport}
}

func observedL2TPAssociation(e *pcaputil.ProtocolEvent) bool {
	return e != nil && e.FlowID != 0 && e.Protocol == "l2tp" &&
		e.Profile == "l2tp3-bounded-wire-envelope" && eventValues(e)["Envelope Verified"] == true
}

func recordProtocolConversation(raw *capturedPacket, e *pcaputil.ProtocolEvent) {
	if e.FlowID == 0 || e.Transport != "udp" && !(e.Transport == "network" && observedL2TPAssociation(e)) {
		return
	}
	raw.conversationID, raw.conversationTransport = e.FlowID, e.Transport
	raw.conversationDomain, raw.conversationDomainSet = e.Domain, true
}

func identityProjection(e *pcaputil.ProtocolEvent) packetPresentation {
	if !credibleTLSIdentity(e) {
		return packetPresentation{}
	}
	p := packetPresentation{protocol: eventProtocolName(e)}
	for _, name := range eventCarriers(e) {
		p.addLayer(name)
	}
	p.addLayer(p.protocol)
	return p
}

// A syntactically plausible ciphertext record is not a handshake. Midstream
// records keep their candidate fields, while only observed session context,
// complete handshake messages, authenticated records or a clear Alert may
// establish a TLS identity. Previously established connection metadata survives.
func credibleTLSIdentity(e *pcaputil.ProtocolEvent) bool {
	if e.Protocol != "tls" {
		return true
	}
	v := eventValues(e)
	if tlsVersion(v["Negotiated Version"]) != "TLS" || v["Authentication Verified"] == true || len(mapList(v["Handshake Messages"])) > 0 {
		return true
	}
	_, alert := v["Alert Description"]
	return alert && number(v["Record Type"]) == 21 && v["Content Visibility"] != "encrypted"
}

// Record framing can identify the wire carrier without a captured handshake.
// It never establishes the negotiated version, keys, or peer identity. Require
// a complete ordinary protected record, not a few matching header bytes.
func strongTLSRecord(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "tls" || e.Error != "" || e.Status != "decoded" && e.Status != "deferred" || len(e.Raw) < 21 || e.Raw[0] != 23 {
		return false
	}
	version := binary.BigEndian.Uint16(e.Raw[1:3])
	return version >= 0x301 && version <= 0x303 && int(binary.BigEndian.Uint16(e.Raw[3:5])) == len(e.Raw)-5
}

func (s *streamStore) observeDatagram(e *pcaputil.ProtocolEvent) {
	if e.FlowID == 0 || ((e.Status != "decoded" && e.Status != "deferred" || e.Error != "") && !protectedConversationEvent(e) && !observedL2TPAssociation(e)) {
		return
	}
	p := identityProjection(e)
	if len(p.stack) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.protocolConversations == nil {
		s.protocolConversations = map[protocolConversationKey]*list.Element{}
	}
	for el := s.protocolConversationLRU.Front(); el != nil; el = s.protocolConversationLRU.Front() {
		v := el.Value.(*protocolConversationIdentity)
		if e.Timestamp.Sub(v.touched) < 10*time.Minute {
			break
		}
		delete(s.protocolConversations, v.key)
		s.protocolConversationLRU.Remove(el)
	}
	key := protocolConversationKey{domain: e.Domain, id: e.FlowID, transport: e.Transport}
	el := s.protocolConversations[key]
	if el == nil {
		if s.protocolConversationLRU.Len() >= s.maxStreams {
			old := s.protocolConversationLRU.Front()
			delete(s.protocolConversations, old.Value.(*protocolConversationIdentity).key)
			s.protocolConversationLRU.Remove(old)
		}
		el = s.protocolConversationLRU.PushBack(&protocolConversationIdentity{key: key})
		s.protocolConversations[key] = el
	}
	v := el.Value.(*protocolConversationIdentity)
	layers := packetPresentation{stack: v.stack}
	for _, name := range p.stack {
		layers.addLayer(name)
	}
	v.stack, v.version = layers.stack, e.ID
	if e.Timestamp.After(v.touched) {
		v.touched = e.Timestamp
	}
	s.protocolConversationLRU.MoveToBack(el)
}

func (s *streamStore) projectDatagram(row packetSummary, raw *capturedPacket) packetSummary {
	if s == nil || raw.conversationID == 0 {
		return row
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if el := s.protocolConversations[conversationKey(raw)]; el != nil {
		v := el.Value.(*protocolConversationIdentity)
		row.ConnectionStack = append([]string(nil), v.stack...)
		if len(v.stack) > 0 {
			row.ConnectionProtocol, row.ConnectionEvidence = v.stack[0], "validated protocol session identity"
			if conversationKey(raw).transport == "network" {
				row.ConnectionEvidence = "observed protocol association; peer identity unverified"
			}
		}
	}
	return row
}

func (s *streamStore) datagramVersion(raw *capturedPacket) uint64 {
	if s == nil || raw.conversationID == 0 {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if el := s.protocolConversations[conversationKey(raw)]; el != nil {
		return el.Value.(*protocolConversationIdentity).version
	}
	return 0
}

func eventCarriers(e *pcaputil.ProtocolEvent) []string {
	if e != nil && len(e.CarrierLayers) > 0 {
		return append([]string(nil), e.CarrierLayers...)
	}
	if e != nil && e.Session["Carrier"] == "NetBIOS Session Service" {
		if names := authenticationLayers(e); len(names) > 0 {
			return append([]string{"NBSS", "SMB2"}, names[:len(names)-1]...)
		}
		return []string{"NBSS"}
	}
	if names := authenticationLayers(e); len(names) > 0 {
		return append([]string{"SMB2"}, names[:len(names)-1]...)
	}
	switch e.Protocol {
	case "grpc":
		return []string{"HTTP/2"}
	case "http3", "doq":
		return []string{"QUIC"}
	case "dot":
		if e.SourceBytes.Kind == "decrypted" {
			return []string{"TLS"}
		}
	case "doh":
		if _, ok := e.Session["Stream ID"]; ok {
			return []string{"HTTP/2"}
		}
		return []string{"HTTP"}
	}
	return nil
}

func datagramConversation(raw *capturedPacket) *streamSnapshot {
	if raw == nil || raw.history == nil || raw.conversationID == 0 {
		return nil
	}
	key := conversationKey(raw)
	v := &streamSnapshot{id: raw.conversationID | uint64(1)<<63, transport: key.transport, closed: "retained message window", domain: key.domain}
	seen := map[pcaputil.PacketReference]bool{}
	metadata := raw.history.Rows("", raw.conversationID)
	proofBudget := 0
	for _, m := range metadata {
		if m.Protocol == "l2tp" && m.Domain == v.domain && m.Transport == v.transport {
			proofBudget = 32 << 10
			break
		}
	}
	remaining := (256 << 10) - proofBudget
	proofIDs := map[uint64]int{}
	start := max(0, len(metadata)-128)
	targets := map[uint64]bool{}
	for _, id := range raw.eventIDs {
		targets[id] = true
	}
	for _, id := range raw.historyPacketEventIDs() {
		targets[id] = true
	}
	for i, meta := range metadata {
		if targets[meta.ID] {
			start = max(0, min(i-64, len(metadata)-128))
			break
		}
	}
	end := min(len(metadata), start+128)
	if start > 0 || end < len(metadata) {
		v.retentionNotice = fmt.Sprintf("Retained event window %d–%d / %d · message history is bounded", start+1, end, len(metadata))
	}
	for _, meta := range metadata[start:end] {
		if meta.Transport != v.transport || meta.Domain != v.domain || len(v.events) >= 128 {
			continue
		}
		e := retainedProtocolDetail(raw.history, meta.ID)
		if e == nil {
			continue
		}
		d := clamp(e.Direction, 0, 1)
		v.endpoints[d], v.endpoints[1-d] = e.Source, e.Destination
		// Preserve validated scalar peers before the detail byte budget can
		// replace this event with a metadata-only row. A late bootstrap
		// retransmission still targets the service port, not the selected peer.
		if peers, ok := eventValues(e)["Observed Control Peers"].([]string); ok && len(peers) == 2 {
			v.endpoints = [2]string{peers[0], peers[1]}
		}
		collectAssociationReferences(v, e, proofIDs)
		e = http3ConversationEvent(e)
		e = boundedEvent(e, &remaining)
		v.events = append(v.events, e)
		if len(v.events) == 1 {
			v.first = e.Timestamp
		}
		v.last = e.Timestamp
		v.version = e.ID
		v.decodeVersion = e.ID
		if len(e.Raw) > 0 {
			v.chunks = append(v.chunks, streamChunk{pdu: e.ID, direction: d, data: bytes.Clone(e.Raw)})
			v.bytes[d] += uint64(len(e.Raw))
		}
		for _, ref := range e.SourceBytes.PacketRefs {
			seen[ref] = true
		}
	}
	if len(v.events) == 0 {
		v.closed = "message history unavailable"
		v.protocolStack = append([]string(nil), raw.connectionStack...)
		return v
	}
	v.packets = uint64(len(seen))
	p := presentEvents(v.events)
	layers := packetPresentation{stack: append([]string(nil), raw.connectionStack...)}
	for _, name := range p.stack {
		layers.addLayer(name)
	}
	v.protocolStack = layers.stack
	v.protocol = p.protocol
	if len(p.stack) > 0 {
		v.protocol = p.stack[0]
	}
	v.evidence = "protocol session association; retained events only"
	retainAssociationProofs(v, raw.history, &proofBudget)
	v.associationReferences = nil
	return v
}

func collectAssociationReferences(v *streamSnapshot, e *pcaputil.ProtocolEvent, seen map[uint64]int) {
	if e.Protocol != "l2tp" {
		return
	}
	for _, key := range []string{"Control Context Proof", "Context Proof"} {
		for _, p := range mapList(eventValues(e)[key]) {
			id, hash := number(p["PDU"]), textValue(p["Wire SHA256"])
			if id == 0 {
				continue
			}
			if len(hash) != 64 {
				hash = ""
			}
			if i, ok := seen[id]; ok {
				if v.associationReferences[i].hash != hash {
					v.associationReferences[i].hash = ""
				}
				continue
			}
			if len(v.associationReferences) >= 128 {
				v.associationLimited = true
				continue
			}
			seen[id] = len(v.associationReferences)
			v.associationReferences = append(v.associationReferences, associationReference{id, hash})
		}
	}
}

// Setup observations are retained separately from this flow's messages. A
// missing, foreign-domain or different-byte PDU never becomes setup evidence.
func retainAssociationProofs(v *streamSnapshot, history *pcaputil.ProtocolInspector, remaining *int) {
	for _, r := range v.associationReferences {
		p := retainedProtocolDetail(history, r.id)
		if p == nil || p.Domain != v.domain || p.Transport != v.transport || p.Protocol != "l2tp" || len(p.Raw) == 0 || fmt.Sprintf("%x", sha256.Sum256(p.Raw)) != r.hash {
			v.associationUnavailable = append(v.associationUnavailable, r.id)
			continue
		}
		name := textValue(eventValues(p)["Packet Name"])
		cost := 96 + len(name) + len(r.hash)
		for _, ref := range p.SourceBytes.PacketRefs {
			cost += 96 + len(ref.Domain.Encapsulation)
		}
		if cost > *remaining {
			v.associationUnavailable = append(v.associationUnavailable, r.id)
			v.associationLimited = true
			continue
		}
		*remaining -= cost
		v.associationProofs = append(v.associationProofs, associationObservation{id: r.id, name: name, hash: r.hash, refs: append([]pcaputil.PacketReference(nil), p.SourceBytes.PacketRefs...)})
	}
}

func datagramEvidenceRows(v *streamSnapshot) []streamLine {
	rows := []streamLine{{"UDP datagram boundaries are preserved. Association uses the protocol engine's flow ID and capture domain, not a port guess.", inkCyan}}
	if v.transport == "network" {
		rows[0].text = "IP message boundaries are preserved. Association uses observed receiver parameters and capture domain; no TCP stream is inferred."
	}
	if len(v.associationProofs) > 0 {
		rows = append(rows, streamLine{"Retained setup observations · wire SHA256 matches the referenced PDU; peer/control authentication not verified", inkGold})
		for _, e := range v.associationProofs {
			var frames []uint64
			for _, ref := range e.refs {
				frames = append(frames, ref.Number)
			}
			rows = append(rows, streamLine{fmt.Sprintf("Setup PDU #%d · %s · frames %v · SHA256 %s", e.id, e.name, frames, e.hash), inkCyan})
		}
	}
	if len(v.associationUnavailable) > 0 {
		rows = append(rows, streamLine{fmt.Sprintf("Setup observations unavailable/evicted or bytes do not match: %v", v.associationUnavailable), inkGold})
	}
	if v.associationLimited {
		rows = append(rows, streamLine{"Setup proof window limit reached; additional observations are unavailable in this view", inkGold})
	}
	for _, e := range v.events {
		rows = append(rows, streamLine{fmt.Sprintf("PDU #%d · %s · %s · %s", e.ID, eventProtocolName(e), e.Status, e.Admission), inkText})
		if e.Error != "" {
			rows = append(rows, streamLine{e.ExpertCode + " · " + e.Error, inkRed})
		}
	}
	return rows
}

func authenticationLayers(e *pcaputil.ProtocolEvent) []string {
	if e.Protocol != "smb2" || e.Session["Compound"] == true {
		return nil
	}
	auth, _ := e.Session["Authentication"].(map[string]any)
	if auth["Status"] != "decoded" {
		return nil
	}
	var names []string
	if auth["Protocol"] == "SPNEGO" {
		names = append(names, "SPNEGO")
	}
	if auth["Protocol"] == "NTLMSSP" {
		names = append(names, "NTLMSSP")
	}
	if child, ok := auth["Mechanism"].(map[string]any); ok && child["Status"] == "decoded" && child["Protocol"] == "NTLMSSP" {
		names = append(names, "NTLMSSP")
	}
	return names
}

// An admitted decoder may explicitly retain an encrypted wire envelope using
// an authenticated connection/CID. This proves its carrier, never its plaintext.
func protectedConversationEvent(e *pcaputil.ProtocolEvent) bool {
	return e != nil && e.Protocol == "quic" && e.FlowID != 0 &&
		e.Admission == "observed-CID-protected-payload" && e.Session["Encrypted"] == true &&
		(e.Status == "context-required" || e.Status == "decoded") &&
		(e.ExpertCode == string(pcaputil.ErrEncrypted) || e.Error == "")
}

func protectedSSHEvent(e *pcaputil.ProtocolEvent) bool {
	return e != nil && e.Protocol == "ssh" && e.Status == "context-required" && e.ExpertCode == string(pcaputil.ErrEncrypted)
}

// Preserve only validated scalar identity on the existing bounded TCP record.
// Retention eviction can discard message bytes without erasing this identity.
// A new TCP generation/domain gets a new record and cannot inherit this stack.
func (s *streamStore) observeProtocol(e *pcaputil.ProtocolEvent) {
	if s == nil || e == nil {
		return
	}
	if e.Transport == "udp" || e.Transport == "network" && observedL2TPAssociation(e) {
		s.observeDatagram(e)
		return
	}
	haltedTLS := e.Protocol == "tls" && e.Error != ""
	if e.Transport != "tcp" || !haltedTLS && !protectedSSHEvent(e) && (e.Error != "" || e.Status != "decoded" && e.Status != "deferred") {
		return
	}
	p := identityProjection(e)
	recordEvidence := strongTLSRecord(e) && !credibleTLSIdentity(e)
	if p.protocol == "" && recordEvidence {
		p = packetPresentation{protocol: "TLS", stack: []string{"TLS"}}
	}
	if p.protocol == "" && !haltedTLS {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for element := s.lru.Back(); element != nil; element = element.Prev() {
		r := element.Value.(*tcpStream)
		if r.key.domain != e.Domain || !((r.sides[0].endpoint == e.Source && r.sides[1].endpoint == e.Destination) || (r.sides[1].endpoint == e.Source && r.sides[0].endpoint == e.Destination)) {
			continue
		}
		// Native generation IDs remain exact when capture timestamps overlap or
		// a held SYN is associated later. Endpoint/time windows alone can splice
		// protocol messages from reused tuples.
		contributes := r.nativeFlowID != 0 && r.nativeFlowID == e.TCPFlowID
		if r.nativeFlowID == 0 {
			for _, ref := range e.SourceBytes.PacketRefs {
				if ref.Domain == r.key.domain && ref.Number >= r.firstReference && ref.Number <= r.lastReference && r.firstReference != 0 {
					contributes = true
					break
				}
			}
		}
		if !contributes {
			continue
		}
		if haltedTLS {
			// Keep the observed connection identity while invalidating decoding.
			// Later ciphertext is not a decoded application PDU, even with keys.
			r.decodeDiagnostic = "TLS decoding halted after " + clipText(e.ExpertCode, 64) + "; subsequent plaintext unavailable"
			if e.Session["Record Failure Isolated"] == true {
				r.decodeDiagnostic = "TLS record rejected; later record authentication may recover; missing plaintext remains unavailable"
			}
			r.version++
			return
		}
		if e.Protocol == "tls" && e.Session["Record Recovery Verified"] == true {
			r.decodeDiagnostic = "TLS record authentication recovered; affected application direction remains incomplete"
		}
		if protectedSSHEvent(e) {
			dir := 0
			if r.sides[1].endpoint == e.Source {
				dir = 1
			}
			r.sshEncrypted[dir] = true
			r.version++
			return
		}
		if transition, _ := e.Session["Protocol Transition"].(string); strings.HasSuffix(transition, "->tls") {
			for i, name := range r.protocolStack {
				if name == eventProtocolName(e) {
					r.protocolStack[i] = name + " (STARTTLS)"
				}
			}
			r.version++
			return
		}
		if recordEvidence {
			if r.nativeIdentity || e.FlowID == 0 {
				return
			}
			if e.ID != r.lastTLSRecordID {
				r.lastTLSRecordID = e.ID
				if r.tlsRecordCount < 2 {
					r.tlsRecordCount++
				}
			}
			if r.tlsRecordCount < 2 {
				return
			}
		}
		layers := packetPresentation{stack: r.protocolStack}
		for _, name := range p.stack {
			layers.addLayer(name)
		}
		r.protocolStack = layers.stack
		if !r.nativeIdentity || e.Protocol == "tls" || e.Protocol == "ssh" || e.Protocol == "websocket" {
			r.protocol = p.protocol
		}
		r.nativeIdentity, r.evidence = true, "validated protocol events"
		if recordEvidence {
			r.evidence = "TLS carrier inferred from repeated complete records; handshake unavailable"
		}
		r.version++
		return
	}
}

func (s *streamStore) annotate(raw *capturedPacket) {
	if s == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if raw.conversationID != 0 && raw.streamID == 0 {
		if el := s.protocolConversations[conversationKey(raw)]; el != nil {
			raw.connectionStack = append([]string(nil), el.Value.(*protocolConversationIdentity).stack...)
		}
		return
	}
	if r := s.records[raw.streamID]; r != nil {
		raw.application, raw.evidence = r.protocol, r.evidence
		raw.connectionStack = append([]string(nil), r.protocolStack...)
		raw.connectionDiagnostic = r.decodeDiagnostic
		raw.opaqueTransport = r.sshEncrypted[0] && r.sshEncrypted[1]
	}
}

func (s *streamStore) projectConversation(row packetSummary, id uint64) packetSummary {
	if s == nil || id == 0 {
		return row
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r := s.records[id]; r != nil {
		row.ConnectionStack = append([]string(nil), r.protocolStack...)
		row.opaqueTransport = r.sshEncrypted[0] && r.sshEncrypted[1]
		return applyApplication(row, r.protocol, r.evidence, id)
	}
	return row
}

func conversationCaption(row packetSummary) string {
	if len(row.ProtocolStack) > 0 {
		text := row.Protocol + " · " + strings.Join(row.ProtocolStack, " › ")
		current := row.ProtocolStack[min(len(row.Layers), len(row.ProtocolStack)):]
		if len(row.ConnectionStack) > 0 && strings.Join(current, "/") != strings.Join(row.ConnectionStack, "/") {
			text += " | connection: " + strings.Join(row.ConnectionStack, " › ")
		}
		return text
	}
	if len(row.ConnectionStack) > 0 {
		return row.Protocol + " · connection: " + strings.Join(append(append([]string(nil), row.Layers...), row.ConnectionStack...), " › ")
	}
	return row.Protocol
}
