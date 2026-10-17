package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// The digest binds every actual distinct carrier in order, not just the final
// opaque continuation. It never replaces the public final Raw/header/event ID.
func dlmsSNInformationDigest(previous [32]byte, wire []byte) [32]byte {
	h := sha256.Sum256(wire)
	var pair [64]byte
	copy(pair[:32], previous[:])
	copy(pair[32:], h[:])
	return sha256.Sum256(pair[:])
}

// One information-fragment group contributes one E0 APDU. The existing inner
// wire group remains separate and charged; information completion owns neither
// an SN business operation nor an LN sequence/association epoch.
func (f *binFlow) consumeDLMSSNGBTInformation(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	s.operationProfile = "dlms-hdlc-sn-gbt-information-segmented"
	fail := func(kind ProtocolErrorKind, why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(kind, why)
	}
	t := s.fragments
	if t == nil {
		if !m.segmented || !dlmsSNGBTInformation(m.information) {
			return fail(ErrUnsupportedFeature, "first SN GBT information segment needs LLC/E0 header", true)
		}
		if s.hasLNContext() {
			return fail(ErrContextRequired, "SN GBT information group cannot consume an observed LN owner", false)
		}
		info := m.information
		// The selected first fragment must contain the complete outer header/count.
		// The declared data can cross subsequent CRC-validated information frames.
		if len(info) < 10 {
			return fail(ErrUnsupportedFeature, "first SN GBT information segment needs complete E0 header/count", true)
		}
		if info[9] > 0x80 && info[9]&127 <= 4 && len(info) < 10+int(info[9]&127) {
			return fail(ErrUnsupportedFeature, "first SN GBT information segment needs complete E0 header/count", true)
		}
		c := wrapperListCursor{wire: info[3:], at: 6}
		n, _, err := c.count()
		if err != nil {
			var pe *ProtocolError
			if errors.As(err, &pe) {
				if pe.Kind == ErrNeedMore {
					return fail(ErrUnsupportedFeature, "first SN GBT information segment needs complete E0 header/count", true)
				}
				return fail(pe.Kind, "SN GBT information segment count invalid", true)
			}
			return fail(ErrMalformedMessage, "SN GBT information segment count invalid", true)
		}
		if n > uint64(min(wrapperBlockBytes+3, f.a.budget.MaxCollectionElements)) {
			return fail(ErrResourceExceeded, "SN GBT information declared data exceeds selected/caller pool", true)
		}
		t = &dlmsFragments{snGBT: true, operation: "SN GBT", request: info[1] == 0xe6, dir: dir, source: m.source, destination: m.destination, first: m.ns, nr: m.nr}
	} else {
		if m.kind != "I" {
			if m.kind == "S" && dir != t.dir && m.source == t.destination && m.destination == t.source {
				advance := (m.nr - ((t.last + 1) & 7)) & 7
				if advance > 0 && advance <= 4 {
					return fail(ErrContextRequired, "supervisory acknowledgement advances beyond SN GBT information fragments", true)
				}
				s.operationProfile = ""
				m.fields["Association"] = "unassociated-link-observation"
				return m.fields, 0, nil
			}
			if m.kind == "S" {
				return fail(ErrContextRequired, "supervisory frame differs from SN GBT information group", false)
			}
			s.invalidate()
			if m.kind == "DM" {
				s.operationProfile = ""
				m.fields["Association"] = "unassociated-link-observation"
				return m.fields, 0, nil
			}
			return fail(ErrContextRequired, "link control interrupts SN GBT information group", false)
		}
		if dir != t.dir || m.source != t.source || m.destination != t.destination || m.nr != t.nr {
			return fail(ErrContextRequired, "SN GBT information continuation direction/address/acknowledgement differ", false)
		}
		if bytes.Equal(t.wire, wire) {
			m.fields["Frame Kind"] = "Segmented SN GBT Information"
			m.fields["HDLC Segmentation"] = dlmsSegmentFields(m, t.count, false)
			m.fields["Association"] = "retransmitted-sn-gbt-information-segment"
			return m.fields, 0, nil
		}
		if m.ns != (t.last+1)&7 {
			return fail(ErrContextRequired, "missing/conflicting/repeated SN GBT information fragment sequence", true)
		}
	}
	if len(m.information) == 0 {
		return fail(ErrMalformedMessage, "empty SN GBT information continuation", true)
	}
	if t.count >= min(wrapperBlockCount, f.a.budget.MaxCollectionElements) || len(m.information) > min(wrapperBlockBytes+3, f.a.budget.MaxCollectionElements)-len(t.info) {
		return fail(ErrResourceExceeded, "SN GBT information bytes/frame count exceed selected/caller pool", true)
	}
	joined := make([]byte, len(t.info)+len(m.information))
	copy(joined, t.info)
	copy(joined[len(t.info):], m.information)
	digest := sha256.Sum256(wire)
	if t.count > 0 {
		digest = dlmsSNInformationDigest(t.snCarrierDigest, wire)
	}
	count := t.count + 1
	meta := dlmsSegmentFields(m, count, !m.segmented)
	if m.segmented {
		t.info, t.wire, t.last, t.count, t.snCarrierDigest = joined, bytes.Clone(wire), m.ns, count, digest
		s.fragments = t
		m.fields["Frame Kind"] = "Segmented SN GBT Information"
		m.fields["HDLC Segmentation"] = meta
		m.fields["Association"] = "unverified-sn-gbt-information-segment"
		return m.fields, 0, nil
	}
	decoded, err := decodeDLMSInformation(m, joined, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	decoded.information = joined
	decoded.snInformationSpan = true
	decoded.snInformationFirstNS = t.first
	decoded.snCarrierDigest = digest
	decoded.snCarrierLLC = joined[1]
	// The old retained fragment and the joined copy remain live during callback.
	// Charge both explicitly even though the fragment slot is cleared for routing.
	decoded.snInformationCharge = 32 + 2*int64(len(t.info)+len(t.wire)+len(joined))
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
