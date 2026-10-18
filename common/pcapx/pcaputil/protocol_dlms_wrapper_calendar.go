package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Calendar Data is a wire-component observation, not a UTC instant or evidence
// of a meter clock's accuracy. Keep unspecified/special fields and signed
// deviation without applying host timezone rules or normalizing impossible dates.
func wrapperCalendar(w []byte) (map[string]any, error) {
	tag, v := w[0], w[1:]
	out := map[string]any{"type": tag, "raw_hex": hex.EncodeToString(w), "value_hex": hex.EncodeToString(v), "utc_instant_verified": false}
	field := func(name string, raw int, low, high, unspecified int, specials map[int]string) error {
		state := "specified"
		if raw == unspecified {
			state = "unspecified"
		} else if special, ok := specials[raw]; ok {
			state = special
		} else if raw < low || raw > high {
			// A selected profile refusal, not a universal invalid-wire assertion: some
			// reference readers normalize out-of-range values to skipped components.
			return wrapperError(ErrUnsupportedFeature, "calendar component outside selected literal/unspecified/special profile")
		}
		out[name], out[name+"_state"] = raw, state
		return nil
	}
	if tag == 25 || tag == 26 {
		year, month, day, dow := int(binary.BigEndian.Uint16(v)), int(v[2]), int(v[3]), int(v[4])
		if err := field("year", year, 1, 9999, 65535, nil); err != nil {
			return nil, err
		}
		if err := field("month", month, 1, 12, 255, map[int]string{254: "daylight-saving-begin", 253: "daylight-saving-end"}); err != nil {
			return nil, err
		}
		if err := field("day_of_month", day, 1, 31, 255, map[int]string{254: "last-day", 253: "second-last-day"}); err != nil {
			return nil, err
		}
		if err := field("day_of_week", dow, 1, 7, 255, nil); err != nil {
			return nil, err
		}
		out["calendar_validity"] = "not-concrete"
		if year != 65535 && month <= 12 && day <= 31 {
			days := 31
			switch month {
			case 4, 6, 9, 11:
				days = 30
			case 2:
				days = 28
				if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
					days = 29
				}
			}
			if day > days {
				return nil, wrapperError(ErrUnsupportedFeature, "literal calendar date outside selected Gregorian profile")
			}
			out["calendar_validity"] = "concrete-valid"
		}
		// The transmitted weekday is retained independently; no object-level claim
		// that it agrees with the date or represents a configured recurrence rule.
		if tag == 25 {
			v = v[5:]
		}
	}
	if tag == 25 || tag == 27 {
		for i, name := range []string{"hour", "minute", "second", "hundredths"} {
			high := 59
			if i == 0 {
				high = 23
			}
			if i == 3 {
				high = 99
			}
			if err := field(name, int(v[i]), 0, high, 255, nil); err != nil {
				return nil, err
			}
		}
	}
	if tag == 25 {
		deviation := int16(binary.BigEndian.Uint16(v[4:6]))
		out["deviation_minutes_raw"] = deviation
		out["deviation_state"] = "specified"
		if deviation == -32768 {
			out["deviation_state"] = "unspecified"
		}
		out["clock_status_raw"] = v[6]
		out["clock_status_state"] = "specified"
		if v[6] == 255 {
			out["clock_status_state"] = "unspecified"
		}
		// Preserve all status/reserved bits. They do not prove that a clock is valid
		// or that deviation can be applied to an observed UTC time.
	}
	return out, nil
}
