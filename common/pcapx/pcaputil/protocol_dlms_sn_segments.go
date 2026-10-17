package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// The SN service has no invoke token. These groups prove only that bounded
// information bytes were observed consecutively on one literal logical link.
// They never acquire a service owner or advance LN sequence/AA history.
func dlmsSNSegmentHeader(info []byte) bool {
	if len(info) < 4 || info[0] != 0xe6 || info[2] != 0 || info[1] != 0xe6 && info[1] != 0xe7 {
		return false
	}
	return dlmsSNReadAPDU(info[3:]) || dlmsSNWriteAPDU(info[3:])
}
func dlmsSNInformationPrefix(w []byte) []byte {
	// Classification only; the public event still requires complete CRC validation.
	if len(w) < 12 || w[0] != 0x7e {
		return nil
	}
	at, end := 3, len(w)-3
	if _, _, err := dlmsAddress(w[:end], &at); err != nil {
		return nil
	}
	if _, _, err := dlmsAddress(w[:end], &at); err != nil || end-at < 3 || w[at]&1 != 0 {
		return nil
	}
	return w[at+3 : end]
}
func (f *binFlow) consumeDLMSSNSegments(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	fail := func(kind ProtocolErrorKind, why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(kind, why)
	}
	checkCount := func(info []byte) error {
		if len(info) < 5 {
			return nil
		}
		if info[4] > 0x80 && info[4]&127 <= 4 && len(info) < 5+int(info[4]&127) {
			return nil
		}
		c := wrapperListCursor{wire: info[3:], at: 1}
		n, _, err := c.count()
		if err != nil {
			var pe *ProtocolError
			if errors.As(err, &pe) {
				return dlmsError(pe.Kind, "SN segment count is outside selected forms")
			}
			return dlmsError(ErrMalformedMessage, "SN segment count invalid")
		}
		if n == 0 {
			return dlmsError(ErrUnsupportedFeature, "SN segments select nonempty lists only")
		}
		if n > uint64(min(f.a.budget.MaxCollectionElements, wrapperSNReadItems)) {
			return dlmsError(ErrResourceExceeded, "SN segment list exceeds selected collection budget")
		}
		return nil
	}
	t := s.fragments
	if t == nil {
		info := m.information
		if !m.segmented || !dlmsSNSegmentHeader(info) {
			return fail(ErrUnsupportedFeature, "first SN segment needs selected LLC/service header", true)
		}
		command := info[3]
		request := command == 5 || command == 6
		if request && info[1] != 0xe6 || !request && info[1] != 0xe7 {
			return fail(ErrMalformedMessage, "SN segment LLC direction differs from command", true)
		}
		// A complete LLC/command is observed. A nonfinal determinant may cross
		// CRC-valid information frames; no list/owner fields are decoded yet.
		if err := checkCount(info); err != nil {
			s.invalidate()
			return nil, 0, err
		}
		if s.transfer != nil || s.setTransfer != nil || s.actionTransfer != nil {
			return fail(ErrContextRequired, "SN group conflicts with retained service transfer", false)
		}
		if p := s.pending; p != nil {
			if !request || p.dir != dir || p.source != m.source || p.destination != m.destination {
				return fail(ErrContextRequired, "SN group differs from retained request link", false)
			}
			s.invalidate() // A distinct same-link request retires the old owner permanently.
		}
		t = &dlmsFragments{snCommand: command, operation: "SN", request: request, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if m.kind != "I" {
			if m.kind == "S" && dir != t.dir && m.source == t.destination && m.destination == t.source {
				advance := (m.nr - ((t.last + 1) & 7)) & 7
				if advance > 0 && advance <= 4 {
					return fail(ErrContextRequired, "supervisory acknowledgement advances beyond SN fragments", true)
				}
				m.fields["Association"] = "unassociated-link-observation"
				return m.fields, 0, nil
			}
			if m.kind == "S" {
				return fail(ErrContextRequired, "supervisory frame differs from SN group", false)
			}
			s.invalidate()
			if m.kind == "DM" {
				m.fields["Association"] = "unassociated-link-observation"
				return m.fields, 0, nil
			}
			return fail(ErrContextRequired, "link control interrupts SN information group", false)
		}
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return fail(ErrContextRequired, "SN continuation direction/address/acknowledgement differ", false)
		}
		if bytes.Equal(t.wire, wire) {
			direction := "response"
			if t.request {
				direction = "request"
			}
			m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " " + map[bool]string{true: "Request", false: "Response"}[t.request]
			m.fields["HDLC Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-sn-" + direction + "-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 {
			return fail(ErrContextRequired, "missing/conflicting/repeated SN fragment sequence", true)
		}
	}
	if len(m.information) == 0 {
		return fail(ErrMalformedMessage, "empty SN continuation information", true)
	}
	if t.count >= min(wrapperBlockCount, f.a.budget.MaxCollectionElements) || len(m.information) > min(wrapperBlockBytes+3, f.a.budget.MaxCollectionElements)-len(t.info) {
		return fail(ErrResourceExceeded, "SN information bytes/frame count exceed selected/caller pool", true)
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	if m.segmented && t.count > 0 {
		if err := checkCount(joined); err != nil {
			s.invalidate()
			return nil, 0, err
		}
	}
	count := t.count + 1
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count = joined, bytes.Clone(wire), m.ns, count
		s.fragments = t
		direction := "response"
		title := "Response"
		if t.request {
			direction, title = "request", "Request"
		}
		m.fields["Frame Kind"] = "Segmented " + t.serviceName() + " " + title
		m.fields["HDLC Segmentation"] = meta
		m.fields["Association"] = "unverified-sn-" + direction + "-segment"
		return m.fields, 0, nil
	}
	// Decode the joined APDU against the final captured carrier; never fabricate
	// a frame/CRC/Raw or convert a completed SN group into a service transaction.
	// Service blocks carried inside an information-fragment group require a
	// separate layered contract. Do not accidentally widen this complete-frame MVP.
	if len(joined) >= 3 && (dlmsSNServiceBlockAPDU(joined[3:]) || dlmsSNUploadAPDUProfile(joined[3:]) != "") {
		return fail(ErrUnsupportedFeature, "SN service-block inside information group is not selected", true)
	}
	decoded, err := decodeDLMSInformation(m, joined, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	s.fragments = nil
	fields, reply, err := f.consumeDecodedDLMS(dir, wire, id, decoded)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	meta["assembled_information_hex"] = hex.EncodeToString(joined)
	fields["HDLC Segmentation"] = meta
	return fields, reply, nil
}
