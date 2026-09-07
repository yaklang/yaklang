package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// RFC 5246 4.7, 7.4.4 and 7.4.8. This is an explicitly selected TLS 1.2
// layout, not a version detector or transcript/signature validator. The
// CertificateRequest signature vector has a two-octet length (erratum 1585
// corrects the appendix's omission). This profile requires a nonempty list
// of pairs as in 7.4.1.4.1; it does not infer usable/negotiated algorithms.
const (
	tlsCertificateAuthMaxBytes   = 131333 // all three Request vectors at wire maxima
	tlsCertificateAuthMaxEntries = 1024   // local limit per signature/name list
)

type tlsCertificateAuthField struct {
	Name, Type string
	Start, End int
	Children   []tlsCertificateAuthField
	List       bool
}

func tlsCertificateAuthLeaf(name, typ string, start, end int) tlsCertificateAuthField {
	return tlsCertificateAuthField{Name: name, Type: typ, Start: start, End: end}
}

func decodeTLS12CertificateAuth(wire []byte, messageType uint8) ([]tlsCertificateAuthField, map[string]any, error) {
	fail := func(message string) ([]tlsCertificateAuthField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tls-certificate-auth: %s", message)
	}
	if messageType != 13 && messageType != 15 {
		return fail("unsupported caller-selected handshake type")
	}
	if len(wire) < 4 || len(wire) > tlsCertificateAuthMaxBytes {
		return fail("handshake exceeds explicit 4..131333 byte profile")
	}
	if wire[0] != messageType {
		return fail("handshake type differs from selected entry")
	}
	if int(wire[1])<<16|int(wire[2])<<8|int(wire[3]) != len(wire)-4 {
		return fail("handshake length differs from exact boundary")
	}
	fields := []tlsCertificateAuthField{
		tlsCertificateAuthLeaf("Handshake Type", "uint8", 0, 1),
		tlsCertificateAuthLeaf("Handshake Length", "uint32", 1, 4),
	}
	info := map[string]any{
		"Version Is Caller Supplied": true, "Explicit Caller Selected Profile": true,
		"Sender Conformance Validated": false, "Registry Values Validated": false,
		"Algorithm Usage Validated": false, "Certificate Compatibility Validated": false,
		"Request Correlation Validated": false, "Handshake Transcript Validated": false,
		"Signature Verified": false, "Peer Identity Validated": false,
		"Handshake Completion Validated": false, "TCP Reassembly Performed": false,
		"Record Reassembly Performed": false, "Distinguished Names DER Parsed": false,
	}
	if messageType == 15 {
		if len(wire) < 8 {
			return fail("truncated DigitallySigned header")
		}
		n := int(binary.BigEndian.Uint16(wire[6:8]))
		if n != len(wire)-8 {
			return fail("signature length differs from handshake boundary")
		}
		// Zero-length signatures are legal opaque-vector encodings, not proof
		// of a valid signature. Unknown algorithm identifiers remain values.
		fields = append(fields,
			tlsCertificateAuthLeaf("Hash Algorithm", "uint8", 4, 5),
			tlsCertificateAuthLeaf("Signature Algorithm", "uint8", 5, 6),
			tlsCertificateAuthLeaf("Signature Length", "uint16", 6, 8),
			tlsCertificateAuthLeaf("Signature", "raw", 8, len(wire)),
		)
		info["Profile"] = "TLS 1.2 CertificateVerify layout"
		info["Empty Signature"] = n == 0
		return fields, info, nil
	}
	if len(wire) < 5 || wire[4] == 0 {
		return fail("nonempty certificate type vector required")
	}
	at := 5 + int(wire[4])
	if at > len(wire) {
		return fail("certificate types exceed handshake")
	}
	fields = append(fields, tlsCertificateAuthLeaf("Certificate Types Length", "uint8", 4, 5))
	types := tlsCertificateAuthField{Name: "Certificate Types", Start: 5, End: at, List: true}
	for p := 5; p < at; p++ {
		types.Children = append(types.Children, tlsCertificateAuthLeaf("Certificate Type", "uint8", p, p+1))
	}
	fields = append(fields, types)
	if len(wire)-at < 2 {
		return fail("truncated signature algorithm vector length")
	}
	sigBytes := int(binary.BigEndian.Uint16(wire[at : at+2]))
	fields = append(fields, tlsCertificateAuthLeaf("Signature Algorithms Length", "uint16", at, at+2))
	at += 2
	if sigBytes < 2 || sigBytes%2 != 0 || sigBytes > len(wire)-at {
		return fail("nonempty complete signature algorithm pairs required")
	}
	if sigBytes/2 > tlsCertificateAuthMaxEntries {
		return fail("signature algorithm count exceeds 1024")
	}
	algorithms := tlsCertificateAuthField{Name: "Signature Algorithms", Start: at, End: at + sigBytes, List: true}
	for end := at + sigBytes; at < end; at += 2 {
		algorithms.Children = append(algorithms.Children, tlsCertificateAuthField{Name: "Signature And Hash Algorithm", Start: at, End: at + 2, Children: []tlsCertificateAuthField{
			tlsCertificateAuthLeaf("Hash Algorithm", "uint8", at, at+1),
			tlsCertificateAuthLeaf("Signature Algorithm", "uint8", at+1, at+2),
		}})
	}
	fields = append(fields, algorithms)
	if len(wire)-at < 2 {
		return fail("truncated distinguished name vector length")
	}
	nameBytes := int(binary.BigEndian.Uint16(wire[at : at+2]))
	fields = append(fields, tlsCertificateAuthLeaf("Distinguished Names Length", "uint16", at, at+2))
	at += 2
	if nameBytes != len(wire)-at {
		return fail("distinguished name vector differs from handshake boundary")
	}
	names := tlsCertificateAuthField{Name: "Distinguished Names", Start: at, End: len(wire), List: true}
	for at < len(wire) {
		if len(names.Children) >= tlsCertificateAuthMaxEntries {
			return fail("distinguished name count exceeds 1024")
		}
		if len(wire)-at < 2 {
			return fail("truncated distinguished name length")
		}
		n := int(binary.BigEndian.Uint16(wire[at : at+2]))
		if n == 0 || n > len(wire)-at-2 {
			return fail("nonempty distinguished name exceeds enclosing vector")
		}
		names.Children = append(names.Children, tlsCertificateAuthField{Name: "Distinguished Name", Start: at, End: at + 2 + n, Children: []tlsCertificateAuthField{
			tlsCertificateAuthLeaf("Distinguished Name Length", "uint16", at, at+2),
			tlsCertificateAuthLeaf("Distinguished Name DER", "raw", at+2, at+2+n),
		}})
		at += 2 + n
	}
	fields = append(fields, names)
	info["Profile"] = "TLS 1.2 CertificateRequest layout"
	info["Certificate Type Count"] = len(types.Children)
	info["Signature Algorithm Count"] = len(algorithms.Children)
	info["Distinguished Name Count"] = len(names.Children)
	return fields, info, nil
}
