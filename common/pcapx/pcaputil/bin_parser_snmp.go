package pcaputil

import (
	"fmt"
	"strconv"
	"strings"
)

// binSNMP is the M0 session state for SNMPv3 (RFC 3412/3414/3416).
// Ports 161/162 are never consulted. Encrypted scopedPDU is fail-closed.
type binSNMP struct {
	pending map[int64]string
}

const (
	snmpFlagAuth       = 0x01
	snmpFlagPriv       = 0x02
	snmpFlagReportable = 0x04
)

func probeSNMP(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		if len(w) == 1 && w[0] == 0x30 {
			return probeNeed("snmp", "3", 1, 2)
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
		return probeNeed("snmp", "3", len(w), 2+int(w[1]&127))
	}
	if n < 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	at := 1 + hdr
	if len(w) < at+2 {
		return probeNeed("snmp", "3", len(w), at+2)
	}
	if w[at] != 0x02 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ml, mh, err := berLength(w[at+1:])
	if err != nil || ml < 1 || ml > 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if mh == 0 {
		return probeNeed("snmp", "3", len(w), at+2+int(w[at+1]&127))
	}
	if len(w) < at+1+mh+ml {
		return probeNeed("snmp", "3", len(w), at+1+mh+ml)
	}
	if snmpBERInt(w[at+1+mh:at+1+mh+ml]) != 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	next := at + 1 + mh + ml
	if next >= 1+hdr+n {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < next+1 {
		return probeNeed("snmp", "3", len(w), next+1)
	}
	// HeaderData is UNIVERSAL SEQUENCE. LDAP protocolOp is APPLICATION.
	if w[next] != 0x30 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("snmp", "3", 92)
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
	return total, f.spec("snmpv3", "SNMPv3"), nil
}

func (s *binSNMP) consume(raw []byte, max int) (map[string]any, error) {
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
	if snmpBERInt(val) != 3 {
		return nil, protocolError(ErrUnsupportedVersion, "SNMP session requires version 3")
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
		"Version":           int64(3),
		"Message ID":        msgID,
		"Maximum Size":      maxSize,
		"Message Flags":     flags,
		"Security Model":    model,
		"Engine ID":         engineID,
		"Engine Boots":      boots,
		"Engine Time":       engineTime,
		"User Name":         string(user),
		"Auth Params Bytes": len(auth),
		"Priv Params Bytes": len(priv),
		"Security Level":    snmpSecurityLevel(flags),
		"Context Level":     "observed",
	}
	if flags&snmpFlagPriv != 0 || tag == 0x04 {
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
	name, reqID, status, index, binds, err := snmpParsePDU(pduTag, pdu)
	if err != nil {
		return nil, err
	}
	info["Packet Name"] = name
	info["PDU Type"] = pduTag
	info["Request ID"] = reqID
	info["Variable Bindings"] = binds
	if name == "GetBulkRequest" {
		info["Non-Repeaters"] = status
		info["Max-Repetitions"] = index
	} else {
		info["Error Status"] = status
		info["Error Index"] = index
	}
	return s.correlate(name, reqID, info, max)
}

func (s *binSNMP) correlate(name string, reqID int64, info map[string]any, max int) (map[string]any, error) {
	switch name {
	case "GetRequest", "GetNextRequest", "SetRequest", "GetBulkRequest", "InformRequest":
		if max <= 0 {
			max = 4096
		}
		if len(s.pending) >= max {
			return info, protocolError(ErrResourceExceeded, "SNMP outstanding request-ids exceed budget")
		}
		s.pending[reqID] = name
		info["Outstanding"] = true
	case "Response", "Report":
		if want, ok := s.pending[reqID]; ok {
			delete(s.pending, reqID)
			info["Matched Request"] = want
			info["Association Status"] = "matched"
		} else {
			info["Unmatched"] = true
			info["Association Status"] = "missing-request"
			info["Context Level"] = "partial"
		}
	case "TrapV2":
		info["Unsolicited"] = true
		info["Association Status"] = "unsolicited"
	}
	return info, nil
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
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x02 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected msgMaxSize INTEGER")
	}
	maxSize = snmpBERInt(val)
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x04 || len(val) != 1 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected 1-byte msgFlags")
	}
	flags = val[0]
	tag, val, at, err = snmpReadTLV(b, at)
	if err != nil || tag != 0x02 {
		return 0, 0, 0, 0, fmt.Errorf("snmp: expected msgSecurityModel INTEGER")
	}
	model = snmpBERInt(val)
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
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x02 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected engineBoots")
	}
	boots = snmpBERInt(val)
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x02 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected engineTime")
	}
	engineTime = snmpBERInt(val)
	tag, val, at, err = snmpReadTLV(seq, at)
	if err != nil || tag != 0x04 {
		return nil, 0, 0, nil, nil, nil, fmt.Errorf("snmp: expected userName")
	}
	user = append([]byte(nil), val...)
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

func snmpParsePDU(tag byte, body []byte) (name string, reqID, status, index int64, binds []map[string]any, err error) {
	name = snmpPDUName(tag)
	if name == "" {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: unsupported PDU type 0x%02x", tag)
	}
	at := 0
	var t byte
	var val []byte
	t, val, at, err = snmpReadTLV(body, at)
	if err != nil || t != 0x02 {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: expected request-id")
	}
	reqID = snmpBERInt(val)
	t, val, at, err = snmpReadTLV(body, at)
	if err != nil || t != 0x02 {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: expected error-status")
	}
	status = snmpBERInt(val)
	t, val, at, err = snmpReadTLV(body, at)
	if err != nil || t != 0x02 {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: expected error-index")
	}
	index = snmpBERInt(val)
	t, val, at, err = snmpReadTLV(body, at)
	if err != nil || t != 0x30 {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: expected variable-bindings SEQUENCE")
	}
	if at != len(body) {
		return "", 0, 0, 0, nil, fmt.Errorf("snmp: trailing PDU data")
	}
	binds, err = snmpParseVarBinds(val)
	if err != nil {
		return "", 0, 0, 0, nil, err
	}
	return name, reqID, status, index, binds, nil
}

func snmpParseVarBinds(b []byte) ([]map[string]any, error) {
	var out []map[string]any
	for at := 0; at < len(b); {
		tag, val, next, err := snmpReadTLV(b, at)
		if err != nil || tag != 0x30 {
			return nil, fmt.Errorf("snmp: expected VarBind SEQUENCE")
		}
		bind, err := snmpParseVarBind(val)
		if err != nil {
			return nil, err
		}
		out = append(out, bind)
		at = next
	}
	return out, nil
}

func snmpParseVarBind(b []byte) (map[string]any, error) {
	tag, oidEnc, at, err := snmpReadTLV(b, 0)
	if err != nil || tag != 0x06 {
		return nil, fmt.Errorf("snmp: expected OBJECT IDENTIFIER")
	}
	vt, venc, next, err := snmpReadTLV(b, at)
	if err != nil {
		return nil, err
	}
	if next != len(b) {
		return nil, fmt.Errorf("snmp: trailing VarBind data")
	}
	info := map[string]any{
		"OID":        snmpDecodeOID(oidEnc),
		"Value Type": snmpValueName(vt),
		"Value Tag":  vt,
	}
	switch vt {
	case 0x02, 0x41, 0x42, 0x43, 0x46:
		info["Value"] = snmpBERInt(venc)
	case 0x04, 0x44:
		info["Value"] = append([]byte(nil), venc...)
	case 0x06:
		info["Value"] = snmpDecodeOID(venc)
	case 0x40:
		if len(venc) == 4 {
			info["Value"] = fmt.Sprintf("%d.%d.%d.%d", venc[0], venc[1], venc[2], venc[3])
		} else {
			info["Value"] = append([]byte(nil), venc...)
		}
	case 0x05, 0x80, 0x81, 0x82:
		info["Value"] = nil
	default:
		info["Value"] = append([]byte(nil), venc...)
	}
	return info, nil
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
	return v
}

func snmpDecodeOID(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	parts := []string{strconv.Itoa(int(b[0] / 40)), strconv.Itoa(int(b[0] % 40))}
	var v uint64
	for _, c := range b[1:] {
		if v > (^uint64(0) >> 7) {
			return strings.Join(parts, ".") + ".overflow"
		}
		v = v<<7 | uint64(c&0x7f)
		if c&0x80 == 0 {
			parts = append(parts, strconv.FormatUint(v, 10))
			v = 0
		}
	}
	return strings.Join(parts, ".")
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
