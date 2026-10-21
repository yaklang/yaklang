package pcaputil

import "fmt"

// variantArray retains scalar element projections in wire order. Part 6
// 5.2.2.16 requires positive dimensions (rank >= 2), whose product must equal
// the flattened array length. Dimensions are observations, not ValueRank or
// Method-signature validation. Nested Variant/ExtensionObject arrays need their
// own bounded schemas and remain unsupported here.
func (c *uaReadCursor) variantArray(enc, typ byte, out map[string]any) map[string]any {
	if enc&0x80 == 0 || typ == 0 {
		c.err = fmt.Errorf("opcua: Variant array encoding mask")
		return out
	}
	widths := [...]int{1: 1, 2: 1, 3: 1, 4: 2, 5: 2, 6: 4, 7: 4, 8: 8, 9: 8, 10: 4, 11: 8, 12: 4, 13: 8, 14: 16, 15: 4, 16: 4, 17: 2, 18: 2, 19: 4, 20: 6, 21: 1, 22: 0, 23: 0, 24: 0, 25: 0, 26: 4, 27: 4, 28: 4, 29: 4, 30: 4, 31: 4}
	if c.structured {
		widths[22], widths[23], widths[24], widths[25] = 3, 1, 1, 1
	}
	minimum := 0
	if int(typ) < len(widths) {
		minimum = widths[typ]
	}
	if minimum == 0 {
		c.err = protocolError(ErrUnsupportedFeature, "OPC UA array element type")
		return out
	}
	n, null := c.array(minimum)
	var values []any
	if !null && c.err == nil {
		values = make([]any, 0, n)
		for i := 0; i < n && c.err == nil; i++ {
			element := c.object(4)
			if c.err != nil {
				break
			}
			element["Encoding"], element["Type ID"] = typ, typ
			values = append(values, c.variantScalar(typ, element, true))
		}
	}
	out["Value"] = values
	if enc&0x40 != 0 && c.err == nil {
		rank, absent := c.array(4)
		if c.err != nil {
			return out
		}
		if null || n == 0 || absent || rank < 2 {
			c.err = fmt.Errorf("opcua: Variant array dimensions rank/null")
			return out
		}
		dimensions := make([]any, 0, rank)
		product := uint64(1)
		for i := 0; i < rank && c.err == nil; i++ {
			dimension := int32(c.u(4))
			// Bound the multiplication by the already bounded element count.
			// This rejects inconsistent shapes without arithmetic overflow.
			if c.err == nil && (dimension <= 0 || uint64(dimension) > uint64(n)/product) {
				c.err = fmt.Errorf("opcua: Variant array dimension product")
				break
			}
			if c.err == nil {
				product *= uint64(dimension)
				dimensions = append(dimensions, dimension)
			}
		}
		if c.err == nil && product != uint64(n) {
			c.err = fmt.Errorf("opcua: Variant array dimensions/length")
		}
		out["Dimensions"] = dimensions
	}
	return out
}
