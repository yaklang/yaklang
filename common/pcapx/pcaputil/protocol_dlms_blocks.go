package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// One complete unsegmented HDLC LN GET transfer. The initial descriptor owns
// the interpretation of its assembled body; each observed request owns only
// the next response identity. Native TransactionID remains zero.
type dlmsTransfer struct {
	initial dlmsPending
	data    []byte
	number  uint32
	blocks  int
}

func dlmsBlockAPDU(p []byte) bool { return len(p) > 1 && p[1] == 2 }
func dlmsBlockError(err error) error {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		return dlmsError(pe.Kind, "Get data-block: "+pe.Message)
	}
	return err
}

func (f *binFlow) consumeDLMSBlock(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s, b := f.dlms, m.block
	context := func(why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(ErrContextRequired, why)
	}
	if s.ambiguous {
		return context("ambiguous conversation requires fresh context", false)
	}
	if m.request {
		t := s.transfer
		if t == nil {
			// Even after a completed exchange, this hop proves missing initial
			// observation. A later request must not give its delayed block an ID.
			return context("Get-next lacks observed initial request/block", true)
		}
		p := &t.initial
		if dir != p.dir || m.source != p.source || m.destination != p.destination || m.flags != p.flags {
			return context("Get-next direction/address/full flags differ from initial request", false)
		}
		if s.pending != nil {
			if s.pending.dir == dir && bytes.Equal(s.pending.wire, wire) {
				m.fields["Association"] = "retransmitted-request"
				return m.fields, 0, nil
			}
			return context("distinct request conflicts with pending Get-next", true)
		}
		if b.blockNumber != t.number || b.blockNumber == ^uint32(0) || !s.seqKnown[dir] || m.ns != s.next[dir] || !s.seqKnown[1-dir] || m.nr != s.next[1-dir] {
			return context("Get-next lacks consecutive observed block/sequence progression", true)
		}
		s.next[dir] = (m.ns + 1) & 7
		s.pending = &dlmsPending{wire: bytes.Clone(wire), kind: "GET", dir: dir, source: m.source, destination: m.destination, id: id, invoke: m.invoke, ns: m.ns, flags: m.flags, choice: 2}
		m.fields["Association"] = "observed-block-request"
		return m.fields, 0, nil
	}
	p := s.pending
	if p == nil {
		retire := s.transfer != nil || s.clientKnown && (!s.seqKnown[dir] || m.ns == s.next[dir])
		return context("data-block response lacks immediately observed request", retire)
	}
	if p.kind != "GET" || p.dir == dir || p.source != m.destination || p.destination != m.source || p.flags != m.flags {
		return context("data-block response endpoint/direction/full flags differ", false)
	}
	if m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
		return context("data-block response HDLC sequence differs from observed request", false)
	}
	t := s.transfer
	expected := uint32(1)
	if t == nil {
		if p.choice != 1 && p.choice != 3 {
			return context("initial request service cannot define assembled result", true)
		}
	} else {
		if p.choice != 2 || t.number == ^uint32(0) {
			return context("data-block lacks observed Get-next", true)
		}
		expected = t.number + 1
	}
	if b.blockNumber != expected {
		return context("data-block number is missing/repeated/nonconsecutive", true)
	}
	if b.blockError {
		s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
		s.pending, s.transfer = nil, nil
		m.fields["Association"] = "observed-response"
		return m.fields, p.id, nil
	}
	if t == nil {
		initial := *p
		initial.wire = nil // No duplicate copy of the initial descriptor.
		t = &dlmsTransfer{initial: initial}
	}
	if t.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(b.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(t.data) {
		s.invalidate()
		return nil, 0, dlmsError(ErrResourceExceeded, "aggregate data-block bytes/count exceed selected/caller pool")
	}
	// consumeDLMS reserved the complete fixed graph and both retained/joined
	// buffers before decode or this copy. No graph is inferred from fragments.
	joined := make([]byte, len(t.data)+len(b.blockData))
	copy(joined, t.data)
	copy(joined[len(t.data):], b.blockData)
	if b.lastBlock {
		if t.initial.choice == 3 {
			wp := &wrapperPending{choice: 3, count: t.initial.count, blocks: t.blocks}
			matched, err := f.finishWrapperListBlock(wp, b, joined)
			if err != nil {
				s.invalidate()
				return nil, 0, dlmsBlockError(err)
			}
			if !matched {
				return context("assembled result count differs from observed descriptors", true)
			}
		} else {
			c := wrapperListCursor{wire: joined, maxDepth: f.a.budget.MaxRecursionDepth}
			data, err := c.data(f.a.budget.MaxCollectionElements, 0)
			if err == nil && c.at != len(joined) {
				err = dlmsError(ErrMalformedMessage, "assembled Data has trailing bytes")
			}
			if err != nil {
				s.invalidate()
				return nil, 0, dlmsBlockError(err)
			}
			b.fields["transfer_complete"], b.fields["assembled_data"], b.fields["assembled_data_hex"], b.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), t.blocks+1
		}
		s.transfer = nil
	} else {
		t.data, t.number = joined, b.blockNumber
		t.blocks++
		s.transfer = t
	}
	s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	s.pending = nil
	m.fields["Association"] = "observed-response"
	return m.fields, p.id, nil
}
