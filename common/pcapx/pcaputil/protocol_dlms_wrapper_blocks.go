package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Selected unciphered GET-normal data-block transfer, not GBT or GET-list blocks.
// Bound total encoded Data and block count before retaining bytes. No observation
// proves a negotiated AA, object meaning or successful meter operation.
const wrapperBlockBytes = 1024
const wrapperBlockCount = 64

func wrapperIsBlock(w []byte) bool       { return len(w) > 9 && w[9] == 2 }
func wrapperUsesDataGraph(w []byte) bool { return wrapperIsList(w) || wrapperIsBlock(w) }

func decodeWrapperBlock(m *wrapperMessage, p []byte, limit int) error {
	f := m.fields
	if m.request {
		if len(p) != 7 {
			return wrapperError(ErrMalformedMessage, "Get-next block number missing or trailing bytes")
		}
		m.blockNumber = binary.BigEndian.Uint32(p[3:])
		f["kind"] = "GetRequestNext"
	} else {
		if len(p) < 9 {
			return wrapperError(ErrMalformedMessage, "Get data-block header truncated")
		}
		if p[3] > 1 {
			return wrapperError(ErrUnsupportedFeature, "last-block boolean outside selected canonical0/1")
		}
		m.lastBlock = p[3] == 1
		m.blockNumber = binary.BigEndian.Uint32(p[4:])
		f["kind"], f["last_block"] = "GetResponseWithDataBlock", m.lastBlock
		switch p[8] {
		case 0:
			c := wrapperListCursor{wire: p, at: 9}
			n, enc, err := c.count()
			if err != nil {
				return err
			}
			if n > uint64(min(limit, wrapperBlockBytes)) {
				return wrapperError(ErrResourceExceeded, "raw data-block exceeds selected byte budget")
			}
			m.blockData, err = c.take(int(n))
			if err != nil {
				return err
			}
			if c.at != len(p) {
				return wrapperError(ErrMalformedMessage, "data-block has trailing bytes")
			}
			f["result_choice"], f["raw_data_length"], f["length_encoding_hex"], f["raw_data_hex"] = "raw-data", n, enc, hex.EncodeToString(m.blockData)
		case 1:
			if len(p) != 10 {
				return wrapperError(ErrMalformedMessage, "block access result missing or trailing bytes")
			}
			switch p[9] {
			case 0, 1, 2, 3, 4, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 250:
			default:
				return wrapperError(ErrUnsupportedFeature, "block access result outside pinned enumeration")
			}
			m.blockError = true
			f["result_choice"], f["data_access_result"] = "data-access-result", p[9]
		default:
			return wrapperError(ErrUnsupportedFeature, "data-block result outside selected choice0/1")
		}
		f["transfer_complete"], f["operation_success_verified"] = false, false
	}
	f["block_number"] = m.blockNumber
	return nil
}

func (p *wrapperPending) retireBlocks()    { p.blockData = nil; p.blockActive = false; p.awaiting = false }
func (p *wrapperPending) ambiguousBlocks() { p.pending = false; p.ambiguous = true; p.retireBlocks() }

// The original request owns TransactionID. Each GET-next owns the following
// ResponseTo. A repeated request/response or missing observed hop makes the
// transfer ambiguous; never stitch bytes based on an invoke ID alone.
func (f *binFlow) consumeWrapperBlock(s *binDLMSWrapper, m *wrapperMessage, dir int, e *ProtocolEvent) (string, error) {
	if m.request {
		p := s.seen[wrapperToken{dir, m.source, m.destination, m.invoke}]
		if p == nil {
			return "unmatched-block-request", nil
		}
		if p.ambiguous {
			return "ambiguous-invoke", nil
		}
		if !p.pending || !p.blockActive || p.awaiting || p.flags != m.flags || p.blockNumber != m.blockNumber || m.blockNumber == ^uint32(0) {
			p.ambiguousBlocks()
			return "ambiguous-block-transfer", nil
		}
		p.awaiting = true
		p.replyID = e.ID
		e.TransactionID = p.id
		return "observed-block-request", nil
	}
	p := s.seen[wrapperToken{1 - dir, m.destination, m.source, m.invoke}]
	if p == nil {
		return "unmatched-response", nil
	}
	if p.ambiguous {
		return "ambiguous-invoke", nil
	}
	if !p.pending || p.flags != m.flags || p.choice != 1 {
		return "unmatched-response", nil
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
	if m.blockError {
		p.pending = false
		p.retireBlocks()
		e.ResponseTo, e.TransactionID = reply, p.id
		return "observed-response", nil
	}
	if p.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(m.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(p.blockData) {
		s.invalidate()
		return "", wrapperError(ErrResourceExceeded, "aggregate block count/encoded Data budget exceeded")
	}
	// Reservation in consumeWrapper includes the bounded retained Data pool and
	// complete final Data graph before this copy or any semantic allocation.
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
			return "", wrapperError(ErrMalformedMessage, "assembled Data has trailing bytes")
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
	}
	e.ResponseTo, e.TransactionID = reply, p.id
	return "observed-response", nil
}
