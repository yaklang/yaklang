package pcaputil

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// This profile describes observed Semtech packet-forwarder v2 datagrams. The
// gateway identifier and random token do not authenticate or correlate peers.
// RF payloads remain opaque; LoRaWAN PHY/MAC and downstream transmission are
// outside this profile.
func validSemtechDatagram(w []byte) bool {
	_, err := decodeSemtechDatagram(w, DefaultParserBudget().MaxCollectionElements)
	return err == nil
}

func semtechMalformed(s string) error { return protocolError(ErrMalformedMessage, "Semtech %s", s) }

func decodeSemtechDatagram(w []byte, maxElements int) (map[string]any, error) {
	if len(w) < 4 || w[0] != 2 {
		return nil, semtechMalformed("version/header is invalid")
	}
	if len(w) > 65507 {
		return nil, protocolError(ErrResourceExceeded, "Semtech datagram exceeds UDP payload limit")
	}
	f := map[string]any{"Protocol Version": 2, "Token": hex.EncodeToString(w[1:3]), "Identifier": int(w[3]), "Context Level": "observed"}
	switch w[3] {
	case 1, 4:
		if len(w) != 4 {
			return nil, semtechMalformed("ACK must be exactly four bytes")
		}
		if w[3] == 1 {
			f["Packet Name"] = "PUSH_ACK"
		} else {
			f["Packet Name"] = "PULL_ACK"
		}
		return f, nil
	case 2:
		if len(w) != 12 {
			return nil, semtechMalformed("PULL_DATA must be exactly twelve bytes")
		}
		f["Packet Name"], f["Gateway EUI"] = "PULL_DATA", hex.EncodeToString(w[4:12])
		return f, nil
	case 0:
		if len(w) <= 12 {
			return nil, semtechMalformed("PUSH_DATA has no gateway/JSON body")
		}
		f["Packet Name"], f["Gateway EUI"] = "PUSH_DATA", hex.EncodeToString(w[4:12])
	case 3, 5:
		return nil, protocolError(ErrUnsupportedFeature, "Semtech downstream is outside upstream datagram profile")
	default:
		return nil, semtechMalformed("identifier is invalid")
	}
	root, err := semtechJSON(w[12:], maxElements)
	if err != nil {
		return nil, err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, semtechMalformed("JSON root must be an object")
	}
	if _, rx := obj["rxpk"]; !rx {
		if _, stat := obj["stat"]; !stat {
			return nil, semtechMalformed("PUSH_DATA requires rxpk or stat")
		}
	}
	for k := range obj {
		if k != "rxpk" && k != "stat" {
			return nil, protocolError(ErrUnsupportedFeature, "Semtech unknown root field %s", k)
		}
	}
	if value, found := obj["rxpk"]; found {
		array, ok := value.([]any)
		if !ok || len(array) == 0 {
			return nil, semtechMalformed("rxpk must be a nonempty array")
		}
		packets := make([]map[string]any, 0, len(array))
		for _, v := range array {
			p, ok := v.(map[string]any)
			if !ok {
				return nil, semtechMalformed("rxpk entry must be an object")
			}
			decoded, e := semtechRX(p)
			if e != nil {
				return nil, e
			}
			packets = append(packets, decoded)
		}
		f["RF Packets"], f["RF Packet Count"] = packets, len(packets)
	}
	if value, found := obj["stat"]; found {
		p, ok := value.(map[string]any)
		if !ok {
			return nil, semtechMalformed("stat must be an object")
		}
		decoded, e := semtechStat(p)
		if e != nil {
			return nil, e
		}
		f["Gateway Statistics"] = decoded
	}
	return f, nil
}

// Decode while charging every object member/array entry before retention, and
// rejecting duplicates at every level. The UDP byte bound caps token/string
// allocations; depth is a separate fixed bound for this nonrecursive profile.
func semtechJSON(w []byte, maxElements int) (any, error) {
	if !utf8.Valid(w) {
		return nil, semtechMalformed("JSON is not UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(w))
	d.UseNumber()
	count := 0
	var value func(int) (any, error)
	value = func(depth int) (any, error) {
		if depth > 8 {
			return nil, protocolError(ErrResourceExceeded, "Semtech JSON nesting exceeds profile bound")
		}
		t, err := d.Token()
		if err != nil {
			return nil, semtechMalformed("JSON syntax is invalid")
		}
		delim, collection := t.(json.Delim)
		if !collection {
			return t, nil
		}
		if delim != '{' && delim != '[' {
			return nil, semtechMalformed("JSON delimiter is invalid")
		}
		obj := map[string]any{}
		array := []any{}
		for d.More() {
			count++
			if count > maxElements {
				return nil, protocolError(ErrResourceExceeded, "Semtech JSON elements exceed budget")
			}
			key := ""
			if delim == '{' {
				k, e := d.Token()
				if e != nil {
					return nil, semtechMalformed("JSON key is invalid")
				}
				var ok bool
				key, ok = k.(string)
				if !ok {
					return nil, semtechMalformed("JSON key is not a string")
				}
				if _, duplicate := obj[key]; duplicate {
					return nil, semtechMalformed("duplicate JSON key")
				}
			}
			v, e := value(depth + 1)
			if e != nil {
				return nil, e
			}
			if delim == '{' {
				obj[key] = v
			} else {
				array = append(array, v)
			}
		}
		closing, e := d.Token()
		if e != nil || delim == '{' && closing != json.Delim('}') || delim == '[' && closing != json.Delim(']') {
			return nil, semtechMalformed("JSON closing delimiter is invalid")
		}
		if delim == '{' {
			return obj, nil
		}
		return array, nil
	}
	v, err := value(1)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, semtechMalformed("JSON has trailing data")
	}
	return v, nil
}

func semtechNumber(v any, low, high float64, integer bool) (float64, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, semtechMalformed("numeric field has wrong type")
	}
	x, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsNaN(x) || math.IsInf(x, 0) || x < low || x > high || integer && !semtechExactInteger(string(n)) {
		return 0, semtechMalformed("numeric field is outside range")
	}
	return x, nil
}

// JSON numbers can use fractions and exponents. Validate integrality from the
// decimal token before float64 rounding can erase a nonzero fractional tail.
// Counting digits avoids constructing a potentially huge power of ten.
func semtechExactInteger(s string) bool {
	exponentText := ""
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		exponentText, s = s[at+1:], s[:at]
	}
	zero := true
	for _, c := range s {
		if c >= '1' && c <= '9' {
			zero = false
			break
		}
	}
	if zero {
		return true // zero is integral regardless of the exponent magnitude
	}
	var exponent int64
	if exponentText != "" {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 32)
		if err != nil {
			return false
		}
	}
	fractionDigits := 0
	if at := strings.IndexByte(s, '.'); at >= 0 {
		fractionDigits = len(s) - at - 1
	}
	trailingZeros := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case '0':
			trailingZeros++
		case '.', '-':
			continue
		default:
			return exponent >= int64(fractionDigits-trailingZeros)
		}
	}
	return true // every digit in the coefficient is zero
}

func semtechFields(p map[string]any, specs map[string][3]float64) (map[string]any, error) {
	f := map[string]any{}
	for key, rangeSpec := range specs {
		if v, found := p[key]; found {
			x, err := semtechNumber(v, rangeSpec[0], rangeSpec[1], rangeSpec[2] != 0)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			f[key] = x
		}
	}
	return f, nil
}

var semtechLoRaRate = regexp.MustCompile(`^SF(?:[7-9]|1[0-2])BW(?:125|250|500)$`)

func semtechRX(p map[string]any) (map[string]any, error) {
	spec := map[string][3]float64{"tmst": {0, 4294967295, 1}, "tmms": {0, 9007199254740991, 1}, "freq": {math.SmallestNonzeroFloat64, math.MaxFloat64, 0}, "chan": {0, 4294967295, 1}, "rfch": {0, 4294967295, 1}, "stat": {-1, 1, 1}, "rssi": {-2147483648, 2147483647, 1}, "lsnr": {-math.MaxFloat64, math.MaxFloat64, 0}, "size": {0, 65535, 1}}
	f, err := semtechFields(p, spec)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"tmst", "freq", "chan", "rfch", "stat", "rssi", "size", "modu", "datr", "data"} {
		if _, ok := p[key]; !ok {
			return nil, semtechMalformed("rxpk missing " + key)
		}
	}
	modu, ok := p["modu"].(string)
	if !ok {
		return nil, semtechMalformed("modu must be a string")
	}
	switch modu {
	case "LORA":
		r, ok := p["datr"].(string)
		if !ok || !semtechLoRaRate.MatchString(r) {
			return nil, semtechMalformed("LoRa datr is invalid")
		}
		f["datr"] = r
		codr, ok := p["codr"].(string)
		if !ok || codr != "4/5" && codr != "4/6" && codr != "4/7" && codr != "4/8" {
			return nil, semtechMalformed("LoRa codr is invalid")
		}
		f["codr"] = codr
		if _, ok := p["lsnr"]; !ok {
			return nil, semtechMalformed("LoRa lsnr is missing")
		}
	case "FSK":
		x, e := semtechNumber(p["datr"], 1, 4294967295, true)
		if e != nil {
			return nil, e
		}
		f["datr"] = x
		if _, found := p["codr"]; found {
			return nil, semtechMalformed("FSK codr is not applicable")
		}
		if _, found := p["lsnr"]; found {
			return nil, semtechMalformed("FSK lsnr is not applicable")
		}
	default:
		return nil, protocolError(ErrUnsupportedFeature, "Semtech modulation outside LoRa/FSK")
	}
	f["modu"] = modu
	if v, found := p["time"]; found {
		s, ok := v.(string)
		if !ok {
			return nil, semtechMalformed("RX time must be a string")
		}
		if _, e := time.Parse(time.RFC3339Nano, s); e != nil {
			return nil, semtechMalformed("RX UTC time is invalid")
		}
		f["time"] = s
	}
	s, ok := p["data"].(string)
	if !ok {
		return nil, semtechMalformed("data must be a base64 string")
	}
	for _, b := range []byte(s) {
		if b == '\r' || b == '\n' {
			return nil, semtechMalformed("base64 data contains line breaks")
		}
	}
	data, e := base64.StdEncoding.Strict().DecodeString(s)
	if e != nil || float64(len(data)) != f["size"].(float64) {
		return nil, semtechMalformed("base64 payload does not match size")
	}
	f["data"], f["Payload"] = s, data
	for k := range p {
		if _, known := spec[k]; !known && k != "modu" && k != "datr" && k != "codr" && k != "time" && k != "data" {
			return nil, protocolError(ErrUnsupportedFeature, "Semtech rxpk field outside profile: %s", k)
		}
	}
	return f, nil
}

func semtechStat(p map[string]any) (map[string]any, error) {
	spec := map[string][3]float64{"lati": {-90, 90, 0}, "long": {-180, 180, 0}, "alti": {-2147483648, 2147483647, 1}, "rxnb": {0, 4294967295, 1}, "rxok": {0, 4294967295, 1}, "rxfw": {0, 4294967295, 1}, "ackr": {0, 100, 0}, "dwnb": {0, 4294967295, 1}, "txnb": {0, 4294967295, 1}}
	f, err := semtechFields(p, spec)
	if err != nil {
		return nil, err
	}
	for _, k := range []string{"time", "rxnb", "rxok", "rxfw", "ackr", "dwnb", "txnb"} {
		if _, ok := p[k]; !ok {
			return nil, semtechMalformed("stat missing " + k)
		}
	}
	s, ok := p["time"].(string)
	if !ok {
		return nil, semtechMalformed("stat time must be a string")
	}
	if _, e := time.Parse("2006-01-02 15:04:05 GMT", s); e != nil {
		return nil, semtechMalformed("stat time must be expanded UTC")
	}
	f["time"] = s
	for k := range p {
		if _, known := spec[k]; !known && k != "time" {
			return nil, protocolError(ErrUnsupportedFeature, "Semtech stat field outside profile: %s", k)
		}
	}
	return f, nil
}
