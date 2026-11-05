package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// TLSCertificateStatusView owns a complete RFC 6066 OCSP vector. Ranges address
// the handshake encoding, never the encrypted record or a physical frame.
// The OCSP bytes are opaque: a vector is not a revocation or trust verdict.
type TLSCertificateStatusView struct {
	Fields map[string]any
	Ranges []TLSCertificateRequestFieldRange
}

func DecodeTLSCertificateStatus(encoded []byte, version uint16) (*TLSCertificateStatusView, error) {
	if version < 0x301 || version > 0x303 {
		return nil, protocolError(ErrUnsupportedVersion, "CertificateStatus requires observed TLS 1.0..1.2")
	}
	if len(encoded) > stream_parser.TLS12CertificateStatusMaxBytes {
		return nil, protocolError(ErrResourceExceeded, "TLS CertificateStatus exceeds bounded OCSP vector profile")
	}
	literals, _, err := stream_parser.DecodeTLS12CertificateStatusLayout(encoded)
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, err.Error())
	}
	v := &TLSCertificateStatusView{Fields: map[string]any{"Status Type Name": "ocsp", "OCSP DER Decoded": false,
		"OCSP Response Validated": false, "Certificate Trust Evaluated": false}}
	for _, f := range literals {
		if f.Name == "Handshake Type" || f.Name == "Handshake Length" {
			continue
		}
		switch f.Name {
		case "Status Type":
			v.Fields[f.Name] = encoded[f.Start]
		case "OCSP Response Length":
			v.Fields[f.Name] = uint32(tlsU24(encoded[f.Start:f.End]))
		case "OCSP Response":
			v.Fields[f.Name] = bytes.Clone(encoded[f.Start:f.End])
		}
		v.Ranges = append(v.Ranges, TLSCertificateRequestFieldRange{Path: []string{f.Name}, Offset: f.Start, Length: f.End - f.Start})
	}
	return v, nil
}

type tlsCertificateStatusState struct {
	offerKnown, offered, selectionKnown, selected bool
	v2Offered, v2Selected                         bool // RFC 6961 layout is deliberately unsupported here.
	observed                                      bool
	lastKnown                                     [2]bool
	last                                          [2]byte
}

func tlsStatusRequestStructure(v []byte, limit int) error {
	bad := func() error { return protocolError(ErrMalformedMessage, "TLS status_request OCSP request vector") }
	if len(v) == 0 {
		return bad()
	}
	if v[0] != 1 {
		return nil // Unknown request layout is opaque, not an OCSP offer.
	}
	if len(v) < 5 {
		return bad()
	}
	n := int(binary.BigEndian.Uint16(v[1:3]))
	if n > len(v)-5 {
		return bad()
	}
	end, count := 3+n, 0
	for at := 3; at < end; count++ {
		if end-at < 2 {
			return bad()
		}
		n := int(binary.BigEndian.Uint16(v[at:]))
		if n == 0 || n > end-at-2 {
			return bad()
		}
		at += 2 + n
		if count >= limit {
			return protocolError(ErrResourceExceeded, "TLS status_request responder count")
		}
	}
	if limit < 0 || int(binary.BigEndian.Uint16(v[end:end+2])) != len(v)-end-2 {
		return bad()
	}
	return nil
}

func (t *binTLS) observeCertificateStatusHello(h *tlsHelloView, client bool, limits ...int) error {
	v, present := h.ext[5]
	if client {
		_, t.certificateStatus.v2Offered = h.ext[17]
		limit := 4096
		if len(limits) > 0 {
			limit = limits[0]
		}
		if present {
			if err := tlsStatusRequestStructure(v, limit); err != nil {
				return err
			}
		}
		t.certificateStatus.offerKnown, t.certificateStatus.offered = true, present && len(v) > 0 && v[0] == 1
	} else {
		_, t.certificateStatus.v2Selected = h.ext[17]
		if present && len(v) != 0 {
			return protocolError(ErrMalformedMessage, "TLS ServerHello status_request acknowledgement must be empty")
		}
		t.certificateStatus.selectionKnown, t.certificateStatus.selected = true, present
	}
	return nil
}

func (t *binTLS) admitCertificateStatus(dir int, b []byte, authenticated bool) (*TLSCertificateStatusView, string, error) {
	if t.version == 0x304 {
		return nil, "", protocolError(ErrMalformedMessage, "TLS 1.3 forbids standalone CertificateStatus")
	}
	if t.client >= 0 && dir == t.client || t.serverHelloDirectionKnown && dir != int(t.serverHelloDirection) {
		return nil, "", protocolError(ErrMalformedMessage, "client sent server-only TLS CertificateStatus")
	}
	if authenticated || t.ccs[dir] || t.app[dir] || t.certificateStatus.observed {
		return nil, "", protocolError(ErrUnsupportedFeature, "TLS repeated or protected CertificateStatus requires a new epoch")
	}
	state := t.certificateStatus
	if state.v2Offered && state.v2Selected {
		return nil, "status_request_v2 negotiation unsupported; response body uninterpreted", nil
	}
	if state.offerKnown && !state.offered || state.selectionKnown && !state.selected {
		return nil, "", protocolError(ErrMalformedMessage, "TLS CertificateStatus lacks observed status_request offer or acknowledgement")
	}
	if state.lastKnown[dir] && state.last[dir] != 11 {
		// A known SKE/CR/later message cannot be erased by capture loss. For
		// a missing Certificate after an observed Hello, an actual gap leaves
		// order unavailable rather than manufacturing a missing-message error.
		if !t.handshakeGap[dir] || state.last[dir] != 2 {
			return nil, "", protocolError(ErrMalformedMessage, "TLS CertificateStatus must immediately follow server Certificate")
		}
	}
	if t.version < 0x301 || t.version > 0x303 {
		return nil, "negotiated version unavailable; body uninterpreted", nil
	}
	if len(b) >= 5 && b[4] != 1 {
		if state.offerKnown && state.offered {
			return nil, "", protocolError(ErrMalformedMessage, "TLS CertificateStatus status_type differs from observed OCSP request")
		}
		return nil, "unsupported status_type; response body uninterpreted", nil
	}
	view, err := DecodeTLSCertificateStatus(b, t.version)
	if err != nil {
		return nil, "", err
	}
	t.certificateStatus.observed = true
	missing := []string{}
	if !state.offerKnown || !state.selectionKnown {
		missing = append(missing, "Hello negotiation unavailable")
	}
	if !state.lastKnown[dir] || t.handshakeGap[dir] {
		missing = append(missing, "preceding message order unavailable")
	}
	if t.client < 0 && !t.serverHelloDirectionKnown {
		missing = append(missing, "sender role unavailable")
	}
	status := "complete OCSP vector decoded; DER, revocation status, signature and certificate trust not evaluated"
	if len(missing) > 0 {
		status = strings.Join(missing, "; ") + "; " + status
	}
	return view, status, nil
}
