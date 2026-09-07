package stream_parser

import (
	"fmt"
	"math/big"
)

// These are bounded syntax profiles, not key construction or validation.
// RSA: RFC 3279 2.3.1. EC: RFC 5480 2.1.1 and 2.2.
// Parameter/algorithm combinations outside these profiles retain opaque bits.
func (r *x509DERReader) publicKey(spki *x509DERElement, oid string) (map[string]any, error) {
	info := map[string]any{"Algorithm OID": oid, "Decoded": false, "Parameters Supported": false, "Key Mathematics Validated": false, "Point Decompressed": false}
	algorithm, key := spki.children[0], spki.children[1]
	var parameter *x509DERElement
	if len(algorithm.children) == 2 {
		parameter = algorithm.children[1]
	}
	switch oid {
	case "1.2.840.113549.1.1.1":
		info["Layout"] = "RSA"
		if parameter == nil || parameter.tag != 5 {
			info["Opaque Reason"] = "RSA profile requires NULL parameters"
			return info, nil
		}
		info["Parameters Supported"] = true
		if r.wire[key.content] != 0 {
			return nil, fmt.Errorf("x509-public-key: RSA requires whole-octet bit string")
		}
		root, err := r.element(key.content+1, key.end, key.depth+1)
		if err != nil {
			return nil, err
		}
		if root.end != key.end || root.tag != 0x30 || len(root.children) != 2 {
			return nil, fmt.Errorf("x509-public-key: exact RSA integer pair required")
		}
		root.name = "RSA Public Key"
		for i, field := range root.children {
			if field.tag != 2 {
				return nil, fmt.Errorf("x509-public-key: RSA INTEGER required")
			}
			raw := r.wire[field.content:field.end]
			// Each encoded integer is capped independently, before big.Int conversion.
			if len(raw) > 8193 || raw[0]&128 != 0 {
				return nil, fmt.Errorf("x509-public-key: RSA integer negative or exceeds 8193 bytes")
			}
			number := new(big.Int).SetBytes(raw)
			if number.Sign() == 0 {
				return nil, fmt.Errorf("x509-public-key: RSA integer must be positive")
			}
			field.name = []string{"RSA Modulus", "RSA Public Exponent"}[i]
			if i == 0 {
				info["Modulus Bit Length"] = number.BitLen()
			} else {
				info["Public Exponent Decimal"] = number.String()
			}
		}
		key.payloadFields = []tlsCertificateField{tlsCertificateLeaf("Unused Bits", "uint8", key.content, key.content+1), {Name: "Bit String Bytes", Start: key.content + 1, End: key.end, Children: []tlsCertificateField{r.field(root)}}}
		info["Decoded"] = true
	case "1.2.840.10045.2.1":
		info["Layout"] = "EC Point"
		if parameter == nil || parameter.tag != 6 {
			info["Opaque Reason"] = "EC profile requires named-curve OID parameters"
			return info, nil
		}
		curve, err := r.oid(parameter)
		if err != nil {
			return nil, err
		}
		info["Named Curve OID"] = curve
		width := map[string]int{"1.2.840.10045.3.1.7": 32, "1.3.132.0.34": 48, "1.3.132.0.35": 66}[curve]
		if width == 0 {
			info["Opaque Reason"] = "unimplemented named-curve coordinate layout"
			return info, nil
		}
		info["Parameters Supported"] = true
		if r.wire[key.content] != 0 || key.end-key.content < 2 {
			return nil, fmt.Errorf("x509-public-key: EC requires nonempty whole-octet bit string")
		}
		start := key.content + 1
		format := r.wire[start]
		coordinates := 1
		switch format {
		case 4:
			coordinates = 2
		case 2, 3:
		default:
			return nil, fmt.Errorf("x509-public-key: unsupported EC point format")
		}
		if key.end-start != 1+coordinates*width {
			return nil, fmt.Errorf("x509-public-key: EC coordinate length differs from named curve")
		}
		fields := []tlsCertificateField{tlsCertificateLeaf("EC Point Format", "uint8", start, start+1), tlsCertificateLeaf("EC X Coordinate", "raw", start+1, start+1+width)}
		if coordinates == 2 {
			fields = append(fields, tlsCertificateLeaf("EC Y Coordinate", "raw", start+1+width, key.end))
		}
		key.payloadFields = []tlsCertificateField{tlsCertificateLeaf("Unused Bits", "uint8", key.content, key.content+1), {Name: "Bit String Bytes", Start: start, End: key.end, Children: fields}}
		info["Decoded"] = true
		info["Point Format"] = int(format)
		info["Coordinate Bytes"] = width
		info["Compressed"] = coordinates == 1
	default:
		info["Opaque Reason"] = "unimplemented public-key algorithm layout"
	}
	return info, nil
}
