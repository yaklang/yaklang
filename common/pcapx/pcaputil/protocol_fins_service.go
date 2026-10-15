package pcaputil

import "encoding/binary"

// W342-E1-18 printed194-198: selected CPU status, cycle and clock fields.
// Reported device values never drive capture time, authentication or execution.
func finsServiceCommand(command uint16) bool {
	return command == 0x0601 || command == 0x0620 || command == 0x0701
}
func finsServiceRequest(command uint16, w []byte, fields map[string]any) error {
	switch command {
	case 0x0601, 0x0701:
		if len(w) != 12 {
			return finsErr(ErrMalformedMessage, "status/clock request has no data")
		}
	case 0x0620:
		if len(w) != 13 {
			return finsErr(ErrMalformedMessage, "cycle request needs one operation byte")
		}
		if w[12] > 1 {
			return finsErr(ErrUnsupportedFeature, "selected cycle initialize00/read01 only")
		}
		fields["cycle_operation"] = uint64(w[12])
	}
	return nil
}
func finsServiceResponse(command uint16, data []byte, p *finsPending, linked bool, fields map[string]any, b ParserBudget) error {
	if b.MaxCollectionElements < 64 {
		return finsErr(ErrResourceExceeded, "status/cycle/clock shared field projection count")
	}
	switch command {
	case 0x0601:
		if len(data) != 26 {
			return finsErr(ErrMalformedMessage, "CPU status response needs26 data bytes")
		}
		modes := map[byte]string{0: "PROGRAM", 2: "MONITOR", 4: "RUN"}
		mode, ok := modes[data[1]]
		if !ok {
			return finsErr(ErrUnsupportedFeature, "selected PROGRAM/MONITOR/RUN CPU mode")
		}
		if data[6] != 0 {
			return finsErr(ErrMalformedMessage, "MSG bitmap leading byte must be zero")
		}
		fields["cpu_status"] = map[string]any{
			"status_raw":                   uint64(data[0]),
			"status_defined_bits":          uint64(data[0] & 0x87),
			"status_undefined_bits":        uint64(data[0] & 0x78),
			"reported_running":             data[0]&1 != 0,
			"reported_flash_write":         data[0]&2 != 0,
			"reported_battery_present":     data[0]&4 != 0,
			"reported_standby":             data[0]&0x80 != 0,
			"mode_raw":                     uint64(data[1]),
			"reported_mode":                mode,
			"fatal_error_data":             finsBE(data[2:4]),
			"nonfatal_error_data":          finsBE(data[4:6]),
			"message_bitmap_raw":           finsBE(data[6:8]),
			"message_bitmap_defined_bits":  uint64(data[7]),
			"message_bitmap_reserved_byte": uint64(data[6]),
			"error_code":                   finsBE(data[8:10]),
			"error_message_hex":            finsHex(data[10:]),
			"device_execution_verified":    false,
		}
	case 0x0620:
		if len(data) != 0 && len(data) != 12 {
			return finsErr(ErrMalformedMessage, "cycle response needs0/12 data bytes")
		}
		if linked {
			op := p.wire[12]
			if op == 0 && len(data) != 0 || op == 1 && len(data) != 12 {
				return finsErr(ErrMalformedMessage, "cycle response differs from observed operation")
			}
			fields["cycle_operation"] = uint64(op)
		}
		observed := map[string]any{
			"request_operation_observed": linked,
			"response_layout":            "empty",
			"device_execution_verified":  false,
			"measurement_truth_verified": false,
		}
		if len(data) == 12 {
			observed["response_layout"] = "values"
			observed["average_ticks_100us"] = uint64(binary.BigEndian.Uint32(data[:4]))
			observed["maximum_ticks_100us"] = uint64(binary.BigEndian.Uint32(data[4:8]))
			observed["minimum_ticks_100us"] = uint64(binary.BigEndian.Uint32(data[8:12]))
		}
		fields["cycle_time"] = observed
	case 0x0701:
		if len(data) != 7 {
			return finsErr(ErrMalformedMessage, "clock response needs7 data bytes")
		}
		if data[6] > 6 {
			return finsErr(ErrMalformedMessage, "clock field range")
		}
		calendar, err := finsReportedCalendar(data[:6])
		if err != nil {
			return err
		}
		calendar["weekday_raw"] = uint64(data[6])
		fields["clock"] = calendar
	}
	return nil
}
