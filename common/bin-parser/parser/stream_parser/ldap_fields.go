package stream_parser

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const ldapFieldsMaxBytes = 1 << 20
const ldapFieldsMaxControls = 4096

// LDAP uses restricted BER, not DER: definite long-form lengths need not be
// minimal. This explicit profile covers BindRequest, not arbitrary operations.
type ldapFieldsReader struct {
	wire    []byte
	at, end int
	err     error
	fields  []tlsCertificateField
}
type ldapFieldsElement struct{ start, index, content, end int }

func (r *ldapFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("ldap-fields: %s at byte %d", s, r.at)
	}
}
func (r *ldapFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	s := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	return r.wire[s:r.at]
}
func (r *ldapFieldsReader) uint(name string, n int) uint64 {
	var v uint64
	for _, b := range r.take(name, "uint32", n) {
		v = v<<8 | uint64(b)
	}
	return v
}
func (r *ldapFieldsReader) open(tag byte) ldapFieldsElement {
	e := ldapFieldsElement{start: r.at, index: len(r.fields)}
	if r.uint("Tag", 1) != uint64(tag) {
		r.fail(fmt.Sprintf("expected tag 0x%02x", tag))
	}
	n := r.uint("Length Prefix", 1)
	if n >= 128 {
		width := int(n & 127)
		if width == 0 || width > 4 {
			r.fail("unsupported BER length width")
			return e
		}
		n = r.uint("Long Length", width)
	}
	if n > uint64(r.end-r.at) {
		r.fail("element exceeds enclosing boundary")
		return e
	}
	e.content, e.end = r.at, r.at+int(n)
	return e
}
func (r *ldapFieldsReader) close(e ldapFieldsElement, name string, list bool) {
	if r.at != e.end {
		r.fail("unconsumed " + name)
	}
	children := append([]tlsCertificateField(nil), r.fields[e.index:]...)
	r.fields = append(r.fields[:e.index], tlsCertificateField{Name: name, Start: e.start, End: r.at, Children: children, List: list})
}
func (r *ldapFieldsReader) within(e ldapFieldsElement, visit func()) {
	if r.err != nil {
		return
	}
	outer := r.end
	r.end = e.end
	visit()
	r.end = outer
}
func (r *ldapFieldsReader) number(name string, maximum uint64) uint64 {
	e := r.open(2)
	n := e.end - e.content
	if r.err != nil {
		return 0
	}
	if n < 1 || n > 4 {
		r.fail("invalid INTEGER width")
		return 0
	}
	b := r.wire[e.content:e.end]
	if b[0]&128 != 0 || n > 1 && b[0] == 0 && b[1]&128 == 0 {
		r.fail("negative or nonminimal INTEGER")
		return 0
	}
	v := r.uint(name, n)
	if v > maximum {
		r.fail("INTEGER range for " + name)
	}
	r.close(e, name+" Encoding", false)
	return v
}
func (r *ldapFieldsReader) octets(tag byte, name string, text bool) map[string]any {
	e := r.open(tag)
	typ := "raw"
	if text {
		typ = "string"
	}
	b := r.take(name, typ, e.end-e.content)
	if text && !utf8.Valid(b) {
		r.fail("invalid UTF-8 in " + name)
	}
	r.close(e, name+" Encoding", false)
	return map[string]any{"Value": bytes.Clone(b), "Relative Byte Range": [2]int{e.content, e.end}}
}
func ldapFieldsNumericOID(b []byte) bool {
	arcs := strings.Split(string(b), ".")
	if len(arcs) < 2 {
		return false
	}
	for _, arc := range arcs {
		if len(arc) == 0 || len(arc) > 1 && arc[0] == '0' {
			return false
		}
		for _, c := range []byte(arc) {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
func (r *ldapFieldsReader) controls(info map[string]any) {
	e := r.open(0xa0)
	var controls []map[string]any
	r.within(e, func() {
		// Only sequence members are list elements, not the envelope tag/length.
		start, index := r.at, len(r.fields)
		for r.err == nil && r.at < r.end {
			if len(controls) == ldapFieldsMaxControls {
				r.fail("controls resource limit")
				break
			}
			c := r.open(0x30)
			m := map[string]any{"Criticality": false, "Control Value Present": false, "Control Semantics Applied": false}
			r.within(c, func() {
				oid := r.octets(4, "Control OID", true)
				if !ldapFieldsNumericOID(oid["Value"].([]byte)) {
					r.fail("invalid numeric control OID")
				}
				m["OID"] = oid
				if r.err == nil && r.at < r.end && r.wire[r.at] == 1 {
					b := r.open(1)
					if b.end-b.content != 1 {
						r.fail("invalid BOOLEAN width")
					}
					if r.uint("Criticality", 1) != 255 {
						r.fail("true must be FF; default false must be absent")
					}
					r.close(b, "Criticality Encoding", false)
					m["Criticality"] = true
				}
				if r.err == nil && r.at < r.end {
					m["Control Value"] = r.octets(4, "Control Value", false)
					m["Control Value Present"] = true
				}
			})
			r.close(c, "Control", false)
			controls = append(controls, m)
		}
		children := append([]tlsCertificateField(nil), r.fields[index:]...)
		r.fields = append(r.fields[:index], tlsCertificateField{Name: "Controls", Start: start, End: r.at, Children: children, List: true})
	})
	r.close(e, "Controls Encoding", false)
	info["Controls"] = controls
}
func decodeLDAPFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if profile != "bind-request" {
		return nil, nil, fmt.Errorf("ldap-fields: unknown explicit profile")
	}
	if len(wire) == 0 || len(wire) > ldapFieldsMaxBytes {
		return nil, nil, fmt.Errorf("ldap-fields: 1..1048576 byte boundary required")
	}
	r := &ldapFieldsReader{wire: wire, end: len(wire)}
	info := map[string]any{"Layout Context": profile, "Message Name": "BindRequest", "Session State Validated": false, "Directory Name Validated": false, "Transport Context Validated": false, "Control Semantics Applied": false}
	outer := r.open(0x30)
	r.within(outer, func() {
		id := r.number("Message ID", 2147483647)
		if id == 0 {
			r.fail("BindRequest requires nonzero message ID")
		}
		info["Message ID"] = id
		bind := r.open(0x60)
		r.within(bind, func() {
			version := r.number("Version", 127)
			if version != 3 {
				r.fail("explicit LDAP version 3 profile required")
			}
			info["Version"] = version
			info["Directory Name"] = r.octets(4, "Directory Name", true)
			if r.err != nil {
				return
			}
			if r.at == r.end {
				r.fail("missing bind choice")
				return
			}
			switch r.wire[r.at] {
			case 0x80:
				info["Choice"] = "simple"
				info["Simple Octets"] = r.octets(0x80, "Simple Octets", false)
			case 0xa3:
				info["Choice"] = "sasl"
				sasl := r.open(0xa3)
				r.within(sasl, func() {
					info["Mechanism"] = r.octets(4, "Mechanism", true)
					info["SASL Octets Present"] = false
					if r.err == nil && r.at < r.end {
						info["SASL Octets"] = r.octets(4, "SASL Octets", false)
						info["SASL Octets Present"] = true
					}
				})
				r.close(sasl, "SASL", false)
			default:
				r.fail("unsupported bind choice")
			}
		})
		r.close(bind, "BindRequest", false)
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
func parseLDAPFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	if profile != "bind-request" {
		return fmt.Errorf("ldap-fields: unknown explicit profile")
	}
	return parseCertificateFieldTree(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeLDAPFields(wire, profile)
	}, "ldap-fields")
}
