package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const pfcpUpdateProfile = "pfcp-v1-association-update-release-core"

func pfcpUpdateType(w []byte) bool { return len(w) >= 2 && w[1] >= 7 && w[1] <= 10 }
func pfcpNodeRequest(w []byte) bool {
	return len(w) >= 8 && w[0]>>5 == 1 && w[0]&1 == 0 && (w[1] == 1 || w[1] == 5 || w[1] == 7 || w[1] == 9)
}
func pfcpNodeReply(w []byte) bool {
	return len(w) >= 8 && w[0]>>5 == 1 && w[0]&1 == 0 && (w[1] == 2 || w[1] == 6 || w[1] == 8 || w[1] == 10)
}

// TS29.244 v16.12.1 tables7.4.4.3–6 and sections7.6/8.2.77/78/120.
// This is a passive selected-core observation. Optional configuration-dependent
// controls remain opaque; captured flags do not establish roles, association
// changes, timer execution or deletion of sessions at an endpoint.
func decodePFCPUpdate(w []byte, limit int) (map[string]any, error) {
	bad := func(s string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, s) }
	if len(w) < 8 {
		return bad("node header incomplete")
	}
	if w[0]>>5 != 1 || !pfcpUpdateType(w) {
		return nil, discoveryError(ErrUnsupportedFeature, "Association Update7/8 and Release9/10 version1 only")
	}
	if w[0]&7 != 0 {
		return bad("node-related Update/Release requires FO=MP=S=0")
	}
	if int(binary.BigEndian.Uint16(w[2:4]))+4 != len(w) {
		return bad("declared PFCP message length differs from UDP boundary")
	}
	if limit < 30 {
		return nil, discoveryError(ErrResourceExceeded, "Update/Release field map exceeds collection budget")
	}
	typ := w[1]
	kind := map[byte]string{7: "AssociationUpdateRequest", 8: "AssociationUpdateResponse", 9: "AssociationReleaseRequest", 10: "AssociationReleaseResponse"}[typ]
	var releaseDirection any
	if typ == 9 {
		releaseDirection = "CP to UP when roles independently known"
	}
	out := map[string]any{"version": 1, "message_type": typ, "kind": kind, "flags_raw": w[0], "s": false, "mp": false, "fo": false, "header_spare_bits_raw": (w[0] >> 3) & 3, "last_header_spare_raw": w[7], "message_length": binary.BigEndian.Uint16(w[2:4]), "sequence": uint32(w[4])<<16 | uint32(w[5])<<8 | uint32(w[6]), "node_id": nil, "cause": nil, "cp_features": nil, "up_features": nil, "smf_set_id": nil, "offending_ie": nil, "association_release_request": nil, "graceful_release_period": nil, "update_request_flags": nil, "sender_role": "unverified", "conditional_presence": "depends on explicit sender/node configuration; not inferred from absence", "association_state_verified": false, "actual_session_deletion_verified": false, "restart_verified": false, "release_direction_requirement": releaseDirection}
	type entry struct {
		value []byte
		item  map[string]any
	}
	first := map[uint16]entry{}
	seen := map[uint16]bool{}
	repeated := map[uint16]int{}
	ies := []map[string]any{}
	extensions := []map[string]any{}
	for at := 8; at < len(w); {
		if len(w)-at < 4 {
			return bad("IE header truncated")
		}
		t, n := binary.BigEndian.Uint16(w[at:at+2]), int(binary.BigEndian.Uint16(w[at+2:at+4]))
		at += 4
		if n > len(w)-at {
			return bad("IE length exceeds message boundary")
		}
		if len(ies) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "Update/Release IE collection budget")
		}
		v := w[at : at+n]
		at += n
		dup := seen[t]
		d := map[string]any{"type": t, "length": n, "value_hex": hex.EncodeToString(v), "duplicate": dup}
		ies = append(ies, d)
		repeat := typ == 7 && (t == 178 || t == 203 || t == 233 || t == 238 || t == 267) || typ == 8 && t == 267
		if dup && !repeat {
			d["handling"] = "ignored repetition (first wins)"
			continue
		}
		seen[t] = true
		if t&0x8000 != 0 {
			d["handling"] = "ignored unknown vendor; retained opaque"
			if n >= 2 {
				d["enterprise_id"], d["vendor_value_hex"] = binary.BigEndian.Uint16(v[:2]), hex.EncodeToString(v[2:])
			} else {
				d["sender_format_warning"] = "vendor EnterpriseID fixed field incomplete; legacy receiver ignores unknown"
			}
			continue
		}
		if repeat {
			d["handling"] = "retained every defined repeated extension; nested semantics not interpreted"
			extensions = append(extensions, map[string]any{"type": t, "raw_hex": hex.EncodeToString(v), "repeat_index": repeated[t]})
			repeated[t]++
			continue
		}
		expected := t == 60 || (typ == 8 || typ == 10) && (t == 19 || t == 40) || (typ == 7 || typ == 8) && (t == 43 || t == 89) || typ == 7 && (t == 111 || t == 112 || t == 162 || t == 180)
		if !expected {
			d["handling"] = "ignored unexpected/unknown IE; retained opaque"
			continue
		}
		first[t] = entry{v, d}
	}
	out["ies"], out["raw_extensions"] = ies, extensions
	cause := byte(0)
	if typ == 8 || typ == 10 {
		e, ok := first[19]
		if !ok || len(e.value) != 1 {
			return bad("mandatory Cause absent or fixed length differs from1")
		}
		cause = e.value[0]
		if cause == 0 || cause == 2 {
			return bad("Cause0 reserved; Cause2 belongs to SessionDeletion")
		}
		interpretation := "defined rejection"
		switch {
		case cause == 1:
			interpretation = "request accepted"
		case cause < 64:
			interpretation = "unspecified acceptance"
		case cause == 64:
			interpretation = "request rejected"
		case cause >= 80:
			interpretation = "unspecified rejection"
		}
		out["cause"] = map[string]any{"value": cause, "accepted": cause < 64, "interpretation": interpretation, "raw_hex": hex.EncodeToString(e.value)}
		e.item["handling"] = "handled first"
	}
	node, ok := first[60]
	if !ok {
		return bad("mandatory NodeID absent (also required in rejection response)")
	}
	nf, err := pfcpNodeID(node.value, limit)
	if err != nil {
		return nil, err
	}
	out["node_id"], node.item["handling"] = nf, "handled first"
	for _, c := range []struct {
		typ uint16
		key string
		cp  bool
	}{{43, "up_features", false}, {89, "cp_features", true}} {
		if e, ok := first[c.typ]; ok {
			if len(e.value) == 0 {
				e.item["handling"] = "ignored semantically incorrect optional IE"
				continue
			}
			f, err := pfcpFeatures(e.value, c.cp, limit)
			if err != nil {
				return nil, err
			}
			out[c.key], e.item["handling"] = f, "handled first"
		}
	}
	if e, ok := first[180]; ok {
		var fq map[string]any
		var err error
		if len(e.value) < 2 {
			err = discoveryError(ErrMalformedMessage, "SMFSetID fixed spare plus FQDN absent")
		} else {
			fq, err = pfcpFQDN(e.value[1:], limit)
		}
		if err != nil {
			var denied *ProtocolError
			if errors.As(err, &denied) && denied.Kind == ErrResourceExceeded {
				return nil, err
			}
			e.item["handling"] = "ignored semantically incorrect optional IE"
		} else {
			out["smf_set_id"] = map[string]any{"spare_raw": e.value[0], "fqdn": fq, "raw_hex": hex.EncodeToString(e.value)}
			e.item["handling"] = "handled first"
		}
	}
	for _, c := range []struct {
		typ uint16
		key string
	}{{111, "association_release_request"}, {112, "graceful_release_period"}, {162, "update_request_flags"}} {
		e, ok := first[c.typ]
		if !ok {
			continue
		}
		v := e.value
		if len(v) == 0 {
			e.item["handling"] = "ignored semantically incorrect optional/unverifiable conditional IE"
			continue
		}
		f := map[string]any{"raw_hex": hex.EncodeToString(v), "extension_hex": hex.EncodeToString(v[1:])}
		switch c.typ {
		case 111:
			f["SARR"], f["URSS"], f["spare_bits_raw"] = v[0]&1 != 0, v[0]&2 != 0, v[0]&252
		case 162:
			f["PARPS"], f["spare_bits_raw"] = v[0]&1 != 0, v[0]&254
		case 112:
			u, n := int(v[0]>>5), int(v[0]&31)
			interpretation := "defined unit"
			var seconds any
			if u == 7 {
				interpretation = "infinite"
			} else {
				seconds = n * []int{2, 60, 600, 3600, 36000, 60, 60}[u]
				if u == 5 || u == 6 {
					interpretation = "reserved unit interpreted as one minute"
				}
			}
			f["unit"], f["value"], f["infinite"], f["stopped"], f["seconds"], f["unit_interpretation"] = u, n, u == 7, u == 0 && n == 0, seconds, interpretation
		}
		out[c.key], e.item["handling"] = f, "observed first; no real node action inferred"
	}
	requiredOffender := cause == 66 || cause == 67 || cause == 69
	if e, ok := first[40]; ok {
		if len(e.value) == 2 {
			out["offending_ie"], e.item["handling"] = binary.BigEndian.Uint16(e.value), "observed offending type"
		} else if requiredOffender {
			return bad("required OffendingIE fixed length must be2")
		} else {
			e.item["handling"] = "ignored semantically incorrect optional IE"
		}
	}
	if requiredOffender && out["offending_ie"] == nil {
		return bad("OffendingIE missing for missing/incorrect IE rejection")
	}
	return out, nil
}

func decodePFCPNode(w []byte, limit int) (map[string]any, error) {
	if pfcpDeletionType(w) {
		return decodePFCPDeletion(w, limit)
	}
	if pfcpSetupType(w) {
		return decodePFCPSetup(w, limit)
	}
	if pfcpUpdateType(w) {
		return decodePFCPUpdate(w, limit)
	}
	return decodePFCPHeartbeat(w, limit)
}

// Called with udpMu held. Section6.4's originating IP/UDP endpoint namespace
// spans remote peers and message kinds. Capture domains remain independent.
// The store is bounded by the shared conversation budget; no auxiliary
// unbudgeted index or request-body copy is needed to detect a collision.
func (a *binParser) quarantinePFCPPeers(e *ProtocolEvent, w []byte, current *list.Element) bool {
	if !pfcpRequestHeader(w) || a.udpSessions == nil {
		return false
	}
	seq := pfcpSequenceNumber(w)
	collision := false
	for _, el := range a.udpSessions.entries {
		if el == current {
			continue
		}
		f := el.Value.(*binUDPEntry).flow
		if f.pfcp == nil || f.domain != e.Domain {
			continue
		}
		dir := -1
		if f.endpoints[0] == e.Source {
			dir = 0
		} else if f.endpoints[1] == e.Source {
			dir = 1
		}
		if dir < 0 {
			continue
		}
		k := pfcpSequence{dir, seq}
		if r := f.pfcp.seen[k]; r != nil && (r.pending || r.ambiguous) {
			f.pfcp.quarantine(k)
			collision = true
		}
	}
	return collision
}

func (s *binPFCP) retainAmbiguous(f *binFlow, k pfcpSequence, w []byte, id uint64, limit int) error {
	if s.blocked {
		return nil
	}
	if s.seen[k] != nil {
		s.quarantine(k)
		return nil
	}
	if len(s.seen) >= limit {
		return discoveryError(ErrResourceExceeded, "cross-kind retained sequence limit")
	}
	if err := f.reserveSession(s.target(w, k)); err != nil {
		return err
	}
	if s.seen == nil {
		s.seen = make(map[pfcpSequence]*pfcpRequest)
	}
	s.seen[k] = &pfcpRequest{wire: bytes.Clone(w), id: id, ambiguous: true}
	return nil
}

// A terminal resource refusal invalidates candidates at this exact endpoint
// pair/domain, including another node profile. Retain only bounded markers so
// delayed replies cannot revive a released candidate. Other peers are intact.
// Called with udpMu held.
func (a *binParser) blockPFCPConversation(e *ProtocolEvent, w []byte) {
	if a.udpSessions == nil {
		return
	}
	for _, el := range a.udpSessions.entries {
		f := el.Value.(*binUDPEntry).flow
		if f.pfcp != nil && f.domain == e.Domain && ((f.endpoints[0] == e.Source && f.endpoints[1] == e.Destination) || (f.endpoints[1] == e.Source && f.endpoints[0] == e.Destination)) {
			if pfcpRequestHeader(w) && !f.pfcp.hasDeniedSequence {
				dir := 0
				if e.Source != f.endpoints[0] {
					dir = 1
				}
				k := pfcpSequence{dir, pfcpSequenceNumber(w)}
				if r := f.pfcp.seen[k]; r != nil && (r.ambiguous || !bytes.Equal(r.wire, w)) {
					f.pfcp.deniedSequence, f.pfcp.hasDeniedSequence = k, true
				}
			}
			f.blockPFCPSetup()
		}
	}
}
