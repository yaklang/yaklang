package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"fmt"
	"math/big"
)

var x509ExtensionNames = map[string]string{
	"2.5.29.14": "Subject Key Identifier", "2.5.29.15": "Key Usage",
	"2.5.29.16": "Private Key Usage Period", "2.5.29.17": "Subject Alternative Names",
	"2.5.29.19": "Basic Constraints", "2.5.29.31": "CRL Distribution Points",
	"2.5.29.32": "Certificate Policies", "2.5.29.35": "Authority Key Identifier",
	"2.5.29.37": "Extended Key Usage", "1.3.6.1.5.5.7.1.1": "Authority Information Access",
	"1.2.840.113533.7.65.0":   "Entrust Version Info",
	"1.3.6.1.4.1.11129.2.4.2": "Signed Certificate Timestamp List",
}

func x509ExtensionError() error { return fmt.Errorf("invalid extension field grammar") }

func x509ExtensionSequence(n *x509DERElement, minimum, maximum int) error {
	if n.tag != 0x30 || len(n.children) < minimum || len(n.children) > maximum {
		return x509ExtensionError()
	}
	return nil
}

func (r *x509DERReader) extensionInteger(n *x509DERElement) (string, error) {
	v := r.wire[n.content:n.end]
	if len(v) == 0 || len(v) > 1024 || v[0]&128 != 0 || len(v) > 1 && v[0] == 0 && v[1]&128 == 0 {
		return "", x509ExtensionError()
	}
	return new(big.Int).SetBytes(v).String(), nil
}

func (r *x509DERReader) extensionBits(n *x509DERElement) ([]int, error) {
	v := r.wire[n.content:n.end]
	if !x509DERBitStringValid(v) {
		return nil, x509ExtensionError()
	}
	n.bitString = true
	var set []int
	for bit := 0; bit < (len(v)-1)*8-int(v[0]); bit++ {
		if v[1+bit/8]&(128>>uint(bit%8)) != 0 {
			set = append(set, bit)
		}
	}
	return set, nil
}

func (r *x509DERReader) extensionIA5(n *x509DERElement) (string, error) {
	v := r.wire[n.content:n.end]
	for _, b := range v {
		if b > 127 {
			return "", x509ExtensionError()
		}
	}
	n.valueType = "string"
	return string(v), nil
}

func (r *x509DERReader) implicitOID(n *x509DERElement) (string, error) {
	encoded := bytes.Clone(r.wire[n.start:n.end])
	encoded[0] = 6 // Decode the implicit value; never change its on-wire tag.
	var oid asn1.ObjectIdentifier
	rest, err := asn1.Unmarshal(encoded, &oid)
	if err != nil || len(rest) != 0 {
		return "", x509ExtensionError()
	}
	return oid.String(), nil
}

// GeneralName choices are tagged explicitly in the field tree. ORAddress
// remains a generic DER subtree, and OtherName values remain type-specific
// ASN.1; neither is silently relabeled as a DNS name or URI.
func (r *x509DERReader) generalName(n *x509DERElement) (map[string]any, error) {
	info := map[string]any{"Tag": n.tag}
	switch n.tag {
	case 0x81, 0x82, 0x86:
		n.name = map[byte]string{0x81: "RFC822 Name", 0x82: "DNS Name", 0x86: "URI"}[n.tag]
		v, err := r.extensionIA5(n)
		if err != nil || len(v) == 0 {
			return nil, x509ExtensionError()
		}
		info["Value"] = v
	case 0x87:
		n.name = "IP Address"
		if n.end-n.content != 4 && n.end-n.content != 16 {
			return nil, x509ExtensionError()
		}
		info["Value"] = bytes.Clone(r.wire[n.content:n.end])
	case 0x88:
		n.name = "Registered ID"
		oid, err := r.implicitOID(n)
		if err != nil {
			return nil, err
		}
		info["OID"] = oid
	case 0xa4:
		n.name = "Directory Name"
		if len(n.children) != 1 {
			return nil, x509ExtensionError()
		}
		if err := r.name(n.children[0], "Name"); err != nil {
			return nil, err
		}
	case 0xa0:
		n.name = "Other Name"
		info["Type Specific Fields Decoded"] = false
		if len(n.children) != 2 || n.children[1].tag != 0xa0 || len(n.children[1].children) != 1 {
			return nil, x509ExtensionError()
		}
		oid, err := r.oid(n.children[0])
		if err != nil {
			return nil, err
		}
		info["OID"] = oid
		n.children[0].name, n.children[1].name = "Other Name Type OID", "Other Name Value"
	case 0xa3:
		n.name = "X400 Address"
		info["Type Specific Fields Decoded"] = false
	case 0xa5:
		n.name = "EDI Party Name"
		if len(n.children) < 1 || len(n.children) > 2 {
			return nil, x509ExtensionError()
		}
		for i, c := range n.children {
			want := byte(0xa1)
			if len(n.children) == 2 && i == 0 {
				want = 0xa0
			}
			if c.tag != want || len(c.children) != 1 {
				return nil, x509ExtensionError()
			}
			c.name = "Party Name"
			if want == 0xa0 {
				c.name = "Name Assigner"
			}
			if err := x509DisplayText(c.children[0], true); err != nil {
				return nil, err
			}
		}
	default:
		return nil, x509ExtensionError()
	}
	info["Type"] = n.name
	return info, nil
}

func (r *x509DERReader) generalNames(n *x509DERElement) ([]map[string]any, error) {
	if len(n.children) == 0 {
		return nil, x509ExtensionError()
	}
	var names []map[string]any
	for _, child := range n.children {
		value, err := r.generalName(child)
		if err != nil {
			return nil, err
		}
		names = append(names, value)
	}
	return names, nil
}

func x509DisplayText(n *x509DERElement, directory bool) error {
	allowed := n.tag == 12 || n.tag == 30
	if directory {
		allowed = allowed || n.tag == 19 || n.tag == 20 || n.tag == 28
	} else {
		allowed = allowed || n.tag == 22 || n.tag == 26
	}
	if !allowed {
		return x509ExtensionError()
	}
	n.name = "Display Text"
	return nil
}

func (r *x509DERReader) policyQualifier(n *x509DERElement) error {
	if err := x509ExtensionSequence(n, 2, 2); err != nil {
		return err
	}
	n.name, n.children[0].name = "Policy Qualifier", "Qualifier OID"
	oid, err := r.oid(n.children[0])
	if err != nil {
		return err
	}
	v := n.children[1]
	switch oid {
	case "1.3.6.1.5.5.7.2.1":
		v.name = "CPS URI"
		if v.tag != 22 {
			return x509ExtensionError()
		}
		_, err = r.extensionIA5(v)
		return err
	case "1.3.6.1.5.5.7.2.2":
		v.name = "User Notice"
		if err := x509ExtensionSequence(v, 0, 2); err != nil {
			return err
		}
		children := v.children
		if len(children) > 0 && children[0].tag == 0x30 {
			notice := children[0]
			notice.name = "Notice Reference"
			if err := x509ExtensionSequence(notice, 2, 2); err != nil {
				return err
			}
			if err := x509DisplayText(notice.children[0], false); err != nil {
				return err
			}
			notice.children[0].name = "Organization"
			numbers := notice.children[1]
			numbers.name = "Notice Numbers"
			if numbers.tag != 0x30 {
				return x509ExtensionError()
			}
			for _, number := range numbers.children {
				if number.tag != 2 {
					return x509ExtensionError()
				}
				number.name = "Notice Number"
			}
			children = children[1:]
		}
		if len(children) > 1 {
			return x509ExtensionError()
		}
		if len(children) == 1 {
			if err := x509DisplayText(children[0], false); err != nil {
				return err
			}
			children[0].name = "Explicit Text"
		}
	default:
		v.name = "Unknown Policy Qualifier Value"
	}
	return nil
}

func (r *x509DERReader) extension(oid string, value *x509DERElement) (map[string]any, bool, error) {
	info := map[string]any{"OID": oid, "Decoded": false}
	name, known := x509ExtensionNames[oid]
	if !known {
		info["Name"] = "Unknown Extension"
		return info, false, nil
	}
	root, err := r.element(value.content, value.end, value.depth+1)
	if err != nil {
		return nil, false, err
	}
	if root.end != value.end {
		return nil, false, fmt.Errorf("trailing bytes after extension value")
	}
	root.name = name
	info["Name"] = name
	if err := r.extensionBody(oid, root, info); err != nil {
		return nil, false, err
	}
	value.embedded = true
	value.children = []*x509DERElement{root}
	info["Decoded"] = true
	return info, true, nil
}

func (r *x509DERReader) extensionBody(oid string, n *x509DERElement, info map[string]any) error {
	switch oid {
	case "2.5.29.14":
		if n.tag != 4 {
			return x509ExtensionError()
		}
		info["Key Identifier"] = bytes.Clone(r.wire[n.content:n.end])
	case "2.5.29.15":
		if n.tag != 3 {
			return x509ExtensionError()
		}
		bits, err := r.extensionBits(n)
		if err != nil {
			return err
		}
		info["Set Bits"] = bits
	case "2.5.29.19":
		if err := x509ExtensionSequence(n, 0, 2); err != nil {
			return err
		}
		children := n.children
		ca := false
		if len(children) > 0 && children[0].tag == 1 {
			ca = r.wire[children[0].content] == 255
			if !ca {
				return fmt.Errorf("default CA false must be omitted")
			}
			children[0].name = "CA"
			children = children[1:]
		}
		info["CA"] = ca
		info["Path Length Present"] = false
		if len(children) > 1 {
			return x509ExtensionError()
		}
		if len(children) == 1 {
			if children[0].tag != 2 {
				return x509ExtensionError()
			}
			children[0].name = "Path Length Constraint"
			v, err := r.extensionInteger(children[0])
			if err != nil {
				return err
			}
			info["Path Length Present"], info["Path Length Decimal"] = true, v
		}
	case "2.5.29.17":
		if n.tag != 0x30 {
			return x509ExtensionError()
		}
		names, err := r.generalNames(n)
		if err != nil {
			return err
		}
		info["Names"] = names
	case "2.5.29.35":
		if err := x509ExtensionSequence(n, 0, 3); err != nil {
			return err
		}
		last := -1
		issuer, serial := false, false
		for _, c := range n.children {
			index := int(c.tag & 31)
			if index <= last || index > 2 {
				return x509ExtensionError()
			}
			last = index
			switch c.tag {
			case 0x80:
				c.name = "Key Identifier"
				info["Key Identifier"] = bytes.Clone(r.wire[c.content:c.end])
			case 0xa1:
				c.name = "Authority Certificate Issuer"
				v, err := r.generalNames(c)
				if err != nil {
					return err
				}
				info["Issuer Names"] = v
				issuer = true
			case 0x82:
				c.name = "Authority Certificate Serial Number"
				v, err := r.extensionInteger(c)
				if err != nil {
					return err
				}
				info["Serial Number Decimal"] = v
				serial = true
			default:
				return x509ExtensionError()
			}
		}
		if issuer != serial {
			return fmt.Errorf("authority issuer and serial must occur together")
		}
	case "2.5.29.37":
		if err := x509ExtensionSequence(n, 1, x509DERMaxNodes); err != nil {
			return err
		}
		var oids []string
		for _, c := range n.children {
			c.name = "Key Purpose OID"
			v, err := r.oid(c)
			if err != nil {
				return err
			}
			oids = append(oids, v)
		}
		info["Key Purpose OIDs"] = oids
	case "1.3.6.1.5.5.7.1.1":
		if err := x509ExtensionSequence(n, 1, x509DERMaxNodes); err != nil {
			return err
		}
		var descriptions []map[string]any
		for _, c := range n.children {
			if err := x509ExtensionSequence(c, 2, 2); err != nil {
				return err
			}
			c.name = "Access Description"
			c.children[0].name = "Access Method OID"
			method, err := r.oid(c.children[0])
			if err != nil {
				return err
			}
			location, err := r.generalName(c.children[1])
			if err != nil {
				return err
			}
			descriptions = append(descriptions, map[string]any{"Method OID": method, "Location": location})
		}
		info["Access Descriptions"] = descriptions
	case "2.5.29.16":
		if err := x509ExtensionSequence(n, 1, 2); err != nil {
			return err
		}
		last := -1
		for _, c := range n.children {
			index := int(c.tag & 31)
			if c.tag < 0x80 || c.tag > 0x81 || index <= last {
				return x509ExtensionError()
			}
			last = index
			name := "Not Before"
			if c.tag == 0x81 {
				name = "Not After"
			}
			copy := *c
			copy.tag = 24
			date, err := r.date(&copy, name)
			if err != nil {
				return err
			}
			c.name, c.valueType = name, "string"
			info[name+" UTC"] = date
		}
	case "2.5.29.32":
		if err := x509ExtensionSequence(n, 1, x509DERMaxNodes); err != nil {
			return err
		}
		var policies []string
		for _, c := range n.children {
			if err := x509ExtensionSequence(c, 1, 2); err != nil {
				return err
			}
			c.name = "Policy Information"
			c.children[0].name = "Policy OID"
			v, err := r.oid(c.children[0])
			if err != nil {
				return err
			}
			policies = append(policies, v)
			if len(c.children) == 2 {
				q := c.children[1]
				q.name = "Policy Qualifiers"
				if err := x509ExtensionSequence(q, 1, x509DERMaxNodes); err != nil {
					return err
				}
				for _, qualifier := range q.children {
					if err := r.policyQualifier(qualifier); err != nil {
						return err
					}
				}
			}
		}
		info["Policy OIDs"] = policies
	case "2.5.29.31":
		return r.distributionPoints(n, info)
	case "1.2.840.113533.7.65.0":
		// Wireshark v4.4.0 CertificateExtensions.asn: optional flag BIT STRING.
		if err := x509ExtensionSequence(n, 1, 2); err != nil {
			return err
		}
		if n.children[0].tag != 27 || len(n.children) == 2 && n.children[1].tag != 3 {
			return x509ExtensionError()
		}
		n.children[0].name = "Entrust Version"
		info["Version Bytes"] = bytes.Clone(r.wire[n.children[0].content:n.children[0].end])
		info["Flags Present"] = len(n.children) == 2
		if len(n.children) == 2 {
			n.children[1].name = "Entrust Info Flags"
			bits, err := r.extensionBits(n.children[1])
			if err != nil {
				return err
			}
			info["Set Flag Bits"] = bits
		}
	case "1.3.6.1.4.1.11129.2.4.2":
		return r.sctList(n, info)
	default:
		return x509ExtensionError()
	}
	return nil
}

func (r *x509DERReader) distributionPoints(n *x509DERElement, info map[string]any) error {
	if err := x509ExtensionSequence(n, 1, x509DERMaxNodes); err != nil {
		return err
	}
	var points []map[string]any
	for _, dp := range n.children {
		if err := x509ExtensionSequence(dp, 1, 3); err != nil {
			return err
		}
		dp.name = "Distribution Point"
		last := -1
		hasName, hasIssuer := false, false
		point := map[string]any{}
		for _, c := range dp.children {
			index := int(c.tag & 31)
			if index <= last || index > 2 {
				return x509ExtensionError()
			}
			last = index
			switch c.tag {
			case 0xa0:
				c.name = "Distribution Point Name"
				if len(c.children) != 1 {
					return x509ExtensionError()
				}
				choice := c.children[0]
				hasName = true
				if choice.tag == 0xa0 {
					choice.name = "Full Name"
					names, err := r.generalNames(choice)
					if err != nil {
						return err
					}
					point["Names"] = names
				} else if choice.tag == 0xa1 {
					// Reuse RDN validation without changing the implicit on-wire tag.
					copy := *choice
					copy.tag = 0x31
					wrapper := &x509DERElement{tag: 0x30, children: []*x509DERElement{&copy}}
					if err := r.name(wrapper, "Relative Name"); err != nil {
						return err
					}
					choice.name = "Name Relative To CRL Issuer"
					for i := 1; i < len(choice.children); i++ {
						a, b := choice.children[i-1], choice.children[i]
						if bytes.Compare(r.wire[a.start:a.end], r.wire[b.start:b.end]) > 0 {
							return x509ExtensionError()
						}
					}
				} else {
					return x509ExtensionError()
				}
			case 0x81:
				c.name = "Reasons"
				bits, err := r.extensionBits(c)
				if err != nil {
					return err
				}
				point["Reason Bits"] = bits
			case 0xa2:
				c.name = "CRL Issuer"
				names, err := r.generalNames(c)
				if err != nil {
					return err
				}
				point["Issuer Names"] = names
				hasIssuer = true
			default:
				return x509ExtensionError()
			}
		}
		if !hasName && !hasIssuer {
			return x509ExtensionError()
		}
		points = append(points, point)
	}
	info["Distribution Points"] = points
	return nil
}
