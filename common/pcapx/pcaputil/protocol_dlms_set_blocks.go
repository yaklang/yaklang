package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// One observed plaintext SET proposal over complete, unsegmented HDLC frames.
// Link sequencing is independent of Wrapper invoke-history policy. Neither a
// reassembled proposal nor a literal response establishes a device write.
type dlmsSetTransfer struct {
	initial                dlmsPending
	data                   []byte
	number                 uint32
	blocks, parameterNodes int
	last                   bool
}

func dlmsSetBlockAPDU(p []byte) bool {
	return len(p) > 1 && (p[0] == 0xc1 || p[0] == 0xc5) && (p[1] == 2 || p[1] == 3)
}
func (t *dlmsSetTransfer) closeFields() map[string]any {
	p := t.initial
	return map[string]any{"Outstanding": 1, "Request Service": p.kind, "Invoke ID": p.invoke, "Source Address": p.source, "Destination Address": p.destination}
}
func (f *binFlow) consumeDLMSSetBlock(dir int, wire []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s, b := f.dlms, m.block
	choice := byte(2)
	if f.dlmsSetListBlockContext(wire, dir) {
		choice = 5
	}
	if m.setResponseAssemblyChoice != 0 {
		choice = m.setResponseAssemblyChoice
	}
	ctx := func(why string, retire bool) (map[string]any, uint64, error) {
		if retire {
			s.invalidate()
		}
		return nil, 0, dlmsError(ErrContextRequired, why)
	}
	if s.ambiguous {
		return ctx("SET block lacks trustworthy conversation", false)
	}
	// A complete foreign service/endpoint/flags response cannot retire an
	// unrelated known pending request. Check its owner before orphan transfer
	// handling, which may conservatively retire an actually missing exchange.
	if !m.request && s.pending != nil {
		p := s.pending
		if p.kind != "SET" || p.choice != choice || p.dir == dir || p.source != m.destination || p.destination != m.source || p.flags != m.flags {
			return ctx("SET block response endpoint/service/full flags differ", false)
		}
	}
	t := s.setTransfer
	if m.request {
		if t != nil {
			p := &t.initial
			if dir != p.dir || m.source != p.source || m.destination != p.destination || m.flags != p.flags {
				return ctx("SET block direction/address/full flags differ", false)
			}
			if s.pending != nil {
				if s.pending.dir == dir && bytes.Equal(s.pending.wire, wire) {
					if b.lastBlock {
						if err := f.finishDLMSSetBlockData(t, b, t.data, t.blocks); err != nil {
							return nil, 0, err
						}
					}
					m.fields["Association"] = "retransmitted-request"
					return m.fields, 0, nil
				}
				return ctx("distinct SET block request lacks preceding response", true)
			}
			if b.blockFirst || t.last || t.number == ^uint32(0) || b.blockNumber != t.number+1 || !s.seqKnown[dir] || m.ns != s.next[dir] || !s.seqKnown[1-dir] || m.nr != s.next[1-dir] {
				return ctx("SET block lacks consecutive observed phase/number/HDLC progression", true)
			}
		} else {
			if !b.blockFirst || b.blockNumber != 1 {
				return ctx("SET block lacks observed first block", true)
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
			s.setTransfer = t
		}
		if t.blocks >= min(f.a.budget.MaxCollectionElements, wrapperBlockCount) || len(b.blockData) > min(f.a.budget.MaxCollectionElements, wrapperBlockBytes)-len(t.data) {
			s.invalidate()
			return nil, 0, dlmsError(ErrResourceExceeded, "aggregate SET block bytes/count exceed selected/caller pool")
		}
		joined := make([]byte, len(t.data)+len(b.blockData))
		copy(joined, t.data)
		copy(joined[len(t.data):], b.blockData)
		if b.lastBlock {
			if err := f.finishDLMSSetBlockData(t, b, joined, t.blocks+1); err != nil {
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
			s.pending = &dlmsPending{wire: bytes.Clone(wire), kind: "SET", dir: dir, source: m.source, destination: m.destination, id: id, invoke: m.invoke, ns: m.ns, flags: m.flags, choice: t.initial.choice, count: t.initial.count}
			m.fields["Association"] = "observed-block-request"
		}
		return m.fields, 0, nil
	}
	p := s.pending
	if t == nil || p == nil {
		return ctx("SET block response lacks immediately observed request", t != nil || s.clientKnown && (!s.seqKnown[dir] || m.ns == s.next[dir]))
	}
	if p.kind != "SET" || p.choice != choice || p.dir == dir || p.source != m.destination || p.destination != m.source || p.flags != m.flags {
		return ctx("SET block response endpoint/service/full flags differ", false)
	}
	if m.nr != (p.ns+1)&7 || s.seqKnown[dir] && m.ns != s.next[dir] {
		return ctx("SET block response HDLC sequence differs", false)
	}
	if b.blockNumber != t.number || b.lastBlock != t.last || choice == 5 && b.lastBlock && (m.choice != 5 || m.count != t.initial.count) {
		return ctx("SET block response phase/number differs", true)
	}
	s.next[dir], s.seqKnown[dir] = (m.ns+1)&7, true
	s.pending = nil
	if b.lastBlock {
		s.setTransfer = nil
	}
	m.fields["Association"] = "observed-response"
	return m.fields, p.id, nil
}

func (f *binFlow) finishDLMSSetBlockData(t *dlmsSetTransfer, b *wrapperMessage, joined []byte, count int) error {
	if t.initial.choice == 5 {
		return f.finishDLMSSetListBlockData(t, b, joined, count)
	}
	c := wrapperListCursor{wire: joined, nodes: t.parameterNodes, maxDepth: f.a.budget.MaxRecursionDepth}
	data, err := c.data(f.a.budget.MaxCollectionElements, 0)
	if err == nil && c.at != len(joined) {
		err = dlmsError(ErrMalformedMessage, "assembled SET Data has trailing bytes")
	}
	if err != nil {
		f.dlms.invalidate()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			err = dlmsError(pe.Kind, "SET data-block: "+pe.Message)
		}
		return err
	}
	b.fields["transfer_complete"], b.fields["assembled_data"], b.fields["assembled_data_hex"], b.fields["observed_block_count"] = true, data, hex.EncodeToString(joined), count
	return nil
}
