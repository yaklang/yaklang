package pcaputil

import "math/bits"

func finsDiagnosticCommand(command uint16) bool {
	return command == 0x0502 || command == 0x0920 || command == 0x2102
}
func finsUnitAddress(v byte) bool { return v == 0 || v >= 0x10 && v <= 0x7f || v == 0xe1 }
func finsMessageOperation(p uint16) (string, error) {
	switch p >> 14 {
	case 0, 1:
		if p&0x3f00 != 0 {
			return "", finsErr(ErrMalformedMessage, "MSG read/clear unused parameter bits")
		}
		if p>>14 == 0 {
			return "read", nil
		}
		return "clear-observation", nil
	case 2:
		return "fal-read", nil
	}
	return "", finsErr(ErrUnsupportedFeature, "selected MSG read/clear/FAL parameter subtype")
}
func finsDiagnosticRequest(command uint16, w []byte, f map[string]any) error {
	switch command {
	case 0x0502:
		if len(w) != 13 && len(w) != 14 {
			return finsErr(ErrMalformedMessage, "connection request unit and optional count")
		}
		if !finsUnitAddress(w[12]) {
			return finsErr(ErrUnsupportedFeature, "selected CPU/bus/SpecialIO/InnerBoard unit address")
		}
		limit := byte(25)
		if len(w) == 14 {
			limit = w[13]
			if limit < 1 || limit > 25 {
				return finsErr(ErrMalformedMessage, "connection item count1..25")
			}
		}
		f["connection_query"] = map[string]any{"start_unit": uint64(w[12]), "requested_limit": uint64(limit), "limit_explicit": len(w) == 14}
	case 0x0920:
		if len(w) != 14 {
			return finsErr(ErrMalformedMessage, "message parameter needs two bytes")
		}
		p := uint16(finsBE(w[12:14]))
		op, err := finsMessageOperation(p)
		if err != nil {
			return err
		}
		q := map[string]any{"operation": op, "parameter": uint64(p)}
		if op == "fal-read" {
			q["fal_number"] = uint64(p & 0x3fff)
		} else {
			q["message_mask"] = uint64(p & 255)
		}
		f["message_query"] = q
	case 0x2102:
		if len(w) != 16 {
			return finsErr(ErrMalformedMessage, "error log query needs record start/count")
		}
		f["error_log_query"] = map[string]any{"beginning_record": finsBE(w[12:14]), "record_count": finsBE(w[14:16])}
	}
	return nil
}
func finsDiagnosticResponse(command uint16, data []byte, p *finsPending, linked bool, f map[string]any, b ParserBudget, code uint16) error {
	switch command {
	case 0x0502:
		if len(data) < 1 {
			return finsErr(ErrMalformedMessage, "connection response count missing")
		}
		n := int(data[0] & 0x7f)
		if n > 25 || len(data) != 1+n*21 {
			return finsErr(ErrMalformedMessage, "connection count/21-byte unit records")
		}
		if n > 0 && b.MaxRecursionDepth < 5 {
			return finsErr(ErrResourceExceeded, "connection shared projection depth")
		}
		if n > (b.MaxCollectionElements-32)/4 {
			return finsErr(ErrResourceExceeded, "connection unit projection")
		}
		out := map[string]any{"count_raw": uint64(data[0]), "count": uint64(n), "last_unit_reported": data[0]&0x80 != 0, "request_context_observed": linked, "device_inventory_verified": false}
		units := make([]any, 0, n)
		if linked {
			limit := uint64(25)
			if len(p.wire) == 14 {
				limit = uint64(p.wire[13])
			}
			if uint64(n) > limit {
				return finsErr(ErrMalformedMessage, "connection response exceeds observed request limit")
			}
			out["request_start_unit"], out["requested_limit"] = uint64(p.wire[12]), limit
		}
		for i := 0; i < n; i++ {
			at := 1 + i*21
			if !finsUnitAddress(data[at]) {
				return finsErr(ErrUnsupportedFeature, "selected returned unit address")
			}
			units = append(units, map[string]any{"unit_address": uint64(data[at]), "model_hex": finsHex(data[at+1 : at+21])})
		}
		out["units"] = units
		f["connection_data"] = out
	case 0x0920:
		if b.MaxCollectionElements < 64 {
			return finsErr(ErrResourceExceeded, "message field projection")
		}
		// W342 printed202: FAL request subtype10 is returned as00. The same
		// response bits can mean a MSG bitmap or a FAL number; do not guess
		// an operation from response length or a failed/reused request generation.
		if !linked {
			return finsErr(ErrContextRequired, "0920 success requires exact observed operation context")
		}
		requested := uint16(finsBE(p.wire[12:14]))
		op, err := finsMessageOperation(requested)
		if err != nil {
			return err
		}
		if op == "clear-observation" {
			if len(data) != 0 {
				return finsErr(ErrMalformedMessage, "clear reply has unexpected parameter/data")
			}
			f["message_data"] = map[string]any{"operation": op, "request_parameter": uint64(requested), "request_message_mask": uint64(requested & 255), "request_context_observed": true, "clearing_verified": false}
			return nil
		}
		if len(data) < 2 {
			return finsErr(ErrMalformedMessage, "short message response parameter")
		}
		param := uint16(finsBE(data[:2]))
		if param&0xc000 != 0 || param != requested&0x3fff {
			return finsErr(ErrMalformedMessage, "message response cleared subtype/number differs from observed request")
		}
		out := map[string]any{"operation": op, "parameter": uint64(param), "request_parameter": uint64(requested), "request_context_observed": true, "device_message_verified": false}
		if op == "fal-read" {
			if len(data) != 18 {
				return finsErr(ErrMalformedMessage, "FAL message needs16 data bytes")
			}
			out["fal_number"], out["message_hex"] = uint64(param), finsHex(data[2:])
		} else {
			n := bits.OnesCount8(byte(param))
			if len(data) != 2+n*32 {
				return finsErr(ErrMalformedMessage, "MSG bitmap/32-byte message records")
			}
			if n > 0 && b.MaxRecursionDepth < 5 {
				return finsErr(ErrResourceExceeded, "message shared projection depth")
			}
			if n > (b.MaxCollectionElements-32)/4 {
				return finsErr(ErrResourceExceeded, "message list projection")
			}
			messages := make([]any, 0, n)
			at := 2
			for number := 0; number < 8; number++ {
				if param&(1<<uint(number)) != 0 {
					messages = append(messages, map[string]any{"number": uint64(number), "data_hex": finsHex(data[at : at+32])})
					at += 32
				}
			}
			out["message_mask"], out["messages"] = uint64(param), messages
		}
		f["message_data"] = out
	case 0x2102:
		out := map[string]any{"request_context_observed": linked, "device_history_verified": false, "partial_range_error_observed": code == 0x1104}
		begin, requested := 0, 0
		if linked {
			begin, requested = int(finsBE(p.wire[12:14])), int(finsBE(p.wire[14:16]))
			out["request_beginning_record"], out["request_record_count"] = uint64(begin), uint64(requested)
		}
		if len(data) == 0 {
			out["layout"] = "empty"
			f["error_log"] = out
			return nil
		}
		if len(data) < 6 {
			return finsErr(ErrMalformedMessage, "short error log counters")
		}
		maximum, stored, n := int(finsBE(data[:2])), int(finsBE(data[2:4])), int(finsBE(data[4:6]))
		if maximum != 20 || stored > 20 || n > stored || len(data) != 6+n*10 {
			return finsErr(ErrMalformedMessage, "error log capacity/count/record length")
		}
		if linked {
			// W342 printed209: overflow returns every available record from the
			// observed beginning, in order. A short response cannot silently omit
			// records, or claim overflow when the requested range fits.
			available := stored - begin
			if available < 0 {
				available = 0
			}
			if n > requested || code == 0 && n != requested || code == 0x1104 && (n >= requested || n != available) || n > 0 && begin+n > stored {
				return finsErr(ErrMalformedMessage, "error log return differs from observed record range/count")
			}
		}
		if n > 0 && b.MaxRecursionDepth < 6 {
			return finsErr(ErrResourceExceeded, "error log shared projection depth")
		}
		if n > (b.MaxCollectionElements-32)/16 {
			return finsErr(ErrResourceExceeded, "error log record projection")
		}
		records := make([]any, 0, n)
		for i := 0; i < n; i++ {
			raw := data[6+i*10 : 6+(i+1)*10]
			calendar, err := finsReportedCalendar([]byte{raw[8], raw[9], raw[6], raw[7], raw[4], raw[5]})
			if err != nil {
				return err
			}
			record := map[string]any{"error_code1": finsBE(raw[:2]), "error_code2": finsBE(raw[2:4]), "reported_time_hex": finsHex(raw[4:]), "reported_time": calendar}
			if linked {
				record["request_based_record_number"] = uint64(begin + i)
			}
			records = append(records, record)
		}
		out["layout"], out["maximum_records"], out["stored_records"], out["returned_records"], out["records"] = "records", uint64(maximum), uint64(stored), uint64(n), records
		f["error_log"] = out
	}
	return nil
}
