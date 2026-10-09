package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

// KNX discovery follows Calimero Core v2.5.1, commit 13badcae4466072561ce26d0d0719b8628651c84.
// It is a reference profile, not a claim of full KNX standard conformance.
// PFCP heartbeat follows TS29.244 Release16 v16.12.1, sections 4, 6.4,
// 7.2.2.2, 7.4.2, 7.6 and 8.2.65. Unknown IE bodies remain opaque.
func discoveryError(kind ProtocolErrorKind, s string) error {
	return protocolError(kind, "discovery: %s", s)
}

func decodeKNXSearch(w []byte, limit int) (map[string]any, error) {
	bad := func(s string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, s) }
	unsupported := func(s string) (map[string]any, error) { return nil, discoveryError(ErrUnsupportedFeature, s) }
	if len(w) < 6 {
		return bad("short KNXnet/IP header")
	}
	if w[0] != 6 {
		return bad("header structure length must be 6")
	}
	if int(binary.BigEndian.Uint16(w[4:6])) != len(w) {
		return bad("total length mismatch")
	}
	if w[1] != 0x10 {
		return unsupported("basic KNXnet/IP version 1.0 only")
	}
	typ := binary.BigEndian.Uint16(w[2:4])
	extended := typ == 0x20b || typ == 0x20c
	if typ != 0x201 && typ != 0x202 && !extended {
		return unsupported("selected basic/extended SearchRequest/SearchResponse only")
	}
	if extended && limit < 7 {
		return nil, discoveryError(ErrResourceExceeded, "extended search field map exceeds collection budget")
	}
	if len(w) < 14 || w[6] != 8 {
		return bad("HPAI structure length must be 8")
	}
	if w[7] != 1 {
		if extended && w[7] == 2 && !bytes.Equal(w[8:14], make([]byte, 6)) {
			return bad("TCP HPAI requires route-back endpoint")
		}
		return unsupported("basic UDP HPAI only; other host protocols not claimed malformed")
	}
	endpoint := map[string]any{"structure_length": 8, "host_protocol": 1, "ipv4": net.IP(w[8:12]).String(), "port": binary.BigEndian.Uint16(w[12:14]), "route_back": bytes.Equal(w[8:14], make([]byte, 6)), "raw_hex": hex.EncodeToString(w[6:14])}
	out := map[string]any{"header_length": 6, "version": 16, "service_type": typ, "total_length": len(w), "endpoint": endpoint, "kind": "SearchRequest"}
	if typ == 0x201 {
		if len(w) != 14 {
			return unsupported("basic SearchRequest trailing service bytes outside bounded profile")
		}
		return out, nil
	}
	if typ == 0x20b {
		out["kind"] = "SearchRequestExtended"
		srps, err := decodeKNXSearchParameters(w[14:], limit)
		if err != nil {
			return nil, err
		}
		out["srps"] = srps
		return out, nil
	}
	out["kind"] = "SearchResponse"
	if extended {
		out["kind"] = "SearchResponseExtended"
	}
	dibs := []map[string]any{}
	seen := map[byte]bool{}
	for at := 14; at < len(w); {
		if len(w)-at < 2 {
			return bad("short DIB header")
		}
		n, t := int(w[at]), w[at+1]
		if n < 2 || n > len(w)-at {
			return bad("DIB length boundary")
		}
		if len(dibs) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "DIB count exceeds collection budget")
		}
		if ((t == 1 || t == 2) || extended && (t >= 1 && t <= 8 || t == 254)) && seen[t] {
			return bad("duplicate basic known DIB type rejected by fixed reference")
		}
		seen[t] = true
		d := w[at : at+n]
		item := map[string]any{"type": t, "length": n, "raw_hex": hex.EncodeToString(d)}
		switch t {
		case 1:
			if extended && limit < 17 {
				return nil, discoveryError(ErrResourceExceeded, "DeviceDIB field map exceeds collection budget")
			}
			if n < 54 {
				return bad("DeviceDIB shorter than fixed fields")
			}
			addr, pid := binary.BigEndian.Uint16(d[4:6]), binary.BigEndian.Uint16(d[6:8])
			name := d[24:54]
			end := bytes.IndexByte(name, 0)
			if end < 0 {
				end = len(name)
			}
			runes := make([]rune, end)
			for i, c := range name[:end] {
				runes[i] = rune(c)
			}
			item["medium"], item["status"], item["programming_mode"] = d[2], d[3], d[3]&1 != 0
			item["individual_address_raw"], item["individual_address"] = addr, fmt.Sprintf("%d.%d.%d", addr>>12, (addr>>8)&15, addr&255)
			item["project_installation_raw"], item["project_number"], item["installation_number"] = pid, pid>>4, pid&15
			item["serial_hex"], item["routing_multicast_ipv4"], item["mac_hex"] = hex.EncodeToString(d[8:14]), net.IP(d[14:18]).String(), hex.EncodeToString(d[18:24])
			item["friendly_name_raw_hex"], item["friendly_name"], item["extension_hex"] = hex.EncodeToString(name), string(runes), hex.EncodeToString(d[54:])
		case 2, 6:
			if t == 6 && !extended {
				item["handling"] = "preserved raw, no semantic claim"
				break
			}
			if (n-2)%2 != 0 {
				return bad("incomplete service family/version pair")
			}
			if (n-2)/2 > limit {
				return nil, discoveryError(ErrResourceExceeded, "service family count exceeds collection budget")
			}
			families := []map[string]any{}
			for j := 2; j < n; j += 2 {
				families = append(families, map[string]any{"id": d[j], "version": d[j+1]})
			}
			item["families"] = families
			if extended {
				item["scope"] = "supported"
				if t == 6 {
					item["scope"] = "secure-supported; no secure-session/authentication claim"
				}
			}
		default:
			item["handling"] = "preserved raw, no semantic claim"
		}
		dibs = append(dibs, item)
		at += n
	}
	if !extended && (!seen[1] || !seen[2]) {
		return unsupported("SearchResponse lacks both MVP DeviceDIB and ServiceFamiliesDIB")
	}
	out["dibs"] = dibs
	return out, nil
}

func decodePFCPSourceIP(v []byte) (map[string]any, error) {
	if len(v) < 1 {
		return nil, fmt.Errorf("SourceIP flags missing")
	}
	flags := v[0]
	at := 1
	out := map[string]any{"flags_raw": flags, "v4": flags&2 != 0, "v6": flags&1 != 0, "mpl": flags&4 != 0}
	for _, c := range []struct {
		mask byte
		n    int
		key  string
	}{{2, 4, "ipv4"}, {1, 16, "ipv6"}} {
		if flags&c.mask == 0 {
			continue
		}
		if len(v)-at < c.n {
			return nil, fmt.Errorf("optional SourceIP flagged field missing")
		}
		out[c.key] = net.IP(v[at : at+c.n]).String()
		at += c.n
	}
	if flags&4 != 0 {
		if at == len(v) {
			return nil, fmt.Errorf("optional SourceIP prefix byte missing")
		}
		out["mask_prefix_length_raw"] = v[at]
		at++
	}
	out["extension_hex"] = hex.EncodeToString(v[at:])
	return out, nil
}
func decodePFCPHeartbeat(w []byte, limit int) (map[string]any, error) {
	bad := func(s string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, s) }
	unsupported := func(s string) (map[string]any, error) { return nil, discoveryError(ErrUnsupportedFeature, s) }
	if len(w) < 8 {
		return bad("short node-related PFCP header")
	}
	if int(binary.BigEndian.Uint16(w[2:4]))+4 != len(w) {
		return bad("PFCP length mismatch")
	}
	if w[0]>>5 != 1 {
		return unsupported("PFCP version 1 only")
	}
	if w[1] != 1 && w[1] != 2 {
		return unsupported("Heartbeat types 1 and 2 only")
	}
	if w[0]&7 != 0 {
		return bad("node-related heartbeat requires FO=MP=S=0")
	}
	kind := "HeartbeatRequest"
	if w[1] == 2 {
		kind = "HeartbeatResponse"
	}
	out := map[string]any{"version": 1, "message_type": w[1], "kind": kind, "flags_raw": w[0], "s": false, "mp": false, "fo": false, "header_spare_bits_raw": (w[0] >> 3) & 3, "message_length": binary.BigEndian.Uint16(w[2:4]), "sequence": pfcpSequenceNumber(w), "last_header_spare_raw": w[7]}
	ies := []map[string]any{}
	seen := map[uint16]bool{}
	haveRecovery := false
	for at := 8; at < len(w); {
		if len(w)-at < 4 {
			return bad("short IE header")
		}
		typ, n := binary.BigEndian.Uint16(w[at:at+2]), int(binary.BigEndian.Uint16(w[at+2:at+4]))
		at += 4
		if n > len(w)-at {
			return bad("IE value exceeds datagram")
		}
		if len(ies) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "IE count exceeds collection budget")
		}
		v := w[at : at+n]
		at += n
		item := map[string]any{"type": typ, "length": n, "value_hex": hex.EncodeToString(v)}
		switch {
		case seen[typ]:
			item["handling"] = "ignored repetition (first wins)"
		case typ&0x8000 != 0:
			item["handling"] = "ignored unknown vendor"
			if n < 2 {
				item["sender_format_warning"] = "enterprise ID shorter than two bytes"
			} else {
				enterprise := binary.BigEndian.Uint16(v[:2])
				item["enterprise_id"], item["vendor_value_hex"] = enterprise, hex.EncodeToString(v[2:])
				if enterprise == 10415 {
					item["sender_format_warning"] = "3GPP enterprise ID forbidden in vendor-specific IE"
				}
			}
		case typ == 96:
			if n < 4 {
				return bad("mandatory Recovery Time Stamp fixed fields incomplete")
			}
			recovery := binary.BigEndian.Uint32(v[:4])
			out["recovery_time_stamp_ntp_seconds_1900"] = recovery
			haveRecovery = true
			item["ntp_seconds_1900"], item["extension_hex"], item["handling"] = recovery, hex.EncodeToString(v[4:]), "handled first"
		case typ == 192 && w[1] == 1:
			fields, err := decodePFCPSourceIP(v)
			if err != nil {
				item["handling"], item["reason"] = "ignored semantically incorrect optional", err.Error()
			} else {
				item["fields"], item["handling"] = fields, "handled optional"
			}
		default:
			item["handling"] = "ignored unknown or unexpected"
		}
		seen[typ] = true
		ies = append(ies, item)
	}
	if !haveRecovery {
		return bad("mandatory Recovery Time Stamp missing")
	}
	out["ies"] = ies
	return out, nil
}

type pfcpSequence struct {
	dir int
	seq uint32
}
type pfcpRequest struct {
	wire               []byte
	id                 uint64
	pending, ambiguous bool
	retransmissions    int
}
type binPFCP struct {
	seen    map[pfcpSequence]*pfcpRequest
	blocked bool
	// A terminal refusal releases the request bodies, but preserves the first
	// already ambiguous sequence's public diagnostic in a fixed-size marker.
	deniedSequence    pfcpSequence
	hasDeniedSequence bool
}

const pfcpIdleTTL = 30 * time.Second

func (s *binPFCP) pendingCount() int {
	n := 0
	for _, r := range s.seen {
		if r.pending {
			n++
		}
	}
	return n
}
func (s *binPFCP) quarantine(k pfcpSequence) {
	if r := s.seen[k]; r != nil {
		r.pending = false
		r.ambiguous = true
	}
}
func (s *binPFCP) target(w []byte, k pfcpSequence) int64 {
	n := int64(4096)
	for _, r := range s.seen {
		n += 256 + int64(len(r.wire))
	}
	if s.seen[k] == nil {
		n += 256 + int64(len(w))
	}
	return n
}
func (s *binPFCP) associate(f *binFlow, e *ProtocolEvent, w []byte, limit int) error {
	seq := pfcpSequenceNumber(w)
	k := pfcpSequence{e.Direction, seq}
	if known, matches := f.a.pfcpReceiverIdentity(e, w); known {
		e.Session["Independent Receiver SEID Matches"] = matches
		if !matches {
			e.Session["Association"], e.Session["Outstanding"] = "receiver-seid-mismatch", s.pendingCount()
			return nil
		}
	}
	association := "unmatched-response"
	if s.blocked {
		association = "ambiguous-conversation"
		blockedKey := k
		if pfcpReplyHeader(w) {
			blockedKey.dir = 1 - e.Direction
		}
		if s.hasDeniedSequence && blockedKey == s.deniedSequence {
			association = "ambiguous-sequence"
		}
	} else if pfcpRequestHeader(w) {
		association = "request"
		r := s.seen[k]
		if r != nil {
			switch {
			case r.ambiguous:
				association = "ambiguous-sequence"
			case !bytes.Equal(r.wire, w):
				s.quarantine(k)
				association = "ambiguous-sequence"
			case !r.pending:
				association = "completed-sequence-reuse"
			case r.retransmissions >= limit:
				return discoveryError(ErrResourceExceeded, "request retransmission limit")
			default:
				r.retransmissions++
				association = "retransmitted-request"
				e.TransactionID = r.id
			}
		} else {
			if len(s.seen) >= limit {
				s.blocked = true
				for _, r := range s.seen {
					r.pending = false
				}
				return discoveryError(ErrResourceExceeded, "retained request sequence limit")
			}
			if err := f.reserveSession(s.target(w, k)); err != nil {
				s.blocked = true
				for _, r := range s.seen {
					r.pending = false
				}
				return err
			}
			if s.seen == nil {
				s.seen = make(map[pfcpSequence]*pfcpRequest)
			}
			s.seen[k] = &pfcpRequest{wire: bytes.Clone(w), id: e.ID, pending: true}
			e.TransactionID = e.ID
		}
	} else if r := s.seen[pfcpSequence{1 - e.Direction, seq}]; r != nil {
		if r.ambiguous {
			association = "ambiguous-sequence"
		} else if r.pending && len(r.wire) >= 8 && r.wire[1]+1 == w[1] {
			r.pending = false
			association = "matched-response"
			e.ResponseTo = r.id
			e.TransactionID = r.id
		}
	}
	e.Session["Association"] = association
	e.Session["Outstanding"] = s.pendingCount()
	return nil
}

func (a *binParser) decodeDiscoveryDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	protocol := explicit
	if protocol == "" {
		if (src == 3671 || dst == 3671) && len(w) >= 2 && w[0] == 6 {
			protocol = "knx"
		}
		if (src == 8805 || dst == 8805) && len(w) >= 2 && w[0]>>5 == 1 {
			protocol = "pfcp"
		}
	}
	if protocol != "knx" && protocol != "pfcp" {
		return false
	}
	if protocol == "pfcp" {
		a.expirePFCP(e.Timestamp)
	}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	temp := &binFlow{a: a}
	defer temp.closeSession()
	var fields map[string]any
	var err error
	if len(w) > limit {
		err = discoveryError(ErrResourceExceeded, "datagram exceeds message byte budget")
	} else if a.budget.MaxRecursionDepth < discoveryFieldDepth(protocol, w) {
		err = discoveryError(ErrResourceExceeded, "field depth exceeds recursion budget")
	} else if err = temp.reserveSession(discoveryProjectionBytes(protocol, w)); err == nil {
		if protocol == "knx" {
			fields, err = decodeKNXSearch(w, a.budget.MaxCollectionElements)
		} else {
			fields, err = decodePFCPNode(w, a.budget.MaxCollectionElements)
		}
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = protocol, discoveryProfile(protocol, w), "wire-and-port-hint", "message"

	if explicit != "" {
		e.Admission = "explicit-decode-as"
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if fields != nil {
		e.semanticFields = cloneSession(fields)
		e.Session = cloneSession(fields)
		if w[1] == 2 && protocol == "pfcp" || protocol == "knx" && binary.BigEndian.Uint16(w[2:4]) == 0x202 {
			e.Direction = 1
		}
	}
	if protocol == "pfcp" {
		a.udpMu.Lock()
		s := a.udpSessions

		prefix := "pfcp/"
		setup := e.Profile == pfcpSetupProfile || e.Profile == pfcpUpdateProfile || e.Profile == pfcpDeletionProfile
		if setup {
			prefix = "pfcp-setup/"
		}
		key := binUDPKey{prefix + e.Source, prefix + e.Destination, e.Domain}
		if key.a > key.b {
			key.a, key.b = key.b, key.a
		}
		var el *list.Element
		if s != nil {
			el = s.entries[key]
		}
		collision := a.quarantinePFCPPeers(e, w, el)
		if err == nil && el == nil && pfcpRequestHeader(w) {
			if s != nil && len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
				err = discoveryError(ErrResourceExceeded, "UDP conversation limit")
			} else {
				f := &binFlow{a: a, id: a.flows.Add(1), protocol: "pfcp", pfcp: &binPFCP{}, endpoints: [2]string{e.Source, e.Destination}, domain: e.Domain}
				if err = f.reserveSession(4352 + int64(len(w))); err == nil {
					if s == nil {
						s = &binUDPStore{entries: make(map[binUDPKey]*list.Element), clock: e.Timestamp}
						a.udpSessions = s
					}
					el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
					s.entries[key] = el
				}
			}
		}
		if el != nil {
			v := el.Value.(*binUDPEntry)
			f := v.flow
			v.touched = s.clock
			s.lru.MoveToBack(el)
			e.FlowID = f.id
			e.Direction = 0
			if e.Source != f.endpoints[0] {
				e.Direction = 1
			}
			if err == nil {
				if collision && pfcpRequestHeader(w) {
					k := pfcpSequence{e.Direction, pfcpSequenceNumber(w)}
					err = f.pfcp.retainAmbiguous(f, k, w, e.ID, a.budget.MaxCollectionElements)
				}
				if err == nil {
					err = f.pfcp.associate(f, e, w, a.budget.MaxCollectionElements)
				}
			} else if pfcpReplyHeader(w) {
				if _, matches := a.pfcpReceiverIdentity(e, w); matches {
					f.pfcp.retireSetupReply(w, e.Direction)
				}
			} else if pfcpRequestHeader(w) {
				k := pfcpSequence{e.Direction, pfcpSequenceNumber(w)}
				if r := f.pfcp.seen[k]; r != nil && !bytes.Equal(r.wire, w) {
					f.pfcp.quarantine(k)
				}
			}

		} else if err == nil {
			e.Session["Association"], e.Session["Outstanding"] = "unmatched-response", 0
		}
		var denied *ProtocolError
		if errors.As(err, &denied) && denied.Kind == ErrResourceExceeded {
			a.blockPFCPConversation(e, w)
		}
		a.udpMu.Unlock()
	}
	if err != nil {
		e.Session, e.semanticFields = nil, nil
	}
	raw := w
	if len(w) > limit || temp.sessionBytes == 0 {
		raw = nil
	}
	// New native profiles report explicit resource denial as limited. Legacy
	// connection-state status compatibility remains in classifySessionError.
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
		err = typed
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

func discoveryFieldDepth(protocol string, w []byte) int {
	if protocol == "pfcp" {
		return 4
	}
	if knxExtendedType(w) {
		return knxExtendedFieldDepth(w)
	}
	if len(w) >= 4 && binary.BigEndian.Uint16(w[2:4]) == 0x201 {
		return 2
	}
	return 5
}
func (s *captureSession) probeDiscovery(w []byte) (ProbeResult, bool) {
	ports := s.f.ports
	protocol := ""
	if (ports[0] == 3671 || ports[1] == 3671) && len(w) > 0 && w[0] == 6 {
		protocol = "knx"
	}
	if (ports[0] == 8805 || ports[1] == 8805) && len(w) > 1 && w[0]>>5 == 1 {
		protocol = "pfcp"
	}
	if protocol == "" {
		return ProbeResult{}, false
	}
	a := s.f.a
	temp := &binFlow{a: a}
	defer temp.closeSession()
	var err error
	if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) || a.budget.MaxRecursionDepth < discoveryFieldDepth(protocol, w) {
		err = discoveryError(ErrResourceExceeded, "datagram exceeds configured probe budget")
	} else if err = temp.reserveSession(discoveryProjectionBytes(protocol, w)); err == nil {
		if protocol == "knx" {
			_, err = decodeKNXSearch(w, a.budget.MaxCollectionElements)
		} else {
			_, err = decodePFCPNode(w, a.budget.MaxCollectionElements)
		}
	}
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, true
	}
	profile := discoveryProfile(protocol, w)

	return probeAccept(protocol, profile, 98), true
}

func (a *binParser) expirePFCP(stamp time.Time) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	s := a.udpSessions
	if s == nil {
		return
	}
	if stamp.After(s.clock) {
		s.clock = stamp
	}
	for el := s.lru.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*binUDPEntry)
		if v.flow.pfcp != nil && s.clock.Sub(v.touched) >= pfcpIdleTTL {
			v.flow.closeSession()
			delete(s.entries, v.key)
			s.lru.Remove(el)
		}
		el = next
	}
}

func discoveryProfile(protocol string, w []byte) string {
	if protocol == "pfcp" {
		if pfcpDeletionType(w) {
			return pfcpDeletionProfile
		}
		if pfcpUpdateType(w) {
			return pfcpUpdateProfile
		}
		if pfcpSetupType(w) {
			return pfcpSetupProfile
		}
		return "pfcp-v1-heartbeat"
	}
	if knxExtendedType(w) {
		return knxExtendedProfile
	}
	return "knx-basic-search"
}

func discoveryProjectionBytes(protocol string, w []byte) int64 {
	base := int64(512)
	if protocol == "pfcp" && (pfcpSetupType(w) || pfcpUpdateType(w) || pfcpDeletionType(w)) {
		return pfcpSetupProjectionBytes + 256*int64(len(w))
	}
	if protocol == "knx" && knxExtendedType(w) {
		// Include nested SRP maps, byte lists and private/public field snapshots.
		// Reservation precedes allocation and is released after each datagram.
		base = 4096
	}
	return base + 128*int64(len(w))
}
