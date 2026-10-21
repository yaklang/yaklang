package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Only the two unambiguous list-block forms select this literal wire profile.
// Next3/ACK2 need an already observed list transfer; they are also used by the
// existing single-attribute service and never establish list context alone.
func wrapperIsSetListBlock(w []byte) bool {
	return wrapperIsSet(w) && len(w) > 9 && (w[8] == 0xc1 && w[9] == 5 || w[8] == 0xc5 && w[9] == 4)
}
func wrapperSetListBlockContext(s *binDLMSWrapper, w []byte, dir int) bool {
	if wrapperIsSetListBlock(w) {
		return true
	}
	if s == nil || len(w) < 11 || !(w[8] == 0xc1 && w[9] == 3 || w[8] == 0xc5 && w[9] == 2) {
		return false
	}
	key := wrapperToken{dir, binary.BigEndian.Uint16(w[2:]), binary.BigEndian.Uint16(w[4:]), w[10] & 15}
	if w[8] == 0xc5 {
		key = wrapperToken{1 - dir, key.destination, key.source, key.invoke}
	}
	p := s.seen[key]
	return p != nil && p.pending && p.command == 0xc1 && p.choice == 5 && p.flags == w[10]
}
func decodeWrapperSetListBlock(m *wrapperMessage, p []byte, limit, depth int) error {
	m.setBlock, m.choice = true, 5
	f := m.fields
	f["transfer_complete"] = false
	if !m.request {
		if len(p) < 8 {
			return wrapperError(ErrMalformedMessage, "SET list last response count/results/block number truncated")
		}
		// The exact existing list-results codec consumes the prefix before uint32.
		// Offsets still refer to the captured APDU; no synthetic wire is emitted.
		if err := decodeWrapperSetList(m, p[:len(p)-4], limit, depth); err != nil {
			return err
		}
		m.blockNumber = binary.BigEndian.Uint32(p[len(p)-4:])
		m.lastBlock = true
		f["kind"], f["block_number"] = "SetResponseLastDataBlockWithList", m.blockNumber
		return nil
	}
	m.blockFirst = true
	c := wrapperListCursor{wire: p, at: 3, maxDepth: depth}
	n, enc, err := c.count()
	if err != nil {
		return err
	}
	if n == 0 {
		return wrapperError(ErrUnsupportedFeature, "empty Set list outside selected profile")
	}
	if n > uint64(min(limit, wrapperListItems)) {
		return wrapperError(ErrResourceExceeded, "Set list count exceeds selected collection budget")
	}
	m.count = int(n)
	f["list_count"], f["list_count_encoding_hex"] = n, enc
	items := make([]map[string]any, 0, int(n))
	for i := 0; i < int(n); i++ {
		start := c.at
		q, e := c.take(10)
		if e != nil {
			return e
		}
		if int8(q[8]) <= 0 {
			return wrapperError(ErrUnsupportedFeature, "positive signed8 Set list attribute profile")
		}
		if q[9] > 1 {
			return wrapperError(ErrUnsupportedFeature, "Set list optional access selection must be canonical0/1")
		}
		row := map[string]any{"index": i, "apdu_offset": start, "class_id": binary.BigEndian.Uint16(q), "logical_name_hex": hex.EncodeToString(q[2:8]), "logical_name": fmt.Sprintf("%d.%d.%d.%d.%d.%d", q[2], q[3], q[4], q[5], q[6], q[7]), "attribute_id": int8(q[8]), "selective_access": false, "access_selection_raw": q[9], "raw_hex": hex.EncodeToString(q), "encoded_length": 10}
		if q[9] == 1 {
			selector, e := c.take(1)
			if e != nil {
				return e
			}
			parameter, e := c.data(limit, 0)
			if e != nil {
				return e
			}
			row["selective_access"], row["access_selector"], row["access_parameters"], row["selector_semantics_verified"] = true, selector[0], parameter, false
			row["raw_hex"], row["encoded_length"] = hex.EncodeToString(p[start:c.at]), c.at-start
		}
		items = append(items, row)
	}
	m.setParameterNodes = c.nodes
	h, err := c.take(5)
	if err != nil {
		return err
	}
	if h[0] > 1 {
		return wrapperError(ErrUnsupportedFeature, "SET block last boolean must be canonical0/1")
	}
	m.lastBlock = h[0] == 1
	m.blockNumber = binary.BigEndian.Uint32(h[1:])
	size, lengthEnc, err := c.count()
	if err != nil {
		return err
	}
	if size > uint64(min(limit, wrapperBlockBytes)) {
		return wrapperError(ErrResourceExceeded, "SET raw block exceeds selected byte budget")
	}
	m.blockData, err = c.take(int(size))
	if err != nil {
		return err
	}
	if c.at != len(p) {
		return wrapperError(ErrMalformedMessage, "SET block has trailing bytes")
	}
	f["kind"], f["descriptors"] = "SetRequestWithListAndFirstDataBlock", items
	f["last_block"], f["block_number"], f["raw_data_length"], f["length_encoding_hex"], f["raw_data_hex"] = m.lastBlock, m.blockNumber, size, lengthEnc, hex.EncodeToString(m.blockData)
	return nil
}

func (f *binFlow) consumeWrapperSetListBlock(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	key := wrapperToken{dir, m.source, m.destination, m.invoke}
	if !m.request {
		key = wrapperToken{1 - dir, m.destination, m.source, m.invoke}
	}
	p := s.seen[key]
	if p != nil && p.ambiguous {
		return p.ambiguousAssociation(), nil
	}
	if m.request && !m.confirmed {
		return "unconfirmed-observation", nil
	}
	if p != nil && p.pending && p.flags != m.flags {
		if m.request {
			return "unmatched-block-request", nil
		}
		return "unmatched-response", nil
	}
	if p != nil && p.pending && (p.command != 0xc1 || p.choice != 5) {
		p.ambiguousBlocks()
		return "ambiguous-service", nil
	}
	if m.request && m.blockFirst {
		if p != nil {
			active := p.pending
			p.ambiguousBlocks()
			if active {
				return "ambiguous-block-transfer", nil
			}
			return p.ambiguousAssociation(), nil
		}
		if m.blockNumber != 1 {
			return "unmatched-block-request", nil
		}
		if len(s.seen) >= f.a.budget.MaxCollectionElements {
			s.invalidate()
			return "", wrapperError(ErrResourceExceeded, "retained SET list block context exceeds collection budget")
		}
		p = &wrapperPending{id: e.ID, command: 0xc1, flags: m.flags, choice: 5, count: m.count, pending: true, setParameterNodes: m.setParameterNodes}
		if s.seen == nil {
			s.seen = make(map[wrapperToken]*wrapperPending)
		}
		s.seen[key] = p
	} else {
		if p == nil || !p.pending || p.flags != m.flags || p.command != 0xc1 || p.choice != 5 {
			if m.request {
				return "unmatched-block-request", nil
			}
			return "unmatched-response", nil
		}
		if !p.blockActive || m.request && (p.awaiting || p.lastSetBlock || p.blockNumber == ^uint32(0) || m.blockNumber != p.blockNumber+1) || !m.request && (!p.awaiting || m.blockNumber != p.blockNumber || m.lastBlock != p.lastSetBlock || m.lastBlock && (m.choice != 5 || m.count != p.count)) {
			p.ambiguousBlocks()
			return "ambiguous-block-transfer", nil
		}
	}
	if !m.request {
		e.TransactionID, e.ResponseTo = p.id, p.replyID
		if m.lastBlock {
			p.pending = false
			p.retireBlocks()
		} else {
			p.awaiting = false
		}
		return "observed-response", nil
	}
	if p.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(m.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(p.blockData) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "aggregate SET list block count/encoded Data budget exceeded")
	}
	joined := make([]byte, len(p.blockData)+len(m.blockData))
	copy(joined, p.blockData)
	copy(joined[len(p.blockData):], m.blockData)
	if m.lastBlock {
		c := wrapperListCursor{wire: joined, nodes: p.setParameterNodes, maxDepth: f.a.budget.MaxRecursionDepth}
		n, enc, err := c.count()
		if err != nil {
			s.invalidate()
			return "", err
		}
		if n > uint64(min(f.a.budget.MaxCollectionElements, wrapperListItems)) {
			s.invalidate()
			return "", wrapperError(ErrResourceExceeded, "assembled Set value count exceeds selected collection budget")
		}
		if n != uint64(p.count) {
			s.invalidate()
			return "", wrapperError(ErrUnsupportedFeature, "Set descriptor/value count relation outside verified equal-count profile")
		}
		values := make([]map[string]any, 0, int(n))
		for i := 0; i < int(n); i++ {
			start := c.at
			data, err := c.data(f.a.budget.MaxCollectionElements, 0)
			if err != nil {
				s.invalidate()
				return "", err
			}
			values = append(values, map[string]any{"index": i, "encoded_offset": start, "data": data, "raw_hex": hex.EncodeToString(joined[start:c.at]), "encoded_length": c.at - start})
		}
		if c.at != len(joined) {
			s.invalidate()
			return "", wrapperError(ErrMalformedMessage, "assembled SET value list has trailing bytes")
		}
		m.fields["transfer_complete"], m.fields["assembled_values"], m.fields["assembled_value_count"], m.fields["assembled_value_count_encoding_hex"], m.fields["assembled_data_hex"], m.fields["observed_block_count"] = true, values, n, enc, hex.EncodeToString(joined), p.blocks+1
		p.blockData = nil
	} else {
		p.blockData = joined
	}
	p.blocks++
	p.blockNumber = m.blockNumber
	p.lastSetBlock = m.lastBlock
	p.blockActive = true
	p.awaiting = true
	p.replyID = e.ID
	e.TransactionID = p.id
	if m.blockFirst {
		return "observed-request", nil
	}
	return "observed-block-request", nil
}
