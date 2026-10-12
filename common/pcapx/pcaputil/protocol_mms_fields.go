package pcaputil

import "bytes"

// The ISO profile is intentionally separate from the historical bare-PDU rule.
// It parses bounded BER directly: no executable rule graph, guessed byte offset,
// or implicit association is used by the capture path.
func mmsSchema(n mmsBER, required, optional []byte) error {
	seen := map[byte]bool{}
	last := -1
	tags := append(append([]byte(nil), required...), optional...)
	for _, c := range n.children {
		if seen[c.tag] {
			return protocolError(ErrMalformedMessage, "MMS duplicate field")
		}
		seen[c.tag] = true
		found := false
		for _, tag := range tags {
			found = found || tag == c.tag
		}
		if !found {
			return protocolError(ErrUnsupportedFeature, "MMS field outside selected grammar")
		}
		// Selected SEQUENCE fields have ascending context tags.
		if int(c.tag&31) < last {
			return protocolError(ErrMalformedMessage, "MMS sequence field order")
		}
		last = int(c.tag & 31)
	}
	for _, tag := range required {
		if !seen[tag] {
			return protocolError(ErrMalformedMessage, "MMS mandatory field missing")
		}
	}
	return nil
}
func mmsNamed(n mmsBER) (map[string]any, error) {
	f := map[string]any{"Choice Tag": n.tag}
	text := func(w []byte) (string, error) {
		if len(w) == 0 {
			return "", protocolError(ErrMalformedMessage, "MMS empty Identifier")
		}
		for _, v := range w {
			if v < 32 || v > 126 {
				return "", protocolError(ErrMalformedMessage, "MMS Identifier VisibleString")
			}
		}
		return string(w), nil
	}
	switch n.tag {
	case 0xa1:
		if len(n.children) != 2 || n.children[0].tag != 0x1a || n.children[1].tag != 0x1a {
			return nil, protocolError(ErrMalformedMessage, "MMS domain ObjectName")
		}
		for i, k := range []string{"Domain ID", "Item ID"} {
			v, e := text(n.children[i].body)
			if e != nil {
				return nil, e
			}
			f[k] = v
		}
	default:
		return nil, protocolError(ErrUnsupportedFeature, "MMS selected ObjectName")
	}
	return f, nil
}
func mmsSingleChild(n mmsBER) (mmsBER, error) {
	if len(n.children) != 1 {
		return mmsBER{}, protocolError(ErrMalformedMessage, "MMS explicit choice requires one value")
	}
	return n.children[0], nil
}
func mmsSelectedFields(n mmsBER, d *mmsDecoded) error {
	switch n.tag {
	case 0xa8, 0xa9:
		if e := mmsSchema(n, []byte{0x81, 0x82, 0xa4}, []byte{0x80, 0x83}); e != nil {
			return e
		}
		d.fields["Calling Limit"], d.fields["Called Limit"], d.fields["Version"] = d.calling, d.called, d.version
		for _, c := range n.children {
			if c.tag == 0x80 || c.tag == 0x83 {
				bits := uint(31)
				key := "Local Detail"
				if c.tag == 0x83 {
					bits = 7
					key = "Nesting Level"
				}
				v, e := mmsUInt(c, bits)
				if e != nil {
					return e
				}
				if v == 0 {
					return protocolError(ErrMalformedMessage, "MMS positive proposed limit")
				}
				d.fields[key] = v
			}
		}
		detail, _ := mmsChild(n, 0xa4)
		if e := mmsSchema(detail, []byte{0x80, 0x81, 0x82}, nil); e != nil {
			return e
		}
		for _, c := range detail.children {
			if c.tag == 0x81 || c.tag == 0x82 {
				key := "Parameter Support"
				if c.tag == 0x82 {
					key = "Services Supported"
					d.services = bytes.Clone(c.body)
				}
				d.fields[key] = map[string]any{"Unused Bits": c.body[0], "Bits": bytes.Clone(c.body[1:])}
			}
		}
	case 0xa0:
		s := n.children[1]
		if d.service == 6 {
			if len(s.children) != 1 || s.children[0].tag != 0xa0 {
				return protocolError(ErrUnsupportedFeature, "MMS named GetVariableAccessAttributes request selected")
			}
			v, e := mmsSingleChild(s.children[0])
			if e != nil {
				return e
			}
			f, e := mmsNamed(v)
			if e != nil {
				return e
			}
			d.fields["Object Name"] = f
		} else {
			if e := mmsSchema(s, []byte{0xa1}, []byte{0x80}); e != nil {
				return e
			}
			spec := false
			for _, c := range s.children {
				if c.tag == 0x80 {
					if len(c.body) != 1 {
						return protocolError(ErrMalformedMessage, "MMS BOOLEAN length")
					}
					spec = c.body[0] != 0
				}
			}
			if spec {
				return protocolError(ErrUnsupportedFeature, "MMS specificationWithResult needs returned variable context profile")
			}
			va, _ := mmsChild(s, 0xa1)
			ls, e := mmsSingleChild(va)
			if e != nil {
				return e
			}
			if ls.tag != 0xa0 {
				return protocolError(ErrContextRequired, "MMS named variable-list definition not observed")
			}
			names := make([]map[string]any, 0, len(ls.children))
			for _, v := range ls.children {
				if v.tag != 0x30 || len(v.children) != 1 || v.children[0].tag != 0xa0 {
					return protocolError(ErrUnsupportedFeature, "MMS named VariableSpecification without alternate access selected")
				}
				name, e := mmsSingleChild(v.children[0])
				if e != nil {
					return e
				}
				f, e := mmsNamed(name)
				if e != nil {
					return e
				}
				names = append(names, f)
			}
			if len(names) == 0 {
				return protocolError(ErrMalformedMessage, "MMS empty selected listOfVariable")
			}
			d.fields["Specification With Result"], d.fields["Variables"] = spec, names
		}
	case 0xa1:
		if d.service != 4 {
			return protocolError(ErrUnsupportedFeature, "MMS GetVariableAccessAttributes type response not yet selected")
		}
		s := n.children[1]
		if e := mmsSchema(s, []byte{0xa1}, nil); e != nil {
			return e
		}
		ls, _ := mmsChild(s, 0xa1)
		results := make([]map[string]any, 0, len(ls.children))
		for _, v := range ls.children {
			if v.tag != 0x80 {
				return protocolError(ErrUnsupportedFeature, "MMS success Data requires its typed service profile")
			}
			code, e := mmsUInt(v, 8)
			if e != nil {
				return e
			}
			if code > 11 {
				return protocolError(ErrMalformedMessage, "MMS DataAccessError enumeration")
			}
			results = append(results, map[string]any{"Failure": code})
		}
		if len(results) == 0 {
			return protocolError(ErrMalformedMessage, "MMS empty selected AccessResult list")
		}
		d.fields["Access Results"] = results
	case 0xa2:
		if e := mmsSchema(n, []byte{0x80, 0xa2}, nil); e != nil {
			return e
		}
		se, _ := mmsChild(n, 0xa2)
		if e := mmsSchema(se, []byte{0xa0}, nil); e != nil {
			return e
		}
		cl, _ := mmsChild(se, 0xa0)
		v, e := mmsSingleChild(cl)
		if e != nil {
			return e
		}
		if v.tag != 0x87 {
			return protocolError(ErrUnsupportedFeature, "MMS selected access ServiceError")
		}
		code, e := mmsUInt(v, 8)
		if e != nil {
			return e
		}
		if code > 4 {
			return protocolError(ErrMalformedMessage, "MMS access ServiceError enumeration")
		}
		d.fields["Service Error"] = map[string]any{"Class": uint32(7), "Access Error": code}
	}
	return nil
}
func mmsSupports(bits []byte, service byte) bool {
	index := int(service)/8 + 1
	return index < len(bits) && bits[index]&(128>>uint(service%8)) != 0
}
