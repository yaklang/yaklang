package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// binSNMP tracks SNMPv1/v2c operations and bounded SNMPv3 USM metadata.
// Ports 161/162 are never sufficient admission evidence. Encrypted scopedPDU
// content remains opaque without caller-supplied key material.
type binSNMP struct {
	pending map[snmpPendingKey]snmpPendingRequest
	clock   time.Time
}

type snmpPendingRequest struct {
	name     string
	lastSeen time.Time
}

type snmpPendingKey struct {
	direction int
	version   int64
	requestID int64
	messageID int64
	security  string
}

const (
	snmpFlagAuth       = 0x01
	snmpFlagPriv       = 0x02
	snmpFlagReportable = 0x04
	snmpPendingTTL     = 10 * time.Minute
)

func probeSNMP(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		if len(w) == 1 && w[0] == 0x30 {
			return probeNeed("snmp", "unknown", 1, 2)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] != 0x30 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, hdr, err := berLength(w[1:])
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if hdr == 0 {
		return probeNeed("snmp", "unknown", len(w), 2+int(w[1]&127))
	}
	if n < 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	at := 1 + hdr
	if len(w) < at+2 {
		return probeNeed("snmp", "unknown", len(w), at+2)
	}
	if w[at] != 0x02 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ml, mh, err := berLength(w[at+1:])
	if err != nil || ml < 1 || ml > 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if mh == 0 {
		return probeNeed("snmp", "unknown", len(w), at+2+int(w[at+1]&127))
	}
	if len(w) < at+1+mh+ml {
		return probeNeed("snmp", "unknown", len(w), at+1+mh+ml)
	}
	if !snmpIntegerCanonical(w[at+1+mh : at+1+mh+ml]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	version := snmpBERInt(w[at+1+mh : at+1+mh+ml])
	versionName := snmpVersionName(version)
	if versionName == "" {
		return ProbeResult{Verdict: ProbeReject}
	}
	next := at + 1 + mh + ml
	if next >= 1+hdr+n {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < next+1 {
		return probeNeed("snmp", versionName, len(w), next+1)
	}
	switch version {
	case 0, 1:
		// Community-based message: OCTET STRING followed by an operation PDU.
		if w[next] != 0x04 {
			return ProbeResult{Verdict: ProbeReject}
		}
		communityLen, communityHdr, err := berLength(w[next+1:])
		if err != nil {
			return ProbeResult{Verdict: ProbeReject}
		}
		if communityHdr == 0 {
			return probeNeed("snmp", versionName, len(w), next+2+int(w[next+1]&127))
		}
		pduAt := next + 1 + communityHdr + communityLen
		if pduAt >= 1+hdr+n {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < pduAt+1 {
			return probeNeed("snmp", versionName, len(w), pduAt+1)
		}
		if !snmpPDUAllowed(version, w[pduAt]) {
			return ProbeResult{Verdict: ProbeReject}
		}
	case 3:
		// SNMPv3 HeaderData is UNIVERSAL SEQUENCE; LDAP protocolOp is
		// APPLICATION, so the shape also safely distinguishes these protocols.
		if w[next] != 0x30 {
			return ProbeResult{Verdict: ProbeReject}
		}
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("snmp", versionName, 92)
}

func (f *binFlow) frameSNMP(w []byte) (int, *binSpec, error) {
	if f.snmp == nil {
		return 0, nil, sessionContext("SNMP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(f.snmp.pending)+1)*128); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	if w[0] != 0x30 {
		return 0, nil, fmt.Errorf("snmp: expected SEQUENCE")
	}
	if len(w) < 2 {
		return 0, nil, nil
	}
	n, hdr, err := berLength(w[1:])
	if err != nil {
		return 0, nil, err
	}
	if hdr == 0 {
		return 0, nil, nil
	}
	total := 1 + hdr + n
	if total > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if total > len(w) {
		return total, nil, nil
	}
	if probe := probeSNMP(w, f.a.config.ProbeBytes); probe.Version != "3" {
		return total, f.spec("snmp", "SNMP"), nil
	}
	return total, f.spec("snmpv3", "SNMPv3"), nil
}

func (s *binSNMP) consume(raw []byte, max int, direction int) (map[string]any, error) {
	return s.consumeAt(raw, max, direction, time.Time{})
}

// consumeAt advances observed-time expiry before parsing each message. SNMP
// UDP/TCP conversations can outlive many request IDs; bounded map cardinality
// alone would otherwise let completed-in-the-capture stale requests consume
// the entire matching budget forever. A zero timestamp is used by stateless
// field decoding and deliberately disables transaction expiry.
func (s *binSNMP) consumeAt(raw []byte, max int, direction int, observed time.Time) (map[string]any, error) {
	s.expirePending(observed)
	tag, msg, next, err := snmpReadTLV(raw, 0)
	if err != nil {
		return nil, err
	}
	if tag != 0x30 || next != len(raw) {
		return nil, fmt.Errorf("snmp: expected whole-message SEQUENCE")
	}
	at := 0
	tag, val, at, err := snmpReadTLV(msg, at)
	if err != nil || tag != 0x02 {
		return nil, fmt.Errorf("snmp: expected INTEGER version")
	}
	if len(val) < 1 || len(val) > 8 || !snmpIntegerCanonical(val) {
		return nil, protocolError(ErrMalformedMessage, "SNMP invalid version integer")
	}
	version := snmpBERInt(val)
	if snmpVersionName(version) == "" {
		return nil, protocolError(ErrUnsupportedVersion, "SNMP version is outside supported v1/v2c/v3")
	}
	if version == 0 || version == 1 {
		return s.consumeCommunityMessage(msg, at, version, direction, max)
	}
	tag, header, at, err := snmpReadTLV(msg, at)
	if err != nil || tag != 0x30 {
		return nil, fmt.Errorf("snmp: expected HeaderData SEQUENCE")
	}
	msgID, maxSize, flags, model, err := snmpParseHeader(header)
	if err != nil {
		return nil, err
	}
	if model != 3 {
		return nil, protocolError(ErrUnsupportedFeature, "SNMPv3 session requires USM (securityModel 3)")
	}
	tag, params, at, err := snmpReadTLV(msg, at)
	if err != nil || tag != 0x04 {
		return nil, fmt.Errorf("snmp: expected USM parameters OCTET STRING")
	}
	engineID, boots, engineTime, user, auth, priv, err := snmpParseUSM(params)
	if err != nil {
		return nil, err
	}
	tag, scoped, at, err := snmpReadTLV(msg, at)
	if err != nil {
		return nil, err
	}
	if at != len(msg) {
		return nil, fmt.Errorf("snmp: trailing message data")
	}
	info := map[string]any{
		"Version":               int64(3),
		"Version Name":          "v3",
		"Message ID":            msgID,
		"Maximum Size":          maxSize,
		"Message Flags":         flags,
		"Security Model":        model,
		"Engine ID":             engineID,
		"Engine Boots":          boots,
		"Engine Time":           engineTime,
		"User Name":             string(user),
		"Auth Params Bytes":     len(auth),
		"Priv Params Bytes":     len(priv),
		"Security Level":        snmpSecurityLevel(flags),
		"Verified":              false,
		"Authentication Status": "not-verified-no-key-material",
		"Context Level":         "observed",
	}
	if flags&snmpFlagPriv != 0 {
		if tag != 0x04 {
			return nil, protocolError(ErrMalformedMessage, "SNMPv3 privacy flag requires encrypted OCTET STRING scopedPDU")
		}
		info["Encrypted"] = true
		info["Packet Name"] = "EncryptedScopedPDU"
		info["Scoped PDU Tag"] = tag
		return info, protocolError(ErrEncrypted, "SNMPv3 scopedPDU is encrypted; keys were not provided")
	}
	if tag != 0x30 {
		return nil, fmt.Errorf("snmp: invalid scoped PDU tag 0x%02x", tag)
	}
	ctxEngine, ctxName, pduTag, pdu, err := snmpParseScoped(scoped)
	if err != nil {
		return nil, err
	}
	info["Context Engine ID"] = ctxEngine
	info["Context Name"] = string(ctxName)
	if !snmpPDUAllowed(3, pduTag) {
		return nil, protocolError(ErrMalformedMessage, "SNMPv3 unsupported PDU for version")
	}
	parsed, err := snmpParsePDU(pduTag, pdu, max)
	if err != nil {
		return nil, err
	}
	info["Packet Name"] = parsed.name
	info["PDU Type"] = pduTag
	info["Variable Bindings"] = parsed.binds
	if parsed.hasRequestID {
		info["Request ID"] = parsed.requestID
	}
	if parsed.name == "TrapV1" {
		for k, v := range parsed.details {
			info[k] = v
		}
	} else if parsed.name == "GetBulkRequest" {
		info["Request ID"] = parsed.requestID
		info["Non-Repeaters"] = parsed.status
		info["Max-Repetitions"] = parsed.index
	} else {
		info["Error Status"] = parsed.status
		info["Error Index"] = parsed.index
	}
	info["Message ID"] = msgID
	info["Security Context"] = "usm-user-unverified"
	return s.correlate(parsed.name, parsed.requestID, msgID, 3, direction, "v3:"+snmpSecurityFingerprint(engineID, user), info, max)
}

func (s *binSNMP) expirePending(observed time.Time) {
	if observed.IsZero() {
		return
	}
	if observed.After(s.clock) {
		s.clock = observed
	}
	if s.pending == nil {
		return
	}
	for key, request := range s.pending {
		if request.lastSeen.IsZero() || s.clock.Before(request.lastSeen) {
			continue
		}
		if s.clock.Sub(request.lastSeen) >= snmpPendingTTL {
			delete(s.pending, key)
		}
	}
}

func (s *binSNMP) consumeCommunityMessage(msg []byte, at int, version int64, direction, max int) (map[string]any, error) {
	tag, community, next, err := snmpReadTLV(msg, at)
	if err != nil || tag != 0x04 {
		return nil, protocolError(ErrMalformedMessage, "SNMP expected community OCTET STRING")
	}
	communityHash := sha256.Sum256(community)
	security := "community:" + hex.EncodeToString(communityHash[:])
	pduTag, pdu, end, err := snmpReadTLV(msg, next)
	if err != nil {
		return nil, err
	}
	if end != len(msg) || !snmpPDUAllowed(version, pduTag) {
		return nil, protocolError(ErrMalformedMessage, "SNMP invalid PDU or trailing message bytes")
	}
	parsed, err := snmpParsePDU(pduTag, pdu, max)
	if err != nil {
		return nil, err
	}
	versionName := snmpVersionName(version)
	info := map[string]any{
		"Version": version, "Version Name": versionName, "Packet Name": parsed.name,
		"PDU Type": pduTag, "Variable Bindings": parsed.binds,
		"Security Level": "community-cleartext-unverified", "Verified": false,
		"Authentication Status": "community-not-verified", "Community Status": "present-unverified-cleartext",
		"Community Length": len(community), "Observation Scope": "message",
	}
	if parsed.hasRequestID {
		info["Request ID"] = parsed.requestID
	}
	if parsed.name == "TrapV1" {
		for k, v := range parsed.details {
			info[k] = v
		}
	} else if parsed.name == "GetBulkRequest" {
		info["Non-Repeaters"] = parsed.status
		info["Max-Repetitions"] = parsed.index
	} else {
		info["Error Status"] = parsed.status
		info["Error Index"] = parsed.index
	}
	return s.correlate(parsed.name, parsed.requestID, 0, version, direction, security, info, max)
}

func (s *binSNMP) correlate(name string, reqID, msgID, version int64, direction int, security string, info map[string]any, max int) (map[string]any, error) {
	if s.pending == nil {
		s.pending = map[snmpPendingKey]snmpPendingRequest{}
	}
	key := snmpPendingKey{direction: direction, version: version, requestID: reqID, messageID: msgID, security: security}
	switch name {
	case "GetRequest", "GetNextRequest", "SetRequest", "GetBulkRequest", "InformRequest":
		if max <= 0 {
			max = 4096
		}
		if len(s.pending) >= max {
			return info, protocolError(ErrResourceExceeded, "SNMP outstanding request-ids exceed budget")
		}
		if _, exists := s.pending[key]; exists {
			info["Retransmission"] = true
		}
		s.pending[key] = snmpPendingRequest{name: name, lastSeen: s.clock}
		info["Outstanding"] = true
	case "Response", "Report":
		key.direction = 1 - direction
		if request, ok := s.pending[key]; ok {
			delete(s.pending, key)
			info["Matched Request"] = request.name
			info["Association Status"] = "matched"
		} else {
			info["Unmatched"] = true
			info["Association Status"] = "missing-request"
			info["Context Level"] = "partial"
		}
	case "TrapV2":
		info["Unsolicited"] = true
		info["Association Status"] = "unsolicited"
	case "TrapV1":
		info["Unsolicited"] = true
		info["Association Status"] = "unsolicited"
	}
	return info, nil
}

func snmpVersionName(version int64) string {
	switch version {
	case 0:
		return "1"
	case 1:
		return "2c"
	case 3:
		return "3"
	default:
		return ""
	}
}

func snmpPDUAllowed(version int64, tag byte) bool {
	switch version {
	case 0: // RFC 1157
		return tag == 0xa0 || tag == 0xa1 || tag == 0xa2 || tag == 0xa3 || tag == 0xa4
	case 1: // SNMPv2c protocol operations
		return tag == 0xa0 || tag == 0xa1 || tag == 0xa2 || tag == 0xa3 || tag == 0xa5 || tag == 0xa6 || tag == 0xa7
	case 3:
		return tag == 0xa0 || tag == 0xa1 || tag == 0xa2 || tag == 0xa3 || tag == 0xa5 || tag == 0xa6 || tag == 0xa7 || tag == 0xa8
	default:
		return false
	}
}

func snmpParseHeader(b []byte) (msgID, maxSize int64, flags byte, model int64, err error) {
	at := 0
	var val []byte
	var tag byte
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x02 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected msgID INTEGER")
	}
	msgID = snmpBERInt(val)
	if len(val) == 0 || len(val) > 4 || !snmpIntegerCanonical(val) || msgID < 0 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: invalid msgID")
	}
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x02 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected msgMaxSize INTEGER")
	}
	maxSize = snmpBERInt(val)
	if len(val) == 0 || len(val) > 5 || !snmpIntegerCanonical(val) || maxSize < 484 || maxSize > 2147483647 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: invalid msgMaxSize")
	}
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x04 || len(val) != 1 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected 1-byte msgFlags")
	}
	flags = val[0]
	if flags&^byte(snmpFlagAuth|snmpFlagPriv|snmpFlagReportable) != 0 || flags&snmpFlagPriv != 0 && flags&snmpFlagAuth == 0 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: invalid msgFlags")
	}
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x02 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected msgSecurityModel INTEGER")
	}
	model = snmpBERInt(val)
	if len(val) == 0 || len(val) > 4 || !snmpIntegerCanonical(val) || model < 0 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: invalid msgSecurityModel")
	}
	if at != len(b) {
		return 0, 0, 0, 0, fmt.Errorf("snmp: header length mismatch")
	}
	return msgID, maxSize, flags, model, nil
}

func snmpParseUSM(b []byte) (engineID []byte, boots, engineTime int64, user, auth, priv []byte, err error) {
	tag, seq, next, err := snmpReadTLV(b, 0)
	if err != nil || tag != 0x30 || next != len(b) {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected USM SEQUENCE")
	}
	at := 0
	var val []byte
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x04 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected engineID")
	}
	engineID = append([]byte(nil), val...)
	if len(engineID) != 0 && (len(engineID) < 5 || len(engineID) > 32) {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: invalid engineID length")
	}
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x02 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected engineBoots")
	}
	if len(val) == 0 || len(val) > 5 || !snmpIntegerCanonical(val) || snmpBERInt(val) < 0 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: invalid engineBoots")
	}
	boots = snmpBERInt(val)
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x02 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected engineTime")
	}
	if len(val) == 0 || len(val) > 5 || !snmpIntegerCanonical(val) || snmpBERInt(val) < 0 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: invalid engineTime")
	}
	engineTime = snmpBERInt(val)
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x04 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected userName")
	}
	user = append([]byte(nil), val...)
	if len(user) > 32 {
		return nil, 0, 0, nil, nil, nil, protocolError(ErrResourceExceeded, "snmp: user name exceeds USM limit")
	}
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x04 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected authParams")
	}
	auth = append([]byte(nil), val...)
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x04 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected privParams")
	}
	priv = append([]byte(nil), val...)
	if at != len(seq) {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: trailing USM data")
	}
	return engineID, boots, engineTime, user, auth, priv, nil
}

func snmpParseScoped(b []byte) (ctxEngine, ctxName []byte, pduTag byte, pdu []byte, err error) {
	at := 0
	var tag byte
	var val []byte
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x04 {
		return nil, nil, 0, nil, fmt.Errorf("snmp: expected contextEngineID")
	}
	ctxEngine = append([]byte(nil), val...)
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x04 {
		return nil, nil, 0, nil, fmt.Errorf("snmp: expected contextName")
	}
	ctxName = append([]byte(nil), val...)
	if at >= len(b) {
		return nil, nil, 0, nil, fmt.Errorf("snmp: missing PDU")
	}
	pduTag, pdu, at, err = snmpReadTLV(b, at)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	if at != len(b) {
		return nil, nil, 0, nil, fmt.Errorf("snmp: trailing scoped PDU data")
	}
	return ctxEngine, ctxName, pduTag, pdu, nil
}

type snmpPDUResult struct {
	name         string
	requestID    int64
	hasRequestID bool
	status       int64
	index        int64
	binds        []map[string]any
	details      map[string]any
}

func snmpParsePDU(tag byte, body []byte, maxElements int) (snmpPDUResult, error) {
	if tag == 0xa4 {
		return snmpParseV1Trap(body, maxElements)
	}
	name := snmpPDUName(tag)
	if name == "" {
		return snmpPDUResult{}, fmt.Errorf("snmp: unsupported PDU type 0x%02x", tag)
	}
	if maxElements <= 0 {
		maxElements = DefaultParserBudget().MaxCollectionElements
	}
	at := 0
	var t byte
	var val []byte
	var err error
	readInteger := func(label string) (int64, error) {
		tag, bytesValue, next, e := snmpReadTLV(body, at)
		if e != nil || tag != 0x02 {
			return 0, fmt.Errorf("snmp: expected %s INTEGER", label)
		}
		if len(bytesValue) == 0 || len(bytesValue) > 8 || !snmpIntegerCanonical(bytesValue) {
			return 0, fmt.Errorf("snmp: invalid or non-minimal %s INTEGER", label)
		}
		at = next
		return snmpBERInt(bytesValue), nil
	}
	reqID, err := readInteger("request-id")
	if err != nil {
		return snmpPDUResult{}, err
	}
	status, err := readInteger("error-status")
	if err != nil {
		return snmpPDUResult{}, err
	}
	index, err := readInteger("error-index")
	if err != nil {
		return snmpPDUResult{}, err
	}
	t, val, at, err = snmpReadTLV(body, at)
	if err != nil || t != 0x30 {
		return snmpPDUResult{}, fmt.Errorf("snmp: expected variable-bindings SEQUENCE")
	}
	if at != len(body) {
		return snmpPDUResult{}, fmt.Errorf("snmp: trailing PDU data")
	}
	binds, err := snmpParseVarBinds(val, maxElements)
	if err != nil {
		return snmpPDUResult{}, err
	}
	return snmpPDUResult{name: name, requestID: reqID, hasRequestID: true, status: status, index: index, binds: binds}, nil
}

func snmpParseV1Trap(body []byte, maxElements int) (snmpPDUResult, error) {
	if maxElements <= 0 {
		maxElements = DefaultParserBudget().MaxCollectionElements
	}
	var result snmpPDUResult
	result.name = "TrapV1"
	result.details = map[string]any{}
	at := 0
	tag, enterprise, next, err := snmpReadTLV(body, at)
	if err != nil || tag != 0x06 {
		return snmpPDUResult{}, fmt.Errorf("snmp: v1 trap expected enterprise OBJECT IDENTIFIER")
	}
	result.details["Enterprise OID"], err = snmpDecodeOIDChecked(enterprise, maxElements)
	if err != nil {
		return snmpPDUResult{}, err
	}
	at = next
	tag, address, next, err := snmpReadTLV(body, at)
	if err != nil || tag != 0x40 || len(address) != 4 {
		return snmpPDUResult{}, fmt.Errorf("snmp: v1 trap expected 4-byte agent address")
	}
	result.details["Agent Address"] = fmt.Sprintf("%d.%d.%d.%d", address[0], address[1], address[2], address[3])
	at = next
	readInteger := func(label string) (int64, error) {
		tag, value, next, e := snmpReadTLV(body, at)
		if e != nil || tag != 0x02 || len(value) == 0 || len(value) > 8 || !snmpIntegerCanonical(value) {
			return 0, fmt.Errorf("snmp: v1 trap expected valid %s INTEGER", label)
		}
		at = next
		return snmpBERInt(value), nil
	}
	generic, err := readInteger("generic-trap")
	if err != nil || generic < 0 || generic > 6 {
		return snmpPDUResult{}, fmt.Errorf("snmp: invalid v1 generic-trap value")
	}
	specific, err := readInteger("specific-trap")
	if err != nil || specific < 0 {
		return snmpPDUResult{}, fmt.Errorf("snmp: invalid v1 specific-trap value")
	}
	tag, ticks, next, err := snmpReadTLV(body, at)
	if err != nil || tag != 0x43 || len(ticks) == 0 || len(ticks) > 5 {
		return snmpPDUResult{}, fmt.Errorf("snmp: v1 trap expected TimeTicks")
	}
	result.details["Generic Trap"], result.details["Specific Trap"] = generic, specific
	result.details["TimeTicks"] = snmpBERUint(ticks)
	at = next
	tag, list, next, err := snmpReadTLV(body, at)
	if err != nil || tag != 0x30 || next != len(body) {
		return snmpPDUResult{}, fmt.Errorf("snmp: v1 trap expected variable-bindings SEQUENCE")
	}
	result.binds, err = snmpParseVarBinds(list, maxElements)
	return result, err
}

func snmpParseVarBinds(b []byte, maxElements int) ([]map[string]any, error) {
	if maxElements <= 0 {
		maxElements = DefaultParserBudget().MaxCollectionElements
	}
	var out []map[string]any
	for at := 0; at < len(b); {
		if len(out) >= maxElements {
			return nil, protocolError(ErrResourceExceeded, "SNMP variable-binding budget exceeded")
		}
		tag, val, next, err := snmpReadTLV(b, at)
		if err != nil || tag != 0x30 {
			return nil, fmt.Errorf("snmp: expected VarBind SEQUENCE")
		}
		bind, err := snmpParseVarBind(val, maxElements)
		if err != nil {
			return nil, err
		}
		out = append(out, bind)
		at = next
	}
	return out, nil
}

func snmpParseVarBind(b []byte, maxElements int) (map[string]any, error) {
	tag, oidEnc, at, err := snmpReadTLV(b, 0)
	if err != nil || tag != 0x06 {
		return nil, fmt.Errorf("snmp: expected OBJECT IDENTIFIER")
	}
	oid, err := snmpDecodeOIDChecked(oidEnc, maxElements)
	if err != nil {
		return nil, err
	}
	vt, venc, next, err := snmpReadTLV(b, at)
	if err != nil {
		return nil, err
	}
	if next != len(b) {
		return nil, fmt.Errorf("snmp: trailing VarBind data")
	}
	info := map[string]any{
		"OID": oid, "Value Type": snmpValueName(vt), "Value Tag": vt,
	}
	switch vt {
	case 0x02:
		if len(venc) == 0 || len(venc) > 8 || !snmpIntegerCanonical(venc) {
			return nil, fmt.Errorf("snmp: invalid or non-minimal INTEGER value")
		}
		info["Value"] = snmpBERInt(venc)
	case 0x41, 0x42, 0x43, 0x46:
		if len(venc) == 0 || len(venc) > 8 {
			return nil, fmt.Errorf("snmp: invalid unsigned value length")
		}
		info["Value"] = snmpBERUint(venc)
	case 0x04, 0x44:
		info["Value"] = append([]byte(nil), venc...)
	case 0x06:
		info["Value"], err = snmpDecodeOIDChecked(venc, maxElements)
		if err != nil {
			return nil, err
		}
	case 0x40:
		if len(venc) != 4 {
			return nil, fmt.Errorf("snmp: invalid IpAddress length")
		}
		info["Value"] = fmt.Sprintf("%d.%d.%d.%d", venc[0], venc[1], venc[2], venc[3])
	case 0x05, 0x80, 0x81, 0x82:
		if len(venc) != 0 {
			return nil, fmt.Errorf("snmp: NULL/exception value must be empty")
		}
		info["Value"] = nil
	default:
		info["Value"] = append([]byte(nil), venc...)
	}
	return info, nil
}

func snmpDecodeOIDChecked(b []byte, maxArcs int) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("snmp: empty OBJECT IDENTIFIER")
	}
	if maxArcs <= 0 {
		maxArcs = DefaultParserBudget().MaxCollectionElements
	}
	values := make([]uint64, 0, min(maxArcs, 16))
	var value uint64
	atSubidentifierStart := true
	for i, octet := range b {
		if atSubidentifierStart && octet == 0x80 {
			return "", fmt.Errorf("snmp: non-minimal OBJECT IDENTIFIER arc")
		}
		if value > (^uint64(0) >> 7) {
			return "", protocolError(ErrResourceExceeded, "SNMP OBJECT IDENTIFIER arc overflow")
		}
		value = value<<7 | uint64(octet&0x7f)
		if octet&0x80 == 0 {
			values = append(values, value)
			if len(values)+1 > maxArcs {
				return "", protocolError(ErrResourceExceeded, "SNMP OBJECT IDENTIFIER arc budget exceeded")
			}
			value = 0
			atSubidentifierStart = true
		} else if i == len(b)-1 {
			return "", fmt.Errorf("snmp: unterminated OBJECT IDENTIFIER arc")
		} else {
			atSubidentifierStart = false
		}
	}
	if len(values) == 0 {
		return "", fmt.Errorf("snmp: malformed OBJECT IDENTIFIER")
	}
	first := values[0]
	var a, second uint64
	switch {
	case first < 40:
		a, second = 0, first
	case first < 80:
		a, second = 1, first-40
	default:
		a, second = 2, first-80
	}
	parts := []string{strconv.FormatUint(a, 10), strconv.FormatUint(second, 10)}
	for _, arc := range values[1:] {
		parts = append(parts, strconv.FormatUint(arc, 10))
	}
	return strings.Join(parts, "."), nil
}

func snmpBERUint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

func snmpSecurityFingerprint(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write(part)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func snmpReadTLV(w []byte, at int) (tag byte, val []byte, next int, err error) {
	if at >= len(w) {
		return 0, nil, at, fmt.Errorf("snmp: truncated TLV")
	}
	tag = w[at]
	if at+1 >= len(w) {
		return 0, nil, at, fmt.Errorf("snmp: truncated BER length")
	}
	n, h, err := berLength(w[at+1:])
	if err != nil {
		return 0, nil, at, err
	}
	if h == 0 || at+1+h+n > len(w) {
		return 0, nil, at, fmt.Errorf("snmp: truncated TLV")
	}
	return tag, w[at+1+h : at+1+h+n], at + 1 + h + n, nil
}

func snmpBERInt(b []byte) int64 {
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	if len(b) > 0 && len(b) < 8 && b[0]&0x80 != 0 {
		v |= -1 << (uint(len(b)) * 8)
	}
	return v
}

func snmpIntegerCanonical(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if len(b) > 1 {
		if b[0] == 0x00 && b[1]&0x80 == 0 {
			return false
		}
		if b[0] == 0xff && b[1]&0x80 != 0 {
			return false
		}
	}
	return true
}

func snmpPDUName(tag byte) string {
	switch tag {
	case 0xa0:
		return "GetRequest"
	case 0xa1:
		return "GetNextRequest"
	case 0xa2:
		return "Response"
	case 0xa3:
		return "SetRequest"
	case 0xa4:
		return "TrapV1"
	case 0xa5:
		return "GetBulkRequest"
	case 0xa6:
		return "InformRequest"
	case 0xa7:
		return "TrapV2"
	case 0xa8:
		return "Report"
	}
	return ""
}

func snmpValueName(tag byte) string {
	switch tag {
	case 0x02:
		return "INTEGER"
	case 0x04:
		return "OCTET STRING"
	case 0x05:
		return "NULL"
	case 0x06:
		return "OBJECT IDENTIFIER"
	case 0x40:
		return "IpAddress"
	case 0x41:
		return "Counter32"
	case 0x42:
		return "Gauge32"
	case 0x43:
		return "TimeTicks"
	case 0x44:
		return "Opaque"
	case 0x46:
		return "Counter64"
	case 0x80:
		return "noSuchObject"
	case 0x81:
		return "noSuchInstance"
	case 0x82:
		return "endOfMibView"
	}
	return fmt.Sprintf("tag_0x%02x", tag)
}

func snmpSecurityLevel(flags byte) string {
	switch {
	case flags&snmpFlagPriv != 0:
		return "authPriv"
	case flags&snmpFlagAuth != 0:
		return "authNoPriv"
	default:
		return "noAuthNoPriv"
	}
}
