package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
)

// Fixed Gurux6e409ae9 ACSE/xDLMS codec layout. Accepted wire negotiation is
// observational only: it never certifies authentication, object rights or a meter.
func wrapperIsACSE(w []byte) bool {
	return len(w) > 8 && w[0] == 0 && w[1] == 1 && (w[8] == 0x60 || w[8] == 0x61)
}

// This header-only refusal hint allocates no key/history maps. A malformed
// unrelated tag cannot poison future untracked association eligibility.
func wrapperACSERefusalHint(w []byte) bool {
	if len(w) < 10 || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 {
		return false
	}
	c := acseCursor{w: w[8:]}
	_, body, e := c.tlv()
	if e != nil || c.at != len(c.w) {
		return false
	}
	parts := acseCursor{w: body}
	context, information := false, false
	var seen uint32
	count := 0
	for parts.at < len(body) {
		count++
		if count > 16 {
			return false
		}
		t, v, e := parts.tlv()
		if e != nil {
			return false
		}
		var bit uint32
		switch t {
		case 0xa1:
			bit = 1
		case 0xa2:
			bit = 2
		case 0xa3:
			bit = 4
		case 0xbe:
			bit = 8
		case 0x80:
			bit = 16
		default:
			return false
		}
		if seen&bit != 0 {
			return false
		}
		seen |= bit
		if w[8] == 0x60 && (t == 0xa2 || t == 0xa3) {
			return false
		}
		if t == 0xa1 {
			if context {
				return false
			}
			oid, e := acseOnlyTLV(v, 6)
			if e != nil || !bytes.Equal(oid, []byte{0x60, 0x85, 0x74, 5, 8, 1, 1}) {
				return false
			}
			context = true
		}
		if t == 0xbe {
			if information {
				return false
			}
			data, e := acseOnlyTLV(v, 4)
			if e != nil || len(data) == 0 {
				return false
			}
			information = true
		}
	}
	return context && (w[8] == 0x60 || seen&6 == 6)
}

type acseCursor struct {
	w  []byte
	at int
}

func (c *acseCursor) tlv() (byte, []byte, error) {
	if len(c.w)-c.at < 2 {
		return 0, nil, wrapperError(ErrMalformedMessage, "ACSE TLV truncated")
	}
	tag := c.w[c.at]
	b := c.w[c.at+1]
	c.at += 2
	n := int(b)
	if b&128 != 0 {
		width := int(b & 127)
		if width == 0 {
			return 0, nil, wrapperError(ErrUnsupportedFeature, "ACSE indefinite length unsupported")
		}
		if width > 2 {
			return 0, nil, wrapperError(ErrResourceExceeded, "ACSE length width exceeds bounded envelope")
		}
		if len(c.w)-c.at < width {
			return 0, nil, wrapperError(ErrMalformedMessage, "ACSE length truncated")
		}
		n = 0
		for _, v := range c.w[c.at : c.at+width] {
			n = n<<8 | int(v)
		}
		c.at += width
		if n < 128 || width == 2 && n < 256 {
			return 0, nil, wrapperError(ErrUnsupportedFeature, "ACSE noncanonical length")
		}
	}
	if n > len(c.w)-c.at {
		return 0, nil, wrapperError(ErrMalformedMessage, "ACSE value truncated")
	}
	v := c.w[c.at : c.at+n]
	c.at += n
	return tag, v, nil
}
func acseOnlyTLV(w []byte, want byte) ([]byte, error) {
	c := acseCursor{w: w}
	tag, v, e := c.tlv()
	if e != nil {
		return nil, e
	}
	if tag != want || c.at != len(w) {
		return nil, wrapperError(ErrMalformedMessage, "ACSE nested tag or trailing bytes")
	}
	return v, nil
}

type wrapperAssociationKey struct {
	dir                 int
	source, destination uint16
}
type wrapperAssociation struct {
	id                 uint64
	pending, ambiguous bool
	conformance        uint32
	maxPDU             uint16
}

func decodeWrapperACSE(w []byte, limit int) (*wrapperMessage, error) {
	if len(w)-8 > min(limit, 4096) {
		return nil, wrapperError(ErrResourceExceeded, "ACSE field-byte budget")
	}
	c := acseCursor{w: w[8:]}
	tag, body, e := c.tlv()
	if e != nil {
		return nil, e
	}
	if c.at != len(c.w) {
		return nil, wrapperError(ErrMalformedMessage, "ACSE APDU trailing bytes")
	}
	request := tag == 0x60
	f := map[string]any{"kind": "AARE", "wrapper_version": uint16(1), "source_wport": binary.BigEndian.Uint16(w[2:4]), "destination_wport": binary.BigEndian.Uint16(w[4:6]), "payload_length": len(w) - 8, "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(w[8:]), "command": tag, "authentication_requested": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false}
	f["protocol_version"], f["protocol_version_explicit"] = 1, false
	if request {
		f["kind"] = "AARQ"
	}
	seen := map[byte]bool{}
	fields := acseCursor{w: body}
	result, diagnostic := byte(0), byte(0)
	diagnosticSource := byte(0)
	var initiate []byte
	for fields.at < len(body) {
		if len(seen) >= min(limit, 16) {
			return nil, wrapperError(ErrResourceExceeded, "ACSE component count budget")
		}
		t, v, e := fields.tlv()
		if e != nil {
			return nil, e
		}
		if seen[t] {
			return nil, wrapperError(ErrMalformedMessage, "duplicate ACSE component")
		}
		seen[t] = true
		switch t {
		case 0xa1:
			oid, e := acseOnlyTLV(v, 6)
			if e != nil {
				return nil, e
			}
			if !bytes.Equal(oid, []byte{0x60, 0x85, 0x74, 5, 8, 1, 1}) {
				return nil, wrapperError(ErrUnsupportedFeature, "ACSE application context outside unciphered LN")
			}
			f["application_context_oid"] = "2.16.756.5.8.1.1"
			f["application_context_oid_hex"] = hex.EncodeToString(oid)
		case 0xa2:
			if request {
				return nil, wrapperError(ErrUnsupportedFeature, "AARQ called AP title not in no-authentication profile")
			}
			v, e = acseOnlyTLV(v, 2)
			if e != nil {
				return nil, e
			}
			if len(v) != 1 {
				return nil, wrapperError(ErrMalformedMessage, "AARE result INTEGER size")
			}
			result = v[0]
			if result > 2 {
				return nil, wrapperError(ErrUnsupportedFeature, "AARE result outside pinned enum")
			}
			f["association_result"] = result
			f["association_accepted"] = result == 0
		case 0xa3:
			if request {
				return nil, wrapperError(ErrUnsupportedFeature, "AARQ AE qualifier unsupported")
			}
			dc := acseCursor{w: v}
			source, inner, e := dc.tlv()
			if e != nil {
				return nil, e
			}
			if (source != 0xa1 && source != 0xa2) || dc.at != len(v) {
				return nil, wrapperError(ErrMalformedMessage, "AARE diagnostic choice")
			}
			d, e := acseOnlyTLV(inner, 2)
			if e != nil {
				return nil, e
			}
			if len(d) != 1 {
				return nil, wrapperError(ErrMalformedMessage, "AARE diagnostic INTEGER size")
			}
			diagnostic = d[0]
			diagnosticSource = source
			if source == 0xa1 && diagnostic > 14 || source == 0xa2 && diagnostic > 2 {
				return nil, wrapperError(ErrUnsupportedFeature, "AARE diagnostic outside pinned enum")
			}
			f["diagnostic_source"], f["diagnostic"] = source, diagnostic
		case 0xbe:
			initiate, e = acseOnlyTLV(v, 4)
			if e != nil {
				return nil, e
			}
			f["user_information_hex"] = hex.EncodeToString(initiate)
		case 0x80:
			if !bytes.Equal(v, []byte{7, 128}) {
				return nil, wrapperError(ErrUnsupportedFeature, "ACSE protocol version outside version1")
			}
			f["protocol_version"], f["protocol_version_explicit"] = 1, true
		default:
			return nil, wrapperError(ErrUnsupportedFeature, "ACSE authentication/title/unselected component")
		}
	}
	if !seen[0xa1] || !request && (!seen[0xa2] || !seen[0xa3]) {
		return nil, wrapperError(ErrMalformedMessage, "ACSE mandatory component missing")
	}
	if !request && result == 0 && diagnostic != 0 {
		return nil, wrapperError(ErrMalformedMessage, "accepted AARE has nonzero diagnostic")
	}
	m := &wrapperMessage{fields: f, source: binary.BigEndian.Uint16(w[2:4]), destination: binary.BigEndian.Uint16(w[4:6]), request: request, acse: true, acseAccepted: !request && result == 0 && diagnostic == 0 && diagnosticSource != 0}
	m.acseInfo = len(initiate) > 0
	f["initiate_information_present"] = m.acseInfo
	if len(initiate) == 0 {
		if seen[0xbe] {
			return nil, wrapperError(ErrMalformedMessage, "empty ACSE user information")
		}
		// User-information is optional in the ACSE envelope. Its absence
		// cannot certify xDLMS negotiation and must not be called bad BER.
		return m, nil
	}
	at := 0
	take := func(n int) ([]byte, error) {
		if n > len(initiate)-at {
			return nil, wrapperError(ErrMalformedMessage, "xDLMS initiate truncated")
		}
		v := initiate[at : at+n]
		at += n
		return v, nil
	}
	b, e := take(1)
	if e != nil {
		return nil, e
	}
	want := byte(8)
	if request {
		want = 1
	}
	if b[0] != want {
		return nil, wrapperError(ErrUnsupportedFeature, "ACSE ciphered/unselected initiate command")
	}
	if request {
		key, e := take(1)
		if e != nil {
			return nil, e
		}
		if key[0] != 0 {
			return nil, wrapperError(ErrUnsupportedFeature, "ACSE dedicated key unsupported")
		}
		allowed, e := take(1)
		if e != nil {
			return nil, e
		}
		if allowed[0] > 1 {
			return nil, wrapperError(ErrMalformedMessage, "initiate response-allowed usage flag")
		}
		f["response_allowed_explicit"] = allowed[0] == 1
		f["response_allowed"] = true
		if allowed[0] == 1 {
			v, e := take(1)
			if e != nil {
				return nil, e
			}
			if v[0] > 1 {
				return nil, wrapperError(ErrUnsupportedFeature, "noncanonical initiate BOOLEAN")
			}
			f["response_allowed"] = v[0] == 1
		}
	}
	qos, e := take(1)
	if e != nil {
		return nil, e
	}
	if qos[0] > 1 {
		return nil, wrapperError(ErrMalformedMessage, "initiate QoS usage flag")
	}
	f["quality_of_service_explicit"] = qos[0] == 1
	if qos[0] == 1 {
		v, e := take(1)
		if e != nil {
			return nil, e
		}
		f["quality_of_service"] = int8(v[0])
	}
	ver, e := take(1)
	if e != nil {
		return nil, e
	}
	if ver[0] != 6 {
		return nil, wrapperError(ErrUnsupportedFeature, "xDLMS version6 only")
	}
	f["dlms_version"] = ver[0]
	conf, e := take(7)
	if e != nil {
		return nil, e
	}
	if !bytes.Equal(conf[:4], []byte{0x5f, 0x1f, 4, 0}) {
		return nil, wrapperError(ErrMalformedMessage, "initiate conformance bit-string encoding")
	}
	// ASN.1 bit positions run MSB-first in each octet. The public mask uses
	// numbered conformance bits, matching the fixed reference enum/codec.
	m.acseConformance = uint32(bits.Reverse8(conf[4])) | uint32(bits.Reverse8(conf[5]))<<8 | uint32(bits.Reverse8(conf[6]))<<16
	f["conformance_bits"] = m.acseConformance
	f["conformance_wire_hex"] = hex.EncodeToString(conf[4:])
	size, e := take(2)
	if e != nil {
		return nil, e
	}
	m.acseMaxPDU = binary.BigEndian.Uint16(size)
	if m.acseMaxPDU < 64 {
		return nil, wrapperError(ErrUnsupportedFeature, "initiate PDU limit below fixed reference minimum64")
	}
	f["max_pdu_size"] = m.acseMaxPDU
	if !request {
		vaa, e := take(2)
		if e != nil {
			return nil, e
		}
		if binary.BigEndian.Uint16(vaa) != 7 {
			return nil, wrapperError(ErrUnsupportedFeature, "LN VAA name7 only")
		}
		f["vaa_name"] = uint16(7)
	}
	if at != len(initiate) {
		return nil, wrapperError(ErrMalformedMessage, "initiate trailing bytes")
	}
	return m, nil
}
func (f *binFlow) consumeWrapperACSE(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	if m.request {
		key := wrapperAssociationKey{dir, m.source, m.destination}
		if p := s.associations[key]; p != nil {
			p.pending = false
			p.ambiguous = true
			return "ambiguous-association-generation", nil
		}
		if len(s.associations) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
			return "", wrapperError(ErrResourceExceeded, "ACSE retained context budget")
		}
		if s.associations == nil {
			s.associations = make(map[wrapperAssociationKey]*wrapperAssociation)
		}
		if !m.acseInfo {
			s.associations[key] = &wrapperAssociation{ambiguous: true}
			return "unverified-association-initiate-context", nil
		}
		s.associations[key] = &wrapperAssociation{id: e.ID, pending: true, conformance: m.acseConformance, maxPDU: m.acseMaxPDU}
		e.TransactionID = e.ID
		return "observed-association-request", nil
	}
	p := s.associations[wrapperAssociationKey{1 - dir, m.destination, m.source}]
	if p == nil {
		return "unmatched-association-response", nil
	}
	if p.ambiguous || !p.pending {
		p.pending = false
		p.ambiguous = true
		return "ambiguous-association-generation", nil
	}
	if m.acseAccepted && !m.acseInfo {
		p.pending = false
		p.ambiguous = true
		return "unverified-association-initiate-context", nil
	}
	m.fields["proposed_conformance_bits"] = p.conformance
	m.fields["proposed_max_pdu_size"] = p.maxPDU
	if m.acseAccepted && m.acseConformance & ^p.conformance != 0 {
		m.fields["unproposed_conformance_bits"] = m.acseConformance & ^p.conformance
		p.pending = false
		p.ambiguous = true
		return "unverified-conformance-selection", nil
	}
	p.pending = false
	e.TransactionID, e.ResponseTo = p.id, p.id
	m.fields["association_observed"] = true
	m.fields["association_request_event"] = p.id
	m.fields["proposed_conformance_bits"] = p.conformance
	m.fields["proposed_max_pdu_size"] = p.maxPDU
	return "observed-association-response", nil
}
