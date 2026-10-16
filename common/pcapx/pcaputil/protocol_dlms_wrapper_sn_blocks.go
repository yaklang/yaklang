package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

// These are observer bounds, not ASN.1 limits. SN has no invoke identifier:
// even an exact alternating block/echo sequence is only an observed wire group,
// never a verified request, object, authenticated association or meter result.
const wrapperSNBlockBytes = 1024
const wrapperSNBlockCount = 64

// Match the same definite determinant widths1..4 as the existing SN cursor.
// This bounded fixed-header scan performs no allocation and no Data decode.
func wrapperSNDefinitePrefix(w []byte, at int) (uint64, int, bool) {
	if at >= len(w) {
		return 0, at, false
	}
	first := w[at]
	at++
	if first < 128 {
		return uint64(first), at, true
	}
	width := int(first & 127)
	if width < 1 || width > 4 || len(w)-at < width {
		return 0, at, false
	}
	var n uint64
	for _, b := range w[at : at+width] {
		n = n<<8 | uint64(b)
	}
	return n, at + width, true
}

func wrapperIsSNServiceBlock(w []byte) bool {
	if len(w) < 10 || w[0] != 0 || w[1] != 1 || w[8] != 5 && w[8] != 12 {
		return false
	}
	_, at, ok := wrapperSNDefinitePrefix(w, 9)
	return ok && at < len(w) && (w[8] == 5 && w[at] == 5 || w[8] == 12 && w[at] == 2)
}

// Admission refusal cannot allocate or decode a Data graph. Only quarantine a
// new UDP carrier for a complete selected single-block fixed header/length.
func wrapperSNBlockRefusalHint(w []byte) bool {
	if !wrapperIsSNServiceBlock(w) || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 {
		return false
	}
	n, at, ok := wrapperSNDefinitePrefix(w, 9)
	if !ok || n != 1 {
		return false
	}
	if w[8] == 5 {
		return len(w) == at+3
	}
	if len(w) < at+5 || w[at+1] > 1 {
		return false
	}
	n, end, ok := wrapperSNDefinitePrefix(w, at+4)
	return ok && n == uint64(len(w)-end)
}

// A separate Wrapper profile keeps HDLC service-block and request-upload
// semantics unchanged. Choice3 is an upload acknowledgement, not Data.
func decodeWrapperSNServiceBlock(w []byte, limit int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, wrapperError(ErrUnsupportedFeature, "SN service-block selected single-result count1 only")
	}
	if limit < 1 {
		return nil, wrapperError(ErrResourceExceeded, "SN service-block collection budget")
	}
	start := c.at
	ch, err := c.take(1)
	if err != nil {
		return nil, err
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{
		"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst,
		"payload_length": len(p), "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(p),
		"command": p[0], "reference_mode": "short-name", "association_observed": false,
		"authentication_verified": false, "object_model_verified": false, "operation_success_verified": false,
		"list_count": n, "list_count_encoding_hex": enc,
	}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], request: p[0] == 5, snRead: true, snServiceBlock: true, count: 1}
	row := map[string]any{"index": 0, "apdu_offset": start}
	if m.request {
		if ch[0] != 5 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN block request requires block-number-access choice5")
		}
		row["access_choice_raw"], row["access_choice"] = ch[0], "block-number-access"
		f["kind"] = "ReadRequest"
	} else {
		row["result_choice_raw"] = ch[0]
		f["kind"] = "ReadResponse"
		switch ch[0] {
		case 2:
			last, err := c.take(1)
			if err != nil {
				return nil, err
			}
			if last[0] > 1 {
				return nil, wrapperError(ErrUnsupportedFeature, "SN last-block selected canonical0/1 only")
			}
			m.lastBlock = last[0] == 1
			row["result_choice"], row["last_block_raw"], row["last_block"] = "data-block-result", last[0], m.lastBlock
		default:
			return nil, wrapperError(ErrUnsupportedFeature, "SN selected data-block-result choice2 only")
		}
	}
	bn, err := c.take(2)
	if err != nil {
		return nil, err
	}
	m.blockNumber = uint32(binary.BigEndian.Uint16(bn))
	row["block_number"] = uint16(m.blockNumber)
	if !m.request {
		n, enc, err := c.count()
		if err != nil {
			return nil, err
		}
		if n > uint64(wrapperSNBlockBytes) {
			return nil, wrapperError(ErrResourceExceeded, "SN service-block raw data exceeds selected byte budget")
		}
		m.blockData, err = c.take(int(n))
		if err != nil {
			return nil, err
		}
		row["raw_data_length"], row["length_encoding_hex"], row["raw_data_hex"] = n, enc, hex.EncodeToString(m.blockData)
		f["transfer_complete"] = false
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "SN service-block has trailing bytes")
	}
	row["encoded_length"], row["raw_hex"] = c.at-start, hex.EncodeToString(p[start:c.at])
	if m.request {
		f["descriptors"] = []map[string]any{row}
	} else {
		f["results"] = []map[string]any{row}
	}
	return m, nil
}

type wrapperSNGroup struct {
	last                       uint16
	blocks                     int
	active, acked, completed   bool
	ambiguous                  bool
	data                       []byte
	lastHash, ackHash          [32]byte
	lastEvent, ackEvent, start uint64
}

func (p *wrapperSNGroup) retire() {
	p.data = nil
	p.active, p.acked, p.completed, p.ambiguous = false, false, false, true
}

func (s *binDLMSWrapper) unfinishedSNBlockKeys() []wrapperGBTKey {
	var out []wrapperGBTKey
	for k, p := range s.snGroups {
		if p.active {
			out = append(out, k)
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
		return a.destination < b.destination
	})
	return out
}

func (s *binDLMSWrapper) retireSNGroupForMessage(m *wrapperMessage, dir int) bool {
	if !m.request {
		return false
	}
	k := wrapperGBTKey{1 - dir, m.destination, m.source}
	// A new complete SN request is an explicit observation boundary, but is
	// not itself retained or treated as a service owner. Removing a group here
	// does not revive a conversation retired by a resource/idle failure.
	_, existed := s.snGroups[k]
	delete(s.snGroups, k)
	return existed
}

func (f *binFlow) consumeWrapperSNServiceBlock(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent, w []byte) (string, error) {
	return f.consumeSNServiceBlockReserved(s, m, dir, e, w, 0)
}

// The floor keeps other HDLC logical links and their LN/fragment state charged.
// Wrapper uses zero; caller-specific addresses and link checks remain outside.
func (f *binFlow) consumeSNServiceBlockReserved(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent, w []byte, floor int64) (string, error) {
	if s.blocked {
		return "unverified-sn-block-after-conversation-invalidation", nil
	}
	key := wrapperGBTKey{dir, m.source, m.destination}
	if m.request {
		key = wrapperGBTKey{1 - dir, m.destination, m.source}
	}
	p := s.snGroups[key]
	h := sha256.Sum256(w)
	if m.request {
		if p == nil {
			if len(s.snGroups) != 0 {
				return "unmatched-sn-block-context", nil
			}
			return "orphan-sn-block-number-access", nil
		}
		if p.ambiguous {
			return "retired-sn-wire-group", nil
		}
		if p.ackEvent != 0 && p.ackHash == h {
			m.fields["duplicate_of_event"] = p.ackEvent
			return "duplicate-wire", nil
		}
		if !p.active {
			return "retired-sn-completed-generation-uncertain", nil
		}
		if uint16(m.blockNumber) != p.last {
			p.retire()
			return "retired-sn-block-phase-or-number-conflict", nil
		}
		if p.acked {
			if p.ackHash == h {
				m.fields["duplicate_of_event"] = p.ackEvent
				return "duplicate-wire", nil
			}
			p.retire()
			return "retired-sn-block-phase-or-number-conflict", nil
		}
		p.acked, p.ackHash, p.ackEvent = true, h, e.ID
		m.fields["acknowledged_block_event"] = p.lastEvent
		return "unverified-sn-block-number-access", nil
	}
	if p != nil {
		if p.ambiguous {
			return "retired-sn-wire-group", nil
		}
		if uint16(m.blockNumber) == p.last {
			if p.lastHash == h {
				m.fields["duplicate_of_event"] = p.lastEvent
				return "duplicate-wire", nil
			}
			wasComplete := p.completed
			p.retire()
			if wasComplete {
				return "retired-sn-completed-generation-uncertain", nil
			}
			return "retired-sn-block-phase-or-number-conflict", nil
		}
		if p.completed {
			p.retire()
			return "retired-sn-completed-generation-uncertain", nil
		}
		if !p.active || !p.acked || p.last == 65535 || m.blockNumber != uint32(p.last)+1 {
			p.retire()
			return "retired-sn-block-phase-or-number-conflict", nil
		}
	} else if m.blockNumber != 1 {
		if len(s.snGroups) != 0 {
			return "unmatched-sn-block-context", nil
		}
		return "orphan-sn-data-block", nil
	}
	total, blocks := len(m.blockData), 1
	if p != nil {
		total, blocks = total+len(p.data), p.blocks+1
	}
	if total > wrapperSNBlockBytes || blocks > min(sessionCollectionLimit(f.a.budget.MaxCollectionElements), wrapperSNBlockCount) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "SN service-block group exceeds byte/block budget")
	}
	if p == nil && len(s.snGroups) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "SN service-block endpoint groups exceed collection budget")
	}
	// Reserve retained storage plus the complete eventual owned Data graph
	// before copying a byte. The old graph remains charged during replacement.
	if err := f.reserveSession(floor + s.storage() + wrapperProjection(w, len(w)) + 512 + 128*int64(total)); err != nil {
		s.invalidate()
		return "", err
	}
	joined := make([]byte, total)
	if p != nil {
		copy(joined, p.data)
		copy(joined[len(p.data):], m.blockData)
	} else {
		copy(joined, m.blockData)
	}
	var data map[string]any
	if m.lastBlock {
		c := wrapperListCursor{wire: joined, maxDepth: f.a.budget.MaxRecursionDepth}
		var err error
		data, err = c.data(f.a.budget.MaxCollectionElements, 0)
		if err == nil && c.at != len(joined) {
			err = wrapperError(ErrMalformedMessage, "SN service-block single Data has trailing bytes")
		}
		if err != nil {
			s.invalidate()
			return "", err
		}
	}
	if p == nil {
		if s.snGroups == nil {
			s.snGroups = make(map[wrapperGBTKey]*wrapperSNGroup)
		}
		p = &wrapperSNGroup{start: e.ID}
		s.snGroups[key] = p
	}
	p.last, p.blocks, p.lastHash, p.lastEvent = uint16(m.blockNumber), blocks, h, e.ID
	p.active, p.acked, p.completed = !m.lastBlock, false, m.lastBlock
	m.fields["transfer_start_event"], m.fields["observed_block_count"] = p.start, blocks
	m.fields["assembled_data_length"] = total
	if m.lastBlock {
		m.fields["transfer_complete"], m.fields["assembled_data_hex"], m.fields["assembled_data"] = true, hex.EncodeToString(joined), data
		p.data = nil
		return "unverified-sn-wire-group-complete", nil
	}
	p.data = joined
	return "unverified-sn-data-block", nil
}
