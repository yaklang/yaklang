package pcaputil

import (
	"bytes"
	"encoding/binary"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// TLSClientKeyExchangeView owns literals in a complete, cipher-selected TLS 1.2
// handshake. Its ranges address that encoding, not a ciphertext or capture frame.
// Public values and RSA ciphertext are opaque; no premaster or peer is verified.
type TLSClientKeyExchangeView struct {
	Profile string
	Fields  map[string]any
	Ranges  []TLSCertificateRequestFieldRange
}

func tls12ClientKeyExchangeProfile(suite uint16) string {
	server, rsa := tls12ServerKeyExchangeProfile(suite)
	switch {
	case rsa:
		return "RSAClient"
	case server == "ECDHEServer":
		return "ECDHEClient"
	case server == "DHEServer":
		return "DHEClient"
	default:
		return ""
	}
}

// DecodeTLS12ClientKeyExchange uses the canonical bin-parser grammar for an
// independently observed TLS 1.2 suite (RFC 5246 7.4.7; RFC 8422 5.7). Unknown,
// PSK and fixed-DH layouts must not be guessed from a plausible vector prefix.
func DecodeTLS12ClientKeyExchange(encoded []byte, version, suite uint16) (*TLSClientKeyExchangeView, error) {
	if version != 0x303 {
		return nil, protocolError(ErrUnsupportedVersion, "TLS ClientKeyExchange needs observed TLS 1.2")
	}
	profile := tls12ClientKeyExchangeProfile(suite)
	if profile == "" {
		return nil, protocolError(ErrContextRequired, "TLS ClientKeyExchange cipher layout unavailable")
	}
	// RFC 5246 7.4.7.2 and RFC 8422 5.7 retain implicit public values
	// carried by a suitable client certificate. Empty is not enough evidence
	// to select or reject that layout; the canonical vector profile is explicit.
	if profile != "RSAClient" && len(encoded) == 4 && bytes.Equal(encoded, []byte{16, 0, 0, 0}) {
		return nil, protocolError(ErrContextRequired, "TLS ClientKeyExchange implicit public-value certificate context unavailable")
	}
	literals, _, err := stream_parser.DecodeTLS12KeyExchangeLayout(encoded, profile)
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, err.Error())
	}
	view := &TLSClientKeyExchangeView{Profile: profile, Fields: map[string]any{}}
	for _, literal := range literals {
		if literal.Name == "Handshake Type" || literal.Name == "Handshake Length" {
			continue
		}
		start, end := literal.Start, literal.End
		var value any
		switch {
		case literal.Type == "raw":
			value = bytes.Clone(encoded[start:end])
		case end-start == 1:
			value = encoded[start]
		case end-start == 2:
			value = binary.BigEndian.Uint16(encoded[start:end])
		}
		view.Fields[literal.Name] = value
		if end > start {
			view.Ranges = append(view.Ranges, TLSCertificateRequestFieldRange{Path: []string{literal.Name}, Offset: start, Length: end - start})
		}
	}
	return view, nil
}

func (t *binTLS) admitClientKeyExchange(dir int, encoded []byte, authenticated bool) (*TLSClientKeyExchangeView, string, error) {
	if t.version == 0x304 {
		return nil, "", protocolError(ErrMalformedMessage, "TLS 1.3 forbids ClientKeyExchange")
	}
	if t.client >= 0 && dir != t.client || t.serverHelloDirectionKnown && dir == int(t.serverHelloDirection) {
		return nil, "", protocolError(ErrMalformedMessage, "server sent TLS ClientKeyExchange")
	}
	if t.version != 0x303 || tls12ClientKeyExchangeProfile(t.suite) == "" {
		return nil, "negotiated version or cipher layout unavailable; body uninterpreted", nil
	}
	if authenticated || t.ccs[dir] || t.app[dir] || t.clientKeyExchangeObserved {
		return nil, "", protocolError(ErrUnsupportedFeature, "TLS repeated or protected ClientKeyExchange requires a new epoch")
	}
	view, err := DecodeTLS12ClientKeyExchange(encoded, t.version, t.suite)
	if typed, ok := err.(*ProtocolError); ok && typed.Kind == ErrContextRequired {
		t.clientKeyExchangeObserved = true
		return nil, "implicit public-value certificate context unavailable; body uninterpreted", nil
	}
	if err != nil {
		return nil, "", err
	}
	t.clientKeyExchangeObserved = true
	status := "body vector decoded; public value, transcript and peer trust not evaluated"
	if view.Profile == "RSAClient" {
		status = "ciphertext vector decoded; premaster not decrypted; transcript and peer trust not evaluated"
	}
	if t.client < 0 && !t.serverHelloDirectionKnown {
		status += "; sender role unavailable"
	}
	return view, status, nil
}
