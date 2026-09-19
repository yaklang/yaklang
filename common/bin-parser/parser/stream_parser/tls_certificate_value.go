package stream_parser

import "fmt"

// RFC 5246 7.4.2 and 7.4.6 certificate_list layout. The caller supplies a
// complete plaintext TLS 1.2 handshake and its boundary. Neither protocol
// version nor sender role can be established from this message in isolation.
// DER (or an alternatively negotiated certificate representation) stays opaque.
const (
	tlsCertificateMaxBytes   = 1 << 20
	tlsCertificateMaxEntries = 1024
)

type tlsCertificateField struct {
	Name, Type string
	// Empty inherits the enclosing grammar's byte order. Mixed-order wire
	// protocols may override an individual field without changing caller config.
	Endian     string
	Start, End int
	Children   []tlsCertificateField
	List       bool
}

func tlsCertificateLeaf(name, typ string, start, end int) tlsCertificateField {
	return tlsCertificateField{Name: name, Type: typ, Start: start, End: end}
}

func tlsCertificateUint24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }

func decodeTLSCertificateHandshake(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	fail := func(message string) ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tls-certificate: %s", message)
	}
	if len(wire) < 7 || len(wire) > tlsCertificateMaxBytes {
		return fail("complete handshake size outside 7..1048576 byte profile")
	}
	if wire[0] != 11 {
		return fail("Certificate handshake type 11 required")
	}
	if tlsCertificateUint24(wire[1:4]) != len(wire)-4 {
		return fail("handshake length differs from exact boundary")
	}
	if tlsCertificateUint24(wire[4:7]) != len(wire)-7 {
		return fail("certificate list length differs from handshake boundary")
	}
	fields := []tlsCertificateField{
		tlsCertificateLeaf("Handshake Type", "uint8", 0, 1),
		tlsCertificateLeaf("Handshake Length", "uint32", 1, 4),
		tlsCertificateLeaf("Certificate List Length", "uint32", 4, 7),
	}
	list := tlsCertificateField{Name: "Certificates", Start: 7, End: len(wire), List: true}
	for at := 7; at < len(wire); {
		if len(list.Children) >= tlsCertificateMaxEntries {
			return fail("certificate count exceeds 1024")
		}
		if len(wire)-at < 3 {
			return fail("truncated certificate length")
		}
		n := tlsCertificateUint24(wire[at : at+3])
		if n == 0 {
			return fail("certificate length must be nonzero")
		}
		if n > len(wire)-at-3 {
			return fail("certificate exceeds enclosing list")
		}
		list.Children = append(list.Children, tlsCertificateField{Name: "Certificate", Start: at, End: at + 3 + n, Children: []tlsCertificateField{
			tlsCertificateLeaf("Certificate Length", "uint32", at, at+3),
			tlsCertificateLeaf("Certificate DER", "raw", at+3, at+3+n),
		}})
		at += 3 + n
	}
	fields = append(fields, list)
	info := map[string]any{
		"Profile": "TLS 1.2 Certificate handshake layout", "Explicit Caller Selected Profile": true,
		"Version Is Caller Supplied": true, "Certificate Count": len(list.Children),
		"Empty Certificate List": len(list.Children) == 0, "DER Parsed": false,
		"Certificate Representation Negotiation Validated": false, "Sender Role Validated": false,
		"Server Certificate Requirements Validated": false, "Certificate Chain Validated": false,
		"Certificate Trust Validated": false, "Peer Identity Validated": false,
		"Handshake Completion Validated": false, "TCP Reassembly Performed": false,
		"Record Reassembly Performed": false,
	}
	return fields, info, nil
}
