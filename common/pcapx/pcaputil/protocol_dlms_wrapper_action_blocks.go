package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Selected Action parameter uploads, not response-download pblocks, GBT,
// method authorization or evidence that a remote object executed the method.
func wrapperIsActionUpload(w []byte) bool {
	return len(w) > 9 && ((w[8] == 0xc3 && (w[9] == 4 || w[9] == 6)) || (w[8] == 0xc7 && w[9] == 4))
}
func decodeWrapperActionUpload(m *wrapperMessage, p []byte, limit int) error {
	m.actionUpload = true
	m.choice = 4
	f := m.fields
	f["transfer_complete"] = false
	c := wrapperListCursor{wire: p, at: 3}
	if m.request {
		m.blockFirst = p[1] == 4
		if m.blockFirst {
			q, err := c.take(9)
			if err != nil {
				return err
			}
			if int8(q[8]) <= 0 {
				return wrapperError(ErrUnsupportedFeature, "positive signed8 Action upload method profile")
			}
			f["kind"], f["class_id"], f["logical_name_hex"], f["logical_name"], f["method_id"] = "ActionRequestWithFirstPblock", binary.BigEndian.Uint16(q), hex.EncodeToString(q[2:8]), fmt.Sprintf("%d.%d.%d.%d.%d.%d", q[2], q[3], q[4], q[5], q[6], q[7]), int8(q[8])
		} else {
			f["kind"] = "ActionRequestWithPblock"
		}
		h, err := c.take(5)
		if err != nil {
			return err
		}
		if h[0] > 1 {
			return wrapperError(ErrUnsupportedFeature, "Action pblock last boolean must be canonical0/1")
		}
		m.lastBlock = h[0] == 1
		m.blockNumber = binary.BigEndian.Uint32(h[1:])
		n, enc, err := c.count()
		if err != nil {
			return err
		}
		if n > uint64(min(limit, wrapperBlockBytes)) {
			return wrapperError(ErrResourceExceeded, "Action raw pblock exceeds selected byte budget")
		}
		m.blockData, err = c.take(int(n))
		if err != nil {
			return err
		}
		f["last_block"], f["raw_data_length"], f["length_encoding_hex"], f["raw_data_hex"] = m.lastBlock, n, enc, hex.EncodeToString(m.blockData)
	} else {
		h, err := c.take(4)
		if err != nil {
			return err
		}
		m.blockNumber = binary.BigEndian.Uint32(h)
		f["kind"] = "ActionResponseNextPblock"
	}
	if c.at != len(p) {
		return wrapperError(ErrMalformedMessage, "Action pblock has trailing bytes")
	}
	f["block_number"] = m.blockNumber
	return nil
}

// A normal terminal response inherits the upload profile only after an exact
// observed final fragment. Orphan/ambiguous normal replies keep their wire
// normal profile and never acquire an upload owner.
func wrapperActionUploadPending(s *binDLMSWrapper, m *wrapperMessage, dir int) bool {
	if m.request || m.command != 0xc7 || m.choice != 1 {
		return false
	}
	p := s.seen[wrapperToken{1 - dir, m.destination, m.source, m.invoke}]
	return p != nil && p.pending && !p.ambiguous && p.command == 0xc3 && p.choice == 4 && p.flags == m.flags
}
func (f *binFlow) consumeWrapperActionUpload(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
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
	if p != nil && p.pending && p.flags == m.flags && (p.command != 0xc3 || p.choice != 4) {
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
			return "", wrapperError(ErrResourceExceeded, "retained Action upload context exceeds collection budget")
		}
		p = &wrapperPending{id: e.ID, command: 0xc3, flags: m.flags, choice: 4, count: 1, pending: true}
		if s.seen == nil {
			s.seen = make(map[wrapperToken]*wrapperPending)
		}
		s.seen[key] = p
	} else {
		if p == nil || !p.pending || p.flags != m.flags || p.command != 0xc3 || p.choice != 4 {
			if m.request {
				return "unmatched-block-request", nil
			}
			return "unmatched-response", nil
		}
		if !p.blockActive || m.request && (p.awaiting || p.lastActionBlock || p.blockNumber == ^uint32(0) || m.blockNumber != p.blockNumber+1) || !m.request && (!p.awaiting || m.actionUpload && (p.lastActionBlock || m.blockNumber != p.blockNumber) || !m.actionUpload && !p.lastActionBlock) {
			p.ambiguousBlocks()
			return "ambiguous-block-transfer", nil
		}
	}
	if !m.request {
		e.TransactionID, e.ResponseTo = p.id, p.replyID
		if !m.actionUpload {
			p.pending = false
			p.retireBlocks()
			e.Profile = "dlms-wrapper-v1-action-block"
		} else {
			p.awaiting = false
		}
		return "observed-response", nil
	}
	if p.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(m.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(p.blockData) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "aggregate Action pblock count/encoded Data budget exceeded")
	}
	joined := make([]byte, len(p.blockData)+len(m.blockData))
	copy(joined, p.blockData)
	copy(joined[len(p.blockData):], m.blockData)
	if m.lastBlock {
		c := wrapperListCursor{wire: joined, maxDepth: f.a.budget.MaxRecursionDepth}
		data, err := c.data(f.a.budget.MaxCollectionElements, 0)
		if err != nil {
			s.invalidate()
			return "", err
		}
		if c.at != len(joined) {
			s.invalidate()
			return "", wrapperError(ErrMalformedMessage, "assembled Action parameter Data has trailing bytes")
		}
		m.fields["transfer_complete"], m.fields["assembled_data"], m.fields["assembled_data_hex"], m.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), p.blocks+1
		p.blockData = nil
	} else {
		p.blockData = joined
	}
	p.blocks++
	p.blockNumber = m.blockNumber
	p.lastActionBlock = m.lastBlock
	p.blockActive = true
	p.awaiting = true
	p.replyID = e.ID
	e.TransactionID = p.id
	if m.blockFirst {
		return "observed-request", nil
	}
	return "observed-block-request", nil
}
