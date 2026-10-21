package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

const mmsServicesProfile = "mms-iso-services-values"

type mmsValueContext struct {
	left    int
	state   *binMMS
	dir     int
	reserve func(int64) error
}

func (m *binMMS) profile() string {
	if m.validateDescriptions {
		return "mms-iso-described-data"
	}
	if m.values {
		return mmsServicesProfile
	}
	return mmsISOProfile
}
func (m *binMMS) projectionFactor(b ParserBudget) int64 {
	if m.values {
		return 512 + 16*int64(b.MaxRecursionDepth)
	}
	return 512
}
func WithProtocolMMSServiceValues(port uint16) CaptureOption {
	return func(c *CaptureConfig) error {
		if err := WithProtocolMMSISO(port)(c); err != nil {
			return err
		}
		c.binParserConfig.DecodeMMSServiceValues = true
		return nil
	}
}
func WithSessionMMSServiceValues() ProtocolSessionOption {
	return func(s *captureSession) error {
		selectedDescriptions := s.f.mms != nil && s.f.mms.validateDescriptions
		if err := WithSessionMMSISO()(s); err != nil {
			return err
		}
		s.f.mms.values = true
		s.f.mms.validateDescriptions = selectedDescriptions
		return nil
	}
}
func mmsSelectedService(pdu, tag byte) bool {
	if pdu == 0xa0 {
		return tag == 0x80 || tag == 0xa1 || tag == 0x82 || tag == 0xa4 || tag == 0xa5 || tag == 0xa6 || tag == 0xab || tag == 0xac || tag == 0xad
	}
	return tag == 0xa0 || tag == 0xa1 || tag == 0xa2 || tag == 0xa4 || tag == 0xa5 || tag == 0xa6 || tag == 0x8b || tag == 0xac || tag == 0xad
}
func mmsBoolean(n mmsBER) (bool, error) {
	if len(n.body) != 1 {
		return false, protocolError(ErrMalformedMessage, "MMS BOOLEAN requires one content octet")
	}
	return n.body[0] != 0, nil
}
func mmsVisible(w []byte) (string, error) {
	for _, c := range w {
		if c < 32 || c > 126 {
			return "", protocolError(ErrMalformedMessage, "MMS VisibleString alphabet")
		}
	}
	return string(w), nil
}
func mmsInteger(n mmsBER, unsigned bool) (string, error) {
	b := n.body
	if len(b) == 0 {
		return "", protocolError(ErrMalformedMessage, "MMS INTEGER is empty")
	}
	if len(b) > 1 && (b[0] == 0 && b[1]&128 == 0 || b[0] == 255 && b[1]&128 != 0) {
		return "", protocolError(ErrMalformedMessage, "MMS INTEGER redundant sign octet")
	}
	if unsigned && b[0]&128 != 0 {
		return "", protocolError(ErrMalformedMessage, "MMS unsigned INTEGER is negative")
	}
	v := new(big.Int).SetBytes(b)
	if !unsigned && b[0]&128 != 0 {
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), uint(8*len(b))))
	}
	return v.String(), nil
}
func (c *mmsValueContext) oid(b []byte) (string, error) {
	if len(b) == 0 {
		return "", protocolError(ErrMalformedMessage, "MMS empty OID")
	}
	var parts []string
	for at := 0; at < len(b); {
		if c.left <= 0 {
			return "", protocolError(ErrResourceExceeded, "MMS shared BER/OID arc budget")
		}
		c.left--
		if b[at] == 128 {
			return "", protocolError(ErrMalformedMessage, "MMS OID nonminimal subidentifier")
		}
		var value uint64
		for {
			if at >= len(b) {
				return "", protocolError(ErrMalformedMessage, "MMS unfinished OID subidentifier")
			}
			x := b[at]
			at++
			if value > math.MaxUint64>>7 {
				return "", protocolError(ErrResourceExceeded, "MMS OID arc exceeds bounded uint64 projection")
			}
			value = value<<7 | uint64(x&127)
			if x&128 == 0 {
				break
			}
		}
		if len(parts) == 0 {
			if value < 40 {
				parts = append(parts, "0", strconv.FormatUint(value, 10))
			} else if value < 80 {
				parts = append(parts, "1", strconv.FormatUint(value-40, 10))
			} else {
				parts = append(parts, "2", strconv.FormatUint(value-80, 10))
			}
		} else {
			parts = append(parts, strconv.FormatUint(value, 10))
		}
	}
	return strings.Join(parts, "."), nil
}
func (c *mmsValueContext) data(n mmsBER) (map[string]any, uint32, error) {
	f := map[string]any{"Tag": n.tag}
	depth := uint32(0)
	switch n.tag {
	case 0x80:
		return nil, 0, protocolError(ErrMalformedMessage, "MMS AccessResult failure is not a nested Data choice")
	case 0xa1, 0xa2:
		f["Type"] = map[byte]string{0xa1: "array", 0xa2: "structure"}[n.tag]
		values := make([]map[string]any, 0, len(n.children))
		for _, v := range n.children {
			child, d, e := c.data(v)
			if e != nil {
				return nil, 0, e
			}
			depth = max(depth, d)
			values = append(values, child)
		}
		f["Values"] = values
		depth++
	case 0x83:
		v, e := mmsBoolean(n)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Value"] = "boolean", v
	case 0x84, 0x8e:
		if e := mmsBits(n); e != nil {
			return nil, 0, e
		}
		f["Type"] = map[byte]string{0x84: "bit-string", 0x8e: "boolean-array"}[n.tag]
		f["Unused Bits"], f["Bits"], f["Bit Count"] = n.body[0], bytes.Clone(n.body[1:]), (len(n.body)-1)*8-int(n.body[0])
	case 0x85, 0x86, 0x8d:
		v, e := mmsInteger(n, n.tag == 0x86)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Value Decimal"] = map[byte]string{0x85: "integer", 0x86: "unsigned", 0x8d: "bcd"}[n.tag], v
	case 0x87:
		f["Type"] = "floating-point"
		switch {
		case len(n.body) == 5 && n.body[0] == 8:
			bits := binary.BigEndian.Uint32(n.body[1:])
			f["Exponent Width"], f["Width Bits"], f["Bits Hex"], f["Value"] = 8, 32, hex.EncodeToString(n.body[1:]), strconv.FormatFloat(float64(math.Float32frombits(bits)), 'g', -1, 32)
		case len(n.body) == 9 && n.body[0] == 11:
			bits := binary.BigEndian.Uint64(n.body[1:])
			f["Exponent Width"], f["Width Bits"], f["Bits Hex"], f["Value"] = 11, 64, hex.EncodeToString(n.body[1:]), strconv.FormatFloat(math.Float64frombits(bits), 'g', -1, 64)
		default:
			return nil, 0, protocolError(ErrUnsupportedFeature, "MMS selected IEEE754 binary32/binary64 FloatingPoint layout")
		}
	case 0x89:
		f["Type"], f["Bytes"] = "octet-string", bytes.Clone(n.body)
	case 0x8a:
		s, e := mmsVisible(n.body)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Text"] = "visible-string", s
	case 0x8c:
		if len(n.body) != 4 && len(n.body) != 6 {
			return nil, 0, protocolError(ErrMalformedMessage, "MMS TimeOfDay requires four or six octets")
		}
		ms := binary.BigEndian.Uint32(n.body)
		f["Type"], f["Milliseconds"], f["Within One Day"] = "binary-time", ms, ms < 86400000
		if len(n.body) == 6 {
			days := binary.BigEndian.Uint16(n.body[4:])
			f["Days Since 1984"], f["Unix Milliseconds"] = days, int64(days)*86400000+441763200000+int64(ms)
		}
	case 0x8f:
		v, e := c.oid(n.body)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["OID"] = "object-identifier", v
	case 0x90:
		if !utf8.Valid(n.body) {
			return nil, 0, protocolError(ErrMalformedMessage, "MMSString is not valid UTF8")
		}
		f["Type"], f["Text"] = "mms-string", string(n.body)
	case 0x91:
		if len(n.body) != 8 {
			return nil, 0, protocolError(ErrMalformedMessage, "MMS UtcTime requires eight octets")
		}
		seconds := binary.BigEndian.Uint32(n.body)
		fraction := uint32(n.body[4])<<16 | uint32(n.body[5])<<8 | uint32(n.body[6])
		f["Type"], f["Seconds"], f["Fraction24"], f["Nanoseconds"], f["Quality Byte"] = "utc-time", seconds, fraction, uint64(fraction)*1000000000>>24, n.body[7]
	default:
		return nil, 0, protocolError(ErrUnsupportedFeature, "MMS Data choice outside selected grammar")
	}
	return f, depth, nil
}
func mmsVariableNames(wrapper mmsBER, extended bool) ([]map[string]any, error) {
	ls, e := mmsSingleChild(wrapper)
	if e != nil {
		return nil, e
	}
	if ls.tag != 0xa0 {
		return nil, protocolError(ErrContextRequired, "MMS named variable-list definition not observed")
	}
	names := make([]map[string]any, 0, len(ls.children))
	for _, v := range ls.children {
		if v.tag != 0x30 || len(v.children) != 1 || v.children[0].tag != 0xa0 {
			return nil, protocolError(ErrUnsupportedFeature, "MMS named VariableSpecification without alternate access selected")
		}
		name, e := mmsSingleChild(v.children[0])
		if e != nil {
			return nil, e
		}
		f, e := mmsNamed(name, extended)
		if e != nil {
			return nil, e
		}
		names = append(names, f)
	}
	if len(names) == 0 {
		return nil, protocolError(ErrMalformedMessage, "MMS empty selected listOfVariable")
	}
	return names, nil
}
func (c *mmsValueContext) typeSpec(n mmsBER) (map[string]any, uint32, error) {
	f := map[string]any{"Tag": n.tag}
	depth := uint32(0)
	switch n.tag {
	case 0xa0:
		child, e := mmsSingleChild(n)
		if e != nil {
			return nil, 0, e
		}
		name, e := mmsNamed(child, true)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Name"] = "named-type", name
	case 0xa1, 0xa2:
		packed := false
		for _, v := range n.children {
			if v.tag == 0x80 {
				b, e := mmsBoolean(v)
				if e != nil {
					return nil, 0, e
				}
				packed = b
			}
		}
		f["Packed"] = packed
		if n.tag == 0xa1 {
			if e := mmsSchema(n, []byte{0x81, 0xa2}, []byte{0x80}); e != nil {
				return nil, 0, e
			}
			count, _ := mmsChild(n, 0x81)
			size, e := mmsUInt(count, 32)
			if e != nil {
				return nil, 0, e
			}
			el, _ := mmsChild(n, 0xa2)
			v, e := mmsSingleChild(el)
			if e != nil {
				return nil, 0, e
			}
			element, d, e := c.typeSpec(v)
			if e != nil {
				return nil, 0, e
			}
			f["Type"], f["Number Of Elements"], f["Element Type"] = "array", size, element
			depth = d + 1
		} else {
			if e := mmsSchema(n, []byte{0xa1}, []byte{0x80}); e != nil {
				return nil, 0, e
			}
			ls, _ := mmsChild(n, 0xa1)
			components := make([]map[string]any, 0, len(ls.children))
			for _, v := range ls.children {
				if v.tag != 0x30 {
					return nil, 0, protocolError(ErrMalformedMessage, "MMS type component SEQUENCE")
				}
				if e := mmsSchema(v, []byte{0xa1}, []byte{0x80}); e != nil {
					return nil, 0, e
				}
				x := map[string]any{}
				for _, z := range v.children {
					if z.tag == 0x80 {
						text, e := mmsVisible(z.body)
						if e != nil {
							return nil, 0, e
						}
						x["Name"] = text
					}
				}
				tr, _ := mmsChild(v, 0xa1)
				tr, e := mmsSingleChild(tr)
				if e != nil {
					return nil, 0, e
				}
				typed, d, e := c.typeSpec(tr)
				if e != nil {
					return nil, 0, e
				}
				depth = max(depth, d)
				x["Type Specification"] = typed
				components = append(components, x)
			}
			f["Type"], f["Components"] = "structure", components
			depth++
		}
	case 0x83, 0x8b, 0x8f, 0x91:
		if len(n.body) != 0 {
			return nil, 0, protocolError(ErrMalformedMessage, "MMS TypeSpecification NULL has content")
		}
		f["Type"] = map[byte]string{0x83: "boolean", 0x8b: "generalized-time", 0x8f: "object-identifier", 0x91: "utc-time"}[n.tag]
	case 0x84, 0x89, 0x8a, 0x90:
		text, e := mmsInteger(n, false)
		if e != nil {
			return nil, 0, e
		}
		value := new(big.Int)
		value.SetString(text, 10)
		if !value.IsInt64() || value.Int64() < math.MinInt32 || value.Int64() > math.MaxInt32 {
			return nil, 0, protocolError(ErrMalformedMessage, "MMS TypeSpecification Integer32 width")
		}
		f["Type"], f["Size Constraint"] = map[byte]string{0x84: "bit-string", 0x89: "octet-string", 0x8a: "visible-string", 0x90: "mms-string"}[n.tag], value.Int64()
	case 0xa7:
		if len(n.children) != 2 || n.children[0].tag != 2 || n.children[1].tag != 2 {
			return nil, 0, protocolError(ErrMalformedMessage, "MMS floating TypeSpecification format/exponent INTEGER pair")
		}
		width, e := mmsUInt(n.children[0], 8)
		if e != nil {
			return nil, 0, e
		}
		exponent, e := mmsUInt(n.children[1], 8)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Width Bits"], f["Exponent Width"] = "floating-point", width, exponent
	case 0x85, 0x86, 0x8d:
		bits, e := mmsUInt(n, 8)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Width Bits"] = map[byte]string{0x85: "integer", 0x86: "unsigned", 0x8d: "bcd"}[n.tag], bits
	case 0x8c:
		date, e := mmsBoolean(n)
		if e != nil {
			return nil, 0, e
		}
		f["Type"], f["Date Present"] = "binary-time", date
	default:
		return nil, 0, protocolError(ErrUnsupportedFeature, "MMS TypeSpecification outside selected grammar")
	}
	return f, depth, nil
}
func (c *mmsValueContext) service(n mmsBER, d *mmsDecoded) error {
	s := n.children[1]
	if d.tag == 0xa0 {
		switch d.service {
		case 0:
			b, e := mmsBoolean(s)
			if e != nil {
				return e
			}
			d.fields["Extended Derivation"] = b
		case 2:
			if len(s.body) != 0 {
				return protocolError(ErrMalformedMessage, "MMS Identify NULL has content")
			}
		}
		return nil
	}
	switch d.service {
	case 0:
		if e := mmsSchema(s, []byte{0x80, 0x81}, []byte{0x82}); e != nil {
			return e
		}
		for _, key := range []struct {
			tag  byte
			name string
		}{{0x80, "VMD Logical Status"}, {0x81, "VMD Physical Status"}} {
			v, _ := mmsChild(s, key.tag)
			num, e := mmsUInt(v, 31)
			if e != nil {
				return e
			}
			if num > 3 {
				return protocolError(ErrUnsupportedFeature, "MMS unselected VMD status code")
			}
			d.fields[key.name] = num
		}
		for _, v := range s.children {
			if v.tag == 0x82 {
				if e := mmsBits(v); e != nil {
					return e
				}
				bits := (len(v.body)-1)*8 - int(v.body[0])
				if bits > 128 {
					return protocolError(ErrMalformedMessage, "MMS Status localDetail exceeds 128 bits")
				}
				d.fields["Local Detail Bits"] = map[string]any{"Unused Bits": v.body[0], "Bits": bytes.Clone(v.body[1:]), "Bit Count": bits}
			}
		}
	case 2:
		if e := mmsSchema(s, []byte{0x80, 0x81, 0x82}, []byte{0xa3}); e != nil {
			return e
		}
		for _, k := range []struct {
			tag  byte
			name string
		}{{0x80, "Vendor Name"}, {0x81, "Model Name"}, {0x82, "Revision"}} {
			v, _ := mmsChild(s, k.tag)
			text, e := mmsVisible(v.body)
			if e != nil {
				return e
			}
			d.fields[k.name] = text
		}
		for _, v := range s.children {
			if v.tag == 0xa3 {
				oids := make([]string, 0, len(v.children))
				for _, z := range v.children {
					if z.tag != 6 {
						return protocolError(ErrMalformedMessage, "MMS Identify abstract syntax OID")
					}
					value, e := c.oid(z.body)
					if e != nil {
						return e
					}
					oids = append(oids, value)
				}
				d.fields["Abstract Syntaxes"] = oids
			}
		}
	case 6:
		if e := mmsSchema(s, []byte{0x80, 0xa2}, []byte{0xa1}); e != nil {
			return e
		}
		v, _ := mmsChild(s, 0x80)
		deletable, e := mmsBoolean(v)
		if e != nil {
			return e
		}
		d.fields["MMS Deletable"] = deletable
		tr, _ := mmsChild(s, 0xa2)
		tr, e = mmsSingleChild(tr)
		if e != nil {
			return e
		}
		typed, depth, e := c.typeSpec(tr)
		if e != nil {
			return e
		}
		d.fields["Type Specification"] = typed
		d.valueDepth = depth
		for _, a := range s.children {
			if a.tag == 0xa1 {
				address, e := mmsSingleChild(a)
				if e != nil {
					return e
				}
				f, e := mmsAddress(address)
				if e != nil {
					return e
				}
				d.fields["Address"] = f
			}
		}
	default:
		return protocolError(ErrFatalSessionError, "MMS internal service selection")
	}
	return nil
}

// Addresses are observed selectors, not verified device locations or permissions.
func mmsAddress(n mmsBER) (map[string]any, error) {
	f := map[string]any{"Choice Tag": n.tag}
	switch n.tag {
	case 0x80:
		value, err := mmsUInt(n, 32)
		if err != nil {
			return nil, err
		}
		f["Numeric"] = value
	case 0x81:
		value, err := mmsVisible(n.body)
		if err != nil {
			return nil, err
		}
		f["Symbolic"] = value
	case 0x82:
		f["Bytes"] = bytes.Clone(n.body)
	case 0xa1, 0xa2:
		joined := make([]byte, 0, len(n.body))
		var collect func(mmsBER) error
		collect = func(v mmsBER) error {
			if v.tag == 4 {
				joined = append(joined, v.body...)
				return nil
			}
			if v.tag != 0x24 {
				return protocolError(ErrMalformedMessage, "MMS constructed Address OCTET STRING segment")
			}
			for _, child := range v.children {
				if err := collect(child); err != nil {
					return err
				}
			}
			return nil
		}
		for _, child := range n.children {
			if err := collect(child); err != nil {
				return nil, err
			}
		}
		if n.tag == 0xa1 {
			value, err := mmsVisible(joined)
			if err != nil {
				return nil, err
			}
			f["Symbolic"] = value
		} else {
			f["Bytes"] = bytes.Clone(joined)
		}
	default:
		return nil, protocolError(ErrUnsupportedFeature, "MMS Address choice")
	}
	return f, nil
}

// WithProtocolMMSDescriptionValidation selects bounded checks against directly
// observed variableDescription types, without resolving named types/device truth.
func WithProtocolMMSDescriptionValidation(port uint16) CaptureOption {
	return func(c *CaptureConfig) error {
		if err := WithProtocolMMSServiceValues(port)(c); err != nil {
			return err
		}
		c.binParserConfig.ValidateMMSDescriptions = true
		return nil
	}
}

// WithSessionMMSDescriptionValidation preserves literal services while selecting
// bounded known-description checks; unresolved type/size context is refused.
func WithSessionMMSDescriptionValidation() ProtocolSessionOption {
	return func(s *captureSession) error {
		if err := WithSessionMMSServiceValues()(s); err != nil {
			return err
		}
		s.f.mms.validateDescriptions = true
		return nil
	}
}

// Description checks consume directly observed schemas. They do not resolve
// named types or establish namespace, memory-layout, endpoint or device truth.
func mmsDescriptionValue(typ, value map[string]any, left *int) error {
	if *left <= 0 {
		return protocolError(ErrResourceExceeded, "MMS description/value comparison budget")
	}
	*left--
	kind := typ["Type"].(string)
	if kind == "named-type" || kind == "bcd" || kind == "generalized-time" || kind == "mms-string" {
		return protocolError(ErrContextRequired, "MMS description requires independently resolved type/size context")
	}
	if value["Type"] != kind {
		return protocolError(ErrMalformedMessage, "MMS Data type differs from observed variableDescription")
	}
	switch kind {
	case "array":
		values := value["Values"].([]map[string]any)
		if uint64(len(values)) != uint64(typ["Number Of Elements"].(uint32)) {
			return protocolError(ErrMalformedMessage, "MMS described array cardinality")
		}
		for _, v := range values {
			if err := mmsDescriptionValue(typ["Element Type"].(map[string]any), v, left); err != nil {
				return err
			}
		}
	case "structure":
		values := value["Values"].([]map[string]any)
		components := typ["Components"].([]map[string]any)
		if len(values) != len(components) {
			return protocolError(ErrMalformedMessage, "MMS described structure cardinality")
		}
		for i, v := range values {
			if err := mmsDescriptionValue(components[i]["Type Specification"].(map[string]any), v, left); err != nil {
				return err
			}
		}
	case "integer", "unsigned":
		width := typ["Width Bits"].(uint32)
		if width == 0 {
			return protocolError(ErrContextRequired, "MMS zero integer width interpretation")
		}
		n, ok := new(big.Int).SetString(value["Value Decimal"].(string), 10)
		if !ok {
			return protocolError(ErrMalformedMessage, "MMS described integer decimal")
		}
		if kind == "unsigned" {
			if n.Sign() < 0 || n.BitLen() > int(width) {
				return protocolError(ErrMalformedMessage, "MMS described unsigned width")
			}
		} else {
			max := new(big.Int).Lsh(big.NewInt(1), uint(width-1))
			min := new(big.Int).Neg(new(big.Int).Set(max))
			max.Sub(max, big.NewInt(1))
			if n.Cmp(min) < 0 || n.Cmp(max) > 0 {
				return protocolError(ErrMalformedMessage, "MMS described signed width")
			}
		}
	case "bit-string", "octet-string", "visible-string":
		size := typ["Size Constraint"].(int64)
		if size < 0 {
			return protocolError(ErrContextRequired, "MMS variable-length bound interpretation needs primary context")
		}
		actual := int64(0)
		switch kind {
		case "bit-string":
			actual = int64(value["Bit Count"].(int))
		case "octet-string":
			actual = int64(len(value["Bytes"].([]byte)))
		case "visible-string":
			actual = int64(len(value["Text"].(string)))
		}
		if size != actual {
			return protocolError(ErrMalformedMessage, "MMS described fixed string/bit length")
		}
	case "floating-point":
		if typ["Width Bits"].(uint32) != uint32(value["Width Bits"].(int)) || typ["Exponent Width"].(uint32) != uint32(value["Exponent Width"].(int)) {
			return protocolError(ErrMalformedMessage, "MMS described floating format")
		}
	case "binary-time":
		_, date := value["Days Since 1984"]
		if date != typ["Date Present"].(bool) {
			return protocolError(ErrMalformedMessage, "MMS described binary-time date mode")
		}
	case "boolean", "object-identifier", "utc-time":
	default:
		return protocolError(ErrContextRequired, "MMS selected description/value type context")
	}
	return nil
}
func mmsDescriptionOperands(names, values []map[string]any, limit int) error {
	if len(names) != len(values) {
		return protocolError(ErrMalformedMessage, "MMS description/value cardinality")
	}
	for i, name := range names {
		if typ, ok := name["Type Specification"].(map[string]any); ok {
			if _, alternate := name["Alternate Access"]; alternate {
				return protocolError(ErrContextRequired, "MMS selected alternate description/Data interpretation")
			}
			if err := mmsDescriptionValue(typ, values[i], &limit); err != nil {
				return err
			}
		}
	}
	return nil
}
