package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Selected plaintext SET DataBlock-SA observations. A complete proposal and a
// literal response never establish permission or a successful device write.
func wrapperIsSetBlock(w []byte) bool {
	return len(w) > 9 && (w[8] == 0xc1 || w[8] == 0xc5) && (w[9] == 2 || w[9] == 3)
}
func decodeWrapperSetBlock(m *wrapperMessage, p []byte, limit, depth int) error {
	f := m.fields
	m.setBlock = true
	m.choice = 2
	c := wrapperListCursor{wire: p, at: 3, maxDepth: depth}
	f["transfer_complete"] = false
	if m.request {
		m.blockFirst = p[1] == 2
		if m.blockFirst {
			q, err := c.take(10)
			if err != nil {
				return err
			}
			if int8(q[8]) <= 0 {
				return wrapperError(ErrUnsupportedFeature, "positive signed8 SET block attribute profile")
			}
			if q[9] > 1 {
				return wrapperError(ErrUnsupportedFeature, "SET block optional selection must be canonical0/1")
			}
			f["kind"] = "SetRequestWithFirstDataBlock"
			f["class_id"], f["logical_name_hex"], f["logical_name"], f["attribute_id"], f["selective_access"] = binary.BigEndian.Uint16(q), hex.EncodeToString(q[2:8]), fmt.Sprintf("%d.%d.%d.%d.%d.%d", q[2], q[3], q[4], q[5], q[6], q[7]), int8(q[8]), q[9] == 1
			if q[9] == 1 {
				selector, e := c.take(1)
				if e != nil {
					return e
				}
				parameter, e := c.data(limit, 0)
				if e != nil {
					return e
				}
				f["access_selection_raw"], f["access_selector"], f["access_parameters"], f["selector_semantics_verified"] = q[9], selector[0], parameter, false
				m.setParameterNodes = c.nodes
			}
		} else {
			f["kind"] = "SetRequestWithDataBlock"
		}
		h, err := c.take(5)
		if err != nil {
			return err
		}
		if h[0] > 1 {
			return wrapperError(ErrUnsupportedFeature, "SET block last boolean must be canonical0/1")
		}
		m.lastBlock = h[0] == 1
		m.blockNumber = binary.BigEndian.Uint32(h[1:])
		n, enc, err := c.count()
		if err != nil {
			return err
		}
		if n > uint64(min(limit, wrapperBlockBytes)) {
			return wrapperError(ErrResourceExceeded, "SET raw block exceeds selected byte budget")
		}
		m.blockData, err = c.take(int(n))
		if err != nil {
			return err
		}
		if c.at != len(p) {
			return wrapperError(ErrMalformedMessage, "SET block has trailing bytes")
		}
		f["last_block"], f["raw_data_length"], f["length_encoding_hex"], f["raw_data_hex"] = m.lastBlock, n, enc, hex.EncodeToString(m.blockData)
	} else if p[1] == 2 {
		h, err := c.take(4)
		if err != nil {
			return err
		}
		m.blockNumber = binary.BigEndian.Uint32(h)
		f["kind"] = "SetResponseDataBlock"
		if c.at != len(p) {
			return wrapperError(ErrMalformedMessage, "SET block ACK has trailing bytes")
		}
	} else {
		h, err := c.take(5)
		if err != nil {
			return err
		}
		if !wrapperDataAccessResultKnown(h[0]) {
			return wrapperError(ErrUnsupportedFeature, "SET last block result outside pinned enumeration")
		}
		m.blockNumber = binary.BigEndian.Uint32(h[1:])
		m.lastBlock = true
		f["kind"], f["data_access_result"], f["result_success"] = "SetResponseLastDataBlock", h[0], h[0] == 0
		if c.at != len(p) {
			return wrapperError(ErrMalformedMessage, "SET last response has trailing bytes")
		}
	}
	f["block_number"] = m.blockNumber
	return nil
}

func (f *binFlow) consumeWrapperSetBlock(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	key := wrapperToken{dir, m.source, m.destination, m.invoke}
	if !m.request {
		key = wrapperToken{1 - dir, m.destination, m.source, m.invoke}
	}
	p := s.seen[key]
	if p != nil && p.ambiguous {
		return p.ambiguousAssociation(), nil
	}
	if !m.confirmed && m.request {
		return "unconfirmed-observation", nil
	}
	if p != nil && p.pending && p.flags != m.flags {
		if m.request {
			return "unmatched-block-request", nil
		}
		return "unmatched-response", nil
	}
	if p != nil && p.pending && p.flags == m.flags && (p.command != 0xc1 || p.choice != 2) {
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
			return "", wrapperError(ErrResourceExceeded, "retained SET block context exceeds collection budget")
		}
		p = &wrapperPending{id: e.ID, command: 0xc1, flags: m.flags, choice: 2, count: 1, pending: true, setParameterNodes: m.setParameterNodes}
		if s.seen == nil {
			s.seen = make(map[wrapperToken]*wrapperPending)
		}
		s.seen[key] = p
	} else {
		if p == nil || !p.pending || p.flags != m.flags || p.command != 0xc1 || p.choice != 2 {
			if m.request {
				return "unmatched-block-request", nil
			}
			return "unmatched-response", nil
		}
		if !p.blockActive || (m.request && (p.awaiting || p.lastSetBlock || p.blockNumber == ^uint32(0) || m.blockNumber != p.blockNumber+1)) || (!m.request && (!p.awaiting || m.blockNumber != p.blockNumber || m.lastBlock != p.lastSetBlock)) {
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
		return "", wrapperError(ErrResourceExceeded, "aggregate SET block count/encoded Data budget exceeded")
	}
	joined := make([]byte, len(p.blockData)+len(m.blockData))
	copy(joined, p.blockData)
	copy(joined[len(p.blockData):], m.blockData)
	if m.lastBlock {
		c := wrapperListCursor{wire: joined, nodes: p.setParameterNodes, maxDepth: f.a.budget.MaxRecursionDepth}
		data, err := c.data(f.a.budget.MaxCollectionElements, 0)
		if err != nil {
			s.invalidate()
			return "", err
		}
		if c.at != len(joined) {
			s.invalidate()
			return "", wrapperError(ErrMalformedMessage, "assembled SET Data has trailing bytes")
		}
		m.fields["transfer_complete"], m.fields["assembled_data"], m.fields["assembled_data_hex"], m.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), p.blocks+1
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
