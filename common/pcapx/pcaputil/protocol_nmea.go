package pcaputil

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// GGA, RMC, unit-labelled VTG, fixed-slot GSA and per-sentence GSV are observed navigation statements. Checksums validate their
// wire representation, not the sender's identity or the accuracy of a fix.
// Single-fragment AIS Class A and Type18 Class B position reports are also
// observed statements; flags and radio status do not prove a device capability.
// Proprietary sentences and tag blocks are unsupported. A datagram must contain
// complete CRLF sentences; selected Type5 pairs may be assembled only when
// both ordered fragments are fully contained in this datagram. No association
// between datagrams is inferred.
func validNMEADatagram(w []byte) bool {
	_, err := decodeNMEADatagram(w, DefaultParserBudget().MaxCollectionElements)
	return err == nil
}

func decodeNMEADatagram(w []byte, maxElements int) (map[string]any, error) {
	return decodeNMEADatagramBudget(w, maxElements, DefaultParserBudget().MaxRecursionDepth)
}

func decodeNMEADatagramBudget(w []byte, maxElements, maxDepth int) (map[string]any, error) {
	if nmeaAIS5Candidate(w) {
		return nmeaAISCompleteType5(w, maxElements, maxDepth)
	}
	if len(w) == 0 {
		return nil, protocolError(ErrMalformedMessage, "NMEA datagram is empty")
	}
	var messages []map[string]any
	for len(w) > 0 {
		if len(messages) >= maxElements {
			return nil, protocolError(ErrResourceExceeded, "NMEA sentence count exceeds budget")
		}
		// A single sentence is a flat field map. Multiple sentences add the
		// outer Messages projection; reject before allocating a second map.
		if maxDepth < 1 || len(messages) > 0 && maxDepth < 2 {
			return nil, protocolError(ErrResourceExceeded, "NMEA sentence projection exceeds depth budget")
		}
		i := bytes.IndexByte(w, '\n')
		if i < 0 {
			return nil, protocolError(ErrMalformedMessage, "NMEA datagram ends within a sentence")
		}
		raw := w[:i+1]
		w = w[i+1:]
		if len(raw) > 82 || len(raw) < 10 || raw[len(raw)-2] != '\r' || raw[0] != '$' && raw[0] != '!' {
			return nil, protocolError(ErrMalformedMessage, "NMEA sentence length/start/termination is invalid")
		}
		body := raw[1 : len(raw)-2]
		star := len(body) - 3
		if star < 1 || body[star] != '*' || bytes.IndexByte(body[:star], '*') >= 0 {
			return nil, protocolError(ErrMalformedMessage, "NMEA checksum suffix is invalid")
		}
		for _, b := range body {
			if b < 32 || b > 126 {
				return nil, protocolError(ErrMalformedMessage, "NMEA sentence is not printable ASCII")
			}
		}
		hi, lo := nmeaHex(body[star+1]), nmeaHex(body[star+2])
		if hi < 0 || lo < 0 {
			return nil, protocolError(ErrMalformedMessage, "NMEA checksum is not hexadecimal")
		}
		var checksum byte
		for _, b := range body[:star] {
			checksum ^= b
		}
		if checksum != byte(hi*16+lo) {
			return nil, protocolError(ErrMalformedMessage, "NMEA XOR checksum mismatch")
		}
		parts := strings.Split(string(body[:star]), ",")
		if len(parts[0]) != 5 {
			return nil, protocolError(ErrUnsupportedFeature, "NMEA sentence identifier is unsupported")
		}
		talker, kind := parts[0][:2], parts[0][2:]
		if raw[0] == '!' {
			if talker != "AI" || kind != "VDM" && kind != "VDO" {
				return nil, protocolError(ErrUnsupportedFeature, "NMEA encapsulation is outside AIVDM/AIVDO profile")
			}
		} else {
			switch talker {
			case "GP", "GN", "GL", "GA", "GB", "BD", "GQ", "GI":
			default:
				return nil, protocolError(ErrUnsupportedFeature, "NMEA talker is outside the GNSS profile")
			}
		}
		fields := map[string]any{"Packet Name": kind, "Sentence ID": parts[0], "Talker": talker, "Checksum": int(checksum), "Checksum Valid": true, "Context Level": "observed"}
		var err error
		switch kind {
		case "VDM", "VDO":
			err = nmeaAIS(parts, fields)
		case "GGA":
			err = nmeaGGA(parts, fields)
		case "RMC":
			err = nmeaRMC(parts, fields)
		case "VTG":
			err = nmeaVTG(parts, fields)
		case "GSA":
			err = nmeaGSA(parts, fields)
		case "GSV":
			err = nmeaGSV(parts, fields, maxElements)
		default:
			err = protocolError(ErrUnsupportedFeature, "NMEA sentence type is outside GGA/RMC/VTG/GSA/GSV profile")
		}
		if err != nil {
			return nil, err
		}
		messages = append(messages, fields)
	}
	if len(messages) == 1 {
		messages[0]["Message Count"] = 1
		return messages[0], nil
	}
	return map[string]any{"Packet Name": "NMEA 0183 sentences", "Message Count": len(messages), "Messages": messages, "Context Level": "observed"}, nil
}

// GSV preserves a single reported picture's slots, including missing values,
// duplicates and sparse nonfinal parts. Some receivers number mixed talkers
// in one picture; neither the talker nor numSV establishes a constellation,
// complete cycle, tracked satellite or relationship with GSA. Signal ID is
// hexadecimal in the selected 4.10 form. Signal level has no inferred unit:
// receiver documentation calls it either SNR or C/No.
func nmeaGSV(p []string, out map[string]any, maxElements int) error {
	if len(p) < 4 {
		return protocolError(ErrMalformedMessage, "NMEA GSV header is truncated")
	}
	slots, extra := (len(p)-4)/4, (len(p)-4)%4
	if slots > 4 || extra == 2 {
		return protocolError(ErrUnsupportedFeature, "NMEA GSV requires at most four slots and optional Signal ID")
	}
	if extra == 3 {
		return protocolError(ErrMalformedMessage, "NMEA GSV satellite slot is truncated")
	}
	if len(p[1]) != 1 || p[1][0] < '1' || p[1][0] > '9' || len(p[2]) != 1 || p[2][0] < '1' || p[2][0] > p[1][0] {
		return protocolError(ErrMalformedMessage, "NMEA GSV message count/index must be 1..9 and index cannot exceed count")
	}
	visible, err := nmeaInteger(p[3], 999)
	if err != nil || len(p[3]) > 3 {
		return protocolError(ErrMalformedMessage, "NMEA GSV reported count must be one-to-three decimal digits")
	}
	if slots > maxElements {
		return protocolError(ErrResourceExceeded, "NMEA GSV satellite slots exceed collection budget")
	}
	out["Cycle Context"] = "unassembled"
	out["Cycle Message Count"], out["Cycle Message Number"] = int(p[1][0]-'0'), int(p[2][0]-'0')
	out["Reported Satellites In View"], out["Reported Satellites In View Wire"] = visible, p[3]
	out["Satellite Slot Count"], out["Signal ID Present"] = slots, extra == 1
	for i := 0; i < slots; i++ {
		for j, field := range []struct {
			key    string
			max    int
			digits int
		}{{"SV ID", 999, 3}, {"Elevation Degrees", 90, 2}, {"Azimuth Degrees", 359, 3}, {"Signal Level", 99, 2}} {
			key, value := fmt.Sprintf("Satellite Slot %d %s", i+1, field.key), p[4+i*4+j]
			out[key+" Present"], out[key+" Wire"] = value != "", value
			if value == "" {
				continue
			}
			number := value
			negative := j == 1 && value[0] == '-'
			if negative {
				number = value[1:]
			}
			n, err := nmeaInteger(number, field.max)
			if err != nil || len(number) > field.digits {
				return protocolError(ErrMalformedMessage, "NMEA GSV %s is outside selected integer range", field.key)
			}
			if j == 1 {
				x := int(n)
				if negative {
					x = -x
				}
				out[key] = x
			} else {
				out[key] = n
			}
		}
	}
	if extra == 1 {
		value := p[len(p)-1]
		out["Signal ID Wire"] = value
		if len(value) > 2 {
			return protocolError(ErrUnsupportedFeature, "NMEA GSV Signal ID exceeds selected two-hex-digit profile")
		}
		if value != "" {
			n := 0
			for _, b := range []byte(value) {
				h := nmeaHex(b)
				if h < 0 {
					return protocolError(ErrMalformedMessage, "NMEA GSV Signal ID must be hexadecimal")
				}
				n = n*16 + h
			}
			out["Signal ID"] = n
		}
	}
	return nil
}

// GSA observes exactly twelve PRN slots, preserving their spelling, order and
// duplicates. The count is populated slots, not distinct satellites or a
// verified fix. System ID is literal: Trimble's QZSS value0 differs from the
// common value5, and talkers/PRNs alone cannot establish receiver context.
// Reported DOP sentinels remain values, without promoting them to valid quality.
func nmeaGSA(p []string, out map[string]any) error {
	if len(p) < 18 {
		return protocolError(ErrMalformedMessage, "NMEA GSA fields are truncated")
	}
	if len(p) != 18 && len(p) != 19 {
		return protocolError(ErrUnsupportedFeature, "NMEA GSA requires twelve PRN slots with optional System ID")
	}
	if p[1] != "A" && p[1] != "M" {
		return protocolError(ErrMalformedMessage, "NMEA GSA selection mode must be A or M")
	}
	if p[2] == "E" {
		return protocolError(ErrUnsupportedFeature, "NMEA GSA vendor dead-reckoning fix is unsupported")
	}
	if p[2] != "1" && p[2] != "2" && p[2] != "3" {
		return protocolError(ErrMalformedMessage, "NMEA GSA reported fix type must be 1, 2 or 3")
	}
	out["Selection Mode"], out["Reported Fix Type"] = p[1], int(p[2][0]-'0')
	populated := 0
	for i := 0; i < 12; i++ {
		key, value := fmt.Sprintf("Satellite Slot %d PRN", i+1), p[i+3]
		out[key+" Present"], out[key+" Wire"] = value != "", value
		if value == "" {
			continue
		}
		n, err := nmeaInteger(value, 999)
		if err != nil || len(value) > 3 || n == 0 {
			return protocolError(ErrMalformedMessage, "NMEA GSA PRN slot must be an unsigned one-to-three-digit nonzero value")
		}
		out[key] = n
		populated++
	}
	out["Reported Satellite Slots"] = populated
	for _, field := range []struct {
		index int
		key   string
	}{{15, "PDOP"}, {16, "HDOP"}, {17, "VDOP"}} {
		value := p[field.index]
		out[field.key+" Present"], out[field.key+" Wire"] = value != "", value
		if value == "" {
			continue
		}
		x, err := nmeaNumber(value, false)
		if err != nil {
			return protocolError(ErrMalformedMessage, "NMEA GSA %s must be a finite unsigned decimal", field.key)
		}
		out[field.key] = x
	}
	out["System ID Present"] = len(p) == 19
	if len(p) == 19 {
		value := p[18]
		out["System ID Wire"] = value
		if value != "" {
			if len(value) != 1 || value[0] < '0' || value[0] > '6' {
				return protocolError(ErrUnsupportedFeature, "NMEA GSA System ID is outside selected literal0..6 profile")
			}
			out["System ID"] = int(value[0] - '0')
		}
	}
	return nil
}

// VTG's unit-labelled form has eight fields, optionally followed by the FAA
// mode. Observe both reported speeds independently: rounded knots and km/h
// need not have exact conversion equality. Empty fields and mode N never
// manufacture a course, speed or fix. The older unlabelled four-value form
// and later mode extensions require their own explicit reference profiles.
func nmeaVTG(p []string, out map[string]any) error {
	if len(p) == 5 {
		return protocolError(ErrUnsupportedFeature, "NMEA VTG unlabelled legacy form is unsupported")
	}
	if len(p) < 9 {
		return protocolError(ErrMalformedMessage, "NMEA VTG fields are truncated")
	}
	if len(p) != 9 && len(p) != 10 {
		return protocolError(ErrUnsupportedFeature, "NMEA VTG requires unit-labelled fields with optional mode")
	}
	for _, unit := range []struct {
		index int
		value string
	}{{2, "T"}, {4, "M"}, {6, "N"}, {8, "K"}} {
		if p[unit.index] != unit.value {
			return protocolError(ErrMalformedMessage, "NMEA VTG unit must be %s", unit.value)
		}
	}
	for _, field := range []struct {
		index  int
		key    string
		course bool
	}{
		{1, "Course Degrees True", true}, {3, "Course Degrees Magnetic", true},
		{5, "Speed Knots", false}, {7, "Speed Kilometers Per Hour", false},
	} {
		out[field.key+" Present"] = p[field.index] != ""
		if p[field.index] == "" {
			continue
		}
		x, err := nmeaNumber(p[field.index], false)
		if err != nil || field.course && x >= 360 {
			return protocolError(ErrMalformedMessage, "NMEA VTG %s is invalid", field.key)
		}
		out[field.key] = x
	}
	out["Mode Present"] = len(p) == 10
	if len(p) == 10 {
		mode := p[9]
		if len(mode) > 1 || mode == " " {
			return protocolError(ErrMalformedMessage, "NMEA VTG mode must be a single indicator or empty")
		}
		switch mode {
		case "", "A", "D", "E", "M", "N", "S":
			out["Mode"] = mode
		default:
			return protocolError(ErrUnsupportedFeature, "NMEA VTG mode is outside selected FAA profile")
		}
	}
	return nil
}

func nmeaHex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'A' && b <= 'F':
		return int(b - 'A' + 10)
	case b >= 'a' && b <= 'f':
		return int(b - 'a' + 10)
	}
	return -1
}

func nmeaNumber(s string, signed bool) (float64, error) {
	if len(s) == 0 || len(s) > 24 {
		return 0, fmt.Errorf("nmea: invalid decimal length")
	}
	first := 0
	if signed && s[0] == '-' {
		first = 1
	}
	if first >= len(s) || s[first] < '0' || s[first] > '9' {
		return 0, fmt.Errorf("nmea: decimal requires an integer part")
	}
	digits, dots := 0, 0
	for i, b := range []byte(s) {
		if b >= '0' && b <= '9' {
			digits++
			continue
		}
		if b == '.' && i > 0 && i < len(s)-1 {
			dots++
			continue
		}
		if b == '-' && signed && i == 0 {
			continue
		}
		return 0, fmt.Errorf("nmea: invalid decimal grammar")
	}
	if digits == 0 || dots > 1 {
		return 0, fmt.Errorf("nmea: invalid decimal")
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
		return 0, fmt.Errorf("nmea: invalid finite decimal")
	}
	return x, nil
}

func nmeaInteger(s string, max int) (int, error) {
	if len(s) == 0 || len(s) > 4 {
		return 0, fmt.Errorf("nmea: invalid integer length")
	}
	n := 0
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return 0, fmt.Errorf("nmea: invalid integer")
		}
		n = n*10 + int(b-'0')
	}
	if n > max {
		return 0, fmt.Errorf("nmea: integer out of range")
	}
	return n, nil
}

func nmeaUTC(s string, out map[string]any) error {
	if len(s) < 6 || len(s) > 16 {
		return fmt.Errorf("nmea: invalid UTC field")
	}
	for _, b := range []byte(s[:6]) {
		if b < '0' || b > '9' {
			return fmt.Errorf("nmea: invalid UTC digits")
		}
	}
	if len(s) > 6 {
		if s[6] != '.' || len(s) == 7 {
			return fmt.Errorf("nmea: invalid UTC fraction")
		}
		for _, b := range []byte(s[7:]) {
			if b < '0' || b > '9' {
				return fmt.Errorf("nmea: invalid UTC fraction")
			}
		}
	}
	h, _ := strconv.Atoi(s[:2])
	m, _ := strconv.Atoi(s[2:4])
	sec, _ := strconv.Atoi(s[4:6])
	if h > 23 || m > 59 || sec > 60 || sec == 60 && (h != 23 || m != 59) {
		return fmt.Errorf("nmea: UTC out of range")
	}
	out["UTC"], out["UTC Hour"], out["UTC Minute"], out["UTC Second"] = s, h, m, sec
	if len(s) > 6 {
		out["UTC Fraction"] = s[7:]
	}
	return nil
}

func nmeaCoordinate(s, hemisphere string, latitude bool) (float64, error) {
	degrees := 3
	maximum := 180
	if latitude {
		degrees = 2
		maximum = 90
	}
	if len(s) < degrees+2 {
		return 0, fmt.Errorf("nmea: coordinate is truncated")
	}
	for _, b := range []byte(s[:degrees+2]) {
		if b < '0' || b > '9' {
			return 0, fmt.Errorf("nmea: invalid coordinate digits")
		}
	}
	if len(s) > degrees+2 && s[degrees+2] != '.' {
		return 0, fmt.Errorf("nmea: coordinate precision grammar is invalid")
	}
	d, _ := strconv.Atoi(s[:degrees])
	minutes, err := nmeaNumber(s[degrees:], false)
	if err != nil || minutes >= 60 || d > maximum || d == maximum && minutes != 0 {
		return 0, fmt.Errorf("nmea: coordinate out of range")
	}
	if latitude && hemisphere != "N" && hemisphere != "S" || !latitude && hemisphere != "E" && hemisphere != "W" {
		return 0, fmt.Errorf("nmea: coordinate hemisphere is invalid")
	}
	x := float64(d) + minutes/60
	if hemisphere == "S" || hemisphere == "W" {
		x = -x
	}
	return x, nil
}

func nmeaPosition(lat, ns, lon, ew string, required bool, out map[string]any) error {
	if lat == "" && lon == "" && ns == "" && ew == "" && !required {
		out["Position Present"] = false
		return nil
	}
	x, err := nmeaCoordinate(lat, ns, true)
	if err != nil {
		return err
	}
	y, err := nmeaCoordinate(lon, ew, false)
	if err != nil {
		return err
	}
	out["Latitude"], out["Longitude"], out["Latitude Hemisphere"], out["Longitude Hemisphere"], out["Position Present"] = x, y, ns, ew, true
	return nil
}

func nmeaGGA(p []string, out map[string]any) error {
	if len(p) != 15 {
		return protocolError(ErrMalformedMessage, "NMEA GGA requires all 14 fields")
	}
	quality, err := nmeaInteger(p[6], 8)
	if err != nil {
		return err
	}
	if p[1] == "" && quality == 0 {
		out["UTC Present"] = false
	} else {
		if err = nmeaUTC(p[1], out); err != nil {
			return err
		}
		out["UTC Present"] = true
	}
	if err = nmeaPosition(p[2], p[3], p[4], p[5], quality != 0, out); err != nil {
		return err
	}
	out["Fix Quality"], out["Position Valid"] = quality, quality != 0
	if p[7] != "" {
		n, e := nmeaInteger(p[7], 99)
		if e != nil {
			return e
		}
		out["Satellites Used"] = n
	} else if quality != 0 {
		return fmt.Errorf("nmea: GGA satellite count missing")
	}
	if p[8] != "" {
		x, e := nmeaNumber(p[8], false)
		if e != nil {
			return e
		}
		out["HDOP"] = x
	} else if quality != 0 {
		return fmt.Errorf("nmea: GGA HDOP missing")
	}
	for _, term := range []struct{ value, unit, key string }{{p[9], p[10], "Altitude Meters"}, {p[11], p[12], "Geoid Separation Meters"}} {
		if term.value != "" {
			if term.unit != "M" {
				return fmt.Errorf("nmea: GGA altitude unit must be M")
			}
			x, e := nmeaNumber(term.value, true)
			if e != nil {
				return e
			}
			out[term.key] = x
		} else if term.unit != "" && term.unit != "M" {
			return fmt.Errorf("nmea: invalid empty altitude unit")
		}
	}
	if p[13] != "" {
		x, e := nmeaNumber(p[13], false)
		if e != nil {
			return e
		}
		out["Differential Age Seconds"] = x
	}
	if p[14] != "" {
		n, e := nmeaInteger(p[14], 9999)
		if e != nil {
			return e
		}
		out["Differential Station ID"] = n
	}
	return nil
}

func nmeaRMC(p []string, out map[string]any) error {
	if len(p) < 12 {
		return protocolError(ErrMalformedMessage, "NMEA RMC fields are truncated")
	}
	if len(p) != 12 && len(p) != 13 {
		return protocolError(ErrUnsupportedFeature, "NMEA RMC requires legacy fields with optional mode")
	}
	if p[2] != "A" && p[2] != "V" {
		return fmt.Errorf("nmea: RMC status must be A or V")
	}
	valid := p[2] == "A"
	if p[1] == "" && !valid {
		out["UTC Present"] = false
	} else {
		if err := nmeaUTC(p[1], out); err != nil {
			return err
		}
		out["UTC Present"] = true
	}
	if err := nmeaPosition(p[3], p[4], p[5], p[6], valid, out); err != nil {
		return err
	}
	out["Status"], out["Position Valid"] = p[2], valid
	if p[7] != "" {
		x, e := nmeaNumber(p[7], false)
		if e != nil {
			return e
		}
		out["Speed Knots"] = x
	}
	if p[8] != "" {
		x, e := nmeaNumber(p[8], false)
		if e != nil || x >= 360 {
			return fmt.Errorf("nmea: RMC course out of range")
		}
		out["Course Degrees True"] = x
	}
	if p[9] != "" {
		if len(p[9]) != 6 {
			return fmt.Errorf("nmea: RMC date length is invalid")
		}
		day, e := nmeaInteger(p[9][:2], 31)
		if e != nil {
			return e
		}
		month, e := nmeaInteger(p[9][2:4], 12)
		if e != nil {
			return e
		}
		year, e := nmeaInteger(p[9][4:], 99)
		if e != nil {
			return e
		}
		if day == 0 || month == 0 {
			return fmt.Errorf("nmea: RMC date has zero month/day")
		}
		days := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
		if year%4 == 0 {
			days[1] = 29
		}
		if day > days[month-1] {
			return fmt.Errorf("nmea: RMC date is invalid")
		}
		out["Date"], out["Date Day"], out["Date Month"], out["Date Year Two Digits"] = p[9], day, month, year
	} else if valid {
		return fmt.Errorf("nmea: valid RMC requires date")
	}
	if p[10] != "" {
		x, e := nmeaNumber(p[10], false)
		if e != nil || x > 180 {
			return fmt.Errorf("nmea: RMC magnetic variation out of range")
		}
		if p[11] != "E" && p[11] != "W" {
			return fmt.Errorf("nmea: RMC magnetic direction is invalid")
		}
		if p[11] == "W" {
			x = -x
		}
		out["Magnetic Variation Degrees"], out["Magnetic Variation Direction"] = x, p[11]
	} else if p[11] != "" {
		return fmt.Errorf("nmea: RMC magnetic direction without value")
	}
	if len(p) == 13 {
		// The optional FAA mode field may be transmitted empty. It supplies
		// no additional validity statement in that case; preserve the status.
		if p[12] != "" && (len(p[12]) != 1 || !strings.Contains("ADEFMNPRS", p[12])) {
			return protocolError(ErrUnsupportedFeature, "NMEA RMC mode is unsupported")
		}
		out["Mode"] = p[12]
		if p[12] == "N" {
			out["Position Valid"] = false
		}
	}
	return nil
}
