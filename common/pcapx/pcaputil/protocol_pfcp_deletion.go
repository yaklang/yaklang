package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const pfcpDeletionProfile = "pfcp-v1-session-deletion-core"

func pfcpDeletionType(w []byte) bool { return len(w) >= 2 && (w[1] == 54 || w[1] == 55) }

// The S bit selects the sequence offset. A node message with an erroneous S
// bit must not borrow SEID bytes as the sequence of another transaction.
func pfcpRequestHeader(w []byte) bool {
	return pfcpNodeRequest(w) || len(w) >= 16 && w[0]>>5 == 1 && w[0]&1 != 0 && w[1] == 54
}
func pfcpReplyHeader(w []byte) bool {
	return pfcpNodeReply(w) || len(w) >= 16 && w[0]>>5 == 1 && w[0]&1 != 0 && w[1] == 55
}
func pfcpSequenceNumber(w []byte) uint32 {
	at := 4
	if pfcpDeletionType(w) && w[0]&1 != 0 {
		at = 12
	}
	return uint32(w[at])<<16 | uint32(w[at+1])<<8 | uint32(w[at+2])
}

// TS29.244 v16.12.1 sections7.2.2.4.2,7.5.6/7 and8.2.91. This
// observation decodes the unbundled header and selected scalar controls.
// Report groups remain ordered raw IEs: an acceptance cause proves neither
// remote deletion nor completion of the subsequent usage-report procedure.
func decodePFCPDeletion(w []byte, limit int) (map[string]any, error) {
	bad := func(s string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, s) }
	if len(w) < 4 {
		return bad("PFCP fixed header incomplete")
	}
	if w[0]>>5 != 1 || !pfcpDeletionType(w) {
		return nil, discoveryError(ErrUnsupportedFeature, "Session Deletion54/55 version1 only")
	}
	if w[0]&1 == 0 {
		return bad("session-related message requires S=1")
	}
	if len(w) < 16 {
		return bad("session header incomplete")
	}
	if w[0]&4 != 0 {
		return nil, discoveryError(ErrUnsupportedFeature, "negotiated session bundle needs independent bundle context")
	}
	if int(binary.BigEndian.Uint16(w[2:4]))+4 != len(w) {
		return bad("declared PFCP message length differs from UDP boundary")
	}
	// Include the two native association metadata entries in the map budget.
	if limit < 34 {
		return nil, discoveryError(ErrResourceExceeded, "Session Deletion field map exceeds collection budget")
	}
	typ, seid, mp := w[1], binary.BigEndian.Uint64(w[4:12]), w[0]&2 != 0
	if typ == 54 && seid == 0 {
		return bad("zero receiver SEID not permitted in SessionDeletionRequest")
	}
	kind, direction, role := "SessionDeletionRequest", "UP receiver local SEID", "unverified CP claim"
	if typ == 55 {
		kind, direction, role = "SessionDeletionResponse", "CP receiver local SEID", "unverified UP claim"
	}
	var priority any
	spare := w[15]
	if mp {
		priority, spare = w[15]>>4, w[15]&15
	}
	out := map[string]any{"version": 1, "message_type": typ, "kind": kind, "flags_raw": w[0], "s": true, "mp": mp, "fo": false, "header_spare_bits_raw": w[0] >> 3 & 3, "last_header_octet_raw": w[15], "message_priority": priority, "last_header_spare_bits_raw": spare, "message_length": binary.BigEndian.Uint16(w[2:4]), "sequence": pfcpSequenceNumber(w), "seid": seid, "seid_hex": hex.EncodeToString(w[4:12]), "seid_direction": direction, "seid0_condition": nil, "cause": nil, "offending_ie": nil, "cp_features": nil, "additional_usage_reports": nil, "report_semantics_verified": false, "session_identity_verified": false, "association_state_verified": false, "actual_session_deleted_verified": false, "deletion_completion_verified": false, "sender_role": role, "conditional_presence": "URR/QER/SRR/network feature context absent; not inferred from capture"}
	type entry struct {
		value []byte
		item  map[string]any
	}
	first, seen := map[uint16]entry{}, map[uint16]bool{}
	groups := map[uint16]string{51: "LoadControlInformation", 54: "OverloadControlInformation", 79: "UsageReport", 252: "PacketRateStatusReport", 214: "SessionReport"}
	repeats := map[uint16]int{}
	ies, opaque := []map[string]any{}, []map[string]any{}
	for at := 16; at < len(w); {
		if len(w)-at < 4 {
			return bad("IE header truncated")
		}
		t, n := binary.BigEndian.Uint16(w[at:at+2]), int(binary.BigEndian.Uint16(w[at+2:at+4]))
		at += 4
		if n > len(w)-at {
			return bad("IE length exceeds message boundary")
		}
		if len(ies) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "Session Deletion IE collection budget")
		}
		v := w[at : at+n]
		at += n
		dup := seen[t]
		d := map[string]any{"type": t, "length": n, "value_hex": hex.EncodeToString(v), "duplicate": dup}
		ies = append(ies, d)
		if dup && !(typ == 55 && (t == 79 || t == 214)) {
			d["handling"] = "ignored repetition (first wins)"
			continue
		}
		seen[t] = true
		if t&0x8000 != 0 {
			d["handling"] = "ignored unknown vendor; retained opaque"
			if n >= 2 {
				d["enterprise_id"], d["vendor_value_hex"] = binary.BigEndian.Uint16(v[:2]), hex.EncodeToString(v[2:])
			} else {
				d["sender_format_warning"] = "vendor EnterpriseID incomplete; legacy ignores unknown"
			}
			continue
		}
		if typ == 55 && t == 114 {
			d["handling"] = "possible rejection exception retained raw; FailedRuleID semantics not decoded"
			continue
		}
		if name := groups[t]; typ == 55 && name != "" {
			d["handling"] = "allowed grouped IE retained raw; grouped semantics NOT verified"
			opaque = append(opaque, map[string]any{"type": t, "kind": name, "raw_hex": hex.EncodeToString(v), "repeat_index": repeats[t], "semantics_verified": false})
			repeats[t]++
			continue
		}
		if typ == 54 && t == 89 || typ == 55 && (t == 19 || t == 40 || t == 126) {
			first[t] = entry{v, d}
		} else {
			d["handling"] = "ignored unexpected/unknown IE; retained opaque"
		}
	}
	out["ies"], out["opaque_groups"] = ies, opaque
	if e, ok := first[89]; ok {
		if len(e.value) == 0 {
			e.item["handling"] = "ignored semantically incorrect optional IE"
		} else {
			f, err := pfcpFeatures(e.value, true, limit)
			if err != nil {
				return nil, err
			}
			out["cp_features"], e.item["handling"] = f, "observed CP features; ARDR supported as request advertisement"
		}
	}
	if typ == 54 {
		return out, nil
	}
	e, ok := first[19]
	if !ok || len(e.value) != 1 {
		return bad("mandatory Cause absent or fixed length differs from1")
	}
	cause := e.value[0]
	if cause == 0 {
		return bad("Cause0 reserved invalid")
	}
	interpretation := "defined rejection"
	switch {
	case cause == 1:
		interpretation = "request accepted"
	case cause == 2:
		interpretation = "more usage reports to send"
	case cause < 64:
		interpretation = "unspecified acceptance"
	case cause == 64:
		interpretation = "request rejected"
	case cause >= 80:
		interpretation = "unspecified rejection"
	}
	out["cause"], e.item["handling"] = map[string]any{"value": cause, "accepted": cause < 64, "interpretation": interpretation, "raw_hex": hex.EncodeToString(e.value)}, "handled first"
	if seid == 0 {
		if cause < 64 {
			return bad("accepted SessionDeletionResponse requires receiver CP SEID")
		}
		condition := "protocol-error response implementation option; request error context unverified"
		if cause == 65 {
			condition = "unknown session response"
		}
		out["seid0_condition"] = condition
	}
	requiredOffender := cause == 66 || cause == 67 || cause == 69
	if e, ok := first[40]; ok {
		if len(e.value) == 2 {
			out["offending_ie"], e.item["handling"] = binary.BigEndian.Uint16(e.value), "observed offending type"
		} else if requiredOffender {
			return bad("required OffendingIE fixed length must be2")
		} else {
			e.item["handling"] = "ignored semantically incorrect optional/unverifiable conditional IE"
		}
	}
	if requiredOffender && out["offending_ie"] == nil {
		return bad("offending type missing for IE missing/incorrect rejection")
	}
	if e, ok := first[126]; ok {
		v := e.value
		if len(v) < 2 {
			if cause == 2 {
				return bad("Cause2 requires complete AdditionalUsageReports fixed fields")
			}
			e.item["handling"] = "ignored semantically incorrect optional/unverifiable conditional IE"
		} else {
			auri, count := v[0]&128 != 0, binary.BigEndian.Uint16(v[:2])&32767
			var interpreted any = count
			if auri {
				interpreted = nil
			}
			out["additional_usage_reports"] = map[string]any{"raw_hex": hex.EncodeToString(v), "AURI": auri, "number_raw": count, "number_interpreted": interpreted, "number_ignored": auri, "extension_hex": hex.EncodeToString(v[2:])}
			e.item["handling"] = "observed first; follow-on report operation NOT executed"
		}
	}
	if cause == 2 && out["additional_usage_reports"] == nil {
		return bad("Cause2 requires AdditionalUsageReports126 under5.2.2.3.1")
	}
	if cause >= 64 {
		for _, d := range ies {
			t := d["type"].(uint16)
			if (t == 79 || t == 126 || t == 252 || t == 214) && !d["duplicate"].(bool) {
				d["handling"] = "additional non-required rejection IE retained raw; not interpreted as usage/deletion success"
			}
		}
	}
	a := first[126].value
	out["additional_reports_expected_claim"] = cause < 64 && (cause == 2 || len(a) >= 2 && (a[0]&128 != 0 || binary.BigEndian.Uint16(a[:2])&32767 != 0))
	return out, nil
}

// A single immutable caller-supplied binding is confined to the isolated
// byte session. It cannot infer an authenticated CP/UP role from packet order.
type pfcpSessionIdentity struct {
	endpoints [2]string
	seids     [2]uint64
}

// WithSessionPFCPSessionIdentity supplies independently obtained receiver-local
// SEIDs for endpoints0 and1 of a UDP ProtocolSession. Request and response SEIDs
// need not be equal. A mismatched receiver SEID is observed without consuming
// a pending candidate. This option does not authenticate a peer or prove that
// a remote session was deleted, and does not enable a live-capture binding.
func WithSessionPFCPSessionIdentity(first, second uint64) ProtocolSessionOption {
	return func(s *captureSession) error {
		if first == 0 || second == 0 {
			return fmt.Errorf("PFCP independent receiver SEIDs must be nonzero")
		}
		s.f.a.pfcpSessionIdentity = &pfcpSessionIdentity{seids: [2]uint64{first, second}}
		return nil
	}
}

func (a *binParser) pfcpReceiverIdentity(e *ProtocolEvent, w []byte) (known, matches bool) {
	b := a.pfcpSessionIdentity
	if b == nil || !pfcpDeletionType(w) || len(w) < 16 || w[0]&1 == 0 {
		return false, true
	}
	for i, endpoint := range b.endpoints {
		if e.Destination == endpoint {
			seid := binary.BigEndian.Uint64(w[4:12])
			// A rejection for an unknown session or erroneous request can use0.
			// It carries no positive independent session-identity evidence.
			if w[1] == 55 && seid == 0 {
				return false, true
			}
			return true, seid == b.seids[i]
		}
	}
	return false, true
}
