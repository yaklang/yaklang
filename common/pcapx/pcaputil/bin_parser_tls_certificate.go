package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

// TLSCertificateFieldRange addresses a literal in the complete owned handshake
// encoding, never in its enclosing record or captured ciphertext.
type TLSCertificateFieldRange = TLSCertificateRequestFieldRange

// TLSCertificateView separates wire vectors from the existing X.509 summaries.
// Parsing these fields does not establish trust, signature validity, negotiated
// certificate type, a matching request context, or handshake conformance.
type TLSCertificateView struct {
	Fields       map[string]any
	Ranges       []TLSCertificateFieldRange
	Certificates []map[string]any
}

// DecodeTLSCertificate requires an observed version and complete handshake. All
// byte values are owned. Entry and extension admission precedes allocation of
// output vectors, certificate hashing and X.509 interpretation.
func DecodeTLSCertificate(encoded []byte, version uint16, maxElements int) (*TLSCertificateView, error) {
	if len(encoded) < 4 || encoded[0] != 11 || tlsU24(encoded[1:4]) != len(encoded)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS Certificate handshake framing")
	}
	if version < 0x301 || version > 0x304 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS Certificate needs negotiated version")
	}
	certs, err := tlsCertificates(encoded[4:], version == 0x304, maxElements)
	if err != nil {
		return nil, err
	}
	view := &TLSCertificateView{Fields: map[string]any{}, Certificates: certs}
	field := func(dst map[string]any, path []string, name string, value any, start, size int) {
		dst[name] = value
		if size > 0 {
			view.Ranges = append(view.Ranges, TLSCertificateFieldRange{
				Path: append(append([]string(nil), path...), name), Offset: start, Length: size,
			})
		}
	}
	p := 4
	if version == 0x304 {
		n := int(encoded[p])
		field(view.Fields, nil, "Context Length", encoded[p], p, 1)
		p++
		field(view.Fields, nil, "Context", bytes.Clone(encoded[p:p+n]), p, n)
		p += n
	}
	field(view.Fields, nil, "Certificate List Length", uint32(tlsU24(encoded[p:])), p, 3)
	p += 3
	entries := make([]map[string]any, 0, len(certs))
	for p < len(encoded) {
		path := []string{"Certificate Entries", strconv.Itoa(len(entries))}
		entry := map[string]any{}
		n := tlsU24(encoded[p:])
		field(entry, path, "Certificate Length", uint32(n), p, 3)
		p += 3
		field(entry, path, "Certificate Data", bytes.Clone(encoded[p:p+n]), p, n)
		p += n
		if version == 0x304 {
			n = int(binary.BigEndian.Uint16(encoded[p:]))
			field(entry, path, "Extensions Length", uint16(n), p, 2)
			p += 2
			end := p + n
			extensions := []map[string]any{}
			for p < end {
				extPath := append(append([]string(nil), path...), "Extensions", strconv.Itoa(len(extensions)))
				ext := map[string]any{}
				field(ext, extPath, "Type", binary.BigEndian.Uint16(encoded[p:]), p, 2)
				n = int(binary.BigEndian.Uint16(encoded[p+2:]))
				field(ext, extPath, "Length", uint16(n), p+2, 2)
				p += 4
				field(ext, extPath, "Data", bytes.Clone(encoded[p:p+n]), p, n)
				p += n
				extensions = append(extensions, ext)
			}
			entry["Extensions"] = extensions
		}
		entries = append(entries, entry)
	}
	view.Fields["Certificate Entries"] = entries
	return view, nil
}
