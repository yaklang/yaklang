package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// One passive Action parameter upload over complete, unsegmented HDLC
// frames. The common bounded transfer record retains only owned encoded bytes.
func dlmsActionUploadAPDU(p []byte) bool {
	return len(p) > 1 && (p[0] == 0xc3 && (p[1] == 4 || p[1] == 6) || p[0] == 0xc7 && p[1] == 4)
}
func (f *binFlow) consumeDLMSActionUpload(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s, b := f.dlms, m.block
	ctx := func(why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(ErrContextRequired, why)
	}
	if s.ambiguous {
		return ctx("Action block lacks trustworthy conversation", false)
	}
	if !m.request && s.pending != nil {
		p := s.pending
		if p.kind != "ACTION" || p.choice != 4 || p.dir == dir || p.source != m.destination || p.destination != m.source || p.flags != m.flags {
			return ctx("Action block response endpoint/service/full flags differ", false)
		}
	}
	t := s.actionTransfer
	if m.request {
		if t != nil {
			p := &t.initial
			if dir != p.dir || m.source != p.source || m.destination != p.destination || m.flags != p.flags {
				return ctx("Action block direction/address/full flags differ", false)
			}
			if s.pending != nil {
				if s.pending.dir == dir && bytes.Equal(s.pending.wire, wire) {
					if b.lastBlock {
						if err := f.finishDLMSActionUploadData(t, b, t.data, t.blocks); err != nil {
							return nil, 0, err
						}
					}
					m.fields["Association"] = "retransmitted-request"
					return m.fields, 0, nil
				}
				return ctx("distinct Action block request lacks preceding response", true)
			}
			if b.blockFirst || t.last || t.number == ^uint32(0) || b.blockNumber != t.number+1 || !s.seqKnown[dir] || m.ns != s.next[dir] || !s.seqKnown[1-dir] || m.nr != s.next[1-dir] {
				return ctx("Action block lacks consecutive observed phase/number/HDLC progression", true)
			}
		} else {
			if !b.blockFirst || b.blockNumber != 1 {
				return ctx("Action block lacks observed first block", true)
			}
			_, _, err := s.consume(m, wire, dir, id)
			if err != nil {
				return nil, 0, err
			}
			initial := *s.pending
			initial.wire = nil
			initial.requestInformation = nil
			initial.requestFrames = 0
			t = &dlmsSetTransfer{initial: initial, parameterNodes: b.setParameterNodes}
			s.actionTransfer = t
		}
		if t.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(b.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(t.data) {
			s.invalidate()
			return nil, 0, dlmsError(ErrResourceExceeded, "aggregate Action block bytes/count exceed selected/caller pool")
		}
		joined := make([]byte, len(t.data)+len(b.blockData))
		copy(joined, t.data)
		copy(joined[len(t.data):], b.blockData)
		if b.lastBlock {
			if err := f.finishDLMSActionUploadData(t, b, joined, t.blocks+1); err != nil {
				return nil, 0, err
			}
		}
		// Keep the bounded encoded proposal until the terminal response. Exact
		// final-fragment retries must expose the same complete Data without
		// retaining any caller-visible map or changing the original reply owner.
		t.data = joined
		t.blocks++
		t.number = b.blockNumber
		t.last = b.lastBlock
		if !b.blockFirst {
			s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
			s.pending = &dlmsPending{wire: bytes.Clone(wire), kind: "ACTION", dir: dir, source: m.source, destination: m.destination, id: id, invoke: m.invoke, ns: m.ns, flags: m.flags, choice: 4, count: 1}
			m.fields["Association"] = "observed-block-request"
		}
		return m.fields, 0, nil
	}
	p := s.pending
	if t == nil || p == nil {
		return ctx("Action block response lacks immediately observed request", t != nil || s.clientKnown && (!s.seqKnown[dir] || m.ns == s.next[dir]))
	}
	if p.kind != "ACTION" || p.choice != 4 || p.dir == dir || p.source != m.destination || p.destination != m.source || p.flags != m.flags {
		return ctx("Action block response endpoint/service/full flags differ", false)
	}
	if m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
		return ctx("Action block response HDLC sequence differs", false)
	}
	if b.actionUpload && (t.last || b.blockNumber != t.number) || !b.actionUpload && !t.last {
		return ctx("Action block response phase/number differs", true)
	}
	s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	s.pending = nil
	if !b.actionUpload {
		s.actionTransfer = nil
		s.operationProfile = "dlms-hdlc-action-upload"
	}
	m.fields["Association"] = "observed-response"
	return m.fields, p.id, nil
}

func (f *binFlow) finishDLMSActionUploadData(t *dlmsSetTransfer, b *wrapperMessage, joined []byte, count int) error {
	c := wrapperListCursor{wire: joined, nodes: t.parameterNodes, maxDepth: f.a.budget.MaxRecursionDepth}
	data, err := c.data(f.a.budget.MaxCollectionElements, 0)
	if err == nil && c.at != len(joined) {
		err = dlmsError(ErrMalformedMessage, "assembled Action Data has trailing bytes")
	}
	if err != nil {
		f.dlms.invalidate()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			err = dlmsError(pe.Kind, "Action pblock: "+pe.Message)
		}
		return err
	}
	b.fields["transfer_complete"], b.fields["assembled_data"], b.fields["assembled_data_hex"], b.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), count
	return nil
}
