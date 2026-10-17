package pcaputil

import (
	"crypto/sha256"
	"errors"
	"sort"
	"strings"
)

// A service group is separate from HDLC information fragmentation and LN invoke
// ownership. Keep full addresses: truncating these to Wrapper's UInt16 ports
// would merge unrelated logical links carried by the same physical endpoints.
type dlmsSNBlockKey struct {
	dir                 int
	source, destination uint32
	upload, write       bool
}
type dlmsSNBlockLink struct {
	wire                           *binDLMSWrapper
	responseNS, responseNR, echoNS byte
	interrupted                    bool
}

func dlmsSNServiceBlockAPDU(p []byte) bool {
	if len(p) < 2 || p[0] != 5 && p[0] != 12 {
		return false
	}
	_, at, ok := wrapperSNDefinitePrefix(p, 1)
	return ok && at < len(p) && (p[0] == 5 && p[at] == 5 || p[0] == 12 && p[at] == 2)
}
func dlmsSNBlockWire(w []byte) bool {
	if len(w) < 3 || w[1]&8 != 0 {
		return false
	}
	info := dlmsSNInformationPrefix(w)
	return len(info) >= 4 && info[0] == 0xe6 && info[2] == 0 && (info[1] == 0xe6 || info[1] == 0xe7) && dlmsSNServiceBlockAPDU(info[3:])
}
func dlmsSNBlockMessageKey(m *dlmsMessage, dir int) dlmsSNBlockKey {
	upload, write := false, false
	if m.snBlock != nil {
		upload, write = m.snBlock.snUpload, m.snBlock.snUpload && m.snBlock.snWrite
	}
	key := dlmsSNBlockKey{dir, m.source, m.destination, upload, write}
	echo := m.request != upload
	if echo {
		key.dir, key.source, key.destination = 1-dir, m.destination, m.source
	}
	return key
}
func (s *binDLMS) retireOrdinarySNBlock(m *dlmsMessage, dir int) {
	if !m.request {
		return
	}
	for key := range s.snBlocks {
		down := !key.upload && key.dir == 1-dir && key.source == m.destination && key.destination == m.source
		up := key.upload && key.dir == dir && key.source == m.source && key.destination == m.destination
		if down || up {
			delete(s.snBlocks, key)
		}
	}
}
func dlmsSNBlockShadowKey(key dlmsSNBlockKey) wrapperSNKey {
	return wrapperSNKey{wrapperGBTKey{key.dir, 0, 0}, key.upload, key.write}
}
func dlmsSNBlockProfile(key dlmsSNBlockKey) string {
	if key.upload {
		if key.write {
			return "dlms-hdlc-sn-write-upload"
		}
		return "dlms-hdlc-sn-read-upload"
	}
	return "dlms-hdlc-sn-read-block"
}
func dlmsSNBlockCloseSummary(key dlmsSNBlockKey) string {
	if key.upload {
		if key.write {
			return "DLMS HDLC SN Write upload wire group ended before complete request body"
		}
		return "DLMS HDLC SN Read upload wire group ended before complete request body"
	}
	return "DLMS HDLC SN Read wire group ended before complete Data"
}
func (f *binFlow) consumeDLMSSNServiceBlock(dir int, w []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	return f.consumeDLMSSNServiceBlockContext(dir, w, id, m, -1, f.a.budget.MaxRecursionDepth, w)
}

// Complete containing APDUs use the same full UInt32 SN group and HDLC sequence
// contract as direct phases. A containing floor includes outer shadows/graphs.
func (f *binFlow) consumeDLMSSNServiceBlockContext(dir int, syntax []byte, id uint64, m *dlmsMessage, containingFloor int64, depth int, identity []byte) (map[string]any, uint64, error) {
	s := f.dlms
	key := dlmsSNBlockMessageKey(m, dir)
	s.operationProfile = dlmsSNBlockProfile(key)
	link := s.snBlocks[key]
	created := link == nil
	if created {
		link = &dlmsSNBlockLink{wire: &binDLMSWrapper{blocked: s.ambiguous}}
	}
	// The shadow has exactly one already-separated UInt32 logical link. Its zero
	// synthetic Wrapper ports never escape or identify the real HDLC context.
	wk := dlmsSNBlockShadowKey(key)
	echo := m.request != m.snBlock.snUpload
	// Wrong caller/address must not consume or interrupt the matching mode's
	// existing group. Each logical link owns its shadow, so classify an absent
	// link against the observed same-mode context before invoking that shadow.
	if created && key.upload && (echo || m.snBlock.blockNumber != 1) {
		for other := range s.snBlocks {
			if other.upload == key.upload && other.write == key.write {
				m.fields["Association"] = "unmatched-sn-block-context"
				return m.fields, 0, nil
			}
		}
	}
	group := link.wire.snGroups[wk]
	body := m.snBlock
	if link.interrupted {
		m.fields["Association"] = "retired-sn-hdlc-service-block-interleaved-link"
		return m.fields, 0, nil
	}
	if group != nil && !group.ambiguous {
		hash := sha256.Sum256(identity)
		exact := !echo && group.lastHash == hash || echo && group.ackEvent != 0 && group.ackHash == hash
		// Without an observed echo, NR cannot be judged against an echo NS.
		// Let the shared phase guard retire that missing-context case first.
		if !exact && group.active && (echo || group.acked) {
			good := m.ns == (link.responseNS+1)&7 && m.nr == (link.echoNS+1)&7
			if echo {
				good = m.ns == link.responseNR && m.nr == (link.responseNS+1)&7
			}
			if !good {
				group.retire()
				m.fields["Association"] = "retired-sn-hdlc-service-block-sequence-conflict"
				return m.fields, 0, nil
			}
		}
	}
	if created && !echo && body.blockNumber == 1 && !s.ambiguous && len(s.snBlocks) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
		s.invalidate()
		return nil, 0, dlmsError(ErrResourceExceeded, "SN service-block logical link budget")
	}
	// Other links, LN pending bytes, ACSE and information fragments stay charged.
	floor := s.storage()
	if containingFloor >= 0 {
		floor = containingFloor
	}
	floor += 128
	if !created {
		floor -= 128 + link.wire.storage()
	}
	event := &ProtocolEvent{ID: id}
	association, err := f.consumeSNServiceBlockContext(link.wire, body, dir, event, syntax, floor, depth, identity)
	if err != nil {
		s.invalidate()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, 0, dlmsError(pe.Kind, "SN service-block: "+strings.TrimPrefix(pe.Message, "DLMS Wrapper "))
		}
		return nil, 0, err
	}
	if len(link.wire.snGroups) > 0 {
		if s.snBlocks == nil {
			s.snBlocks = make(map[dlmsSNBlockKey]*dlmsSNBlockLink)
		}
		s.snBlocks[key] = link
	}
	switch association {
	case "unverified-sn-data-block", "unverified-sn-wire-group-complete", "unverified-sn-upload-block", "unverified-sn-upload-wire-group-complete", "unverified-sn-write-upload-block", "unverified-sn-write-upload-wire-group-complete":
		link.responseNS, link.responseNR = m.ns, m.nr
	case "unverified-sn-block-number-access", "unverified-sn-upload-echo", "unverified-sn-write-upload-echo":
		link.echoNS = m.ns
	}
	m.fields["Association"] = association
	return m.fields, 0, nil
}

func dlmsSNBlockRefusalHint(w []byte) bool {
	if !dlmsSNBlockWire(w) {
		return false
	}
	info := dlmsSNInformationPrefix(w)
	p := info[3:]
	n, at, ok := wrapperSNDefinitePrefix(p, 1)
	if !ok || n != 1 {
		return false
	}
	if p[0] == 5 {
		return len(p) == at+3
	}
	if len(p) < at+5 || p[at+1] > 1 {
		return false
	}
	n, end, ok := wrapperSNDefinitePrefix(p, at+4)
	return ok && n == uint64(len(p)-end)
}
func (s *binDLMS) hasActiveSNBlocks() bool {
	for _, l := range s.snBlocks {
		if l.wire.outstanding() > 0 {
			return true
		}
	}
	return false
}
func (s *binDLMS) unfinishedSNBlockKeys() []dlmsSNBlockKey {
	var out []dlmsSNBlockKey
	for key, l := range s.snBlocks {
		if l.wire.outstanding() > 0 {
			out = append(out, key)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.dir != b.dir {
			return a.dir < b.dir
		}
		if a.source != b.source {
			return a.source < b.source
		}
		if a.destination != b.destination {
			return a.destination < b.destination
		}
		if a.upload != b.upload {
			return !a.upload && b.upload
		}
		return !a.write && b.write
	})
	return out
}
func (s *binDLMS) snBlockCloseFields(key dlmsSNBlockKey) map[string]any {
	p := s.snBlocks[key].wire.snGroups[dlmsSNBlockShadowKey(key)]
	fields := map[string]any{"Buffered SN Groups": 1, "source_address": key.source, "destination_address": key.destination, "observed_block_count": p.blocks, "block_number": p.last}
	length := "assembled_data_length"
	if key.upload {
		length = "assembled_body_length"
	}
	fields[length] = len(p.data)
	return fields
}

// This complete-frame MVP requires an uninterrupted observed service-block /
// echo alternation on its logical link. Other observed link frames may expose
// progression not represented by the selected group (including a whole modulo8
// cycle); do not assemble across them or reinterpret old LN sequence history.
// Full supervisory/window and interleaved service support remain pending.
func (s *binDLMS) observeSNBlockInterruption(m *dlmsMessage, dir int) {
	for key, l := range s.snBlocks {
		if m.snBlock != nil && m.snBlock.snUpload == key.upload && (m.snBlock.snUpload && m.snBlock.snWrite) == key.write {
			continue
		}
		p := l.wire.snGroups[dlmsSNBlockShadowKey(key)]
		if p == nil || !p.active {
			continue
		}
		matched := dir == key.dir && m.source == key.source && m.destination == key.destination || dir != key.dir && m.source == key.destination && m.destination == key.source
		if matched {
			p.retire()
			l.interrupted = true
		}
	}
}
