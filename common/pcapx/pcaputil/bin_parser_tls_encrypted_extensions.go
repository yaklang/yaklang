package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

// TLSEncryptedExtensionsFieldRange addresses the complete owned handshake,
// including its four-byte header, never the enclosing ciphertext or frame.
type TLSEncryptedExtensionsFieldRange = TLSCertificateRequestFieldRange

// TLSEncryptedExtensionsView decodes the extension envelope and ALPN selection.
// Other extension payloads remain opaque; this is not a negotiation, transcript
// or peer-authentication verdict.
type TLSEncryptedExtensionsView struct {
	Fields map[string]any
	Ranges []TLSEncryptedExtensionsFieldRange
}

// DecodeTLSEncryptedExtensions requires TLS 1.3 and the entire message. All
// returned byte slices are owned. Framing, duplicates, known RFC 8446 locations
// and ALPN grammar are checked before constructing output collections.
func DecodeTLSEncryptedExtensions(encoded []byte, version uint16, maxElements int) (*TLSEncryptedExtensionsView, error) {
	if version != 0x304 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS EncryptedExtensions needs TLS 1.3")
	}
	if len(encoded) < 6 || encoded[0] != 8 || tlsU24(encoded[1:4]) != len(encoded)-4 || int(binary.BigEndian.Uint16(encoded[4:])) != len(encoded)-6 {
		return nil, protocolError(ErrMalformedMessage, "TLS EncryptedExtensions handshake framing")
	}
	var seen [1024]uint64
	count := 0
	for at := 6; at < len(encoded); {
		if len(encoded)-at < 4 {
			return nil, protocolError(ErrMalformedMessage, "TLS extension header")
		}
		kind, n := binary.BigEndian.Uint16(encoded[at:]), int(binary.BigEndian.Uint16(encoded[at+2:]))
		at += 4
		word, bit := kind/64, uint64(1)<<(kind%64)
		if n > len(encoded)-at || seen[word]&bit != 0 {
			return nil, protocolError(ErrMalformedMessage, "TLS duplicate or truncated extension")
		}
		seen[word] |= bit
		if tlsForbiddenEncryptedExtension(kind) {
			return nil, protocolError(ErrMalformedMessage, "TLS forbidden EncryptedExtensions extension")
		}
		if kind == 16 {
			// Check the complete vector even when the collection budget is zero.
			if _, err := tlsALPNName(encoded[at:at+n], true); err != nil {
				return nil, err
			}
		}
		count++
		at += n
	}
	if maxElements < 0 || count > maxElements {
		return nil, protocolError(ErrResourceExceeded, "TLS EncryptedExtensions collection limit")
	}
	view := &TLSEncryptedExtensionsView{Fields: map[string]any{}}
	field := func(dst map[string]any, path []string, name string, value any, start, size int) {
		dst[name] = value
		if size > 0 {
			view.Ranges = append(view.Ranges, TLSEncryptedExtensionsFieldRange{Path: append(append([]string(nil), path...), name), Offset: start, Length: size})
		}
	}
	field(view.Fields, nil, "Extensions Length", binary.BigEndian.Uint16(encoded[4:]), 4, 2)
	extensions := make([]map[string]any, 0, count)
	for at := 6; at < len(encoded); {
		kind, n := binary.BigEndian.Uint16(encoded[at:]), int(binary.BigEndian.Uint16(encoded[at+2:]))
		path := []string{"Extensions", strconv.Itoa(len(extensions))}
		extension := map[string]any{}
		field(extension, path, "Type", kind, at, 2)
		field(extension, path, "Length", uint16(n), at+2, 2)
		at += 4
		field(extension, path, "Data", bytes.Clone(encoded[at:at+n]), at, n)
		if kind == 16 {
			field(extension, path, "ALPN Length", binary.BigEndian.Uint16(encoded[at:]), at, 2)
			field(extension, path, "Protocol Length", encoded[at+2], at+2, 1)
			// Protocol names are arbitrary octets, not necessarily UTF-8.
			field(extension, path, "Protocol", bytes.Clone(encoded[at+3:at+n]), at+3, n-3)
		}
		extensions = append(extensions, extension)
		at += n
	}
	view.Fields["Extensions"] = extensions
	return view, nil
}

// RFC 8446 section 4.2's registered-message table. New/unknown extensions are
// opaque, not assigned a speculative location or payload grammar.
func tlsForbiddenEncryptedExtension(kind uint16) bool {
	switch kind {
	case 5, 13, 18, 21, 41, 43, 44, 45, 47, 48, 49, 50, 51:
		return true
	}
	return false
}
