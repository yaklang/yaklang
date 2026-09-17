package pcaputil

import (
	"bytes"
	"fmt"
)

type ldapRequest struct {
	tag       byte
	direction int
	startTLS  bool
}
type binLDAP struct{ pending map[uint64]ldapRequest }

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
	if hdr == 0 {
		return probeNeed("ldap", "3", len(w), 2+int(w[1]&127))
	}
	if n < 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if 1+hdr+n > limit && len(w) < 1+hdr+2 {
		return probeNeed("ldap", "3", len(w), 1+hdr+2)
	}
	if len(w) < 1+hdr+2 {
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
	if err := f.reserveSession(256 + int64(len(f.ldap.pending)+1)*128); err != nil {
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
	used := 0
	if err := validateLDAPBER(w[:total], 0, f.a.budget, &used); err != nil {
		return 0, nil, err
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

func (l *binLDAP) consume(dir int, raw []byte, entry string, limit int) (map[string]any, error) {
	id, tag, body, err := ldapOperation(raw)
	if err != nil {
		return nil, err
	}
	info := map[string]any{"Message ID": id, "Entry": entry, "Message Name": ldapMessageName[tag], "Context Level": "observed"}
	switch tag {
	case 0x60, 0x63, 0x66, 0x68, 0x4a, 0x6c, 0x6e, 0x77:
		if _, ok := l.pending[id]; ok {
			return info, fmt.Errorf("ldap: MessageID reused before response")
		}
		if len(l.pending) >= limit {
			return info, protocolError(ErrResourceExceeded, "LDAP outstanding requests exceed limit")
		}
		startTLS := false
		if tag == 0x77 && len(body) > 1 && body[0] == 0x80 {
			n, h, e := berLength(body[1:])
			if e == nil && h > 0 && 1+h+n <= len(body) {
				startTLS = bytes.Equal(body[1+h:1+h+n], []byte("1.3.6.1.4.1.1466.20037"))
			}
			info["StartTLS"] = startTLS
		}
		l.pending[id] = ldapRequest{tag: tag, direction: dir, startTLS: startTLS}
		info["Outstanding"] = true
	case 0x61, 0x65, 0x67, 0x69, 0x6b, 0x6d, 0x6f, 0x78, 0x64, 0x73:
		request, ok := l.pending[id]
		if ok {
			expected := request.tag + 1
			if request.tag == 0x4a {
				expected = 0x6b
			}
			validSearch := request.tag == 0x63 && (tag == 0x64 || tag == 0x65 || tag == 0x73)
			if dir == request.direction || tag != expected && !validSearch {
				return info, sessionContext("LDAP response does not match request operation/direction")
			}
			info["Matched Request"] = true
			if tag != 0x64 && tag != 0x73 {
				delete(l.pending, id)
			}
			if tag == 0x78 && request.startTLS {
				if len(body) < 3 {
					return info, fmt.Errorf("ldap: truncated StartTLS result")
				}
				n, h, e := berLength(body[1:])
				if len(body) < 3 || body[0] != 10 || e != nil || h == 0 || n != 1 || 1+h+n > len(body) {
					return info, fmt.Errorf("ldap: malformed StartTLS result")
				}
				info["StartTLS"] = body[1+h] == 0
			}
		} else if id != 0 {
			info["Unsolicited"] = true
		}
	case 0x50:
		if len(body) == 0 || len(body) > 4 || body[0]&128 != 0 {
			return info, fmt.Errorf("ldap: invalid Abandon target")
		}
		var target uint64
		for _, b := range body {
			target = target<<8 | uint64(b)
		}
		if req, ok := l.pending[target]; ok && req.direction == dir {
			delete(l.pending, target)
		}
		info["Abandoned Message ID"] = target
	case 0x42:
		clear(l.pending)
	}
	return info, nil
}

func ldapOperation(w []byte) (uint64, byte, []byte, error) {
	bad := func() (uint64, byte, []byte, error) {
		return 0, 0, nil, fmt.Errorf("ldap: invalid LDAPMessage envelope")
	}
	if len(w) < 2 || w[0] != 0x30 {
		return bad()
	}
	n, h, err := berLength(w[1:])
	if err != nil || h == 0 || 1+h+n != len(w) {
		return bad()
	}
	at := 1 + h
	if at+2 > len(w) || w[at] != 2 {
		return bad()
	}
	ml, mh, err := berLength(w[at+1:])
	if err != nil || mh == 0 || ml < 1 || ml > 4 || at+1+mh+ml >= len(w) {
		return bad()
	}
	at += 1 + mh
	if w[at]&128 != 0 {
		return bad()
	}
	var id uint64
	for _, b := range w[at : at+ml] {
		id = id<<8 | uint64(b)
	}
	at += ml
	tag := w[at]
	if at+2 > len(w) {
		return bad()
	}
	ol, oh, err := berLength(w[at+1:])
	if err != nil || oh == 0 || at+1+oh+ol > len(w) {
		return bad()
	}
	if id == 0 && tag != 0x78 {
		return bad()
	}
	return id, tag, w[at+1+oh : at+1+oh+ol], nil
}

func ldapMessageID(w []byte) (uint64, byte, error) {
	id, tag, _, err := ldapOperation(w)
	return id, tag, err
}

func validateLDAPBER(w []byte, depth int, budget ParserBudget, used *int) error {
	if depth >= budget.MaxRecursionDepth {
		return protocolError(ErrResourceExceeded, "LDAP BER nesting exceeds limit")
	}
	for len(w) > 0 {
		if len(w) < 2 {
			return fmt.Errorf("ldap: truncated BER value")
		}
		n, h, err := berLength(w[1:])
		if err != nil {
			return err
		}
		if h == 0 || n > len(w)-1-h {
			return fmt.Errorf("ldap: truncated BER value")
		}
		*used++
		if *used > budget.MaxCollectionElements {
			return protocolError(ErrResourceExceeded, "LDAP BER elements exceed limit")
		}
		if w[0]&32 != 0 {
			if err := validateLDAPBER(w[1+h:1+h+n], depth+1, budget, used); err != nil {
				return err
			}
		}
		w = w[1+h+n:]
	}
	return nil
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
