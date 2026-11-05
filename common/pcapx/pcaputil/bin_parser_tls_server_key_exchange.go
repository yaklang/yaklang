package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// TLSServerKeyExchangeView owns the literal fields of one complete signed
// TLS 1.2 ephemeral key exchange. Ranges address the handshake, not its record.
// Parsing does not verify a signature, public value, transcript or peer trust.
type TLSServerKeyExchangeView struct {
	Fields map[string]any
	Ranges []TLSCertificateRequestFieldRange
}

// This explicit allowlist excludes PSK hints, anonymous exchanges and unknown
// layouts. Cipher suites are taken from the IANA TLS Cipher Suites registry;
// the signed bodies follow RFC 5246 7.4.3 and RFC 8422 5.4.
func tls12ServerKeyExchangeProfile(suite uint16) (profile string, forbidden bool) {
	switch suite {
	case 0xc009, 0xc00a, 0xc023, 0xc024, 0xc02b, 0xc02c, // ECDHE_ECDSA
		0xc013, 0xc014, 0xc027, 0xc028, 0xc02f, 0xc030, // ECDHE_RSA
		0xcca8, 0xcca9:
		return "ECDHEServer", false
	case 0x0033, 0x0039, 0x0067, 0x006b, 0x009e, 0x009f, 0xccaa, // DHE_RSA
		0x0032, 0x0038, 0x0040, 0x006a, 0x00a2, 0x00a3: // DHE_DSS
		return "DHEServer", false
	case 0x0005, 0x000a, 0x002f, 0x0035, 0x003c, 0x003d, 0x009c, 0x009d: // RSA
		return "", true
	}
	return "", false
}

// DecodeTLS12ServerKeyExchange shares the canonical bin-parser grammar, selected
// only by a separately observed TLS 1.2 cipher suite. It returns owned byte
// vectors and exact ranges; no connection state or algorithm is inferred.
func DecodeTLS12ServerKeyExchange(encoded []byte, version, suite uint16) (*TLSServerKeyExchangeView, error) {
	if version != 0x303 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS ServerKeyExchange needs observed TLS 1.2")
	}
	profile, forbidden := tls12ServerKeyExchangeProfile(suite)
	if forbidden {
		return nil, protocolError(ErrMalformedMessage, "TLS RSA key exchange forbids ServerKeyExchange")
	}
	if profile == "" {
		return nil, protocolError(ErrContextRequired, "TLS ServerKeyExchange cipher layout unavailable")
	}
	literals, _, err := stream_parser.DecodeTLS12KeyExchangeLayout(encoded, profile)
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, err.Error())
	}
	view := &TLSServerKeyExchangeView{Fields: map[string]any{}}
	for _, literal := range literals {
		if literal.Name == "Handshake Type" || literal.Name == "Handshake Length" || literal.Name == "Signature Algorithm" {
			continue
		}
		start, end, name := literal.Start, literal.End, literal.Name
		var value any
		switch {
		case name == "Hash Algorithm":
			// TLS 1.2 also uses combined SignatureScheme identifiers such as
			// rsa_pss_rsae_sha256 (0804). Do not mislabel 08 as an old hash.
			name, end = "Signature Algorithm", start+2
			value = binary.BigEndian.Uint16(encoded[start:end])
		case literal.Type == "raw":
			value = bytes.Clone(encoded[start:end])
		case end-start == 1:
			value = encoded[start]
		case end-start == 2:
			value = binary.BigEndian.Uint16(encoded[start:end])
		}
		view.Fields[name] = value
		if end > start {
			view.Ranges = append(view.Ranges, TLSCertificateRequestFieldRange{Path: []string{name}, Offset: start, Length: end - start})
		}
	}
	return view, nil
}

type tls12KeyExchangeOffers struct {
	observed, hasGroups, hasSignatures bool
	groups, signatures                 []byte // immutable, owned ClientHello vectors
}

func (t *binTLS) observeKeyExchangeOffers(h *tlsHelloView) {
	_, groups := h.ext[10]
	_, signatures := h.ext[13]
	t.keyExchangeOffers = tls12KeyExchangeOffers{observed: true, hasGroups: groups, hasSignatures: signatures,
		groups: bytes.Clone(h.ext[10]), signatures: bytes.Clone(h.ext[13])}
}

func tls12KeyExchangeOffered(v []byte, selected uint16, limit int, name string) error {
	if len(v) < 4 || int(binary.BigEndian.Uint16(v)) != len(v)-2 || (len(v)-2)%2 != 0 {
		return protocolError(ErrMalformedMessage, "TLS ServerKeyExchange captured ClientHello "+name+" vector")
	}
	if (len(v)-2)/2 > limit {
		return protocolError(ErrResourceExceeded, "TLS ServerKeyExchange ClientHello "+name+" count")
	}
	if !tlsContains16(v[2:], selected) {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS ServerKeyExchange %s 0x%04x was not offered in captured ClientHello", name, selected))
	}
	return nil
}

func tls12ServerKeyExchangeSigner(suite uint16) string {
	switch suite {
	case 0xc009, 0xc00a, 0xc023, 0xc024, 0xc02b, 0xc02c, 0xcca9:
		return "ECDSA/EdDSA"
	case 0x0032, 0x0038, 0x0040, 0x006a, 0x00a2, 0x00a3:
		return "DSA"
	default:
		return "RSA"
	}
}

func tls12SignatureSigner(algorithm uint16) string {
	switch algorithm {
	case 0x0201, 0x0301, 0x0401, 0x0501, 0x0601, 0x0804, 0x0805, 0x0806, 0x0809, 0x080a, 0x080b:
		return "RSA"
	case 0x0202, 0x0302, 0x0402, 0x0502, 0x0602:
		return "DSA"
	case 0x0203, 0x0303, 0x0403, 0x0503, 0x0603, 0x0807, 0x0808:
		return "ECDSA/EdDSA"
	}
	return "" // preserve unknown scheme literals without guessing a key family
}

func (t *binTLS) admitServerKeyExchange(dir int, encoded []byte, limits ...int) (*TLSServerKeyExchangeView, string, error) {
	if t.version == 0x304 {
		return nil, "", protocolError(ErrMalformedMessage, "TLS 1.3 forbids ServerKeyExchange")
	}
	if t.client >= 0 && dir == t.client || t.serverHelloDirectionKnown && dir != int(t.serverHelloDirection) {
		return nil, "", protocolError(ErrMalformedMessage, "client sent TLS ServerKeyExchange")
	}
	profile, forbidden := tls12ServerKeyExchangeProfile(t.suite)
	if t.version != 0x303 || profile == "" && !forbidden {
		return nil, "negotiated version or cipher layout unavailable; body uninterpreted", nil
	}
	view, err := DecodeTLS12ServerKeyExchange(encoded, t.version, t.suite)
	if err != nil {
		return nil, "", err
	}
	limit := 4096
	if len(limits) > 0 {
		limit = limits[0]
	}
	algorithm := view.Fields["Signature Algorithm"].(uint16)
	if tlsGREASE16(algorithm) {
		return nil, "", protocolError(ErrMalformedMessage, fmt.Sprintf("TLS ServerKeyExchange selects reserved GREASE signature algorithm 0x%04x", algorithm))
	}
	if signer := tls12SignatureSigner(algorithm); signer != "" && signer != tls12ServerKeyExchangeSigner(t.suite) {
		return nil, "", protocolError(ErrMalformedMessage, "TLS ServerKeyExchange signature algorithm disagrees with cipher authentication family")
	}
	if group, ok := view.Fields["Named Group"].(uint16); ok {
		if tlsGREASE16(group) {
			return nil, "", protocolError(ErrMalformedMessage, fmt.Sprintf("TLS ServerKeyExchange selects reserved GREASE named group 0x%04x", group))
		}
		if t.keyExchangeOffers.hasGroups {
			if err := tls12KeyExchangeOffered(t.keyExchangeOffers.groups, group, limit, "supported_groups"); err != nil {
				return nil, "", err
			}
		}
	}
	if t.keyExchangeOffers.hasSignatures {
		if err := tls12KeyExchangeOffered(t.keyExchangeOffers.signatures, algorithm, limit, "signature_algorithms"); err != nil {
			return nil, "", err
		}
	}
	status := "body vectors decoded; signature not evaluated; public values, transcript and peer trust not evaluated"
	if !t.keyExchangeOffers.observed {
		status += "; ClientHello offer context unavailable"
	}
	return view, status, nil
}
