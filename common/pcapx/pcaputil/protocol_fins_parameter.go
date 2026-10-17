package pcaputil

// W342 parameter-area operations are passive observations. In particular a
// zero acknowledgement does not prove physical writing in the CPU's RUN mode.
func finsParameterCommand(cmd uint16) bool { return cmd >= 0x0201 && cmd <= 0x0203 }

func finsParameterHeader(d []byte) (map[string]any, int, bool, error) {
	if len(d) < 6 {
		return nil, 0, false, finsErr(ErrMalformedMessage, "parameter area/beginning/count header")
	}
	area := finsBE(d[:2])
	var name string
	var capacity uint64
	switch area {
	case 0x8000:
		name, capacity = "combined-parameter-area", 8000
	case 0x8010:
		name, capacity = "plc-setup", 512
	case 0x8012:
		name, capacity = "io-table-registration", 1280
	case 0x8013:
		name, capacity = "routing-table", 512
	case 0x8002:
		name, capacity = "cpu-bus-unit-setup", 5184
	default:
		return nil, 0, false, finsErr(ErrUnsupportedFeature, "selected W342 parameter area table")
	}
	raw := finsBE(d[4:6])
	n := int(raw & 0x7fff)
	last := raw&0x8000 != 0
	return map[string]any{"area_code": area, "area_name": name, "selected_area_words": capacity, "beginning_word": finsBE(d[2:4]), "word_count": uint64(n), "count_raw": raw, "last_word": last, "execution_verified": false}, n, last, nil
}
func finsParameterQuery(cmd uint16, d []byte, b ParserBudget) (map[string]any, error) {
	if b.MaxRecursionDepth < 5 || b.MaxCollectionElements < 64 {
		return nil, finsErr(ErrResourceExceeded, "parameter-area shared projection budget")
	}
	out, n, last, err := finsParameterHeader(d)
	if err != nil {
		return nil, err
	}
	switch cmd {
	case 0x0201:
		if len(d) != 6 || last {
			return nil, finsErr(ErrMalformedMessage, "read header/count bit15 must be zero")
		}
		out["operation"] = "read-observation"
	case 0x0202:
		if len(d) != 6+2*n {
			return nil, finsErr(ErrMalformedMessage, "write exact word count/data")
		}
		if n > 498 {
			return nil, finsErr(ErrUnsupportedFeature, "selected 498-word parameter write limit")
		}
		if n > b.MaxCollectionElements-64 {
			return nil, finsErr(ErrResourceExceeded, "parameter write values")
		}
		values := make([]any, n)
		for j := range values {
			values[j] = finsBE(d[6+2*j : 8+2*j])
		}
		out["operation"], out["data_hex"], out["values"] = "write-observation", finsHex(d[6:]), values
	case 0x0203:
		// Clear count is a scalar16-bit number, not read/write's last-word bitfield.
		n = int(finsBE(d[4:6]))
		out["word_count"], out["count_raw"], out["last_word"] = uint64(n), uint64(n), false
		if len(d) != 8 || finsBE(d[2:4]) != 0 || finsBE(d[6:8]) != 0 {
			return nil, finsErr(ErrMalformedMessage, "clear beginning/data must be0000")
		}
		out["operation"], out["clear_word"] = "clear-observation", uint64(0)
	default:
		return nil, finsErr(ErrUnsupportedFeature, "selected parameter commands")
	}
	out["zero_count"] = n == 0
	return out, nil
}
func finsParameterRequest(cmd uint16, w []byte, f map[string]any, b ParserBudget) error {
	out, err := finsParameterQuery(cmd, w[12:], b)
	if err != nil {
		return err
	}
	f["parameter_area"] = out
	return nil
}
func finsParameterResponse(cmd uint16, data []byte, p *finsPending, linked bool, f map[string]any, b ParserBudget, code uint16) error {
	if cmd != 0x0201 {
		if len(data) != 0 {
			return finsErr(ErrMalformedMessage, "parameter acknowledgement must be empty")
		}
		if !linked {
			return finsErr(ErrContextRequired, "parameter acknowledgement lacks exact observed request")
		}
		out, err := finsParameterQuery(cmd, p.wire[12:], b)
		if err != nil {
			return err
		}
		out["request_context_observed"], out["acknowledgement_observed"] = true, true
		f["parameter_area"] = out
		return nil
	}
	if b.MaxRecursionDepth < 5 || b.MaxCollectionElements < 64 {
		return finsErr(ErrResourceExceeded, "parameter-area shared projection budget")
	}
	out, n, last, err := finsParameterHeader(data)
	if err != nil {
		return err
	}
	if n > 498 {
		return finsErr(ErrUnsupportedFeature, "selected 498-word parameter read response")
	}
	if len(data) != 6+2*n {
		return finsErr(ErrMalformedMessage, "read exact word count/data")
	}
	start, capacity := int(finsBE(data[2:4])), int(out["selected_area_words"].(uint64))
	if start+n > capacity || last != (n > 0 && start+n == capacity) {
		return finsErr(ErrMalformedMessage, "read last-word/count boundary")
	}
	if code == 0x1104 && (n == 0 || start+n != capacity) {
		return finsErr(ErrMalformedMessage, "range partial must contain last word")
	}
	if code == 0x1108 && n != 498 {
		return finsErr(ErrMalformedMessage, "length partial must return498 words")
	}
	if linked {
		q := p.wire[12:]
		wanted := int(finsBE(q[4:6]))
		available := max(capacity-start, 0)
		if finsBE(data[:2]) != finsBE(q[:2]) || finsBE(data[2:4]) != finsBE(q[2:4]) {
			return finsErr(ErrMalformedMessage, "read echoed area/beginning differs from request")
		}
		if code == 0 && n != wanted {
			return finsErr(ErrMalformedMessage, "complete read count differs from request")
		}
		if code == 0x1104 {
			if wanted > 498 && available < 498 {
				return finsErr(ErrContextRequired, "simultaneous count/range priority not fixed by selected source")
			}
			if wanted <= available || n != available {
				return finsErr(ErrMalformedMessage, "range partial must return all available words")
			}
		}
		if code == 0x1108 && (wanted <= 498 || available < 498) {
			return finsErr(ErrMalformedMessage, "length partial lacks over498 request context")
		}
	}
	if n > b.MaxCollectionElements-64 {
		return finsErr(ErrResourceExceeded, "parameter read values")
	}
	values := make([]any, n)
	for j := range values {
		values[j] = finsBE(data[6+2*j : 8+2*j])
	}
	out["operation"], out["request_context_observed"], out["partial_response"], out["partial_end_code"] = "read-observation", linked, code != 0, uint64(code)
	out["data_hex"], out["values"], out["zero_count"] = finsHex(data[6:]), values, n == 0
	f["parameter_area"] = out
	return nil
}
