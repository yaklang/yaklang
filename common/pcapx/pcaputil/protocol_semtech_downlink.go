package pcaputil

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

const semtechDownlinkProfile = "semtech-v2-downlink-observation"
const semtechDownlinkProjectionBytes int64 = 32 << 10

// Downlink observations follow packet_forwarder v4.0.1 PROTOCOL.TXT 5.4/5.5/6.
// Missing sender defaults, hardware policy and GPS conversion stay unresolved.
// Tokens and EUI bytes neither authenticate peers nor prove RF delivery.
func decodeSemtechDownlink(w []byte, limit int) (map[string]any, error) {
	if len(w) < 4 {
		return nil, semtechMalformed("short packet-forwarder header")
	}
	if w[0] != 2 {
		return nil, protocolError(ErrUnsupportedFeature, "Semtech downlink protocol version2 only")
	}
	if len(w) > 65507 {
		return nil, protocolError(ErrResourceExceeded, "Semtech datagram exceeds UDP payload limit")
	}
	kind := w[3]
	header := 4
	if kind == 5 {
		header = 12
	} else if kind != 3 {
		return nil, protocolError(ErrUnsupportedFeature, "Semtech selected downstream PULL_RESP/TX_ACK only")
	}
	if len(w) < header {
		return nil, semtechMalformed("short TX_ACK EUI header")
	}
	raw := w[header:]
	name := "PULL_RESP"
	var gateway any
	if kind == 5 {
		name = "TX_ACK"
		gateway = hex.EncodeToString(w[4:12])
	}
	f := map[string]any{"version": 2, "token_hex": hex.EncodeToString(w[1:3]), "identifier": int(kind), "kind": name, "gateway_eui_hex": gateway, "json_raw_hex": hex.EncodeToString(raw), "json_present": len(raw) > 0, "response_to": 0, "correlation": "observed-fields only; no transaction inference", "unknown_root": map[string]any{}, "unknown_body": map[string]any{}}
	if kind == 5 && len(raw) == 0 {
		f["json_number_lexemes"] = []map[string]any{}
		f["error"], f["error_origin"], f["feedback_success"], f["rf_delivery_proven"] = "NONE", "empty-body no-error report", true, false
	} else {
		value, err := semtechJSON(raw, min(limit, 256))
		if err != nil {
			return nil, err
		}
		root, ok := value.(map[string]any)
		if !ok {
			return nil, semtechMalformed("JSON root must be object")
		}
		key := "txpk"
		if kind == 5 {
			key = "txpk_ack"
		}
		body, ok := root[key].(map[string]any)
		if !ok {
			return nil, semtechMalformed(key + " must be object")
		}
		unknown := map[string]any{}
		for k, v := range root {
			if k != key {
				unknown[k] = semtechObserved(v)
			}
		}
		f["unknown_root"] = unknown
		f["provided_fields"] = semtechObserved(body)
		f["json_number_lexemes"] = semtechNumberLexemes(raw)
		if kind == 5 {
			unknown = map[string]any{}
			for k, v := range body {
				if k != "error" {
					unknown[k] = semtechObserved(v)
				}
			}
			f["unknown_body"] = unknown
			v, exists := body["error"]
			if !exists {
				return nil, protocolError(ErrUnsupportedFeature, "Semtech missing error default is unspecified")
			}
			s, ok := v.(string)
			if !ok {
				return nil, semtechMalformed("TX_ACK error must be string")
			}
			switch s {
			case "NONE", "TOO_LATE", "TOO_EARLY", "COLLISION_PACKET", "COLLISION_BEACON", "TX_FREQ", "TX_POWER", "GPS_UNLOCKED":
			default:
				return nil, protocolError(ErrUnsupportedFeature, "Semtech unknown TX_ACK error")
			}
			f["error"], f["error_origin"], f["feedback_success"], f["rf_delivery_proven"] = s, "explicit JSON report", s == "NONE", false
		} else if err := semtechTXObservation(body, f); err != nil {
			return nil, err
		}
	}
	fields := map[string]any{"Protocol Version": 2, "Token": hex.EncodeToString(w[1:3]), "Identifier": int(kind), "Context Level": "observed", "Packet Name": name, "Downlink Observation": f}
	if gateway != nil {
		fields["Gateway EUI"] = gateway
	}
	if err := semtechProjectionLimit(fields, limit, 16); err != nil {
		return nil, err
	}
	return fields, nil
}

// JSON numeric lexemes remain exact even beyond float64's integer precision.
// Decimal display is bounded by token length: a huge exponent uses scientific
// notation, never a huge allocation or power of ten.
func semtechDecimal(s string, multiplyMillion bool) string {
	negative := ""
	if strings.HasPrefix(s, "-") {
		negative = "-"
		s = s[1:]
	}
	exponent := int64(0)
	var largeExponent *big.Int
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		text := s[at+1:]
		x, err := strconv.ParseInt(text, 10, 64)
		if err != nil || x > 65507 || x < -65507 {
			largeExponent = new(big.Int)
			largeExponent.SetString(text, 10)
		} else {
			exponent = x
		}
		s = s[:at]
	}
	if at := strings.IndexByte(s, '.'); at >= 0 {
		fraction := int64(len(s) - at - 1)
		if largeExponent != nil {
			largeExponent.Sub(largeExponent, big.NewInt(fraction))
		} else {
			exponent -= fraction
		}
		s = s[:at] + s[at+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	if multiplyMillion {
		if largeExponent != nil {
			largeExponent.Add(largeExponent, big.NewInt(6))
		} else {
			exponent += 6
		}
	}
	if largeExponent != nil {
		largeExponent.Add(largeExponent, big.NewInt(int64(len(s)-1)))
		sign := ""
		if largeExponent.Sign() >= 0 {
			sign = "+"
		}
		tail := ""
		if len(s) > 1 {
			tail = "." + s[1:]
		}
		return negative + s[:1] + tail + "E" + sign + largeExponent.String()
	}
	adjusted := exponent + int64(len(s)) - 1
	if exponent > 0 || adjusted < -6 || exponent < -65507 || exponent > 65507 {
		tail := ""
		if len(s) > 1 {
			tail = "." + s[1:]
		}
		sign := ""
		if adjusted >= 0 {
			sign = "+"
		}
		return negative + s[:1] + tail + "E" + sign + strconv.FormatInt(adjusted, 10)
	}
	point := int64(len(s)) + exponent
	if point <= 0 {
		return negative + "0." + strings.Repeat("0", int(-point)) + s
	}
	if point < int64(len(s)) {
		return negative + s[:point] + "." + s[point:]
	}
	return negative + s
}
func semtechObserved(v any) any {
	switch x := v.(type) {
	case json.Number:
		return map[string]any{"json_number": semtechDecimal(string(x), false)}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, q := range x {
			out[k] = semtechObserved(q)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, q := range x {
			out[i] = semtechObserved(q)
		}
		return out
	default:
		return v
	}
}

// Empty digits denote numeric zero. Overflow exponents retain their sign;
// they are compared without expanding them into big integers or allocations.
func semtechDecimalParts(s string) (digits string, exp int64, negative, overflow bool) {
	negative = strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		v, err := strconv.ParseInt(s[at+1:], 10, 64)
		if err != nil || v > 65507 || v < -65507 {
			overflow = true
			v = 65508
			if strings.HasPrefix(s[at+1:], "-") {
				v = -65508
			}
		}
		exp = v
		s = s[:at]
	}
	if at := strings.IndexByte(s, '.'); at >= 0 {
		exp -= int64(len(s) - at - 1)
		s = s[:at] + s[at+1:]
	}
	return strings.TrimLeft(s, "0"), exp, negative, overflow
}
func semtechNumberLexemes(w []byte) []map[string]any {
	out := []map[string]any{}
	for i := 0; i < len(w); {
		if w[i] == '"' {
			i++
			for i < len(w) {
				if w[i] == '\\' {
					i += 2
					continue
				}
				if w[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		if w[i] == '-' || w[i] >= '0' && w[i] <= '9' {
			start := i
			i++
			for i < len(w) && strings.ContainsRune("0123456789.eE+-", rune(w[i])) {
				i++
			}
			out = append(out, map[string]any{"byte_offset": start, "lexeme": string(w[start:i])})
		} else {
			i++
		}
	}
	return out
}
func semtechTXNumber(v any, key string, integer bool, high string) (json.Number, error) {
	n, ok := v.(json.Number)
	if !ok {
		return "", semtechMalformed(key + " must be finite JSON number")
	}
	digits, exp, negative, overflow := semtechDecimalParts(string(n))
	if digits == "" {
		return n, nil
	} // zero, including negative zero
	if negative {
		return "", semtechMalformed(key + " has invalid unsigned/integer value")
	}
	trailing := int64(len(digits) - len(strings.TrimRight(digits, "0")))
	if integer && (overflow && exp < 0 || !overflow && exp < -trailing) {
		return "", semtechMalformed(key + " has invalid unsigned/integer value")
	}
	maxDigits, maxExp, _, _ := semtechDecimalParts(high)
	// Compare aligned coefficient digits, without float rounding or constructing
	// untrusted powers of ten. Work and storage are bounded by the UDP token.
	adjusted := exp + int64(len(digits)) - 1
	maxAdjusted := maxExp + int64(len(maxDigits)) - 1
	over := overflow && exp > 0 || !overflow && adjusted > maxAdjusted
	if !overflow && adjusted == maxAdjusted {
		for i := 0; i < max(len(digits), len(maxDigits)); i++ {
			l, r := byte('0'), byte('0')
			if i < len(digits) {
				l = digits[i]
			}
			if i < len(maxDigits) {
				r = maxDigits[i]
			}
			if l != r {
				over = l > r
				break
			}
		}
	}
	if over {
		return "", protocolError(ErrUnsupportedFeature, "Semtech %s outside bounded reference representation", key)
	}
	return n, nil
}
func semtechTXBase64(v any) ([]byte, error) {
	s, ok := v.(string)
	if !ok {
		return nil, semtechMalformed("invalid Base64 alphabet/padding")
	}
	core := strings.TrimRight(s, "=")
	for _, c := range core {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return nil, semtechMalformed("invalid Base64 alphabet/padding")
		}
	}
	pad := len(s) - len(core)
	if pad > 2 || len(core)%4 == 1 || pad > 0 && (len(s)%4 != 0 || pad != (4-len(core)%4)%4) {
		return nil, semtechMalformed("invalid Base64 padding/remainder")
	}
	if base64.RawStdEncoding.DecodedLen(len(core)) > 256 {
		return nil, protocolError(ErrResourceExceeded, "Semtech local payload budget256 bytes")
	}
	data, err := base64.RawStdEncoding.DecodeString(core) // fixed receiver accepts unused tail bits
	if err != nil {
		return nil, semtechMalformed("invalid Base64")
	}
	return data, nil
}
func semtechTXObservation(p, f map[string]any) error {
	known := map[string]bool{}
	for _, k := range []string{"imme", "tmst", "tmms", "freq", "rfch", "powe", "modu", "datr", "codr", "fdev", "ipol", "prea", "size", "data", "ncrc"} {
		known[k] = true
	}
	unknown := map[string]any{}
	for k, v := range p {
		if !known[k] {
			unknown[k] = semtechObserved(v)
		}
	}
	f["unknown_body"] = unknown
	for _, k := range []string{"imme", "ncrc"} {
		if v, found := p[k]; found {
			if _, ok := v.(bool); !ok {
				return semtechMalformed(k + " must be bool")
			}
		}
	}
	mode := "unspecified-default"
	var selected any
	var selectedValue any
	if p["imme"] == true {
		mode = "immediate"
	} else {
		for _, k := range []string{"tmst", "tmms"} {
			if v, exists := p[k]; exists {
				high := "4294967295"
				mode = "concentrator_timestamp"
				if k == "tmms" {
					high = "18446744073709551615"
					mode = "gps_milliseconds"
				}
				n, err := semtechTXNumber(v, k, true, high)
				if err != nil {
					return err
				}
				selected = k
				selectedValue = semtechDecimal(string(n), false)
				break
			}
		}
	}
	ignored := map[string]any{}
	for _, k := range []string{"tmst", "tmms", "time"} {
		if v, ok := p[k]; ok && k != selected {
			ignored[k] = semtechObserved(v)
		}
	}
	f["schedule"], f["schedule_selected_field"], f["schedule_selected_value"], f["schedule_ignored_fields"], f["gps_conversion_performed"] = mode, selected, selectedValue, ignored, false
	highs := map[string]string{"rfch": "255", "powe": "127", "prea": "65535", "size": "65535"}
	for _, k := range []string{"rfch", "powe", "prea", "size"} {
		if v, ok := p[k]; ok {
			if _, err := semtechTXNumber(v, k, true, highs[k]); err != nil {
				return err
			}
		}
	}
	f["frequency_mhz"], f["frequency_hz_exact"], f["frequency_hz_integral"] = nil, nil, nil
	if v, ok := p["freq"]; ok {
		n, err := semtechTXNumber(v, "freq", false, "4294.967295")
		if err != nil {
			return err
		}
		f["frequency_mhz"] = semtechDecimal(string(n), false)
		hz := semtechDecimal(string(n), true)
		f["frequency_hz_exact"], f["frequency_hz_integral"] = hz, semtechExactInteger(hz)
	}
	var modu any = p["modu"]
	if modu != nil {
		if _, ok := modu.(string); !ok {
			return semtechMalformed("modu must be string")
		}
	}
	if modu != nil && modu != "LORA" && modu != "FSK" {
		return protocolError(ErrUnsupportedFeature, "Semtech unknown modulation")
	}
	f["modulation"] = modu
	ignoredModulation := map[string]any{}
	ignoredKeys := []string{"datr", "codr", "fdev", "ipol", "prea"}
	if modu == "LORA" {
		ignoredKeys = []string{"fdev"}
	} else if modu == "FSK" {
		ignoredKeys = []string{"codr", "ipol"}
	}
	for _, k := range ignoredKeys {
		if v, ok := p[k]; ok {
			ignoredModulation[k] = semtechObserved(v)
		}
	}
	f["modulation_ignored_fields"] = ignoredModulation
	if modu == "LORA" {
		if v, ok := p["ipol"]; ok {
			if _, isBool := v.(bool); !isBool {
				return semtechMalformed("active LORA ipol must be bool")
			}
		}
	}

	for _, k := range []string{"lora_sf", "lora_bandwidth_khz", "coding_rate_normalized", "fsk_bitrate", "fsk_deviation_hz", "fsk_reference_deviation_khz", "preamble_provided", "preamble_reference_effective", "data_hex", "declared_size", "decoded_size"} {
		f[k] = nil
	}
	if v, ok := p["datr"]; ok {
		switch modu {
		case "FSK":
			n, err := semtechTXNumber(v, "FSK datr", true, "4294967295")
			if err != nil {
				return err
			}
			f["fsk_bitrate"] = semtechDecimal(string(n), false)
		case "LORA":
			s, ok := v.(string)
			if !ok {
				return semtechMalformed("LORA datr must be string")
			}
			if !semtechLoRaRate.MatchString(s) {
				return protocolError(ErrUnsupportedFeature, "Semtech LORA datr outside fixed complete subset")
			}
			at := strings.Index(s, "BW")
			sf, _ := strconv.Atoi(s[2:at])
			bw, _ := strconv.Atoi(s[at+2:])
			f["lora_sf"], f["lora_bandwidth_khz"] = sf, bw
		}
	}
	if v, ok := p["codr"]; ok {
		s, ok := v.(string)
		if !ok && modu == "LORA" {
			return semtechMalformed("active LORA codr must be string")
		}
		if modu == "LORA" {
			rate := map[string]string{"4/5": "4/5", "4/6": "4/6", "2/3": "4/6", "4/7": "4/7", "4/8": "4/8", "1/2": "4/8"}[s]
			if rate == "" {
				return protocolError(ErrUnsupportedFeature, "Semtech unknown coding-rate identifier")
			}
			f["coding_rate_normalized"] = rate
		}
	}
	numberInt := func(k string) int64 {
		// All callers are nonnegative integral values already bounded at uint32 or
		// uint16. Scientific notation cannot overflow after the exact range check.
		digits, exp, _, _ := semtechDecimalParts(string(p[k].(json.Number)))
		if digits == "" {
			return 0
		}
		if exp < 0 {
			digits = digits[:int64(len(digits))+exp]
		} else {
			digits += strings.Repeat("0", int(exp))
		}
		v, _ := strconv.ParseInt(digits, 10, 64)
		return v
	}
	if modu == "FSK" {
		if v, ok := p["fdev"]; ok {
			if _, err := semtechTXNumber(v, "FSK fdev", true, "4294967295"); err != nil {
				return err
			}
			hz := numberInt("fdev")
			f["fsk_deviation_hz"], f["fsk_reference_deviation_khz"] = semtechDecimal(string(v.(json.Number)), false), hz/1000
			if hz/1000 > 255 {
				return protocolError(ErrUnsupportedFeature, "Semtech FSK deviation exceeds uint8 kHz reference representation")
			}
		}
	}
	if modu == "LORA" || modu == "FSK" {
		floor, def := int64(6), int64(8)
		if modu == "FSK" {
			floor, def = 3, 5
		}
		if _, ok := p["prea"]; ok {
			n := numberInt("prea")
			f["preamble_provided"], f["preamble_reference_effective"] = n, max(floor, n)
		} else {
			f["preamble_reference_effective"] = def
		}
	}
	var declared any
	if _, ok := p["size"]; ok {
		declared = numberInt("size")
	}
	f["declared_size"] = declared
	if v, ok := p["data"]; ok {
		data, err := semtechTXBase64(v)
		if err != nil {
			return err
		}
		f["data_hex"], f["decoded_size"] = hex.EncodeToString(data), len(data)
		if declared != nil && int64(len(data)) != declared.(int64) {
			return semtechMalformed("strict local size/data-consistency policy; fixed receiver only warns")
		}
	}
	missing := []string{}
	for _, k := range []string{"freq", "rfch", "modu", "datr", "size", "data"} {
		if _, ok := p[k]; !ok {
			missing = append(missing, k)
		}
	}
	if modu == "LORA" {
		if _, ok := p["codr"]; !ok {
			missing = append(missing, "codr")
		}
	}
	if modu == "FSK" {
		if _, ok := p["fdev"]; !ok {
			missing = append(missing, "fdev")
		}
	}
	f["reference_required_missing"], f["reference_metadata_complete"], f["sender_defaults_not_resolved"] = missing, len(missing) == 0, append([]string{}, missing...)
	f["radio_config_validation_performed"], f["transmission_success_claim"] = false, false
	return nil
}
func semtechProjectionLimit(v any, elements, depth int) error {
	if depth < 1 {
		return protocolError(ErrResourceExceeded, "Semtech projected field depth exceeds budget")
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) > elements {
			return protocolError(ErrResourceExceeded, "Semtech projected field map exceeds budget")
		}
		for _, q := range x {
			if err := semtechProjectionLimit(q, elements, depth-1); err != nil {
				return err
			}
		}
	case []map[string]any:
		if len(x) > elements {
			return protocolError(ErrResourceExceeded, "Semtech projected array exceeds budget")
		}
		for _, q := range x {
			if err := semtechProjectionLimit(q, elements, depth-1); err != nil {
				return err
			}
		}
	case []any:
		if len(x) > elements {
			return protocolError(ErrResourceExceeded, "Semtech projected array exceeds budget")
		}
		for _, q := range x {
			if err := semtechProjectionLimit(q, elements, depth-1); err != nil {
				return err
			}
		}
	case []string:
		if len(x) > elements {
			return protocolError(ErrResourceExceeded, "Semtech projected array exceeds budget")
		}
	}
	return nil
}
func semtechDownlinkPortEvidence(w []byte, src, dst uint16) bool {
	return len(w) >= 4 && w[0] == 2 && (w[3] == 3 || w[3] == 5) && (src == 1700 || dst == 1700)
}
func (a *binParser) decodeSemtechDownlinkDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit bool) bool {
	if !explicit && !semtechDownlinkPortEvidence(w, src, dst) {
		return false
	}
	// Explicit selection can diagnose malformed headers, but leaves valid
	// upstream datagrams in their compatible existing profile.
	if len(w) >= 4 && (w[3] == 0 || w[3] == 1 || w[3] == 2 || w[3] == 4) {
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "semtech-udp", semtechDownlinkProfile, "wire-and-port-hint", "message"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	temp := &binFlow{a: a}
	defer temp.closeSession()
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	var fields map[string]any
	var err error
	if len(w) > limit {
		err = protocolError(ErrResourceExceeded, "Semtech downlink exceeds frame/message budget")
	} else if err = temp.reserveSession(semtechDownlinkProjectionBytes + 512*int64(len(w))); err == nil {
		fields, err = decodeSemtechDownlink(w, a.budget.MaxCollectionElements)
		if err == nil {
			err = semtechProjectionLimit(fields, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		}
	}
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
		err = typed
	}
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.Session = map[string]any{"Association": "unassociated-downlink-observation", "Authentication Verified": false, "RF Delivery Proven": false}
	}
	raw := w
	if len(w) > limit || temp.sessionBytes == 0 {
		raw = nil
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil && len(w) > 0 {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

func (s *captureSession) probeSemtechDownlink(w []byte) (ProbeResult, bool) {
	if !semtechDownlinkPortEvidence(w, s.f.ports[0], s.f.ports[1]) {
		return ProbeResult{}, false
	}
	a := s.f.a
	temp := &binFlow{a: a}
	defer temp.closeSession()
	var err error
	if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		err = protocolError(ErrResourceExceeded, "Semtech downlink probe exceeds byte budget")
	} else if err = temp.reserveSession(semtechDownlinkProjectionBytes + 512*int64(len(w))); err == nil {
		var f map[string]any
		f, err = decodeSemtechDownlink(w, a.budget.MaxCollectionElements)
		if err == nil {
			err = semtechProjectionLimit(f, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		}
	}
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, true
	}
	return probeAccept("semtech-udp", semtechDownlinkProfile, 98), true
}

// The outer UDP limit must retain an admitted downlink's typed diagnostic.
// The native handler checks this limit before JSON parsing, reservation or raw
// copying; no oversized body or random off-port token grants admission.
func (a *binParser) refuseSemtechDownlinkOversize(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	if explicit != "" && explicit != "semtech-udp" {
		return false
	}
	return a.decodeSemtechDownlinkDatagram(e, w, src, dst, explicit == "semtech-udp")
}
