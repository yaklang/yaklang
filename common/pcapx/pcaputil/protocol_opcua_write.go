package pcaputil

import "fmt"

// decodeOPCUAWrite observes bounded scalar WriteValue messages. A returned
// status is captured wire data, not proof that a device applied the write.
func decodeOPCUAWrite(w []byte, response bool, max int, reserve func(int64) error, options ...uaValueOptions) (map[string]any, error) {
	c := newUAValueCursor(w, max, reserve, options)
	out := c.object(3)
	if c.err != nil {
		return nil, c.err
	}
	out["Header"] = c.header(response)
	if response {
		n, null := c.array(4)
		if !null && c.err == nil {
			results := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				results = append(results, uint32(c.u(4)))
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
		// TwoByteNodeId + AttributeId + empty/null IndexRange + mask-only
		// DataValue is the smallest legal encoded WriteValue (11 bytes).
		n, null := c.array(11)
		if !null && c.err == nil {
			nodes := make([]any, 0, n)
			for i := 0; i < n && c.err == nil; i++ {
				m := c.object(4)
				if c.err != nil {
					break
				}
				m["Node ID"] = c.node()
				m["Attribute ID"] = uint32(c.u(4))
				m["Index Range"] = c.blob(true)
				m["Value"] = c.value()
				nodes = append(nodes, m)
			}
			out["Nodes To Write"] = nodes
		} else {
			out["Nodes To Write"] = nil
		}
	}
	c.finishDiagnostics()
	if c.err != nil {
		return nil, c.err
	}
	if c.at != len(w) {
		return nil, fmt.Errorf("opcua: Write trailing bytes")
	}
	return out, nil
}
