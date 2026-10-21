package pcaputil

import "fmt"

type uaValueOptions struct {
	arrays, structured bool
	maxDepth           int
}

type uaDiagnosticRef struct {
	index int32
	key   string
	value map[string]any
}

func newUAValueCursor(w []byte, max int, reserve func(int64) error, options []uaValueOptions) *uaReadCursor {
	c := &uaReadCursor{wire: w, max: max, reserve: reserve, maxDepth: 16}
	if len(options) != 0 {
		o := options[0]
		c.arrays, c.structured = o.arrays || o.structured, o.structured
		if o.maxDepth > 0 {
			c.maxDepth = o.maxDepth
		}
	}
	return c
}

func (s *binOPCUA) valueOptions() uaValueOptions {
	return uaValueOptions{arrays: s.arrays, structured: s.structured, maxDepth: s.maxDepth}
}

// The structured profile bounds actual value-graph containers, separately
// from message bytes. Existing scalar profiles retain their collection contract.
func (c *uaReadCursor) enterValue() bool {
	if c.err != nil {
		return false
	}
	if !c.structured {
		return true
	}
	if c.depth >= c.maxDepth || c.elements >= c.max {
		c.err = protocolError(ErrResourceExceeded, "OPC UA value graph depth/elements")
		return false
	}
	c.depth++
	c.elements++
	return true
}

func (c *uaReadCursor) leaveValue() {
	if c.structured {
		c.depth--
	}
}

func (c *uaReadCursor) diagnostic() map[string]any {
	if c.structured {
		if c.diagDepth >= 10 {
			c.err = protocolError(ErrResourceExceeded, "OPC UA DiagnosticInfo recursion")
			return nil
		}
		c.diagDepth++
		defer func() { c.diagDepth-- }()
	}
	mask := byte(c.u(1))
	if c.err != nil {
		return nil
	}
	if mask&0x80 != 0 {
		c.err = fmt.Errorf("opcua: Read DiagnosticInfo reserved bit")
		return nil
	}
	if mask != 0 && !c.structured {
		c.err = protocolError(ErrUnsupportedFeature, "OPC UA Read nonempty DiagnosticInfo")
		return nil
	}
	fields := 1
	if mask != 0 {
		if !c.enterValue() {
			return nil
		}
		defer c.leaveValue()
		fields = 8
	}
	m := c.object(fields)
	if c.err != nil {
		return nil
	}
	m["Encoding"] = mask
	// Wire order is Locale before LocalizedText, despite the reversed mask bits.
	for _, field := range []struct {
		mask byte
		key  string
	}{{1, "Symbolic ID"}, {2, "Namespace URI"}, {8, "Locale"}, {4, "Localized Text"}} {
		key := field.key
		if mask&field.mask != 0 {
			index := int32(c.u(4))
			m[key+" Index"] = index
			if c.err == nil && index < -1 {
				c.err = fmt.Errorf("opcua: DiagnosticInfo negative string table index")
			}
			if c.err == nil && index >= 0 {
				// Charge append capacity and later resolved-map entries before
				// allocation. All references belong to this complete message.
				c.charge(144)
				if c.err == nil {
					c.diagRefs = append(c.diagRefs, uaDiagnosticRef{index, key, m})
				}
			}
		}
	}
	if mask&0x10 != 0 {
		m["Additional Info"] = c.blob(true)
	}
	if mask&0x20 != 0 {
		m["Inner Status Code"] = uint32(c.u(4))
	}
	if mask&0x40 != 0 {
		m["Inner Diagnostic Info"] = c.diagnostic()
	}
	return m
}

func (c *uaReadCursor) finishDiagnostics() {
	if c.err != nil {
		return
	}
	for _, ref := range c.diagRefs {
		if !c.tableSeen {
			c.err = protocolError(ErrContextRequired, "OPC UA DiagnosticInfo string table not observed")
			return
		}
		if int64(ref.index) >= int64(len(c.stringTable)) {
			c.err = protocolError(ErrDesynchronized, "OPC UA DiagnosticInfo string table index")
			return
		}
		text, ok := c.stringTable[ref.index].(string)
		if !ok {
			c.err = protocolError(ErrContextRequired, "OPC UA DiagnosticInfo null string table entry")
			return
		}
		ref.value[ref.key] = text
	}
}
