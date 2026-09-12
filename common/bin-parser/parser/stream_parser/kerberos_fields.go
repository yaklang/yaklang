package stream_parser

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const kerberosFieldsMaxBytes = 1 << 20
const kerberosFieldsMaxList = 1024

// Passive, exact DER message layouts (RFC 4120 and RFC 6113). Ciphertext,
// checksums and unrecognized PA-DATA stay bytes; no keys or session outcomes
// are computed. Integer metadata retains signed values without treating short
// two's-complement DER encodings as a padded native machine integer.
type kerberosFieldsDecoder struct {
	r                                      *x509DERReader
	values, padata                         []map[string]any
	ciphers, cipherBytes, tickets, unknown int
}

type kerberosFieldSpec struct {
	id       byte
	name     string
	optional bool
}

func (d *kerberosFieldsDecoder) sequence(n *x509DERElement, name string, extensible bool, specs ...kerberosFieldSpec) ([31]*x509DERElement, error) {
	var out [31]*x509DERElement
	if n.tag != 0x30 {
		return out, fmt.Errorf("kerberos-fields: %s requires SEQUENCE", name)
	}
	n.name = name
	// Explicit context tags are the closed range 0..30. Stack tables avoid
	// allocating and hashing two maps at every nested SEQUENCE.
	var known [31]*kerberosFieldSpec
	for _, s := range specs {
		if s.id >= 31 {
			return out, fmt.Errorf("kerberos-fields: invalid field specification")
		}
	}
	for i := range specs {
		known[specs[i].id] = &specs[i]
	}
	previous := -1
	for _, field := range n.children {
		id := int(field.tag & 31)
		if field.tag&0xe0 != 0xa0 || id == 31 || id <= previous || len(field.children) != 1 {
			return out, fmt.Errorf("kerberos-fields: invalid explicit field/order in %s", name)
		}
		previous = id
		s := known[id]
		if s == nil {
			if !extensible {
				return out, fmt.Errorf("kerberos-fields: unsupported field %d in %s", id, name)
			}
			field.name = fmt.Sprintf("Unparsed %s Field %d", name, id)
			field.payloadFields = []tlsCertificateField{tlsCertificateLeaf("Encoded Extension", "raw", field.content, field.end)}
			d.unknown++
			continue
		}
		field.name = s.name
		field.children[0].name = s.name + " Encoding"
		out[s.id] = field.children[0]
	}
	for _, s := range specs {
		if !s.optional && out[s.id] == nil {
			return out, fmt.Errorf("kerberos-fields: missing %s", s.name)
		}
	}
	return out, nil
}

func (d *kerberosFieldsDecoder) value(n *x509DERElement, kind string, v any) {
	d.values = append(d.values, map[string]any{"Field": n.name, "Kind": kind, "Value": v, "Relative Byte Range": [2]int{n.content, n.end}})
}

func (d *kerberosFieldsDecoder) integer(n *x509DERElement, unsigned bool, maximum int64) (int64, error) {
	if n == nil {
		return 0, nil
	}
	if n.tag != 2 || n.end-n.content < 1 || n.end-n.content > 5 {
		return 0, fmt.Errorf("kerberos-fields: bounded INTEGER required for %s", n.name)
	}
	b := d.r.wire[n.content:n.end]
	var v int64
	if b[0]&128 != 0 {
		v = -1
	}
	for _, octet := range b {
		v = v<<8 | int64(octet)
	}
	minimum := int64(math.MinInt32)
	if unsigned {
		minimum = 0
	}
	if v < minimum || v > maximum {
		return 0, fmt.Errorf("kerberos-fields: integer range for %s", n.name)
	}
	d.value(n, "Integer", v)
	return v, nil
}

func (d *kerberosFieldsDecoder) number(n *x509DERElement) error {
	_, err := d.integer(n, false, math.MaxInt32)
	return err
}
func (d *kerberosFieldsDecoder) u32(n *x509DERElement) error {
	_, err := d.integer(n, true, math.MaxUint32)
	return err
}
func (d *kerberosFieldsDecoder) fixed(n *x509DERElement, want int64) error {
	v, err := d.integer(n, false, math.MaxInt32)
	if err != nil {
		return err
	}
	if n == nil || v != want {
		return fmt.Errorf("kerberos-fields: required integer %d", want)
	}
	return nil
}
func (d *kerberosFieldsDecoder) string(n *x509DERElement) error {
	if n == nil {
		return nil
	}
	if n.tag != 0x1b || n.end-n.content > 65536 {
		return fmt.Errorf("kerberos-fields: bounded KerberosString required")
	}
	for _, b := range d.r.wire[n.content:n.end] {
		if b > 127 {
			return fmt.Errorf("kerberos-fields: KerberosString outside IA5 profile")
		}
	}
	n.valueType = "string"
	d.value(n, "KerberosString", string(d.r.wire[n.content:n.end]))
	return nil
}
func (d *kerberosFieldsDecoder) date(n *x509DERElement) error {
	if n == nil {
		return nil
	}
	if n.tag != 24 {
		return fmt.Errorf("kerberos-fields: generalized UTC time required")
	}
	v, err := d.r.date(n, n.name)
	if err == nil {
		d.value(n, "KerberosTime", v)
	}
	return err
}
func (d *kerberosFieldsDecoder) octets(n *x509DERElement) error {
	if n == nil || n.tag != 4 {
		return fmt.Errorf("kerberos-fields: OCTET STRING required")
	}
	return nil
}
func (d *kerberosFieldsDecoder) flags(n *x509DERElement) error {
	if n == nil || n.tag != 3 || (n.end-n.content-1)*8-int(d.r.wire[n.content]) < 32 {
		return fmt.Errorf("kerberos-fields: flags require at least 32 bits")
	}
	d.value(n, "Flags", map[string]any{"First 32 Bits": uint64(binary.BigEndian.Uint32(d.r.wire[n.content+1:])), "Meaningful Bit Count": (n.end-n.content-1)*8 - int(d.r.wire[n.content])})
	return nil
}
func (d *kerberosFieldsDecoder) list(n *x509DERElement, name string, nonempty bool, visit func(*x509DERElement) error) error {
	if n == nil {
		return nil
	}
	if n.tag != 0x30 || len(n.children) > kerberosFieldsMaxList || nonempty && len(n.children) == 0 {
		return fmt.Errorf("kerberos-fields: invalid or excessive %s list", name)
	}
	n.name = name
	for i, item := range n.children {
		item.name = fmt.Sprintf("%s Item %d", name, i)
		if err := visit(item); err != nil {
			return err
		}
	}
	return nil
}
func (d *kerberosFieldsDecoder) principal(n *x509DERElement) error {
	if n == nil {
		return nil
	}
	f, err := d.sequence(n, n.name, false, kerberosFieldSpec{0, "Name Type", false}, kerberosFieldSpec{1, "Name Components", false})
	if err != nil {
		return err
	}
	if err := d.number(f[0]); err != nil {
		return err
	}
	return d.list(f[1], "Name Components", true, d.string)
}
func (d *kerberosFieldsDecoder) encrypted(n *x509DERElement) error {
	if n == nil {
		return nil
	}
	f, err := d.sequence(n, n.name, false, kerberosFieldSpec{0, "Encryption Type", false}, kerberosFieldSpec{1, "Key Version", true}, kerberosFieldSpec{2, "Cipher Bytes", false})
	if err != nil {
		return err
	}
	if err = d.number(f[0]); err != nil {
		return err
	}
	if err = d.u32(f[1]); err != nil {
		return err
	}
	if err = d.octets(f[2]); err != nil {
		return err
	}
	d.ciphers++
	d.cipherBytes += f[2].end - f[2].content
	return nil
}
func (d *kerberosFieldsDecoder) checksum(n *x509DERElement) error {
	f, err := d.sequence(n, n.name, false, kerberosFieldSpec{0, "Checksum Type", false}, kerberosFieldSpec{1, "Checksum Bytes", false})
	if err != nil {
		return err
	}
	if err = d.number(f[0]); err != nil {
		return err
	}
	return d.octets(f[1])
}
func (d *kerberosFieldsDecoder) application(n *x509DERElement, tag byte, name string) (*x509DERElement, error) {
	if n.tag != tag || len(n.children) != 1 || n.children[0].tag != 0x30 {
		return nil, fmt.Errorf("kerberos-fields: %s application wrapper required", name)
	}
	n.name = name
	return n.children[0], nil
}
func (d *kerberosFieldsDecoder) ticket(n *x509DERElement) error {
	s, err := d.application(n, 0x61, "Ticket")
	if err != nil {
		return err
	}
	f, err := d.sequence(s, "Ticket Fields", false, kerberosFieldSpec{0, "Ticket Version", false}, kerberosFieldSpec{1, "Ticket Realm", false}, kerberosFieldSpec{2, "Ticket Server Name", false}, kerberosFieldSpec{3, "Ticket Encoded Part", false})
	if err != nil {
		return err
	}
	if err = d.fixed(f[0], 5); err != nil {
		return err
	}
	if err = d.string(f[1]); err != nil {
		return err
	}
	if err = d.principal(f[2]); err != nil {
		return err
	}
	if err = d.encrypted(f[3]); err != nil {
		return err
	}
	d.tickets++
	return nil
}
func (d *kerberosFieldsDecoder) address(n *x509DERElement) error {
	f, err := d.sequence(n, n.name, false, kerberosFieldSpec{0, "Address Type", false}, kerberosFieldSpec{1, "Address Bytes", false})
	if err != nil {
		return err
	}
	if err = d.number(f[0]); err != nil {
		return err
	}
	return d.octets(f[1])
}
func (d *kerberosFieldsDecoder) requestBody(n *x509DERElement) error {
	f, err := d.sequence(n, "KDC Request Body", false,
		kerberosFieldSpec{0, "KDC Options", false}, kerberosFieldSpec{1, "Client Name", true}, kerberosFieldSpec{2, "Realm", false}, kerberosFieldSpec{3, "Server Name", true},
		kerberosFieldSpec{4, "From Time", true}, kerberosFieldSpec{5, "Till Time", false}, kerberosFieldSpec{6, "Renew Till Time", true}, kerberosFieldSpec{7, "Nonce", false},
		kerberosFieldSpec{8, "Requested Encryption Types", false}, kerberosFieldSpec{9, "Host Addresses", true}, kerberosFieldSpec{10, "Encoded Authorization Data", true}, kerberosFieldSpec{11, "Additional Tickets", true})
	if err != nil {
		return err
	}
	for _, call := range []func() error{
		func() error { return d.flags(f[0]) },
		func() error { return d.principal(f[1]) },
		func() error { return d.string(f[2]) },
		func() error { return d.principal(f[3]) },
		func() error { return d.date(f[4]) },
		func() error { return d.date(f[5]) },
		func() error { return d.date(f[6]) },
		func() error { return d.u32(f[7]) },
		func() error { return d.list(f[8], "Requested Encryption Types", true, d.number) },
		func() error { return d.list(f[9], "Host Addresses", false, d.address) },
		func() error { return d.encrypted(f[10]) },
		func() error { return d.list(f[11], "Additional Tickets", false, d.ticket) },
	} {
		if err := call(); err != nil {
			return err
		}
	}
	return nil
}

func (d *kerberosFieldsDecoder) embedded(n *x509DERElement, visit func(*x509DERElement) error) error {
	if err := d.octets(n); err != nil {
		return err
	}
	inner, err := d.r.element(n.content, n.end, n.depth+1)
	if err != nil {
		return err
	}
	if inner.end != n.end {
		return fmt.Errorf("kerberos-fields: trailing embedded DER")
	}
	inner.name = n.name + " Inner"
	if err = visit(inner); err != nil {
		return err
	}
	n.embedded, n.children = true, []*x509DERElement{inner}
	return nil
}
func (d *kerberosFieldsDecoder) fast(n *x509DERElement, message byte) error {
	if n.tag != 0xa0 || len(n.children) != 1 {
		return fmt.Errorf("kerberos-fields: unsupported FAST choice")
	}
	n.name = "FAST Armored Data"
	request := message == 10 || message == 12
	if !request {
		f, err := d.sequence(n.children[0], "FAST Reply Fields", true, kerberosFieldSpec{0, "FAST Reply Encoded Part", false})
		if err != nil {
			return err
		}
		return d.encrypted(f[0])
	}
	f, err := d.sequence(n.children[0], "FAST Request Fields", true, kerberosFieldSpec{0, "FAST Armor", true}, kerberosFieldSpec{1, "FAST Request Checksum", false}, kerberosFieldSpec{2, "FAST Request Encoded Part", false})
	if err != nil {
		return err
	}
	if message == 10 && f[0] == nil {
		return fmt.Errorf("kerberos-fields: AS FAST request requires armor")
	}
	if f[0] != nil {
		armor, err := d.sequence(f[0], "FAST Armor Fields", true, kerberosFieldSpec{0, "Armor Type", false}, kerberosFieldSpec{1, "Armor Bytes", false})
		if err != nil {
			return err
		}
		if err = d.number(armor[0]); err != nil {
			return err
		}
		if err = d.octets(armor[1]); err != nil {
			return err
		}
		// Armor bytes require their own mechanism context; retain them raw.
	}
	if err = d.checksum(f[1]); err != nil {
		return err
	}
	return d.encrypted(f[2])
}
func (d *kerberosFieldsDecoder) paList(n *x509DERElement, message byte) error {
	return d.list(n, "PA-DATA", false, func(item *x509DERElement) error {
		f, err := d.sequence(item, item.name, false, kerberosFieldSpec{1, "PA Type", false}, kerberosFieldSpec{2, "PA Value", false})
		if err != nil {
			return err
		}
		typ, err := d.integer(f[1], false, math.MaxInt32)
		if err != nil {
			return err
		}
		if err = d.octets(f[2]); err != nil {
			return err
		}
		decoded := true
		switch typ {
		case 1:
			err = d.embedded(f[2], func(inner *x509DERElement) error {
				if inner.tag != 0x6e {
					return fmt.Errorf("kerberos-fields: PA-TGS-REQ requires AP-REQ")
				}
				return d.message(inner)
			})
		case 2:
			err = d.embedded(f[2], d.encrypted)
		case 136:
			err = d.embedded(f[2], func(inner *x509DERElement) error { return d.fast(inner, message) })
		case 149:
			// RFC 6806 section 11: request values are normally empty,
			// but receivers ignore nonempty values instead of rejecting
			// them. Such bytes, and reply values, remain explicitly raw.
			decoded = (message == 10 || message == 12) && f[2].content == f[2].end
		default:
			decoded = false
		}
		if err != nil {
			return err
		}
		d.padata = append(d.padata, map[string]any{"Type": typ, "Payload Layout Decoded": decoded, "Relative Byte Range": [2]int{f[2].content, f[2].end}})
		return nil
	})
}

func (d *kerberosFieldsDecoder) message(n *x509DERElement) error {
	msg := n.tag & 31
	name := map[byte]string{10: "AS-REQ", 11: "AS-REP", 12: "TGS-REQ", 13: "TGS-REP", 14: "AP-REQ", 15: "AP-REP", 30: "KRB-ERROR"}[msg]
	if name == "" {
		return fmt.Errorf("kerberos-fields: unsupported message application tag")
	}
	s, err := d.application(n, 0x60|msg, name)
	if err != nil {
		return err
	}
	var f [31]*x509DERElement
	if msg == 10 || msg == 12 {
		f, err = d.sequence(s, name+" Fields", false, kerberosFieldSpec{1, "Protocol Version", false}, kerberosFieldSpec{2, "Message Type", false}, kerberosFieldSpec{3, "PA-DATA", true}, kerberosFieldSpec{4, "Request Body", false})
		if err != nil {
			return err
		}
		if err = d.fixed(f[1], 5); err != nil {
			return err
		}
		if err = d.fixed(f[2], int64(msg)); err != nil {
			return err
		}
		if err = d.paList(f[3], msg); err != nil {
			return err
		}
		return d.requestBody(f[4])
	}
	specs := []kerberosFieldSpec{{0, "Protocol Version", false}, {1, "Message Type", false}}
	switch msg {
	case 11, 13:
		specs = append(specs, kerberosFieldSpec{2, "PA-DATA", true}, kerberosFieldSpec{3, "Client Realm", false}, kerberosFieldSpec{4, "Client Name", false}, kerberosFieldSpec{5, "Reply Ticket", false}, kerberosFieldSpec{6, "Reply Encoded Part", false})
	case 14:
		specs = append(specs, kerberosFieldSpec{2, "AP Options", false}, kerberosFieldSpec{3, "AP Ticket", false}, kerberosFieldSpec{4, "Encoded Authenticator", false})
	case 15:
		specs = append(specs, kerberosFieldSpec{2, "AP Reply Encoded Part", false})
	case 30:
		specs = append(specs, kerberosFieldSpec{2, "Client Time", true}, kerberosFieldSpec{3, "Client Microseconds", true}, kerberosFieldSpec{4, "Server Time", false}, kerberosFieldSpec{5, "Server Microseconds", false}, kerberosFieldSpec{6, "Error Code", false}, kerberosFieldSpec{7, "Client Realm", true}, kerberosFieldSpec{8, "Client Name", true}, kerberosFieldSpec{9, "Realm", false}, kerberosFieldSpec{10, "Server Name", false}, kerberosFieldSpec{11, "Error Text", true}, kerberosFieldSpec{12, "Error Data", true})
	}
	f, err = d.sequence(s, name+" Fields", false, specs...)
	if err != nil {
		return err
	}
	if err = d.fixed(f[0], 5); err != nil {
		return err
	}
	if err = d.fixed(f[1], int64(msg)); err != nil {
		return err
	}
	switch msg {
	case 11, 13:
		if err = d.paList(f[2], msg); err != nil {
			return err
		}
		if err = d.string(f[3]); err != nil {
			return err
		}
		if err = d.principal(f[4]); err != nil {
			return err
		}
		if err = d.ticket(f[5]); err != nil {
			return err
		}
		return d.encrypted(f[6])
	case 14:
		if err = d.flags(f[2]); err != nil {
			return err
		}
		if err = d.ticket(f[3]); err != nil {
			return err
		}
		return d.encrypted(f[4])
	case 15:
		return d.encrypted(f[2])
	case 30:
		for _, id := range []byte{2, 4} {
			if err = d.date(f[id]); err != nil {
				return err
			}
		}
		for _, id := range []byte{3, 5} {
			if _, err = d.integer(f[id], true, 999999); err != nil {
				return err
			}
		}
		code, err := d.integer(f[6], false, math.MaxInt32)
		if err != nil {
			return err
		}
		for _, id := range []byte{7, 9, 11} {
			if err = d.string(f[id]); err != nil {
				return err
			}
		}
		for _, id := range []byte{8, 10} {
			if err = d.principal(f[id]); err != nil {
				return err
			}
		}
		if f[12] != nil {
			if err = d.octets(f[12]); err != nil {
				return err
			}
			if code == 25 {
				return d.embedded(f[12], func(inner *x509DERElement) error { return d.paList(inner, 30) })
			}
		}
	}
	return nil
}

func decodeKerberosFields(wire []byte, tcp bool) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 2 || len(wire) > kerberosFieldsMaxBytes {
		return nil, nil, fmt.Errorf("kerberos-fields: input outside 2..1048576 bytes")
	}
	at := 0
	var fields []tlsCertificateField
	if tcp {
		if len(wire) < 6 || binary.BigEndian.Uint32(wire)&0x80000000 != 0 || uint64(binary.BigEndian.Uint32(wire)) != uint64(len(wire)-4) {
			return nil, nil, fmt.Errorf("kerberos-fields: exact nonreserved TCP record boundary required")
		}
		fields = append(fields, tlsCertificateLeaf("Record Length", "uint32", 0, 4))
		at = 4
	}
	d := &kerberosFieldsDecoder{r: &x509DERReader{wire: wire}}
	root, err := d.r.element(at, len(wire), 0)
	if err != nil {
		return nil, nil, fmt.Errorf("kerberos-fields: %w", err)
	}
	if root.end != len(wire) {
		return nil, nil, fmt.Errorf("kerberos-fields: trailing message bytes")
	}
	if err = d.message(root); err != nil {
		return nil, nil, err
	}
	fields = append(fields, d.r.field(root))
	info := map[string]any{"Profile": "Kerberos DER message fields", "Message Type": int64(root.tag & 31), "TCP Record Wrapped": tcp, "Decoded Fields": d.values, "PA-DATA Details": d.padata, "Ticket Count": d.tickets, "Encrypted Part Count": d.ciphers, "Cipher Byte Count": d.cipherBytes, "Unparsed Extension Count": d.unknown, "DER Element Count": d.r.nodes, "Decryption Performed": false, "Checksums Verified": false, "Peer Identity Validated": false, "Message Exchange Validated": false, "TCP Reassembly Performed": false, "Structured Generation Supported": false}
	return fields, info, nil
}

func parseKerberosFields(node *base.Node, process func(*base.Node) (func(bool), error), tcp bool) error {
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeKerberosFields(w, tcp) }, "kerberos-fields")
}
