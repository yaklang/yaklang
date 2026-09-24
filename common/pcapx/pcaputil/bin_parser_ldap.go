package pcaputil

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

type ldapRequest struct {
	tag           byte
	direction     int
	startTLS      bool
	saslMechanism string
	entries       uint32
	references    uint32
}

type ldapPendingKey struct {
	direction int
	messageID uint64
}

type binLDAP struct {
	pending           map[ldapPendingKey]ldapRequest
	saslBindSucceeded bool
	reserveMemory     func(int64) error
}

// retainedBytes conservatively charges the LDAP correlation table and any
// mechanism name retained across request/response messages. SASL credentials
// themselves are never kept in the session state.
func (l *binLDAP) retainedBytes() int64 {
	n := int64(256)
	for _, request := range l.pending {
		n += 128 + int64(len(request.saslMechanism))
	}
	return n
}

func (l *binLDAP) reserve(target int64) error {
	if l.reserveMemory == nil {
		return nil
	}
	return l.reserveMemory(target)
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
	// INTEGER messageID then an APPLICATION protocolOp. SNMPv3 uses the
	// same SEQUENCE+INTEGER prefix, but HeaderData is UNIVERSAL SEQUENCE.
	at := 1 + hdr
	if w[at] != 0x02 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ml, mh, err := berLength(w[at+1:])
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if mh == 0 {
		return probeNeed("ldap", "3", len(w), at+2+int(w[at+1]&127))
	}
	next := at + 1 + mh + ml
	if next >= 1+hdr+n {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < next+1 {
		return probeNeed("ldap", "3", len(w), next+1)
	}
	if w[next]&0xc0 != 0x40 {
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
	f.ldap.reserveMemory = f.reserveSession
	if err := f.ldap.reserve(f.ldap.retainedBytes()); err != nil {
		return 0, nil, err
	}
	if len(w) == 0 {
		return 0, nil, nil
	}
	if f.ldap.saslBindSucceeded && w[0] != 0x30 {
		// A successful SASL Bind only proves that the bind result succeeded.
		// It does not expose the SASL layer selected by mechanism negotiation,
		// so a length-prefixed record cannot safely be called protected data.
		return 0, nil, sessionContext("LDAP SASL security-layer negotiation was not observed")
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
	if l.pending == nil {
		l.pending = make(map[ldapPendingKey]ldapRequest)
	}
	id, tag, body, err := ldapOperation(raw)
	if err != nil {
		return nil, err
	}
	info := map[string]any{"Message ID": id, "Entry": entry, "Message Name": ldapMessageName[tag], "Context Level": "observed"}
	switch tag {
	case 0x60, 0x63, 0x66, 0x68, 0x4a, 0x6c, 0x6e, 0x77:
		key := ldapPendingKey{direction: dir, messageID: id}
		if _, ok := l.pending[key]; ok {
			return info, fmt.Errorf("ldap: MessageID reused before response")
		}
		if len(l.pending) >= limit {
			return info, protocolError(ErrResourceExceeded, "LDAP outstanding requests exceed limit")
		}
		startTLS := false
		mechanism := ""
		if tag == 0x60 {
			mechanism, err = ldapBindSASLMechanism(body)
			if err != nil {
				return info, err
			}
			if mechanism != "" {
				info["SASL Mechanism"] = mechanism
				info["Credentials Visibility"] = "secret material is not projected"
			}
		}
		if tag == 0x77 && len(body) > 1 && body[0] == 0x80 {
			n, h, e := berLength(body[1:])
			if e == nil && h > 0 && 1+h+n <= len(body) {
				startTLS = bytes.Equal(body[1+h:1+h+n], []byte("1.3.6.1.4.1.1466.20037"))
			}
			info["StartTLS"] = startTLS
		}
		request := ldapRequest{tag: tag, direction: dir, startTLS: startTLS, saslMechanism: mechanism}
		if err := l.reserve(l.retainedBytes() + 128 + int64(len(mechanism))); err != nil {
			return info, err
		}
		if tag == 0x60 {
			l.saslBindSucceeded = false
		}
		l.pending[key] = request
		info["Outstanding"] = true
	case 0x61, 0x65, 0x67, 0x69, 0x6b, 0x6d, 0x6f, 0x78, 0x64, 0x73:
		requestKey := ldapPendingKey{direction: 1 - dir, messageID: id}
		request, ok := l.pending[requestKey]
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
			code, hasCode := ldapResultCode(body)
			if hasCode {
				info["Result Code"] = code
			}
			if tag == 0x64 || tag == 0x73 {
				if tag == 0x64 {
					request.entries++
					info["Entries Seen"] = request.entries
				} else {
					request.references++
					info["References Seen"] = request.references
				}
				info["Operation Progress"] = "partial"
				l.pending[requestKey] = request
			} else {
				delete(l.pending, requestKey)
				info["Operation Complete"] = true
			}
			if tag == 0x65 && request.tag == 0x63 {
				info["Search Complete"] = true
				info["Entries Returned"] = request.entries
				info["References Returned"] = request.references
			}
			if tag == 0x61 {
				if hasCode && code == 0 {
					info["Authentication Status"] = "success-result-observed"
				} else {
					info["Authentication Status"] = "failure-or-malformed-result-observed"
				}
				info["Identity Verified"] = false
				if hasCode && code == 0 && request.saslMechanism != "" {
					l.saslBindSucceeded = true
					info["SASL Mechanism"] = request.saslMechanism
					info["SASL Layer State"] = "bind-succeeded; selected security layer not observed"
				}
			}
			if tag == 0x78 && request.startTLS {
				if !hasCode {
					return info, fmt.Errorf("ldap: malformed StartTLS result")
				}
				info["StartTLS"] = code == 0
			}
		} else if tag == 0x78 && id == 0 {
			// RFC 4511 permits an unsolicited ExtendedResponse with messageID
			// zero. It is not correlated with any outstanding request.
			info["Unsolicited"] = true
		} else if id != 0 {
			if _, sameDirection := l.pending[ldapPendingKey{direction: dir, messageID: id}]; sameDirection {
				return info, sessionContext("LDAP response direction matches pending request")
			}
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
		key := ldapPendingKey{direction: dir, messageID: target}
		if req, ok := l.pending[key]; ok && req.direction == dir {
			delete(l.pending, key)
			info["Abandon Matched"] = true
		} else {
			info["Abandon Matched"] = false
		}
		info["Abandoned Message ID"] = target
	case 0x42:
		clear(l.pending)
		l.saslBindSucceeded = false
	}
	return info, nil
}

func (l *binLDAP) consumeProtected(dir int, raw []byte) (map[string]any, error) {
	if len(raw) < 4 {
		return nil, protocolError(ErrMalformedMessage, "LDAP SASL record length is truncated")
	}
	n := uint64(binary.BigEndian.Uint32(raw[:4]))
	if n != uint64(len(raw)-4) {
		return nil, protocolError(ErrMalformedMessage, "LDAP SASL record boundary mismatch")
	}
	return map[string]any{
		"Message Name":                    "SASLProtectedRecord",
		"Protected Length":                len(raw) - 4,
		"Transport Length":                len(raw),
		"Direction":                       dir,
		"Security Layer State":            "observed-after-successful-SASL-Bind",
		"Payload Visibility":              "opaque; no mechanism keys or unwrap context",
		"Plaintext Directory Data Parsed": false,
		"Authentication Verified":         false,
	}, nil
}

func ldapResultCode(body []byte) (uint64, bool) {
	if len(body) < 3 || body[0] != 0x0a {
		return 0, false
	}
	n, h, err := berLength(body[1:])
	if err != nil || h == 0 || n < 1 || n > 4 || 1+h+n > len(body) {
		return 0, false
	}
	var code uint64
	for _, b := range body[1+h : 1+h+n] {
		code = code<<8 | uint64(b)
	}
	return code, true
}

func ldapBindSASLMechanism(body []byte) (string, error) {
	version, rest, err := ldapReadTLV(body)
	if err != nil || version.tag != 0x02 {
		return "", protocolError(ErrMalformedMessage, "LDAP BindRequest version is malformed")
	}
	name, rest, err := ldapReadTLV(rest)
	if err != nil || name.tag != 0x04 {
		return "", protocolError(ErrMalformedMessage, "LDAP BindRequest name is malformed")
	}
	if len(rest) == 0 || rest[0] != 0xa3 {
		return "", nil
	}
	sasl, _, err := ldapReadTLV(rest)
	if err != nil || sasl.tag != 0xa3 {
		return "", protocolError(ErrMalformedMessage, "LDAP SASL credentials are malformed")
	}
	mechanism, _, err := ldapReadTLV(sasl.value)
	if err != nil || mechanism.tag != 0x04 || len(mechanism.value) == 0 || len(mechanism.value) > 256 {
		return "", protocolError(ErrMalformedMessage, "LDAP SASL mechanism is malformed")
	}
	for _, b := range mechanism.value {
		if b < 0x21 || b > 0x7e {
			return "", protocolError(ErrMalformedMessage, "LDAP SASL mechanism is not printable ASCII")
		}
	}
	return strings.ToUpper(string(mechanism.value)), nil
}

type ldapTLV struct {
	tag   byte
	value []byte
}

func ldapReadTLV(wire []byte) (ldapTLV, []byte, error) {
	if len(wire) < 2 {
		return ldapTLV{}, nil, protocolError(ErrMalformedMessage, "LDAP TLV is truncated")
	}
	n, h, err := berLength(wire[1:])
	if err != nil || h == 0 || n > len(wire)-1-h {
		return ldapTLV{}, nil, protocolError(ErrMalformedMessage, "LDAP TLV length is invalid")
	}
	end := 1 + h + n
	return ldapTLV{tag: wire[0], value: wire[1+h : end]}, wire[end:], nil
}

func ldapRedactedProjection(value any) any {
	return ldapRedactedProjectionContext(value, false)
}

func ldapRedactedProjectionContext(value any, inheritedSensitive bool) any {
	switch current := value.(type) {
	case map[string]any:
		name := ""
		if field, ok := current["Name"].(string); ok {
			name = field
		}
		if field, ok := current["Field"].(string); ok {
			name = field
		}
		sensitive := inheritedSensitive || ldapSensitiveFieldName(name) || ldapContainsSensitiveAttribute(current)
		for _, key := range []string{"Attribute Description", "Attribute Type", "Attribute Selector"} {
			if field, ok := current[key]; ok && ldapSensitiveFieldName(ldapProjectionString(field)) {
				sensitive = true
			}
		}
		out := make(map[string]any, len(current)+1)
		for key, child := range current {
			if ldapSensitiveFieldName(key) || sensitive && ldapSensitiveValueField(key) && !ldapAttributeNameField(key) {
				out[key] = ldapSecretSummary(child)
				continue
			}
			out[key] = ldapRedactedProjectionContext(child, sensitive && !ldapAttributeNameField(key))
		}
		if sensitive {
			out["Redacted"] = true
		}
		return out
	case []any:
		out := make([]any, len(current))
		for i, child := range current {
			out[i] = ldapRedactedProjectionContext(child, inheritedSensitive)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(current))
		for i, child := range current {
			out[i] = ldapRedactedProjectionContext(child, inheritedSensitive).(map[string]any)
		}
		return out
	case []byte:
		return bytes.Clone(current)
	default:
		return current
	}
}

func ldapProjectionString(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case []byte:
		return string(current)
	case map[string]any:
		for _, key := range []string{"Value", "Bytes", "Octets"} {
			if nested, ok := current[key]; ok {
				return ldapProjectionString(nested)
			}
		}
	}
	return ""
}

func ldapContainsSensitiveAttribute(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(strings.TrimSpace(key))
			if (strings.Contains(lower, "attribute description") || strings.Contains(lower, "attribute type")) && ldapSensitiveFieldName(ldapProjectionString(child)) {
				return true
			}
			if ldapContainsSensitiveAttribute(child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if ldapContainsSensitiveAttribute(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range current {
			if ldapContainsSensitiveAttribute(child) {
				return true
			}
		}
	}
	return false
}

func ldapAttributeNameField(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "attribute description") || strings.Contains(lower, "attribute type") || strings.Contains(lower, "attribute selector")
}

func ldapSensitiveValueField(name string) bool {
	lower := strings.ToLower(name)
	for _, fragment := range []string{"value", "bytes", "octets", "raw", "content", "credential", "password", "secret", "token"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func ldapSensitiveFieldName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	attributeName := strings.SplitN(lower, ";", 2)[0]
	switch attributeName {
	case "krbprincipalkey", "krbmasterkey", "krbmkey", "krbextradata", "supplementalcredentials", "msds-managedpassword":
		return true
	}
	return lower == "simple octets" || lower == "sasl octets" || strings.Contains(lower, "credential") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "unicodepwd") || strings.Contains(lower, "ntpwd") || strings.Contains(lower, "lmpwd") || lower == "auth value"
}

func ldapSecretSummary(value any) map[string]any {
	return map[string]any{"Redacted": true, "Length": ldapSecretLength(value), "Visibility": "not exported"}
}

func ldapSecretLength(value any) int {
	switch current := value.(type) {
	case []byte:
		return len(current)
	case string:
		return len(current)
	case []any:
		n := 0
		for _, nested := range current {
			n += ldapSecretLength(nested)
		}
		return n
	case []map[string]any:
		n := 0
		for _, nested := range current {
			n += ldapSecretLength(nested)
		}
		return n
	case map[string]any:
		total := 0
		for key, nested := range current {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "attribute value") {
				if encoded, ok := nested.(string); ok {
					if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
						total += len(decoded)
					} else {
						total += len(encoded)
					}
					continue
				}
			}
			if ldapSensitiveValueField(key) {
				total += ldapSecretLength(nested)
			}
		}
		return total
	}
	return 0
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
