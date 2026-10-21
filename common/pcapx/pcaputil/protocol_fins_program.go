package pcaputil

import "encoding/binary"

// W342 program transfer is a passive wire observation. Program instruction
// encoding, device protection/mode and physical execution are not inferred.
func finsProgramCommand(cmd uint16) bool { return cmd >= 0x0306 && cmd <= 0x0308 }

func finsProgramHeader(d []byte, limit int) (map[string]any, int, error) {
	if len(d) < 8 {
		return nil, 0, finsErr(ErrMalformedMessage, "program number/address/count header")
	}
	program, start, raw := finsBE(d[:2]), uint64(binary.BigEndian.Uint32(d[2:6])), finsBE(d[6:8])
	n := int(raw & 0x7fff)
	if program != 0xffff || start%4 != 0 || n%4 != 0 || n > limit {
		return nil, 0, finsErr(ErrMalformedMessage, "programFFFF, four-byte alignment and command byte limit")
	}
	return map[string]any{"program_number": program, "beginning_address": start, "byte_count": uint64(n), "count_raw": raw, "last_chunk": raw&0x8000 != 0, "execution_verified": false, "instruction_decode_verified": false, "zero_count": n == 0}, n, nil
}
func finsProgramQuery(cmd uint16, d []byte, b ParserBudget) (map[string]any, error) {
	if b.MaxRecursionDepth < 5 || b.MaxCollectionElements < 64 {
		return nil, finsErr(ErrResourceExceeded, "program-area shared projection budget")
	}
	if cmd == 0x0308 {
		if len(d) != 3 || finsBE(d[:2]) != 0xffff || d[2] != 0 && d[2] != 0x10 {
			return nil, finsErr(ErrMalformedMessage, "program clear requiresFFFF and00/10 code")
		}
		return map[string]any{"operation": "clear-observation", "program_number": uint64(0xffff), "clear_code": uint64(d[2]), "with_interrupt_tasks": d[2] == 0x10, "execution_verified": false, "instruction_decode_verified": false}, nil
	}
	limit := 992
	if cmd == 0x0307 {
		limit = 996
	}
	out, n, err := finsProgramHeader(d, limit)
	if err != nil {
		return nil, err
	}
	if cmd == 0x0306 {
		if len(d) != 8 || out["last_chunk"].(bool) {
			return nil, finsErr(ErrMalformedMessage, "program read exact header and zero count bit15")
		}
		out["operation"] = "read-observation"
	} else {
		if len(d) != 8+n {
			return nil, finsErr(ErrMalformedMessage, "program write exact declared byte count")
		}
		out["operation"], out["data_hex"] = "write-observation", finsHex(d[8:])
	}
	return out, nil
}
func finsProgramRequest(cmd uint16, w []byte, f map[string]any, b ParserBudget) error {
	out, err := finsProgramQuery(cmd, w[12:], b)
	if err != nil {
		return err
	}
	f["program_area"] = out
	return nil
}
func finsProgramResponse(cmd uint16, d []byte, p *finsPending, linked bool, f map[string]any, b ParserBudget) error {
	if b.MaxRecursionDepth < 5 || b.MaxCollectionElements < 64 {
		return finsErr(ErrResourceExceeded, "program-area shared projection budget")
	}
	if cmd == 0x0308 {
		if len(d) != 0 {
			return finsErr(ErrMalformedMessage, "program clear acknowledgement must be empty")
		}
		if !linked {
			return finsErr(ErrContextRequired, "program clear acknowledgement lacks exact observed parameters")
		}
		out, err := finsProgramQuery(cmd, p.wire[12:], b)
		if err != nil {
			return err
		}
		out["request_context_observed"], out["acknowledgement_observed"] = true, true
		f["program_area"] = out
		return nil
	}
	limit := 992
	if cmd == 0x0307 {
		limit = 996
	}
	out, n, err := finsProgramHeader(d, limit)
	if err != nil {
		return err
	}
	wanted := 8
	if cmd == 0x0306 {
		wanted += n
	}
	if len(d) != wanted {
		return finsErr(ErrMalformedMessage, "program read exact byte extent/write exact echoed header")
	}
	if cmd == 0x0306 && n == 0 && out["last_chunk"].(bool) {
		return finsErr(ErrMalformedMessage, "zero program read bytes cannot include last-address data")
	}
	if linked {
		q := p.wire[12:]
		request, err := finsProgramQuery(cmd, q, b)
		if err != nil {
			return err
		}
		if finsBE(d[:2]) != finsBE(q[:2]) || uint64(binary.BigEndian.Uint32(d[2:6])) != uint64(binary.BigEndian.Uint32(q[2:6])) || uint64(n) != request["byte_count"].(uint64) || cmd == 0x0307 && out["last_chunk"] != request["last_chunk"] {
			return finsErr(ErrMalformedMessage, "program response differs from observed number/address/count/last flag")
		}
	}
	out["request_context_observed"] = linked
	if cmd == 0x0306 {
		out["operation"], out["data_hex"] = "read-observation", finsHex(d[8:])
	} else {
		out["operation"], out["acknowledgement_observed"] = "write-acknowledgement-observation", true
		if linked {
			out["requested_data_hex"] = finsHex(p.wire[20:])
		}
	}
	f["program_area"] = out
	return nil
}
