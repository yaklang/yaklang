package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ISO 8583 card-originated interchange messages over a two-byte length
// prefixed TCP carrier. Field types and maximum lengths follow the public 1987
// data-element table; elements reserved for ISO/national/private use keep their
// declared widths instead of invented subfields. PAN, track and PIN elements
// are marked sensitive but returned as captured evidence; masking is a caller
// policy and is not applied here.
//
// The envelope must validate completely: MTI digits, primary bitmap and the
// optional secondary bitmap, every set element placed exactly, and no leftover
// bytes. Unplaced bytes are a malformed message, never an inferred private
// element. This profile has no request/response association yet.

type iso8583Encoding uint8

const (
	iso8583ASCII iso8583Encoding = iota
	iso8583BCD
)

type iso8583FieldType uint8

const (
	iso8583Numeric iso8583FieldType = iota // n, x+n
	iso8583Text                            // a, an, ans, ns, z
	iso8583Binary                          // b
)

type iso8583LengthType uint8

const (
	iso8583Fixed iso8583LengthType = iota
	iso8583LL                      // two-digit length prefix
	iso8583LLL                     // three-digit length prefix
)

type iso8583FieldSpec struct {
	Name      string
	Type      iso8583FieldType
	Length    iso8583LengthType
	Max       int // digits/characters for n/text, bits for binary
	Sensitive bool
}

func iso8583SpecTable() map[int]iso8583FieldSpec {
	specs := make(map[int]iso8583FieldSpec, 128)
	add := func(field int, name string, t iso8583FieldType, l iso8583LengthType, max int, sensitive bool) {
		specs[field] = iso8583FieldSpec{Name: name, Type: t, Length: l, Max: max, Sensitive: sensitive}
	}
	fixed := func(field int, name string, t iso8583FieldType, max int) {
		add(field, name, t, iso8583Fixed, max, false)
	}
	ll := func(field int, name string, t iso8583FieldType, max int) {
		add(field, name, t, iso8583LL, max, false)
	}
	lll := func(field int, name string, t iso8583FieldType, max int) {
		add(field, name, t, iso8583LLL, max, false)
	}

	add(1, "Bitmap, secondary", iso8583Binary, iso8583Fixed, 64, false)
	add(2, "Primary account number (PAN)", iso8583Numeric, iso8583LL, 19, true)
	fixed(3, "Processing code", iso8583Numeric, 6)
	fixed(4, "Amount, transaction", iso8583Numeric, 12)
	fixed(5, "Amount, settlement", iso8583Numeric, 12)
	fixed(6, "Amount, cardholder billing", iso8583Numeric, 12)
	fixed(7, "Transmission date and time", iso8583Numeric, 10)
	fixed(8, "Amount, cardholder billing fee", iso8583Numeric, 8)
	fixed(9, "Conversion rate, settlement", iso8583Numeric, 8)
	fixed(10, "Conversion rate, cardholder billing", iso8583Numeric, 8)
	fixed(11, "System trace audit number (STAN)", iso8583Numeric, 6)
	fixed(12, "Time, local transaction", iso8583Numeric, 6)
	fixed(13, "Date, local transaction", iso8583Numeric, 4)
	fixed(14, "Date, expiration", iso8583Numeric, 4)
	fixed(15, "Date, settlement", iso8583Numeric, 4)
	fixed(16, "Date, conversion", iso8583Numeric, 4)
	fixed(17, "Date, capture", iso8583Numeric, 4)
	fixed(18, "Merchant type", iso8583Numeric, 4)
	fixed(19, "Acquiring institution country code", iso8583Numeric, 3)
	fixed(20, "PAN extended country code", iso8583Numeric, 3)
	fixed(21, "Forwarding institution country code", iso8583Numeric, 3)
	fixed(22, "Point of service entry mode", iso8583Numeric, 3)
	fixed(23, "Application PAN sequence number", iso8583Numeric, 3)
	fixed(24, "Function code (network international identifier)", iso8583Numeric, 3)
	fixed(25, "Point of service condition code", iso8583Numeric, 2)
	fixed(26, "Point of service capture code", iso8583Numeric, 2)
	fixed(27, "Authorizing identification response length", iso8583Numeric, 1)
	for _, field := range []int{28, 29, 30, 31} {
		fixed(field, "Amount, fee", iso8583Numeric, 8)
	}
	ll(32, "Acquiring institution identification code", iso8583Numeric, 11)
	ll(33, "Forwarding institution identification code", iso8583Numeric, 11)
	ll(34, "Primary account number, extended", iso8583Text, 28)
	add(35, "Track 2 data", iso8583Text, iso8583LL, 37, true)
	add(36, "Track 3 data", iso8583Numeric, iso8583LLL, 104, true)
	fixed(37, "Retrieval reference number", iso8583Text, 12)
	fixed(38, "Authorization identification response", iso8583Text, 6)
	fixed(39, "Response code", iso8583Text, 2)
	fixed(40, "Service restriction code", iso8583Text, 3)
	fixed(41, "Card acceptor terminal identification", iso8583Text, 8)
	fixed(42, "Card acceptor identification code", iso8583Text, 15)
	fixed(43, "Card acceptor name and location", iso8583Text, 40)
	ll(44, "Additional response data", iso8583Text, 25)
	add(45, "Track 1 data", iso8583Text, iso8583LL, 76, true)
	lll(46, "Additional data, ISO", iso8583Text, 999)
	lll(47, "Additional data, national", iso8583Text, 999)
	lll(48, "Additional data, private", iso8583Text, 999)
	fixed(49, "Currency code, transaction", iso8583Text, 3)
	fixed(50, "Currency code, settlement", iso8583Text, 3)
	fixed(51, "Currency code, cardholder billing", iso8583Text, 3)
	add(52, "Personal identification number data", iso8583Binary, iso8583Fixed, 64, true)
	fixed(53, "Security related control information", iso8583Numeric, 16)
	lll(54, "Additional amounts", iso8583Text, 120)
	lll(55, "ICC data, EMV", iso8583Text, 999)
	for field := 56; field <= 63; field++ {
		lll(field, "Reserved for ISO, national or private use", iso8583Text, 999)
	}
	add(64, "Message authentication code", iso8583Binary, iso8583Fixed, 64, false)
	add(65, "Bitmap, tertiary", iso8583Binary, iso8583Fixed, 1, false)
	fixed(66, "Settlement code", iso8583Numeric, 1)
	fixed(67, "Extended payment code", iso8583Numeric, 2)
	fixed(68, "Receiving institution country code", iso8583Numeric, 3)
	fixed(69, "Settlement institution country code", iso8583Numeric, 3)
	fixed(70, "Network management information code", iso8583Numeric, 3)
	fixed(71, "Message number", iso8583Numeric, 4)
	fixed(72, "Last message number", iso8583Numeric, 4)
	fixed(73, "Action date", iso8583Numeric, 6)
	for field := 74; field <= 81; field++ {
		fixed(field, "Message count", iso8583Numeric, 10)
	}
	for field := 82; field <= 85; field++ {
		fixed(field, "Processing fee amount", iso8583Numeric, 12)
	}
	for field := 86; field <= 89; field++ {
		fixed(field, "Total amount", iso8583Numeric, 16)
	}
	fixed(90, "Original data elements", iso8583Numeric, 42)
	fixed(91, "File update code", iso8583Text, 1)
	fixed(92, "File security code", iso8583Text, 2)
	fixed(93, "Response indicator", iso8583Text, 5)
	fixed(94, "Service indicator", iso8583Text, 7)
	fixed(95, "Replacement amounts", iso8583Text, 42)
	add(96, "Message security code", iso8583Binary, iso8583Fixed, 64, false)
	fixed(97, "Net settlement amount", iso8583Numeric, 16)
	fixed(98, "Payee", iso8583Text, 25)
	ll(99, "Settlement institution identification code", iso8583Numeric, 11)
	ll(100, "Receiving institution identification code", iso8583Numeric, 11)
	ll(101, "File name", iso8583Text, 17)
	ll(102, "Account identification 1", iso8583Text, 28)
	ll(103, "Account identification 2", iso8583Text, 28)
	lll(104, "Transaction description", iso8583Text, 100)
	for field := 105; field <= 127; field++ {
		lll(field, "Reserved for ISO, national or private use", iso8583Text, 999)
	}
	add(128, "Message authentication code", iso8583Binary, iso8583Fixed, 64, false)
	return specs
}

var iso8583FieldSpecs = iso8583SpecTable()

const iso8583Profile = "iso8583-length-prefixed"

type iso8583Session struct {
	bigEndian   bool
	clientDir   int
	clientKnown bool
}

func iso8583Length(w []byte, bigEndian bool) int {
	if bigEndian {
		return int(binary.BigEndian.Uint16(w[:2]))
	}
	return int(binary.LittleEndian.Uint16(w[:2]))
}

func iso8583BCDByte(v byte) (byte, byte, bool) {
	hi, lo := v>>4, v&0x0f
	if hi > 9 || lo > 9 {
		return 0, 0, false
	}
	return hi, lo, true
}

func iso8583BCDValue(raw []byte) (string, bool) {
	var out strings.Builder
	out.Grow(len(raw) * 2)
	for _, v := range raw {
		hi, lo, ok := iso8583BCDByte(v)
		if !ok {
			return "", false
		}
		out.WriteByte('0' + hi)
		out.WriteByte('0' + lo)
	}
	return out.String(), true
}

type iso8583Envelope struct {
	Encoding iso8583Encoding
	MTI      string
	Bitmap   []byte
	Offset   int
}

// iso8583MTIDigits validates the four published MTI dimensions: version,
// message class, message function and message origin. Reserved values keep
// their explicit refusal instead of being treated as decodable elements.
func iso8583MTIDigits(b []byte) error {
	version := b[0] - '0'
	switch version {
	case 0, 1, 2, 8, 9:
	case 3, 4, 5, 6, 7:
		return protocolError(ErrUnsupportedVersion, "iso8583: MTI uses a version digit reserved by ISO")
	default:
		return protocolError(ErrMalformedMessage, "iso8583: MTI version digit is not a published value")
	}
	if class := b[1] - '0'; class < 1 || class > 8 {
		return protocolError(ErrMalformedMessage, "iso8583: MTI message class is outside 1..8")
	}
	if function := b[2] - '0'; function > 7 {
		return protocolError(ErrMalformedMessage, "iso8583: MTI message function is outside 0..7")
	}
	if origin := b[3] - '0'; origin > 4 {
		return protocolError(ErrMalformedMessage, "iso8583: MTI message origin is outside 0..4")
	}
	return nil
}

// parseISO8583Envelope validates the MTI and bitmap. NeedMore is returned when
// more bytes can complete the envelope; Malformed is returned when the visible
// bytes can never do so.
func parseISO8583Envelope(body []byte) (*iso8583Envelope, error) {
	if len(body) < 4 {
		return nil, protocolError(ErrNeedMore, "iso8583: message is shorter than the MTI")
	}
	env := &iso8583Envelope{}
	if body[0] >= '0' && body[0] <= '9' {
		env.Encoding = iso8583ASCII
		for i := 0; i < 4; i++ {
			if body[i] < '0' || body[i] > '9' {
				return nil, protocolError(ErrMalformedMessage, "iso8583: ASCII MTI is not four decimal digits")
			}
		}
		if err := iso8583MTIDigits(body[:4]); err != nil {
			return nil, err
		}
		env.MTI = string(body[:4])
		if len(body) < 20 {
			return nil, protocolError(ErrNeedMore, "iso8583: ASCII primary bitmap is incomplete")
		}
		bitmap, err := hex.DecodeString(string(body[4:20]))
		if err != nil {
			return nil, protocolError(ErrMalformedMessage, "iso8583: ASCII primary bitmap is not hexadecimal")
		}
		env.Bitmap, env.Offset = bitmap, 20
		if bitmap[0]&0x80 != 0 {
			if len(body) < 36 {
				return nil, protocolError(ErrNeedMore, "iso8583: ASCII secondary bitmap is incomplete")
			}
			secondary, err := hex.DecodeString(string(body[20:36]))
			if err != nil {
				return nil, protocolError(ErrMalformedMessage, "iso8583: ASCII secondary bitmap is not hexadecimal")
			}
			env.Bitmap, env.Offset = append(bitmap, secondary...), 36
		}
		return env, nil
	}
	if len(body) < 2 {
		return nil, protocolError(ErrNeedMore, "iso8583: binary MTI is incomplete")
	}
	for i := 0; i < 2; i++ {
		if _, _, ok := iso8583BCDByte(body[i]); !ok {
			return nil, protocolError(ErrMalformedMessage, "iso8583: binary MTI is not packed BCD")
		}
	}
	digits, _ := iso8583BCDValue(body[:2])
	if err := iso8583MTIDigits([]byte(digits)); err != nil {
		return nil, err
	}
	env.Encoding = iso8583BCD
	env.MTI = fmt.Sprintf("%02x%02x", body[0], body[1])
	if len(body) < 10 {
		return nil, protocolError(ErrNeedMore, "iso8583: binary primary bitmap is incomplete")
	}
	bitmap := make([]byte, 8, 16)
	copy(bitmap, body[2:10])
	env.Bitmap, env.Offset = bitmap, 10
	if bitmap[0]&0x80 != 0 {
		if len(body) < 18 {
			return nil, protocolError(ErrNeedMore, "iso8583: binary secondary bitmap is incomplete")
		}
		env.Bitmap, env.Offset = append(bitmap, body[10:18]...), 18
	}
	return env, nil
}

func iso8583LengthName(l iso8583LengthType) string {
	switch l {
	case iso8583LL:
		return "LLVAR"
	case iso8583LLL:
		return "LLLVAR"
	default:
		return "fixed"
	}
}

func iso8583TypeName(t iso8583FieldType) string {
	switch t {
	case iso8583Numeric:
		return "numeric"
	case iso8583Binary:
		return "binary"
	default:
		return "text"
	}
}

// iso8583ReadField consumes one element and returns its value, the wire length
// in bytes and an error. BCD numerics decode the declared byte width; an
// odd-length element keeps its leading pad digit instead of guessing a trim.
func iso8583ReadField(w []byte, field int, spec iso8583FieldSpec, enc iso8583Encoding) (any, int, int, error) {
	length, header := spec.Max, 0
	switch spec.Length {
	case iso8583LL, iso8583LLL:
		digits := 2
		if spec.Length == iso8583LLL {
			digits = 3
		}
		if enc == iso8583ASCII {
			if len(w) < digits {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d length prefix is incomplete", field))
			}
			for i := 0; i < digits; i++ {
				if w[i] < '0' || w[i] > '9' {
					return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d length prefix is not decimal", field))
				}
			}
			value, err := strconv.Atoi(string(w[:digits]))
			if err != nil {
				return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d length prefix is invalid", field))
			}
			length, header = value, digits
		} else {
			size := 1
			if digits == 3 {
				size = 2
			}
			if len(w) < size {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d length prefix is incomplete", field))
			}
			value, ok := iso8583BCDValue(w[:size])
			if !ok {
				return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d length prefix is not packed BCD", field))
			}
			numeric, err := strconv.Atoi(value)
			if err != nil {
				return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d length prefix is invalid", field))
			}
			length, header = numeric, size
		}
	}
	if length < 0 || length > spec.Max {
		return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d declares %d beyond the %d limit", field, length, spec.Max))
	}

	var value any
	body := 0
	switch spec.Type {
	case iso8583Binary:
		if enc == iso8583ASCII {
			body = (length + 3) / 4
			if len(w) < header+body {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d value is incomplete", field))
			}
			// The ASCII carrier spells the same bit string in hexadecimal. Both
			// encodings must produce identical bytes so a caller never depends on
			// the carrier's notation.
			decoded, err := hex.DecodeString(string(w[header : header+body]))
			if err != nil {
				return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d is not hexadecimal", field))
			}
			value = decoded
		} else {
			body = (length + 7) / 8
			if len(w) < header+body {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d value is incomplete", field))
			}
			value = append([]byte(nil), w[header:header+body]...)
		}
	case iso8583Numeric:
		if enc == iso8583ASCII {
			body = length
			if len(w) < header+body {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d value is incomplete", field))
			}
			raw := w[header : header+body]
			for _, c := range raw {
				if c < '0' || c > '9' {
					return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d is not decimal", field))
				}
			}
			value = string(raw)
		} else {
			body = (length + 1) / 2
			if len(w) < header+body {
				return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d value is incomplete", field))
			}
			numeric, ok := iso8583BCDValue(w[header : header+body])
			if !ok {
				return nil, 0, 0, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: element %d is not packed BCD", field))
			}
			// Odd-width numeric elements carry a leading pad nibble. Comparing
			// the ASCII and binary captures of the same issuer shows the pad
			// digit is dropped: field 22 is "021" in both carriers.
			if length%2 == 1 && len(numeric) == length+1 && numeric[0] == '0' {
				numeric = numeric[1:]
			}
			value = numeric
		}
	default:
		body = length
		if len(w) < header+body {
			return nil, 0, 0, protocolError(ErrNeedMore, fmt.Sprintf("iso8583: element %d value is incomplete", field))
		}
		// Text elements carry raw characters in both encodings. The binary
		// capture keeps "12345   " here, so nibble-decoding is never guessed
		// for a text element even when every byte is a decimal nibble.
		value = string(w[header : header+body])
	}
	return value, length, header + body, nil
}

func iso8583MTIPart(digit byte, names map[byte]string) string {
	if name, ok := names[digit]; ok {
		return name
	}
	return "Reserved"
}

var (
	iso8583MTIVersions = map[byte]string{'0': "ISO 8583:1987", '1': "ISO 8583:1993", '2': "ISO 8583:2003", '8': "National use", '9': "Private use"}
	iso8583MTIClasses  = map[byte]string{'1': "Authorization", '2': "Financial", '3': "File actions", '4': "Reversal and charge-back", '5': "Reconciliation", '6': "Administrative", '7': "Fee collection", '8': "Network management"}
	iso8583MTIFuncs    = map[byte]string{'0': "Request", '1': "Request response", '2': "Advice", '3': "Advice response", '4': "Notification", '5': "Notification acknowledgement", '6': "Instruction", '7': "Instruction acknowledgement"}
	iso8583MTIOrigins  = map[byte]string{'0': "Acquirer", '1': "Acquirer repeat", '2': "Issuer", '3': "Issuer repeat", '4': "Other"}
)

// decodeISO8583Message returns the owned field tree for one complete message
// body (without the length prefix).
func decodeISO8583Message(body []byte, budget ParserBudget) (map[string]any, error) {
	env, err := parseISO8583Envelope(body)
	if err != nil {
		return nil, err
	}
	encoding := "ascii"
	if env.Encoding == iso8583BCD {
		encoding = "bcd"
	}
	out := map[string]any{
		"MTI":              env.MTI,
		"MTI Version":      iso8583MTIPart(env.MTI[0], iso8583MTIVersions),
		"MTI Class":        iso8583MTIPart(env.MTI[1], iso8583MTIClasses),
		"MTI Function":     iso8583MTIPart(env.MTI[2], iso8583MTIFuncs),
		"MTI Origin":       iso8583MTIPart(env.MTI[3], iso8583MTIOrigins),
		"Encoding":         encoding,
		"Bitmap":           strings.ToUpper(hex.EncodeToString(env.Bitmap)),
		"Secondary Bitmap": len(env.Bitmap) > 8,
	}
	offset := env.Offset
	elements := map[string]any{}
	order := make([]any, 0, 16)
	limit := budget.MaxCollectionElements
	if limit <= 0 {
		limit = DefaultParserBudget().MaxCollectionElements
	}
	for index := 0; index < len(env.Bitmap)*8; index++ {
		if env.Bitmap[index/8]&(0x80>>uint(index%8)) == 0 {
			continue
		}
		field := index + 1
		if field == 1 || field == 65 {
			continue
		}
		if len(order) >= limit {
			return nil, protocolError(ErrResourceExceeded, "iso8583: data element count exceeds the collection budget")
		}
		spec, ok := iso8583FieldSpecs[field]
		if !ok {
			return nil, protocolError(ErrUnsupportedFeature, fmt.Sprintf("iso8583: data element %d has no published definition", field))
		}
		// ISO 8583:1987 defines element 39 as two alphanumeric characters; the
		// 1993 and 2003 revisions widen it to three numeric digits. Both public
		// captures of one exchange pin this: every 1987 message aligns at two
		// characters and the single 1200 advice at three.
		if field == 39 && (env.MTI[0] == '1' || env.MTI[0] == '2') {
			spec = iso8583FieldSpec{Name: spec.Name, Type: iso8583Numeric, Length: iso8583Fixed, Max: 3}
		}
		value, length, consumed, err := iso8583ReadField(body[offset:], field, spec, env.Encoding)
		if err != nil {
			return nil, err
		}
		offset += consumed
		element := map[string]any{
			"Name":        spec.Name,
			"Value":       value,
			"Type":        iso8583TypeName(spec.Type),
			"Length":      length,
			"Length Type": iso8583LengthName(spec.Length),
		}
		if spec.Sensitive {
			element["Sensitive"] = true
		}
		elements[strconv.Itoa(field)] = element
		order = append(order, field)
	}
	if offset != len(body) {
		return nil, protocolError(ErrMalformedMessage, fmt.Sprintf("iso8583: %d bytes remain outside the declared data elements", len(body)-offset))
	}
	out["Data Elements"] = elements
	out["Data Element Order"] = order
	return out, nil
}

// probeISO8583 admits a two-byte length-prefixed ISO 8583 stream. It validates
// the complete first message (envelope and every set element) so a weak
// "four digits then hex" prefix cannot claim unrelated text. The bool reports
// the length-prefix byte order that admitted the stream.
func probeISO8583(w []byte, limit int) (ProbeResult, bool) {
	if len(w) < 2 {
		return ProbeResult{Verdict: ProbeReject}, false
	}
	for _, bigEndian := range []bool{false, true} {
		length := iso8583Length(w, bigEndian)
		if length < 12 || length > limit {
			continue
		}
		total := 2 + length
		body := w[2:]
		if len(body) < length {
			// Envelope bytes that can never become valid (a reserved MTI
			// version, a non-hex ASCII bitmap) reject the carrier instead of
			// holding the flow open for more bytes.
			if _, err := parseISO8583Envelope(body); err != nil {
				if status, _ := classifySessionError(err); status != "incomplete" {
					continue
				}
			}
			return probeNeed("iso8583", "", len(w), total), bigEndian
		}
		if _, err := decodeISO8583Message(body[:length], DefaultParserBudget()); err != nil {
			continue
		}
		return probeAccept("iso8583", "", 88), bigEndian
	}
	return ProbeResult{Verdict: ProbeReject}, false
}

func (f *binFlow) frameISO8583(w []byte) (int, *binSpec, error) {
	s := f.iso8583
	if s == nil {
		return 0, nil, sessionContext("ISO 8583 session was not observed")
	}
	if err := f.reserveSession(1024); err != nil {
		return 0, nil, err
	}
	if len(w) < 2 {
		return 0, nil, nil
	}
	length := iso8583Length(w, s.bigEndian)
	// Only the carrier is validated here. A complete but too-short body is a
	// message-level refusal so both encodings of the same exchange classify it
	// identically instead of splitting between framing and semantic errors.
	if length < 1 {
		return 0, nil, protocolError(ErrMalformedMessage, "iso8583: empty carried message")
	}
	if length > f.a.config.MaxMessageBytes {
		return 0, nil, fmt.Errorf("iso8583: declared message length %d exceeds the message byte budget", length)
	}
	total := 2 + length
	if len(w) < total {
		return 0, nil, nil
	}
	return total, &binSpec{}, nil
}
