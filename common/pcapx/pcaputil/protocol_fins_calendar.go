package pcaputil

// Literal two-digit reported calendar fields; no century, epoch or timezone.
func finsReportedCalendar(data []byte) (map[string]any, error) {
	if len(data) != 6 {
		return nil, finsErr(ErrMalformedMessage, "reported calendar needs six BCD fields")
	}
	var v [6]int
	for i, c := range data {
		if c&15 > 9 || c>>4 > 9 {
			return nil, finsErr(ErrMalformedMessage, "clock value is not BCD")
		}
		v[i] = int(c>>4)*10 + int(c&15)
	}
	year, month, day, hour, minute, second := v[0], v[1], v[2], v[3], v[4], v[5]
	if month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59 {
		return nil, finsErr(ErrMalformedMessage, "clock field range")
	}
	days := [12]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	// YY00 may mean leap2000, though1900 would not be leap. Do not pick a century.
	if year%4 == 0 {
		days[1] = 29
	}
	if day > days[month-1] {
		return nil, finsErr(ErrMalformedMessage, "impossible clock calendar fields")
	}
	return map[string]any{"year_two_digits": uint64(year), "month": uint64(month), "day": uint64(day), "hour": uint64(hour), "minute": uint64(minute), "second": uint64(second), "century_observed": false, "timezone_observed": false, "device_clock_verified": false}, nil
}
