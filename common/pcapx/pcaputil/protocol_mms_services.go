package pcaputil

// These services preserve reported wire values and observed request context.
// They do not establish a complete device catalogue or physical write success.
func (c *mmsValueContext) discoveryWrite(s mmsBER, d *mmsDecoded) error {
	if d.service == 1 {
		if d.tag == 0xa0 {
			if err := mmsSchema(s, []byte{0xa0, 0xa1}, []byte{0x82}); err != nil {
				return err
			}
			class, _ := mmsChild(s, 0xa0)
			class, err := mmsSingleChild(class)
			if err != nil {
				return err
			}
			if class.tag != 0x80 {
				return protocolError(ErrUnsupportedFeature, "MMS extended ObjectClass outside selected basic class")
			}
			n, err := mmsUInt(class, 31)
			if err != nil {
				return err
			}
			names := []string{"named-variable", "scattered-access", "named-variable-list", "named-type", "semaphore", "event-condition", "event-action", "event-enrollment", "journal", "domain", "program-invocation", "operator-station"}
			if n >= uint32(len(names)) {
				return protocolError(ErrUnsupportedFeature, "MMS ObjectClass outside selected enumeration")
			}
			d.fields["Object Class"], d.fields["Object Class Name"] = n, names[n]
			scope, _ := mmsChild(s, 0xa1)
			scope, err = mmsSingleChild(scope)
			if err != nil {
				return err
			}
			f := map[string]any{"Choice Tag": scope.tag}
			switch scope.tag {
			case 0x80, 0x82:
				if len(scope.body) != 0 {
					return protocolError(ErrMalformedMessage, "MMS ObjectScope NULL has content")
				}
				f["Scope"] = map[byte]string{0x80: "vmd", 0x82: "association"}[scope.tag]
			case 0x81:
				name, err := mmsVisible(scope.body)
				if err != nil {
					return err
				}
				f["Scope"], f["Identifier"] = "domain", name
			default:
				return protocolError(ErrUnsupportedFeature, "MMS ObjectScope choice")
			}
			d.fields["Object Scope"] = f
			for _, v := range s.children {
				if v.tag == 0x82 {
					name, err := mmsVisible(v.body)
					if err != nil {
						return err
					}
					d.fields["Continue After"] = name
				}
			}
			return nil
		}
		if err := mmsSchema(s, []byte{0xa0}, []byte{0x81}); err != nil {
			return err
		}
		list, _ := mmsChild(s, 0xa0)
		names := make([]string, 0, len(list.children))
		for _, v := range list.children {
			if v.tag != 0x1a {
				return protocolError(ErrMalformedMessage, "MMS listOfIdentifier requires VisibleString")
			}
			name, err := mmsVisible(v.body)
			if err != nil {
				return err
			}
			names = append(names, name)
		}
		more := true // ASN.1 DEFAULT TRUE, including the empty returned list.
		for _, v := range s.children {
			if v.tag == 0x81 {
				var err error
				more, err = mmsBoolean(v)
				if err != nil {
					return err
				}
			}
		}
		d.fields["Identifiers"], d.fields["More Follows"] = names, more
		return nil
	}
	if d.tag == 0xa0 {
		// Both named VariableAccessSpecification and listOfData use [0].
		// A tag-indexed SEQUENCE schema would incorrectly discard one field.
		if len(s.children) != 2 || s.children[1].tag != 0xa0 {
			return protocolError(ErrMalformedMessage, "MMS Write positional variable/data pair")
		}
		names, err := c.variableAccess(s.children[0], d)
		if err != nil {
			return err
		}
		list := s.children[1]
		if len(names) != len(list.children) {
			return protocolError(ErrMalformedMessage, "MMS Write variable/data cardinality")
		}
		values := make([]map[string]any, 0, len(list.children))
		for _, v := range list.children {
			value, depth, err := c.data(v)
			if err != nil {
				return err
			}
			d.valueDepth = max(d.valueDepth, depth)
			values = append(values, value)
		}
		d.items = len(names)
		if d.binding == nil {
			d.fields["Variables"] = names
		}
		d.fields["Data"] = values
		return nil
	}
	results := make([]map[string]any, 0, len(s.children))
	for _, v := range s.children {
		switch v.tag {
		case 0x80:
			n, err := mmsUInt(v, 8)
			if err != nil {
				return err
			}
			if n > 11 {
				return protocolError(ErrMalformedMessage, "MMS Write DataAccessError enumeration")
			}
			results = append(results, map[string]any{"Failure": n})
		case 0x81:
			if len(v.body) != 0 {
				return protocolError(ErrMalformedMessage, "MMS Write success NULL has content")
			}
			results = append(results, map[string]any{"Success": true})
		default:
			return protocolError(ErrUnsupportedFeature, "MMS Write result choice")
		}
	}
	if len(results) == 0 {
		return protocolError(ErrMalformedMessage, "MMS empty selected Write result list")
	}
	d.items = len(results)
	d.fields["Write Results"] = results
	return nil
}
