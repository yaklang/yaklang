package pcaputil

import "fmt"

// decodeOPCUABrowseNext preserves literal continuation bytes. The decoder does
// not certify token identity, an address space or successful device traversal.
func decodeOPCUABrowseNext(w []byte, response bool, max int, reserve func(int64) error, options ...uaValueOptions) (map[string]any, error) {
	if response {
		// BrowseNext and Browse carry the same ResponseHeader, BrowseResult
		// and DiagnosticInfo wire sequence, independently checked by codec.
		return decodeOPCUABrowse(w, true, max, reserve, options...)
	}
	c := newUAValueCursor(w, max, reserve, options)
	out := c.object(3)
	if c.err != nil {
		return nil, c.err
	}
	out["Header"] = c.header(false)
	out["Release Continuation Points"] = c.u(1) != 0
	n, null := c.array(4)
	if !null && c.err == nil {
		points := make([]any, 0, n)
		for i := 0; i < n && c.err == nil; i++ {
			points = append(points, c.blob(false))
		}
		out["Continuation Points"] = points
	} else {
		out["Continuation Points"] = nil
	}
	c.finishDiagnostics()
	if c.err != nil {
		return nil, c.err
	}
	if c.at != len(w) {
		return nil, fmt.Errorf("opcua: BrowseNext trailing bytes")
	}
	return out, nil
}
