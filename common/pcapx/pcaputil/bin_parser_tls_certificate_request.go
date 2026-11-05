package pcaputil

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"strconv"
)

// TLSCertificateRequestFieldRange addresses one literal in a complete, owned
// handshake encoding (including its four-byte header). It never addresses the
// containing record, TCP segment or physical frame. Empty values have no span.
type TLSCertificateRequestFieldRange struct {
	Path           []string
	Offset, Length int
}

// TLSCertificateRequestView describes the bounded body grammar, not certificate
// trust, signature validity, handshake order or peer authentication.
type TLSCertificateRequestView struct {
	Fields map[string]any
	Ranges []TLSCertificateRequestFieldRange
}

// DecodeTLSCertificateRequest requires an observed negotiated version. A legacy
// record version alone cannot distinguish TLS 1.2 from TLS 1.3. Every returned
// octet slice is owned. Vector bounds and exact consumption are checked before
// applying the aggregate collection limit or constructing result entries.
// TLS 1.2 follows RFC 5246 7.4.4 with verified errata 2864/2865; TLS 1.3
// follows RFC 9846 4.4.2. Unknown TLS 1.3 extensions remain uninterpreted.
func DecodeTLSCertificateRequest(encoded []byte, version uint16, maxElements int) (*TLSCertificateRequestView, error) {
	if len(encoded) < 4 || encoded[0] != 13 || tlsU24(encoded[1:4]) != len(encoded)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS CertificateRequest handshake framing")
	}
	if version < 0x301 || version > 0x304 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS CertificateRequest needs negotiated version")
	}
	p := tlsCertificateRequestDecoder{wire: encoded, version: version}
	if err := p.parse(); err != nil {
		return nil, err
	}
	if maxElements < 0 || p.elements > maxElements {
		return nil, protocolError(ErrResourceExceeded, "TLS CertificateRequest collection limit")
	}
	p.build, p.elements, p.ranges = true, 0, nil
	if err := p.parse(); err != nil {
		return nil, err
	}
	return &TLSCertificateRequestView{Fields: p.fields, Ranges: p.ranges}, nil
}

type tlsCertificateRequestDecoder struct {
	wire     []byte
	version  uint16
	build    bool
	elements int
	fields   map[string]any
	ranges   []TLSCertificateRequestFieldRange
}

func (d *tlsCertificateRequestDecoder) bad() error {
	return protocolError(ErrMalformedMessage, "TLS CertificateRequest body vector")
}

func (d *tlsCertificateRequestDecoder) field(dst map[string]any, prefix []string, key string, value any, start, size int) {
	if !d.build {
		return
	}
	dst[key] = value
	if size > 0 {
		path := append(append([]string(nil), prefix...), key)
		d.ranges = append(d.ranges, TLSCertificateRequestFieldRange{Path: path, Offset: start, Length: size})
	}
}

func (d *tlsCertificateRequestDecoder) signatures(dst map[string]any, prefix []string, start, end int) (int, error) {
	if end-start < 2 {
		return 0, d.bad()
	}
	n := int(binary.BigEndian.Uint16(d.wire[start:]))
	if n < 2 || n > 65534 || n%2 != 0 || n > end-start-2 {
		return 0, d.bad()
	}
	d.field(dst, prefix, "Signature Algorithms Length", uint16(n), start, 2)
	d.elements += n / 2
	var entries []map[string]any
	if d.build {
		entries = make([]map[string]any, 0, n/2)
	}
	for pos := start + 2; pos < start+2+n; pos += 2 {
		if d.build {
			entry := map[string]any{}
			path := append(append([]string(nil), prefix...), "Signature Algorithms", strconv.Itoa(len(entries)))
			d.field(entry, path, "Algorithm", binary.BigEndian.Uint16(d.wire[pos:]), pos, 2)
			entries = append(entries, entry)
		}
	}
	if d.build {
		dst["Signature Algorithms"] = entries
	}
	return start + 2 + n, nil
}

func (d *tlsCertificateRequestDecoder) authorities(dst map[string]any, prefix []string, start, end int, nonempty bool) error {
	if end-start < 2 {
		return d.bad()
	}
	n := int(binary.BigEndian.Uint16(d.wire[start:]))
	if n != end-start-2 || nonempty && n < 3 {
		return d.bad()
	}
	d.field(dst, prefix, "Certificate Authorities Length", uint16(n), start, 2)
	var entries []map[string]any
	if d.build {
		entries = make([]map[string]any, 0)
	}
	for pos := start + 2; pos < end; {
		if end-pos < 2 {
			return d.bad()
		}
		size := int(binary.BigEndian.Uint16(d.wire[pos:]))
		if size < 1 || size > end-pos-2 {
			return d.bad()
		}
		d.elements++
		if d.build {
			der := bytes.Clone(d.wire[pos+2 : pos+2+size])
			var name pkix.RDNSequence
			rest, err := asn1.Unmarshal(der, &name)
			if err != nil || len(rest) != 0 {
				return protocolError(ErrMalformedMessage, "TLS CertificateRequest distinguished name DER")
			}
			entry := map[string]any{"Distinguished Name": name.String()}
			path := append(append([]string(nil), prefix...), "Certificate Authorities", strconv.Itoa(len(entries)))
			d.field(entry, path, "Length", uint16(size), pos, 2)
			d.field(entry, path, "DER", der, pos+2, size)
			entries = append(entries, entry)
		}
		pos += 2 + size
	}
	if d.build {
		dst["Certificate Authorities"] = entries
	}
	return nil
}

func (d *tlsCertificateRequestDecoder) parse() error {
	if d.build {
		d.fields = map[string]any{}
	}
	b, end, pos := d.wire, len(d.wire), 4
	if d.version != 0x304 {
		if pos >= end || b[pos] == 0 || int(b[pos]) > end-pos-1 {
			return d.bad()
		}
		n := int(b[pos])
		d.elements += n
		d.field(d.fields, nil, "Certificate Types Length", byte(n), pos, 1)
		pos++
		var types []map[string]any
		if d.build {
			types = make([]map[string]any, n)
			for i := range types {
				types[i] = map[string]any{}
				d.field(types[i], []string{"Certificate Types", strconv.Itoa(i)}, "Type", b[pos+i], pos+i, 1)
			}
			d.fields["Certificate Types"] = types
		}
		pos += n
		if d.version == 0x303 {
			var err error
			pos, err = d.signatures(d.fields, nil, pos, end)
			if err != nil {
				return err
			}
		}
		return d.authorities(d.fields, nil, pos, end, false)
	}
	if pos >= end || int(b[pos]) > end-pos-1 {
		return d.bad()
	}
	n := int(b[pos])
	d.field(d.fields, nil, "Context Length", byte(n), pos, 1)
	pos++
	if d.build {
		d.field(d.fields, nil, "Context", append([]byte{}, b[pos:pos+n]...), pos, n)
	}
	pos += n
	if end-pos < 2 || int(binary.BigEndian.Uint16(b[pos:])) != end-pos-2 {
		return d.bad()
	}
	d.field(d.fields, nil, "Extensions Length", binary.BigEndian.Uint16(b[pos:]), pos, 2)
	pos += 2
	var seen [1024]uint64
	var extensions []map[string]any
	if d.build {
		extensions = make([]map[string]any, 0)
	}
	for index := 0; pos < end; index++ {
		if end-pos < 4 {
			return d.bad()
		}
		typ, size := binary.BigEndian.Uint16(b[pos:]), int(binary.BigEndian.Uint16(b[pos+2:]))
		bit := uint64(1) << (typ % 64)
		if seen[typ/64]&bit != 0 || size > end-pos-4 {
			return d.bad()
		}
		seen[typ/64] |= bit
		d.elements++
		var entry map[string]any
		var path []string
		if d.build {
			entry = map[string]any{}
			path = []string{"Extensions", strconv.Itoa(index)}
		}
		d.field(entry, path, "Type", typ, pos, 2)
		d.field(entry, path, "Length", uint16(size), pos+2, 2)
		pos += 4
		switch typ {
		case 13, 50:
			last, err := d.signatures(entry, path, pos, pos+size)
			if err != nil {
				return err
			}
			if last != pos+size {
				return d.bad()
			}
		case 47:
			if err := d.authorities(entry, path, pos, pos+size, true); err != nil {
				return err
			}
		default:
			if d.build {
				d.field(entry, path, "Data", append([]byte{}, b[pos:pos+size]...), pos, size)
				entry["Interpretation"] = "uninterpreted extension"
			}
		}
		if d.build {
			extensions = append(extensions, entry)
		}
		pos += size
	}
	if seen[13/64]&(uint64(1)<<(13%64)) == 0 {
		return protocolError(ErrMalformedMessage, "TLS CertificateRequest missing signature_algorithms")
	}
	if d.build {
		d.fields["Extensions"] = extensions
	}
	return nil
}
