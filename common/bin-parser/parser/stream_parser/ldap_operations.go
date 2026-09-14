package stream_parser

import "fmt"

// RFC 4511 sections 4.1.9, 4.2.2, 4.3 and 4.5. These are explicit,
// single-message profiles. No directory operation or matching rule is run.
func ldapOperationTag(profile string) byte {
	switch profile {
	case "bind-response":
		return 0x61
	case "unbind-request":
		return 0x42
	case "search-request":
		return 0x63
	case "search-entry":
		return 0x64
	case "search-done":
		return 0x65
	case "search-reference":
		return 0x73
	}
	return 0
}

func (r *ldapFieldsReader) item() bool {
	r.items++
	if r.items > 4096 {
		r.fail("operation item resource limit")
	}
	return r.err == nil
}

func (r *ldapFieldsReader) list(name string, visit func()) {
	start, index := r.at, len(r.fields)
	visit()
	children := append([]tlsCertificateField(nil), r.fields[index:]...)
	r.fields = append(r.fields[:index], tlsCertificateField{Name: name, Start: start, End: r.at, List: true, Children: children})
}

func (r *ldapFieldsReader) enumeration(name string, max uint64) uint64 {
	e := r.open(10)
	if r.err != nil {
		return 0
	}
	n := e.end - e.content
	if n < 1 || n > 4 {
		r.fail("invalid ENUMERATED width")
		return 0
	}
	b := r.wire[e.content:e.end]
	if b[0]&128 != 0 || n > 1 && b[0] == 0 && b[1]&128 == 0 {
		r.fail("negative or nonminimal ENUMERATED")
		return 0
	}
	v := r.uint(name, n)
	if v > max {
		r.fail("ENUMERATED range for " + name)
	}
	r.close(e, name+" Encoding", false)
	return v
}

func (r *ldapFieldsReader) boolean(tag byte, name string) bool {
	e := r.open(tag)
	if e.end-e.content != 1 {
		r.fail("invalid BOOLEAN width")
	}
	v := r.uint(name, 1)
	if v != 0 && v != 255 {
		r.fail("true must be FF")
	}
	r.close(e, name+" Encoding", false)
	return v == 255
}

func (r *ldapFieldsReader) uriList(name string) {
	count := 0
	r.list(name, func() {
		for r.err == nil && r.at < r.end && r.item() {
			r.octets(4, "URI", true)
			count++
		}
	})
	if count == 0 {
		r.fail("empty URI list")
	}
}

func (r *ldapFieldsReader) result(info map[string]any) {
	// ENUMERATED is extensible; unknown nonnegative result codes stay numeric.
	code := r.enumeration("Result Code", 2147483647)
	info["Result Code"] = code
	referral := false
	r.octets(4, "Matched DN", true)
	r.octets(4, "Diagnostic Message", true)
	if r.err == nil && r.at < r.end && r.wire[r.at] == 0xa3 {
		referral = true
		e := r.open(0xa3)
		r.within(e, func() { r.uriList("Referrals") })
		r.close(e, "Referral Encoding", false)
	}
	if (code == 10) != referral {
		r.fail("referral must accompany only referral result code")
	}
}

func (r *ldapFieldsReader) assertion() {
	r.octets(4, "Attribute Description", true)
	r.octets(4, "Assertion Value", false)
}

func (r *ldapFieldsReader) filter(depth int) {
	if depth > 64 {
		r.fail("filter depth resource limit")
		return
	}
	if !r.item() {
		return
	}
	if r.at >= r.end {
		r.fail("missing filter")
		return
	}
	tag := r.wire[r.at]
	if tag == 0x87 {
		r.octets(tag, "Present Attribute", true)
		return
	}
	if tag < 0xa0 || tag > 0xa9 || tag == 0xa7 {
		r.fail("unknown filter choice")
		return
	}
	e := r.open(tag)
	names := map[byte]string{0xa0: "And", 0xa1: "Or", 0xa2: "Not", 0xa3: "Equality", 0xa4: "Substrings", 0xa5: "Greater Or Equal", 0xa6: "Less Or Equal", 0xa8: "Approximate", 0xa9: "Extensible"}
	r.within(e, func() {
		switch tag {
		case 0xa0, 0xa1:
			count := 0
			r.list("Filters", func() {
				for r.err == nil && r.at < r.end {
					r.filter(depth + 1)
					count++
				}
			})
			if count == 0 {
				r.fail("empty boolean filter")
			}
		case 0xa2:
			r.filter(depth + 1)
		case 0xa3, 0xa5, 0xa6, 0xa8:
			r.assertion()
		case 0xa4:
			r.octets(4, "Attribute Description", true)
			sub := r.open(0x30)
			r.within(sub, func() {
				count, final := 0, false
				r.list("Substrings", func() {
					for r.err == nil && r.at < r.end && r.item() {
						t := r.wire[r.at]
						if final || t < 0x80 || t > 0x82 || t == 0x80 && count > 0 {
							r.fail("invalid substring order")
							return
						}
						r.octets(t, []string{"Initial", "Any", "Final"}[int(t-0x80)], false)
						final = t == 0x82
						count++
					}
				})
				if count == 0 {
					r.fail("empty substring filter")
				}
			})
			r.close(sub, "Substrings Encoding", false)
		case 0xa9:
			rule, typ := false, false
			if r.err == nil && r.at < r.end && r.wire[r.at] == 0x81 {
				r.octets(0x81, "Matching Rule", true)
				rule = true
			}
			if r.err == nil && r.at < r.end && r.wire[r.at] == 0x82 {
				r.octets(0x82, "Attribute Description", true)
				typ = true
			}
			if !rule && !typ {
				r.fail("extensible filter requires type or matching rule")
			}
			r.octets(0x83, "Match Value", false)
			if r.err == nil && r.at < r.end {
				if !r.boolean(0x84, "DN Attributes") {
					r.fail("default false DN Attributes must be absent")
				}
			}
		}
	})
	r.close(e, names[tag]+" Filter", false)
}

func (r *ldapFieldsReader) search() {
	r.octets(4, "Base Object", true)
	r.enumeration("Scope", 2)
	r.enumeration("Deref Aliases", 3)
	r.number("Size Limit", 2147483647)
	r.number("Time Limit", 2147483647)
	r.boolean(1, "Types Only")
	r.filter(0)
	e := r.open(0x30)
	r.within(e, func() {
		r.list("Attributes", func() {
			for r.err == nil && r.at < r.end && r.item() {
				r.octets(4, "Attribute Selector", true)
			}
		})
	})
	r.close(e, "Attributes Encoding", false)
}

func (r *ldapFieldsReader) searchEntry() {
	r.octets(4, "Object Name", true)
	e := r.open(0x30)
	r.within(e, func() {
		r.list("Attributes", func() {
			for r.err == nil && r.at < r.end && r.item() {
				a := r.open(0x30)
				r.within(a, func() {
					r.octets(4, "Attribute Description", true)
					v := r.open(0x31)
					r.within(v, func() {
						r.list("Values", func() {
							for r.err == nil && r.at < r.end && r.item() {
								r.octets(4, "Attribute Value", false)
							}
						})
					})
					r.close(v, "Values Encoding", false)
				})
				r.close(a, "Partial Attribute", false)
			}
		})
	})
	r.close(e, "Attributes Encoding", false)
}

func decodeLDAPOperationFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	tag := ldapOperationTag(profile)
	if tag == 0 {
		return nil, nil, fmt.Errorf("ldap-fields: unknown explicit profile")
	}
	if len(wire) == 0 || len(wire) > ldapFieldsMaxBytes {
		return nil, nil, fmt.Errorf("ldap-fields: 1..1048576 byte boundary required")
	}
	r := &ldapFieldsReader{wire: wire, end: len(wire)}
	names := map[byte]string{0x61: "BindResponse", 0x42: "UnbindRequest", 0x63: "SearchRequest", 0x64: "SearchResultEntry", 0x65: "SearchResultDone", 0x73: "SearchResultReference"}
	info := map[string]any{"Layout Context": profile, "Message Name": names[tag], "Session State Validated": false, "Directory Name Validated": false, "Transport Context Validated": false, "Control Semantics Applied": false, "Matching Rules Applied": false}
	outer := r.open(0x30)
	r.within(outer, func() {
		id := r.number("Message ID", 2147483647)
		if id == 0 {
			r.fail("operation requires nonzero message ID")
		}
		info["Message ID"] = id
		op := r.open(tag)
		r.within(op, func() {
			switch tag {
			case 0x61, 0x65:
				r.result(info)
				if tag == 0x61 && r.err == nil && r.at < r.end {
					r.octets(0x87, "Server SASL Credentials", false)
				}
			case 0x42: // primitive NULL; close enforces the empty content
			case 0x63:
				r.search()
			case 0x64:
				r.searchEntry()
			case 0x73:
				r.uriList("References")
			}
		})
		r.close(op, names[tag], false)
		if r.err == nil && r.at < r.end {
			r.controls(info)
		}
	})
	r.close(outer, "LDAPMessage", false)
	if r.at != len(wire) {
		r.fail("trailing input")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	return r.fields, info, nil
}
