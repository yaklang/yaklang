package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"fmt"
	"math/big"
	"time"
	"unicode/utf8"
)

const x509DERMaxNodes = 16384
const x509DERMaxDepth = 32

// x509DERElement retains offsets into the caller's exact input, including the
// tag and length encoding. Decoded metadata never pretends to have wire spans.
type x509DERElement struct {
	start, tagEnd, content, end int
	tag                         byte
	name                        string
	children                    []*x509DERElement
	depth                       int
	embedded, bitString         bool
	valueType                   string
	payloadFields               []tlsCertificateField
}

type x509DERReader struct {
	wire         []byte
	nodes        int
	payloadNodes int
}

func (r *x509DERReader) element(at, limit, depth int) (*x509DERElement, error) {
	if depth > x509DERMaxDepth || r.nodes+r.payloadNodes >= x509DERMaxNodes {
		return nil, fmt.Errorf("x509-der: depth/node resource limit exceeded")
	}
	r.nodes++
	if at < 0 || limit > len(r.wire) || at >= limit {
		return nil, fmt.Errorf("x509-der: missing element")
	}
	n := &x509DERElement{start: at, tag: r.wire[at], name: "Element", depth: depth}
	at++
	if n.tag&31 == 31 {
		var tagNumber uint32
		for count := 0; ; count++ {
			if at >= limit || count >= 5 || tagNumber > 0x1ffffff || (count == 0 && r.wire[at]&127 == 0) {
				return nil, fmt.Errorf("x509-der: invalid high tag encoding")
			}
			b := r.wire[at]
			at++
			tagNumber = tagNumber<<7 | uint32(b&127)
			if b&128 == 0 {
				break
			}
		}
		if tagNumber < 31 {
			return nil, fmt.Errorf("x509-der: nonminimal high tag")
		}
	}
	n.tagEnd = at
	if at >= limit {
		return nil, fmt.Errorf("x509-der: missing length")
	}
	length := int(r.wire[at])
	at++
	if length >= 128 {
		count := length & 127
		if count == 0 || count > 3 || count > limit-at || r.wire[at] == 0 {
			return nil, fmt.Errorf("x509-der: indefinite, oversized or nonminimal length")
		}
		length = 0
		for i := 0; i < count; i++ {
			length = length<<8 | int(r.wire[at])
			at++
		}
		if length < 128 {
			return nil, fmt.Errorf("x509-der: nonminimal long length")
		}
	}
	if length > limit-at {
		return nil, fmt.Errorf("x509-der: element exceeds parent boundary")
	}
	n.content, n.end = at, at+length
	if n.tag == 0 {
		return nil, fmt.Errorf("x509-der: unsupported reserved tag")
	}
	if n.tag&32 != 0 {
		// Known universal primitive values cannot use BER constructed forms.
		if n.tag&192 == 0 && n.tag&31 != 16 && n.tag&31 != 17 {
			return nil, fmt.Errorf("x509-der: unsupported constructed universal tag")
		}
		for at < n.end {
			child, err := r.element(at, n.end, depth+1)
			if err != nil {
				return nil, err
			}
			n.children = append(n.children, child)
			at = child.end
		}
		if n.tag == 0x31 {
			for i := 1; i < len(n.children); i++ {
				a, b := n.children[i-1], n.children[i]
				if bytes.Compare(r.wire[a.start:a.end], r.wire[b.start:b.end]) > 0 {
					return nil, fmt.Errorf("x509-der: noncanonical SET ordering")
				}
			}
		}
	} else {
		v := r.wire[n.content:n.end]
		switch n.tag {
		case 1:
			if len(v) != 1 || (v[0] != 0 && v[0] != 255) {
				return nil, fmt.Errorf("x509-der: invalid boolean")
			}
		case 2, 10:
			if len(v) == 0 || len(v) > 1 && (v[0] == 0 && v[1]&128 == 0 || v[0] == 255 && v[1]&128 != 0) {
				return nil, fmt.Errorf("x509-der: invalid integer")
			}
		case 3:
			if !x509DERBitStringValid(v) {
				return nil, fmt.Errorf("x509-der: invalid bit string")
			}
		case 5:
			if len(v) != 0 {
				return nil, fmt.Errorf("x509-der: nonempty NULL")
			}
		case 6:
			if _, err := r.oid(n); err != nil {
				return nil, err
			}
		case 12:
			if !utf8.Valid(v) {
				return nil, fmt.Errorf("x509-der: invalid UTF8String")
			}
		case 16, 17:
			return nil, fmt.Errorf("x509-der: primitive sequence/set")
		}
	}
	return n, nil
}

func x509DERBitStringValid(v []byte) bool {
	return len(v) > 0 && v[0] <= 7 && (len(v) > 1 || v[0] == 0) && (v[0] == 0 || v[len(v)-1]&byte((1<<v[0])-1) == 0)
}

func (r *x509DERReader) oid(n *x509DERElement) (string, error) {
	if n.tag != 6 {
		return "", fmt.Errorf("x509-der: object identifier required")
	}
	var oid asn1.ObjectIdentifier
	rest, err := asn1.Unmarshal(r.wire[n.start:n.end], &oid)
	if err != nil || len(rest) != 0 {
		return "", fmt.Errorf("x509-der: invalid object identifier")
	}
	return oid.String(), nil
}

func (r *x509DERReader) algorithm(n *x509DERElement, name string) (string, error) {
	if n.tag != 0x30 || len(n.children) < 1 || len(n.children) > 2 {
		return "", fmt.Errorf("x509-der: invalid algorithm identifier")
	}
	n.name, n.children[0].name = name, "Algorithm OID"
	if len(n.children) == 2 {
		n.children[1].name = "Algorithm Parameters"
	}
	return r.oid(n.children[0])
}

func (r *x509DERReader) name(n *x509DERElement, name string) error {
	if n.tag != 0x30 {
		return fmt.Errorf("x509-der: name sequence required")
	}
	n.name = name
	for _, rdn := range n.children {
		if rdn.tag != 0x31 || len(rdn.children) == 0 {
			return fmt.Errorf("x509-der: nonempty relative name SET required")
		}
		rdn.name = "Relative Distinguished Name"
		for _, attr := range rdn.children {
			if attr.tag != 0x30 || len(attr.children) != 2 {
				return fmt.Errorf("x509-der: attribute type/value pair required")
			}
			if _, err := r.oid(attr.children[0]); err != nil {
				return err
			}
			attr.name = "Attribute"
			attr.children[0].name, attr.children[1].name = "Attribute OID", "Attribute Value"
		}
	}
	return nil
}

func (r *x509DERReader) date(n *x509DERElement, name string) (string, error) {
	n.name = name
	value := string(r.wire[n.content:n.end])
	layout := "060102150405Z"
	if n.tag == 24 {
		layout = "20060102150405Z"
	} else if n.tag != 23 {
		return "", fmt.Errorf("x509-der: UTC or generalized time required")
	}
	if len(value) != len(layout) || value[len(value)-1] != 'Z' {
		return "", fmt.Errorf("x509-der: whole-second UTC time required")
	}
	for _, b := range []byte(value[:len(value)-1]) {
		if b < '0' || b > '9' {
			return "", fmt.Errorf("x509-der: invalid time digit")
		}
	}
	d, err := time.Parse(layout, value)
	if err != nil {
		return "", fmt.Errorf("x509-der: invalid calendar time")
	}
	// time.Parse uses 1969 rather than the ASN.1 1950 century boundary.
	if n.tag == 23 && value[:2] >= "50" && d.Year() >= 2000 {
		d = d.AddDate(-100, 0, 0)
	}
	return d.Format(time.RFC3339), nil
}

func (r *x509DERReader) field(n *x509DERElement) tlsCertificateField {
	f := tlsCertificateField{Name: n.name, Start: n.start, End: n.end}
	// The validated DER tree already gives the exact child count. Allocate its
	// projection once instead of repeatedly copying pointer-bearing descriptors.
	bitString := n.tag == 3 || n.bitString || n.name == "Issuer Unique ID" || n.name == "Subject Unique ID"
	count := 3
	switch {
	case n.payloadFields != nil:
		count = 2 + len(n.payloadFields)
	case n.tag&32 != 0 || n.embedded:
		count = 2 + len(n.children)
	case bitString:
		count = 4
	}
	f.Children = make([]tlsCertificateField, 2, count)
	f.Children[0] = tlsCertificateLeaf("DER Tag", "raw", n.start, n.tagEnd)
	f.Children[1] = tlsCertificateLeaf("DER Length Encoding", "raw", n.tagEnd, n.content)
	if n.payloadFields != nil {
		f.Children = append(f.Children, n.payloadFields...)
	} else if n.tag&32 != 0 || n.embedded {
		for _, child := range n.children {
			f.Children = append(f.Children, r.field(child))
		}
	} else if bitString {
		f.Children = append(f.Children, tlsCertificateLeaf("Unused Bits", "uint8", n.content, n.content+1), tlsCertificateLeaf("Bit String Bytes", "raw", n.content+1, n.end))
	} else {
		typ := "raw"
		if n.tag == 12 || n.tag == 19 || n.tag == 22 || n.tag == 23 || n.tag == 24 {
			typ = "string"
		}
		if n.name == "Version Number" {
			typ = "uint8"
		}
		if n.valueType != "" {
			typ = n.valueType
		}
		f.Children = append(f.Children, tlsCertificateLeaf("Value", typ, n.content, n.end))
	}
	return f
}

func decodeX509CertificateDER(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	return decodeX509CertificateProfile(wire, false, false)
}

func decodeX509CertificateDERExtensions(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	return decodeX509CertificateProfile(wire, true, false)
}

func decodeX509CertificateDERPublicKey(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	return decodeX509CertificateProfile(wire, true, true)
}

func decodeX509CertificateProfile(wire []byte, expandExtensions, expandPublicKey bool) ([]tlsCertificateField, map[string]any, error) {
	fail := func(message string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("x509-der: %s", message)
	}
	if len(wire) < 2 || len(wire) > tlsCertificateMaxBytes {
		return fail("explicit certificate size outside 2..1048576 bytes")
	}
	r := &x509DERReader{wire: wire}
	root, err := r.element(0, len(wire), 0)
	if err != nil {
		return nil, nil, err
	}
	if root.end != len(wire) || root.tag != 0x30 || len(root.children) != 3 {
		return fail("exact three-element Certificate sequence required")
	}
	root.name = "Certificate"
	tbs, signatureAlgorithm, signature := root.children[0], root.children[1], root.children[2]
	if tbs.tag != 0x30 || signature.tag != 3 {
		return fail("invalid TBS or signature type")
	}
	tbs.name, signature.name = "TBSCertificate", "Signature Value"
	outerOID, err := r.algorithm(signatureAlgorithm, "Signature Algorithm")
	if err != nil {
		return nil, nil, err
	}
	children := tbs.children
	version := 0
	if len(children) > 0 && children[0].tag == 0xa0 {
		v := children[0]
		if len(v.children) != 1 || v.children[0].tag != 2 || v.children[0].end-v.children[0].content != 1 {
			return fail("invalid explicit version")
		}
		version = int(wire[v.children[0].content])
		if version < 1 || version > 2 {
			return fail("explicit version must be v2/v3; default v1 is omitted")
		}
		v.name, v.children[0].name = "Explicit Version", "Version Number"
		children = children[1:]
	}
	if len(children) < 6 || children[0].tag != 2 {
		return fail("missing required TBS fields")
	}
	serial := children[0]
	serial.name = "Serial Number"
	serialBytes := wire[serial.content:serial.end]
	if len(serialBytes) > 1024 {
		return fail("serial number exceeds local 1024-byte limit")
	}
	serialNumber := new(big.Int).SetBytes(serialBytes)
	if serialBytes[0]&128 != 0 {
		serialNumber.Sub(serialNumber, new(big.Int).Lsh(big.NewInt(1), uint(len(serialBytes))*8))
	}
	innerOID, err := r.algorithm(children[1], "TBS Signature Algorithm")
	if err != nil {
		return nil, nil, err
	}
	if err := r.name(children[2], "Issuer"); err != nil {
		return nil, nil, err
	}
	validity := children[3]
	if validity.tag != 0x30 || len(validity.children) != 2 {
		return fail("invalid validity pair")
	}
	validity.name = "Validity"
	notBefore, err := r.date(validity.children[0], "Not Before")
	if err != nil {
		return nil, nil, err
	}
	notAfter, err := r.date(validity.children[1], "Not After")
	if err != nil {
		return nil, nil, err
	}
	if err := r.name(children[4], "Subject"); err != nil {
		return nil, nil, err
	}
	spki := children[5]
	if spki.tag != 0x30 || len(spki.children) != 2 || spki.children[1].tag != 3 {
		return fail("invalid subject public key info")
	}
	spki.name, spki.children[1].name = "Subject Public Key Info", "Subject Public Key"
	keyOID, err := r.algorithm(spki.children[0], "Public Key Algorithm")
	if err != nil {
		return nil, nil, err
	}
	var publicKeyInfo map[string]any
	if expandPublicKey {
		publicKeyInfo, err = r.publicKey(spki, keyOID)
		if err != nil {
			return nil, nil, err
		}
	}
	var extensionOIDs []string
	var extensionDetails []map[string]any
	decodedExtensions := 0
	lastOptional := 0
	for _, optional := range children[6:] {
		index := int(optional.tag & 31)
		if index <= lastOptional || index > 3 || index == 0 || version == 0 {
			return fail("invalid optional field order/version")
		}
		lastOptional = index
		if index <= 2 {
			if optional.tag != byte(0x80+index) || !x509DERBitStringValid(wire[optional.content:optional.end]) {
				return fail("invalid implicit unique identifier")
			}
			optional.name = "Issuer Unique ID"
			if index == 2 {
				optional.name = "Subject Unique ID"
			}
			continue
		}
		if version != 2 || optional.tag != 0xa3 || len(optional.children) != 1 || optional.children[0].tag != 0x30 {
			return fail("v3 explicit extension sequence required")
		}
		optional.name, optional.children[0].name = "Explicit Extensions", "Extensions"
		extensions := optional.children[0].children
		if len(extensions) == 0 || len(extensions) > 1024 {
			return fail("extension count outside 1..1024")
		}
		seen := map[string]bool{}
		for _, ext := range extensions {
			if ext.tag != 0x30 || len(ext.children) < 2 || len(ext.children) > 3 {
				return fail("invalid extension sequence")
			}
			ext.name, ext.children[0].name = "Extension", "Extension OID"
			oid, err := r.oid(ext.children[0])
			if err != nil {
				return nil, nil, err
			}
			if seen[oid] {
				return fail("duplicate extension identifier")
			}
			seen[oid] = true
			extensionOIDs = append(extensionOIDs, oid)
			if len(ext.children) == 3 {
				critical := ext.children[1]
				if critical.tag != 1 || wire[critical.content] != 255 {
					return fail("explicit critical flag must be TRUE; default FALSE is omitted")
				}
				critical.name = "Critical"
			}
			value := ext.children[len(ext.children)-1]
			if value.tag != 4 {
				return fail("extension value octet string required")
			}
			value.name = "Extension Value"
			if expandExtensions {
				detail, decoded, err := r.extension(oid, value)
				if err != nil {
					return nil, nil, fmt.Errorf("x509-extension %s: %w", oid, err)
				}
				extensionDetails = append(extensionDetails, detail)
				if decoded {
					decodedExtensions++
				}
			}
		}
	}
	info := map[string]any{
		"Profile": "X.509 Certificate DER layout", "Certificate Version": version + 1,
		"Serial Number Decimal": serialNumber.String(), "Signature Algorithm OID": outerOID,
		"TBS Signature Algorithm OID": innerOID, "Public Key Algorithm OID": keyOID,
		"Signature Algorithm Encodings Match": bytes.Equal(wire[children[1].start:children[1].end], wire[signatureAlgorithm.start:signatureAlgorithm.end]),
		"Not Before UTC":                      notBefore, "Not After UTC": notAfter,
		"Extension Count": len(extensionOIDs), "Extension OIDs": extensionOIDs,
		"DER Element Count": r.nodes, "DER Framing Parsed": true,
		"All DER Semantics Validated": false, "Extension Contents Decoded": false,
		"Public Key Validated": false, "Signature Verified": false,
		"Certificate Chain Validated": false, "Certificate Trust Validated": false,
		"Peer Identity Validated": false, "Current Validity Checked": false,
		"TCP Reassembly Performed": false, "Structured Generation Supported": false,
	}
	if expandExtensions {
		info["Profile"] = "X.509 Certificate DER with Extensions"
		info["Extension Decoding Enabled"] = true
		info["Extension Contents Decoded"] = len(extensionOIDs) > 0 && decodedExtensions == len(extensionOIDs)
		info["Decoded Extension Count"] = decodedExtensions
		info["Opaque Extension Count"] = len(extensionOIDs) - decodedExtensions
		info["Extension Details"] = extensionDetails
		info["Extension Semantics Validated"] = false
		info["SCT Signatures Verified"] = false
	}
	if expandPublicKey {
		info["Profile"] = "X.509 Certificate DER with Extensions and Public Key"
		info["Public Key Decoding Enabled"] = true
		info["Public Key Fields Decoded"] = publicKeyInfo["Decoded"]
		info["Public Key Details"] = publicKeyInfo
	}
	return []tlsCertificateField{r.field(root)}, info, nil
}
