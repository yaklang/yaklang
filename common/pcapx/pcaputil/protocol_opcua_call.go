package pcaputil

import "fmt"

// decodeOPCUACall observes scalar method arguments and results. It cannot prove
// Object/Method metadata, permissions, argument signatures or device execution.
func decodeOPCUACall(w []byte, response bool, max int, reserve func(int64) error, options ...uaValueOptions) (map[string]any, error) {
	c := newUAValueCursor(w, max, reserve, options)
	out := c.object(3)
	if c.err != nil {
		return nil, c.err
	}
	out["Header"] = c.header(response)
	variants := func() any {
		n, null := c.array(1)
		if null || c.err != nil {
			return nil
		}
		values := make([]any, 0, n)
		for i := 0; i < n && c.err == nil; i++ {
			values = append(values, c.variant())
		}
		return values
	}
	diagnostics := func() any {
		n, null := c.array(1)
		if null || c.err != nil {
			return nil
		}
		values := make([]any, 0, n)
		for i := 0; i < n && c.err == nil; i++ {
			values = append(values, c.diagnostic())
		}
		return values
	}
	minimum := 8
	if response {
		minimum = 16
	}
	n, null := c.array(minimum)
	var values []any
	if !null && c.err == nil {
		values = make([]any, 0, n)
		for i := 0; i < n && c.err == nil; i++ {
			m := c.object(4)
			if c.err != nil {
				break
			}
			if !response {
				m["Object ID"] = c.node()
				m["Method ID"] = c.node()
				m["Input Arguments"] = variants()
			} else {
				status := uint32(c.u(4))
				m["Status Code"] = status
				count, empty := c.array(4)
				var results []any
				if !empty && c.err == nil {
					results = make([]any, 0, count)
					for j := 0; j < count && c.err == nil; j++ {
						results = append(results, uint32(c.u(4)))
					}
				}
				m["Input Argument Results"] = results
				m["Input Argument Diagnostic Infos"] = diagnostics()
				m["Output Arguments"] = variants()
				if c.err != nil {
					break
				}
				diags, _ := m["Input Argument Diagnostic Infos"].([]any)
				outputs, _ := m["Output Arguments"].([]any)
				// Part 4 5.12.2: argument results occur only for Bad_InvalidArgument,
				// and a Bad operation cannot carry output arguments. Do not couple the
				// output count to the input count: Method metadata is not observed here.
				if len(results) != 0 && status&0xffff0000 != 0x80ab0000 {
					return nil, protocolError(ErrDesynchronized, "OPC UA Call input results without Bad_InvalidArgument")
				}
				if len(results) != 0 && len(diags) != 0 && len(results) != len(diags) {
					return nil, protocolError(ErrDesynchronized, "OPC UA Call input diagnostics/results count")
				}
				if status&0x80000000 != 0 && len(outputs) != 0 {
					return nil, protocolError(ErrDesynchronized, "OPC UA Call Bad result with output arguments")
				}
			}
			values = append(values, m)
		}
	}
	if response {
		out["Results"] = values
		out["Diagnostic Infos"] = diagnostics()
	} else {
		out["Methods To Call"] = values
	}
	c.finishDiagnostics()
	if c.err != nil {
		return nil, c.err
	}
	if c.at != len(w) {
		return nil, fmt.Errorf("opcua: Call trailing bytes")
	}
	return out, nil
}
