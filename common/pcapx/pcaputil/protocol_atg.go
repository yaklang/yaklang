package pcaputil

import (
	"bytes"
	"math"
	"strconv"
)

// ATG scope: Veeder-Root TLS-450 computer-format i201TT inventory and
// i202TT version1 delivery queries, checksum-protected replies and rejection.
// Display format, passwords, changed ETX, writes and other commands are outside
// these profiles. Measurement units and gauge identity are not inferred.
func probeATG(w []byte, limit int) ProbeResult {
	prefix := []byte{1, 'i', '2', '0'}
	n := min(len(w), len(prefix))
	if !bytes.Equal(w[:n], prefix[:n]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) > 4 && w[4] != '1' && w[4] != '2' {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 7 {
		return ProbeResult{Verdict: ProbeNeedMore, Protocol: "atg"}
	}
	if _, err := atgDecimal(w[5:7], 99); err != nil {
		return ProbeResult{Verdict: ProbeReject, Protocol: "atg"}
	}
	if limit < 7 {
		return ProbeResult{Verdict: ProbeReject, Protocol: "atg"}
	}
	return ProbeResult{Verdict: ProbeAccept, Protocol: "atg", Version: "TLS450-i20" + string(w[4]) + "-computer"}
}

type atgPending struct{ dir, tank, function byte }
type binATG struct {
	clientDir, maxElements, count int
	ids                           [16]uint64
	seen                          [2][2]uint64
	pending                       [16]atgPending
	ambiguous                     uint16
	clientKnown                   bool
}

func (s *binATG) sessionBytes() int64 { return 256 }
func (s *binATG) outstanding() int    { return s.count }
func (s *binATG) candidates(indices []int) []map[string]any {
	out := make([]map[string]any, 0, len(indices))
	for _, i := range indices {
		out = append(out, map[string]any{"Function": "i20" + string(s.pending[i].function), "Tank Selector": int(s.pending[i].tank), "Observed Request ID": s.ids[i]})
	}
	return out
}
func (s *binATG) remove(i int) {
	copy(s.pending[i:], s.pending[i+1:s.count])
	copy(s.ids[i:], s.ids[i+1:s.count])
	low := s.ambiguous & ((uint16(1) << i) - 1)
	s.ambiguous = low | ((s.ambiguous >> (i + 1)) << i)
	s.count--
	s.pending[s.count] = atgPending{}
	s.ids[s.count] = 0
}

func atgMalformed(reason string) error { return protocolError(ErrMalformedMessage, "ATG %s", reason) }

func atgFrameSize(w []byte, request bool, maxBytes int) (int, error) {
	if maxBytes <= 0 {
		return 0, protocolError(ErrResourceExceeded, "ATG frame budget is zero")
	}
	if len(w) == 0 {
		return 0, nil
	}
	if w[0] != 1 {
		return 0, atgMalformed("SOH is absent")
	}
	if request {
		if maxBytes < 7 {
			return 0, protocolError(ErrResourceExceeded, "ATG command exceeds frame budget")
		}
		if len(w) < 7 {
			return 0, nil
		}
		if probeATG(w[:7], 7).Verdict != ProbeAccept {
			return 0, protocolError(ErrUnsupportedFeature, "ATG command outside selected inventory/delivery computer profile")
		}
		return 7, nil
	}
	if i := bytes.IndexByte(w, 3); i >= 0 {
		if i+1 > maxBytes {
			return 0, protocolError(ErrResourceExceeded, "ATG response exceeds frame budget")
		}
		return i + 1, nil
	}
	if len(w) >= maxBytes {
		return 0, protocolError(ErrResourceExceeded, "ATG unterminated response exceeds frame budget")
	}
	return 0, nil
}

func atgDecimal(w []byte, max int) (int, error) {
	if len(w) == 0 {
		return 0, atgMalformed("decimal field is empty")
	}
	n := 0
	for _, b := range w {
		if b < '0' || b > '9' {
			return 0, atgMalformed("decimal field is invalid")
		}
		n = n*10 + int(b-'0')
	}
	if n > max {
		return 0, atgMalformed("decimal field is outside range")
	}
	return n, nil
}

func atgHex(w []byte) (uint64, error) {
	if len(w) == 0 || len(w) > 8 {
		return 0, atgMalformed("hex field size is invalid")
	}
	for _, b := range w {
		if !(b >= '0' && b <= '9' || b >= 'A' && b <= 'F') {
			return 0, atgMalformed("hex field must use upper-case ASCII hex")
		}
	}
	x, err := strconv.ParseUint(string(w), 16, 32)
	if err != nil {
		return 0, atgMalformed("hex field is invalid")
	}
	return x, nil
}

func atgDate(w []byte, out map[string]any) error {
	if len(w) != 10 {
		return atgMalformed("date field has wrong length")
	}
	yy, e := atgDecimal(w[:2], 99)
	if e != nil {
		return e
	}
	mm, e := atgDecimal(w[2:4], 12)
	if e != nil {
		return e
	}
	dd, e := atgDecimal(w[4:6], 31)
	if e != nil {
		return e
	}
	h, e := atgDecimal(w[6:8], 23)
	if e != nil {
		return e
	}
	m, e := atgDecimal(w[8:10], 59)
	if e != nil {
		return e
	}
	if mm == 0 || dd == 0 {
		return atgMalformed("date has zero month/day")
	}
	days := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if yy%4 == 0 {
		days[1] = 29
	}
	if dd > days[mm-1] {
		return atgMalformed("calendar date is invalid")
	}
	out["Date/Time"], out["Year Two Digits"], out["Month"], out["Day"], out["Hour"], out["Minute"] = string(w), yy, mm, dd, h, m
	return nil
}

func decodeATGInventory(w []byte, maxElements int) (map[string]any, error) {
	if len(w) < 10 || w[0] != 1 || w[len(w)-1] != 3 {
		return nil, atgMalformed("response envelope is incomplete")
	}
	checksum, e := atgHex(w[len(w)-5 : len(w)-1])
	if e != nil {
		return nil, e
	}
	var sum uint16
	for _, b := range w[:len(w)-5] {
		if b > 127 {
			return nil, atgMalformed("response is not7-bitASCII")
		}
		sum += uint16(b)
	}
	if sum+uint16(checksum) != 0 {
		return nil, atgMalformed("response checksum mismatch")
	}
	f := map[string]any{"Context Level": "observed", "Checksum": int(checksum), "Checksum Valid": true, "Unit System": "unobserved"}
	if bytes.Equal(w[1:5], []byte("9999")) && len(w) == 10 {
		f["Packet Name"] = "Command Rejected"
		return f, nil
	}
	if len(w) >= 24 && bytes.Equal(w[1:5], []byte("i202")) && bytes.Equal(w[len(w)-7:len(w)-5], []byte("&&")) {
		return decodeATGDeliveryBody(w, f, maxElements)
	}
	if len(w) < 24 || !bytes.Equal(w[1:5], []byte("i201")) || !bytes.Equal(w[len(w)-7:len(w)-5], []byte("&&")) {
		return nil, protocolError(ErrUnsupportedFeature, "ATG response outside i201TT computer profile")
	}
	selector, e := atgDecimal(w[5:7], 99)
	if e != nil {
		return nil, e
	}
	f["Packet Name"], f["Function"], f["Tank Selector"] = "In-Tank Inventory Response", "i201", selector
	if e = atgDate(w[7:17], f); e != nil {
		return nil, e
	}
	body := w[17 : len(w)-7]
	tanks := []map[string]any{}
	seen := [100]bool{}
	for len(body) > 0 {
		if len(tanks) >= maxElements {
			return nil, protocolError(ErrResourceExceeded, "ATG tank count exceeds budget")
		}
		if len(body) < 9 {
			return nil, atgMalformed("tank record header is truncated")
		}
		tank, e := atgDecimal(body[:2], 99)
		if e != nil {
			return nil, e
		}
		if tank == 0 || seen[tank] {
			return nil, atgMalformed("tank number is zero or duplicate")
		}
		seen[tank] = true
		if body[2] < 32 || body[2] > 126 {
			return nil, atgMalformed("product code is not printableASCII")
		}
		status, e := atgHex(body[3:7])
		if e != nil {
			return nil, e
		}
		n, e := atgHex(body[7:9])
		if e != nil {
			return nil, e
		}
		if n != 7 {
			return nil, protocolError(ErrUnsupportedFeature, "ATG profile requires seven inventory values")
		}
		if int(n) > maxElements {
			return nil, protocolError(ErrResourceExceeded, "ATG inventory value count exceeds budget")
		}
		if len(body) < 9+8*int(n) {
			return nil, atgMalformed("inventory float fields are truncated")
		}
		if selector != 0 && tank != selector {
			return nil, atgMalformed("selected tank does not match response record")
		}
		row := map[string]any{"Tank Number": tank, "Product Code": string(body[2:3]), "Status Bits": int(status), "Delivery In Progress": status&1 != 0, "Leak Test In Progress": status&2 != 0, "Invalid Fuel Height Alarm": status&4 != 0, "Value Count": 7}
		for i, k := range []string{"Volume", "TC Volume", "Ullage", "Height", "Water", "Temperature", "Water Volume"} {
			bits, e := atgHex(body[9+8*i : 17+8*i])
			if e != nil {
				return nil, e
			}
			x := math.Float32frombits(uint32(bits))
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return nil, atgMalformed("inventory float is not finite")
			}
			row[k] = float64(x)
		}
		tanks = append(tanks, row)
		body = body[9+8*int(n):]
	}
	if selector != 0 && len(tanks) != 1 {
		return nil, atgMalformed("specific tank response must contain one record")
	}
	f["Tanks"], f["Tank Count"] = tanks, len(tanks)
	return f, nil
}

// The wire carries a function and tank selector, but no transaction sequence.
// Repeated selectors and untagged rejections retain unresolved candidates rather
// than inventing a FIFO owner. This history is bounded within the original state charge.
func (s *binATG) consume(dir int, w []byte, evidence ...uint64) (map[string]any, error) {
	if !s.clientKnown {
		return nil, protocolError(ErrContextRequired, "ATG requires observed client direction")
	}
	if dir == s.clientDir {
		if len(w) != 7 || probeATG(w, 7).Verdict != ProbeAccept {
			return nil, protocolError(ErrUnsupportedFeature, "ATG command outside selected read-only profile")
		}
		if s.count >= len(s.pending) || s.count >= s.maxElements {
			return nil, protocolError(ErrResourceExceeded, "ATG pending queries exceed budget")
		}
		tank, e := atgDecimal(w[5:7], 99)
		if e != nil {
			return nil, e
		}
		id := uint64(0)
		if len(evidence) > 0 {
			id = evidence[0]
		}
		fn := int(w[4] - '1')
		word, bit := tank/64, uint(tank%64)
		amb := s.seen[fn][word]&(uint64(1)<<bit) != 0
		s.seen[fn][word] |= uint64(1) << bit
		if amb {
			for i := 0; i < s.count; i++ {
				if s.pending[i].function == w[4] && int(s.pending[i].tank) == tank {
					s.ambiguous |= uint16(1) << i
				}
			}
			s.ambiguous |= uint16(1) << s.count
		}
		s.pending[s.count] = atgPending{dir: byte(dir), tank: byte(tank), function: w[4]}
		s.ids[s.count] = id
		s.count++
		name, function := "In-Tank Inventory Request", "i201"
		if w[4] == '2' {
			name, function = "In-Tank Delivery Request", "i202"
		}
		out := map[string]any{"Packet Name": name, "Function": function, "Tank Selector": tank, "Role": "request", "Outstanding Requests": s.count, "Context Level": "observed"}
		if w[4] == '2' || amb {
			out["Association"], out["Observed Request ID"] = "observed-request", id
			if amb {
				out["Association"], out["Observed Request ID"] = "ambiguous-query", uint64(0)
			}
		}
		return out, nil
	}
	f, e := decodeATGInventory(w, s.maxElements)
	if e != nil {
		return nil, e
	}
	if s.count == 0 {
		return nil, protocolError(ErrContextRequired, "ATG response has no observed query")
	}
	reject := f["Packet Name"] == "Command Rejected"
	matches := make([]int, 0, s.count)
	for i := 0; i < s.count; i++ {
		if reject || (f["Tank Selector"] == int(s.pending[i].tank) && len(w) > 4 && w[4] == s.pending[i].function) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		if w[4] == '2' || s.pending[0].function == '2' {
			return nil, protocolError(ErrContextRequired, "ATG delivery reply lacks observed function/tank context")
		}
		return nil, atgMalformed("response command differs from observed queries")
	}
	i := matches[0]
	amb := len(matches) > 1 || s.ambiguous&(uint16(1)<<i) != 0
	f["Role"], f["Association"] = "response", "matched"
	if reject && amb {
		for n := 0; n < s.count; n++ {
			s.ambiguous |= uint16(1) << n
		}
		f["In Reply To"] = "unobserved"
	} else {
		f["In Reply To"], f["Request Tank Selector"] = "In-Tank Inventory Request", int(s.pending[i].tank)
		if s.pending[i].function == '2' {
			f["In Reply To"], f["Request Function"], f["Observed Request ID"] = "In-Tank Delivery Request", "i202", s.ids[i]
		}
	}
	if amb {
		f["Association"], f["Observed Request ID"] = "ambiguous-response", uint64(0)
		f["Candidate Requests"] = s.candidates(matches)
	} else {
		s.remove(i)
	}
	f["Outstanding Requests"] = s.count
	return f, nil
}
