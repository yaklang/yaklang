package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Action response pblocks carry pure A-XDR Data. They carry no ActionResult,
// optional return flag or Get-style result discriminator. Observation of the
// complete value does not establish authorization or method execution.
func wrapperIsActionDownload(w []byte) bool {
	return wrapperIsAction(w) && len(w) > 9 && w[9] == 2
}
func decodeWrapperActionDownload(m *wrapperMessage, p []byte, limit int) error {
	m.actionDownload = true
	m.choice = 2
	f := m.fields
	if m.request {
		if len(p) != 7 {
			return wrapperError(ErrMalformedMessage, "Action next-pblock number missing or trailing bytes")
		}
		m.blockNumber = binary.BigEndian.Uint32(p[3:])
		f["kind"] = "ActionRequestNextPblock"
	} else {
		c := wrapperListCursor{wire: p, at: 3}
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
			return wrapperError(ErrResourceExceeded, "Action response pblock exceeds selected byte budget")
		}
		m.blockData, err = c.take(int(n))
		if err != nil {
			return err
		}
		if c.at != len(p) {
			return wrapperError(ErrMalformedMessage, "Action response pblock has trailing bytes")
		}
		f["kind"], f["last_block"], f["raw_data_length"], f["length_encoding_hex"], f["raw_data_hex"], f["transfer_complete"] = "ActionResponseWithPblock", m.lastBlock, n, enc, hex.EncodeToString(m.blockData), false
	}
	f["block_number"] = m.blockNumber
	return nil
}

func (f *binFlow) consumeWrapperActionDownload(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	key := wrapperToken{dir, m.source, m.destination, m.invoke}
	if !m.request {
		key = wrapperToken{1 - dir, m.destination, m.source, m.invoke}
	}
	p := s.seen[key]
	if p == nil {
		if m.request {
			return "unmatched-block-request", nil
		}
		return "unmatched-response", nil
	}
	if p.ambiguous {
		return p.ambiguousAssociation(), nil
	}
	if !p.pending || p.flags != m.flags {
		if m.request {
			return "unmatched-block-request", nil
		}
		return "unmatched-response", nil
	}
	if p.command != 0xc3 || p.choice != 1 && p.choice != 2 {
		p.ambiguousBlocks()
		return "ambiguous-service", nil
	}
	if m.request {
		// The request acknowledges the number just received, not the next number.
		if !p.blockActive || p.awaiting || m.blockNumber != p.blockNumber || m.blockNumber == ^uint32(0) {
			p.ambiguousBlocks()
			return "ambiguous-block-transfer", nil
		}
		p.awaiting = true
		p.replyID = e.ID
		e.TransactionID = p.id
		return "observed-block-request", nil
	}
	expected := uint32(1)
	reply := p.id
	if p.blockActive {
		expected = p.blockNumber + 1
		reply = p.replyID
	}
	if p.blockActive && !p.awaiting || m.blockNumber != expected {
		p.ambiguousBlocks()
		return "ambiguous-block-transfer", nil
	}
	if p.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(m.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(p.blockData) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "aggregate Action response pblock count/encoded Data budget exceeded")
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
			return "", wrapperError(ErrMalformedMessage, "assembled Action response Data has trailing bytes")
		}
		m.fields["transfer_complete"], m.fields["assembled_data"], m.fields["assembled_data_hex"], m.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), p.blocks+1
		p.pending = false
		p.retireBlocks()
	} else {
		p.blockData = joined
		p.blockActive = true
		p.awaiting = false
		p.blockNumber = m.blockNumber
		p.blocks++
		p.choice = 2
	}
	e.TransactionID, e.ResponseTo = p.id, reply
	return "observed-response", nil
}
