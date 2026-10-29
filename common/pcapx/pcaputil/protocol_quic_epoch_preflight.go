package pcaputil

// Validate transport state on owned copies before a future key epoch can
// install keys or deliver any of its preceding frames. Application dissectors
// are intentionally not invoked: a bad HTTP/3 message is not a bad AEAD key.
func (q *binQUIC) validateFutureTransportFrames(dir, space int, frames []map[string]any, max int) error {
	probe := *q
	probe.cids = make(map[string]int, len(q.cids))
	for cid, owner := range q.cids {
		probe.cids[cid] = owner
	}
	for d := 0; d < 2; d++ {
		probe.cidSequences[d] = cloneQUICCIDSequences(q.cidSequences[d])
		probe.cidRetired[d] = cloneQUICCIDSequences(q.cidRetired[d])
	}
	streams := map[uint64]*quicAssembler{}
	created := 0
	stream := func(id uint64) *quicAssembler {
		if s := streams[id]; s != nil {
			return s
		}
		var assembly quicAssembler
		if old := q.streams[id]; old != nil {
			assembly = cloneQUICEpochAssembler(old.reassembly[dir])
		} else {
			if len(q.streams)+created >= max {
				return nil
			}
			created++
		}
		streams[id] = &assembly
		return &assembly
	}
	var crypto *quicAssembler
	for _, frame := range frames {
		switch frame["Frame Type"] {
		case "NEW_CONNECTION_ID", "RETIRE_CONNECTION_ID":
			copy := make(map[string]any, len(frame))
			for name, value := range frame {
				copy[name] = value
			}
			if err := probe.applyFrames(dir, space, []map[string]any{copy}, map[string]any{}, max); err != nil {
				return err
			}
		case "STREAM":
			id, _ := frame["Stream ID"].(uint64)
			if id&2 != 0 && int(id&1) != dir {
				return protocolError(ErrMalformedMessage, "QUIC unidirectional stream wrong sender")
			}
			r := stream(id)
			if r == nil {
				return protocolError(ErrResourceExceeded, "QUIC stream budget exceeded")
			}
			offset, _ := frame["Offset"].(uint64)
			data, _ := frame["Stream Data"].([]byte)
			fin, _ := frame["FIN"].(bool)
			if _, _, _, err := r.feed(offset, data, fin, q.byteLimit, max); err != nil {
				return err
			}
		case "RESET_STREAM":
			id, _ := frame["Stream ID"].(uint64)
			if r := stream(id); r != nil {
				final, _ := frame["Final Size"].(uint64)
				if _, _, _, err := r.feed(final, nil, true, q.byteLimit, max); err != nil {
					return err
				}
			}
		case "STOP_SENDING":
			stream(frame["Stream ID"].(uint64))
		case "CRYPTO":
			if crypto == nil {
				copy := cloneQUICEpochAssembler(q.cryptoData[dir][space])
				crypto = &copy
			}
			offset, _ := frame["Offset"].(uint64)
			data, _ := frame["Crypto Data"].([]byte)
			if _, _, _, err := crypto.feed(offset, data, false, q.byteLimit, max); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneQUICCIDSequences(source map[uint64]string) map[uint64]string {
	copy := make(map[uint64]string, len(source))
	for sequence, cid := range source {
		copy[sequence] = cid
	}
	return copy
}

func cloneQUICEpochAssembler(source quicAssembler) quicAssembler {
	copy := source
	copy.data = cloneQUICEpochBytes(source.data)
	copy.pending = make([]quicPiece, len(source.pending), cap(source.pending))
	for i, piece := range source.pending {
		copy.pending[i] = quicPiece{off: piece.off, data: cloneQUICEpochBytes(piece.data)}
	}
	return copy
}

func cloneQUICEpochBytes(source []byte) []byte {
	if source == nil {
		return nil
	}
	// Reassembly budgets account for retained capacity, not just length.
	// Shrinking copies could pass a probe whose real assembler must refuse.
	owned := make([]byte, len(source), cap(source))
	copy(owned, source)
	return owned
}
