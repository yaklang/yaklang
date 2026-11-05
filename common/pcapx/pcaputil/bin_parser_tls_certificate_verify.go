package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// TLSCertificateVerifyView describes a complete handshake body. Its signature
// bytes are owned. SignatureOffset addresses the complete handshake encoding,
// never an enclosing ciphertext record or physical frame.
type TLSCertificateVerifyView struct {
	Fields          map[string]any
	SignatureOffset int
}

// DecodeTLSCertificateVerify checks the exact envelope and signature vector.
// TLS 1.0/1.1 have no algorithm field; TLS 1.2/1.3 carry a complete two-byte
// identifier. An unknown identifier or an empty opaque vector remains literal
// evidence, not proof of an allowed scheme, valid signature or handshake order.
// The negotiated version must be observed independently of the record version.
func DecodeTLSCertificateVerify(encoded []byte, version uint16) (*TLSCertificateVerifyView, error) {
	if len(encoded) < 4 || encoded[0] != 15 || tlsU24(encoded[1:4]) != len(encoded)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS CertificateVerify handshake framing")
	}
	if version < 0x301 || version > 0x304 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS CertificateVerify needs negotiated version")
	}
	lengthOffset := 4
	if version >= 0x303 {
		lengthOffset = 6
	}
	if len(encoded) < lengthOffset+2 {
		return nil, protocolError(ErrMalformedMessage, "TLS CertificateVerify signature vector header")
	}
	n := binary.BigEndian.Uint16(encoded[lengthOffset:])
	if int(n) != len(encoded)-lengthOffset-2 {
		return nil, protocolError(ErrMalformedMessage, "TLS CertificateVerify signature vector length")
	}
	fields := map[string]any{"Signature Length": n, "Signature": bytes.Clone(encoded[lengthOffset+2:])}
	if version >= 0x303 {
		fields["Signature Algorithm"] = binary.BigEndian.Uint16(encoded[4:])
	}
	return &TLSCertificateVerifyView{Fields: fields, SignatureOffset: lengthOffset + 2}, nil
}
