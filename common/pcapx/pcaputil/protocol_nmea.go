package pcaputil

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// GGA and RMC are observed navigation statements. Checksums validate their
// wire representation, not the sender's identity or the accuracy of a fix.
// AIS, proprietary sentences, tag blocks and other sentence types are outside
// this profile. A datagram must contain complete independent CRLF sentences.
func validNMEADatagram(w []byte) bool {
	_, err := decodeNMEADatagram(w, DefaultParserBudget().MaxCollectionElements)
	return err == nil
}

func decodeNMEADatagram(w []byte, maxElements int) (map[string]any, error) {
	if len(w) == 0 {
		return nil, protocolError(ErrMalformedMessage, "NMEA datagram is empty")
	}
	var messages []map[string]any
	for len(w) > 0 {
		if len(messages) >= maxElements {
			return nil, protocolError(ErrResourceExceeded, "NMEA sentence count exceeds budget")
		}
		i := bytes.IndexByte(w, '\n')
		if i < 0 {
			return nil, protocolError(ErrMalformedMessage, "NMEA datagram ends within a sentence")
		}
		raw := w[:i+1]
		w = w[i+1:]
		if len(raw) > 82 || len(raw) < 10 || raw[len(raw)-2] != '\r' || raw[0] != '$' {
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
		switch talker {
		case "GP", "GN", "GL", "GA", "GB", "BD", "GQ", "GI":
		default:
			return nil, protocolError(ErrUnsupportedFeature, "NMEA talker is outside the GNSS profile")
		}
		fields := map[string]any{"Packet Name": kind, "Sentence ID": parts[0], "Talker": talker, "Checksum": int(checksum), "Checksum Valid": true, "Context Level": "observed"}
		var err error
		switch kind {
		case "GGA":
			err = nmeaGGA(parts, fields)
		case "RMC":
			err = nmeaRMC(parts, fields)
		default:
			err = protocolError(ErrUnsupportedFeature, "NMEA sentence type is outside GGA/RMC profile")
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
