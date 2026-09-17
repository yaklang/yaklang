package pcaputil

import (
	"fmt"
)

type binLDAP struct {
	pending map[uint64]string
}

func probeLDAP(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		if len(w) == 1 && w[0] == 0x30 {
			return probeNeed("ldap", "3", 1, 2)
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
	if n < 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if 1+hdr+n > limit && len(w) < 1+hdr+2 {
		return probeNeed("ldap", "3", len(w), 1+hdr+2)
	}
	// INTEGER messageID then an APPLICATION protocolOp.
	if len(w) >= 1+hdr+2 && w[1+hdr] != 0x02 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = n
	return probeAccept("ldap", "3", 85)
}

func berLength(w []byte) (n, hdr int, err error) {
	if len(w) < 1 {
		return 0, 0, fmt.Errorf("ldap: truncated BER length")
	}
	b := w[0]
	if b < 0x80 {
		return int(b), 1, nil
	}
	width := int(b & 0x7f)
	if width == 0 || width > 4 {
		return 0, 0, fmt.Errorf("ldap: unsupported BER length")
	}
	if len(w) < 1+width {
		return 0, 0, nil
	}
	for i := 0; i < width; i++ {
		n = n<<8 | int(w[1+i])
	}
	return n, 1 + width, nil
}

func (f *binFlow) frameLDAP(w []byte) (int, *binSpec, error) {
	if f.ldap == nil {
		return 0, nil, sessionContext("LDAP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(f.ldap.pending))*32); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	if w[0] != 0x30 {
		return 0, nil, fmt.Errorf("ldap: expected SEQUENCE")
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
	entry, err := ldapEntry(w[:total])
	if err != nil {
		return 0, nil, err
	}
	return total, f.spec("ldap_fields", entry), nil
}

func ldapEntry(w []byte) (string, error) {
	_, hdr, err := berLength(w[1:])
	if err != nil {
		return "", err
	}
	at := 1 + hdr
	if at >= len(w) || w[at] != 0x02 {
		return "", fmt.Errorf("ldap: expected INTEGER messageID")
	}
	ml, mh, err := berLength(w[at+1:])
	if err != nil || mh == 0 {
		return "", fmt.Errorf("ldap: truncated messageID")
	}
	at += 1 + mh + ml
	if at >= len(w) {
		return "", fmt.Errorf("ldap: missing protocolOp")
	}
	tag := w[at]
	name, ok := ldapTagEntry[tag]
	if !ok {
		return "", fmt.Errorf("ldap: unsupported protocolOp 0x%02x", tag)
	}
	return name, nil
}

var ldapTagEntry = map[byte]string{
	0x60: "LDAPBindRequestFields",
	0x61: "LDAPBindResponseFields",
	0x42: "LDAPUnbindRequestFields",
	0x63: "LDAPSearchRequestFields",
	0x64: "LDAPSearchEntryFields",
	0x65: "LDAPSearchDoneFields",
	0x73: "LDAPSearchReferenceFields",
	0x66: "LDAPModifyRequestFields",
	0x67: "LDAPModifyResponseFields",
	0x68: "LDAPAddRequestFields",
	0x69: "LDAPAddResponseFields",
	0x4a: "LDAPDelRequestFields",
	0x6b: "LDAPDelResponseFields",
	0x6c: "LDAPModifyDNRequestFields",
	0x6d: "LDAPModifyDNResponseFields",
	0x6e: "LDAPCompareRequestFields",
	0x6f: "LDAPCompareResponseFields",
	0x50: "LDAPAbandonRequestFields",
	0x77: "LDAPExtendedRequestFields",
	0x78: "LDAPExtendedResponseFields",
}

func (l *binLDAP) consume(raw []byte, entry string) (map[string]any, error) {
	id, tag, err := ldapMessageID(raw)
	if err != nil {
		return nil, err
	}
	info := map[string]any{
		"Message ID":   id,
		"Entry":        entry,
		"Message Name": ldapMessageName[tag],
		"Context Level": "observed",
	}
	switch tag {
	case 0x60, 0x63, 0x66, 0x68, 0x4a, 0x6c, 0x6e, 0x77:
		l.pending[id] = entry
		info["Outstanding"] = true
	case 0x61, 0x65, 0x67, 0x69, 0x6b, 0x6d, 0x6f, 0x78:
		if _, ok := l.pending[id]; ok {
			delete(l.pending, id)
			info["Matched Request"] = true
		} else if id != 0 {
			info["Unsolicited"] = true
		}
	case 0x42, 0x50:
		delete(l.pending, id)
	}
	if tag == 0x77 || tag == 0x78 {
		info["StartTLS"] = bytesContainsOID(raw, []byte("1.3.6.1.4.1.1466.20037"))
	}
	return info, nil
}

func ldapMessageID(w []byte) (uint64, byte, error) {
	_, hdr, err := berLength(w[1:])
	if err != nil {
		return 0, 0, err
	}
	at := 1 + hdr
	ml, mh, err := berLength(w[at+1:])
	if err != nil || mh == 0 || at+1+mh+ml > len(w) {
		return 0, 0, fmt.Errorf("ldap: truncated messageID")
	}
	var id uint64
	for _, b := range w[at+1+mh : at+1+mh+ml] {
		id = id<<8 | uint64(b)
	}
	op := at + 1 + mh + ml
	if op >= len(w) {
		return 0, 0, fmt.Errorf("ldap: missing protocolOp")
	}
	return id, w[op], nil
}

var ldapMessageName = map[byte]string{
	0x60: "BindRequest", 0x61: "BindResponse", 0x42: "UnbindRequest",
	0x63: "SearchRequest", 0x64: "SearchResultEntry", 0x65: "SearchResultDone",
	0x73: "SearchResultReference", 0x66: "ModifyRequest", 0x67: "ModifyResponse",
	0x68: "AddRequest", 0x69: "AddResponse", 0x4a: "DelRequest", 0x6b: "DelResponse",
	0x6c: "ModifyDNRequest", 0x6d: "ModifyDNResponse", 0x6e: "CompareRequest",
	0x6f: "CompareResponse", 0x50: "AbandonRequest", 0x77: "ExtendedRequest",
	0x78: "ExtendedResponse",
}

func bytesContainsOID(w, oid []byte) bool {
	if len(oid) == 0 || len(w) < len(oid) {
		return false
	}
	for i := 0; i+len(oid) <= len(w); i++ {
		ok := true
		for j := range oid {
			if w[i+j] != oid[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
