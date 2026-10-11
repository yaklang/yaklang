package pcaputil

import "fmt"

func (c *uaReadCursor) qualifiedName() map[string]any {
	m := c.object(2)
	if c.err != nil {
		return nil
	}
	m["Namespace"] = uint16(c.u(2))
	m["Name"] = c.blob(true)
	return m
}
func (c *uaReadCursor) localizedText() map[string]any {
	m := c.object(3)
	if c.err != nil {
		return nil
	}
	mask := byte(c.u(1))
	m["Encoding"] = mask
	if mask&0xfc != 0 {
		c.err = fmt.Errorf("opcua: LocalizedText reserved bits")
		return m
	}
	if mask&1 != 0 {
		m["Locale"] = c.blob(true)
	}
	if mask&2 != 0 {
		m["Text"] = c.blob(true)
	}
	return m
}
func (c *uaReadCursor) browseReference() map[string]any {
	m := c.object(7)
	if c.err != nil {
		return nil
	}
	m["Reference Type ID"] = c.node()
	m["Is Forward"] = c.u(1) != 0
	m["Node ID"] = c.nodeFlags(true)
	m["Browse Name"] = c.qualifiedName()
	m["Display Name"] = c.localizedText()
	m["Node Class"] = int32(c.u(4))
	m["Type Definition"] = c.nodeFlags(true)
	return m
}

// decodeOPCUABrowse is a bounded wire observation, not an address-space or
// continuation-point identity assertion. Header/array budgets are shared with
// the scalar Read profile and aggregate across every nested reference array.
func decodeOPCUABrowse(w []byte, response bool, max int, reserve func(int64) error) (map[string]any, error) {
	c := &uaReadCursor{wire: w, max: max, reserve: reserve}
	out := c.object(4)
	if c.err != nil {
		return nil, c.err
	}
	out["Header"] = c.header(response)
	if response {
		n, null := c.array(12)
		if !null && c.err == nil {
			results := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				m := c.object(3)
				if c.err != nil {
					break
				}
				m["Status Code"] = uint32(c.u(4))
				m["Continuation Point"] = c.blob(false)
				k, nul := c.array(18)
				if !nul && c.err == nil {
					refs := make([]any, 0, k)
					for j := 0; j < k && c.err == nil; j++ {
						refs = append(refs, c.browseReference())
					}
					m["References"] = refs
				} else {
					m["References"] = nil
				}
				results = append(results, m)
			}
			out["Results"] = results
		} else {
			out["Results"] = nil
		}
		n, null = c.array(1)
		if !null && c.err == nil {
			diags := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				diags = append(diags, c.diagnostic())
			}
			out["Diagnostic Infos"] = diags
		} else {
			out["Diagnostic Infos"] = nil
		}
	} else {
		view := c.object(3)
		if c.err != nil {
			return nil, c.err
		}
		view["View ID"] = c.node()
		view["Timestamp Ticks"] = int64(c.u(8))
		view["Version"] = uint32(c.u(4))
		out["View"] = view
		out["Requested Max References Per Node"] = uint32(c.u(4))
		n, null := c.array(17)
		if !null && c.err == nil {
			nodes := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				m := c.object(6)
				if c.err != nil {
					break
				}
				m["Node ID"] = c.node()
				m["Browse Direction"] = int32(c.u(4))
				m["Reference Type ID"] = c.node()
				m["Include Subtypes"] = c.u(1) != 0
				m["Node Class Mask"] = uint32(c.u(4))
				m["Result Mask"] = uint32(c.u(4))
				nodes = append(nodes, m)
			}
			out["Nodes To Browse"] = nodes
		} else {
			out["Nodes To Browse"] = nil
		}
	}
	if c.err != nil {
		return nil, c.err
	}
	if c.at != len(w) {
		return nil, fmt.Errorf("opcua: Browse trailing bytes")
	}
	return out, nil
}
