package pcaputil

// Only passive wire observations. No memory is applied, reconstructed or verified.
func finsOperationCommand(cmd uint16) bool { return cmd == 0x0102 || cmd == 0x0103 || cmd == 0x0105 }

func finsOperationFields(cmd uint16, w []byte, b ParserBudget) (map[string]any, error) {
	if b.MaxRecursionDepth < 5 || b.MaxCollectionElements < 64 {
		return nil, finsErr(ErrResourceExceeded, "memory operation shared projection budget")
	}
	var out map[string]any
	var count int
	if cmd == 0x0105 {
		if len(w) != 22 {
			return nil, finsErr(ErrMalformedMessage, "transfer needs two descriptors and word count")
		}
		source, sk, _, err := finsMemoryDescriptor(w[12:16], false)
		if err != nil {
			return nil, err
		}
		destination, dk, _, err := finsMemoryDescriptor(w[16:20], false)
		if err != nil {
			return nil, err
		}
		if sk != "word" || dk != "word" {
			return nil, finsErr(ErrUnsupportedFeature, "selected word transfer layouts only")
		}
		count = int(finsBE(w[20:22]))
		out = map[string]any{"operation": "transfer-observation", "source": source, "destination": destination, "item_count": uint64(count)}
	} else {
		if len(w) < 18 || cmd == 0x0103 && len(w) != 20 {
			return nil, finsErr(ErrMalformedMessage, "write/fill descriptor count and data length")
		}
		target, kind, width, err := finsMemoryDescriptor(w[12:16], false)
		if err != nil {
			return nil, err
		}
		count = int(finsBE(w[16:18]))
		out = map[string]any{"target": target, "item_count": uint64(count)}
		if cmd == 0x0102 {
			if len(w)-18 != count*width {
				return nil, finsErr(ErrMalformedMessage, "write exact item width/count")
			}
			if count > 1994/width {
				return nil, finsErr(ErrUnsupportedFeature, "selected2012-byte write frame capacity")
			}
			if count > b.MaxCollectionElements-64 {
				return nil, finsErr(ErrResourceExceeded, "write value projection")
			}
			values := make([]any, count)
			for i := range values {
				if kind == "bit" {
					if w[18+i] > 1 {
						return nil, finsErr(ErrMalformedMessage, "plain write bit must be00/01")
					}
					values[i] = w[18+i] != 0
				} else {
					values[i] = finsBE(w[18+2*i : 20+2*i])
				}
			}
			out["operation"], out["values"] = "write-observation", values
		} else {
			if kind != "word" {
				return nil, finsErr(ErrUnsupportedFeature, "selected word fill layouts only")
			}
			out["operation"], out["fill_word"] = "fill-observation", finsBE(w[18:20])
		}
		out["data_hex"] = finsHex(w[18:])
	}
	out["zero_count"], out["execution_verified"] = count == 0, false
	return out, nil
}
func finsOperationRequest(cmd uint16, w []byte, f map[string]any, b ParserBudget) error {
	out, err := finsOperationFields(cmd, w, b)
	if err != nil {
		return err
	}
	f["memory_operation"] = out
	return nil
}
func finsOperationResponse(cmd uint16, data []byte, p *finsPending, linked bool, f map[string]any, b ParserBudget) error {
	if len(data) != 0 {
		return finsErr(ErrMalformedMessage, "memory acknowledgement must have no data")
	}
	if !linked {
		return finsErr(ErrContextRequired, "memory acknowledgement lacks exact observed operation")
	}
	// Rebuild from private owned bytes; caller mutation of request fields cannot
	// change the response's descriptor, values or transaction generation.
	out, err := finsOperationFields(cmd, p.wire, b)
	if err != nil {
		return err
	}
	out["request_context_observed"], out["acknowledgement_observed"] = true, true
	f["memory_operation"] = out
	return nil
}
