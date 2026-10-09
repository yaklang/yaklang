package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
)

const pfcpSetupProfile = "pfcp-v1-association-setup-core"
const pfcpSetupProjectionBytes int64 = 32 << 10

func pfcpSetupType(w []byte) bool { return len(w) >= 2 && (w[1] == 5 || w[1] == 6) }

// TS29.244 v16.12.1 8.2.38/131 uses RFC1035 3.1 labels with the final root
// octet omitted. Preserve binary labels/case; compression and guessed suffixes
// would change the captured identity. The identity is an unauthenticated claim.
func pfcpFQDN(w []byte, limit int) (map[string]any, error) {
	if len(w) == 0 || len(w) > 254 {
		return nil, discoveryError(ErrMalformedMessage, "FQDN encoded length outside1..254")
	}
	labels := []string{}
	names := []string{}
	for at := 0; at < len(w); {
		n := int(w[at])
		at++
		if n == 0 || n > 63 || n > len(w)-at {
			return nil, discoveryError(ErrMalformedMessage, "FQDN label width/boundary")
		}
		if len(labels) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "FQDN label collection budget")
		}
		v := w[at : at+n]
		at += n
		labels = append(labels, hex.EncodeToString(v))
		var name strings.Builder
		for _, b := range v {
			if b >= 33 && b <= 126 && b != '.' && b != '\\' {
				name.WriteByte(b)
			} else {
				fmt.Fprintf(&name, "\\%03d", b)
			}
		}
		names = append(names, name.String())
	}
	return map[string]any{"labels_hex": labels, "presentation": strings.Join(names, "."), "encoded_hex": hex.EncodeToString(w)}, nil
}
func pfcpNodeID(v []byte, limit int) (map[string]any, error) {
	if len(v) == 0 {
		return nil, discoveryError(ErrMalformedMessage, "NodeID type missing")
	}
	typ := v[0] & 15
	out := map[string]any{"type": typ, "spare_raw": v[0] >> 4, "raw_hex": hex.EncodeToString(v)}
	switch typ {
	case 0, 1:
		n := 4
		if typ == 1 {
			n = 16
		}
		if len(v) < n+1 {
			return nil, discoveryError(ErrMalformedMessage, "NodeID fixed address incomplete")
		}
		out["address"], out["extension_hex"] = net.IP(v[1:1+n]).String(), hex.EncodeToString(v[n+1:])
	case 2:
		f, err := pfcpFQDN(v[1:], limit)
		if err != nil {
			return nil, err
		}
		out["fqdn"], out["extension_hex"] = f, ""
	default:
		return nil, discoveryError(ErrUnsupportedFeature, "future NodeID type has no interpreted identity")
	}
	return out, nil
}

var pfcpUPFeatureNames = []string{"BUCP", "DDND", "DLBD", "TRST", "FTUP", "PFDM", "HEEU", "TREU", "EMPU", "PDIU", "UDBC", "QUOAC", "TRACE", "FRRT", "PFDE", "EPFAR", "DPDRA", "ADPDP", "UEIP", "SSET", "MNOP", "MTE", "BUNDL", "GCOM", "MPAS", "RTTL", "VTIME", "NORP", "IPTV", "IP6PL", "TSCU", "MPTCP", "ATSSS-LL", "QFQM", "GPQM", "MT-EDT", "CIOT", "ETHAR", "DDDS", "RDS", "RTTWP"}
var pfcpCPFeatureNames = []string{"LOAD", "OVRL", "EPFAR", "SSET", "BUNDL", "MPAS", "ARDR", "UIAUR"}

func pfcpFeatures(v []byte, cp bool, limit int) (map[string]any, error) {
	names := pfcpUPFeatureNames
	known := 8
	masks := []byte{255, 255, 255, 255, 255, 1, 0, 2}
	count := 42
	if cp {
		names = pfcpCPFeatureNames
		known = 1
		masks = []byte{255}
		count = 8
	}
	if count > limit {
		return nil, discoveryError(ErrResourceExceeded, "feature flag map exceeds collection budget")
	}
	flags := map[string]any{}
	for i, name := range names {
		flags[name] = i/8 < len(v) && v[i/8]&(1<<uint(i%8)) != 0
	}
	if !cp {
		flags["DBDM"] = len(v) > 7 && v[7]&2 != 0
	}
	raw := make([]byte, len(v))
	for i, b := range v {
		raw[i] = b
		if i < len(masks) {
			raw[i] &= ^masks[i]
		}
	}
	return map[string]any{"raw_hex": hex.EncodeToString(v), "flags": flags, "uninterpreted_bits_hex": hex.EncodeToString(raw), "extension_hex": hex.EncodeToString(v[min(known, len(v)):])}, nil
}

// Selected node Setup fields follow TS29.24416.12.1. Recovery is observed but
// explicitly ignored for restart decisions (7.4.4.1/2 notes). Optional or
// unverifiable conditional invalid IE values follow 7.6; TLV bounds never do.
func decodePFCPSetup(w []byte, limit int) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, why) }
	if len(w) < 8 {
		return bad("node header incomplete")
	}
	if w[0]>>5 != 1 {
		return nil, discoveryError(ErrUnsupportedFeature, "Setup version1 only")
	}
	if !pfcpSetupType(w) {
		return nil, discoveryError(ErrUnsupportedFeature, "AssociationSetup5/6 only")
	}
	if w[0]&7 != 0 {
		return bad("node-related Setup requires FO=MP=S=0")
	}
	if int(binary.BigEndian.Uint16(w[2:4]))+4 != len(w) {
		return bad("declared Setup message length differs from UDP boundary")
	}
	if limit < 25 {
		return nil, discoveryError(ErrResourceExceeded, "Setup field map exceeds collection budget")
	}
	kind := "AssociationSetupRequest"
	if w[1] == 6 {
		kind = "AssociationSetupResponse"
	}
	out := map[string]any{"version": 1, "message_type": w[1], "kind": kind, "flags_raw": w[0], "s": false, "mp": false, "fo": false, "header_spare_bits_raw": (w[0] >> 3) & 3, "last_header_spare_raw": w[7], "message_length": binary.BigEndian.Uint16(w[2:4]), "sequence": pfcpSequenceNumber(w), "node_id": nil, "recovery": nil, "cause": nil, "cp_features": nil, "up_features": nil, "smf_set_id": nil, "offending_ie": nil, "association_flags": nil, "sender_role": "unverified", "conditional_presence": "cannot infer absent feature support from capture alone", "restart_verified": false}
	type entry struct {
		value []byte
		item  map[string]any
	}
	first := map[uint16]entry{}
	ies := []map[string]any{}
	for at := 8; at < len(w); {
		if len(w)-at < 4 {
			return bad("IE header truncated")
		}
		typ, n := binary.BigEndian.Uint16(w[at:at+2]), int(binary.BigEndian.Uint16(w[at+2:at+4]))
		at += 4
		if n > len(w)-at {
			return bad("IE value exceeds message boundary")
		}
		if len(ies) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "Setup IE count exceeds collection budget")
		}
		v := w[at : at+n]
		at += n
		_, dup := first[typ]
		d := map[string]any{"type": typ, "length": n, "value_hex": hex.EncodeToString(v), "duplicate": dup}
		ies = append(ies, d)
		if dup {
			d["handling"] = "ignored repetition (first wins)"
			continue
		}
		first[typ] = entry{v, d}
		if typ&0x8000 != 0 {
			d["handling"] = "ignored unknown vendor"
			if n < 2 {
				d["sender_format_warning"] = "EnterpriseID fixed field incomplete, unknown IE ignored"
			} else {
				id := binary.BigEndian.Uint16(v[:2])
				d["enterprise_id"], d["vendor_value_hex"] = id, hex.EncodeToString(v[2:])
				if id == 10415 {
					d["sender_format_warning"] = "Enterprise10415 forbidden for vendor IE"
				}
			}
		} else {
			switch typ {
			case 60, 96, 19, 43, 89, 180, 40, 184, 259:
			default:
				d["handling"] = "preserved raw; semantics not decoded"
			}
		}
	}
	out["ies"] = ies
	rejected := false
	cause := byte(0)
	if w[1] == 6 {
		e, ok := first[19]
		if !ok {
			return bad("mandatory Cause absent")
		}
		if len(e.value) != 1 {
			return bad("Cause fixed length must be1")
		}
		cause = e.value[0]
		if cause == 0 || cause == 2 {
			return bad("Cause0 invalid or Cause2 belongs to SessionDeletion")
		}
		rejected = cause >= 64
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
		out["cause"] = map[string]any{"value": cause, "accepted": !rejected, "interpretation": interpretation, "raw_hex": hex.EncodeToString(e.value)}
		e.item["handling"] = "handled first"
	} else if e, ok := first[19]; ok {
		e.item["handling"] = "ignored unexpected IE in request"
	}
	node, ok := first[60]
	if !ok {
		return bad("mandatory NodeID absent (also required for rejection)")
	}
	nf, err := pfcpNodeID(node.value, limit)
	if err != nil {
		return nil, err
	}
	out["node_id"] = nf
	node.item["handling"] = "handled first"
	r, ok := first[96]
	if !ok && !rejected {
		return bad("mandatory Recovery absent")
	}
	if ok {
		v := r.value
		if len(v) < 4 {
			if !rejected {
				return bad("Recovery fixed fields incomplete")
			}
			r.item["handling"] = "ignored semantically incorrect non-required IE in rejection response"
		} else {
			out["recovery"] = map[string]any{"ntp_seconds_1900": binary.BigEndian.Uint32(v[:4]), "extension_hex": hex.EncodeToString(v[4:]), "raw_hex": hex.EncodeToString(v), "restart_handling": "ignored in Association Setup"}
			r.item["handling"] = "observed first; restart comparison disabled for Setup"
		}
	}
	mpas := false
	for _, c := range []struct {
		typ uint16
		key string
		cp  bool
	}{{43, "up_features", false}, {89, "cp_features", true}} {
		if e, ok := first[c.typ]; ok {
			if len(e.value) == 0 {
				e.item["handling"] = "ignored semantically incorrect optional/unverifiable conditional IE"
				continue
			}
			f, err := pfcpFeatures(e.value, c.cp, limit)
			if err != nil {
				return nil, err
			}
			out[c.key] = f
			e.item["handling"] = "handled first"
			if c.cp {
				mpas = e.value[0]&32 != 0 && !rejected
			}
		}
	}
	if e, ok := first[180]; ok {
		var fq map[string]any
		var err error
		if len(e.value) < 2 {
			err = discoveryError(ErrMalformedMessage, "SMFSetID fixed spare plus FQDN missing")
		} else {
			fq, err = pfcpFQDN(e.value[1:], limit)
		}
		if err != nil {
			var denied *ProtocolError
			if errors.As(err, &denied) && denied.Kind == ErrResourceExceeded {
				return nil, err
			}
			if mpas {
				return nil, err
			}
			e.item["handling"] = "ignored semantically incorrect optional IE"
		} else {
			out["smf_set_id"] = map[string]any{"spare_raw": e.value[0], "fqdn": fq, "raw_hex": hex.EncodeToString(e.value)}
			e.item["handling"] = "handled first"
		}
	}
	if mpas && out["smf_set_id"] == nil {
		return bad("verifiable CP MPAS conditional SMFSetID missing")
	}
	requiredOffender := rejected && (cause == 66 || cause == 67 || cause == 69)
	if e, ok := first[40]; ok {
		if len(e.value) == 2 {
			out["offending_ie"] = binary.BigEndian.Uint16(e.value)
			e.item["handling"] = "observed offending type"
		} else if requiredOffender {
			return bad("required OffendingIE fixed length must be2")
		} else {
			e.item["handling"] = "ignored semantically incorrect optional/unexpected IE"
		}
	}
	if requiredOffender && out["offending_ie"] == nil {
		return bad("OffendingIE missing for missing/incorrect IE rejection")
	}
	for _, typ := range []uint16{184, 259} {
		e, ok := first[typ]
		if !ok {
			continue
		}
		if (typ == 184) != (w[1] == 6) {
			e.item["handling"] = "ignored unexpected association flags"
			continue
		}
		v := e.value
		if len(v) == 0 {
			e.item["handling"] = "ignored semantically incorrect optional IE"
			continue
		}
		mask, spare := byte(1), byte(254)
		if typ == 184 {
			mask, spare = 2, 252
		}
		f := map[string]any{"type": typ, "raw_hex": hex.EncodeToString(v), "UUPSI": v[0]&mask != 0, "spare_bits_raw": v[0] & spare, "extension_hex": hex.EncodeToString(v[1:])}
		if typ == 184 {
			f["PSREI"] = v[0]&1 != 0
		}
		out["association_flags"] = f
		e.item["handling"] = "observed flags; claim not independently verified"
	}
	return out, nil
}

// An erroneous supported reply ends its matching transaction (7.6.1) only
// when the message type and S bit locate a trustworthy sequence. A session
// reply uses12..14; a node reply must never use SEID bytes at4..6.
func (s *binPFCP) retireSetupReply(w []byte, dir int) {
	if !pfcpReplyHeader(w) {
		return
	}
	k := pfcpSequence{1 - dir, pfcpSequenceNumber(w)}
	if r := s.seen[k]; r != nil && len(r.wire) >= 8 && r.wire[1]+1 == w[1] {
		r.pending = false
	}
}
func (f *binFlow) blockPFCPSetup() {
	f.pfcp = &binPFCP{blocked: true}
	// Keep only the bounded terminal marker. The retained ledger already covers
	// this smaller amount, so releasing request bodies cannot exceed the budget.
	if f.sessionBytes > 512 {
		f.a.buffered.Add(512 - f.sessionBytes)
		f.sessionBytes = 512
	}
}

// The generic UDP byte limit precedes native decoding. Retire this exact
// conversation before rejecting a bounded node header; otherwise a denied
// request leaves an earlier request eligible for a later, ambiguous response.
// No IE parsing, wire copy or new conversation allocation is needed here.
func (a *binParser) refusePFCPOversize(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	if explicit != "" && explicit != "pfcp" || explicit == "" && src != 8805 && dst != 8805 {
		return false
	}
	if !(pfcpRequestHeader(w) || pfcpReplyHeader(w)) {
		return false
	}
	a.expirePFCP(e.Timestamp)
	prefix := "pfcp-setup/"
	if w[1] == 1 || w[1] == 2 {
		prefix = "pfcp/"
	}
	key := binUDPKey{prefix + e.Source, prefix + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	if a.udpSessions != nil {
		if el := a.udpSessions.entries[key]; el != nil {
			e.FlowID = el.Value.(*binUDPEntry).flow.id
		}
		a.quarantinePFCPPeers(e, w, nil)
		a.blockPFCPConversation(e)
	}
	a.udpMu.Unlock()
	e.Protocol, e.Profile, e.Admission = "pfcp", discoveryProfile("pfcp", w), "wire-and-port-hint"
	if explicit != "" {
		e.Admission = "explicit-decode-as"
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	a.finishProtocolDatagram(e, nil, nil, discoveryError(ErrResourceExceeded, "UDP PFCP datagram exceeds message byte budget"))
	e.Completeness = e.Status
	a.limited.Add(uint64(len(w)))
	return true
}
