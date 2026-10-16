package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Fixed Gurux6e409ae9 handleGbt/getLNPdu layout. Passive reconstruction does
// not establish negotiated conformance, association, authentication or a meter operation.
const wrapperGBTBytes = 65535
const wrapperGBTBlocks = 64

func wrapperIsGBT(w []byte) bool { return len(w) > 8 && w[0] == 0 && w[1] == 1 && w[8] == 0xe0 }

// A mutated unrelated APDU tag cannot retire untracked Wrapper eligibility.
// Only bounded fixed-header/count checks are allowed before resource admission.
func wrapperGBTRefusalHint(w []byte) bool {
	if len(w) < 15 || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 || w[9]&63 == 0 || binary.BigEndian.Uint16(w[10:12]) == 0 {
		return false
	}
	at := 15
	n := uint64(w[14])
	if w[14]&128 != 0 {
		width := int(w[14] & 127)
		if width != 1 && width != 2 && width != 4 || len(w) < at+width {
			return false
		}
		n = 0
		for _, b := range w[at : at+width] {
			n = n<<8 | uint64(b)
		}
		at += width
		if n < 128 || width > 1 && n < 256 || width > 2 && n < 65536 {
			return false
		}
	}
	return n == uint64(len(w)-at)
}
func decodeWrapperGBT(w []byte, limit int) (*wrapperMessage, error) {
	p := w[8:]
	if len(p) < 7 {
		return nil, wrapperError(ErrMalformedMessage, "GBT header/count truncated")
	}
	c := wrapperListCursor{wire: p, at: 6}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n > uint64(min(limit, wrapperGBTBytes)) {
		return nil, wrapperError(ErrResourceExceeded, "GBT block data exceeds byte budget")
	}
	data, err := c.take(int(n))
	if err != nil {
		return nil, err
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "GBT block has trailing data")
	}
	bn, ack := binary.BigEndian.Uint16(p[2:4]), binary.BigEndian.Uint16(p[4:6])
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{"kind": "GeneralBlockTransfer", "wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "payload_length": len(p), "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(p), "command": byte(0xe0), "block_control": p[1], "last_block": p[1]&128 != 0, "streaming": p[1]&64 != 0, "window_size": p[1] & 63, "window_semantics_verified": false, "block_number": bn, "block_number_ack": ack, "block_data_length": n, "length_encoding_hex": enc, "block_data_hex": hex.EncodeToString(data), "transfer_complete": false, "association_observed": false, "authentication_verified": false, "negotiated_conformance_verified": false, "operation_success_verified": false}
	return &wrapperMessage{fields: f, source: src, destination: dst, gbt: true, gbtControl: p[1], gbtNumber: bn, gbtAck: ack, blockData: data, lastBlock: p[1]&128 != 0}, nil
}

type wrapperGBTKey struct {
	dir                 int
	source, destination uint16
}
type wrapperGBTBlock struct {
	hash  [32]byte
	event uint64
}
type wrapperGBTTransfer struct {
	last                         uint16
	active, ambiguous, completed bool
	data                         []byte
	blocks                       map[uint16]wrapperGBTBlock
	start                        uint64
}

func (f *binFlow) consumeWrapperGBT(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent, w []byte) (string, error) {
	key := wrapperGBTKey{dir, m.source, m.destination}
	p := s.gbt[key]
	peer := s.gbt[wrapperGBTKey{1 - dir, m.destination, m.source}]
	// Reserve retained chunks, hash/event histories and the complete eventual
	// owned Data projections before appending or constructing an inner envelope.
	total := len(m.blockData)
	if p != nil {
		total += len(p.data)
	}
	if total > wrapperGBTBytes {
		return "", wrapperError(ErrResourceExceeded, "GBT transfer byte budget exceeded")
	}
	charge := s.storage() + wrapperProjection(w, len(w)) + 512 + 128*int64(total)
	if err := f.reserveSession(charge); err != nil {
		return "", err
	}
	if p == nil {
		if len(s.gbt) >= sessionCollectionLimit(f.a.budget.MaxCollectionElements) {
			return "", wrapperError(ErrResourceExceeded, "GBT direction/wPort contexts exceed collection budget")
		}
		if s.gbt == nil {
			s.gbt = make(map[wrapperGBTKey]*wrapperGBTTransfer)
		}
		p = &wrapperGBTTransfer{blocks: make(map[uint16]wrapperGBTBlock), start: e.ID}
		s.gbt[key] = p
	}
	if m.gbtControl&63 == 0 {
		p.ambiguous = true
		p.active = false
		p.data = nil
		return "unverified-gbt-zero-window", nil
	}
	ackOK := m.gbtAck == 0
	if peer != nil && !peer.ambiguous && m.gbtAck != 0 {
		if b, ok := peer.blocks[m.gbtAck]; ok {
			m.fields["acknowledged_block_event"] = b.event
			ackOK = true
		}
	}
	if !ackOK {
		p.ambiguous = true
		p.active = false
		p.data = nil
		return "unverified-gbt-acknowledgement", nil
	}
	h := sha256.Sum256(w)
	if old, ok := p.blocks[m.gbtNumber]; ok {
		if old.hash == h {
			m.fields["duplicate_of_event"] = old.event
			return "duplicate-gbt-block", nil
		}
		p.ambiguous = true
		p.active = false
		p.data = nil
		return "ambiguous-gbt-block-reuse", nil
	}
	if len(p.blocks) >= min(wrapperGBTBlocks, sessionCollectionLimit(f.a.budget.MaxCollectionElements)) {
		return "", wrapperError(ErrResourceExceeded, "GBT block history exceeds collection budget")
	}
	p.blocks[m.gbtNumber] = wrapperGBTBlock{h, e.ID}
	if p.ambiguous {
		return "ambiguous-gbt-transfer", nil
	}
	if p.last == 65535 || m.gbtNumber != p.last+1 {
		p.ambiguous = true
		p.active = false
		p.data = nil
		return "unverified-gbt-block-sequence", nil
	}
	p.last = m.gbtNumber
	if len(m.blockData) == 0 {
		m.fields["acknowledgement_only"] = true
		if m.gbtAck != 0 && peer != nil {
			e.ResponseTo = peer.blocks[m.gbtAck].event
		}
		return "observed-gbt-acknowledgement", nil
	}
	if p.completed {
		p.ambiguous = true
		p.data = nil
		return "ambiguous-gbt-transfer-generation", nil
	}
	if len(p.data) == 0 {
		p.start = e.ID
	}
	p.data = append(p.data, m.blockData...)
	p.active = !m.lastBlock
	m.fields["transfer_start_event"] = p.start
	m.fields["assembled_data_length"] = len(p.data)
	if !m.lastBlock {
		return "observed-gbt-fragment", nil
	}
	// New Action-normal support is confined to direct Wrapper messages. A GBT
	// transfer does not implicitly authorize a new inner-service implementation.
	if len(p.data) > 0 && (p.data[0] == 0xc3 || p.data[0] == 0xc7) {
		return "", wrapperError(ErrUnsupportedFeature, "GBT-wrapped Action is not implemented")
	}
	// Reuse the exact existing complete Get decoder and invoke context, with
	// its complete-message event as owner. No GBT ACK alone completes a Get.
	inner := make([]byte, 8, len(p.data)+8)
	copy(inner, w[:8])
	binary.BigEndian.PutUint16(inner[6:8], uint16(len(p.data)))
	inner = append(inner, p.data...)
	charge = s.storage() + wrapperProjection(w, len(w)) + 2*wrapperProjection(inner, len(inner)) + 512 + 128*int64(total)
	if err := f.reserveSession(charge); err != nil {
		return "", err
	}
	decoded, err := decodeDLMSWrapperBudget(inner, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth-1)
	if err != nil {
		return "", err
	}
	if decoded.gbt {
		return "", wrapperError(ErrUnsupportedFeature, "nested GBT APDU is not an unciphered Get")
	}
	if err = f.consumeWrapperReserved(dir, inner, e, charge); err != nil {
		return "", err
	}
	if err = f.reserveSession(charge); err != nil {
		return "", err
	}
	assoc, _ := e.Session["Association"].(string)
	m.fields["reassembled_apdu"] = cloneSession(e.semanticFields)
	m.fields["reassembled_apdu_hex"] = hex.EncodeToString(p.data)
	m.fields["transfer_complete"] = true
	p.completed = true
	p.active = false
	p.data = nil
	return assoc, nil
}
