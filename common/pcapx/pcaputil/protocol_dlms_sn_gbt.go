package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"strings"
)

// This is a conservative complete-I-frame observation profile, not a negotiated
// HDLC window, SN association, or successful device operation.
type dlmsSNGBTKey struct {
	dir                 int
	source, destination uint32
}
type dlmsSNGBTLink struct {
	wire              *binDLMSWrapper
	lastDir           int
	ns, nr            byte
	observed, retired bool
}

func dlmsSNGBTInformation(info []byte) bool {
	return len(info) >= 4 && info[0] == 0xe6 && (info[1] == 0xe6 || info[1] == 0xe7) && info[2] == 0 && info[3] == 0xe0
}
func dlmsSNGBTWire(w []byte) bool {
	return len(w) >= 3 && w[1]&8 == 0 && dlmsSNGBTInformation(dlmsSNInformationPrefix(w))
}
func dlmsSNGBTEnvelope(p []byte) []byte {
	w := make([]byte, 8, len(p)+8)
	w[1] = 1
	binary.BigEndian.PutUint16(w[6:8], uint16(len(p)))
	return append(w, p...)
}
func decodeDLMSSNGBT(m *dlmsMessage, p []byte, llc byte, maxElements, depth int) (*dlmsMessage, error) {
	if depth < 4 {
		return nil, dlmsError(ErrResourceExceeded, "SN GBT projection exceeds recursion budget")
	}
	w := dlmsSNGBTEnvelope(p)
	body, err := decodeWrapperGBT(w, maxElements)
	if err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, dlmsError(pe.Kind, "SN GBT: "+strings.TrimPrefix(pe.Message, "DLMS Wrapper "))
		}
		return nil, err
	}
	for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex"} {
		delete(body.fields, k)
	}
	m.kind, m.request, m.snGBT = "SNGBT", llc == 0xe6, body
	m.fields["Frame Kind"], m.fields["GBT"] = "General Block Transfer", body.fields
	return m, nil
}
func (s *binDLMS) snGBTKey(m *dlmsMessage, dir int) dlmsSNGBTKey {
	key := dlmsSNGBTKey{dir, m.source, m.destination}
	if !m.request {
		key = dlmsSNGBTKey{1 - dir, m.destination, m.source}
	}
	return key
}
func (s *binDLMS) hasLNContext() bool {
	return s.pending != nil || s.fragments != nil || s.transfer != nil || s.setTransfer != nil || s.actionTransfer != nil
}
func (l *dlmsSNGBTLink) retire() {
	l.retired = true
	for _, p := range l.wire.gbt {
		p.ambiguous = true
		p.active = false
		p.data = nil
	}
}
func (s *binDLMS) observeSNGBTInterruption(m *dlmsMessage, dir int) {
	if m.snGBT != nil || dlmsSNGBTInformation(m.information) {
		return
	}
	for k, l := range s.snGBT {
		same := k.dir == dir && k.source == m.source && k.destination == m.destination || k.dir == 1-dir && k.source == m.destination && k.destination == m.source
		if same && l.wire.unfinishedGBTCounts() != [2]int{} {
			l.retire()
		}
	}
}
func (f *binFlow) consumeDLMSSNGBT(dir int, w []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	s.operationProfile = "dlms-hdlc-sn-gbt"
	if s.hasLNContext() {
		return nil, 0, dlmsError(ErrContextRequired, "SN GBT cannot consume an observed LN owner or information group")
	}
	if s.ambiguous {
		m.fields["Association"] = "ambiguous-gbt-hdlc-link"
		return m.fields, 0, nil
	}
	key := s.snGBTKey(m, dir)
	l := s.snGBT[key]
	created := l == nil
	if created {
		// A new logical caller/address cannot consume another link's prefix.
		if s.clientKnown && key.dir != s.clientDir {
			m.fields["Association"] = "unmatched-sn-gbt-context"
			return m.fields, 0, nil
		}
		if m.snGBT.gbtNumber != 1 {
			m.fields["Association"] = "unmatched-sn-gbt-context"
			return m.fields, 0, nil
		}
		if len(s.snGBT) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
			s.invalidate()
			return nil, 0, dlmsError(ErrResourceExceeded, "SN GBT logical link budget")
		}
		l = &dlmsSNGBTLink{wire: &binDLMSWrapper{}}
	}
	if l.retired {
		m.fields["Association"] = "ambiguous-gbt-hdlc-link"
		return m.fields, 0, nil
	}
	if m.snGBT.gbtControl&63 != 1 {
		l.retire()
		if !created {
			s.snGBT[key] = l
		}
		return nil, 0, dlmsError(ErrUnsupportedFeature, "SN GBT requires observed window-one alternation")
	}
	wk := wrapperGBTKey{dir, 0, 0}
	p := l.wire.gbt[wk]
	exact := false
	if p != nil {
		if b, ok := p.blocks[m.snGBT.gbtNumber]; ok {
			exact = b.hash == sha256.Sum256(w)
		}
	}
	if l.observed && !exact && (dir == l.lastDir || m.ns != l.nr || m.nr != (l.ns+1)&7) {
		l.retire()
		m.fields["Association"] = "retired-sn-hdlc-gbt-sequence-conflict"
		return m.fields, 0, nil
	}
	// A repeated block with a different full HDLC identity contradicts this
	// observed window-one history. Retire it before projecting any peer ACK;
	// the ACK cannot be attributed using an already contradictory frame.
	if p != nil && !exact {
		if _, exists := p.blocks[m.snGBT.gbtNumber]; exists {
			l.retire()
			m.fields["Association"] = "ambiguous-gbt-block-reuse"
			return m.fields, 0, nil
		}
	}
	// The full UInt32 key owns one private shadow; no truncated address appears
	// as a Wrapper port. Other logical links and ordinary state stay charged.
	floor := s.storage() + 128 + dlmsProjection(w)
	if !created {
		floor -= 128 + l.wire.storage()
	}
	event := &ProtocolEvent{ID: id}
	assoc, err := f.consumeWrapperGBTReserved(l.wire, m.snGBT, dir, event, dlmsSNGBTEnvelope(m.information[3:]), floor, w)
	if err != nil {
		s.invalidate()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, 0, dlmsError(pe.Kind, "SN GBT: "+strings.TrimPrefix(pe.Message, "DLMS Wrapper "))
		}
		return nil, 0, err
	}
	if s.snGBT == nil {
		s.snGBT = make(map[dlmsSNGBTKey]*dlmsSNGBTLink)
	}
	s.snGBT[key] = l
	// On this window-one HDLC profile, a contradictory observed frame
	// makes progress in either direction uncertain. Do not leave its peer
	// eligible to finish an independent core transfer after an ack0 frame.
	for _, p := range l.wire.gbt {
		if p.ambiguous {
			l.retire()
			break
		}
	}
	if !exact {
		l.lastDir, l.ns, l.nr, l.observed = dir, m.ns, m.nr, true
	}
	m.fields["Association"] = assoc
	return m.fields, event.ResponseTo, nil
}
func (s *binDLMS) hasActiveSNGBT() bool {
	for _, l := range s.snGBT {
		if l.wire.unfinishedGBTCounts() != [2]int{} {
			return true
		}
	}
	return false
}

type dlmsSNGBTClose struct {
	key dlmsSNGBTKey
	dir int
}

func (s *binDLMS) unfinishedSNGBTKeys() []dlmsSNGBTClose {
	var out []dlmsSNGBTClose
	for key, l := range s.snGBT {
		for dir, n := range l.wire.unfinishedGBTCounts() {
			if n > 0 {
				out = append(out, dlmsSNGBTClose{key, dir})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.dir != b.dir {
			return a.dir < b.dir
		}
		if a.key.source != b.key.source {
			return a.key.source < b.key.source
		}
		return a.key.destination < b.key.destination
	})
	return out
}
func (s *binDLMS) snGBTCloseFields(k dlmsSNGBTClose) map[string]any {
	p := s.snGBT[k.key].wire.gbt[wrapperGBTKey{k.dir, 0, 0}]
	src, dst := k.key.source, k.key.destination
	if k.dir != k.key.dir {
		src, dst = dst, src
	}
	return map[string]any{"Outstanding": 1, "source_address": src, "destination_address": dst, "observed_block_count": len(p.blocks), "block_number": p.last, "assembled_data_length": len(p.data)}
}

// Fixed header and canonical definite count only. This runs before admission
// of an oversized datagram: never allocate an envelope or inspect Data here.
func dlmsSNGBTRefusalHint(w []byte) bool {
	if !dlmsSNGBTWire(w) {
		return false
	}
	p := dlmsSNInformationPrefix(w)[3:]
	if len(p) < 7 || p[1]&63 == 0 || binary.BigEndian.Uint16(p[2:4]) == 0 {
		return false
	}
	n, at := uint64(p[6]), 7
	if p[6]&128 != 0 {
		width := int(p[6] & 127)
		if width != 1 && width != 2 && width != 4 || len(p)-at < width {
			return false
		}
		n = 0
		for _, b := range p[at : at+width] {
			n = n<<8 | uint64(b)
		}
		at += width
		if n < 128 || width > 1 && n < 256 || width > 2 && n < 65536 {
			return false
		}
	}
	return n == uint64(len(p)-at)
}
