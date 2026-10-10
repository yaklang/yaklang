package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

const wrapperCompactRows = 64

type wrapperCompactDescription struct {
	tag      byte
	count    int
	children []*wrapperCompactDescription
	fields   map[string]any
	minimum  int
}

// Compact values omit their Data tags. Bound both schema maps and all expanded
// values by the same aggregate node pool; a byte-only node estimate is unsafe.
func (c *wrapperListCursor) compactDescription(limit, level int) (*wrapperCompactDescription, error) {
	if c.nodes >= min(limit, wrapperDataNodes) || level >= wrapperDataLevels || 5+2*level > c.maxDepth {
		return nil, wrapperError(ErrResourceExceeded, "compact description node/depth budget exceeded")
	}
	c.nodes++
	start := c.at
	w, err := c.take(1)
	if err != nil {
		return nil, err
	}
	d := &wrapperCompactDescription{tag: w[0], fields: map[string]any{"type": w[0]}}
	if d.tag == 1 || d.tag == 2 {
		var n uint64
		var enc string
		if d.tag == 1 {
			w, err = c.take(2)
			if err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(w))
			enc = hex.EncodeToString(w)
		} else {
			n, enc, err = c.count()
			if err != nil {
				return nil, err
			}
		}
		if n > uint64(min(limit, wrapperDataNodes)-c.nodes) {
			return nil, wrapperError(ErrResourceExceeded, "compact descriptor count exceeds remaining node budget")
		}
		d.count = int(n)
		d.fields["length"], d.fields["length_encoding_hex"] = n, enc
		childCount := d.count
		if d.tag == 1 {
			childCount = 1
		}
		d.children = make([]*wrapperCompactDescription, 0, min(childCount, len(c.wire)-c.at))
		children := make([]map[string]any, 0, min(childCount, len(c.wire)-c.at))
		for i := 0; i < childCount; i++ {
			child, err := c.compactDescription(limit, level+1)
			if err != nil {
				return nil, err
			}
			d.children = append(d.children, child)
			children = append(children, child.fields)
			d.minimum = min(wrapperListOctets+1, d.minimum+child.minimum)
		}
		if d.tag == 1 {
			d.minimum = min(wrapperListOctets+1, d.minimum*d.count)
		}
		d.fields["elements"] = children
	} else if n := wrapperScalarSize(d.tag); n >= 0 {
		d.minimum = n
	} else if d.tag == 4 || d.tag == 9 || d.tag == 10 || d.tag == 12 {
		d.minimum = 1
	} else {
		return nil, wrapperError(ErrUnsupportedFeature, "compact description Data type outside selected profile")
	}
	d.fields["raw_hex"] = hex.EncodeToString(c.wire[start:c.at])
	return d, nil
}

func (c *wrapperListCursor) compactValue(d *wrapperCompactDescription, limit, level int) (map[string]any, error) {
	if c.nodes >= min(limit, wrapperDataNodes) || level >= wrapperDataLevels || 4+2*level > c.maxDepth {
		return nil, wrapperError(ErrResourceExceeded, "expanded compact Data node/depth budget exceeded")
	}
	c.nodes++
	start := c.at
	var out map[string]any
	if d.tag == 1 || d.tag == 2 {
		if d.count > min(limit, wrapperDataNodes)-c.nodes {
			return nil, wrapperError(ErrResourceExceeded, "compact child count exceeds remaining Data budget")
		}
		children := make([]map[string]any, 0, d.count)
		for i := 0; i < d.count; i++ {
			index := i
			if d.tag == 1 {
				index = 0
			}
			child := d.children[index]
			v, err := c.compactValue(child, limit, level+1)
			if err != nil {
				return nil, err
			}
			children = append(children, v)
		}
		out = map[string]any{"type": d.tag, "length": d.count, "elements": children}
	} else {
		n := wrapperScalarSize(d.tag)
		if n >= 0 {
			if _, err := c.take(n); err != nil {
				return nil, err
			}
		} else {
			count, _, err := c.count()
			if err != nil {
				return nil, err
			}
			size := count
			if d.tag == 4 {
				if count > uint64(min(limit, wrapperListOctets))*8 {
					return nil, wrapperError(ErrResourceExceeded, "compact bit-string byte budget exceeded")
				}
				size = (count + 7) / 8
			} else if count > uint64(min(limit, wrapperListOctets)) {
				return nil, wrapperError(ErrResourceExceeded, "compact variable value byte budget exceeded")
			}
			if _, err := c.take(int(size)); err != nil {
				return nil, err
			}
		}
		// This tag-prefixed buffer is only a bounded decoder input, never captured
		// provenance. Public raw_hex below contains actual contents bytes only.
		encoded := make([]byte, 1+c.at-start)
		encoded[0] = d.tag
		copy(encoded[1:], c.wire[start:c.at])
		scalar := wrapperListCursor{wire: encoded}
		v, err := scalar.scalar(limit)
		if err != nil {
			return nil, err
		}
		out = v
	}
	out["raw_hex"], out["contents_offset"], out["type_inferred_from_description"] = hex.EncodeToString(c.wire[start:c.at]), start, true
	return out, nil
}

func (c *wrapperListCursor) compact(limit, level, start int) (map[string]any, error) {
	before := c.nodes
	d, err := c.compactDescription(limit, level)
	if err != nil {
		return nil, err
	}
	descriptionNodes := c.nodes - before
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n > uint64(min(limit, wrapperListOctets)) {
		return nil, wrapperError(ErrResourceExceeded, "compact contents byte budget exceeded")
	}
	contents, err := c.take(int(n))
	if err != nil {
		return nil, err
	}
	// Empty contents prove zero rows, even for an empty/null description.
	// A nonempty contents stream cannot encode zero-width rows or make progress.
	if d.minimum == 0 && n != 0 {
		return nil, wrapperError(ErrMalformedMessage, "nonempty compact contents with zero-width description")
	}
	values := wrapperListCursor{wire: contents, nodes: c.nodes, maxDepth: c.maxDepth}
	capacity := 0
	if d.minimum != 0 {
		capacity = min(wrapperCompactRows, int(n)/d.minimum)
	}
	rows := make([]map[string]any, 0, capacity)
	for values.at < len(contents) {
		if len(rows) >= min(limit, wrapperCompactRows) {
			return nil, wrapperError(ErrResourceExceeded, "compact row count budget exceeded")
		}
		if len(contents)-values.at < d.minimum {
			return nil, wrapperError(ErrMalformedMessage, "compact final row truncated")
		}
		prior := values.at
		row, err := values.compactValue(d, limit, level+1)
		if err != nil {
			return nil, err
		}
		if values.at <= prior {
			return nil, wrapperError(ErrUnsupportedFeature, "compact row made no byte progress")
		}
		rows = append(rows, row)
	}
	valueNodes := values.nodes - c.nodes
	c.nodes = values.nodes
	return map[string]any{"type": byte(19), "raw_hex": hex.EncodeToString(c.wire[start:c.at]), "type_description": d.fields, "contents_length": n, "contents_length_encoding_hex": enc, "contents_hex": hex.EncodeToString(contents), "row_count": len(rows), "rows": rows, "description_node_count": descriptionNodes, "value_node_count": valueNodes}, nil
}
