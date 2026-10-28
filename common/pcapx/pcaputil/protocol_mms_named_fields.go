package pcaputil

// Named-list observations preserve literal names and wire results. A matching
// observed definition/lookup is required before resolving a later Read/Write.
func (c *mmsValueContext) listMembers(n mmsBER, d *mmsDecoded) ([]map[string]any, error) {
	n.tag = 0xa0
	return c.variableEntries(n, d)
}
func (c *mmsValueContext) namedService(s mmsBER, d *mmsDecoded) error {
	switch d.service {
	case 11:
		if d.tag == 0xa1 {
			if len(s.body) != 0 {
				return protocolError(ErrMalformedMessage, "MMS DefineNamedVariableList response NULL has content")
			}
			d.fields["Reported Definition Accepted"] = true
			return nil
		}
		if len(s.children) != 2 || s.children[1].tag != 0xa0 {
			return protocolError(ErrMalformedMessage, "MMS DefineNamedVariableList name/member pair")
		}
		name, err := mmsNamed(s.children[0], true)
		if err != nil {
			return err
		}
		names, err := c.listMembers(s.children[1], d)
		if err != nil {
			return err
		}
		d.fields["Variable List"], d.fields["Variables"] = name, names
		d.items = len(names)
	case 12:
		if d.tag == 0xa0 {
			n, err := mmsSingleChild(s)
			if err != nil {
				return err
			}
			name, err := mmsNamed(n, true)
			if err != nil {
				return err
			}
			d.fields["Variable List"] = name
			return nil
		}
		if err := mmsSchema(s, []byte{0x80, 0xa1}, nil); err != nil {
			return err
		}
		b, _ := mmsChild(s, 0x80)
		deletable, err := mmsBoolean(b)
		if err != nil {
			return err
		}
		ls, _ := mmsChild(s, 0xa1)
		names, err := c.listMembers(ls, d)
		if err != nil {
			return err
		}
		d.fields["MMS Deletable"], d.fields["Variables"] = deletable, names
		d.items = len(names)
	case 13:
		if d.tag == 0xa1 {
			if err := mmsSchema(s, []byte{0x80, 0x81}, nil); err != nil {
				return err
			}
			matched, _ := mmsChild(s, 0x80)
			deleted, _ := mmsChild(s, 0x81)
			m, err := mmsUInt(matched, 32)
			if err != nil {
				return err
			}
			n, err := mmsUInt(deleted, 32)
			if err != nil {
				return err
			}
			if n > m {
				return protocolError(ErrMalformedMessage, "MMS deleted count exceeds matched count")
			}
			d.fields["Number Matched"], d.fields["Number Deleted"] = m, n
			return nil
		}
		if err := mmsSchema(s, nil, []byte{0x80, 0xa1, 0x82}); err != nil {
			return err
		}
		scope := uint32(0)
		for _, v := range s.children {
			switch v.tag {
			case 0x80:
				n, err := mmsUInt(v, 8)
				if err != nil {
					return err
				}
				if n > 3 {
					return protocolError(ErrUnsupportedFeature, "MMS DeleteNamedVariableList scope")
				}
				scope = n
			case 0xa1:
				names := make([]map[string]any, 0, len(v.children))
				for _, x := range v.children {
					name, err := mmsNamed(x, true)
					if err != nil {
						return err
					}
					names = append(names, name)
				}
				d.fields["List Names"] = names
			case 0x82:
				domain, err := mmsVisible(v.body)
				if err != nil {
					return err
				}
				d.fields["Domain Name"] = domain
			}
		}
		d.fields["Scope Of Delete"] = scope
	}
	return nil
}
func (c *mmsValueContext) variableAccess(choice mmsBER, d *mmsDecoded) ([]map[string]any, error) {
	if choice.tag == 0xa0 {
		return c.variableEntries(choice, d)
	}
	if choice.tag != 0xa1 {
		return nil, protocolError(ErrUnsupportedFeature, "MMS VariableAccessSpecification choice")
	}
	n, err := mmsSingleChild(choice)
	if err != nil {
		return nil, err
	}
	name, err := mmsNamed(n, true)
	if err != nil {
		return nil, err
	}
	if c.state == nil {
		return nil, protocolError(ErrContextRequired, "MMS named variable-list definition not observed")
	}
	key := mmsKey(name)
	peer := 1 - c.dir
	entry, ok := c.state.lists[peer][key]
	if !ok {
		return nil, protocolError(ErrContextRequired, "MMS named variable-list definition not observed")
	}
	units := 0
	for _, name := range entry.names {
		units += mmsSelectorUnits(name)
		if tags, ok := name["Component Tags"].([]byte); ok {
			units += len(tags)
		}
	}
	if units > c.left {
		return nil, protocolError(ErrResourceExceeded, "MMS resolved member collection budget")
	}
	if c.reserve != nil {
		if err = c.reserve(512 + 8*mmsNamesStorage(entry.names)); err != nil {
			return nil, err
		}
	}
	c.left -= units
	names := cloneSessionValue(entry.names).([]map[string]any)
	d.binding = &mmsListBinding{key: key, peer: peer, revision: entry.revision, event: entry.event}
	d.fields["Variable List"], d.fields["Observed Resolved Variables"], d.fields["Observed List Event"] = name, names, entry.event
	return names, nil
}
func (c *mmsValueContext) readAccess(s mmsBER, d *mmsDecoded) error {
	if err := mmsSchema(s, []byte{0xa1}, []byte{0x80}); err != nil {
		return err
	}
	spec := false
	for _, v := range s.children {
		if v.tag == 0x80 {
			b, err := mmsBoolean(v)
			if err != nil {
				return err
			}
			spec = b
		}
	}
	va, _ := mmsChild(s, 0xa1)
	choice, err := mmsSingleChild(va)
	if err != nil {
		return err
	}
	names, err := c.variableAccess(choice, d)
	if err != nil {
		return err
	}
	d.fields["Specification With Result"] = spec
	d.items = len(names)
	if d.binding == nil {
		d.fields["Variables"] = names
	}
	return nil
}
func (c *mmsValueContext) serviceError(n mmsBER, d *mmsDecoded) error {
	if err := mmsSchema(n, []byte{0x80, 0xa2}, nil); err != nil {
		return err
	}
	s, _ := mmsChild(n, 0xa2)
	if err := mmsSchema(s, []byte{0xa0}, []byte{0x81, 0x82, 0xa3}); err != nil {
		return err
	}
	cl, _ := mmsChild(s, 0xa0)
	v, err := mmsSingleChild(cl)
	if err != nil {
		return err
	}
	if v.tag < 0x80 || v.tag > 0x8c {
		return protocolError(ErrUnsupportedFeature, "MMS ServiceError class")
	}
	klass := int(v.tag - 0x80)
	fields := map[string]any{"Class": uint32(klass)}
	if klass == 12 {
		value, err := mmsInteger(v, false)
		if err != nil {
			return err
		}
		fields["Code Decimal"] = value
	} else {
		maxCode := []uint32{4, 4, 6, 5, 5, 3, 1, 4, 7, 1, 2, 9}
		code, err := mmsUInt(v, 8)
		if err != nil {
			return err
		}
		if code > maxCode[klass] {
			return protocolError(ErrMalformedMessage, "MMS ServiceError enumeration")
		}
		if klass == 7 {
			fields["Access Error"] = code
		} else {
			fields["Code"] = code
			fields["Class Name"] = []string{"vmd-state", "application-reference", "definition", "resource", "service", "service-preempt", "time-resolution", "access", "initiate", "conclude", "cancel", "file"}[klass]
		}
	}
	for _, v := range s.children {
		switch v.tag {
		case 0x81:
			value, err := mmsInteger(v, false)
			if err != nil {
				return err
			}
			fields["Additional Code Decimal"] = value
		case 0x82:
			text, err := mmsVisible(v.body)
			if err != nil {
				return err
			}
			fields["Additional Description"] = text
		case 0xa3:
			specific, err := mmsSingleChild(v)
			if err != nil {
				return err
			}
			if specific.tag != 0x86 {
				return protocolError(ErrUnsupportedFeature, "MMS selected deleteNamedVariableList error detail")
			}
			count, err := mmsUInt(specific, 32)
			if err != nil {
				return err
			}
			fields["Delete Named List Deleted"] = count
			d.deleteSpecific = true
		}
	}
	d.fields["Service Error"] = fields
	return nil
}
