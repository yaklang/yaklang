package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"unicode/utf8"
)

// uaReadCursor projects a finite UA Binary Read profile. Counts are charged
// before allocation, across the entire message; DateTime stays signed wire ticks.
type uaReadCursor struct {
	wire                       []byte
	arrays, structured         bool
	depth, maxDepth, diagDepth int
	diagRefs                   []uaDiagnosticRef
	stringTable                []any
	tableSeen                  bool
	at, elements, max          int
	memory                     int64
	reserve                    func(int64) error
	err                        error
}

func (c *uaReadCursor) charge(n int64) {
	if c.err != nil {
		return
	}
	c.memory += n
	if c.reserve != nil {
		c.err = c.reserve(c.memory)
	}
}
func (c *uaReadCursor) object(fields int) map[string]any {
	c.charge(int64(128 + fields*80))
	if c.err != nil {
		return nil
	}
	return make(map[string]any, fields)
}
func (c *uaReadCursor) take(n int) []byte {
	if c.err != nil {
		return nil
	}
	if n < 0 || n > len(c.wire)-c.at {
		c.err = fmt.Errorf("opcua: Read field boundary")
		return nil
	}
	b := c.wire[c.at : c.at+n]
	c.at += n
	return b
}
func (c *uaReadCursor) u(n int) uint64 {
	b := c.take(n)
	if b == nil {
		return 0
	}
	switch n {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(b))
	case 4:
		return uint64(binary.LittleEndian.Uint32(b))
	case 8:
		return binary.LittleEndian.Uint64(b)
	}
	panic("internal UA integer width")
}
func (c *uaReadCursor) blob(text bool) any {
	n := int32(c.u(4))
	if c.err != nil {
		return nil
	}
	if n == -1 {
		return nil
	}
	if n < 0 || int64(n) > int64(len(c.wire)-c.at) {
		c.err = fmt.Errorf("opcua: Read string/bytes length")
		return nil
	}
	c.charge(int64(n) * 2)
	b := c.take(int(n))
	if c.err != nil {
		return nil
	}
	if text {
		if !utf8.Valid(b) {
			c.err = fmt.Errorf("opcua: Read invalid UTF-8")
			return nil
		}
		return string(b)
	}
	return hex.EncodeToString(b)
}
func (c *uaReadCursor) array(minBytes int) (int, bool) {
	n := int32(c.u(4))
	if c.err != nil {
		return 0, false
	}
	if n == -1 {
		return 0, true
	}
	if n < 0 {
		c.err = fmt.Errorf("opcua: Read array length")
		return 0, false
	}
	if int64(n) > int64(c.max-c.elements) {
		c.err = protocolError(ErrResourceExceeded, "OPC UA Read collection elements")
		return 0, false
	}
	if minBytes > 0 && int64(n) > int64((len(c.wire)-c.at)/minBytes) {
		c.err = fmt.Errorf("opcua: Read array boundary")
		return 0, false
	}
	c.elements += int(n)
	c.charge(int64(n) * 24)
	return int(n), false
}
func (c *uaReadCursor) node() map[string]any {
	return c.nodeFlags(false)
}
func (c *uaReadCursor) nodeFlags(expanded bool) map[string]any {
	m := c.object(5)
	enc := byte(c.u(1))
	if c.err != nil {
		return nil
	}
	m["Encoding"] = enc
	if !expanded && enc&0xc0 != 0 {
		c.err = fmt.Errorf("opcua: plain NodeId has expanded flags")
		return m
	}
	switch enc & 0x3f {
	case 0:
		m["Namespace"] = uint16(0)
		m["Identifier"] = uint32(c.u(1))
	case 1:
		m["Namespace"] = uint16(c.u(1))
		m["Identifier"] = uint32(c.u(2))
	case 2:
		m["Namespace"] = uint16(c.u(2))
		m["Identifier"] = uint32(c.u(4))
	case 3:
		m["Namespace"] = uint16(c.u(2))
		m["Identifier"] = c.blob(true)
	case 4:
		m["Namespace"] = uint16(c.u(2))
		m["Identifier Wire Hex"] = hex.EncodeToString(c.take(16))
	case 5:
		m["Namespace"] = uint16(c.u(2))
		m["Identifier"] = c.blob(false)
	default:
		c.err = fmt.Errorf("opcua: Read NodeId encoding")
	}
	if expanded {
		if enc&0x80 != 0 {
			m["Namespace URI"] = c.blob(true)
		}
		if enc&0x40 != 0 {
			m["Server Index"] = uint32(c.u(4))
		}
	}
	return m
}
func (c *uaReadCursor) extension() map[string]any {
	m := c.object(2)
	if c.err != nil {
		return nil
	}
	m["Type ID"] = c.node()
	enc := byte(c.u(1))
	m["Encoding"] = enc
	// Encoded custom bodies require their DataType schema. Keep a typed refusal,
	// rather than inferring authentication or silently ignoring custom headers.
	if c.err == nil && enc != 0 {
		if enc == 1 || enc == 2 {
			if c.structured {
				c.charge(160)
				m["Body Wire Hex"] = c.blob(false)
				m["Semantic Status"] = "extension-object-body-opaque"
			} else {
				c.err = protocolError(ErrUnsupportedFeature, "OPC UA Read encoded AdditionalHeader")
			}
		} else {
			c.err = fmt.Errorf("opcua: Read ExtensionObject encoding")
		}
	}
	return m
}
func (c *uaReadCursor) header(response bool) map[string]any {
	m := c.object(8)
	if c.err != nil {
		return nil
	}
	if !response {
		m["Authentication Token"] = c.node()
	}
	m["Timestamp Ticks"] = int64(c.u(8))
	m["Request Handle"] = uint32(c.u(4))
	if response {
		m["Service Result"] = uint32(c.u(4))
		m["Service Diagnostics"] = c.diagnostic()
		n, null := c.array(4)
		if !null && c.err == nil {
			values := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				values = append(values, c.blob(true))
			}
			m["String Table"] = values
		} else {
			m["String Table"] = nil
		}
		c.tableSeen = true
		c.stringTable, _ = m["String Table"].([]any)
	} else {
		m["Return Diagnostics"] = uint32(c.u(4))
		m["Audit Entry ID"] = c.blob(true)
		m["Timeout Hint"] = uint32(c.u(4))
	}
	m["Additional Header"] = c.extension()
	return m
}
func (c *uaReadCursor) variant() map[string]any {
	if !c.enterValue() {
		return nil
	}
	defer c.leaveValue()
	m := c.object(4)
	if c.err != nil {
		return nil
	}
	enc := byte(c.u(1))
	typ := enc & 63
	m["Encoding"] = enc
	m["Type ID"] = typ
	if enc&0xc0 != 0 {
		if !c.arrays {
			c.err = protocolError(ErrUnsupportedFeature, "OPC UA Read array/dimensional Variant")
			return m
		}
		return c.variantArray(enc, typ, m)
	}
	return c.variantScalar(typ, m)
}

func (c *uaReadCursor) variantScalar(typ byte, m map[string]any, arrayElement ...bool) map[string]any {
	switch typ {
	case 0:
		m["Value"] = nil
	case 1:
		m["Value"] = c.u(1) != 0
	case 2:
		m["Value"] = int8(c.u(1))
	case 3:
		m["Value"] = uint8(c.u(1))
	case 4:
		m["Value"] = int16(c.u(2))
	case 5:
		m["Value"] = uint16(c.u(2))
	case 6:
		m["Value"] = int32(c.u(4))
	case 7:
		m["Value"] = uint32(c.u(4))
	case 8:
		m["Value"] = int64(c.u(8))
	case 9:
		m["Value"] = c.u(8)
	case 10:
		bits := uint32(c.u(4))
		m["IEEE Bits"] = bits
		v := math.Float32frombits(bits)
		if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
			m["Value"] = v
		}
	case 11:
		bits := c.u(8)
		m["IEEE Bits"] = bits
		v := math.Float64frombits(bits)
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			m["Value"] = v
		}
	case 12:
		m["Value"] = c.blob(true)
	case 13:
		m["Value"] = int64(c.u(8))
	case 14:
		m["Value Wire Hex"] = hex.EncodeToString(c.take(16))
	case 15:
		m["Value"] = c.blob(false)
	case 16:
		// Part 6 5.2.2.8: UTF-8 serialized XML carried as a ByteString.
		// No XML parser, entity resolution or grammar-validity claim is made.
		m["Value"] = c.blob(true)
		m["Semantic Status"] = "xml-wire-text-observation"
	case 17:
		m["Value"] = c.node()
	case 18:
		value := c.nodeFlags(true)
		if c.err == nil && value["Encoding"].(byte)&0x80 != 0 {
			if uri, ok := value["Namespace URI"].(string); !ok || uri == "" {
				c.err = fmt.Errorf("opcua: ExpandedNodeId URI flag without URI")
				return m
			}
			// Retain the literal wire index, but explicitly mark its lack of
			// namespace authority when a URI is present (Part 6 5.2.2.10).
			c.charge(80)
			if c.err == nil {
				value["Namespace Index Ignored"] = true
			}
		}
		m["Value"] = value
	case 19:
		m["Value"] = uint32(c.u(4))
	case 20:
		m["Value"] = c.qualifiedName()
	case 21:
		m["Value"] = c.localizedText()
	case 22, 23, 24, 25:
		if !c.structured {
			c.err = protocolError(ErrUnsupportedFeature, "OPC UA structured Variant")
			return m
		}
		switch typ {
		case 22:
			m["Value"] = c.extension()
		case 23:
			m["Value"] = c.value()
		case 24:
			if len(arrayElement) == 0 || !arrayElement[0] {
				c.err = fmt.Errorf("opcua: scalar Variant cannot contain Variant")
				return m
			}
			m["Value"] = c.variant()
		case 25:
			m["Value"] = c.diagnostic()
		}
	case 26, 27, 28, 29, 30, 31:
		// Part 6 5.2.2.16 explicitly requires forward-compatible byte
		// preservation for these IDs. Their future application meaning is
		// unknown; do not project a known DataType or decode an object body.
		m["Value"] = c.blob(false)
		m["Semantic Status"] = "future-type-bytes-observation"
	default:
		c.err = protocolError(ErrUnsupportedFeature, "OPC UA Read Variant built-in type")
	}
	return m
}
func (c *uaReadCursor) value() map[string]any {
	if !c.enterValue() {
		return nil
	}
	defer c.leaveValue()
	m := c.object(7)
	if c.err != nil {
		return nil
	}
	mask := byte(c.u(1))
	m["Encoding"] = mask
	if mask&0xc0 != 0 {
		c.err = fmt.Errorf("opcua: Read DataValue reserved bits")
		return m
	}
	if mask&1 != 0 {
		m["Value"] = c.variant()
	}
	if mask&2 != 0 {
		m["Status Code"] = uint32(c.u(4))
	}
	if mask&4 != 0 {
		m["Source Timestamp Ticks"] = int64(c.u(8))
	}
	if mask&8 != 0 {
		m["Server Timestamp Ticks"] = int64(c.u(8))
	}
	if mask&16 != 0 {
		m["Source Picoseconds"] = uint16(c.u(2))
	}
	if mask&32 != 0 {
		m["Server Picoseconds"] = uint16(c.u(2))
	}
	return m
}
func decodeOPCUARead(w []byte, response bool, max int, reserve func(int64) error, options ...uaValueOptions) (map[string]any, error) {
	c := newUAValueCursor(w, max, reserve, options)
	out := c.object(5)
	if c.err != nil {
		return nil, c.err
	}
	out["Header"] = c.header(response)
	if response {
		n, null := c.array(1)
		if !null && c.err == nil {
			a := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				a = append(a, c.value())
			}
			out["Results"] = a
		} else {
			out["Results"] = nil
		}
		n, null = c.array(1)
		if !null && c.err == nil {
			a := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				a = append(a, c.diagnostic())
			}
			out["Diagnostic Infos"] = a
		} else {
			out["Diagnostic Infos"] = nil
		}
	} else {
		bits := c.u(8)
		out["Max Age IEEE Bits"] = bits
		v := math.Float64frombits(bits)
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			out["Max Age"] = v
		}
		out["Timestamps To Return"] = int32(c.u(4))
		n, null := c.array(12)
		if !null && c.err == nil {
			a := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				m := c.object(4)
				if c.err != nil {
					break
				}
				m["Node ID"] = c.node()
				m["Attribute ID"] = uint32(c.u(4))
				m["Index Range"] = c.blob(true)
				q := c.object(2)
				if c.err != nil {
					break
				}
				q["Namespace"] = uint16(c.u(2))
				q["Name"] = c.blob(true)
				m["Data Encoding"] = q
				a = append(a, m)
			}
			out["Nodes To Read"] = a
		} else {
			out["Nodes To Read"] = nil
		}
	}
	c.finishDiagnostics()
	if c.err != nil {
		return nil, c.err
	}
	if c.at != len(w) {
		return nil, fmt.Errorf("opcua: Read trailing bytes")
	}
	return out, nil
}
